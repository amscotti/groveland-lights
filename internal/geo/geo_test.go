package geo

import (
	"math"
	"math/rand"
	"testing"
)

// approxM reports whether got is within tol metres of the hand-computed
// reference want.
func approxM(got, want, tol float64) bool {
	return math.Abs(got-want) <= tol
}

// TestDistanceMTable checks DistanceM against hand-computed analytic values.
// Reference derivations (mean Earth radius R = 6371008.8 m):
//
//	pure latitude: d = R * dLat_rad exactly (haversine reduces to R*dLat
//	  when longitudes match), so one degree = R*pi/180 = 111195.080 m;
//	pure longitude: d = R * dLon_rad * cos(lat); cos(42.76 deg) = 0.7342040;
//	diagonal: Pythagoras on the exact haversine terms;
//	antipodes: d = pi*R = 20015114.442 m (half a great circle).
func TestDistanceMTable(t *testing.T) {
	groveland := Point{Lat: 42.76, Lon: -71.03}
	tests := []struct {
		name string
		a, b Point
		want float64
	}{
		{
			name: "one degree of latitude at the equator",
			a:    Point{Lat: 0, Lon: 0},
			b:    Point{Lat: 1, Lon: 0},
			want: 111195.080,
		},
		{
			name: "Groveland 0.001 deg north",
			a:    groveland,
			b:    Point{Lat: 42.7610, Lon: -71.03},
			want: 111.195,
		},
		{
			name: "Groveland 0.01 deg west",
			a:    groveland,
			b:    Point{Lat: 42.76, Lon: -71.04},
			want: 816.399,
		},
		{
			name: "Groveland diagonal 0.005 deg south and east",
			a:    groveland,
			b:    Point{Lat: 42.7550, Lon: -71.0250},
			want: 689.745,
		},
		{
			name: "antipodes on the equator",
			a:    Point{Lat: 0, Lon: 0},
			b:    Point{Lat: 0, Lon: 180},
			want: 20015114.442,
		},
		{
			name: "antipodes through Groveland",
			a:    groveland,
			b:    Point{Lat: -42.76, Lon: 108.97},
			want: 20015114.442,
		},
		{
			name: "identical points are zero metres apart",
			a:    groveland,
			b:    groveland,
			want: 0,
		},
	}
	const tolM = 0.5
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DistanceM(tt.a, tt.b)
			if !approxM(got, tt.want, tolM) {
				t.Fatalf("DistanceM(%v, %v) = %.4f m, want %.3f m (+/- %g m)", tt.a, tt.b, got, tt.want, tolM)
			}
		})
	}
}

// TestDistanceMProperties checks the metric laws over seeded random triples
// plus a deterministic set of nasty points (poles, antipodes, dateline).
// The properties hold exactly in mathematics; the slacks only absorb IEEE
// rounding of the haversine evaluation:
//
//   - fpSlackM: ordinary sums round at ~1e-8 m;
//   - antipodalSlackM: when a leg approaches the antipodes, the sin^2 terms
//     sum to ~1 and plain haversine loses sqrt(machine-eps)*R precision —
//     metre-scale (observed: 0.05 m on pole/antipode triples). The
//     inequality still holds far inside this envelope.
func TestDistanceMProperties(t *testing.T) {
	rng := rand.New(rand.NewSource(204)) // fixed seed: failures reproduce
	randPoint := func() Point {
		return Point{Lat: rng.Float64()*180 - 90, Lon: rng.Float64()*360 - 180}
	}
	const (
		loops           = 2000
		fpSlackM        = 1e-6
		antipodalZoneM  = 1.9e7 // legs beyond this run inside the antipodal zone
		antipodalSlackM = 2.0
	)
	check := func(a, b, c Point) {
		t.Helper()
		ab, bc, ac := DistanceM(a, b), DistanceM(b, c), DistanceM(a, c)
		if ab != DistanceM(b, a) {
			t.Fatalf("DistanceM not symmetric: %v -> %v = %v, reversed = %v", a, b, ab, DistanceM(b, a))
		}
		if ab < 0 || bc < 0 || ac < 0 {
			t.Fatalf("negative distance: ab=%v bc=%v ac=%v", ab, bc, ac)
		}
		slack := fpSlackM
		if ab > antipodalZoneM || bc > antipodalZoneM || ac > antipodalZoneM {
			slack = antipodalSlackM
		}
		if ac > ab+bc+slack {
			t.Fatalf("triangle inequality violated: a=%v b=%v c=%v: ac=%v > ab+bc=%v (slack %v)", a, b, c, ac, ab+bc, slack)
		}
	}
	for i := 0; i < loops; i++ {
		check(randPoint(), randPoint(), randPoint())
	}
	nasty := []Point{
		{Lat: 0, Lon: 0},
		{Lat: 90, Lon: 0},
		{Lat: -90, Lon: 0},
		{Lat: 0, Lon: 180},
		{Lat: 0, Lon: -180},
		{Lat: 42.76, Lon: -71.03},
		{Lat: -42.76, Lon: 108.97},
		{Lat: 89.99999, Lon: 179.99999},
		{Lat: -89.99999, Lon: -179.99999},
	}
	for _, a := range nasty {
		for _, b := range nasty {
			for _, c := range nasty {
				check(a, b, c)
			}
		}
	}
}
