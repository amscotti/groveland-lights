package web

// Admin login: one-time ADMIN_TOKEN exchange for an admin session (A-201).
// The frozen flow (acceptance/drafts/admin_auth_go_test.go.txt):
//
//	GET  /admin/login                  -> 200 form page (no/blank token)
//	GET  /admin/login?token=<wrong>    -> 403
//	GET  /admin/login?token=<valid>    -> 302 /admin + gl_session cookie
//	GET  (replay, any client)          -> 403 (consumption is server-side)
//	POST /admin/login                  -> the same exchange, CSRF-verified
//
// auth.go owns the routes and the exchange; session.go owns the cookie and
// the AdminOnly guard. The page below is the minimal contract markup only;
// the template pipeline render is T-205's (pages/admin-login.html, replacing
// this inline page WITHOUT changing the frozen strings).

import (
	"bytes"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"

	"github.com/amscotti/groveland-lights/internal/store"
)

func init() {
	register(route{pattern: "GET /admin/login", handler: adminLoginGet})
	register(route{pattern: "POST /admin/login", handler: adminLoginPost})
}

// loginForm is one login request parsed at the boundary; the handlers below
// consult only these typed fields and never touch the form or query again.
type loginForm struct {
	Token     string // the submitted (or queried) admin token
	CSRFToken string // POST only: the double-submit csrf_token field
}

// parseLoginGet lifts the GET query into a loginForm.
func parseLoginGet(r *http.Request) loginForm {
	return loginForm{Token: r.URL.Query().Get("token")}
}

// parseLoginPost parses the POST form exactly once and lifts it into a
// loginForm.
func parseLoginPost(r *http.Request) (loginForm, error) {
	if err := r.ParseForm(); err != nil {
		return loginForm{}, err
	}
	return loginForm{
		Token:     r.PostForm.Get("token"),
		CSRFToken: r.PostForm.Get(CSRFTokenName),
	}, nil
}

func adminLoginGet(w http.ResponseWriter, r *http.Request) {
	s := serverFrom(r.Context())
	s.adminLogin(w, r, parseLoginGet(r))
}

func adminLoginPost(w http.ResponseWriter, r *http.Request) {
	s := serverFrom(r.Context())
	form, err := parseLoginPost(r)
	if err != nil {
		s.Log.Error("admin login: parse", "err", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := VerifyCSRF(r, form.CSRFToken); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.adminLogin(w, r, form)
}

// tokenMatches reports whether the submitted token is the configured admin
// credential in either of its two operator-facing forms. ADMIN_TOKEN is
// provisioned as base64 (docs/PLAN.md env table), and config.Load stores the
// DECODED bytes in Cfg.AdminToken — while the operator and the frozen A-201
// flow present the env value verbatim, i.e. the base64 encoding of exactly
// those bytes. Both strings therefore denote one and the same secret, so
// both encodings are accepted; that widens nothing (anyone who can compute
// the encoding already knows the secret). Both comparisons run
// unconditionally and constant-time.
func tokenMatches(submitted string, admin []byte) bool {
	encoded := base64.StdEncoding.EncodeToString(admin)
	mDecoded := subtle.ConstantTimeCompare([]byte(submitted), admin)
	mEncoded := subtle.ConstantTimeCompare([]byte(submitted), []byte(encoded))
	return mDecoded == 1 || mEncoded == 1
}

// adminLogin runs the shared exchange for a parsed form. A blank token
// renders the form page (the contract's no/blank -> 200); a wrong token is a
// uniform 403; a valid one is consumed globally and exchanged for an admin
// session cookie with a 302 to /admin. The token value is never logged.
func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request, form loginForm) {
	if form.Token == "" {
		s.renderLoginPage(w)
		return
	}
	if s.Store == nil {
		s.Log.Error("admin login: store not wired")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// Uniform constant-time comparison against the configured ADMIN_TOKEN
	// (in either operator-facing encoding; see tokenMatches). On success the
	// canonical config bytes are what gets consumed below, so every accepted
	// encoding burns the same one-time row.
	if !tokenMatches(form.Token, s.Cfg.AdminToken) {
		s.Log.Info("admin login: token rejected")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := s.Store.ConsumeAdminToken(r.Context(), s.Cfg.AdminToken); err != nil {
		if errors.Is(err, store.ErrTokenReused) {
			s.Log.Info("admin login: token replay refused")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		s.Log.Error("admin login: consume token", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	uid, err := s.Store.EnsureAdminUser(r.Context())
	if err != nil {
		s.Log.Error("admin login: ensure admin user", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	sid, err := s.Store.CreateSession(r.Context(), uid, "admin")
	if err != nil {
		s.Log.Error("admin login: create session", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setSessionCookie(s.Cfg, w, sid)
	s.Log.Info("admin login: session issued", "user_id", int64(uid))
	http.Redirect(w, r, "/admin", http.StatusFound)
}

// loginPageTmpl is the minimal login page: exactly the frozen contract
// markup (heading "Admin login", label "Token" over input name "token",
// button "Sign in"). T-205 replaces it with pages/admin-login.html through
// the template pipeline, keeping these strings byte-for-byte.
var loginPageTmpl = template.Must(template.New("admin-login").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Admin login — Groveland Lights</title>
</head>
<body>
<main>
<h1>Admin login</h1>
<form method="get" action="/admin/login">
<label for="token">Token</label>
<input id="token" name="token" type="password" autocomplete="off" required>
<button type="submit">Sign in</button>
</form>
</main>
</body>
</html>
`))

// renderLoginPage answers 200 with the frozen login form. The template is
// static (no data), so it is rendered into a buffer: a failure can never
// half-write the response.
func (s *Server) renderLoginPage(w http.ResponseWriter) {
	var buf bytes.Buffer
	if err := loginPageTmpl.Execute(&buf, nil); err != nil {
		s.Log.Error("admin login: render", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}
