package geo

import "testing"

// metresPerDegLat = R*pi/180 for the frozen mean Earth radius: the exact
// metres-per-degree factor for pure-latitude arcs at ANY latitude (the
// haversine reduces to R*dLat_rad when the longitudes match).
const metresPerDegLat = 111195.0802

// northOf returns p moved m metres north along its meridian. Construction
// error is micrometres, far inside the test tolerances.
func northOf(p Point, m float64) Point {
	return Point{Lat: p.Lat + m/metresPerDegLat, Lon: p.Lon}
}

// TestThresholdsAreFrozen pins the A-205 radii: bumping either constant is a
// contract change and must fail this test loudly.
func TestThresholdsAreFrozen(t *testing.T) {
	if NumberedRadiusM != 25.0 {
		t.Fatalf("NumberedRadiusM = %v, want 25.0 (sizes the near-candidate window)", NumberedRadiusM)
	}
	if UnnumberedRadiusM != 8.0 {
		t.Fatalf("UnnumberedRadiusM = %v, want 8.0 (hard unnumbered duplicate radius)", UnnumberedRadiusM)
	}
}

func TestClassifyDuplicate(t *testing.T) {
	base := Point{Lat: 42.76, Lon: -71.03}
	tests := []struct {
		name       string
		newAssetNo string
		newPt      Point
		existing   []Candidate
		want       DupDecision
		wantDup    bool
		tolM       float64 // tolerance for want.DistanceM; unused when wantDup is false
	}{
		{
			name:       "same asset_no anywhere is numbered duplicate",
			newAssetNo: "GP-017",
			newPt:      base,
			existing:   []Candidate{{LightID: 1, AssetNo: "GP-017", Point: northOf(base, 2000)}},
			want:       DupDecision{Numbered: true, ExistingID: 1, DistanceM: 2000},
			wantDup:    true,
			tolM:       0.5,
		},
		{
			name:       "same asset_no nearby is still a numbered duplicate",
			newAssetNo: "GP-017",
			newPt:      base,
			existing:   []Candidate{{LightID: 2, AssetNo: "GP-017", Point: northOf(base, 7.9)}},
			want:       DupDecision{Numbered: true, ExistingID: 2, DistanceM: 7.9},
			wantDup:    true,
			tolM:       0.05,
		},
		{
			name:       "first numbered match wins on slice order",
			newAssetNo: "GP-017",
			newPt:      base,
			existing: []Candidate{
				{LightID: 11, AssetNo: "GP-017", Point: northOf(base, 3000)},
				{LightID: 12, AssetNo: "GP-017", Point: northOf(base, 20)},
			},
			want:    DupDecision{Numbered: true, ExistingID: 11, DistanceM: 3000},
			wantDup: true,
			tolM:    0.5,
		},
		{
			name:       "numbered match wins over unnumbered proximity",
			newAssetNo: "GP-017",
			newPt:      base,
			existing: []Candidate{
				{LightID: 13, AssetNo: "", Point: northOf(base, 3)},
				{LightID: 14, AssetNo: "GP-017", Point: northOf(base, 4000)},
			},
			want:    DupDecision{Numbered: true, ExistingID: 14, DistanceM: 4000},
			wantDup: true,
			tolM:    0.5,
		},
		{
			name:       "different asset_no never matches",
			newAssetNo: "GP-017",
			newPt:      base,
			existing:   []Candidate{{LightID: 19, AssetNo: "GP-018", Point: base}},
			wantDup:    false,
		},
		{
			name:       "numbered new fixture near unnumbered existing is NOT an unnumbered duplicate",
			newAssetNo: "GP-017",
			newPt:      base,
			existing:   []Candidate{{LightID: 17, AssetNo: "", Point: base}},
			wantDup:    false,
		},
		{
			name:       "unnumbered within 8 m is a duplicate",
			newAssetNo: "",
			newPt:      base,
			existing:   []Candidate{{LightID: 15, AssetNo: "", Point: northOf(base, 7.9)}},
			want:       DupDecision{Numbered: false, ExistingID: 15, DistanceM: 7.9},
			wantDup:    true,
			tolM:       0.05,
		},
		{
			name:       "unnumbered 8.1 m away is not a duplicate",
			newAssetNo: "",
			newPt:      base,
			existing:   []Candidate{{LightID: 16, AssetNo: "", Point: northOf(base, 8.1)}},
			wantDup:    false,
		},
		{
			name:       "unnumbered new fixture near a numbered existing is not a duplicate",
			newAssetNo: "",
			newPt:      base,
			existing:   []Candidate{{LightID: 18, AssetNo: "GP-030", Point: northOf(base, 3)}},
			wantDup:    false,
		},
		{
			name:       "empty candidate list never duplicates",
			newAssetNo: "GP-017",
			newPt:      base,
			existing:   nil,
			wantDup:    false,
		},
		{
			name:       "empty candidate list never duplicates an unnumbered submission",
			newAssetNo: "",
			newPt:      base,
			existing:   []Candidate{},
			wantDup:    false,
		},
		{
			name:       "nearest candidate inside the radius is reported",
			newAssetNo: "",
			newPt:      base,
			existing: []Candidate{
				{LightID: 5, AssetNo: "", Point: northOf(base, 6.9)},
				{LightID: 6, AssetNo: "", Point: northOf(base, 3.1)},
				{LightID: 7, AssetNo: "", Point: northOf(base, 7.9)},
			},
			want:    DupDecision{Numbered: false, ExistingID: 6, DistanceM: 3.1},
			wantDup: true,
			tolM:    0.05,
		},
		{
			name:       "tie keeps the earlier candidate",
			newAssetNo: "",
			newPt:      base,
			existing: []Candidate{
				{LightID: 8, AssetNo: "", Point: northOf(base, 6.5)},
				{LightID: 9, AssetNo: "", Point: northOf(base, 6.5)},
			},
			want:    DupDecision{Numbered: false, ExistingID: 8, DistanceM: 6.5},
			wantDup: true,
			tolM:    0.05,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ClassifyDuplicate(tt.newAssetNo, tt.newPt, tt.existing)
			if ok != tt.wantDup {
				t.Fatalf("ClassifyDuplicate(%q, %v, %d candidates) ok = %v, want %v (decision %+v)",
					tt.newAssetNo, tt.newPt, len(tt.existing), ok, tt.wantDup, got)
			}
			if got.Numbered != tt.want.Numbered || got.ExistingID != tt.want.ExistingID {
				t.Fatalf("ClassifyDuplicate(%q, %v, %d candidates) decision = %+v, want %+v",
					tt.newAssetNo, tt.newPt, len(tt.existing), got, tt.want)
			}
			if tt.wantDup && !approxM(got.DistanceM, tt.want.DistanceM, tt.tolM) {
				t.Fatalf("ClassifyDuplicate(%q, %v, %d candidates) DistanceM = %.4f, want %.3f (+/- %g m)",
					tt.newAssetNo, tt.newPt, len(tt.existing), got.DistanceM, tt.want.DistanceM, tt.tolM)
			}
		})
	}
}
