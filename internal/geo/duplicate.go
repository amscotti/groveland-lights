package geo

// Candidate is one existing fixture offered for duplicate comparison. Callers
// build the slice from the near-candidate query (NumberedRadiusM window plus
// the asset_no lookup); this package only classifies.
type Candidate struct {
	LightID int64
	AssetNo string // "" when the existing fixture is unnumbered
	Point   Point
}

// DupDecision reports why a submission is a possible duplicate.
type DupDecision struct {
	Numbered   bool    // true: same non-empty asset_no as an existing light
	ExistingID int64   // the light the submission collides with
	DistanceM  float64 // metres between the submission and ExistingID's point
}

// ClassifyDuplicate returns (decision, true) when newAssetNo matches a
// candidate's non-empty asset_no (Numbered), or when BOTH the new fixture is
// unnumbered (newAssetNo == "") AND a candidate with empty AssetNo stands
// within UnnumberedRadiusM. Otherwise (zero, false).
//
// Numbered matching is distance-independent: the unique index catches exact
// asset_no reuse anywhere in town, so the first candidate in slice order with
// an equal non-empty asset_no wins. The unnumbered rule reports the nearest
// qualifying candidate; on an exact distance tie the earlier candidate wins.
func ClassifyDuplicate(newAssetNo string, newPt Point, existing []Candidate) (DupDecision, bool) {
	// Numbered: any candidate sharing the new non-empty asset_no is a
	// duplicate regardless of distance (an empty newAssetNo must never match
	// empty candidate asset numbers — that is the unnumbered rule below).
	if newAssetNo != "" {
		for _, c := range existing {
			if c.AssetNo == newAssetNo {
				return DupDecision{
					Numbered:   true,
					ExistingID: c.LightID,
					DistanceM:  DistanceM(newPt, c.Point),
				}, true
			}
		}
		return DupDecision{}, false
	}

	// Unnumbered: both fixtures unnumbered AND standing within the radius.
	// Strictly-less comparison keeps the earlier candidate on ties.
	best := -1
	var bestM float64
	for i, c := range existing {
		if c.AssetNo != "" {
			continue
		}
		d := DistanceM(newPt, c.Point)
		if d > UnnumberedRadiusM {
			continue
		}
		if best < 0 || d < bestM {
			best, bestM = i, d
		}
	}
	if best < 0 {
		return DupDecision{}, false
	}
	return DupDecision{
		Numbered:   false,
		ExistingID: existing[best].LightID,
		DistanceM:  bestM,
	}, true
}
