//go:build acceptance

// @A-103 — CSRF-protected /probe counter persisted in SQLite (frozen contract,
// written by T-100 before the feature tasks branch).
//
// ACTIVATED by T-105 from acceptance/drafts/probe_go_test.go.txt (rename;
// the draft's byte-exact text stays auditable in git history). Exactly one
// content edit was made — the mechanical regex pin documented at probeReCount
// below; every assertion is the frozen text unchanged.
//
// Contract owners: T-102 (double-submit CSRF: hidden input named
// `csrf_token`; 403 on missing or tampered token), T-103 (routes and
// persistence: GET /probe fragment; POST /probe HTMX/no-JS flows; counter in
// SQLite at DB_PATH).
//
// Frozen flow (byte-exact expectations for workers):
//
//	GET  /        -> 200 (session cookies captured by the jar)
//	GET  /probe   -> 200 fragment containing "Count: N" and a hidden input
//	                 named csrf_token (attribute order is free)
//	POST /probe   -> 403 when csrf_token is missing or tampered
//	POST /probe   -> 200 with incremented "Count:" when the token is valid
//	                 and HX-Request: true is sent (HTMX flow)
//	POST /probe   -> 303 Location: / when the token is valid but no
//	                 HX-Request header is sent (no-JS form flow)
//	restart       -> SIGTERM + respawn on the same DB_PATH shows the same
//	                 count (the counter lives in SQLite, not memory)
package acceptance

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
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
	// T-105 activation edit (the only one): the frozen contract pins the
	// markup as `Count: <span id="probe-count">N</span>` (drafts README,
	// T-103 packet), so the digits always live inside #probe-count. The
	// draft's `Count:\s*(\d+)` could never match that markup; this pin is
	// strictly tighter (it also proves N is held by #probe-count).
	probeReCount = regexp.MustCompile(`Count:\s*<span id="probe-count">(\d+)</span>`)
	probeReInput = regexp.MustCompile(`<input[^>]*name="csrf_token"[^>]*>`)
	probeReValue = regexp.MustCompile(`value="([^"]*)"`)
)

// TestA103ProbeRequiresCSRF: POST /probe without a token, or with a tampered
// token (same shape, wrong value), is rejected with 403 and leaves the counter
// unchanged — the next valid POST still numbers from the original count.
func TestA103ProbeRequiresCSRF(t *testing.T) {
	s := probeSpawnDir(t, "", "")
	c := probeClient(t)

	if status, _, body := probeDo(t, c, http.MethodGet, s.baseURL+"/", nil, false); status != http.StatusOK {
		t.Fatalf("GET /: status %d, want 200; body:\n%s", status, body)
	}
	_, initial := probeFragment(t, c, s.baseURL)

	// Missing token -> 403.
	if status, _, _ := probeDo(t, c, http.MethodPost, s.baseURL+"/probe", nil, true); status != http.StatusForbidden {
		t.Fatalf("POST /probe without csrf_token: status %d, want 403", status)
	}

	// Tampered token (same shape, wrong value) -> 403.
	token, _ := probeFragment(t, c, s.baseURL)
	form := url.Values{"csrf_token": {probeTamper(token)}}
	if status, _, _ := probeDo(t, c, http.MethodPost, s.baseURL+"/probe", form, true); status != http.StatusForbidden {
		t.Fatalf("POST /probe with tampered csrf_token: status %d, want 403", status)
	}

	// Counter unchanged after the rejected POSTs, and a subsequent valid POST
	// numbers consistently from the initial count.
	token, now := probeFragment(t, c, s.baseURL)
	if now != initial {
		t.Fatalf("counter changed after rejected POSTs: %d, want %d", now, initial)
	}
	form = url.Values{"csrf_token": {token}}
	status, _, body := probeDo(t, c, http.MethodPost, s.baseURL+"/probe", form, true)
	if status != http.StatusOK {
		t.Fatalf("POST /probe with valid csrf_token: status %d, want 200; body:\n%s", status, body)
	}
	m := probeReCount.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("POST /probe response has no \"Count: N\" text; body:\n%s", body)
	}
	if got, err := strconv.Atoi(m[1]); err != nil || got != initial+1 {
		t.Fatalf("counter after valid POST = %v (parsed %q), want %d", m[1], m[1], initial+1)
	}
}

// TestA103ProbeIncrementsAndPersists: a valid HTMX POST increments; the count
// survives SIGTERM + respawn on the same DB_PATH; a valid POST without
// HX-Request is the no-JS flow and must answer 303 with Location: /.
func TestA103ProbeIncrementsAndPersists(t *testing.T) {
	s := probeSpawnDir(t, "", "")
	c := probeClient(t)

	if status, _, body := probeDo(t, c, http.MethodGet, s.baseURL+"/", nil, false); status != http.StatusOK {
		t.Fatalf("GET /: status %d, want 200; body:\n%s", status, body)
	}

	// Valid HTMX POST increments the fragment count.
	token, initial := probeFragment(t, c, s.baseURL)
	form := url.Values{"csrf_token": {token}}
	status, _, body := probeDo(t, c, http.MethodPost, s.baseURL+"/probe", form, true)
	if status != http.StatusOK {
		t.Fatalf("POST /probe (HTMX): status %d, want 200; body:\n%s", status, body)
	}
	if m := probeReCount.FindStringSubmatch(body); m == nil {
		t.Fatalf("POST /probe (HTMX) response has no \"Count: N\" text; body:\n%s", body)
	} else if got, err := strconv.Atoi(m[1]); err != nil || got != initial+1 {
		t.Fatalf("POST /probe (HTMX) count = %q, want %d", m[1], initial+1)
	}

	// SIGTERM (graceful shutdown flushes SQLite), respawn on the same
	// DB_PATH, and the count is unchanged. A fresh GET /probe mints whatever
	// session/CSRF state the new process needs.
	probeStop(t, s)
	s2 := probeSpawnDir(t, s.dbPath, s.photoDir)
	_, after := probeFragment(t, c, s2.baseURL)
	if after != initial+1 {
		t.Fatalf("count after restart = %d, want %d (not persisted?)", after, initial+1)
	}

	// No-JS form flow: valid token, no HX-Request header -> 303 to /.
	token2, _ := probeFragment(t, c, s2.baseURL)
	form2 := url.Values{"csrf_token": {token2}}
	status, header, body := probeDo(t, c, http.MethodPost, s2.baseURL+"/probe", form2, false)
	if status != http.StatusSeeOther {
		t.Fatalf("POST /probe without HX-Request: status %d, want 303; body:\n%s", status, body)
	}
	if loc := header.Get("Location"); loc != "/" {
		t.Fatalf("POST /probe without HX-Request: Location = %q, want \"/\"", loc)
	}
}

// --- probe helpers (self-contained on purpose: activation is a pure rename,
// so this file must not depend on helpers defined elsewhere in the package) ---

// probeTamper returns a token of the same shape (same length) with a wrong
// value: the first character is flipped.
func probeTamper(token string) string {
	if token == "" {
		return "tampered"
	}
	b := []byte(token)
	if b[0] == 'A' {
		b[0] = 'B'
	} else {
		b[0] = 'A'
	}
	return string(b)
}

// probeClient is a cookie-carrying client that does NOT follow redirects, so
// the 303 of the no-JS flow is observable.
func probeClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{
		Jar:     jar,
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// probeDo performs one request; form != nil makes it a urlencoded POST. It
// returns status, headers and the fully-read body.
func probeDo(t *testing.T, c *http.Client, method, target string, form url.Values, hx bool) (int, http.Header, string) {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: read body: %v", method, target, err)
	}
	return resp.StatusCode, resp.Header, string(b)
}

// probeFragment GETs /probe and enforces the fragment contract: 200, contains
// "Count: N" and a hidden input named csrf_token. Returns (token, count).
func probeFragment(t *testing.T, c *http.Client, base string) (string, int) {
	t.Helper()
	status, _, body := probeDo(t, c, http.MethodGet, base+"/probe", nil, false)
	if status != http.StatusOK {
		t.Fatalf("GET /probe: status %d, want 200; body:\n%s", status, body)
	}
	m := probeReCount.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("GET /probe: fragment has no \"Count: N\" text; body:\n%s", body)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("GET /probe: bad count %q: %v", m[1], err)
	}
	input := probeReInput.FindString(body)
	if input == "" {
		t.Fatalf("GET /probe: no hidden input named csrf_token; body:\n%s", body)
	}
	v := probeReValue.FindStringSubmatch(input)
	if v == nil {
		t.Fatalf("GET /probe: csrf_token input has no value; body:\n%s", body)
	}
	return v[1], n
}

type probeSyncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *probeSyncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *probeSyncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// probeServer is one spawned instance; dbPath/photoDir are remembered so a
// second spawn can reuse the same database.
type probeServer struct {
	cmd      *exec.Cmd
	baseURL  string
	dbPath   string
	photoDir string
	out      *probeSyncBuf
	stopped  bool
}

// probeSpawnDir starts the server (APP_ENV=test) on its own port. Empty
// dbPath/photoDir mean "fresh paths under a t.TempDir()"; pass the previous
// server's values to respawn on the same database.
func probeSpawnDir(t *testing.T, dbPath, photoDir string) *probeServer {
	t.Helper()
	bin := probeFindBin(t)
	dir := t.TempDir()
	if dbPath == "" {
		dbPath = filepath.Join(dir, "app.db")
	}
	if photoDir == "" {
		photoDir = filepath.Join(dir, "photos")
	}
	port := probeFreePort(t)
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	out := &probeSyncBuf{}
	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = []string{
		"PORT=" + strconv.Itoa(port),
		"APP_ENV=test",
		"BASE_URL=" + base,
		"DB_PATH=" + dbPath,
		"PHOTO_DIR=" + photoDir,
		"SESSION_KEY=dGVzdC1zZXNzaW9uLWtleS0zMi1ieXRlcy1hYQ==",
		"IP_HASH_KEY=dGVzdC1pcGhhc2gta2V5LTMyLWJ5dGVzLWJi",
		"ADMIN_TOKEN=dGVzdC1hZG1pbi10b2tlbi0zMi1ieXRl",
		"TOWN_BBOX=42.70,-71.09,42.82,-70.96",
		"TZ=America/New_York",
		"PATH=" + os.Getenv("PATH"),
	}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", bin, err)
	}
	s := &probeServer{cmd: cmd, baseURL: base, dbPath: dbPath, photoDir: photoDir, out: out}
	t.Cleanup(func() { probeKill(t, s) })
	probeWaitHealthy(t, s, 5*time.Second)
	return s
}

// probeStop sends SIGTERM and waits (bounded) for a graceful shutdown so all
// writes are flushed before a respawn. A hang is a failure, never a skip.
func probeStop(t *testing.T, s *probeServer) {
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

// probeKill is the cleanup stop (SIGKILL + wait), idempotent.
func probeKill(t *testing.T, s *probeServer) {
	t.Helper()
	if s.stopped {
		return
	}
	s.stopped = true
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
}

// probeFindBin resolves the binary like m1FindServerBin: $BIN, else bin/server
// by walking up to the repo root. Never skips: a missing binary is a failure.
func probeFindBin(t *testing.T) string {
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

// probeFreePort picks a free TCP port by listening on 127.0.0.1:0, then
// closing the listener.
func probeFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen :0: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// probeWaitHealthy polls /healthz until 200 with a context deadline; a hung
// or crashed server fails the test with its output instead of hanging.
func probeWaitHealthy(t *testing.T, s *probeServer, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	client := &http.Client{Timeout: 1 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/healthz", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
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
			probeKill(t, s)
			t.Fatalf("server not healthy within %s; output:\n%s", timeout, s.out.String())
		case <-time.After(50 * time.Millisecond):
		}
	}
}
