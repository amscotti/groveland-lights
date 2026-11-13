package web

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/amscotti/groveland-lights/internal/config"
	"github.com/amscotti/groveland-lights/internal/store"
)

// authAdminToken is the ADMIN_TOKEN the auth tests configure; the frozen
// exchange compares every submission against exactly this value.
const authAdminToken = "baseline-test-admin-token"

// authSessionKey is the signing key for the auth tests' cookies (32 bytes).
var authSessionKey = []byte("0123456789abcdef0123456789abcdef")

// The frozen login-page markup (mirrors the acceptance draft's regexes).
var (
	authReHeading = regexp.MustCompile(`<h1[^>]*>\s*Admin login\s*</h1>`)
	authReLabel   = regexp.MustCompile(`<label[^>]*>\s*Token\s*</label>`)
	authReInput   = regexp.MustCompile(`<input[^>]*name="token"[^>]*>`)
	authReButton  = regexp.MustCompile(`<button[^>]*>\s*Sign in\s*</button>`)
)

// newAuthServer builds a Server wired to a real temp-file store (migrations
// applied by store.Open) with a known ADMIN_TOKEN and signing key, exactly
// the way main wires web.New(cfg, log, db).
func newAuthServer(t *testing.T, env config.Env) (*Server, *store.DB) {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg := testConfig(env)
	cfg.AdminToken = []byte(authAdminToken)
	cfg.SessionKey = authSessionKey
	s, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, db
}

// authGet runs a GET through the full middleware chain and returns the
// recorder (redirects stay observable: ServeHTTP never follows them).
func authGet(t *testing.T, s *Server, target string, hx bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// authPostForm runs a urlencoded POST through the full middleware chain,
// optionally carrying the csrf_token cookie.
func authPostForm(t *testing.T, s *Server, target, csrfCookie string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if csrfCookie != "" {
		req.AddCookie(&http.Cookie{Name: CSRFTokenName, Value: csrfCookie})
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// sessionCookieOf extracts the gl_session cookie from a response, failing
// the test when it is absent.
func sessionCookieOf(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	for _, c := range res.Cookies() {
		if c.Name == SessionCookieName {
			return c
		}
	}
	t.Fatalf("response set no %s cookie; Set-Cookie: %v", SessionCookieName, res.Header.Values("Set-Cookie"))
	return nil // unreachable
}

// assertLoginMarkup fails unless the body carries all four frozen elements.
func assertLoginMarkup(t *testing.T, body string) {
	t.Helper()
	for _, re := range []*regexp.Regexp{authReHeading, authReLabel, authReInput, authReButton} {
		if !re.MatchString(body) {
			t.Errorf("login page missing the frozen %q element; body:\n%s", re.String(), body)
		}
	}
}

// assertFrozenCookieFlags checks the exact attribute set the A-201 draft
// parses and compares: Path=/, Max-Age=86400, HttpOnly, SameSite=Lax, with
// Secure exactly in prod.
func assertFrozenCookieFlags(t *testing.T, c *http.Cookie, env config.Env) {
	t.Helper()
	if c.Path != "/" {
		t.Errorf("cookie Path = %q, want exactly \"/\"", c.Path)
	}
	if c.MaxAge != 86400 {
		t.Errorf("cookie Max-Age = %d, want 86400 (24 h)", c.MaxAge)
	}
	if !c.HttpOnly {
		t.Error("cookie lacks the HttpOnly flag")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite = %v, want Lax", c.SameSite)
	}
	if want := env == config.EnvProd; c.Secure != want {
		t.Errorf("cookie Secure = %v, want %v (env %s)", c.Secure, want, env)
	}
}

// assertRedirectToLogin asserts the full-page guard refusal: 302 with
// Location /admin/login and the guarded handler's body never written.
func assertRedirectToLogin(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusFound {
		t.Fatalf("guard status = %d, want 302; body:\n%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/login" {
		t.Fatalf("guard Location = %q, want %q", loc, "/admin/login")
	}
	if strings.Contains(rec.Body.String(), "admin-ok") {
		t.Fatal("guarded handler ran despite the refusal")
	}
}

// mustCSRFToken mints a CSRF token for the POST tests, failing on error.
func mustCSRFToken(t *testing.T) string {
	t.Helper()
	tok, err := newCSRFToken()
	if err != nil {
		t.Fatalf("newCSRFToken: %v", err)
	}
	return tok
}

// TestAdminLoginExchange covers the frozen one-time exchange happy path:
// the valid token mints a live admin session and redirects.
func TestAdminLoginExchange(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"valid token first use sets session and redirects", func(t *testing.T) {
			s, db := newAuthServer(t, config.EnvTest)
			rec := authGet(t, s, "/admin/login?token="+authAdminToken, false)
			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302; body:\n%s", rec.Code, rec.Body.String())
			}
			if loc := rec.Header().Get("Location"); loc != "/admin" {
				t.Fatalf("Location = %q, want %q", loc, "/admin")
			}
			c := sessionCookieOf(t, rec)
			assertFrozenCookieFlags(t, c, config.EnvTest)
			// The cookie value must verify and name a live admin session.
			id, ok := sessionIDOf(authSessionKey, c.Value)
			if !ok {
				t.Fatalf("minted cookie value %q does not verify", c.Value)
			}
			uid, role, valid, err := db.SessionValid(context.Background(), id)
			if err != nil || !valid {
				t.Fatalf("minted session %q not valid: ok=%v err=%v", id, valid, err)
			}
			if role != "admin" {
				t.Fatalf("minted session role = %q, want admin", role)
			}
			if uid <= 0 {
				t.Fatalf("minted session user id = %d, want > 0", uid)
			}
		}},
		{"prod minted cookie is Secure", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvProd)
			rec := authGet(t, s, "/admin/login?token="+authAdminToken, false)
			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302", rec.Code)
			}
			assertFrozenCookieFlags(t, sessionCookieOf(t, rec), config.EnvProd)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}

// TestAdminLoginRejections covers the refusal paths of the exchange; case
// names are the assertions.
func TestAdminLoginRejections(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"wrong token 403", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			rec := authGet(t, s, "/admin/login?token=definitely-not-the-token", false)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body:\n%s", rec.Code, rec.Body.String())
			}
			// A refusal must not mint a session or set any cookie.
			res := rec.Result()
			defer func() { _ = res.Body.Close() }()
			if n := len(res.Cookies()); n != 0 {
				t.Errorf("wrong-token 403 set %d cookie(s), want 0", n)
			}
		}},
		{"replayed token 403 from a fresh cookie-less client", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			first := authGet(t, s, "/admin/login?token="+authAdminToken, false)
			if first.Code != http.StatusFound {
				t.Fatalf("first exchange status = %d, want 302", first.Code)
			}
			// A brand-new request with NO cookies: the one-time property is
			// server-side (login_tokens), so a fresh client must not help.
			second := authGet(t, s, "/admin/login?token="+authAdminToken, false)
			if second.Code != http.StatusForbidden {
				t.Fatalf("replay status = %d, want 403; body:\n%s", second.Code, second.Body.String())
			}
			res := second.Result()
			defer func() { _ = res.Body.Close() }()
			if n := len(res.Cookies()); n != 0 {
				t.Errorf("replay 403 set %d cookie(s), want 0", n)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}

// TestAdminLoginTokenEncodings pins the ADMIN_TOKEN encoding reconciliation:
// config.Load stores the DECODED bytes of the base64-provisioned env value,
// while the operator and the A-201 oracle submit the env value verbatim.
// Both strings denote one secret, so both must exchange (and burn the same
// one-time row).
func TestAdminLoginTokenEncodings(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"base64 env-value form of the token is accepted", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			envForm := base64.StdEncoding.EncodeToString([]byte(authAdminToken))
			rec := authGet(t, s, "/admin/login?token="+envForm, false)
			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302; body:\n%s", rec.Code, rec.Body.String())
			}
			if loc := rec.Header().Get("Location"); loc != "/admin" {
				t.Fatalf("Location = %q, want %q", loc, "/admin")
			}
			assertFrozenCookieFlags(t, sessionCookieOf(t, rec), config.EnvTest)
		}},
		{"either accepted encoding burns the same one-time row", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			envForm := base64.StdEncoding.EncodeToString([]byte(authAdminToken))
			// First use via the env-value form.
			first := authGet(t, s, "/admin/login?token="+envForm, false)
			if first.Code != http.StatusFound {
				t.Fatalf("env-form exchange status = %d, want 302", first.Code)
			}
			// The decoded form denotes the same consumed secret: refused.
			decoded := authGet(t, s, "/admin/login?token="+authAdminToken, false)
			if decoded.Code != http.StatusForbidden {
				t.Fatalf("decoded-form after consume: status = %d, want 403", decoded.Code)
			}
			// The env-value replay: refused too.
			replay := authGet(t, s, "/admin/login?token="+envForm, false)
			if replay.Code != http.StatusForbidden {
				t.Fatalf("env-form replay: status = %d, want 403", replay.Code)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}

// TestAdminLoginForm covers the form page and the CSRF-verified POST
// variant of the exchange; case names are the assertions.
func TestAdminLoginForm(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"no or blank token renders the frozen form", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			for _, target := range []string{"/admin/login", "/admin/login?token="} {
				rec := authGet(t, s, target, false)
				if rec.Code != http.StatusOK {
					t.Errorf("GET %s: status = %d, want 200", target, rec.Code)
				}
				assertLoginMarkup(t, rec.Body.String())
			}
		}},
		{"valid POST exchange with matching csrf", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			ct := mustCSRFToken(t)
			form := url.Values{"token": {authAdminToken}, CSRFTokenName: {ct}}
			rec := authPostForm(t, s, "/admin/login", ct, form)
			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302; body:\n%s", rec.Code, rec.Body.String())
			}
			if loc := rec.Header().Get("Location"); loc != "/admin" {
				t.Fatalf("Location = %q, want %q", loc, "/admin")
			}
			assertFrozenCookieFlags(t, sessionCookieOf(t, rec), config.EnvTest)
		}},
		{"POST with mismatched csrf 403", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			form := url.Values{"token": {authAdminToken}, CSRFTokenName: {mustCSRFToken(t)}}
			rec := authPostForm(t, s, "/admin/login", mustCSRFToken(t), form)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body:\n%s", rec.Code, rec.Body.String())
			}
		}},
		{"POST without csrf 403", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			form := url.Values{"token": {authAdminToken}}
			rec := authPostForm(t, s, "/admin/login", "", form)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body:\n%s", rec.Code, rec.Body.String())
			}
		}},
		{"POST blank token renders the form again", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			ct := mustCSRFToken(t)
			form := url.Values{"token": {""}, CSRFTokenName: {ct}}
			rec := authPostForm(t, s, "/admin/login", ct, form)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (the form page); body:\n%s", rec.Code, rec.Body.String())
			}
			assertLoginMarkup(t, rec.Body.String())
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}
