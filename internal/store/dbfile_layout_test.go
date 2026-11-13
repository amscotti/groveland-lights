package store

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestChecksumIsFirstAndOnlyHexTokenInDBFile pins the on-disk layout property
// that the A-104 acceptance draft depends on: after a clean open, write and
// close, the recorded checksum is the FIRST 64-char lowercase-hex token in the
// raw database file, and at this schema size it is the ONLY one. The draft
// byte-tampers the first such token and expects the next boot to refuse with a
// checksum mismatch; this test proves that assumption at the store level,
// without spawning the server.
//
// If a future migration stores earlier hex-looking data (e.g. UUID columns)
// before the schema_migrations page, the first-token property can legitimately
// change: update that migration's task consciously, and T-105's sanctioned
// index pin is the fallback. This test failing is the canary for exactly that.
func TestChecksumIsFirstAndOnlyHexTokenInDBFile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "layout.db")

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

	// A clean Close checkpoints and removes the -wal sidecar, leaving the
	// checksum text in the main database file.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read db file: %v", err)
	}
	re := regexp.MustCompile(`[0-9a-f]{64}`)
	locs := re.FindAllIndex(data, -1)
	if len(locs) == 0 {
		t.Fatalf("no 64-char lowercase-hex token in the %d-byte database file", len(data))
	}
	first := string(data[locs[0][0]:locs[0][1]])
	if first != recorded {
		t.Fatalf("first hex token at offset %d = %s, want the recorded checksum %s",
			locs[0][0], first, recorded)
	}
	if len(locs) != 1 {
		t.Fatalf("found %d hex-64 tokens in the db file, want exactly 1 (the recorded checksum): first at offset %d, next at offset %d (%s); the A-104 first-token oracle is ambiguous",
			len(locs), locs[0][0], locs[1][0], string(data[locs[1][0]:locs[1][1]]))
	}
}
