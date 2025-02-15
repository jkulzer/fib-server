package geo

import (
	"context"
	// "encoding/json"
	// "errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/jkulzer/fib-server/helpers"
	"github.com/jkulzer/fib-server/sharedModels"

	// "github.com/golang/geo/s2"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geo"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/simplify"

	"github.com/jkulzer/osm"
	"github.com/jkulzer/osm/osmpbf"
)

type ProcessedData struct {
	CityBoundary                   *osm.Relation
	Nodes                          map[osm.NodeID]*osm.Node
	Ways                           map[osm.WayID]*osm.Way
	Relations                      map[osm.RelationID]*osm.Relation
	AllRailRoutes                  map[osm.RelationID]*osm.Relation
	McDonaldsNodes                 map[osm.NodeID]*osm.Node
	McDonaldsWays                  map[osm.WayID]*osm.Way
	IkeaWays                       map[osm.WayID]*osm.Way
	SpreeLineStrings               []orb.LineString
	RailwayStations                map[osm.NodeID]*osm.Node
	Bezirke                        map[osm.RelationID]*osm.Relation
	Ortsteile                      map[osm.RelationID]*osm.Relation
	MapMarshalledFeatureCollection []byte
}

func ProcessData() ProcessedData {

	osmFile, err := os.Open("./berlin-latest.osm.pbf")
	if err != nil {
		log.Err(err)
	}
	scanner := osmpbf.New(context.Background(), osmFile, 4)

	log.Info().Msg("starting processing of OSM data. this is blocking")

	bezirke := make(map[osm.RelationID]*osm.Relation)
	ortsteile := make(map[osm.RelationID]*osm.Relation)

	nodes := make(map[osm.NodeID]*osm.Node)
	ways := make(map[osm.WayID]*osm.Way)
	relations := make(map[osm.RelationID]*osm.Relation)

	// hiding point validity
	railwayStations := make(map[osm.NodeID]*osm.Node)

	mcDonaldsNodes := make(map[osm.NodeID]*osm.Node)
	mcDonaldsWays := make(map[osm.WayID]*osm.Way)

	ikeaWays := make(map[osm.WayID]*osm.Way)

	subwayLines := make(map[osm.RelationID]*osm.Relation)
	sbahnLines := make(map[osm.RelationID]*osm.Relation)
	allRailRoutes := make(map[osm.RelationID]*osm.Relation)

	var spreeRelation *osm.Relation
	var spreeLineStrings []orb.LineString

	var berlinBoundary *osm.Relation

	for scanner.Scan() {
		// Get the next OSM object
		obj := scanner.Object()

		switch v := obj.(type) {
		case *osm.Node:
			nodes[v.ID] = v
			if v.Tags.Find("railway") == "station" || v.Tags.Find("railway") == "halt" {
				if v.Tags.Find("usage ") != "tourism" {
					railwayStations[v.ID] = v
				}
			}
			if v.Tags.Find("brand") == "McDonald's" {
				mcDonaldsNodes[v.ID] = v
			}
		case *osm.Way:
			ways[v.ID] = v
			brandTag := v.Tags.Find("brand")
			if brandTag == "McDonald's" {
				mcDonaldsWays[v.ID] = v
			}
			if brandTag == "IKEA" && !strings.Contains(v.Tags.Find("name"), "Planning studio") {
				ikeaWays[v.ID] = v
			}
		case *osm.Relation:
			relations[v.ID] = v
			if v.Tags.Find("admin_level") == "9" && v.Tags.Find("name:prefix") == "Bezirk" {
				bezirke[v.ID] = v
			}
			if v.Tags.Find("admin_level") == "10" {
				ortsteile[v.ID] = v
			}
			if v.Tags.Find("admin_level") == "4" && v.Tags.Find("de:amtlicher_gemeindeschluessel") == "11000000" {
				berlinBoundary = v
			}
			routeTag := v.Tags.Find("route")
			if routeTag == "subway" {
				subwayLines[v.ID] = v
				allRailRoutes[v.ID] = v
			} else if routeTag == "light_rail" {
				sbahnLines[v.ID] = v
				allRailRoutes[v.ID] = v
			} else if v.Tags.Find("service") == "regional" {
				allRailRoutes[v.ID] = v
			}
			if v.Tags.Find("name") == "Spree" {
				spreeRelation = v
			}
		default:
			// Handle other OSM object types if needed
		}
	}

	fc := geojson.NewFeatureCollection()

	var berlinBoundaryLineStrings []orb.LineString

	for _, member := range spreeRelation.Members {
		if member.Type == "way" {
			wayID, err := member.ElementID().WayID()
			if err != nil {
				log.Err(err).Msg("")
				continue
			}
			way := ways[wayID]
			if way != nil {
				lineString := LineStringFromWay(way, nodes)
				spreeLineStrings = append(spreeLineStrings, lineString)
			}
		}
	}

	for _, member := range berlinBoundary.Members {
		if member.Type == "way" {
			wayID, err := member.ElementID().WayID()
			if err != nil {
				log.Err(err).Msg("")
				continue
			}
			way := ways[wayID]
			lineString := LineStringFromWay(way, nodes)
			berlinBoundaryLineStrings = append(berlinBoundaryLineStrings, lineString)
		}
	}
	berlinBoundaryRing, err := RingFromLineStrings(berlinBoundaryLineStrings)
	berlinBoundaryRing.Reverse()
	if err != nil {
		berlinBoundaryPolygon := orb.Polygon([]orb.Ring{berlinBoundaryRing})
		// simplify.DouglasPeucker(0.001).Polygon(berlinBoundaryPolygon)
		berlinBoundaryFeature := geojson.NewFeature(berlinBoundaryPolygon)
		berlinBoundaryFeature.Properties["category"] = "game_area_border"
		fc.Append(berlinBoundaryFeature)
	}
	marshalledFC, _ := fc.MarshalJSON()
	// writeAndMarshallFC(fc)

	log.Info().Msg("finished processing of OSM data")

	return ProcessedData{
		CityBoundary:                   berlinBoundary,
		Bezirke:                        bezirke,
		Ortsteile:                      ortsteile,
		Nodes:                          nodes,
		Ways:                           ways,
		Relations:                      relations,
		McDonaldsNodes:                 mcDonaldsNodes,
		McDonaldsWays:                  mcDonaldsWays,
		IkeaWays:                       ikeaWays,
		SpreeLineStrings:               spreeLineStrings,
		AllRailRoutes:                  allRailRoutes,
		RailwayStations:                railwayStations,
		MapMarshalledFeatureCollection: marshalledFC,
	}
}

func PointIsValidZoneCenter(hiderPoint orb.Point, data ProcessedData) bool {
	for _, railwayStation := range data.RailwayStations {
		railwayStationPoint := helpers.NodeToPoint(*railwayStation)
		distanceFromRailStation := geo.DistanceHaversine(hiderPoint, railwayStationPoint)
		if distanceFromRailStation <= sharedModels.HidingZoneRadius {
			return true
		}
	}
	return false
}

func LineStringFromWay(way *osm.Way, nodes map[osm.NodeID]*osm.Node) orb.LineString {
	var lineString orb.LineString
	if way != nil {
		for _, wayNode := range way.Nodes {
			point := nodes[wayNode.ID].Point()
			lineString = append(lineString, point)
		}
	}
	return lineString
}

func appendLineString(lineString orb.LineString, collection *orb.Collection) {
	threshold := 0.0000001
	simplify.DouglasPeucker(threshold).Simplify(lineString)
	*collection = append(*collection, lineString)
}

func addToFeatureCollection(category string, fc *geojson.FeatureCollection, collection orb.Collection) {
	for _, geometry := range collection {
		feature := geojson.NewFeature(geometry)
		feature.Properties["category"] = category
		fc.Append(feature)
	}
}

func RingFromLineStrings(lineStrings []orb.LineString) (orb.Ring, error) {
	var theRing orb.Ring
	for _, ls := range lineStrings {
		if len(theRing) > 0 {
			if theRing[len(theRing)-1] != ls[0] {
				if theRing[len(theRing)-1] == ls[len(ls)-1] {
					ls.Reverse()
				} else {
					continue
				}
			}
		}
		for _, point := range ls {
			theRing = append(theRing, point)
		}
	}

	theRing = append(theRing, theRing[0])
	return theRing, nil
}

func appendLSToRing(lineString orb.LineString, ring *orb.Ring) {
	for _, point := range lineString {
		*ring = append(*ring, point)
	}
}

func writeAndMarshallFC(fc *geojson.FeatureCollection) {
	marshalledFC, _ := fc.MarshalJSON()

	fmt.Println("length of json: " + fmt.Sprint(len(marshalledFC)))
	f, err := os.Create("mapdata.geojson")

	defer f.Close()
	_, err = f.Write(marshalledFC)
	if err != nil {
		log.Err(err).Msg("")
	}
}

func RelationToMultiPolygon(relationToConvert osm.Relation, nodes map[osm.NodeID]*osm.Node, ways map[osm.WayID]*osm.Way) (orb.MultiPolygon, error) {
	var lineStrings []orb.LineString
	for _, member := range relationToConvert.Members {
		if member.Type == "way" {
			wayID, err := member.ElementID().WayID()
			if err != nil {
				return orb.MultiPolygon{}, err
			}
			way := ways[wayID]
			if way == nil {
				continue
			}
			lineStrings = append(lineStrings, LineStringFromWay(way, nodes))
		}
	}

	multiPolygon := lineStringsToMultiPolygon(lineStrings)

	return multiPolygon, nil
}

func lineStringsToMultiPolygon(lineStrings []orb.LineString) orb.MultiPolygon {
	var multiPolygon orb.MultiPolygon
	var polygon orb.Polygon

	firstLineString := lineStrings[0]

	var initialRing orb.Ring
	initialRing = append(initialRing, firstLineString[0])

	polygon = append(polygon, initialRing)

	lineStrings = slices.Delete(lineStrings, 0, 0)

	for len(lineStrings) > 0 {
		foundMatch := false
	forLoop:
		for lsIndex, ls := range lineStrings {
			lastPoint := polygon[0][len(polygon[0])-1]
			switch lastPoint {
			case ls[0]:
				appendLSToRing(ls, &polygon[0])
				lineStrings = slices.Delete(lineStrings, lsIndex, lsIndex+1)
				foundMatch = true
				break forLoop
			case ls[len(ls)-1]:
				ls.Reverse()
				appendLSToRing(ls, &polygon[0])
				lineStrings = slices.Delete(lineStrings, lsIndex, lsIndex+1)
				foundMatch = true
				break forLoop
			}
		}
		if !foundMatch {
			break
		}
	}
	var additionalPolygons orb.MultiPolygon
	if len(lineStrings) > 0 {
		additionalPolygons = lineStringsToMultiPolygon(lineStrings)
	}
	multiPolygon = append(multiPolygon, additionalPolygons...)
	multiPolygon = append(multiPolygon, polygon)
	return multiPolygon
}
