package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/amscotti/groveland-lights/internal/store/gen"
)

// NewPhoto is the parsed insert-photo input: the on-disk path (relative to
// PHOTO_DIR) of the re-encoded JPEG plus its processed pixel dimensions,
// size in bytes and hex sha256. created_at is stamped by the store; owner_id
// stays NULL until the auth slice attaches photos to user accounts.
type NewPhoto struct {
	Path   string
	Width  int
	Height int
	Bytes  int64
	SHA256 string
}

// Photo is one photo row as read back from the database.
type Photo struct {
	ID        PhotoID
	OwnerID   *int64
	Path      string
	Width     int
	Height    int
	Bytes     int64
	SHA256    string
	CreatedAt time.Time
}

// photoFromGen converts a generated row into the domain type.
func photoFromGen(g gen.Photo) (Photo, error) {
	p := Photo{
		ID:      PhotoID(g.ID),
		OwnerID: fromNullInt(g.OwnerID),
		Path:    g.Path,
		Width:   int(g.Width),
		Height:  int(g.Height),
		Bytes:   g.Bytes,
		SHA256:  g.Sha256,
	}
	var err error
	if p.CreatedAt, err = rfc3339("created_at", g.CreatedAt); err != nil {
		return Photo{}, err
	}
	return p, nil
}

// CreatePhoto inserts one photo row and returns it as read back. The path is
// UNIQUE in the schema: storing a path that is already recorded fails (the
// photo pipeline mints a fresh path per upload, so a duplicate means a
// storage collision and must surface as an error, never a silent overwrite).
func (db *DB) CreatePhoto(ctx context.Context, n NewPhoto) (Photo, error) {
	row, err := db.q.CreatePhoto(ctx, gen.CreatePhotoParams{
		Path:      n.Path,
		Width:     int64(n.Width),
		Height:    int64(n.Height),
		Bytes:     n.Bytes,
		Sha256:    n.SHA256,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return Photo{}, fmt.Errorf("store: create photo: %w", err)
	}
	return photoFromGen(row)
}

// PhotoByID returns one photo row; an unknown id wraps ErrNotFound.
func (db *DB) PhotoByID(ctx context.Context, id PhotoID) (Photo, error) {
	row, err := db.q.GetPhoto(ctx, int64(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Photo{}, ErrNotFound
		}
		return Photo{}, fmt.Errorf("store: photo %d: %w", id, err)
	}
	return photoFromGen(row)
}

// PhotoList returns up to limit photo rows in id order (oldest first).
func (db *DB) PhotoList(ctx context.Context, limit int) ([]Photo, error) {
	if limit < 0 {
		return nil, fmt.Errorf("store: photo list: limit %d must be >= 0", limit)
	}
	rows, err := db.q.ListPhotos(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("store: photo list: %w", err)
	}
	out := make([]Photo, 0, len(rows))
	for _, g := range rows {
		p, err := photoFromGen(g)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
