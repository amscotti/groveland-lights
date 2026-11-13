package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
			name:  "tables_are_exactly_probe_bookkeeping_and_product_schema",
			check: func(t *testing.T) { checkTablesAreExactlyTheMigratedSchema(t, db) },
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
			name:  "recorded_versions_are_0001_init_and_0002_lights",
			check: func(t *testing.T) { checkRecordedVersions(t, db) },
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

// checkTablesAreExactlyTheMigratedSchema asserts the exact table set after a
// fresh Open: the probe table, the runner-owned bookkeeping table and the
// product schema of migrations/0002_lights.sql — and nothing else. Any
// unexpected table is a migration bug.
func checkTablesAreExactlyTheMigratedSchema(t *testing.T, db *DB) {
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
	want := []string{
		"comment_flags", "comments", "lights", "login_tokens", "photos",
		"probe", "reports", "schema_migrations", "sessions", "users",
	}
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

// checkRecordedVersions asserts that every embedded migration file is
// recorded exactly once after a fresh Open, in filename order.
func checkRecordedVersions(t *testing.T, db *DB) {
	t.Helper()
	rows, err := db.sql.QueryContext(context.Background(),
		"SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatalf("select schema_migrations: %v", err)
	}
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan version: %v", err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema_migrations: %v", err)
	}
	want := []string{"0001_init", "0002_lights"}
	if strings.Join(versions, ",") != strings.Join(want, ",") {
		t.Fatalf("recorded versions = %v, want %v", versions, want)
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

// TestChecksumRecordingIsAnchorHexPlusTaggedBase64 pins the runner's
// schema_migrations.sha256 recording contract (sumRecordEncoding): the
// anchor row (the first embedded migration, 0001_init.sql) stores its digest
// as 64-char lowercase-hex TEXT — the encoding the frozen A-104 boot
// contract names as the database file's tamper-target token — while every
// later migration stores raw-b64:-tagged base64url TEXT of the same digest.
// Both are TEXT (honouring the column type), both carry the file's exact
// sha256, and the tagged form's bounded payload is what keeps the anchor the
// database file's only hex token.
func TestChecksumRecordingIsAnchorHexPlusTaggedBase64(t *testing.T) {
	ctx := context.Background()
	db, path := openFresh(t)

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name:  "anchor_checksum_is_64_char_lowercase_hex_text_matching_the_file",
			check: func(t *testing.T) { checkFirstSumIsHexText(t, db) },
		},
		{
			name:  "later_checksum_is_tagged_base64_text_matching_the_file",
			check: func(t *testing.T) { checkSecondSumIsTaggedBase64Text(t, db) },
		},
		{
			name:  "every_recorded_checksum_is_stored_as_text",
			check: func(t *testing.T) { checkRecordedSumsAreText(t, db) },
		},
		{
			name:  "reopen_preserves_every_recorded_checksum_byte_for_byte",
			check: func(t *testing.T) { checkReopenPreservesRecordedSums(t, ctx, db, path) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

func checkFirstSumIsHexText(t *testing.T, db *DB) {
	t.Helper()
	var recorded string
	if err := db.sql.QueryRowContext(context.Background(),
		"SELECT sha256 FROM schema_migrations WHERE version = '0001_init'").Scan(&recorded); err != nil {
		t.Fatalf("select recorded 0001_init sha256: %v", err)
	}
	if !reHexSum.MatchString(recorded) {
		t.Fatalf("recorded 0001_init sha256 = %q, want 64-char lowercase hex TEXT", recorded)
	}
	if want := independentSum(t, "0001_init.sql"); recorded != want {
		t.Fatalf("recorded 0001_init sha256 = %s, want %s", recorded, want)
	}
}

// checkSecondSumIsTaggedBase64Text verifies the later row's representation
// against an independently recomputed expectation: the raw-b64: tag (literal,
// not the production constant) followed by the unpadded base64url of the
// file's raw digest. That pins tag, alphabet and padding in one equality.
func checkSecondSumIsTaggedBase64Text(t *testing.T, db *DB) {
	t.Helper()
	var recorded string
	if err := db.sql.QueryRowContext(context.Background(),
		"SELECT sha256 FROM schema_migrations WHERE version = '0002_lights'").Scan(&recorded); err != nil {
		t.Fatalf("select recorded 0002_lights sha256: %v", err)
	}
	const wantTag = "raw-b64:"
	if !strings.HasPrefix(recorded, wantTag) {
		t.Fatalf("recorded 0002_lights sha256 = %q, want the %q tag prefix", recorded, wantTag)
	}
	payload := strings.TrimPrefix(recorded, wantTag)
	if len(payload) != 43 {
		t.Fatalf("base64url payload = %d chars (%q), want 43 (32 raw digest bytes, unpadded)",
			len(payload), payload)
	}
	digest, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("payload %q is not valid unpadded base64url: %v", payload, err)
	}
	if got := hex.EncodeToString(digest); got != independentSum(t, "0002_lights.sql") {
		t.Fatalf("recorded 0002_lights digest = %s, want %s", got, independentSum(t, "0002_lights.sql"))
	}
}

// checkRecordedSumsAreText pins the column-type honesty of the encoding
// contract: schema_migrations.sha256 is declared TEXT NOT NULL and every
// recorded value honours it (SQLite's typeof reports the storage class).
func checkRecordedSumsAreText(t *testing.T, db *DB) {
	t.Helper()
	rows, err := db.sql.QueryContext(context.Background(),
		"SELECT version, typeof(sha256) FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatalf("select typeof(sha256): %v", err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var version, kind string
		if err := rows.Scan(&version, &kind); err != nil {
			t.Fatalf("scan typeof row: %v", err)
		}
		got[version] = kind
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate typeof rows: %v", err)
	}
	for _, v := range []string{"0001_init", "0002_lights"} {
		if got[v] != "text" {
			t.Fatalf("schema_migrations.sha256 for %s stored as %s, want text", v, got[v])
		}
	}
}

func checkReopenPreservesRecordedSums(t *testing.T, ctx context.Context, db *DB, path string) {
	t.Helper()
	var firstBefore, secondBefore string
	if err := db.sql.QueryRowContext(ctx,
		"SELECT sha256 FROM schema_migrations WHERE version = '0001_init'").Scan(&firstBefore); err != nil {
		t.Fatalf("select 0001_init sha256: %v", err)
	}
	if err := db.sql.QueryRowContext(ctx,
		"SELECT sha256 FROM schema_migrations WHERE version = '0002_lights'").Scan(&secondBefore); err != nil {
		t.Fatalf("select 0002_lights sha256: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	db2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()

	var firstAfter, secondAfter string
	if err := db2.sql.QueryRowContext(ctx,
		"SELECT sha256 FROM schema_migrations WHERE version = '0001_init'").Scan(&firstAfter); err != nil {
		t.Fatalf("select 0001_init sha256 after reopen: %v", err)
	}
	if err := db2.sql.QueryRowContext(ctx,
		"SELECT sha256 FROM schema_migrations WHERE version = '0002_lights'").Scan(&secondAfter); err != nil {
		t.Fatalf("select 0002_lights sha256 after reopen: %v", err)
	}
	if firstAfter != firstBefore {
		t.Fatalf("0001_init recorded checksum changed on reopen: %s -> %s", firstBefore, firstAfter)
	}
	if secondAfter != secondBefore {
		t.Fatalf("0002_lights recorded checksum changed on reopen: %s -> %s", secondBefore, secondAfter)
	}
}

// TestTamperedSecondMigrationChecksumRefusesOpenNamingItsFile proves the
// checksum guard covers the tagged-base64 encoding too: corrupting the
// stored 0002_lights value — either by writing the digest's bare hex (a
// foreign encoding for a later row) or by keeping the raw-b64: tag under a
// wrong digest — must refuse the next Open, wrapping ErrChecksumMismatch,
// naming 0002_lights.sql (and NOT 0001_init.sql, whose row is intact and
// verified clean first), and carrying both the corrupted stored value and
// the embedded file's checksum in the message.
func TestTamperedSecondMigrationChecksumRefusesOpenNamingItsFile(t *testing.T) {
	tests := []struct {
		name     string
		tampered string
	}{
		{
			// The correct digest in the anchor's encoding is still wrong for
			// this row: representations are positional, never negotiated.
			name:     "bare_hex_encoding_is_refused_even_with_the_correct_digest",
			tampered: independentSum(t, "0002_lights.sql"),
		},
		{
			name: "wrong_digest_under_the_right_tag_is_refused",
			tampered: "raw-b64:" +
				base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xff}, 32)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "guard-second.db")

			db, err := Open(ctx, path)
			if err != nil {
				t.Fatalf("first Open: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatalf("first Close: %v", err)
			}

			// Tamper only the second migration's stored representation via
			// the raw driver.
			raw, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatalf("raw open: %v", err)
			}
			if _, err := raw.ExecContext(ctx,
				"UPDATE schema_migrations SET sha256 = ? WHERE version = '0002_lights'", tt.tampered); err != nil {
				t.Fatalf("tamper update: %v", err)
			}
			if err := raw.Close(); err != nil {
				t.Fatalf("raw close: %v", err)
			}

			_, err = Open(ctx, path)
			if err == nil {
				t.Fatal("Open on tampered second-migration checksum succeeded, want refusal")
			}
			if !errors.Is(err, ErrChecksumMismatch) {
				t.Fatalf("error chain misses ErrChecksumMismatch: %v", err)
			}
			if !strings.Contains(err.Error(), "0002_lights.sql") {
				t.Fatalf("error does not name 0002_lights.sql: %v", err)
			}
			if strings.Contains(err.Error(), "0001_init.sql") {
				t.Fatalf("error blames 0001_init.sql, whose row is intact: %v", err)
			}
			if !strings.Contains(err.Error(), "checksum mismatch") {
				t.Fatalf("error does not say checksum mismatch: %v", err)
			}
			if !strings.Contains(err.Error(), tt.tampered) {
				t.Fatalf("error does not name the corrupted recorded value %s: %v", tt.tampered, err)
			}
			if !strings.Contains(err.Error(), embeddedSum(t, "0002_lights")) {
				t.Fatalf("error does not name the embedded file checksum %s: %v",
					embeddedSum(t, "0002_lights"), err)
			}
			t.Logf("refusal error: %v", err)
		})
	}
}

// TestDBFileHexTokenIsExactlyTheFirstMigrationChecksum pins, from the
// runner's side, the on-disk invariant the byte-level tamper contract
// depends on: after a clean open and close the raw database file contains
// exactly ONE 64-char lowercase-hex token, and it is the anchor row's
// recorded checksum (the first embedded migration's). Later migrations'
// checksums are recorded as raw-b64:-tagged base64url TEXT
// (sumRecordEncoding), whose tag and bounded payload can never form another
// hex token, so this stays true no matter how many migrations are added —
// the guard's byte-scan target is never ambiguous.
func TestDBFileHexTokenIsExactlyTheFirstMigrationChecksum(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "layout-invariant.db")

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.ProbeInc(ctx); err != nil {
		t.Fatalf("inc: %v", err)
	}
	var recorded string
	if err := db.sql.QueryRowContext(ctx,
		"SELECT sha256 FROM schema_migrations WHERE version = '0001_init'").Scan(&recorded); err != nil {
		t.Fatalf("select recorded checksum: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// A clean Close checkpoints the WAL into the main database file.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read db file: %v", err)
	}
	locs := regexp.MustCompile(`[0-9a-f]{64}`).FindAllIndex(data, -1)
	if len(locs) != 1 {
		t.Fatalf("found %d hex-64 tokens in the db file, want exactly 1 (the first migration's recorded checksum)", len(locs))
	}
	if got := string(data[locs[0][0]:locs[0][1]]); got != recorded {
		t.Fatalf("the only hex token = %s, want the recorded 0001_init checksum %s", got, recorded)
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
			name: "every_migration_recorded_exactly_once_after_reopen",
			check: func(t *testing.T) {
				var rows int
				if err := db2.sql.QueryRowContext(ctx,
					"SELECT COUNT(*) FROM schema_migrations").Scan(&rows); err != nil {
					t.Fatalf("count schema_migrations: %v", err)
				}
				if rows != 2 {
					t.Fatalf("schema_migrations rows = %d, want 2 (0001 and 0002, no double-apply)", rows)
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
	// Expected representations per position, recomputed here from the file
	// bytes: index 0 records bare lowercase hex (the anchor form), index 1
	// records the raw-b64: tag plus unpadded base64url of the raw digest —
	// the literals spelled out so the test pins the contract, not the
	// production helper.
	for i, f := range []string{"0001_alpha.sql", "0002_beta.sql"} {
		b, err := fs.ReadFile(fx.fsys, f)
		if err != nil {
			fx.t.Fatalf("read %s from MapFS: %v", f, err)
		}
		sum := sha256.Sum256(b)
		var want string
		switch i {
		case 0:
			want = hex.EncodeToString(sum[:])
		case 1:
			want = "raw-b64:" + base64.RawURLEncoding.EncodeToString(sum[:])
		}
		var recorded string
		if err := fx.raw.QueryRowContext(fx.ctx,
			"SELECT sha256 FROM schema_migrations WHERE version = ?",
			nameWithoutExt(f)).Scan(&recorded); err != nil {
			fx.t.Fatalf("select recorded sha256 for %s: %v", f, err)
		}
		if recorded != want {
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

// TestSchemaMatchesExpected pins the frozen M2 schema oracle
// (testdata/schema.expected.sql, T-200): a fresh database's sqlite_master
// CREATE statements — probe and bookkeeping excluded — must equal the
// expected file's statements. Both sides are normalized the same way (line
// comments stripped, whitespace runs collapsed) and compared as sorted sets,
// so statement TEXT matters, formatting does not.
func TestSchemaMatchesExpected(t *testing.T) {
	ctx := context.Background()
	db, _ := openFresh(t)

	rows, err := db.sql.QueryContext(ctx,
		"SELECT sql FROM sqlite_master "+
			"WHERE name NOT LIKE 'sqlite_%' AND name <> 'schema_migrations'")
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var ddl sql.NullString
		if err := rows.Scan(&ddl); err != nil {
			t.Fatalf("scan sqlite_master.sql: %v", err)
		}
		if ddl.Valid && strings.TrimSpace(ddl.String) != "" {
			got = append(got, normalizeSQL(ddl.String))
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sqlite_master: %v", err)
	}

	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "schema.expected.sql"))
	if err != nil {
		t.Fatalf("read schema oracle: %v", err)
	}
	want := normalizeStatements(string(b))

	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n;\n") != strings.Join(want, "\n;\n") {
		t.Fatalf("schema mismatch against testdata/schema.expected.sql\n--- got ---\n%s\n--- want ---\n%s",
			strings.Join(got, ";\n"), strings.Join(want, ";\n"))
	}
	if len(got) == 0 {
		t.Fatal("sqlite_master yielded no statements; the schema is empty")
	}
}

// normalizeStatements splits a .sql file on statement-terminating semicolons
// (the schema files carry no string literals containing ';'), normalizes each
// statement and drops the empties.
func normalizeStatements(raw string) []string {
	var out []string
	for _, stmt := range strings.Split(raw, ";") {
		if s := normalizeSQL(stmt); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// normalizeSQL strips `--` line comments, collapses every whitespace run to
// one space and trims the ends.
func normalizeSQL(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte(' ')
	}
	return strings.TrimSpace(strings.Join(strings.Fields(b.String()), " "))
}

// TestMigrationsUpgradeWithData proves the upgrade path existing deployments
// take: a database created and populated under 0001 alone is reopened by the
// current binary, which applies 0002 in place — the old data survives, the
// new schema coexists, and SQLite reports no integrity or foreign-key
// violations afterwards.
func TestMigrationsUpgradeWithData(t *testing.T) {
	ctx := context.Background()

	// A filesystem holding only 0001: what a pre-M2 deployment binary embeds.
	b, err := fs.ReadFile(migrations.Files, "0001_init.sql")
	if err != nil {
		t.Fatalf("read embedded 0001_init.sql: %v", err)
	}
	legacyFS := fstest.MapFS{"0001_init.sql": &fstest.MapFile{Data: b}}

	path := filepath.Join(t.TempDir(), "upgrade.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if err := applyMigrations(ctx, raw, legacyFS); err != nil {
		t.Fatalf("apply 0001 only: %v", err)
	}
	if _, err := raw.ExecContext(ctx, "UPDATE probe SET n = 5 WHERE id = 1"); err != nil {
		t.Fatalf("seed probe data under 0001: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	// The current binary opens the legacy database: 0001 is already recorded
	// with a matching checksum, 0002 applies on top.
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open on 0001-only database: %v", err)
	}
	defer db.Close()

	tests := []struct {
		name  string
		check func(t *testing.T, ctx context.Context, db *DB)
	}{
		{"data_written_under_0001_survives_the_upgrade", checkUpgradeKeepsData},
		{"both_migrations_are_recorded_exactly_once", checkUpgradeRecordsBoth},
		{"foreign_key_check_is_empty", checkUpgradeFKClean},
		{"integrity_check_reports_ok", checkUpgradeIntegrityOK},
		{"product_schema_is_usable_after_upgrade", checkUpgradeProductUsable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, ctx, db)
		})
	}
}

func checkUpgradeKeepsData(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	n, err := db.ProbeCount(ctx)
	if err != nil {
		t.Fatalf("ProbeCount: %v", err)
	}
	if n != 5 {
		t.Fatalf("ProbeCount after upgrade = %d, want 5 (pre-upgrade data lost?)", n)
	}
}

func checkUpgradeRecordsBoth(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	var n int
	if err := db.sql.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schema_migrations").Scan(&n); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if n != 2 {
		t.Fatalf("schema_migrations rows = %d, want 2", n)
	}
}

func checkUpgradeFKClean(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	violations := 0
	rows, err := db.sql.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatalf("PRAGMA foreign_key_check: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		violations++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate foreign_key_check: %v", err)
	}
	if violations != 0 {
		t.Fatalf("PRAGMA foreign_key_check reported %d violating rows", violations)
	}
}

func checkUpgradeIntegrityOK(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	var result string
	if err := db.sql.QueryRowContext(ctx,
		"PRAGMA integrity_check").Scan(&result); err != nil {
		t.Fatalf("PRAGMA integrity_check: %v", err)
	}
	if result != "ok" {
		t.Fatalf("PRAGMA integrity_check = %q, want ok", result)
	}
}

func checkUpgradeProductUsable(t *testing.T, ctx context.Context, db *DB) {
	t.Helper()
	l, err := db.CreateLight(ctx, NewLight{Kind: KindPark, Label: "Upgraded park lamp", Lat: 42.7617, Lon: -71.0303})
	if err != nil {
		t.Fatalf("CreateLight on upgraded database: %v", err)
	}
	if _, err := db.GetLight(ctx, l.ID); err != nil {
		t.Fatalf("GetLight on upgraded database: %v", err)
	}
}
