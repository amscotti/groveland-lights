package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"testing"
)

// ptr helpers for building optional-column inputs in the tables below.

func strPtr(s string) *string      { return &s }
func floatPtr(f float64) *float64  { return &f }
func photoPtr(id PhotoID) *PhotoID { return &id }
func int64Ptr(i int64) *int64      { return &i }

// grovelandLat/Lon are the acceptance drafts' illustrative fixture
// coordinates (inside TOWN_BBOX); the duplicate tests build their offsets
// around this point.
const (
	grovelandLat = 42.7617
	grovelandLon = -71.0303
)

// haversineM is the test-side oracle for ground distance in metres (mean
// Earth radius), deliberately independent of the store's flat degree
// conversion: candidates are placed with it and classified with it.
func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusM = 6371008.8
	rad := math.Pi / 180
	dPhi := (lat2 - lat1) * rad
	dLambda := (lon2 - lon1) * rad
	phi1 := lat1 * rad
	phi2 := lat2 * rad
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLambda/2)*math.Sin(dLambda/2)
	return 2 * earthRadiusM * math.Asin(math.Sqrt(a))
}

// offsetNorth returns the point dM metres north of (lat, lon); offsetEast
// returns the point dM metres east.
func offsetNorth(lat, lon, dM float64) (float64, float64) {
	return lat + dM/metersPerDegree, lon
}

func offsetEast(lat, lon, dM float64) (float64, float64) {
	cos := math.Cos(lat * math.Pi / 180)
	return lat, lon + dM/(metersPerDegree*cos)
}

// mustCreateLight inserts one light or fails the test.
func mustCreateLight(t *testing.T, ctx context.Context, db *DB, n NewLight) Light {
	t.Helper()
	l, err := db.CreateLight(ctx, n)
	if err != nil {
		t.Fatalf("CreateLight(%s): %v", n.Label, err)
	}
	return l
}

// mustSoftDelete soft-deletes one light or fails the test.
func mustSoftDelete(t *testing.T, ctx context.Context, db *DB, id LightID) {
	t.Helper()
	if err := db.SoftDeleteLight(ctx, id); err != nil {
		t.Fatalf("SoftDeleteLight(%d): %v", id, err)
	}
}

// lightIDs returns the ids of a result page (test helper).
func lightIDs(ls []Light) []LightID {
	out := make([]LightID, 0, len(ls))
	for _, l := range ls {
		out = append(out, l.ID)
	}
	return out
}

// TestCreateLight covers insert + readback, duplicate detection (the A-205
// store half's numbered case) and the column constraints the schema pins.
func TestCreateLight(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		check func(t *testing.T, ctx context.Context, db *DB)
	}{
		{"created_light_round_trips_every_column_with_status_defaults", checkCreateLightRoundTrip},
		{"unset_optionals_store_as_null", checkCreateLightNullOptionals},
		{"duplicate_asset_no_wraps_sentinel_and_saves_nothing", checkCreateLightDuplicateAssetNo},
		{"asset_no_is_reusable_after_soft_delete", checkCreateLightAssetNoReusableAfterDelete},
		{"invalid_kind_is_refused", checkCreateLightInvalidKind},
		{"photo_reference_must_exist", checkCreateLightPhotoReferenceFK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openFresh(t)
			tt.check(t, ctx, db)
		})
	}
}

func checkCreateLightRoundTrip(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	// created_by references users(id): seed the referenced account raw.
	if _, err := db.sql.ExecContext(ctx,
		"INSERT INTO users (id, handle, email, role, points, created_at) "+
			"VALUES (7, 'admin', 'admin@example.test', 'admin', 0, '2026-01-01T00:00:00Z')"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	created := mustCreateLight(t, ctx, db, NewLight{
		AssetNo:   strPtr("GP-017"),
		Kind:      KindBuilding,
		Label:     "Field house door",
		Lat:       grovelandLat,
		Lon:       grovelandLon,
		AccuracyM: floatPtr(4.5),
		GmlPoleNo: strPtr("GML-042"),
		CreatedBy: int64Ptr(7),
	})
	if created.ID <= 0 {
		t.Fatalf("created id = %d, want a positive lights.id", created.ID)
	}
	got, err := db.GetLight(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetLight: %v", err)
	}
	if got.AssetNo == nil || *got.AssetNo != "GP-017" {
		t.Fatalf("AssetNo = %v, want GP-017", got.AssetNo)
	}
	if got.Kind != KindBuilding {
		t.Fatalf("Kind = %q, want building", got.Kind)
	}
	if got.Label != "Field house door" {
		t.Fatalf("Label = %q, want Field house door", got.Label)
	}
	if got.Lat != grovelandLat || got.Lon != grovelandLon {
		t.Fatalf("position = (%v, %v), want (%v, %v)", got.Lat, got.Lon, grovelandLat, grovelandLon)
	}
	checkOptionalColumns(t, got)
	checkStatusDefaults(t, ctx, db, created.ID)
}

// checkOptionalColumns asserts the explicit optional columns of the round
// trip (unset PhotoID excluded — covered by the null-optionals check).
func checkOptionalColumns(t *testing.T, got Light) {
	t.Helper()
	if got.AccuracyM == nil || *got.AccuracyM != 4.5 {
		t.Fatalf("AccuracyM = %v, want 4.5", got.AccuracyM)
	}
	if got.PhotoID != nil {
		t.Fatalf("PhotoID = %v, want nil", got.PhotoID)
	}
	if got.GmlPoleNo == nil || *got.GmlPoleNo != "GML-042" {
		t.Fatalf("GmlPoleNo = %v, want GML-042", got.GmlPoleNo)
	}
	if got.CreatedBy == nil || *got.CreatedBy != 7 {
		t.Fatalf("CreatedBy = %v, want 7", got.CreatedBy)
	}
}

// checkStatusDefaults asserts the server-side status columns a fresh row
// must carry (the values status.Derive later manages).
func checkStatusDefaults(t *testing.T, ctx context.Context, db *DB, id LightID) {
	t.Helper()
	got, err := db.GetLight(ctx, id)
	if err != nil {
		t.Fatalf("GetLight: %v", err)
	}
	if got.Status != "unknown" {
		t.Fatalf("Status = %q, want the schema default unknown", got.Status)
	}
	if got.StatusSource != "none" {
		t.Fatalf("StatusSource = %q, want the schema default none", got.StatusSource)
	}
	if got.StatusAt != nil || got.StatusConfirmedAt != nil || got.DeletedAt != nil {
		t.Fatalf("status/deleted timestamps = (%v, %v, %v), want all nil",
			got.StatusAt, got.StatusConfirmedAt, got.DeletedAt)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps = (%v, %v), want both stamped", got.CreatedAt, got.UpdatedAt)
	}
}

func checkCreateLightNullOptionals(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	created := mustCreateLight(t, ctx, db, NewLight{
		Kind: KindPark, Label: "Path light", Lat: grovelandLat, Lon: grovelandLon,
	})
	got, err := db.GetLight(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetLight: %v", err)
	}
	if got.AssetNo != nil || got.AccuracyM != nil || got.PhotoID != nil ||
		got.GmlPoleNo != nil || got.CreatedBy != nil {
		t.Fatalf("optional columns = (%v, %v, %v, %v, %v), want all nil",
			got.AssetNo, got.AccuracyM, got.PhotoID, got.GmlPoleNo, got.CreatedBy)
	}
}

func checkCreateLightDuplicateAssetNo(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	mustCreateLight(t, ctx, db, NewLight{
		AssetNo: strPtr("GP-017"), Kind: KindPark,
		Label: "Ball field lamp", Lat: grovelandLat, Lon: grovelandLon,
	})
	_, err := db.CreateLight(ctx, NewLight{
		AssetNo: strPtr("GP-017"), Kind: KindPark,
		Label: "Ball field lamp twin",
		Lat:   grovelandLat + 0.00005, Lon: grovelandLon + 0.00005,
	})
	if !errors.Is(err, ErrDuplicateAssetNo) {
		t.Fatalf("duplicate asset_no error = %v, want ErrDuplicateAssetNo", err)
	}
	var n int
	if err := db.sql.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM lights WHERE asset_no = 'GP-017'").Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if n != 1 {
		t.Fatalf("lights rows for GP-017 = %d, want 1 (the twin must not be saved)", n)
	}
}

func checkCreateLightAssetNoReusableAfterDelete(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	first := mustCreateLight(t, ctx, db, NewLight{
		AssetNo: strPtr("GP-018"), Kind: KindPark,
		Label: "Old fixture", Lat: grovelandLat, Lon: grovelandLon,
	})
	mustSoftDelete(t, ctx, db, first.ID)
	if _, err := db.CreateLight(ctx, NewLight{
		AssetNo: strPtr("GP-018"), Kind: KindPark,
		Label: "Replacement fixture", Lat: grovelandLat, Lon: grovelandLon,
	}); err != nil {
		t.Fatalf("reusing a deleted light's asset_no failed: %v", err)
	}
}

func checkCreateLightInvalidKind(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	if _, err := db.CreateLight(ctx, NewLight{
		Kind: "shop", Label: "Not a kind", Lat: grovelandLat, Lon: grovelandLon,
	}); err == nil {
		t.Fatal("CreateLight with kind=shop succeeded, want an error")
	}
}

func checkCreateLightPhotoReferenceFK(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	if _, err := db.CreateLight(ctx, NewLight{
		Kind: KindPark, Label: "Dangling photo", Lat: grovelandLat, Lon: grovelandLon,
		PhotoID: photoPtr(PhotoID(999)),
	}); err == nil {
		t.Fatal("CreateLight with a non-existent photo_id succeeded, want an FK error")
	}
	seed := mustCreatePhoto(t, ctx, db, "photos/ok.jpg", 320, 200)
	if _, err := db.CreateLight(ctx, NewLight{
		Kind: KindPark, Label: "Attached photo", Lat: grovelandLat, Lon: grovelandLon,
		PhotoID: photoPtr(seed.ID),
	}); err != nil {
		t.Fatalf("CreateLight with an existing photo_id: %v", err)
	}
}

// TestGetLight: soft delete hides the row from direct reads (A-208 store
// half); unknown ids wrap ErrNotFound.
func TestGetLight(t *testing.T) {
	ctx := context.Background()
	t.Run("missing_id_wraps_err_not_found", func(t *testing.T) {
		db, _ := openFresh(t)
		if _, err := db.GetLight(ctx, LightID(999999)); !errors.Is(err, ErrNotFound) {
			t.Fatalf("GetLight(missing) error = %v, want ErrNotFound", err)
		}
	})
	t.Run("soft_deleted_id_wraps_err_not_found", func(t *testing.T) {
		db, _ := openFresh(t)
		l := mustCreateLight(t, ctx, db, NewLight{
			Kind: KindPark, Label: "Gone lamp", Lat: grovelandLat, Lon: grovelandLon,
		})
		mustSoftDelete(t, ctx, db, l.ID)
		if _, err := db.GetLight(ctx, l.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("GetLight(deleted) error = %v, want ErrNotFound", err)
		}
	})
}

// TestLightByAssetNo: the numbered-duplicate lookup must match live rows
// only — a deleted fixture's asset number is invisible (and reusable).
func TestLightByAssetNo(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		check func(t *testing.T, ctx context.Context, db *DB)
	}{
		{"finds_the_seeded_live_light", checkLightByAssetNoFindsLive},
		{"unknown_asset_no_wraps_err_not_found", checkLightByAssetNoUnknown},
		{"soft_deleted_asset_no_wraps_err_not_found", checkLightByAssetNoDeleted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openFresh(t)
			tt.check(t, ctx, db)
		})
	}
}

func checkLightByAssetNoFindsLive(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	created := mustCreateLight(t, ctx, db, NewLight{
		AssetNo: strPtr("GP-017"), Kind: KindPark,
		Label: "Ball field lamp", Lat: grovelandLat, Lon: grovelandLon,
	})
	got, err := db.LightByAssetNo(ctx, "GP-017")
	if err != nil {
		t.Fatalf("LightByAssetNo: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("LightByAssetNo id = %d, want %d", got.ID, created.ID)
	}
}

func checkLightByAssetNoUnknown(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	if _, err := db.LightByAssetNo(ctx, "GP-404"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("LightByAssetNo(unknown) error = %v, want ErrNotFound", err)
	}
}

func checkLightByAssetNoDeleted(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	l := mustCreateLight(t, ctx, db, NewLight{
		AssetNo: strPtr("GP-DEL"), Kind: KindPark,
		Label: "Removed fixture", Lat: grovelandLat, Lon: grovelandLon,
	})
	mustSoftDelete(t, ctx, db, l.ID)
	if _, err := db.LightByAssetNo(ctx, "GP-DEL"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("LightByAssetNo(deleted) error = %v, want ErrNotFound", err)
	}
}

// seedLights creates n distinct lights and returns them oldest first.
func seedLights(t *testing.T, ctx context.Context, db *DB, n int) []Light {
	t.Helper()
	out := make([]Light, 0, n)
	for i := 0; i < n; i++ {
		lat, lon := offsetNorth(grovelandLat, grovelandLon, float64(30*i+2))
		out = append(out, mustCreateLight(t, ctx, db, NewLight{
			Kind: KindPark, Label: "Seed lamp", Lat: lat, Lon: lon,
		}))
	}
	return out
}

// TestRecentLights pins the admin list's query shape: newest first with the
// frozen id DESC tie-break, paging without overlap, deleted excluded.
func TestRecentLights(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		check func(t *testing.T, ctx context.Context, db *DB)
	}{
		{"newest_first_with_id_desc_tiebreak", checkRecentNewestFirst},
		{"limit_and_offset_page_without_overlap", checkRecentPaging},
		{"soft_deleted_lights_are_excluded", checkRecentExcludesDeleted},
		{"negative_limit_or_offset_are_refused", checkRecentRejectsNegative},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openFresh(t)
			tt.check(t, ctx, db)
		})
	}
}

func checkRecentNewestFirst(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	seeded := seedLights(t, ctx, db, 3)
	got, err := db.RecentLights(ctx, 10, 0)
	if err != nil {
		t.Fatalf("RecentLights: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("RecentLights returned %d rows, want 3", len(got))
	}
	// Rapid creates share one created_at second, so the frozen id DESC
	// tie-break decides: the last-seeded id must come first.
	if got[0].ID != seeded[len(seeded)-1].ID {
		t.Fatalf("first row id = %d, want the newest seed %d", got[0].ID, seeded[len(seeded)-1].ID)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].ID <= got[i].ID {
			t.Fatalf("rows not newest-first: ids %v", lightIDs(got))
		}
	}
}

func checkRecentPaging(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	seedLights(t, ctx, db, 5)
	page1, err := db.RecentLights(ctx, 3, 0)
	if err != nil {
		t.Fatalf("RecentLights page 1: %v", err)
	}
	page2, err := db.RecentLights(ctx, 3, 3)
	if err != nil {
		t.Fatalf("RecentLights page 2: %v", err)
	}
	if len(page1) != 3 || len(page2) != 2 {
		t.Fatalf("page sizes = (%d, %d), want (3, 2)", len(page1), len(page2))
	}
	seen := map[LightID]bool{}
	for _, l := range append(append([]Light{}, page1...), page2...) {
		if seen[l.ID] {
			t.Fatalf("id %d appears on both pages", l.ID)
		}
		seen[l.ID] = true
	}
	if page1[0].ID == page2[0].ID {
		t.Fatal("page 2 repeats page 1's head")
	}
}

func checkRecentExcludesDeleted(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	seeded := seedLights(t, ctx, db, 3)
	mustSoftDelete(t, ctx, db, seeded[1].ID)
	got, err := db.RecentLights(ctx, 10, 0)
	if err != nil {
		t.Fatalf("RecentLights: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("RecentLights returned %d rows, want 2 (deleted excluded)", len(got))
	}
	for _, l := range got {
		if l.ID == seeded[1].ID {
			t.Fatalf("deleted light %d is still listed", seeded[1].ID)
		}
	}
}

func checkRecentRejectsNegative(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	if _, err := db.RecentLights(ctx, -1, 0); err == nil {
		t.Fatal("RecentLights(limit=-1) succeeded, want an error")
	}
	if _, err := db.RecentLights(ctx, 10, -2); err == nil {
		t.Fatal("RecentLights(offset=-2) succeeded, want an error")
	}
}

// seedFix is one placed fixture in the duplicate-detection table.
type seedFix struct {
	name string
	n    NewLight
	lat  float64
	lon  float64
}

// dupSeed builds the A-205 candidate set around the drafts' fixture point:
// Haversine-derived offsets straddling the 8 m (unnumbered) and 25 m
// (numbered) duplicate radii, one far fixture and one deleted fixture.
func dupSeed() []seedFix {
	lat790, lon790 := offsetNorth(grovelandLat, grovelandLon, 7.9)
	lat810, lon810 := offsetNorth(grovelandLat, grovelandLon, 8.1)
	lat2490, lon2490 := offsetEast(grovelandLat, grovelandLon, 24.9)
	lat2510, lon2510 := offsetEast(grovelandLat, grovelandLon, 25.1)
	latFar, lonFar := offsetEast(grovelandLat, grovelandLon, 150)
	latGone, lonGone := offsetNorth(grovelandLat, grovelandLon, 5)
	return []seedFix{
		{"base", NewLight{AssetNo: strPtr("GP-017"), Kind: KindPark, Label: "Ball field lamp"}, grovelandLat, grovelandLon},
		{"north_7_9m", NewLight{Kind: KindPark, Label: "Path light one"}, lat790, lon790},
		{"north_8_1m", NewLight{Kind: KindPark, Label: "Path light two"}, lat810, lon810},
		{"east_24_9m", NewLight{Kind: KindStreet, Label: "Street lamp near"}, lat2490, lon2490},
		{"east_25_1m", NewLight{Kind: KindStreet, Label: "Street lamp far"}, lat2510, lon2510},
		{"east_150m", NewLight{Kind: KindPark, Label: "Different park"}, latFar, lonFar},
		{"deleted_5m", NewLight{AssetNo: strPtr("GP-DEL"), Kind: KindPark, Label: "Removed fixture"}, latGone, lonGone},
	}
}

// TestNearbyLights is the A-205 duplicate-detection store half: candidates
// seeded at Haversine-derived offsets must reach the bbox prefilter at the
// 8 m and 25 m duplicate radii, soft-deleted and far fixtures must never be
// returned, and rows just outside the exact radius (8.1 m, 25.1 m) surface
// by design (the documented over-cover margin) with their exact distance
// above the radius — classifying them out is internal/geo's job.
func TestNearbyLights(t *testing.T) {
	ctx := context.Background()
	t.Run("unnumbered_radius_8m_includes_inside_and_threshold_candidates", func(t *testing.T) {
		db, _ := openFresh(t)
		made := seedDupCandidates(t, ctx, db)
		checkNearby(t, ctx, db, made, 8,
			[]string{"base", "north_7_9m", "north_8_1m"},
			[]string{"east_24_9m", "east_25_1m", "east_150m", "deleted_5m"})
	})
	t.Run("numbered_radius_25m_includes_inside_and_threshold_candidates", func(t *testing.T) {
		db, _ := openFresh(t)
		made := seedDupCandidates(t, ctx, db)
		checkNearby(t, ctx, db, made, 25,
			[]string{"base", "north_7_9m", "north_8_1m", "east_24_9m", "east_25_1m"},
			[]string{"east_150m", "deleted_5m"})
	})
	t.Run("just_outside_candidates_carry_distance_above_the_radius", func(t *testing.T) {
		db, _ := openFresh(t)
		made := seedDupCandidates(t, ctx, db)
		checkNearbyOutsideRows(t, ctx, db, made)
	})
	t.Run("asset_no_lookup_matches_only_the_live_fixture", func(t *testing.T) {
		db, _ := openFresh(t)
		made := seedDupCandidates(t, ctx, db)
		got, err := db.LightByAssetNo(ctx, "GP-017")
		if err != nil {
			t.Fatalf("LightByAssetNo(GP-017): %v", err)
		}
		if got.ID != made["base"].ID {
			t.Fatalf("LightByAssetNo id = %d, want the seeded %d", got.ID, made["base"].ID)
		}
		if _, err := db.LightByAssetNo(ctx, "GP-DEL"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("deleted asset lookup error = %v, want ErrNotFound", err)
		}
	})
	t.Run("negative_radius_is_refused", func(t *testing.T) {
		db, _ := openFresh(t)
		if _, err := db.NearbyLights(ctx, grovelandLat, grovelandLon, -1); err == nil {
			t.Fatal("NearbyLights(radius=-1) succeeded, want an error")
		}
	})
}

// seedDupCandidates inserts the A-205 candidate set and soft-deletes the
// deleted fixture, returning the made rows by seed name.
func seedDupCandidates(t *testing.T, ctx context.Context, db *DB) map[string]Light {
	t.Helper()
	seed := dupSeed()
	made := map[string]Light{}
	for _, s := range seed {
		made[s.name] = mustCreateLight(t, ctx, db, NewLight{
			AssetNo: s.n.AssetNo, Kind: s.n.Kind, Label: s.n.Label,
			Lat: s.lat, Lon: s.lon,
		})
	}
	mustSoftDelete(t, ctx, db, made["deleted_5m"].ID)
	return made
}

// checkNearby asserts one radius's membership: every named candidate in
// wantIn must be returned, none of wantOut may be, and no deleted row may
// appear.
func checkNearby(t *testing.T, ctx context.Context, db *DB, made map[string]Light, radius float64, wantIn, wantOut []string) {
	t.Helper()
	got, err := db.NearbyLights(ctx, grovelandLat, grovelandLon, radius)
	if err != nil {
		t.Fatalf("NearbyLights(r=%v): %v", radius, err)
	}
	returned := map[string]bool{}
	for _, l := range got {
		for name, m := range made {
			if l.ID == m.ID {
				returned[name] = true
			}
		}
		if l.DeletedAt != nil {
			t.Errorf("radius %v m: soft-deleted light %d returned", radius, l.ID)
		}
	}
	for _, name := range wantIn {
		if !returned[name] {
			s := seedByName(t, dupSeed(), name)
			t.Errorf("radius %v m: candidate %q (%.1f m away) missing from results",
				radius, name, haversineM(grovelandLat, grovelandLon, s.lat, s.lon))
		}
	}
	for _, name := range wantOut {
		if returned[name] {
			t.Errorf("radius %v m: candidate %q must not be returned", radius, name)
		}
	}
}

// checkNearbyOutsideRows pins the division of labour: the 8.1 m and 25.1 m
// candidates appear in the prefilter while their Haversine distance is above
// the radius, so the exact Go-side filter (internal/geo) is what excludes
// them — the store never clips near-threshold candidates.
func checkNearbyOutsideRows(t *testing.T, ctx context.Context, db *DB, made map[string]Light) {
	t.Helper()
	got8, err := db.NearbyLights(ctx, grovelandLat, grovelandLon, 8)
	if err != nil {
		t.Fatalf("NearbyLights(r=8): %v", err)
	}
	got25, err := db.NearbyLights(ctx, grovelandLat, grovelandLon, 25)
	if err != nil {
		t.Fatalf("NearbyLights(r=25): %v", err)
	}
	seen8, seen25 := false, false
	for _, l := range got8 {
		if l.ID == made["north_8_1m"].ID {
			seen8 = true
		}
	}
	for _, l := range got25 {
		if l.ID == made["east_25_1m"].ID {
			seen25 = true
		}
	}
	lat810, lon810 := offsetNorth(grovelandLat, grovelandLon, 8.1)
	lat2510, lon2510 := offsetEast(grovelandLat, grovelandLon, 25.1)
	if seen8 {
		if d := haversineM(grovelandLat, grovelandLon, lat810, lon810); d <= 8 {
			t.Fatalf("8 m results hold a row whose distance %.3f m is inside the radius; seed offsets are wrong", d)
		}
	}
	if seen25 {
		if d := haversineM(grovelandLat, grovelandLon, lat2510, lon2510); d <= 25 {
			t.Fatalf("25 m results hold a row whose distance %.3f m is inside the radius; seed offsets are wrong", d)
		}
	}
}

// seedByName finds a seed fixture by name (test helper).
func seedByName(t *testing.T, seed []seedFix, name string) seedFix {
	t.Helper()
	for _, s := range seed {
		if s.name == name {
			return s
		}
	}
	t.Fatalf("unknown seed name %q", name)
	return seedFix{}
}

// TestEditLight: label + asset_no updates, untouched columns preserved,
// ErrNotFound on missing/deleted ids, ErrDuplicateAssetNo on collisions.
func TestEditLight(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		check func(t *testing.T, ctx context.Context, db *DB)
	}{
		{"edit_updates_label_and_asset_no_and_stamps_updated_at", checkEditUpdatesColumns},
		{"edit_can_clear_the_asset_number", checkEditClearsAssetNo},
		{"missing_or_deleted_id_wraps_err_not_found", checkEditNotFound},
		{"reusing_another_lights_asset_no_wraps_duplicate_sentinel", checkEditDuplicate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openFresh(t)
			tt.check(t, ctx, db)
		})
	}
}

func checkEditUpdatesColumns(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	created := mustCreateLight(t, ctx, db, NewLight{
		AssetNo: strPtr("GP-001"), Kind: KindPark,
		Label: "Old label", Lat: grovelandLat, Lon: grovelandLon,
	})
	edited, err := db.EditLight(ctx, created.ID, "New label", strPtr("GP-002"))
	if err != nil {
		t.Fatalf("EditLight: %v", err)
	}
	if edited.Label != "New label" {
		t.Fatalf("edited label = %q, want New label", edited.Label)
	}
	if edited.AssetNo == nil || *edited.AssetNo != "GP-002" {
		t.Fatalf("edited asset_no = %v, want GP-002", edited.AssetNo)
	}
	if edited.Kind != KindPark {
		t.Fatalf("edited kind = %q, want park (untouched column changed)", edited.Kind)
	}
	if edited.Lat != grovelandLat || edited.Lon != grovelandLon {
		t.Fatalf("edited position = (%v, %v), want unchanged", edited.Lat, edited.Lon)
	}
	got, err := db.GetLight(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetLight after edit: %v", err)
	}
	if got.Label != "New label" || got.AssetNo == nil || *got.AssetNo != "GP-002" {
		t.Fatalf("persisted row = (%q, %v), want (New label, GP-002)", got.Label, got.AssetNo)
	}
	if got.UpdatedAt.Before(got.CreatedAt) {
		t.Fatalf("updated_at %v before created_at %v", got.UpdatedAt, got.CreatedAt)
	}
}

func checkEditClearsAssetNo(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	created := mustCreateLight(t, ctx, db, NewLight{
		AssetNo: strPtr("GP-003"), Kind: KindPark,
		Label: "Numbered", Lat: grovelandLat, Lon: grovelandLon,
	})
	if _, err := db.EditLight(ctx, created.ID, "Numbered", nil); err != nil {
		t.Fatalf("EditLight(nil asset): %v", err)
	}
	got, err := db.GetLight(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetLight: %v", err)
	}
	if got.AssetNo != nil {
		t.Fatalf("asset_no = %v, want nil after the clear", got.AssetNo)
	}
}

func checkEditNotFound(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	if _, err := db.EditLight(ctx, LightID(999999), "x", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("EditLight(missing) error = %v, want ErrNotFound", err)
	}
	l := mustCreateLight(t, ctx, db, NewLight{
		Kind: KindPark, Label: "Doomed", Lat: grovelandLat, Lon: grovelandLon,
	})
	mustSoftDelete(t, ctx, db, l.ID)
	if _, err := db.EditLight(ctx, l.ID, "zombie", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("EditLight(deleted) error = %v, want ErrNotFound", err)
	}
}

func checkEditDuplicate(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	mustCreateLight(t, ctx, db, NewLight{
		AssetNo: strPtr("GP-A"), Kind: KindPark, Label: "A", Lat: grovelandLat, Lon: grovelandLon,
	})
	b := mustCreateLight(t, ctx, db, NewLight{
		AssetNo: strPtr("GP-B"), Kind: KindPark, Label: "B", Lat: grovelandLat, Lon: grovelandLon,
	})
	if _, err := db.EditLight(ctx, b.ID, "B", strPtr("GP-A")); !errors.Is(err, ErrDuplicateAssetNo) {
		t.Fatalf("EditLight onto GP-A error = %v, want ErrDuplicateAssetNo", err)
	}
	got, err := db.GetLight(ctx, b.ID)
	if err != nil {
		t.Fatalf("GetLight: %v", err)
	}
	if got.AssetNo == nil || *got.AssetNo != "GP-B" {
		t.Fatalf("refused edit changed the row anyway: asset_no = %v, want GP-B", got.AssetNo)
	}
}

// TestSoftDeleteLight is the A-208 store half: one delete hides the row from
// every read, stamps deleted_at in place, and frees the asset number.
func TestSoftDeleteLight(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		check func(t *testing.T, ctx context.Context, db *DB)
	}{
		{"deleted_light_vanishes_from_every_read_and_frees_the_asset_no", checkSoftDeleteHidesEverywhere},
		{"the_row_is_soft_deleted_not_hard_deleted", checkSoftDeleteStampsColumn},
		{"missing_or_already_deleted_id_wraps_err_not_found", checkSoftDeleteNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openFresh(t)
			tt.check(t, ctx, db)
		})
	}
}

func checkSoftDeleteHidesEverywhere(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	l := mustCreateLight(t, ctx, db, NewLight{
		AssetNo: strPtr("GP-019"), Kind: KindPark,
		Label: "Condemned lamp", Lat: grovelandLat, Lon: grovelandLon,
	})
	mustSoftDelete(t, ctx, db, l.ID)
	if _, err := db.GetLight(ctx, l.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetLight after delete = %v, want ErrNotFound", err)
	}
	if _, err := db.LightByAssetNo(ctx, "GP-019"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("LightByAssetNo after delete = %v, want ErrNotFound", err)
	}
	recent, err := db.RecentLights(ctx, 10, 0)
	if err != nil {
		t.Fatalf("RecentLights: %v", err)
	}
	for _, r := range recent {
		if r.ID == l.ID {
			t.Fatal("deleted light still in RecentLights")
		}
	}
	near, err := db.NearbyLights(ctx, grovelandLat, grovelandLon, 25)
	if err != nil {
		t.Fatalf("NearbyLights: %v", err)
	}
	for _, r := range near {
		if r.ID == l.ID {
			t.Fatal("deleted light still in NearbyLights")
		}
	}
	if _, err := db.CreateLight(ctx, NewLight{
		AssetNo: strPtr("GP-019"), Kind: KindPark,
		Label: "Successor lamp", Lat: grovelandLat, Lon: grovelandLon,
	}); err != nil {
		t.Fatalf("asset_no not freed by soft delete: %v", err)
	}
}

func checkSoftDeleteStampsColumn(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	l := mustCreateLight(t, ctx, db, NewLight{
		Kind: KindPark, Label: "Softly gone", Lat: grovelandLat, Lon: grovelandLon,
	})
	mustSoftDelete(t, ctx, db, l.ID)
	var deletedAt sql.NullString
	if err := db.sql.QueryRowContext(ctx,
		"SELECT deleted_at FROM lights WHERE id = ?", int64(l.ID)).Scan(&deletedAt); err != nil {
		t.Fatalf("select deleted row: %v", err)
	}
	if !deletedAt.Valid || deletedAt.String == "" {
		t.Fatal("deleted_at is NULL after SoftDeleteLight; the row was hard-deleted or not stamped")
	}
}

func checkSoftDeleteNotFound(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	if err := db.SoftDeleteLight(ctx, LightID(999999)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SoftDeleteLight(missing) error = %v, want ErrNotFound", err)
	}
	l := mustCreateLight(t, ctx, db, NewLight{
		Kind: KindPark, Label: "Once", Lat: grovelandLat, Lon: grovelandLon,
	})
	mustSoftDelete(t, ctx, db, l.ID)
	if err := db.SoftDeleteLight(ctx, l.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second SoftDeleteLight error = %v, want ErrNotFound", err)
	}
}

func TestParseKind(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Kind
		wantErr bool
	}{
		{name: "park_is_valid", in: "park", want: KindPark},
		{name: "building_is_valid", in: "building", want: KindBuilding},
		{name: "street_is_valid", in: "street", want: KindStreet},
		{name: "capitalised_is_refused", in: "Park", wantErr: true},
		{name: "empty_is_refused", in: "", wantErr: true},
		{name: "padded_is_refused", in: "park ", wantErr: true},
		{name: "unknown_word_is_refused", in: "shop", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseKind(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseKind(%q) = %q, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseKind(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("ParseKind(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBBoxDegrees(t *testing.T) {
	tests := []struct {
		name string
		lat  float64
		rM   float64
	}{
		{name: "zero_radius_spans_nothing", lat: grovelandLat, rM: 0},
		{name: "groveland_latitude_radius_25m", lat: grovelandLat, rM: 25},
		{name: "groveland_latitude_radius_8m", lat: grovelandLat, rM: 8},
		{name: "near_polar_latitude_stays_finite", lat: 89.9, rM: 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkBBox(t, tt.lat, tt.rM)
		})
	}
}

func checkBBox(t *testing.T, lat, rM float64) {
	t.Helper()
	latDelta, lonDelta := bboxDegrees(lat, rM)
	if math.IsNaN(latDelta) || math.IsInf(latDelta, 0) {
		t.Fatalf("latDelta = %v, want finite", latDelta)
	}
	if math.IsNaN(lonDelta) || math.IsInf(lonDelta, 0) {
		t.Fatalf("lonDelta = %v, want finite", lonDelta)
	}
	if rM == 0 {
		if latDelta != 0 || lonDelta != 0 {
			t.Fatalf("deltas = (%v, %v), want zero for a zero radius", latDelta, lonDelta)
		}
		return
	}
	// The latitude span must over-cover the radius (the superset contract)
	// and stay within radius * safety plus rounding slack.
	spanM := latDelta * metersPerDegree
	if spanM < rM {
		t.Fatalf("latitude span %.3f m clips the %.1f m radius", spanM, rM)
	}
	if spanM > rM*nearbySafety*1.01 {
		t.Fatalf("latitude span %.3f m exceeds radius * safety (%.3f m)", spanM, rM*nearbySafety)
	}
	// The longitude span at this latitude must likewise over-cover.
	cos := math.Abs(math.Cos(lat * math.Pi / 180))
	lonSpanM := lonDelta * metersPerDegree * cos
	if lonSpanM < rM && lonDelta < 180 {
		t.Fatalf("longitude span %.3f m clips the %.1f m radius", lonSpanM, rM)
	}
}
