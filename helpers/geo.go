package helpers

import (
	"math"

	"github.com/golang/geo/s2"

	"github.com/jkulzer/osm"

	"github.com/paulmach/orb"
	orbGeo "github.com/paulmach/orb/geo"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/project"
)

func NodeToPoint(node osm.Node) orb.Point {
	var point orb.Point
	point[0] = node.Lon
	point[1] = node.Lat
	return point
}

func WayNodeToPoint(wayNode osm.WayNode) orb.Point {
	var point orb.Point
	point[0] = wayNode.Lon
	point[1] = wayNode.Lat
	return point
}

func OrbPointToGeoPoint(point orb.Point) s2.Point {
	return s2.PointFromLatLng(s2.LatLngFromDegrees(point.Lat(), point.Lon()))
}

func OsmNodeToGeoPoint(node osm.Node) s2.Point {
	return s2.PointFromLatLng(s2.LatLngFromDegrees(node.Lat, node.Lon))
}

func GeoPointToOrbPoint(point s2.Point) orb.Point {
	// Convert s2.Point to s2.LatLng
	latLng := s2.LatLngFromPoint(point)

	// Convert latitude and longitude from radians to degrees
	lat := radiansToDegrees(latLng.Lat.Radians())
	lng := radiansToDegrees(latLng.Lng.Radians())

	// Return orb.Point in (longitude, latitude) format
	return orb.Point{lng, lat}
}

func radiansToDegrees(rad float64) float64 {
	return rad * 180.0 / math.Pi
}

func NewCircle(center orb.Point, radius float64) geojson.Feature {
	scaleFactor := metersToMercator(center, radius)
	var ring orb.Ring

	wgsFirstRingPoint := pointOnCircle(center, scaleFactor, -math.Pi)

	ring = append(ring, wgsFirstRingPoint)

	for i := -math.Pi; i <= math.Pi; i = i + 0.05 {
		wgsRingPoint := pointOnCircle(center, scaleFactor, i)

		ring = append(ring, wgsRingPoint)
	}
	ring = append(ring, wgsFirstRingPoint)

	ring.Reverse()
	return *geojson.NewFeature(ring)
}
func NewInverseCircle(center orb.Point, radius float64) geojson.Feature {
	scaleFactor := metersToMercator(center, radius)
	var ring orb.Ring

	wgsFirstRingPoint := pointOnCircle(center, scaleFactor, -math.Pi)

	leftBound := 12.5
	rightBound := float64(14)
	topBound := 52.7
	bottomBound := float64(52)

	var leftTopPoint orb.Point
	leftTopPoint[0] = leftBound
	leftTopPoint[1] = topBound

	var rightTopPoint orb.Point
	rightTopPoint[0] = rightBound
	rightTopPoint[1] = topBound

	var rightBottomPoint orb.Point
	rightBottomPoint[0] = rightBound
	rightBottomPoint[1] = bottomBound

	var leftBottomPoint orb.Point
	leftBottomPoint[0] = leftBound
	leftBottomPoint[1] = bottomBound

	ring = append(ring, leftTopPoint)
	ring = append(ring, rightTopPoint)
	ring = append(ring, rightBottomPoint)
	ring = append(ring, leftBottomPoint)
	ring = append(ring, leftTopPoint)

	ring = append(ring, wgsFirstRingPoint)

	for i := -math.Pi; i <= math.Pi; i = i + 0.05 {
		wgsRingPoint := pointOnCircle(center, scaleFactor, i)

		ring = append(ring, wgsRingPoint)
	}
	ring = append(ring, wgsFirstRingPoint)
	ring = append(ring, leftTopPoint)

	ring.Reverse()
	return *geojson.NewFeature(ring)
}

func metersToMercator(center orb.Point, meters float64) float64 {

	pointOnCircle := orbGeo.PointAtBearingAndDistance(center, -90, meters)

	centerProj := project.Point(center, project.WGS84.ToMercator)
	pointOnCircleProj := project.Point(pointOnCircle, project.WGS84.ToMercator)

	distanceProj := centerProj[0] - pointOnCircleProj[0]

	return distanceProj

}

func pointOnCircle(center orb.Point, scaleFactor float64, positionOnCirc float64) orb.Point {
	projCenter := project.Point(center, project.WGS84.ToMercator)
	firstPoint := projCenter
	firstPoint[1] = firstPoint[1] + math.Sin(positionOnCirc)*scaleFactor
	firstPoint[0] = firstPoint[0] + math.Cos(positionOnCirc)*scaleFactor
	wgsFirstRingPoint := project.Point(firstPoint, project.Mercator.ToWGS84)
	return wgsFirstRingPoint
}
