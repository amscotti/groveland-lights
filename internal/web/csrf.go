package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
)

// Double-submit CSRF primitives. The token lives only in a cookie and a
// matching form field — no server-side session state — so every render can
// mint or reuse one statelessly and every POST can verify it. T-103's /probe
// handler consumes this API: render time calls IssueCSRFToken, POST time
// calls VerifyCSRF and answers 403 on ErrCSRF.

const (
	// CSRFTokenName is the cookie name and the hidden-input name of the
	// double-submit token; the acceptance contract freezes "csrf_token".
	CSRFTokenName = "csrf_token"

	// csrfTokenBytes is the entropy floor for issued tokens: 32 crypto/rand
	// bytes, base64url-encoded without padding (43 characters).
	csrfTokenBytes = 32

	// csrfMaxLen bounds token parsing so a hostile multi-megabyte field or
	// cookie cannot cost decode work; valid tokens are 43 characters.
	csrfMaxLen = 256
)

// ErrCSRF reports a missing, malformed or mismatched CSRF token. It is
// deliberately uniform: callers answer 403 without learning which check
// failed.
var ErrCSRF = errors.New("csrf: token mismatch")

// IssueCSRFToken returns the CSRF token for this request and makes sure the
// CSRFTokenName cookie carries it. A well-formed cookie on the request is
// reused as-is and no new cookie is sent (idempotent per session-less phase:
// repeated renders keep one token per browser); otherwise 32 fresh
// crypto/rand bytes are minted as base64url and set as a cookie with Path=/,
// HttpOnly, SameSite=Lax and Secure in prod. It must be called from a handler
// inside the server chain (the context carries the config, like every
// handler).
func IssueCSRFToken(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(CSRFTokenName); err == nil && validCSRFToken(c.Value) {
		return c.Value
	}
	token, err := newCSRFToken()
	if err != nil {
		// Without entropy no safe token exists; do not render a form.
		http.Error(w, "internal error", http.StatusInternalServerError)
		return ""
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure comes from cfg.IsProd(); dev/test intentionally serve without TLS
		Name:     CSRFTokenName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   serverFrom(r.Context()).Cfg.IsProd(),
	})
	return token
}

// VerifyCSRF reports whether the submitted form value matches the request's
// CSRFTokenName cookie. Missing cookie, empty or non-base64url submission,
// an under-length token and a constant-time-compare mismatch all yield
// ErrCSRF.
func VerifyCSRF(r *http.Request, submitted string) error {
	c, err := r.Cookie(CSRFTokenName)
	if err != nil || !validCSRFToken(c.Value) || !validCSRFToken(submitted) {
		return ErrCSRF
	}
	if subtle.ConstantTimeCompare([]byte(c.Value), []byte(submitted)) != 1 {
		return ErrCSRF
	}
	return nil
}

// newCSRFToken mints 32 crypto/rand bytes as unpadded base64url.
func newCSRFToken() (string, error) {
	b := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// validCSRFToken accepts exactly what newCSRFToken emits (or longer): strict
// base64url decoding to at least csrfTokenBytes of entropy, within the
// length bound.
func validCSRFToken(tok string) bool {
	if tok == "" || len(tok) > csrfMaxLen {
		return false
	}
	b, err := base64.RawURLEncoding.DecodeString(tok)
	return err == nil && len(b) >= csrfTokenBytes
}
