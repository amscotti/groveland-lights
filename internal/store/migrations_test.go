package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/amscotti/groveland-lights/migrations"
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

// independentSum computes sha256 over the embedded file bytes WITHOUT going
// through loadMigrations, so the recorded checksum is checked against the
// digest the contract names (sha256 of file bytes), not merely against the
// runner's own derivation.
func independentSum(t *testing.T, file string) string {
	t.Helper()
	b, err := fs.ReadFile(migrations.Files, file)
	if err != nil {
		t.Fatalf("read embedded %s: %v", file, err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
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
	type col struct {
		name    string
		ctype   string
		notNull int
		dflt    sql.NullString
		pk      int
	}
	var cols []col
	for rows.Next() {
		var cid int
		var c col
		if err := rows.Scan(&cid, &c.name, &c.ctype, &c.notNull, &c.dflt, &c.pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info: %v", err)
	}
	if len(cols) != 2 || cols[0].name != "id" || cols[1].name != "n" {
		t.Fatalf("probe columns = %+v, want [id n]", cols)
	}
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "id_type_is_integer", got: cols[0].name + " " + cols[0].ctype, want: "id INTEGER"},
		{name: "id_is_primary_key", got: fmt.Sprint(cols[0].pk), want: "1"},
		{name: "n_type_is_integer", got: cols[1].name + " " + cols[1].ctype, want: "n INTEGER"},
		{name: "n_is_not_null", got: fmt.Sprint(cols[1].notNull), want: "1"},
		// table_info reports DEFAULT 0 as the literal text "0".
		{name: "n_defaults_to_zero", got: cols[1].dflt.String, want: "0"},
		{name: "n_is_not_primary_key", got: fmt.Sprint(cols[1].pk), want: "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}

	t.Run("check_constraint_pins_id_to_1", func(t *testing.T) {
		// The CHECK constraint lives in the stored DDL, not in table_info.
		var ddl string
		if err := db.sql.QueryRowContext(context.Background(),
			"SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'probe'").Scan(&ddl); err != nil {
			t.Fatalf("select probe DDL: %v", err)
		}
		for _, want := range []string{"CHECK (id = 1)", "NOT NULL", "DEFAULT 0"} {
			if !strings.Contains(ddl, want) {
				t.Fatalf("probe DDL %q does not contain %q", ddl, want)
			}
		}
	})
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
	if want := independentSum(t, "0001_init.sql"); sum != want {
		t.Fatalf("recorded sha256 = %s, want independent sha256 of embedded file bytes %s", sum, want)
	}
	// The runner's derivation must agree with the independent digest.
	if want := embeddedSum(t, "0001_init"); sum != want {
		t.Fatalf("recorded sha256 = %s, want runner-derived sha256 %s", sum, want)
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
			// The message must carry the actual values, not just the labels:
			// the tampered recorded sum and the embedded file sum.
			if !strings.Contains(err.Error(), tt.tamperWith) {
				t.Fatalf("error does not name the recorded (tampered) checksum %s: %v", tt.tamperWith, err)
			}
			fileSum := embeddedSum(t, "0001_init")
			if !strings.Contains(err.Error(), fileSum) {
				t.Fatalf("error does not name the embedded file checksum %s: %v", fileSum, err)
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

// TestRecordedVersionWithoutEmbeddedFileRefusesOpen exercises the branch where
// schema_migrations names a version that no embedded file provides: the
// database remembers a migration the binary has never heard of, so boot must
// refuse. This is the deployment-drift half of the guard (A-104).
func TestRecordedVersionWithoutEmbeddedFileRefusesOpen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ghost.db")

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	// Record a version no embedded file defines, via the raw driver.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := raw.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, sha256, applied_at) VALUES (?, ?, ?)",
		"0002_ghost", strings.Repeat("cd", 32),
		"2000-01-01T00:00:00Z"); err != nil {
		t.Fatalf("insert ghost row: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	_, err = Open(ctx, path)
	if err == nil {
		t.Fatal("Open with a recorded version absent from the embedded files succeeded, want refusal")
	}
	if !strings.Contains(err.Error(), "0002_ghost") {
		t.Fatalf("error does not name the ghost version 0002_ghost: %v", err)
	}
	if errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("ghost version must not be reported as a checksum mismatch: %v", err)
	}
	t.Logf("refusal error: %v", err)
}

// TestApplyMigrationsAppliesUnrecordedFilesInOrder drives applyMigrations
// directly over a synthetic migration set (fstest.MapFS) to prove that
// unrecorded files apply in filename order: the second file may reference
// objects created by the first, which fails if the order is violated.
func TestApplyMigrationsAppliesUnrecordedFilesInOrder(t *testing.T) {
	ctx := context.Background()
	fsys := fstest.MapFS{
		"0001_alpha.sql": &fstest.MapFile{Data: []byte("CREATE TABLE alpha (v TEXT NOT NULL);")},
		"0002_beta.sql":  &fstest.MapFile{Data: []byte("INSERT INTO alpha (v) VALUES ('from-beta');")},
	}

	raw, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "order.db"))
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	defer raw.Close()

	if err := applyMigrations(ctx, raw, fsys); err != nil {
		t.Fatalf("applyMigrations: %v", err)
	}
	fx := orderFixture{t: t, ctx: ctx, raw: raw, fsys: fsys}

	tests := []struct {
		name  string
		check func(orderFixture)
	}{
		{name: "second_file_applied_after_first", check: checkSecondFileAppliedAfterFirst},
		{name: "both_versions_recorded_in_order", check: checkBothVersionsRecordedInOrder},
		{name: "recorded_checksums_are_sha256_of_synthetic_file_bytes", check: checkSyntheticFileChecksums},
		{name: "reapply_of_same_set_is_a_no_op", check: checkReapplyIsNoOp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx.t = t
			tt.check(fx)
		})
	}
}

// orderFixture carries the synthetic migration set and its database to the
// named sub-checks (keeping each function under the cyclop budget).
type orderFixture struct {
	t    *testing.T
	ctx  context.Context
	raw  *sql.DB
	fsys fstest.MapFS
}

func checkSecondFileAppliedAfterFirst(fx orderFixture) {
	fx.t.Helper()
	var v string
	if err := fx.raw.QueryRowContext(fx.ctx, "SELECT v FROM alpha").Scan(&v); err != nil {
		fx.t.Fatalf("select from alpha (0002 depends on 0001's table): %v", err)
	}
	if v != "from-beta" {
		fx.t.Fatalf("alpha.v = %q, want from-beta", v)
	}
}

func checkBothVersionsRecordedInOrder(fx orderFixture) {
	fx.t.Helper()
	rows, err := fx.raw.QueryContext(fx.ctx,
		"SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		fx.t.Fatalf("select versions: %v", err)
	}
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			fx.t.Fatalf("scan version: %v", err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		fx.t.Fatalf("iterate versions: %v", err)
	}
	want := []string{"0001_alpha", "0002_beta"}
	if strings.Join(versions, ",") != strings.Join(want, ",") {
		fx.t.Fatalf("recorded versions = %v, want %v", versions, want)
	}
}

func checkSyntheticFileChecksums(fx orderFixture) {
	fx.t.Helper()
	for _, f := range []string{"0001_alpha.sql", "0002_beta.sql"} {
		b, err := fs.ReadFile(fx.fsys, f)
		if err != nil {
			fx.t.Fatalf("read %s from MapFS: %v", f, err)
		}
		sum := sha256.Sum256(b)
		var recorded string
		if err := fx.raw.QueryRowContext(fx.ctx,
			"SELECT sha256 FROM schema_migrations WHERE version = ?",
			nameWithoutExt(f)).Scan(&recorded); err != nil {
			fx.t.Fatalf("select recorded sha256 for %s: %v", f, err)
		}
		if want := hex.EncodeToString(sum[:]); recorded != want {
			fx.t.Fatalf("recorded sha256 for %s = %s, want %s", f, recorded, want)
		}
	}
}

func checkReapplyIsNoOp(fx orderFixture) {
	fx.t.Helper()
	if err := applyMigrations(fx.ctx, fx.raw, fx.fsys); err != nil {
		fx.t.Fatalf("re-apply: %v", err)
	}
	var n int
	if err := fx.raw.QueryRowContext(fx.ctx,
		"SELECT COUNT(*) FROM schema_migrations").Scan(&n); err != nil {
		fx.t.Fatalf("count rows: %v", err)
	}
	if n != 2 {
		fx.t.Fatalf("schema_migrations rows = %d after re-apply, want 2", n)
	}
}
