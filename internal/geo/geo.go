// Package geo provides the metric half of the duplicate-detection rules for
// new fixture submissions (A-204/A-205): great-circle distance and
// duplicate-proximity classification. Pure geometry — no HTTP, no DB, no
// dependencies. Bounding-box membership is config.BBox.Contains; handlers
// call that directly and nothing here re-implements it.
package geo

import "math"

// Duplicate thresholds (metres). NumberedRadiusM sizes the near-candidate
// query; same-asset_no duplicates are caught by the unique index + lookup, so
// the 25 m figure only bounds the candidate window. UnnumberedRadiusM is the
// hard "possible duplicate" distance for unnumbered fixtures.
const (
	NumberedRadiusM   = 25.0
	UnnumberedRadiusM = 8.0
)

// meanEarthRadiusM is the IUGG mean Earth radius in metres. Frozen so
// distances are reproducible across releases and test expectations.
const meanEarthRadiusM = 6371008.8

// degToRad converts decimal degrees to radians.
const degToRad = math.Pi / 180

// Point is a latitude/longitude pair in decimal degrees (WGS84 degrees; the
// spherical haversine model ignores the ellipsoid flattening, which is far
// below the 8 m decision threshold's care factor).
type Point struct{ Lat, Lon float64 }

// DistanceM is the great-circle distance between two points in metres
// (Haversine, mean Earth radius 6371008.8 m — freeze this constant).
func DistanceM(a, b Point) float64 {
	dLat := (b.Lat - a.Lat) * degToRad
	dLon := (b.Lon - a.Lon) * degToRad
	s := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(a.Lat*degToRad)*math.Cos(b.Lat*degToRad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	// Near the antipodes the two terms sum to 1 and float rounding can
	// overshoot by an ulp; clamp so Asin stays finite and the distance
	// degrades to exactly half a great circle.
	if s > 1 {
		s = 1
	}
	return 2 * meanEarthRadiusM * math.Asin(math.Sqrt(s))
}
