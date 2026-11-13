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

// TestWithLoggingRequestFields feeds requests through the production
// Handler() chain and asserts every request line carries method, route
// pattern, status and a duration >= 0 (A-105 contract at unit level). The
// request-id clause of A-105 is deliberately NOT covered here: see the
// non-coverage note in the body and harness/handoffs/T-102.md (BLOCKED).
func TestWithLoggingRequestFields(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantPattern string // Go's ServeMux leaves the pattern empty for unmatched routes
		wantCode    int
	}{
		{"matched route logs its pattern", "/healthz", "GET /healthz", http.StatusOK},
		{"unmatched route logs empty pattern", "/no-such-path", "", http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			s := bufferedServer(t, config.EnvTest, &buf)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, newTestRequest(http.MethodGet, tc.path))
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			lines := requestTextLines(buf.String())
			if len(lines) != 1 {
				t.Fatalf("want exactly 1 request line, got %d:\n%s", len(lines), buf.String())
			}
			// The pattern key must be present even when its value is
			// legitimately empty (unmatched route): an omitted key and an
			// empty value are different regressions.
			if !strings.Contains(lines[0], "pattern=") {
				t.Errorf("request line without pattern key: %s", lines[0])
			}
			f := textFields(lines[0])
			if f["method"] != http.MethodGet {
				t.Errorf("method = %q, want GET", f["method"])
			}
			if f["pattern"] != tc.wantPattern {
				t.Errorf("pattern = %q, want %q", f["pattern"], tc.wantPattern)
			}
			// The logged status must be the status actually sent, not merely
			// a plausible number.
			if got := f["status"]; got != strconv.Itoa(rec.Code) {
				t.Errorf("logged status = %q, want the response status %d", got, rec.Code)
			}
			dur, err := strconv.Atoi(f["dur_ms"])
			if err != nil {
				t.Fatalf("dur_ms = %q is not numeric: %v", f["dur_ms"], err)
			}
			if dur < 0 {
				t.Errorf("dur_ms = %d, want >= 0", dur)
			}
			// Deliberate non-coverage: the logged req_id value. Handler() in
			// web.go composes withLogging OUTSIDE withRequestID, so the logger
			// still holds the pre-enrichment request and every served line
			// logs the "-" no-id sentinel (requestIDOf), while the response
			// header carries the real id. An assertion that passes today
			// would rubber-stamp the sentinel as a request id; the failing
			// assertion belongs with the fix, which is a one-line reorder in
			// web.go — outside this task's owns, escalated as BLOCKED in
			// harness/handoffs/T-102.md. No test in this package asserts the
			// served chain's logged req_id. What IS covered on the served
			// chain is the id-assigner half of the clause: the response
			// carries a real 16-hex X-Request-Id for withLogging to have
			// logged.
			if id := rec.Header().Get("X-Request-Id"); !reRealRequestID.MatchString(id) {
				t.Errorf("response X-Request-Id = %q, want a real 16-hex request id", id)
			}
		})
	}
}

// reRealRequestID matches exactly what newRequestID mints: 16 lowercase hex
// characters (8 crypto/rand bytes). It never matches the "-" sentinel that
// requestIDOf returns when no id is in the request context.
var reRealRequestID = regexp.MustCompile(`^[0-9a-f]{16}$`)

// TestWithLoggingLogsContextRequestID is a unit test of withLogging's OWN
// contract: when the request being logged carries an id in its context —
// which happens exactly when the logger is composed inside withRequestID —
// the logged req_id is that request's real id (equal to the response
// X-Request-Id header), never the "-" sentinel. Here withRequestID wraps
// withLogging only to put a real id in the context; that is the arrangement
// withLogging was built for, and this test pins what each middleware unit
// contributes to A-105's request-id clause.
//
// NOT production coverage: Handler() in web.go composes the pair the other
// way around (logger outside id-assigner), so served requests log "-" and no
// test in this package asserts the served chain's logged req_id — that gap
// is escalated as BLOCKED in harness/handoffs/T-102.md. Whoever swaps the
// two composition lines in web.go should move this assertion onto
// s.Handler() so it guards the real chain.
func TestWithLoggingLogsContextRequestID(t *testing.T) {
	var buf bytes.Buffer
	s := bufferedServer(t, config.EnvTest, &buf)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	h := s.withRequestID(s.withLogging(mux))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newTestRequest(http.MethodGet, "/healthz"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	lines := requestTextLines(buf.String())
	if len(lines) != 1 {
		t.Fatalf("want exactly 1 request line, got %d:\n%s", len(lines), buf.String())
	}
	f := textFields(lines[0])
	hdrID := rec.Header().Get("X-Request-Id")
	if !reRealRequestID.MatchString(hdrID) {
		t.Fatalf("response X-Request-Id = %q, want a 16-hex request id", hdrID)
	}
	if f["req_id"] != hdrID {
		t.Fatalf("logged req_id = %q, want the request's real id %q (the response X-Request-Id)", f["req_id"], hdrID)
	}
	if f["method"] != http.MethodGet {
		t.Errorf("method = %q, want GET", f["method"])
	}
	if f["pattern"] != "GET /healthz" {
		t.Errorf("pattern = %q, want GET /healthz", f["pattern"])
	}
	if f["status"] != strconv.Itoa(rec.Code) {
		t.Errorf("logged status = %q, want the response status %d", f["status"], rec.Code)
	}
	if dur, err := strconv.Atoi(f["dur_ms"]); err != nil || dur < 0 {
		t.Errorf("dur_ms = %q, want a number >= 0", f["dur_ms"])
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
// is a request record (plus its pattern and logged status). Mirrors the A-105
// acceptance contract for the fields coverable through the served chain:
// method, pattern key, numeric status, numeric dur_ms >= 0. The logged
// req_id value is deliberately not checked — see the non-coverage note in
// TestWithLoggingRequestFields and harness/handoffs/T-102.md (BLOCKED).
func jsonRequestLine(t *testing.T, line string) (ok bool, pattern, status string) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		t.Fatalf("prod log line is not JSON: %q: %v", line, err)
	}
	if msg, _ := obj["msg"].(string); msg != "request" {
		return false, "", ""
	}
	if m, _ := obj["method"].(string); m == "" {
		t.Errorf("request line without method: %s", line)
	}
	pattern, _ = obj["pattern"].(string)
	if _, ok := obj["pattern"]; !ok {
		t.Errorf("request line without pattern key: %s", line)
	}
	if st, ok := obj["status"].(json.Number); !ok {
		t.Errorf("request line without numeric status: %s", line)
	} else {
		status = st.String()
	}
	dur, ok := obj["dur_ms"].(json.Number)
	if !ok {
		t.Errorf("request line without numeric dur_ms: %s", line)
	} else if d, err := dur.Int64(); err != nil || d < 0 {
		t.Errorf("dur_ms = %v, want a number >= 0", dur)
	}
	// req_id is deliberately not asserted here: through the served chain the
	// logged value is the "-" no-id sentinel, and neither a value check nor
	// a mere key-presence check would be an honest assertion of A-105's
	// request-id clause (see the non-coverage note in
	// TestWithLoggingRequestFields and the BLOCKED handoff).
	return true, pattern, status
}

// TestProdLoggingIsJSON constructs the logger the way cmd/server/main.go does
// in prod (slog.NewJSONHandler) and asserts: every stdout line is JSON,
// request lines carry every field coverable through the served chain
// (method, pattern key, numeric status, numeric dur_ms; the logged req_id
// value is deliberately not asserted — see the BLOCKED note in
// TestWithLoggingRequestFields), each logged status is the status actually
// sent, and the injected client IP never appears.
func TestProdLoggingIsJSON(t *testing.T) {
	const clientIP = "203.0.113.77"
	var buf bytes.Buffer
	s := bufferedServer(t, config.EnvProd, &buf)
	h := s.Handler()

	type served struct {
		path string
		rec  *httptest.ResponseRecorder
	}
	var servedReqs []served
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/healthz", http.StatusOK},
		{"/no-such-path", http.StatusNotFound},
	} {
		r := newTestRequest(http.MethodGet, tc.path)
		r.Header.Set("X-Forwarded-For", clientIP)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != tc.want {
			t.Fatalf("GET %s: status = %d, want %d", tc.path, rec.Code, tc.want)
		}
		servedReqs = append(servedReqs, served{path: tc.path, rec: rec})
	}

	out := buf.String()
	if strings.Contains(out, clientIP) {
		t.Fatalf("client IP %s leaked into prod logs:\n%s", clientIP, out)
	}

	type reqLine struct{ pattern, status string }
	var reqs []reqLine
	sawHealthz := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		isReq, pattern, status := jsonRequestLine(t, line)
		if !isReq {
			continue
		}
		reqs = append(reqs, reqLine{pattern: pattern, status: status})
		if pattern == "GET /healthz" {
			sawHealthz = true
		}
	}
	if len(reqs) != len(servedReqs) {
		t.Fatalf("want %d request lines, got %d; output:\n%s", len(servedReqs), len(reqs), out)
	}
	// Requests are served sequentially, so log order matches serve order:
	// each logged status must equal the status its request actually got.
	for i, rl := range reqs {
		if want := strconv.Itoa(servedReqs[i].rec.Code); rl.status != want {
			t.Errorf("GET %s: logged status = %q, want the response status %s", servedReqs[i].path, rl.status, want)
		}
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
