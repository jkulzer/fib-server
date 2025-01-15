package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"

	"github.com/jkulzer/fib-server/helpers"

	"github.com/golang/geo/s2"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"

	"github.com/jkulzer/osm"
	"github.com/jkulzer/osm/osmpbf"
)

func ProcessData() {

	osmFile, err := os.Open("./berlin-latest.osm.pbf")
	if err != nil {
		log.Err(err)
	}
	scanner := osmpbf.New(context.Background(), osmFile, 4)

	log.Info().Msg("starting processing of OSM data. this is blocking")

	bezirke := make(map[osm.RelationID]*osm.Relation)

	nodes := make(map[osm.NodeID]*osm.Node)
	ways := make(map[osm.WayID]*osm.Way)

	// Scan and populate the relations map
	for scanner.Scan() {
		// Get the next OSM object
		obj := scanner.Object()

		switch v := obj.(type) {
		case *osm.Node:
			_ = v
			nodes[v.ID] = v
			// g.AddNode(simple.Node(v.ID))
		case *osm.Way:
			ways[v.ID] = v
		case *osm.Relation:
			if v.Tags.Find("admin_level") == "9" && v.Tags.Find("name:prefix") == "Bezirk" {
				bezirke[v.ID] = v
			}
		default:
			// Handle other OSM object types if needed
		}
	}
	log.Info().Msg("finished processing of OSM data")

	testPoint := orb.Point{52.507546, 13.525537}

	for _, relation := range bezirke {

		// var geoPoints []s2.Point
		var orbPoints []orb.Point

		fmt.Println(relation.Tags.Find("name"))
		for _, member := range relation.Members {
			if member.Type == osm.TypeWay {
				wayID, err := member.ElementID().WayID()
				if err != nil {
					log.Err(err)
					continue
				} else {
					for _, wayNode := range ways[wayID].Nodes {

						node := nodes[wayNode.ID]
						// geoPoint := helpers.OsmNodeToGeoPoint(*node)
						orbPoint := helpers.NodeToPoint(*node)

						prevNodeIdentical := false
						if len(orbPoints) > 0 {
							if orbPoints[len(orbPoints)-1] == orbPoint {

							}
						}
						if prevNodeIdentical == false {

							orbPoints = append(orbPoints, orbPoint)
						} else {
							fmt.Println("identical nodes")
						}
					}
				}
			}
		}

		feature := geojson.NewFeature(orb.LineString(orbPoints))

		feature.Properties["name"] = "Sample LineString"

		// Marshal the feature to GeoJSON
		geoJSON, err := json.MarshalIndent(feature, "", "  ")
		if err != nil {
			fmt.Printf("Error marshalling GeoJSON: %v\n", err)
			return
		}

		_ = os.WriteFile("./bezirk.geojson", []byte(string(geoJSON)), 0644)

		bezirkLoop := s2.LoopFromPoints(geoPoints)

		var loopList []*s2.Loop
		loopList = append(loopList, bezirkLoop)
		bezirkPolygon := s2.PolygonFromLoops(loopList)

		// fmt.Println(bezirkPolygon.ContainsPoint(helpers.OrbPointToGeoPoint(testPoint)))
		fmt.Println(bezirkPolygon.ContainsPoint(helpers.OrbPointToGeoPoint(testPoint)))
	}
}
