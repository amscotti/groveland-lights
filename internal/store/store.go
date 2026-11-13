// Package store owns the SQLite database: opening it with the right pragmas,
// applying the embedded migrations under a checksum guard, and every query.
// The public API (Open/Close/ProbeCount/ProbeInc) is frozen; other packages
// compile against it.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/amscotti/groveland-lights/internal/store/gen"

	// Pure-Go SQLite driver (no cgo). Registers as "sqlite".
	_ "modernc.org/sqlite"
)

// DB wraps one SQLite connection pool with the sqlc-generated queries.
type DB struct {
	sql *sql.DB
	q   *gen.Queries
}

// dsn builds the driver DSN: WAL journal, 5s busy timeout and enforced
// foreign keys on every pooled connection (these pragmas are per-connection,
// so they must come from the DSN, not a one-off Exec).
func dsn(dbPath string) string {
	return "file:" + dbPath +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(1)"
}

// errBadDSNPath is wrapped when dbPath contains a character that the DSN
// grammar would swallow (query separator, fragment separator or a partial
// percent-escape) instead of treating as part of the filename.
var errBadDSNPath = errors.New("cannot appear unescaped in a SQLite DSN")

// checkDSNPath rejects paths whose DSN encoding is ambiguous. Open never
// escapes silently: a path that would be reinterpreted by the driver's URL
// parsing is refused rather than opened under a different name.
func checkDSNPath(dbPath string) error {
	for _, r := range dbPath {
		if r == '?' || r == '#' || r == '%' {
			return fmt.Errorf("store: db path %q: %q %w", dbPath, string(r), errBadDSNPath)
		}
	}
	return nil
}

// Open opens (creating if necessary) the SQLite database at dbPath, applies
// any not-yet-recorded migrations in filename order, and validates that every
// already-recorded migration still has the checksum it was applied with. A
// checksum mismatch returns an error naming the offending file: callers must
// refuse to boot. Close the returned DB when done.
func Open(ctx context.Context, dbPath string) (*DB, error) {
	if err := checkDSNPath(dbPath); err != nil {
		return nil, err
	}
	sqlDB, err := sql.Open("sqlite", dsn(dbPath))
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", dbPath, err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("store: ping %s: %w", dbPath, err)
	}
	if err := applyMigrations(ctx, sqlDB, migrationsFS); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return &DB{sql: sqlDB, q: gen.New(sqlDB)}, nil
}

// Close releases the database pool. WAL data is checkpointed into the main
// database file on the last connection close.
func (db *DB) Close() error {
	if err := db.sql.Close(); err != nil {
		return fmt.Errorf("store: close: %w", err)
	}
	return nil
}

// ErrNotFound is returned when an expected row does not exist.
var ErrNotFound = errors.New("store: not found")

// ProbeCount returns the current probe counter value. It wraps ErrNotFound
// if the seeded probe row is missing.
func (db *DB) ProbeCount(ctx context.Context) (int64, error) {
	n, err := db.q.ProbeCount(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("store: probe count: %w", err)
	}
	return n, nil
}

// ProbeInc atomically increments the probe counter and returns the new value.
// The UPDATE ... RETURNING runs inside a transaction, so concurrent callers
// never lose an increment and each sees a distinct value.
func (db *DB) ProbeInc(ctx context.Context) (int64, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: probe inc: begin: %w", err)
	}
	// Commit's error is authoritative; Rollback after a successful Commit is
	// a harmless no-op returned by the driver.
	defer func() { _ = tx.Rollback() }()
	n, err := db.q.WithTx(tx).ProbeInc(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: probe inc: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: probe inc: commit: %w", err)
	}
	return n, nil
}
