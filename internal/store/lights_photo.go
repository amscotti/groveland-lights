package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/amscotti/groveland-lights/internal/store/gen"
)

// EditLightPhoto applies the duplicate-merge write (A-205): it updates a live
// light's label, asset number and photo reference in ONE transaction, stamps
// updated_at and returns the row as read back. The label and photo either both
// land or neither does, so a failed photo write cannot leave a half-applied
// merge behind.
//
// A missing or soft-deleted id wraps ErrNotFound. Reusing another live light's
// asset number wraps ErrDuplicateAssetNo and rolls the whole update back; any
// other driver failure wraps the underlying error. The photo reference is
// checked by the schema's foreign key, so an unknown photo id fails the update
// (and therefore the transaction) rather than storing a dangling reference.
func (db *DB) EditLightPhoto(ctx context.Context, id LightID, label string, assetNo *string, photoID PhotoID) (Light, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return Light{}, fmt.Errorf("store: edit light photo %d: begin: %w", id, err)
	}
	// Commit's error is authoritative; Rollback after a successful Commit is a
	// harmless no-op returned by the driver.
	defer func() { _ = tx.Rollback() }()

	q := db.q.WithTx(tx)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := q.UpdateLight(ctx, gen.UpdateLightParams{
		Label:     label,
		AssetNo:   toNullString(assetNo),
		UpdatedAt: now,
		ID:        int64(id),
	}); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return Light{}, ErrNotFound
		case isDuplicateAssetNo(err):
			return Light{}, fmt.Errorf("store: edit light photo %d: %w: %w", id, ErrDuplicateAssetNo, err)
		default:
			return Light{}, fmt.Errorf("store: edit light photo %d: %w", id, err)
		}
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE lights SET photo_id = ?1 WHERE id = ?2 AND deleted_at IS NULL`,
		int64(photoID), int64(id))
	if err != nil {
		return Light{}, fmt.Errorf("store: edit light photo %d: photo: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Light{}, fmt.Errorf("store: edit light photo %d: photo rows: %w", id, err)
	}
	if n == 0 {
		return Light{}, ErrNotFound
	}
	row, err := q.GetLight(ctx, int64(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Light{}, ErrNotFound
		}
		return Light{}, fmt.Errorf("store: edit light photo %d: read back: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return Light{}, fmt.Errorf("store: edit light photo %d: commit: %w", id, err)
	}
	return lightFromGen(row)
}
