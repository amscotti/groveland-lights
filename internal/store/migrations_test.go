package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// openFresh opens a store on a brand-new database file in a fresh temp dir.
func openFresh(t *testing.T) (*DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

// reHexSum matches a 64-char lowercase hex sha256 as recorded in
// schema_migrations (A-104 frozen contract).
var reHexSum = regexp.MustCompile(`^[0-9a-f]{64}$`)

// embeddedSum returns the checksum the runner derives for an embedded file.
func embeddedSum(t *testing.T, version string) string {
	t.Helper()
	migs, err := loadMigrations(migrationsFS)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	for _, m := range migs {
		if m.version == version {
			return m.sum
		}
	}
	t.Fatalf("no embedded migration with version %q", version)
	return ""
}

func TestMigrationsApplyCleanly(t *testing.T) {
	db, path := openFresh(t)

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name:  "probe_and_schema_migrations_are_the_only_tables",
			check: func(t *testing.T) { checkOnlyProbeAndBookkeepingTables(t, db) },
		},
		{
			name:  "probe_table_columns_match_migration_ddl",
			check: func(t *testing.T) { checkProbeColumns(t, db) },
		},
		{
			name:  "counter_starts_at_zero",
			check: func(t *testing.T) { checkCounterStartsAtZero(t, db) },
		},
		{
			name:  "probe_row_is_seeded_with_id_1",
			check: func(t *testing.T) { checkProbeRowSeeded(t, db) },
		},
		{
			name:  "migration_recorded_as_0001_init_version",
			check: func(t *testing.T) { checkRecordedVersion(t, db) },
		},
		{
			name:  "recorded_checksum_is_64_char_lowercase_hex_of_file",
			check: func(t *testing.T) { checkRecordedChecksum(t, db) },
		},
		{
			name:  "database_file_created_on_disk",
			check: func(t *testing.T) { checkDBFileExists(t, path) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

func checkOnlyProbeAndBookkeepingTables(t *testing.T, db *DB) {
	t.Helper()
	rows, err := db.sql.QueryContext(context.Background(),
		"SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sqlite_master: %v", err)
	}
	want := []string{"probe", "schema_migrations"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("tables = %v, want %v", names, want)
	}
}

func checkProbeColumns(t *testing.T, db *DB) {
	t.Helper()
	rows, err := db.sql.QueryContext(context.Background(), "PRAGMA table_info(probe)")
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		cols = append(cols, name+" "+ctype)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info: %v", err)
	}
	want := []string{"id INTEGER", "n INTEGER"}
	if strings.Join(cols, "|") != strings.Join(want, "|") {
		t.Fatalf("probe columns = %q, want %q", cols, want)
	}
}

func checkCounterStartsAtZero(t *testing.T, db *DB) {
	t.Helper()
	n, err := db.ProbeCount(context.Background())
	if err != nil {
		t.Fatalf("ProbeCount: %v", err)
	}
	if n != 0 {
		t.Fatalf("ProbeCount = %d, want 0 on a fresh database", n)
	}
}

func checkProbeRowSeeded(t *testing.T, db *DB) {
	t.Helper()
	var id int64
	err := db.sql.QueryRowContext(context.Background(), "SELECT id FROM probe").Scan(&id)
	if err != nil {
		t.Fatalf("select probe id: %v", err)
	}
	if id != 1 {
		t.Fatalf("probe row id = %d, want 1", id)
	}
}

func checkRecordedVersion(t *testing.T, db *DB) {
	t.Helper()
	var version string
	err := db.sql.QueryRowContext(context.Background(),
		"SELECT version FROM schema_migrations").Scan(&version)
	if err != nil {
		t.Fatalf("select schema_migrations: %v", err)
	}
	if version != "0001_init" {
		t.Fatalf("recorded version = %q, want 0001_init", version)
	}
}

func checkRecordedChecksum(t *testing.T, db *DB) {
	t.Helper()
	var sum string
	err := db.sql.QueryRowContext(context.Background(),
		"SELECT sha256 FROM schema_migrations WHERE version = '0001_init'").Scan(&sum)
	if err != nil {
		t.Fatalf("select sha256: %v", err)
	}
	if !reHexSum.MatchString(sum) {
		t.Fatalf("recorded sha256 = %q, want 64-char lowercase hex", sum)
	}
	if want := embeddedSum(t, "0001_init"); sum != want {
		t.Fatalf("recorded sha256 = %s, want sha256 of embedded file %s", sum, want)
	}
}

func checkDBFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database file %s missing after Open: %v", path, err)
	}
}

func TestMigrationChecksumGuard(t *testing.T) {
	tests := []struct {
		name       string
		tamperWith string
	}{
		{
			name:       "replaced_hex_refuses_open",
			tamperWith: strings.Repeat("ab", 32),
		},
		{
			name:       "uppercase_checksum_refuses_open",
			tamperWith: strings.ToUpper(embeddedSum(t, "0001_init")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "guard.db")

			db, err := Open(ctx, path)
			if err != nil {
				t.Fatalf("first Open: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatalf("first Close: %v", err)
			}

			// Tamper the recorded checksum with the raw driver (in-package
			// use of the driver the store just added).
			raw, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatalf("raw open: %v", err)
			}
			if _, err := raw.ExecContext(ctx,
				"UPDATE schema_migrations SET sha256 = ?", tt.tamperWith); err != nil {
				t.Fatalf("tamper update: %v", err)
			}
			if err := raw.Close(); err != nil {
				t.Fatalf("raw close: %v", err)
			}

			// Re-open must refuse, naming the migration file.
			_, err = Open(ctx, path)
			if err == nil {
				t.Fatal("Open on tampered checksum succeeded, want refusal")
			}
			if !errors.Is(err, ErrChecksumMismatch) {
				t.Fatalf("error chain misses ErrChecksumMismatch: %v", err)
			}
			if !strings.Contains(err.Error(), "0001_init.sql") {
				t.Fatalf("error does not name 0001_init.sql: %v", err)
			}
			if !strings.Contains(err.Error(), "checksum mismatch") {
				t.Fatalf("error does not say checksum mismatch: %v", err)
			}
			if !strings.Contains(err.Error(), "recorded") || !strings.Contains(err.Error(), "file") {
				t.Fatalf("error does not name both recorded and file checksums: %v", err)
			}
			t.Logf("refusal error: %v", err)
		})
	}
}

func TestMigrationsIdempotentOnReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "idem.db")

	db1, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if _, err := db1.ProbeInc(ctx); err != nil {
		t.Fatalf("ProbeInc: %v", err)
	}
	var sum, appliedAt string
	if err := db1.sql.QueryRowContext(ctx,
		"SELECT sha256, applied_at FROM schema_migrations WHERE version = '0001_init'",
	).Scan(&sum, &appliedAt); err != nil {
		t.Fatalf("select bookkeeping row: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	db2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen on applied database: %v (double-apply or validation failure?)", err)
	}
	defer db2.Close()

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "counter_survives_reopen_without_reapplying",
			check: func(t *testing.T) {
				n, err := db2.ProbeCount(ctx)
				if err != nil {
					t.Fatalf("ProbeCount after reopen: %v", err)
				}
				if n != 1 {
					t.Fatalf("ProbeCount after reopen = %d, want 1", n)
				}
			},
		},
		{
			name: "exactly_one_migration_row_after_reopen",
			check: func(t *testing.T) {
				var rows int
				if err := db2.sql.QueryRowContext(ctx,
					"SELECT COUNT(*) FROM schema_migrations").Scan(&rows); err != nil {
					t.Fatalf("count schema_migrations: %v", err)
				}
				if rows != 1 {
					t.Fatalf("schema_migrations rows = %d, want 1 (no double-apply)", rows)
				}
			},
		},
		{
			name: "recorded_row_is_untouched_by_reopen",
			check: func(t *testing.T) {
				var gotSum, gotAt string
				if err := db2.sql.QueryRowContext(ctx,
					"SELECT sha256, applied_at FROM schema_migrations WHERE version = '0001_init'",
				).Scan(&gotSum, &gotAt); err != nil {
					t.Fatalf("select bookkeeping row: %v", err)
				}
				if gotSum != sum {
					t.Fatalf("sha256 changed on reopen: %s -> %s", sum, gotSum)
				}
				if gotAt != appliedAt {
					t.Fatalf("applied_at changed on reopen: %s -> %s", appliedAt, gotAt)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t)
		})
	}
}
