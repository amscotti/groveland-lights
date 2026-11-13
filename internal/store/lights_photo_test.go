package store

import (
	"context"
	"errors"
	"testing"
)

// TestEditLightPhoto covers the duplicate-merge store write (A-205): the label,
// asset number and photo reference update together, and every failure branch
// leaves the row untouched.
func TestEditLightPhoto(t *testing.T) {
	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{"updates label asset and photo in one call", checkEditLightPhotoRoundTrip},
		{"unknown id wraps ErrNotFound", checkEditLightPhotoMissing},
		{"soft-deleted id wraps ErrNotFound", checkEditLightPhotoDeleted},
		{"duplicate asset rolls the label and photo back", checkEditLightPhotoDuplicateAsset},
		{"unknown photo id fails and rolls the label back", checkEditLightPhotoUnknownPhoto},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

func checkEditLightPhotoRoundTrip(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	db, _ := openFresh(t)
	first := mustCreatePhoto(t, ctx, db, "photos/first.jpg", 320, 200)
	second := mustCreatePhoto(t, ctx, db, "photos/second.jpg", 800, 600)
	light := mustCreateLight(t, ctx, db, NewLight{
		AssetNo: strPtr("GP-001"), Kind: KindPark, Label: "Original",
		Lat: grovelandLat, Lon: grovelandLon, PhotoID: photoPtr(first.ID),
	})

	got, err := db.EditLightPhoto(ctx, light.ID, "Merged label", strPtr("GP-002"), second.ID)
	if err != nil {
		t.Fatalf("EditLightPhoto: %v", err)
	}
	if got.Label != "Merged label" {
		t.Errorf("Label = %q, want Merged label", got.Label)
	}
	if got.AssetNo == nil || *got.AssetNo != "GP-002" {
		t.Errorf("AssetNo = %v, want GP-002", got.AssetNo)
	}
	if got.PhotoID == nil || *got.PhotoID != second.ID {
		t.Errorf("PhotoID = %v, want %d", got.PhotoID, second.ID)
	}

	readBack, err := db.GetLight(ctx, light.ID)
	if err != nil {
		t.Fatalf("GetLight: %v", err)
	}
	if readBack.Label != "Merged label" || readBack.PhotoID == nil || *readBack.PhotoID != second.ID {
		t.Errorf("read back = %+v, want the merged label and photo", readBack)
	}
}

func checkEditLightPhotoMissing(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	db, _ := openFresh(t)
	photo := mustCreatePhoto(t, ctx, db, "photos/missing.jpg", 10, 10)
	if _, err := db.EditLightPhoto(ctx, LightID(9_999_999), "Nope", nil, photo.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("EditLightPhoto(missing) = %v, want ErrNotFound", err)
	}
}

func checkEditLightPhotoDeleted(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	db, _ := openFresh(t)
	photo := mustCreatePhoto(t, ctx, db, "photos/deleted.jpg", 10, 10)
	light := mustCreateLight(t, ctx, db, NewLight{
		Kind: KindPark, Label: "Deleted", Lat: grovelandLat, Lon: grovelandLon,
	})
	mustSoftDelete(t, ctx, db, light.ID)
	if _, err := db.EditLightPhoto(ctx, light.ID, "Nope", nil, photo.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("EditLightPhoto(deleted) = %v, want ErrNotFound", err)
	}
}

func checkEditLightPhotoDuplicateAsset(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	db, _ := openFresh(t)
	first := mustCreatePhoto(t, ctx, db, "photos/keep.jpg", 320, 200)
	second := mustCreatePhoto(t, ctx, db, "photos/new.jpg", 640, 480)
	mustCreateLight(t, ctx, db, NewLight{
		AssetNo: strPtr("GP-100"), Kind: KindPark, Label: "Taken",
		Lat: grovelandLat, Lon: grovelandLon,
	})
	target := mustCreateLight(t, ctx, db, NewLight{
		AssetNo: strPtr("GP-101"), Kind: KindPark, Label: "Target",
		Lat: grovelandLat, Lon: grovelandLon, PhotoID: photoPtr(first.ID),
	})

	if _, err := db.EditLightPhoto(ctx, target.ID, "Changed", strPtr("GP-100"), second.ID); !errors.Is(err, ErrDuplicateAssetNo) {
		t.Fatalf("EditLightPhoto(duplicate asset) = %v, want ErrDuplicateAssetNo", err)
	}
	after, err := db.GetLight(ctx, target.ID)
	if err != nil {
		t.Fatalf("GetLight: %v", err)
	}
	if after.Label != "Target" {
		t.Errorf("rolled-back Label = %q, want Target", after.Label)
	}
	if after.PhotoID == nil || *after.PhotoID != first.ID {
		t.Errorf("rolled-back PhotoID = %v, want %d", after.PhotoID, first.ID)
	}
}

func checkEditLightPhotoUnknownPhoto(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	db, _ := openFresh(t)
	photo := mustCreatePhoto(t, ctx, db, "photos/original.jpg", 320, 200)
	target := mustCreateLight(t, ctx, db, NewLight{
		Kind: KindPark, Label: "Original", Lat: grovelandLat, Lon: grovelandLon,
		PhotoID: photoPtr(photo.ID),
	})

	_, err := db.EditLightPhoto(ctx, target.ID, "Changed", nil, PhotoID(9_999_999))
	if err == nil {
		t.Fatal("EditLightPhoto(unknown photo) succeeded, want a foreign-key error")
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown photo = %v, want a DB error not ErrNotFound", err)
	}
	after, err := db.GetLight(ctx, target.ID)
	if err != nil {
		t.Fatalf("GetLight: %v", err)
	}
	if after.Label != "Original" || after.PhotoID == nil || *after.PhotoID != photo.ID {
		t.Errorf("failed photo write left %+v, want the original label/photo", after)
	}
}
