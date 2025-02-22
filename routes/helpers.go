package routes

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"github.com/jkulzer/fib-server/geo"
	"github.com/jkulzer/fib-server/helpers"
	"github.com/jkulzer/fib-server/models"
	"github.com/jkulzer/fib-server/sharedModels"

	"github.com/jkulzer/osm"
	"github.com/paulmach/orb"
	orbGeo "github.com/paulmach/orb/geo"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/planar"

	osmLookup "github.com/turistikrota/osm"

	"github.com/engelsjk/polygol"

	"github.com/rs/zerolog/log"
)

func closerOrFurtherFromObject(lobby models.Lobby, fc *geojson.FeatureCollection, processedData geo.ProcessedData, w http.ResponseWriter, objectNodes map[osm.NodeID]*osm.Node, objectWays map[osm.WayID]*osm.Way) (returnLobby models.Lobby, closer bool, distance float64, err error) {
	var seekerPoint orb.Point
	seekerPoint[1] = lobby.SeekerLat
	seekerPoint[0] = lobby.SeekerLon

	var hiderPoint orb.Point
	hiderPoint[1] = lobby.HiderLat
	hiderPoint[0] = lobby.HiderLon

	// the 1 in the math.Inf argument is that it's positive infinity
	seekerDistance := float64(math.Inf(1))
	hiderDistance := float64(math.Inf(1))

	var objectPoints []orb.Point

	for _, node := range objectNodes {
		point := node.Point()
		objectPoints = append(objectPoints, point)
	}
	for _, area := range objectWays {
		wayNodes := area.Nodes
		var lineString orb.LineString
		for _, wayNode := range wayNodes {
			node := processedData.Nodes[wayNode.ID]
			if node == nil {
				log.Warn().Msg("couldn't find node with ID " + fmt.Sprint(wayNode.ID) + " in nodes map")
				continue
			}
			lineString = append(lineString, helpers.NodeToPoint(*node))
		}
		objectPoints = append(objectPoints, lineString.Bound().Center())
	}

	for _, point := range objectPoints {
		seekerCurrentDistance := orbGeo.DistanceHaversine(seekerPoint, point)
		if seekerCurrentDistance < seekerDistance && seekerCurrentDistance != 0 {
			seekerDistance = seekerCurrentDistance
		}
		hiderCurrentDistance := orbGeo.DistanceHaversine(hiderPoint, point)
		if hiderCurrentDistance < hiderDistance && hiderCurrentDistance != 0 {
			hiderDistance = hiderCurrentDistance
		}
	}

	log.Debug().Msg(fmt.Sprint("seeker distance is ", seekerDistance, " and hider distance is ", hiderDistance))

	var circleList []orb.Ring
	var circleGeomList []polygol.Geom

	// var circleGeomList
	for index, point := range objectPoints {
		if point == sharedModels.ZeroPoint {
			log.Warn().Msg("point at index " + fmt.Sprint(index) + " of " + fmt.Sprint(len(objectPoints)) + " is zero")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(nil)
			return
		}
		circle := helpers.NewCircle(point, seekerDistance)
		circleList = append(circleList, circle)
		geomCircle := helpers.G2p(orb.Polygon{circle})
		circleGeomList = append(circleGeomList, geomCircle)
	}

	if len(circleGeomList) == 0 {
		log.Warn().Msg("length of circle list is 0")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(nil)
		return
	}

	var isCloser bool
	if seekerDistance < hiderDistance {
		isCloser = true
	} else {
		isCloser = false
	}

	// closer to the object than the hider
	if isCloser {
		for _, circle := range circleList {
			fc.Append(geojson.NewFeature(circle))
		}
		// farther away from the object than the hider
	} else {
		outsideGeom := helpers.G2p(orb.Polygon{sharedModels.WideOutsideBound()})
		diff, err := polygol.Difference(outsideGeom, circleGeomList...)
		if err != nil {
			log.Err(err).Msg("failed to get polygol difference")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(nil)
			return lobby, false, 0.0, err

		}
		diffMultiPolygon := helpers.P2g(diff)
		fc.Append(geojson.NewFeature(diffMultiPolygon))
	}

	lobby, err = helpers.SaveFC(lobby, fc)
	if err != nil {
		log.Err(err).Msg("failed to save FC to DB")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(nil)
		return lobby, false, 0.0, err
	}

	return lobby, isCloser, seekerDistance, nil
}

func closerOrFurtherFromOrbLine(lobby models.Lobby, fc *geojson.FeatureCollection, w http.ResponseWriter, lineStrings []orb.LineString) (returnLobby models.Lobby, closer bool, distance float64, err error) {
	var seekerPoint orb.Point
	seekerPoint[1] = lobby.SeekerLat
	seekerPoint[0] = lobby.SeekerLon

	var hiderPoint orb.Point
	hiderPoint[1] = lobby.HiderLat
	hiderPoint[0] = lobby.HiderLon

	seekerDistance := math.Inf(1)
	hiderDistance := math.Inf(1)

	for _, lineString := range lineStrings {
		for _, point := range lineString {
			currentSeekerDistance := orbGeo.DistanceHaversine(point, seekerPoint)
			if currentSeekerDistance < seekerDistance && currentSeekerDistance != 0 {
				seekerDistance = currentSeekerDistance
			}
			currentHiderDistance := orbGeo.DistanceHaversine(point, hiderPoint)
			if currentHiderDistance < hiderDistance && currentHiderDistance != 0 {
				hiderDistance = currentHiderDistance
			}
		}
	}

	log.Debug().Msg(fmt.Sprint("hider distance ", hiderDistance, " and seeker distance ", seekerDistance))

	var multiPoly orb.MultiPolygon

	for _, lineString := range lineStrings {
		lineStringLength := len(lineString)
		for index, point := range lineString {
			if index < lineStringLength-1 {
				nextPoint := lineString[index+1]

				lineBearing := orbGeo.Bearing(point, nextPoint)
				normalBearing := helpers.NormalizeBearing(lineBearing + 90)
				antinormalBearing := helpers.NormalizeBearing(lineBearing - 90)

				offsetPoint := orbGeo.PointAtBearingAndDistance(point, normalBearing, seekerDistance)
				offsetNextPoint := orbGeo.PointAtBearingAndDistance(nextPoint, normalBearing, seekerDistance)
				antiOffsetPoint := orbGeo.PointAtBearingAndDistance(point, antinormalBearing, seekerDistance)
				antiOffsetNextPoint := orbGeo.PointAtBearingAndDistance(nextPoint, antinormalBearing, seekerDistance)
				ring := orb.Ring{offsetPoint, offsetNextPoint, antiOffsetNextPoint, antiOffsetPoint, offsetPoint}

				multiPoly = append(multiPoly, orb.Polygon{ring})
			}
			multiPoly = append(multiPoly, orb.Polygon{helpers.NewCircle(point, seekerDistance)})
		}
	}

	geom := polygol.Geom(helpers.G2p(multiPoly))
	union, err := polygol.Union(geom)
	if err != nil {
		log.Err(err).Msg("failed performing union operation")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(nil)
		return lobby, false, 0.0, err
	}

	multiPoly = helpers.P2g(union)

	isCloser := planar.MultiPolygonContains(multiPoly, hiderPoint)

	if isCloser {
		outsideGeom := helpers.G2p(orb.Polygon{sharedModels.WideOutsideBound()})
		diff, err := polygol.Difference(outsideGeom, helpers.G2p(multiPoly))
		if err != nil {
			log.Err(err).Msg("failed to get polygol difference")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(nil)
			return lobby, false, 0.0, err

		}
		diffMultiPolygon := helpers.P2g(diff)
		fc.Append(geojson.NewFeature(diffMultiPolygon))
	} else {
		fc.Append(geojson.NewFeature(multiPoly))
	}

	lobby, err = helpers.SaveFC(lobby, fc)
	if err != nil {
		log.Err(err).Msg("failed to save FC to DB")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(nil)
		return lobby, false, 0.0, err
	}

	return lobby, isCloser, seekerDistance, nil
}

func getClosestAdressString(point orb.Point) (string, error) {
	ctx := context.Background()
	reverseResult, err := osmLookup.Reverse(ctx, point.Lat(), point.Lon())
	if err != nil {
		return "", err
	}

	addr := reverseResult.Address

	addressString := addr.Road + " " + addr.HouseNumber + ", " + addr.Suburb + " " + addr.County

	return addressString, nil
}
