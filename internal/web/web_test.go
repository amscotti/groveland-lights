package web

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amscotti/groveland-lights/internal/config"
)

func testConfig(env config.Env) config.Config {
	return config.Config{
		Port: "0", AppEnv: env, BaseURL: "http://test",
		DBPath: tPath, PhotoDir: tPath,
		SessionKey: []byte("k"), IPHashKey: []byte("k"), AdminToken: []byte("k"),
		Timezone: "America/New_York",
	}
}

var tPath = os.TempDir()

func newTestServer(t *testing.T, env config.Env) *Server {
	t.Helper()
	s, err := New(testConfig(env), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestHealthz(t *testing.T) {
	s := newTestServer(t, config.EnvTest)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestSecurityHeadersPresent(t *testing.T) {
	s := newTestServer(t, config.EnvTest)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "frame-ancestors 'none'", "base-uri 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP missing %q: %s", want, csp)
		}
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff")
	}
	if rec.Header().Get("Referrer-Policy") != "same-origin" {
		t.Error("missing Referrer-Policy")
	}
}

func TestHomeRenders(t *testing.T) {
	s := newTestServer(t, config.EnvTest)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Groveland Lights") {
		t.Error("home missing title")
	}
}

func TestHomeNoTestHooksInProd(t *testing.T) {
	s := newTestServer(t, config.EnvProd)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Contains(rec.Body.String(), "test-hooks.js") {
		t.Error("prod render includes test hooks (must be compiled out per §5.1)")
	}
}

func TestHomeTestHooksInTestEnv(t *testing.T) {
	s := newTestServer(t, config.EnvTest)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rec.Body.String(), "test-hooks.js") {
		t.Error("test render missing test hooks")
	}
}

func TestStaticServed(t *testing.T) {
	s := newTestServer(t, config.EnvTest)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/css/app.css", nil))
	if rec.Code != 200 {
		t.Fatalf("css code = %d", rec.Code)
	}
}

// TestRenderAll renders every template with fixture data and writes snapshots to
// testdata/rendered for html-validate (Makefile templates-check target).
func TestRenderAll(t *testing.T) {
	s := newTestServer(t, config.EnvTest)
	outDir := filepath.Join("..", "..", "testdata", "rendered")
	if os.Getenv("GL_RENDER_SNAPSHOTS") == "" {
		t.Skip("set GL_RENDER_SNAPSHOTS=1 (make templates-check) to write rendered snapshots")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name := range s.tpl.pages {
		rec := httptest.NewRecorder()
		s.tpl.renderPage(rec, filepath.Base(name), homeData{Title: "Groveland Lights", TestEnv: false, EnvLabel: "prod"})
		out := filepath.Join(outDir, strings.TrimPrefix(name, "pages/"))
		if err := os.WriteFile(out, rec.Body.Bytes(), 0o644); err != nil {
			t.Fatalf("write %s: %v", out, err)
		}
	}
}
