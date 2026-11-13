package store

import (
	"context"
	"errors"
	"testing"
)

// testPhotoDigest is a stand-in hex sha256 for fixture rows (the path is the
// uniqueness key in these tests; the digest is just another stored column).
const testPhotoDigest = "52846307a4650b3e8b983792012314d18221c441bcd051b168ae1d059dbad8a9"

// mustCreatePhoto inserts one photo row with the given path and dimensions,
// failing the test on error.
func mustCreatePhoto(t *testing.T, ctx context.Context, db *DB, path string, w, h int) Photo {
	t.Helper()
	p, err := db.CreatePhoto(ctx, NewPhoto{
		Path:   path,
		Width:  w,
		Height: h,
		Bytes:  12345,
		SHA256: testPhotoDigest,
	})
	if err != nil {
		t.Fatalf("CreatePhoto(%s): %v", path, err)
	}
	return p
}

// TestCreatePhoto: metadata round-trip, UNIQUE-path enforcement and the
// photo -> light reference wiring the upload flow depends on.
func TestCreatePhoto(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		check func(t *testing.T, ctx context.Context, db *DB)
	}{
		{"created_photo_round_trips_every_column", checkPhotoRoundTrip},
		{"duplicate_path_is_refused", checkPhotoDuplicatePath},
		{"missing_id_wraps_err_not_found", checkPhotoByIDNotFound},
		{"created_light_can_reference_the_photo", checkPhotoReferencedByLight},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openFresh(t)
			tt.check(t, ctx, db)
		})
	}
}

func checkPhotoRoundTrip(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	created, err := db.CreatePhoto(ctx, NewPhoto{
		Path:   "photos/2026/ab12cd34.jpg",
		Width:  800,
		Height: 600,
		Bytes:  48213,
		SHA256: testPhotoDigest,
	})
	if err != nil {
		t.Fatalf("CreatePhoto: %v", err)
	}
	if created.ID <= 0 {
		t.Fatalf("created id = %d, want a positive photos.id", created.ID)
	}
	got, err := db.PhotoByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("PhotoByID: %v", err)
	}
	if got.Path != "photos/2026/ab12cd34.jpg" {
		t.Fatalf("Path = %q, want the stored path", got.Path)
	}
	if got.Width != 800 || got.Height != 600 {
		t.Fatalf("dimensions = %dx%d, want 800x600", got.Width, got.Height)
	}
	if got.Bytes != 48213 {
		t.Fatalf("Bytes = %d, want 48213", got.Bytes)
	}
	if got.SHA256 != testPhotoDigest {
		t.Fatalf("SHA256 = %q, want the stored digest", got.SHA256)
	}
	if got.OwnerID != nil {
		t.Fatalf("OwnerID = %v, want nil (photos start unowned)", got.OwnerID)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero; the store must stamp it")
	}
}

func checkPhotoDuplicatePath(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	mustCreatePhoto(t, ctx, db, "photos/same.jpg", 320, 200)
	if _, err := db.CreatePhoto(ctx, NewPhoto{
		Path: "photos/same.jpg", Width: 100, Height: 100, Bytes: 1, SHA256: "ff",
	}); err == nil {
		t.Fatal("CreatePhoto with a duplicate path succeeded, want an error")
	}
	var n int
	if err := db.sql.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM photos WHERE path = 'photos/same.jpg'").Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if n != 1 {
		t.Fatalf("photos rows for the duplicate path = %d, want 1", n)
	}
}

func checkPhotoByIDNotFound(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	if _, err := db.PhotoByID(ctx, PhotoID(999999)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("PhotoByID(missing) error = %v, want ErrNotFound", err)
	}
}

func checkPhotoReferencedByLight(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	photo := mustCreatePhoto(t, ctx, db, "photos/ref.jpg", 640, 480)
	l := mustCreateLight(t, ctx, db, NewLight{
		Kind: KindPark, Label: "Lamp with photo", Lat: grovelandLat, Lon: grovelandLon,
		PhotoID: photoPtr(photo.ID),
	})
	got, err := db.GetLight(ctx, l.ID)
	if err != nil {
		t.Fatalf("GetLight: %v", err)
	}
	if got.PhotoID == nil || *got.PhotoID != photo.ID {
		t.Fatalf("light photo reference = %v, want %d", got.PhotoID, photo.ID)
	}
}

func TestListPhotos(t *testing.T) {
	ctx := context.Background()

	t.Run("lists_in_id_order_up_to_the_limit", func(t *testing.T) {
		db, _ := openFresh(t)
		first := mustCreatePhoto(t, ctx, db, "photos/one.jpg", 10, 10)
		second := mustCreatePhoto(t, ctx, db, "photos/two.jpg", 20, 20)
		mustCreatePhoto(t, ctx, db, "photos/three.jpg", 30, 30)

		got, err := db.PhotoList(ctx, 2)
		if err != nil {
			t.Fatalf("PhotoList: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("PhotoList(limit=2) returned %d rows, want 2", len(got))
		}
		if got[0].ID != first.ID || got[1].ID != second.ID {
			t.Fatalf("order = (%d, %d), want ascending from the oldest (%d, %d)",
				got[0].ID, got[1].ID, first.ID, second.ID)
		}
	})

	t.Run("empty_table_returns_no_rows", func(t *testing.T) {
		db, _ := openFresh(t)
		got, err := db.PhotoList(ctx, 10)
		if err != nil {
			t.Fatalf("PhotoList: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("PhotoList on an empty table returned %d rows, want 0", len(got))
		}
	})
}
