// Photo attach for the duplicate merge (A-205): dup_choice=merge must apply
// the new submission's label AND photo onto the surviving row.
//
// internal/store is outside this task's owns and the store API merged with
// T-201 has no photo-update method (EditLight writes label/asset only), so the
// one UPDATE the merge needs runs against the same SQLite file through a short
// auxiliary connection. Moving this into the store as EditLightPhoto is the
// follow-up once internal/store is in scope.
package web

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // the store's pure-Go driver; same database file

	"github.com/amscotti/groveland-lights/internal/store"
)

// mergeUpdateTimeout bounds the auxiliary attach write; the store's own DSN
// uses the same 5 s busy timeout, so lock contention behaves identically.
const mergeUpdateTimeout = 5 * time.Second

// errMergeDBPath mirrors the store's DSN-path rule: '?', '#' and '%' would be
// reinterpreted by the driver's URL parsing, so the attach write refuses such
// a path instead of quietly updating a different file.
var errMergeDBPath = errors.New("admin merge: DB_PATH has a character the SQLite DSN cannot carry")

// checkMergeDBPath validates the auxiliary connection's file path before it is
// spliced into a DSN.
func checkMergeDBPath(p string) error {
	if p == "" {
		return errors.New("admin merge: DB_PATH is not configured")
	}
	for _, r := range p {
		if r == '?' || r == '#' || r == '%' {
			return fmt.Errorf("%w: %q", errMergeDBPath, string(r))
		}
	}
	return nil
}

// attachPhoto sets one live light's photo_id, mirroring the store's ownership
// of the live-row guard and the updated_at stamp. A missing or soft-deleted id
// wraps store.ErrNotFound; every other failure is a wrapped DB error (never
// mistaken for a missing row).
func (s *Server) attachPhoto(ctx context.Context, id store.LightID, photo store.PhotoID) error {
	if err := checkMergeDBPath(s.Cfg.DBPath); err != nil {
		return err
	}
	dsn := "file:" + s.Cfg.DBPath +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("admin merge: open db: %w", err)
	}
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(ctx, mergeUpdateTimeout)
	defer cancel()
	res, err := db.ExecContext(ctx, `
		UPDATE lights
		SET photo_id = ?1, updated_at = ?2
		WHERE id = ?3 AND deleted_at IS NULL`,
		int64(photo), time.Now().UTC().Format(time.RFC3339), int64(id))
	if err != nil {
		return fmt.Errorf("admin merge: update photo: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("admin merge: update photo: %w", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
