package web

// Admin session cookie and guard (A-201). auth.go exchanges the one-time
// ADMIN_TOKEN for a session; this file owns the gl_session cookie format and
// the AdminOnly wrapper that T-205 puts around every /admin route. The
// cookie value is base64url(session id) + "." + hex(HMAC-SHA256(SessionKey,
// id)): the HMAC makes the value unforgeable, so a cookie alone never
// unlocks anything without its live sessions row.

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/amscotti/groveland-lights/internal/config"
	"github.com/amscotti/groveland-lights/internal/store"
)

// SessionCookieName is the admin session cookie, frozen by the A-201
// contract ("gl_session").
const SessionCookieName = "gl_session"

// sessionCookieTTL is the cookie Max-Age and matches the store-side session
// lifetime: the contract freezes Max-Age=86400 (24 h).
const sessionCookieTTL = 24 * time.Hour

// sessionValue encodes id for the cookie: base64url(id) + "." +
// hex(HMAC-SHA256(key, id)).
func sessionValue(key []byte, id string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(id))
	return base64.RawURLEncoding.EncodeToString([]byte(id)) + "." + hex.EncodeToString(mac.Sum(nil))
}

// sessionIDOf verifies a sessionValue-encoded cookie and returns the session
// id it carries. Any structural defect or signature mismatch (compared
// constant-time) yields ok=false.
func sessionIDOf(key []byte, value string) (id string, ok bool) {
	encoded, sig, found := strings.Cut(value, ".")
	if !found {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) == 0 {
		return "", false
	}
	id = string(raw)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(id))
	if subtle.ConstantTimeCompare([]byte(sig), []byte(hex.EncodeToString(mac.Sum(nil)))) != 1 {
		return "", false
	}
	return id, true
}

// setSessionCookie sets the admin session cookie for id on the response:
// Path=/, HttpOnly, SameSite=Lax, Max-Age 86400, Secure exactly in prod
// (frozen attribute set; the A-201 draft parses and compares them exactly).
func setSessionCookie(cfg config.Config, w http.ResponseWriter, id string) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure comes from cfg.IsProd(); dev/test intentionally serve without TLS (same door as csrf.go)
		Name:     SessionCookieName,
		Value:    sessionValue(cfg.SessionKey, id),
		Path:     "/",
		MaxAge:   int(sessionCookieTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cfg.IsProd(),
	})
}

// AdminOnly guards an admin handler: a request carrying a cookie that
// verifies against a live, unexpired, admin-role session passes through;
// everyone else is bounced to the login page — a 302 for full-page
// navigations, 401 with HX-Redirect: /admin/login and an empty body for
// HTMX fragment requests (frozen contract A-201). T-205 wraps every /admin
// route with this.
func AdminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := serverFrom(r.Context())
		if !s.validAdminSession(r) {
			if r.Header.Get("HX-Request") != "" {
				w.Header().Set("HX-Redirect", "/admin/login")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/admin/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

// validAdminSession verifies the gl_session cookie end to end: structure and
// signature, live store row, expiry and admin role. Any failure refuses; the
// reason goes to the log, never to the response — and neither the token nor
// any credential value is ever logged.
func (s *Server) validAdminSession(r *http.Request) bool {
	c, err := r.Cookie(SessionCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	id, ok := sessionIDOf(s.Cfg.SessionKey, c.Value)
	if !ok {
		s.Log.Info("admin session: cookie signature rejected")
		return false
	}
	if s.Store == nil {
		s.Log.Error("admin session: store not wired")
		return false
	}
	_, role, valid, err := s.Store.SessionValid(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrBadCredentials) {
			// Expired: best-effort cleanup of the stale row so expired
			// sessions do not accumulate, then the ordinary refusal path.
			if derr := s.Store.DeleteSession(r.Context(), id); derr != nil {
				s.Log.Error("admin session: expire cleanup", "err", derr)
			}
			s.Log.Info("admin session: expired session refused")
			return false
		}
		s.Log.Error("admin session: lookup failed", "err", err)
		return false // fail closed
	}
	if !valid {
		return false
	}
	return role == "admin"
}
