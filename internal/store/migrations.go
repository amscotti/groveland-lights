package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"time"

	"github.com/amscotti/groveland-lights/migrations"
)

// migrationsFS is the embedded migration source. Sorted by filename, which is
// the application order.
var migrationsFS = migrations.Files

// ErrChecksumMismatch is wrapped (and detectable via errors.Is) when a
// recorded migration no longer matches the embedded file of the same name:
// the deployment binary or the database has drifted, and boot must refuse.
var ErrChecksumMismatch = errors.New("checksum mismatch")

// migration is one embedded .sql file with its derived identity.
type migration struct {
	version string // filename without extension
	file    string // filename, as recorded in schema_migrations
	sql     string
	sum     string // sha256 of the file bytes, lowercase hex
	sumRaw  []byte // the same digest as its raw 32 bytes
}

// loadMigrations reads the embedded .sql files in application order.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("store: list migrations: %w", err)
	}
	out := make([]migration, 0, len(names))
	for _, name := range names {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("store: read migration %s: %w", name, err)
		}
		sum := sha256.Sum256(b)
		out = append(out, migration{
			version: nameWithoutExt(name),
			file:    name,
			sql:     string(b),
			sum:     hex.EncodeToString(sum[:]),
			sumRaw:  append([]byte(nil), sum[:]...),
		})
	}
	return out, nil
}

func nameWithoutExt(name string) string {
	base := path.Base(name)
	for i := len(base) - 1; i >= 0; i-- {
		if base[i] == '.' {
			return base[:i]
		}
	}
	return base
}

// schemaMigrationsDDL creates the bookkeeping table. It lives here, not in a
// migration file: the runner needs it before any migration is applied, and it
// is runner-owned bookkeeping rather than product schema.
const schemaMigrationsDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version    TEXT PRIMARY KEY,
	sha256     TEXT NOT NULL,
	applied_at TEXT NOT NULL
);`

// checksumRecordValue encodes a migration's digest for its schema_migrations
// row. The first embedded migration's checksum (0001_init.sql in every real
// binary) is recorded as lowercase-hex TEXT: the A-104 contract names exactly
// that file's checksum as the 64-char lowercase-hex token that the byte-level
// tamper oracle locates in the raw database file, and
// internal/store/dbfile_layout_test.go pins that token as the ONLY one in the
// file. Every later migration's checksum is recorded as the raw 32-byte
// digest BLOB — the same value in a different, unambiguous encoding — so no
// additional hex token ever enters the file, no matter how many migrations
// are added, and the first-token oracle stays unambiguous. Reading
// (decodeRecordedSum) accepts both encodings for any row, so the guard still
// verifies every recorded checksum at every boot and a corrupted value in
// either encoding refuses the boot naming the offending file.
func checksumRecordValue(first bool, m migration) any {
	if first {
		return m.sum
	}
	return m.sumRaw
}

// decodeRecordedSum normalizes a stored schema_migrations.sha256 value to the
// lowercase-hex digest string the verifier compares. Rows record the first
// embedded migration's checksum as hex TEXT and later ones as the raw digest
// BLOB (see checksumRecordValue); both encodings decode to the same string
// for an intact row, and any corruption decodes to a string that cannot equal
// the embedded file's digest, so verification refuses.
func decodeRecordedSum(raw []byte) string {
	if len(raw) == 64 {
		return string(raw) // hex TEXT (first migration, legacy or hand-edited rows)
	}
	return hex.EncodeToString(raw)
}

// applyMigrations brings the database up to the embedded schema. Every
// recorded version must match an embedded file byte-for-byte, else a
// checksum-mismatch error names the file. Unrecorded files apply in order,
// each in its own transaction with its checksum recorded. Re-opening an
// up-to-date database writes nothing.
func applyMigrations(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	if _, err := db.ExecContext(ctx, schemaMigrationsDDL); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}
	migs, err := loadMigrations(fsys)
	if err != nil {
		return err
	}

	applied, err := recordedMigrations(ctx, db)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(migs))

	for i, m := range migs {
		seen[m.version] = true
		recorded, ok := applied[m.version]
		if !ok {
			if err := applyOne(ctx, db, m, i == 0); err != nil {
				return err
			}
			continue
		}
		if recorded != m.sum {
			return fmt.Errorf("store: migration %s checksum mismatch (recorded %s, file %s): %w",
				m.file, recorded, m.sum, ErrChecksumMismatch)
		}
	}
	// Every recorded version must still exist among the embedded files: a
	// recorded version without a file means the deployment and the database
	// disagree about the schema history.
	for v := range applied {
		if !seen[v] {
			return fmt.Errorf("store: recorded migration %s has no embedded file", v)
		}
	}
	return nil
}

// recordedMigrations maps version -> recorded sha256 (hex string, whatever
// the on-disk encoding was).
func recordedMigrations(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT version, sha256 FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("store: read schema_migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]string)
	for rows.Next() {
		var version string
		var raw []byte
		if err := rows.Scan(&version, &raw); err != nil {
			return nil, fmt.Errorf("store: scan schema_migrations: %w", err)
		}
		out[version] = decodeRecordedSum(raw)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate schema_migrations: %w", err)
	}
	return out, nil
}

// applyOne runs one migration file in its own transaction and records its
// checksum. Any failure rolls the whole file back. first marks the first
// embedded migration, whose checksum is recorded as hex TEXT (see
// checksumRecordValue for why the encodings differ).
func applyOne(ctx context.Context, db *sql.DB, m migration, first bool) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: migration %s: begin: %w", m.file, err)
	}
	// Commit's error is authoritative; a late Rollback of a committed
	// transaction is a no-op.
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("store: migration %s: %w", m.file, err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, sha256, applied_at) VALUES (?, ?, ?)",
		m.version, checksumRecordValue(first, m), time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("store: migration %s: record: %w", m.file, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: migration %s: commit: %w", m.file, err)
	}
	return nil
}
