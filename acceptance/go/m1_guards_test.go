//go:build acceptance

// M1 guards (T-100). A-101 and A-105 are satisfied by the deployed skeleton;
// these tests pin them as regressions. They are black-box: they either hit the
// server started by scripts/acceptance-go.sh (BASE_URL) or spawn bin/server
// themselves. Stdlib only.
package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- spawn helpers (conventions shared with the inert drafts in
// acceptance/drafts/: unique temp dir via t.TempDir(), free port by listening
// on :0 and closing, deferred kill+wait, context-bounded waits, never skip) ---

// syncBuf is a mutex-guarded buffer: exec's pipe-copy goroutines write while
// the test may read, so concurrent access must be safe.
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

// m1FindServerBin resolves the server binary: $BIN if set, else bin/server
// found by walking up from the test's working directory (the package dir) to
// the repo root. Not finding it is a FAILURE, never a skip: run `make build`.
func m1FindServerBin(t *testing.T) string {
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
	t.Fatal("server binary not found: run `make build` or set BIN=<path to bin/server>")
	return "" // unreachable
}

// m1FreePort picks a free TCP port by listening on 127.0.0.1:0 and closing.
func m1FreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen :0: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// m1Server is a self-spawned server instance.
type m1Server struct {
	cmd     *exec.Cmd
	baseURL string
	out     *syncBuf
	stopped bool
}

// m1Spawn starts the server binary on its own port and temp dir. appEnv is the
// APP_ENV value (test|prod). The .env.test dummy secrets are inlined so the
// spawn is hermetic; the env is constructed explicitly so no inherited
// BASE_URL/PORT/DB_PATH from the outer harness can leak in twice.
func m1Spawn(t *testing.T, appEnv string) *m1Server {
	t.Helper()
	bin := m1FindServerBin(t)
	dir := t.TempDir()
	port := m1FreePort(t)
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	out := &syncBuf{}
	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = []string{
		"PORT=" + strconv.Itoa(port),
		"APP_ENV=" + appEnv,
		"BASE_URL=" + base,
		"DB_PATH=" + filepath.Join(dir, "app.db"),
		"PHOTO_DIR=" + filepath.Join(dir, "photos"),
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
	s := &m1Server{cmd: cmd, baseURL: base, out: out}
	t.Cleanup(s.stop)
	return s
}

// stop kills the process and waits; idempotent. Waiting matters: exec drains
// its output pipes only once Wait returns, so Output() is complete afterwards.
func (s *m1Server) stop() {
	if s.stopped {
		return
	}
	s.stopped = true
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
}

// Output returns everything the process wrote to stdout+stderr so far.
func (s *m1Server) Output() string { return s.out.String() }

// m1WaitHealthy polls /healthz until 200 or the deadline; a hung or crashed
// server fails the test (with process output) instead of hanging the suite.
func m1WaitHealthy(t *testing.T, s *m1Server, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	client := &http.Client{Timeout: 1 * time.Second}
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
			s.stop()
			t.Fatalf("server not healthy within %s; output:\n%s", timeout, s.Output())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// TestA101HealthzFast: GET /healthz answers 200 with body ok within 2 seconds.
func TestA101HealthzFast(t *testing.T) {
	base := envOr("BASE_URL", "http://localhost:8099")
	start := time.Now()
	resp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	elapsed := time.Since(start)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("healthz answered in %s, want < 2s", elapsed)
	}
}

// TestA105ProdLogsJSONNoIP: under APP_ENV=prod every stdout line is JSON;
// request lines carry method, route pattern, status, duration and request id;
// the client IP (sent via X-Forwarded-For) never appears in the logs.
func TestA105ProdLogsJSONNoIP(t *testing.T) {
	s := m1Spawn(t, "prod")
	m1WaitHealthy(t, s, 5*time.Second)

	const clientIP = "203.0.113.77"
	client := &http.Client{Timeout: 2 * time.Second}
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/healthz", http.StatusOK},
		{"/no-such-path", http.StatusNotFound},
	} {
		req, err := http.NewRequest(http.MethodGet, s.baseURL+tc.path, nil)
		if err != nil {
			t.Fatalf("new request %s: %v", tc.path, err)
		}
		req.Header.Set("X-Forwarded-For", clientIP)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		if got := resp.StatusCode; got != tc.want {
			t.Errorf("GET %s: status = %d, want %d", tc.path, got, tc.want)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	// Let the child's request logs reach the pipe, then stop so the pipes
	// drain fully (Wait joins exec's copy goroutines).
	time.Sleep(300 * time.Millisecond)
	s.stop()
	out := s.Output()

	if strings.Contains(out, clientIP) {
		t.Fatalf("client IP %s leaked into prod logs:\n%s", clientIP, out)
	}

	requestLines := 0
	sawHealthz := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(line))
		dec.UseNumber()
		var obj map[string]any
		if err := dec.Decode(&obj); err != nil {
			t.Fatalf("prod stdout line is not JSON: %q: %v", line, err)
		}
		if msg, _ := obj["msg"].(string); msg != "request" {
			continue // startup/shutdown lines: JSON, but not request lines
		}
		requestLines++
		method, _ := obj["method"].(string)
		if method == "" {
			t.Errorf("request line without method: %s", line)
		}
		if _, ok := obj["pattern"]; !ok {
			t.Errorf("request line without pattern key: %s", line)
		}
		status, ok := obj["status"].(json.Number)
		if !ok {
			t.Errorf("request line without numeric status: %s", line)
		}
		if _, ok := obj["dur_ms"].(json.Number); !ok {
			t.Errorf("request line without numeric duration field dur_ms: %s", line)
		}
		if id, _ := obj["req_id"].(string); id == "" {
			t.Errorf("request line without req_id: %s", line)
		}
		// Matched routes log their registered pattern. An unmatched route
		// (404) legitimately has no pattern, so only key presence is
		// asserted for it; the healthz line must carry the real one.
		pattern, _ := obj["pattern"].(string)
		if method == http.MethodGet && pattern == "GET /healthz" && status.String() == "200" {
			sawHealthz = true
		}
	}
	if requestLines < 2 {
		t.Fatalf("want at least 2 request log lines, got %d; output:\n%s", requestLines, out)
	}
	if !sawHealthz {
		t.Errorf("no request line with pattern GET /healthz and status 200:\n%s", out)
	}
}
