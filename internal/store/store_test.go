package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestWALSettingsAssertedViaPragmas(t *testing.T) {
	db, path := openFresh(t)
	ctx := context.Background()

	tests := []struct {
		name   string
		pragma string
		want   string
	}{
		{
			name:   "journal_mode_is_wal",
			pragma: "PRAGMA journal_mode",
			want:   "wal",
		},
		{
			name:   "foreign_keys_enforced",
			pragma: "PRAGMA foreign_keys",
			want:   "1",
		},
		{
			name:   "busy_timeout_is_5000_milliseconds",
			pragma: "PRAGMA busy_timeout",
			want:   "5000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			if err := db.sql.QueryRowContext(ctx, tt.pragma).Scan(&got); err != nil {
				t.Fatalf("%s: %v", tt.pragma, err)
			}
			if got != tt.want {
				t.Fatalf("%s = %q, want %q", tt.pragma, got, tt.want)
			}
		})
	}

	t.Run("wal_sidecar_file_exists_while_open", func(t *testing.T) {
		if _, err := db.ProbeInc(ctx); err != nil {
			t.Fatalf("ProbeInc: %v", err)
		}
		if _, err := os.Stat(path + "-wal"); err != nil {
			t.Fatalf("WAL sidecar %s missing: %v", path+"-wal", err)
		}
	})
}

func TestProbeIncReturnsSequentialValues(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		incs     int
		wantLast int64
	}{
		{
			name:     "first_inc_returns_1",
			incs:     1,
			wantLast: 1,
		},
		{
			name:     "third_inc_returns_3",
			incs:     3,
			wantLast: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openFresh(t)
			var last int64
			var err error
			for i := 0; i < tt.incs; i++ {
				last, err = db.ProbeInc(ctx)
				if err != nil {
					t.Fatalf("ProbeInc #%d: %v", i+1, err)
				}
			}
			if last != tt.wantLast {
				t.Fatalf("ProbeInc returned %d after %d incs, want %d", last, tt.incs, tt.wantLast)
			}
			n, err := db.ProbeCount(ctx)
			if err != nil {
				t.Fatalf("ProbeCount: %v", err)
			}
			if n != tt.wantLast {
				t.Fatalf("ProbeCount = %d, want %d", n, tt.wantLast)
			}
		})
	}
}

func TestProbeIncAtomicAndPersistent(t *testing.T) {
	t.Run("count_persists_across_close_and_reopen", probeCountPersistsAcrossCloseAndReopen)
	t.Run("concurrent_incs_yield_distinct_values_and_exact_total", probeConcurrentIncsAreAtomic)
}

func probeCountPersistsAcrossCloseAndReopen(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "persist.db")

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := db.ProbeInc(ctx); err != nil {
			t.Fatalf("ProbeInc #%d: %v", i+1, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()

	n, err := db2.ProbeCount(ctx)
	if err != nil {
		t.Fatalf("ProbeCount after reopen: %v", err)
	}
	if n != 3 {
		t.Fatalf("ProbeCount after close+reopen = %d, want 3", n)
	}
}

func probeConcurrentIncsAreAtomic(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	db, _ := openFresh(t)

	const workers = 10
	values := make(chan int64, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			n, err := db.ProbeInc(ctx)
			if err != nil {
				errs <- err
				return
			}
			values <- n
		}()
	}

	got := make([]int64, 0, workers)
	for i := 0; i < workers; i++ {
		select {
		case err := <-errs:
			t.Fatalf("concurrent ProbeInc: %v", err)
		case v := <-values:
			got = append(got, v)
		}
	}
	sort.Slice(got, func(a, b int) bool { return got[a] < got[b] })
	for i, v := range got {
		if v != int64(i+1) {
			t.Fatalf("concurrent incs returned %v; values must be exactly 1..%d (lost update at position %d)",
				got, workers, i)
		}
	}

	n, err := db.ProbeCount(ctx)
	if err != nil {
		t.Fatalf("ProbeCount: %v", err)
	}
	if n != workers {
		t.Fatalf("ProbeCount = %d after %d concurrent incs, want %d", n, workers, workers)
	}
}

func TestProbeCountMissingRowWrapsErrNotFound(t *testing.T) {
	ctx := context.Background()
	db, _ := openFresh(t)

	if _, err := db.sql.ExecContext(ctx, "DELETE FROM probe"); err != nil {
		t.Fatalf("delete probe row: %v", err)
	}

	if _, err := db.ProbeCount(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ProbeCount with no row: err = %v, want ErrNotFound", err)
	}
	if _, err := db.ProbeInc(ctx); err == nil {
		t.Fatal("ProbeInc with no row succeeded, want an error (the seeded row is required)")
	}
}

func TestOpenRefusesUnusablePath(t *testing.T) {
	tests := []struct {
		name    string
		dbPath  string
		wantErr string // substring the refusal must contain
	}{
		{
			name:    "missing_parent_directory_fails_open",
			dbPath:  filepath.Join(t.TempDir(), "no-such-subdir", "app.db"),
			wantErr: "no-such-subdir",
		},
		{
			name:    "query_separator_in_path_is_refused",
			dbPath:  filepath.Join(t.TempDir(), "odd?name.db"),
			wantErr: "?",
		},
		{
			name:    "fragment_separator_in_path_is_refused",
			dbPath:  filepath.Join(t.TempDir(), "odd#name.db"),
			wantErr: "#",
		},
		{
			name:    "percent_escape_in_path_is_refused",
			dbPath:  filepath.Join(t.TempDir(), "odd%20name.db"),
			wantErr: "%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := Open(context.Background(), tt.dbPath)
			if err == nil {
				_ = db.Close()
				t.Fatalf("Open(%q) succeeded, want an error", tt.dbPath)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error for %q does not mention %q: %v", tt.dbPath, tt.wantErr, err)
			}
			if !strings.Contains(err.Error(), tt.dbPath) {
				t.Fatalf("error does not name the path %q: %v", tt.dbPath, err)
			}
		})
	}

	t.Run("dsn_rejection_wraps_typed_error", func(t *testing.T) {
		_, err := Open(context.Background(), filepath.Join(t.TempDir(), "a#b.db"))
		if err == nil {
			t.Fatal("Open succeeded for a path containing '#', want refusal")
		}
		if !errors.Is(err, errBadDSNPath) {
			t.Fatalf("error chain misses errBadDSNPath: %v", err)
		}
	})
}
