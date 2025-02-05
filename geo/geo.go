package geo

import (
	"context"
	// "encoding/json"
	"fmt"
	"os"
	"slices"

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

	// map stuff
	primaryHighways := make(map[osm.WayID]*osm.Way)
	secondaryHighways := make(map[osm.WayID]*osm.Way)
	tertiaryHighways := make(map[osm.WayID]*osm.Way)
	rivers := make(map[osm.WayID]*osm.Way)

	subwayLines := make(map[osm.RelationID]*osm.Relation)
	sbahnLines := make(map[osm.RelationID]*osm.Relation)

	var berlinBoundary *osm.Relation

	for scanner.Scan() {
		// Get the next OSM object
		obj := scanner.Object()

		switch v := obj.(type) {
		case *osm.Node:
			_ = v
			nodes[v.ID] = v
			if v.Tags.Find("railway") == "station" || v.Tags.Find("railway") == "halt" {
				if v.Tags.Find("usage ") != "tourism" {
					railwayStations[v.ID] = v
				}
			}
			// g.AddNode(simple.Node(v.ID))
		case *osm.Way:
			ways[v.ID] = v
			if v.Tags.Find("waterway") == "river" {
				rivers[v.ID] = v
			}
			highwayTag := v.Tags.Find("highway")
			if highwayTag == "primary" {
				primaryHighways[v.ID] = v
			} else if highwayTag == "secondary" {
				secondaryHighways[v.ID] = v
			} else if highwayTag == "tertiary" {
				tertiaryHighways[v.ID] = v
			}
		case *osm.Relation:
			relations[v.ID] = v
			if v.Tags.Find("admin_level") == "9" && v.Tags.Find("name:prefix") == "Bezirk" {
				bezirke[v.ID] = v
			}
			if v.Tags.Find("admin_level") == "10" && v.Tags.Find("name:prefix") == "Ortsteil" {
				ortsteile[v.ID] = v
			}
			if v.Tags.Find("admin_level") == "4" && v.Tags.Find("de:amtlicher_gemeindeschluessel") == "11000000" {
				berlinBoundary = v
			}
			routeTag := v.Tags.Find("route")
			if routeTag == "subway" {
				subwayLines[v.ID] = v
			} else if routeTag == "light_rail" {
				sbahnLines[v.ID] = v
			}
		default:
			// Handle other OSM object types if needed
		}
	}

	fc := geojson.NewFeatureCollection()

	bezirkPolygons := make(map[string]orb.Polygon)
	for _, bezirkRelation := range bezirke {
		bezirkName := bezirkRelation.Tags.Find("name")
		var bezirkLineStrings []orb.LineString
		for _, member := range bezirkRelation.Members {
			if member.Type == "way" {
				wayID, err := member.ElementID().WayID()
				if err != nil {
					log.Err(err).Msg("")
					continue
				}
				way := ways[wayID]
				lineString := lineStringFromWay(way, nodes)
				bezirkLineStrings = append(bezirkLineStrings, lineString)
			}
		}
		for _, ls := range bezirkLineStrings {
			fc.Append(geojson.NewFeature(ls))
		}
		writeAndMarshallFC(fc)
		bezirkPolygon := orb.Polygon(ringFromLineStrings(bezirkLineStrings))
		bezirkPolygons[bezirkName] = bezirkPolygon
	}

	for _, polygon := range bezirkPolygons {
		fc.Append(geojson.NewFeature(polygon))
	}

	var berlinBoundaryLineStrings []orb.LineString

	for _, member := range berlinBoundary.Members {
		if member.Type == "way" {
			wayID, err := member.ElementID().WayID()
			if err != nil {
				log.Err(err).Msg("")
				continue
			}
			way := ways[wayID]
			lineString := lineStringFromWay(way, nodes)
			berlinBoundaryLineStrings = append(berlinBoundaryLineStrings, lineString)
		}
	}
	berlinBoundaryPolygon := orb.Polygon(ringFromLineStrings(berlinBoundaryLineStrings))
	// simplify.DouglasPeucker(0.001).Polygon(berlinBoundaryPolygon)
	berlinBoundaryFeature := geojson.NewFeature(berlinBoundaryPolygon)
	berlinBoundaryFeature.Properties["category"] = "game_area_border"
	fc.Append(berlinBoundaryFeature)

	marshalledFC, _ := fc.MarshalJSON()

	// writeAndMarshallFC(fc)

	log.Info().Msg("finished processing of OSM data")

	return ProcessedData{
		CityBoundary:                   berlinBoundary,
		Nodes:                          nodes,
		Ways:                           ways,
		Relations:                      relations,
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

func lineStringFromWay(way *osm.Way, nodes map[osm.NodeID]*osm.Node) orb.LineString {
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

func ringFromLineStrings(lineStrings []orb.LineString) []orb.Ring {
	var theRing orb.Ring
	startLineString := lineStrings[0]

	for _, startLSPoint := range startLineString {
		theRing = append(theRing, startLSPoint)
	}
	lineStrings = slices.Delete(lineStrings, 0, 1)

	lineStringsLength := len(lineStrings)

	for lineStringsLength > 3 {
		lastPointAsRef := theRing[len(theRing)-1]
	forLoop:
		for lsIndex, lineString := range lineStrings {
			firstLSElement := lineString[0]
			lastLSElement := lineString[len(lineString)-1]
			switch lastPointAsRef {
			case firstLSElement:
				appendLSToRing(lineString, &theRing)
				lineStrings = slices.Delete(lineStrings, lsIndex, lsIndex+1)
				lineStringsLength--
				break forLoop
			case lastLSElement:
				lineString.Reverse()
				appendLSToRing(lineString, &theRing)
				lineStrings = slices.Delete(lineStrings, lsIndex, lsIndex+1)
				lineStringsLength--
				break forLoop
			}
		}
	}
	theRing = append(theRing, theRing[0])
	theRing.Reverse()
	var returnRing []orb.Ring
	return append(returnRing, theRing)
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
