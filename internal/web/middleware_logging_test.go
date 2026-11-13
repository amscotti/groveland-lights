package web

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/amscotti/groveland-lights/internal/config"
)

// bufferedServer builds a Server whose logger writes into buf. prod selects
// the slog.NewJSONHandler that cmd/server/main.go installs under APP_ENV=prod;
// every other env gets the text handler main.go uses elsewhere.
func bufferedServer(t *testing.T, env config.Env, buf *bytes.Buffer) *Server {
	t.Helper()
	var h slog.Handler
	if env == config.EnvProd {
		h = slog.NewJSONHandler(buf, nil)
	} else {
		h = slog.NewTextHandler(buf, nil)
	}
	s, err := New(testConfig(env), slog.New(h))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// newTestRequest builds a request the way the existing web tests do.
func newTestRequest(method, target string) *http.Request {
	return httptest.NewRequestWithContext(context.Background(), method, target, nil)
}

// textField matches one key=value pair of a slog text line, capturing both
// the quoted form (values with spaces, e.g. pattern="GET /healthz") and the
// bare form.
var textField = regexp.MustCompile(`(\w+)=(?:"((?:[^"\\]|\\.)*)"|(\S+))`)

func textFields(line string) map[string]string {
	fields := map[string]string{}
	for _, m := range textField.FindAllStringSubmatch(line, -1) {
		v := m[3] // bare form (\S+ always captures at least one char)
		if m[3] == "" {
			v = m[2] // quoted form; may legitimately be empty (pattern="")
		}
		fields[m[1]] = v
	}
	return fields
}

// requestTextLines returns the lines of a text log that are request records.
func requestTextLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "msg=request") {
			lines = append(lines, line)
		}
	}
	return lines
}

// TestWithLoggingRequestFields feeds requests through the middleware chain
// over a real mux and asserts every request line carries method, route
// pattern, status and a duration >= 0 (A-105 contract at unit level).
func TestWithLoggingRequestFields(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantPattern string // Go's ServeMux leaves the pattern empty for unmatched routes
		wantStatus  string
	}{
		{"matched route logs its pattern", "/healthz", "GET /healthz", "200"},
		{"unmatched route logs empty pattern", "/no-such-path", "", "404"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			s := bufferedServer(t, config.EnvTest, &buf)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, newTestRequest(http.MethodGet, tc.path))
			if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 200 or 404", rec.Code)
			}
			lines := requestTextLines(buf.String())
			if len(lines) != 1 {
				t.Fatalf("want exactly 1 request line, got %d:\n%s", len(lines), buf.String())
			}
			f := textFields(lines[0])
			if f["method"] != http.MethodGet {
				t.Errorf("method = %q, want GET", f["method"])
			}
			if f["pattern"] != tc.wantPattern {
				t.Errorf("pattern = %q, want %q", f["pattern"], tc.wantPattern)
			}
			if f["status"] != tc.wantStatus {
				t.Errorf("status = %q, want %q", f["status"], tc.wantStatus)
			}
			dur, err := strconv.Atoi(f["dur_ms"])
			if err != nil {
				t.Fatalf("dur_ms = %q is not numeric: %v", f["dur_ms"], err)
			}
			if dur < 0 {
				t.Errorf("dur_ms = %d, want >= 0", dur)
			}
			if f["req_id"] == "" {
				t.Error("req_id missing from request line")
			}
		})
	}
}

// TestWithLoggingOmitsClientIP: neither the connection address nor an
// injected X-Forwarded-For may appear anywhere in the log output (A-105).
func TestWithLoggingOmitsClientIP(t *testing.T) {
	var buf bytes.Buffer
	s := bufferedServer(t, config.EnvTest, &buf)
	r := newTestRequest(http.MethodGet, "/healthz")
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.77")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)
	out := buf.String()
	if len(requestTextLines(out)) != 1 {
		t.Fatalf("want exactly 1 request line, got:\n%s", out)
	}
	for _, leak := range []string{"192.0.2.1", "203.0.113.77"} {
		if strings.Contains(out, leak) {
			t.Fatalf("log output contains %s:\n%s", leak, out)
		}
	}
}

// jsonRequestLine validates one prod log line as JSON and returns whether it
// is a request record (plus its pattern). Mirrors the A-105 acceptance
// contract: request lines carry method, pattern key, numeric status, numeric
// dur_ms >= 0 and a req_id.
func jsonRequestLine(t *testing.T, line string) (bool, string) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		t.Fatalf("prod log line is not JSON: %q: %v", line, err)
	}
	if msg, _ := obj["msg"].(string); msg != "request" {
		return false, ""
	}
	if m, _ := obj["method"].(string); m == "" {
		t.Errorf("request line without method: %s", line)
	}
	pattern, _ := obj["pattern"].(string)
	if _, ok := obj["pattern"]; !ok {
		t.Errorf("request line without pattern key: %s", line)
	}
	if _, ok := obj["status"].(json.Number); !ok {
		t.Errorf("request line without numeric status: %s", line)
	}
	dur, ok := obj["dur_ms"].(json.Number)
	if !ok {
		t.Errorf("request line without numeric dur_ms: %s", line)
	} else if d, err := dur.Int64(); err != nil || d < 0 {
		t.Errorf("dur_ms = %v, want a number >= 0", dur)
	}
	if id, _ := obj["req_id"].(string); id == "" {
		t.Errorf("request line without req_id: %s", line)
	}
	return true, pattern
}

// TestProdLoggingIsJSON constructs the logger the way cmd/server/main.go does
// in prod (slog.NewJSONHandler) and asserts: every stdout line is JSON,
// request lines carry the full field set, and the injected client IP never
// appears.
func TestProdLoggingIsJSON(t *testing.T) {
	const clientIP = "203.0.113.77"
	var buf bytes.Buffer
	s := bufferedServer(t, config.EnvProd, &buf)
	h := s.Handler()

	for _, p := range []string{"/healthz", "/no-such-path"} {
		r := newTestRequest(http.MethodGet, p)
		r.Header.Set("X-Forwarded-For", clientIP)
		h.ServeHTTP(httptest.NewRecorder(), r)
	}

	out := buf.String()
	if strings.Contains(out, clientIP) {
		t.Fatalf("client IP %s leaked into prod logs:\n%s", clientIP, out)
	}

	requests := 0
	sawHealthz := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		isReq, pattern := jsonRequestLine(t, line)
		if !isReq {
			continue
		}
		requests++
		if pattern == "GET /healthz" {
			sawHealthz = true
		}
	}
	if requests != 2 {
		t.Fatalf("want 2 request lines, got %d; output:\n%s", requests, out)
	}
	if !sawHealthz {
		t.Errorf("no request line with pattern GET /healthz:\n%s", out)
	}
}

// TestMiddlewareGuards pins the remaining chain invariants at unit level
// (existing middleware, no production-code edits).
func TestMiddlewareGuards(t *testing.T) {
	t.Run("recover turns panic into 500", func(t *testing.T) {
		var buf bytes.Buffer
		s := bufferedServer(t, config.EnvTest, &buf)
		h := s.withRecover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("boom")
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, newTestRequest(http.MethodGet, "/"))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "internal error") {
			t.Errorf("body = %q, want the generic internal error text", rec.Body.String())
		}
		if !strings.Contains(buf.String(), "panic") {
			t.Error("panic not logged")
		}
	})

	t.Run("recover passes clean handlers through", func(t *testing.T) {
		var buf bytes.Buffer
		s := bufferedServer(t, config.EnvTest, &buf)
		h := s.withRecover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, newTestRequest(http.MethodGet, "/"))
		if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
			t.Fatalf("status/body = %d/%q, want 200/ok", rec.Code, rec.Body.String())
		}
		if strings.Contains(buf.String(), "panic") {
			t.Error("clean handler logged a panic")
		}
	})

	t.Run("request id in header matches context", func(t *testing.T) {
		s := bufferedServer(t, config.EnvTest, &bytes.Buffer{})
		var hdrID, ctxID string
		h := s.withRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hdrID = w.Header().Get("X-Request-Id")
			ctxID = requestIDOf(r.Context())
		}))
		h.ServeHTTP(httptest.NewRecorder(), newTestRequest(http.MethodGet, "/"))
		if hdrID == "" {
			t.Fatal("X-Request-Id header not set")
		}
		if hdrID != ctxID {
			t.Fatalf("header %q != context %q", hdrID, ctxID)
		}
	})

	t.Run("request ids differ per request", func(t *testing.T) {
		s := bufferedServer(t, config.EnvTest, &bytes.Buffer{})
		h := s.withRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		first := httptest.NewRecorder()
		h.ServeHTTP(first, newTestRequest(http.MethodGet, "/"))
		second := httptest.NewRecorder()
		h.ServeHTTP(second, newTestRequest(http.MethodGet, "/"))
		a := first.Header().Get("X-Request-Id")
		b := second.Header().Get("X-Request-Id")
		if a == "" || b == "" {
			t.Fatalf("empty request id(s): %q, %q", a, b)
		}
		if a == b {
			t.Fatalf("two requests share request id %q", a)
		}
	})
}
