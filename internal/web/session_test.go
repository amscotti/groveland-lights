package web

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/amscotti/groveland-lights/internal/config"
	"github.com/amscotti/groveland-lights/internal/store"
)

// guardedStub is the handler AdminOnly protects in these tests; it writes a
// marker body so every case can tell "passed through" from "refused".
func guardedStub(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("admin-ok"))
}

// guardedGet builds a GET /admin request carrying the given cookie value
// ("" = no cookie) and optionally the HTMX header, then runs the guard.
func guardedGet(s *Server, cookieValue string, hx bool) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/admin", nil)
	if cookieValue != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: cookieValue})
	}
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	AdminOnly(guardedStub)(rec, req.WithContext(context.WithValue(req.Context(), ctxServer, s)))
	return rec
}

// adminCookie exchanges the test ADMIN_TOKEN through the full chain and
// returns the minted gl_session cookie (and the session id it carries).
func adminCookie(t *testing.T, s *Server) (*http.Cookie, string) {
	t.Helper()
	rec := authGet(t, s, "/admin/login?token="+authAdminToken, false)
	if rec.Code != http.StatusFound {
		t.Fatalf("token exchange status = %d, want 302; body:\n%s", rec.Code, rec.Body.String())
	}
	c := sessionCookieOf(t, rec)
	id, ok := sessionIDOf(authSessionKey, c.Value)
	if !ok {
		t.Fatalf("minted cookie value %q does not verify", c.Value)
	}
	return c, id
}

// TestAdminOnlyGuard covers the guard as a table of flows; case names are
// the assertions.
func TestAdminOnlyGuard(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"anon full page redirected to login", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			assertRedirectToLogin(t, guardedGet(s, "", false))
		}},
		{"anon HTMX gets 401 plus HX-Redirect", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			rec := guardedGet(s, "", true)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body:\n%s", rec.Code, rec.Body.String())
			}
			if loc := rec.Header().Get("HX-Redirect"); loc != "/admin/login" {
				t.Fatalf("HX-Redirect = %q, want %q", loc, "/admin/login")
			}
			if rec.Body.Len() != 0 {
				t.Fatalf("401 body = %q, want empty", rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "admin-ok") {
				t.Fatal("guarded handler ran despite the refusal")
			}
		}},
		{"admin cookie passes AdminOnly", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			c, _ := adminCookie(t, s)
			rec := guardedGet(s, c.Value, false)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body:\n%s", rec.Code, rec.Body.String())
			}
			if rec.Body.String() != "admin-ok" {
				t.Fatalf("guarded handler did not run; body = %q", rec.Body.String())
			}
		}},
		{"admin HTMX with a valid cookie passes too", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			c, _ := adminCookie(t, s)
			rec := guardedGet(s, c.Value, true)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body:\n%s", rec.Code, rec.Body.String())
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}

// TestAdminOnlyGuardRefusals covers every rejection path of the guard; case
// names are the assertions.
func TestAdminOnlyGuardRefusals(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"tampered cookie signature rejected", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			c, _ := adminCookie(t, s)
			tampered := c.Value[:len(c.Value)-1]
			if c.Value[len(c.Value)-1] == 'a' {
				tampered += "b"
			} else {
				tampered += "a"
			}
			assertRedirectToLogin(t, guardedGet(s, tampered, false))
		}},
		{"expired session rejected (injected clock)", func(t *testing.T) {
			s, db := newAuthServer(t, config.EnvTest)
			c, id := adminCookie(t, s)
			// Push the auth slice's clock past the session's 24 h lifetime;
			// restored on cleanup so no other test sees a shifted clock.
			prev := store.Now
			store.Now = func() time.Time { return prev().Add(25 * time.Hour) }
			t.Cleanup(func() { store.Now = prev })

			assertRedirectToLogin(t, guardedGet(s, c.Value, false))
			// The expired row was swept: no row answers for the id any more
			// (missing sessions carry a nil error, unlike expired ones).
			_, _, ok, err := db.SessionValid(context.Background(), id)
			if ok || err != nil {
				t.Fatalf("expired session row survived: ok=%v err=%v", ok, err)
			}
		}},
		{"user-role session rejected", func(t *testing.T) {
			s, db := newAuthServer(t, config.EnvTest)
			ctx := context.Background()
			uid, err := db.EnsureAdminUser(ctx)
			if err != nil {
				t.Fatalf("EnsureAdminUser: %v", err)
			}
			sid, err := db.CreateSession(ctx, uid, "user")
			if err != nil {
				t.Fatalf("CreateSession: %v", err)
			}
			assertRedirectToLogin(t, guardedGet(s, sessionValue(authSessionKey, sid), false))
		}},
		{"cookie signed with another key rejected", func(t *testing.T) {
			s, db := newAuthServer(t, config.EnvTest)
			ctx := context.Background()
			uid, err := db.EnsureAdminUser(ctx)
			if err != nil {
				t.Fatalf("EnsureAdminUser: %v", err)
			}
			sid, err := db.CreateSession(ctx, uid, "admin")
			if err != nil {
				t.Fatalf("CreateSession: %v", err)
			}
			otherKey := []byte("99999999999999999999999999999999")
			assertRedirectToLogin(t, guardedGet(s, sessionValue(otherKey, sid), false))
		}},
		{"structurally invalid cookie rejected", func(t *testing.T) {
			s, _ := newAuthServer(t, config.EnvTest)
			assertRedirectToLogin(t, guardedGet(s, "garbage", false))
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}

// TestSessionValueRoundTrip pins the cookie codec directly: the exact
// structure the guard parses.
func TestSessionValueRoundTrip(t *testing.T) {
	id := "MgisOMh2YRG2S9YRShtazoR-Ci1Q8WfJmmvKlj7C_5g"
	value := sessionValue(authSessionKey, id)
	encoded, sig, found := strings.Cut(value, ".")
	// The frozen format is base64url(id) + "." + hex(HMAC-SHA256(key, id)):
	// the id is encoded AGAIN for the cookie, and the codec must invert it.
	if !found || encoded != base64.RawURLEncoding.EncodeToString([]byte(id)) {
		t.Fatalf("sessionValue = %q, want <base64url(id)>.<signature>", value)
	}
	if len(sig) != 64 {
		t.Fatalf("signature = %q (%d chars), want 64 hex chars", sig, len(sig))
	}
	got, ok := sessionIDOf(authSessionKey, value)
	if !ok || got != id {
		t.Fatalf("sessionIDOf = (%q, %v), want (%q, true)", got, ok, id)
	}

	tests := []struct {
		name  string
		value string
	}{
		{"missing dot rejected", id},
		{"undecodable id part rejected", "###.deadbeef"},
		{"empty id part rejected", ".deadbeef"},
		{"wrong signature rejected", id + ".00000000000000000000000000000000"},
		{"signature part missing rejected", id + "."},
		{"empty value rejected", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := sessionIDOf(authSessionKey, tc.value); ok {
				t.Fatalf("sessionIDOf accepted %q as %q", tc.value, got)
			}
		})
	}
}
