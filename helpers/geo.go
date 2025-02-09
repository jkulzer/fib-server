package helpers

import (
	// "fmt"
	"math"

	"github.com/golang/geo/s2"

	// "github.com/rs/zerolog/log"

	"github.com/jkulzer/fib-server/sharedModels"
	"github.com/jkulzer/osm"

	"github.com/paulmach/orb"
	orbGeo "github.com/paulmach/orb/geo"
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

func NewCircle(center orb.Point, radius float64) orb.Ring {
	scaleFactor := metersToMercator(center, radius)
	var ring orb.Ring

	wgsFirstRingPoint := pointOnCircle(center, scaleFactor, -math.Pi)

	ring = append(ring, wgsFirstRingPoint)

	for i := -math.Pi; i <= math.Pi; i = i + 0.05 {
		wgsRingPoint := pointOnCircle(center, scaleFactor, i)

		ring = append(ring, wgsRingPoint)
	}
	ring = append(ring, wgsFirstRingPoint)

	// ring.Reverse()
	return ring
}
func NewInverseCircle(center orb.Point, radius float64) orb.Polygon {

	circle := NewCircle(center, radius)

	circle.Reverse()
	polygon := orb.Polygon{sharedModels.WideOutsideBound(), circle}
	return polygon
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

func GeoRingToS2(ring orb.Ring) *s2.Loop {
	var s2Points []s2.Point
	for _, point := range ring {
		s2Points = append(s2Points, s2.PointFromLatLng(s2.LatLngFromDegrees(point[1], point[0])))
	}

	return s2.LoopFromPoints(s2Points)
}

func S2LoopToGeoRing(loop *s2.Loop) orb.Ring {
	s2Points := loop.Vertices()

	var ring orb.Ring

	for _, s2Point := range s2Points {
		ring = append(ring, GeoPointToOrbPoint(s2Point))
	}
	return ring
}

func G2p(g orb.Geometry) [][][][]float64 {

	var p [][][][]float64

	switch v := g.(type) {
	case orb.Polygon:
		p = make([][][][]float64, 1)
		p[0] = make([][][]float64, len(v))
		for i := range v { // rings
			p[0][i] = make([][]float64, len(v[i]))
			for j := range v[i] { // points
				pt := v[i][j]
				p[0][i][j] = []float64{pt.X(), pt.Y()}
			}
		}
	case orb.MultiPolygon:
		p = make([][][][]float64, len(v))
		for i := range v { // polygons
			p[i] = make([][][]float64, len(v[i]))
			for j := range v[i] { // rings
				p[i][j] = make([][]float64, len(v[i][j]))
				for k := range v[i][j] { // points
					pt := v[i][j][k]
					p[i][j][k] = []float64{pt.X(), pt.Y()}
				}
			}
		}
	}

	return p
}

func P2g(p [][][][]float64) orb.MultiPolygon {

	g := make(orb.MultiPolygon, len(p))

	for i := range p {
		g[i] = make([]orb.Ring, len(p[i]))
		for j := range p[i] {
			g[i][j] = make([]orb.Point, len(p[i][j]))
			for k := range p[i][j] {
				pt := p[i][j][k]
				point := orb.Point{pt[0], pt[1]}
				g[i][j][k] = point
			}
		}
	}
	return g
}
