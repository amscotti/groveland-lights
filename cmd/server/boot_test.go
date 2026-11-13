//go:build smoke

// Boot-level integration tests for the server binary: the wiring main.go
// does before serving (store.Open at boot, store handed to web.New, graceful
// shutdown) is invisible to in-process handler tests, so these spawn
// bin/server and exercise it black-box on a real port with a temp SQLite DB. They are the
// unit-side equivalent of the A-103/A-104 draft contracts (persistence across
// SIGTERM; checksum-mismatch boot refusal) and complement the protected
// acceptance suite.
//
// Run explicitly (they spawn a listening binary, so they must not run inside
// plain `go test ./...`): `make build` once, then
//
//	go test -tags smoke -count=1 ./cmd/server/
//
// Binary resolution mirrors the acceptance suite: $BIN if set, else bin/server
// found by walking up to the repo root. A missing binary is a failure, never a
// skip. Stdlib only.
package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
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

var (
	bootReCount = regexp.MustCompile(`<span id="probe-count">(\d+)</span>`)
	bootReHex   = regexp.MustCompile("[0-9a-f]{64}")
)

// syncBuf is a mutex-guarded buffer: the server writes while the test may
// read, so concurrent access must be safe.
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// bootFindBin resolves the server binary: $BIN if set, else bin/server found
// by walking up from the package dir to the repo root. Not finding it is a
// FAILURE, never a skip: run `make build`.
func bootFindBin(t *testing.T) string {
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
	return ""
}

// bootFreePort picks a free TCP port by listening on 127.0.0.1:0, then closing.
func bootFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen :0: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// bootServer is one spawned instance. dbPath is remembered so a second spawn
// can reopen the same database (the persistence path).
type bootServer struct {
	cmd     *exec.Cmd
	base    string
	dbPath  string
	out     *syncBuf
	stopped bool
}

// bootSpawn starts the binary (APP_ENV=test) on a free port with dbPath as
// DB_PATH and waits for /healthz. Cleanup kills + waits idempotently.
func bootSpawn(t *testing.T, dbPath string) *bootServer {
	t.Helper()
	bin := bootFindBin(t)
	port := bootFreePort(t)
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	out := &syncBuf{}
	cmd := exec.Command(bin)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(),
		"PORT="+strconv.Itoa(port),
		"APP_ENV=test",
		"BASE_URL="+base,
		"DB_PATH="+dbPath,
		"PHOTO_DIR="+filepath.Join(t.TempDir(), "photos"),
		"SESSION_KEY=dGVzdC1zZXNzaW9uLWtleS0zMi1ieXRlcy1hYQ==",
		"IP_HASH_KEY=dGVzdC1pcGhhc2gta2V5LTMyLWJ5dGVzLWJi",
		"ADMIN_TOKEN=dGVzdC1hZG1pbi10b2tlbi0zMi1ieXRl",
		"TOWN_BBOX=42.70,-71.09,42.82,-70.96",
		"TZ=America/New_York",
	)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", bin, err)
	}
	s := &bootServer{cmd: cmd, base: base, dbPath: dbPath, out: out}
	t.Cleanup(func() {
		if s.stopped {
			return
		}
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
	})
	bootWaitHealthy(t, s, 5*time.Second)
	return s
}

// bootStop sends SIGTERM and waits (bounded) for the graceful shutdown, so
// the store close checkpoints WAL before a respawn.
func bootStop(t *testing.T, s *bootServer) {
	t.Helper()
	if s.stopped {
		return
	}
	s.stopped = true
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	if err := s.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("SIGTERM server: %v", err)
	}
	select {
	case <-done:
		return
	case <-time.After(10 * time.Second):
		_ = s.cmd.Process.Kill()
		<-done
		t.Fatalf("server did not exit within 10s of SIGTERM; output:\n%s", s.out.String())
	}
}

func bootWaitHealthy(t *testing.T, s *bootServer, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	client := &http.Client{Timeout: time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/healthz", nil)
		if err != nil {
			t.Fatalf("new health request: %v", err)
		}
		if resp, err := client.Do(req); err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case <-ctx.Done():
			_ = s.cmd.Process.Kill()
			_ = s.cmd.Wait()
			t.Fatalf("server not healthy within %s; output:\n%s", timeout, s.out.String())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// bootDo performs one request against the spawned server without following
// redirects; form != nil makes it a urlencoded POST, hx adds HX-Request.
func bootDo(t *testing.T, s *bootServer, method, target string, cookie *http.Cookie, form url.Values, hx bool) (int, http.Header, string) {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(context.Background(), method, target, body)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: read body: %v", method, target, err)
	}
	return resp.StatusCode, resp.Header, string(b)
}

// bootFragment GETs /probe and enforces the fragment contract; it returns the
// CSRF cookie and the count.
func bootFragment(t *testing.T, s *bootServer) (*http.Cookie, int64) {
	t.Helper()
	status, headers, body := bootDo(t, s, http.MethodGet, s.base+"/probe", nil, nil, false)
	if status != http.StatusOK {
		t.Fatalf("GET /probe: status %d, want 200; body:\n%s", status, body)
	}
	var cookie *http.Cookie
	for _, c := range (&http.Response{Header: headers}).Cookies() {
		if c.Name == "csrf_token" {
			cookie = c
		}
	}
	if cookie == nil || cookie.Value == "" {
		t.Fatalf("GET /probe minted no csrf_token cookie; body:\n%s", body)
	}
	m := bootReCount.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("GET /probe fragment shows no count in #probe-count; body:\n%s", body)
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		t.Fatalf("GET /probe count %q: %v", m[1], err)
	}
	return cookie, n
}

// bootTamper flips one byte of the first 64-char lowercase-hex token in the
// DB file (the recorded migration checksum), keeping the length identical.
func bootTamper(t *testing.T, dbPath string) string {
	t.Helper()
	b, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read db: %v", err)
	}
	loc := bootReHex.FindIndex(b)
	if loc == nil {
		t.Fatalf("no 64-char hex token found in %s", dbPath)
	}
	if b[loc[0]] != 'f' {
		b[loc[0]] = 'f'
	} else {
		b[loc[0]] = '0'
	}
	tampered := filepath.Join(filepath.Dir(dbPath), "tampered.db")
	if err := os.WriteFile(tampered, b, 0o600); err != nil {
		t.Fatalf("write tampered db: %v", err)
	}
	return tampered
}

// TestBootSmokeProbeFlowsAndPersistence: the home page mounts the probe
// section, rejected POSTs leave the counter untouched, a valid HTMX POST
// increments, the count survives SIGTERM + respawn on the same DB_PATH, and
// a valid no-JS POST answers 303 to /.
func TestBootSmokeProbeFlowsAndPersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	s := bootSpawn(t, dbPath)

	status, _, body := bootDo(t, s, http.MethodGet, s.base+"/", nil, nil, false)
	if status != http.StatusOK {
		t.Fatalf("GET /: status %d, want 200; body:\n%s", status, body)
	}
	for _, want := range []string{`section id="probe"`, `hx-get="/probe"`, `<a href="/probe">Probe</a>`} {
		if !strings.Contains(body, want) {
			t.Fatalf("GET / missing probe mount markup %q; body:\n%s", want, body)
		}
	}

	cookie, n := bootFragment(t, s)

	// Missing token -> 403.
	if status, _, _ = bootDo(t, s, http.MethodPost, s.base+"/probe", cookie, nil, true); status != http.StatusForbidden {
		t.Fatalf("POST /probe without csrf_token: status %d, want 403", status)
	}
	// Tampered token (same shape, wrong value) -> 403.
	tampered := []byte(cookie.Value)
	if tampered[0] == 'A' {
		tampered[0] = 'B'
	} else {
		tampered[0] = 'A'
	}
	form := url.Values{"csrf_token": {string(tampered)}}
	if status, _, _ = bootDo(t, s, http.MethodPost, s.base+"/probe", cookie, form, true); status != http.StatusForbidden {
		t.Fatalf("POST /probe with tampered csrf_token: status %d, want 403", status)
	}
	// Counter unchanged after the rejected POSTs.
	_, now := bootFragment(t, s)
	if now != n {
		t.Fatalf("counter changed after rejected POSTs: %d, want %d", now, n)
	}

	// Valid HTMX POST -> 200 with n+1.
	form = url.Values{"csrf_token": {cookie.Value}}
	status, _, body = bootDo(t, s, http.MethodPost, s.base+"/probe", cookie, form, true)
	if status != http.StatusOK {
		t.Fatalf("POST /probe (HTMX): status %d, want 200; body:\n%s", status, body)
	}
	m := bootReCount.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("POST /probe (HTMX) fragment shows no count; body:\n%s", body)
	}
	if got, err := strconv.ParseInt(m[1], 10, 64); err != nil || got != n+1 {
		t.Fatalf("POST /probe (HTMX) count = %v, want %d", m[1], n+1)
	}

	// SIGTERM (graceful shutdown closes the store), respawn on the same
	// DB_PATH: the count is unchanged — the A-103 restart path.
	bootStop(t, s)
	s2 := bootSpawn(t, s.dbPath)
	_, after := bootFragment(t, s2)
	if after != n+1 {
		t.Fatalf("count after restart = %d, want %d (not persisted?)", after, n+1)
	}

	// No-JS form flow: valid token, no HX-Request -> 303 to /.
	cookie2, _ := bootFragment(t, s2)
	form = url.Values{"csrf_token": {cookie2.Value}}
	status, headers, body := bootDo(t, s2, http.MethodPost, s2.base+"/probe", cookie2, form, false)
	if status != http.StatusSeeOther {
		t.Fatalf("POST /probe without HX-Request: status %d, want 303; body:\n%s", status, body)
	}
	if loc := headers.Get("Location"); loc != "/" {
		t.Fatalf("POST /probe without HX-Request: Location = %q, want \"/\"", loc)
	}
}

// TestBootSmokeRefusesTamperedChecksum: booting on a DB whose recorded
// migration checksum no longer matches the embedded file must exit non-zero
// with output naming 0001_init.sql and "checksum" (the A-104 runtime half).
func TestBootSmokeRefusesTamperedChecksum(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	s := bootSpawn(t, dbPath)
	bootStop(t, s) // graceful close checkpoints WAL into the main DB file

	tampered := bootTamper(t, dbPath)
	bin := bootFindBin(t)
	out := &syncBuf{}
	cmd := exec.Command(bin)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(),
		"PORT="+strconv.Itoa(bootFreePort(t)),
		"APP_ENV=test",
		"BASE_URL=http://127.0.0.1:1",
		"DB_PATH="+tampered,
		"PHOTO_DIR="+filepath.Join(t.TempDir(), "photos"),
		"SESSION_KEY=dGVzdC1zZXNzaW9uLWtleS0zMi1ieXRlcy1hYQ==",
		"IP_HASH_KEY=dGVzdC1pcGhhc2gta2V5LTMyLWJ5dGVzLWJi",
		"ADMIN_TOKEN=dGVzdC1hZG1pbi10b2tlbi0zMi1ieXRl",
		"TOWN_BBOX=42.70,-71.09,42.82,-70.96",
		"TZ=America/New_York",
	)
	cmd.Stdout, cmd.Stderr = out, out
	_ = cmd.Run() // expected to fail; ProcessState below is the assertion
	if ps := cmd.ProcessState; ps == nil || ps.ExitCode() == 0 {
		t.Fatalf("tampered DB boot: expected non-zero exit; output:\n%s", out.String())
	}
	got := out.String()
	if !strings.Contains(got, "0001_init.sql") || !strings.Contains(got, "checksum") {
		t.Fatalf("refusal output must name 0001_init.sql and checksum; output:\n%s", got)
	}
}
