package store

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestChecksumIsFirstHexTokenInDBFile pins the on-disk layout property that
// the A-104 acceptance draft depends on: after a clean open, write and close,
// the recorded checksum is the FIRST 64-char lowercase-hex token in the raw
// database file (at this schema size it is also the only one). The draft
// byte-tampers the first such token and expects the next boot to refuse with
// a checksum mismatch; this test proves that assumption at the store level,
// without spawning the server.
//
// If a future migration stores earlier hex-looking data (e.g. UUID columns)
// before the schema_migrations page, the first-token property can legitimately
// change: update that migration's task consciously, and T-105's sanctioned
// index pin is the fallback.
func TestChecksumIsFirstHexTokenInDBFile(t *testing.T) {
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
		t.Fatalf("first hex token = %s, want the recorded checksum %s", first, recorded)
	}
	t.Logf("%d hex-64 token(s); first at offset %d is the recorded checksum", len(locs), locs[0][0])
	if len(locs) > 1 {
		t.Logf("note: additional hex tokens present: %s",
			strings.Join([]string{string(data[locs[1][0]:locs[1][1]])}, ", "))
	}
}
