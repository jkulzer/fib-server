package helpers

import (
	"math"

	"github.com/golang/geo/s2"
	"github.com/jkulzer/osm"
	"github.com/paulmach/orb"
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
