//go:build acceptance

// @A-104 — migration runner refuses to boot on a checksum mismatch (frozen
// contract, written by T-100 before the feature tasks branch).
//
// INERT DRAFT: the `.go.txt` extension means the Go toolchain never compiles
// this file. T-105 activates it verbatim, with a rename only:
//
//	git mv acceptance/drafts/migrations_go_test.go.txt acceptance/go/migrations_test.go
//
// NO content edits are allowed at activation, with exactly ONE sanctioned
// exception: if TestA104RefusesTamperedChecksum fails because the FIRST
// 64-char lowercase-hex token found in the DB file is not the migration
// checksum recorded in schema_migrations, T-105 may pin a different token
// index in migTamperFirstHex (the `0` in `locs[0]`). That is the ONLY edit
// allowed. Any such pin MUST be documented in harness/handoffs/T-105.md with
// the observed token offsets.
//
// Contract owners: T-101 (migration runner). Exact contract strings: the
// first migration file is migrations/0001_init.sql; its sha256 checksum is
// recorded in schema_migrations as 64-char lowercase hex; a mismatch found at
// startup must refuse to boot (non-zero exit) with output naming both
// "0001_init.sql" and the word "checksum".
//
// This draft FAILS against the current skeleton by design (no migrations:
// DB_PATH is never even created, so the tamper step cannot run).
//
// Driver-free by necessity: go.mod has no sqlite driver at T-100 and none may
// be added, so tampering is done by byte-editing the DB file, keeping the
// file length identical.
package acceptance

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// --- mig spawn helpers (self-contained: activation must be a rename only) ---

type migSyncBuf struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *migSyncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *migSyncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// migFindBin resolves the server binary: $BIN if set, else bin/server walked
// up from the test working directory to the repo root. Never skips: a missing
// binary is a failure (run `make build` first).
func migFindBin(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("BIN"); p != "" {
		abs, err := filepath.Abs(p)
		if err != nil {
			t.Fatalf("BIN=%q: %v", p, err)
		}
		if _, err := os.Stat(abs); err != nil {
			t.Fatalf("BIN=%q: %v", p, err)
		}
		return abs
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		cand := filepath.Join(dir, "bin", "server")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("server binary not found: run `make build` or set BIN")
	return "" // unreachable
}

// migFreePort picks a free TCP port by listening on 127.0.0.1:0 and closing.
func migFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen :0: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// migServer is one spawned instance; out captures combined stdout+stderr so
// both channels can be asserted for the refusal wording.
type migServer struct {
	cmd      *exec.Cmd
	baseURL  string
	dbPath   string
	photoDir string
	out      *migSyncBuf
	stopped  bool
}

// migStart launches the server on a free port with the given DB_PATH and
// PHOTO_DIR. It deliberately does NOT wait for health (the tamper test must
// observe the process refusing to boot) and does NOT register a cleanup: the
// tests own the lifecycle. APP_ENV=test; .env.test dummy secrets.
func migStart(t *testing.T, dbPath, photoDir string) *migServer {
	t.Helper()
	bin := migFindBin(t)
	port := migFreePort(t)
	out := &migSyncBuf{}
	cmd := exec.Command(bin)
	cmd.Dir = filepath.Dir(dbPath)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.Env = []string{
		"PORT=" + strconv.Itoa(port),
		"APP_ENV=test",
		"BASE_URL=http://127.0.0.1:" + strconv.Itoa(port),
		"DB_PATH=" + dbPath,
		"PHOTO_DIR=" + photoDir,
		"SESSION_KEY=dGVzdC1zZXNzaW9uLWtleS0zMi1ieXRlcy1hYQ==",
		"IP_HASH_KEY=dGVzdC1pcGhhc2gta2V5LTMyLWJ5dGVzLWJi",
		"ADMIN_TOKEN=dGVzdC1hZG1pbi10b2tlbi0zMi1ieXRl",
		"TOWN_BBOX=42.70,-71.09,42.82,-70.96",
		"TZ=America/New_York",
		"PATH=" + os.Getenv("PATH"),
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", bin, err)
	}
	return &migServer{
		cmd:      cmd,
		baseURL:  "http://127.0.0.1:" + strconv.Itoa(port),
		dbPath:   dbPath,
		photoDir: photoDir,
		out:      out,
	}
}

// migWaitHealthy polls /healthz until 200 with a context deadline; failure
// dumps the process output. A hung server fails the test, never the suite.
func migWaitHealthy(t *testing.T, s *migServer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := &http.Client{Timeout: time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/healthz", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case <-ctx.Done():
			migStop(t, s)
			t.Fatalf("server not healthy within 10s; output:\n%s", s.out.String())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// migStop shuts the process down gracefully (SIGTERM, bounded wait) so
// SQLite flushes and checkpoints before the DB file is tampered; falls back
// to SIGKILL after 10 s. Idempotent; a second Wait after an already-observed
// exit merely returns an error, which is ignored.
func migStop(t *testing.T, s *migServer) {
	t.Helper()
	if s.stopped {
		return
	}
	s.stopped = true
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	_ = s.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = s.cmd.Process.Kill()
		<-done
	}
}

// migTamperFirstHex rewrites the FIRST 64-char lowercase-hex run in data with
// a different 64-char hex value, in place, keeping the length identical.
// (Pinning a different index is T-105's only sanctioned edit — see header.)
func migTamperFirstHex(t *testing.T, data []byte) []byte {
	t.Helper()
	re := regexp.MustCompile(`[0-9a-f]{64}`)
	locs := re.FindIndex(data)
	if locs == nil {
		t.Fatalf("no 64-char lowercase-hex token found in DB file (%d bytes)", len(data))
	}
	replacement := []byte(strings.Repeat("deadbeef", 8))
	if bytes.Equal(data[locs[0]:locs[1]], replacement) {
		replacement = []byte(strings.Repeat("01234567", 8))
	}
	copy(data[locs[0]:locs[1]], replacement)
	return data
}

// --- the frozen A-104 Go assertions ---

// TestA104AppliesMigrations: on a fresh DB_PATH the server applies migrations
// at startup without error — it stays up, /healthz answers 200, and the DB
// file exists.
func TestA104AppliesMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	s := migStart(t, dbPath, filepath.Join(dir, "photos"))
	t.Cleanup(func() { migStop(t, s) })

	// Stays up + healthy: migrations ran at boot without failing.
	migWaitHealthy(t, s)

	// The database file was created at startup.
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("DB file %s not created at startup: %v", dbPath, err)
	}
}

// TestA104RefusesTamperedChecksum: after one healthy boot, rewriting one
// stored checksum token makes the next boot REFUSE: the process exits
// non-zero and its combined output names the migration file and the checksum
// mismatch.
func TestA104RefusesTamperedChecksum(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	photoDir := filepath.Join(dir, "photos")

	// First boot: healthy, migrations applied, then stop.
	s := migStart(t, dbPath, photoDir)
	t.Cleanup(func() { migStop(t, s) })
	migWaitHealthy(t, s)
	migStop(t, s)

	// Byte-level tamper: no sqlite driver is available (and none may be
	// added at T-100), so the checksum token is found via regexp over the
	// raw file and replaced with different hex of the same length.
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read DB file: %v", err)
	}
	tampered := migTamperFirstHex(t, data)
	if len(tampered) != len(data) {
		t.Fatalf("tamper changed the file length: %d -> %d", len(data), len(tampered))
	}
	if err := os.WriteFile(dbPath, tampered, 0o600); err != nil {
		t.Fatalf("write DB file back: %v", err)
	}

	// Respawn on the tampered DB: the process must EXIT non-zero, quickly.
	// A goroutine + timeout turns "keeps running" into a test failure
	// instead of a hang.
	s2 := migStart(t, dbPath, photoDir)
	t.Cleanup(func() { migStop(t, s2) })
	exited := make(chan error, 1)
	go func() { exited <- s2.cmd.Wait() }()
	select {
	case err := <-exited:
		if err == nil {
			t.Fatalf("server booted on a tampered migration checksum (exit status 0); output:\n%s", s2.out.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("server still running 15s after boot on a tampered checksum; output:\n%s", s2.out.String())
	}

	// The refusal must name the migration file and mention the checksum.
	out := strings.ToLower(s2.out.String())
	if !strings.Contains(out, "0001_init.sql") {
		t.Errorf("refusal output does not name the migration file 0001_init.sql:\n%s", s2.out.String())
	}
	if !strings.Contains(out, "checksum") {
		t.Errorf("refusal output does not mention the checksum mismatch:\n%s", s2.out.String())
	}
}
