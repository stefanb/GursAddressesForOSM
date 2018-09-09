package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/jonas-p/go-shp"
	"github.com/paulmach/go.geojson"
	"golang.org/x/text/encoding/charmap"
)

var inputShapeFileName = flag.String("in", "data/temp/HS-etrs89/SI.GURS.RPE.PUB.HS-etrs89.shp", "Input ShapeFile to read")
var outputGeoJSONFileName = flag.String("out", "data/slovenia-housenumbers.geojson", "Output GeoJSON file to save")

// Reads 2 columns from shapeFileName and returns them as a map
func readShapefileToMap(shapeFileName string, keyColumnName, valueColumnName string) map[string]string {
	result := make(map[string]string)

	var keyColumnIndex, valueColumnIndex = -1, -1

	//log.Printf("Reading %s... into map %s:%s", shapeFileName, keyColumnName, valueColumnName)
	shapeReader, err := shp.Open(shapeFileName)
	if err != nil {
		log.Fatal(err)
	}
	defer shapeReader.Close()

	for i, v := range shapeReader.Fields() {
		/*
			if keyColumnIndex != -1 && v.String() == keyColumnName {
				keyColumnIndex = i
			} else if valueColumnIndex != -1 && v.String() == valueColumnName {
				valueColumnIndex = i
			}
		*/
		if v.String() == keyColumnName {
			keyColumnIndex = i
		} else if v.String() == valueColumnName {
			valueColumnIndex = i
		}

		if valueColumnIndex != -1 && keyColumnIndex != -1 {
			break
		}
	}

	// loop through all features in the shapefile
	//i := 0
	var valueUtf string
	for shapeReader.Next() {
		//i++
		valueUtf = DecodeWindows1250(shapeReader.Attribute(valueColumnIndex))
		valueUtf = strings.Trim(valueUtf, "\u0000") // trim null characters to remove null strings (when no bilingual name)

		if len(valueUtf) > 0 {
			result[DecodeWindows1250(shapeReader.Attribute(keyColumnIndex))] = valueUtf
		}
	}

	// Convert map to slice of key-value pairs to show as sample records.
	const maxSamplesCount = 10
	samples := [][]string{}
	for key, value := range result {
		samples = append(samples, []string{key, value})
		if len(samples) >= maxSamplesCount {
			break
		}
	}

	if len(result) == 0 {
		log.Printf("WARNING: %s read NO records!", shapeFileName)
	} else {
		//log.Printf("%s: read %d/%d records, eg: %s", shapeFileName, len(result), i, samples)
	}
	return result
}

const (
	// 7 decimals
	roundingFactor = 10000000

	// OpenStreetMap tags:
	tagHousenumber = "addr:housenumber"
	tagCity        = "addr:city"
	tagPostCode    = "addr:postcode"
	tagStreet      = "addr:street"
	tagPlace       = "addr:place"
	tagSourceDate  = "source:addr:date"
	tagSource      = "source:addr"
	tagSourceValue = "GURS"

	// could be either "source:addr:ref", "source:ref", "ref:GURS:HS_MID"
	tagRef = "ref:GURS:HS_MID"

	tagLangPostfixSlovenian = ":sl"
	tagLangPostfixItalian   = ":it"
	tagLangPostfixHungarian = ":hu"
	bilingualSeparator      = " / "
)

// lookup maps
var ptCodeMap, ptNameMap, ulNameMap, ulNameDjMap, naNameMap, naNameDjMap map[string]string

// ReadLookups reads all needed shapefiles in parallel to maps memory for later use
func ReadLookups() {
	var wg sync.WaitGroup
	wg.Add(6)

	go func() {
		ptCodeMap = readShapefileToMap("data/temp/PT/SI.GURS.RPE.PUB.PT.shp", "PT_MID", "PT_ID")
		wg.Done()
	}()

	go func() {
		ptNameMap = readShapefileToMap("data/temp/PT/SI.GURS.RPE.PUB.PT.shp", "PT_MID", "PT_UIME")
		wg.Done()
	}()

	go func() {
		ulNameMap = readShapefileToMap("data/temp/UL/SI.GURS.RPE.PUB.UL.shp", "UL_MID", "UL_UIME")
		wg.Done()
	}()

	go func() {
		ulNameDjMap = readShapefileToMap("data/temp/UL/SI.GURS.RPE.PUB.UL.shp", "UL_MID", "UL_DJ")
		wg.Done()
	}()

	go func() {
		naNameMap = readShapefileToMap("data/temp/NA/SI.GURS.RPE.PUB.NA.shp", "NA_MID", "NA_UIME")
		wg.Done()
	}()

	go func() {
		naNameDjMap = readShapefileToMap("data/temp/NA/SI.GURS.RPE.PUB.NA.shp", "NA_MID", "NA_DJ")
		wg.Done()
	}()

	wg.Wait()
}

// ReadShapefile reads the given shapefile and returns the geoJson
func ReadShapefile(shapefilename string) *geojson.FeatureCollection {

	//log.Printf("Reading %s...", shapefilename)

	// open a shapefile for reading
	// shape, err := shp.Open("points.shp")
	shapeReader, err := shp.Open(shapefilename)
	if err != nil {
		log.Fatal(err)
	}
	defer shapeReader.Close()

	// fields from the attribute table (DBF)
	//	fields := shape.Fields()

	featureCollection := geojson.NewFeatureCollection()
	//i := 0

	// loop through all features in the shapefile
	for shapeReader.Next() {
		//i++
		//		n, p := shapeReader.Shape()
		_, p := shapeReader.Shape()

		//shapeReader
		// to benchmark just the reading:
		//featureCollection.AddFeature(geojson.NewPointFeature([]float64{0,0}))
		//continue

		//n, _ := shapeReader.Shape()

		/*if i>30 {
			fmt.Println("Had enough")
			break
		}*/

		if shapeReader.Attribute(12) != "V" {
			fmt.Println("skipping invalid...")
			continue
		}

		// print feature
		//		fmt.Println(reflect.TypeOf(p).Elem(), p.BBox())

		bb := p.BBox()
		// prepare rounded coordinates:
		lat := math.Round(bb.MinY*roundingFactor) / roundingFactor
		lon := math.Round(bb.MinX*roundingFactor) / roundingFactor
		f := geojson.NewPointFeature([]float64{lat, lon})

		/*
		   http://www.e-prostor.gov.si/fileadmin/struktura/RPE_struktura.pdf
		   idx#	Ime polja Definicija polja Opis polja
		   0	ENOTA C 2 Šifra enote
		   1	HS_MID N 8.0 Identifikator hišne številke
		   2	HS N 3.0 Hišna številka
		   3	HD C 1 Dodatek k hišni številki
		   4	LABELA C 4 Hišna številka z dodatkom – združen zapis polj HS in HD
		   5	UL_MID N 8.0 Identifikator ulice
		   6	NA_MID N 8.0 Identifikator naselja
		   7	OB_MID N 8.0 Identifikator občine
		   8	PT_MID N 8.0 Identifikator poštnega okoliša
		   9	PO_MID N 8.0 Identifikator prostorskega okoliša
		   10	D_OD D 8 Datum veljavnosti
		   11	DV_OD D 8 Datum vnosa v bazo
		   12	STATUS C 1 Status veljavnosti zapisa (V – veljavno stanje)
		   13	Y_C N 6.0 Y koordinata centroida hišne številke
		   14	X_C N 6.0 X koordinata centroida hišne številke
		*/
		labela := shapeReader.Attribute(4)

		f.SetProperty(tagHousenumber, strings.ToLower(DecodeWindows1250(labela)))

		ulMid := shapeReader.Attribute(5)

		if ulName, streetNameExists := ulNameMap[ulMid]; streetNameExists {
			// street name exists

			if ulNameDj, bilingualStreetNameExists := ulNameDjMap[ulMid]; bilingualStreetNameExists && ulNameDj != ulName {
				// bilingual street name exists
				f.SetProperty(tagStreet, ulName+bilingualSeparator+ulNameDj)
				//f.SetProperty(tagStreet, strings.Join([]string{ulName, bilingualSeparator, ulNameDj}, ""))
				f.SetProperty(tagStreet+tagLangPostfixSlovenian, ulName)
				f.SetProperty(ApplyTagLanguagePostfix(tagStreet, lon), ulNameDj)
			} else {
				// only slovenian name
				f.SetProperty(tagStreet, ulName)
			}
		} else {
			// no street name, only place
			naMid := shapeReader.Attribute(6)
			naName := naNameMap[naMid]

			if naNameDj, bilingualPlaceNameExists := naNameDjMap[naMid]; bilingualPlaceNameExists && naNameDj != naName {
				// bilingual place name exists
				f.SetProperty(tagPlace, naName+bilingualSeparator+naNameDj)
				//f.SetProperty(tagStreet, strings.Join([]string{naName, bilingualSeparator, naNameDj}, ""))
				f.SetProperty(tagPlace+tagLangPostfixSlovenian, naName)
				f.SetProperty(ApplyTagLanguagePostfix(tagPlace, lon), naNameDj)
			} else {
				// only slovenian name
				f.SetProperty(tagPlace, naName)
			}

		}

		ptMid := shapeReader.Attribute(8)
		f.SetProperty(tagPostCode, ptCodeMap[ptMid])

		f.SetProperty(tagCity, ptNameMap[ptMid])

		dateOd := shapeReader.Attribute(10)
		// slice it up into nice iso YYYY-MM-DD format:
		f.SetProperty(tagSourceDate, dateOd[0:4]+"-"+dateOd[4:6]+"-"+dateOd[6:8])
		//f.SetProperty(tagSourceDate, fmt.Sprintf("%s-%s-%s", dateOd[0:4], dateOd[4:6], dateOd[6:8]))

		f.SetProperty(tagSource, tagSourceValue)

		hsMid := shapeReader.Attribute(1)
		f.SetProperty(tagRef, hsMid)

		featureCollection.AddFeature(f)
	}

	return featureCollection
}

// SortFeatureCollection sorts the Features of the given FeatureCollection for reproducable results and better compression
func SortFeatureCollection(featureCollection geojson.FeatureCollection) {

	sort.Slice(featureCollection.Features[:], func(i, j int) bool {
		PostLeft := featureCollection.Features[i].Properties[tagPostCode].(string)
		PostRight := featureCollection.Features[j].Properties[tagPostCode].(string)
		if PostLeft < PostRight {
			return true
		}
		if PostLeft > PostRight {
			return false
		}

		if featureCollection.Features[i].Properties[tagStreet] != nil {
			if featureCollection.Features[j].Properties[tagStreet] != nil {
				StreetLeft := featureCollection.Features[i].Properties[tagStreet].(string)
				StreetRight := featureCollection.Features[j].Properties[tagStreet].(string)
				if StreetLeft < StreetRight {
					return true
				}
				if StreetLeft > StreetRight {
					return false
				}
			}
		}

		if featureCollection.Features[i].Properties[tagPlace] != nil {
			if featureCollection.Features[j].Properties[tagPlace] != nil {
				PlaceLeft := featureCollection.Features[i].Properties[tagPlace].(string)
				PlaceRight := featureCollection.Features[j].Properties[tagPlace].(string)
				if PlaceLeft < PlaceRight {
					return true
				}
				if PlaceLeft > PlaceRight {
					return false
				}
			}
		}

		return NormalizeHouseNumber(featureCollection.Features[i].Properties[tagHousenumber].(string)) < NormalizeHouseNumber(featureCollection.Features[j].Properties[tagHousenumber].(string))
	})
}

// NormalizeHouseNumber returns comparable house number (4 digits, followed by one letter or _)
func NormalizeHouseNumber(housenumber string) string {

	if lastRune, n := utf8.DecodeLastRuneInString(housenumber); n == 0 || unicode.IsDigit(lastRune) {
		// pad right side with _ if it ends with digit to accommodate for 1-letter suffixes
		// pad left with zeros to get to 3-digits
		// eg "12" -> "012_" (4 characters)
		return fmt.Sprintf("%03s_", housenumber)
	}

	// there is already a letter at the end,
	// just pad left side with zeros to keep all lengths equal to 4 characters
	// eg "12c" -> "012c" (4 characters)
	return fmt.Sprintf("%04s", housenumber)
}

// ApplyTagLanguagePostfix applies language postfix to the given prefix based on longitude
func ApplyTagLanguagePostfix(prefix string, longitude float64) string {

	// Bilingual names with longitude greater than (right, east of this meridian) are considered in Hungarian, otherwise in Italian
	const ItalianHungarianSplitLongitude = 14.5

	if longitude > ItalianHungarianSplitLongitude {
		// assume Hungarian
		return prefix + tagLangPostfixHungarian
	}

	// assume Italian
	return prefix + tagLangPostfixItalian
}

func main() {
	flag.Parse()

	ReadLookups()
	log.Printf("Reading %s...", *inputShapeFileName)

	featureCollection := ReadShapefile(*inputShapeFileName)

	log.Printf("Sorting %d features...", len(featureCollection.Features))
	SortFeatureCollection(*featureCollection)

	//rawJSON, err := featureCollection.MarshalJSON()
	rawJSON, err := json.MarshalIndent(featureCollection, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	err = ioutil.WriteFile(*outputGeoJSONFileName, rawJSON, 0644)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Saved %d addresses to %s.", len(featureCollection.Features), *outputGeoJSONFileName)

}

// DecodeWindows1250bytes decodes win1250 []byte and returns UTF-8 string
func DecodeWindows1250bytes(enc []byte) string {
	dec := charmap.Windows1250.NewDecoder()
	out, _ := dec.Bytes(enc)
	return string(out)
	// return strings.Trim(string(out), "\u0000") // trim null characters to remove null strings (when no bilingual name)
}

// DecodeWindows1250 decodes win1250 string and returns UTF-8 string
func DecodeWindows1250(str string) string {
	return DecodeWindows1250bytes([]byte(str))
}

// EncodeWindows1250 encodes the given utf string into Windows 1250
func EncodeWindows1250(inp string) string {
	enc := charmap.Windows1250.NewEncoder()
	out, _ := enc.String(inp)
	return out
}