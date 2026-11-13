// Probe feature: a global, SQLite-backed counter exposed as an HTMX fragment
// at /probe and mounted on the home page inside <section id="probe">. It is
// the M1 vertical slice through the whole stack: the store opened at boot,
// double-submit CSRF, an HTMX partial and a no-JS form fallback (303 back to
// the home page).
//
// The store reaches these handlers through Server.Store (wired by main via
// web.New); while it is nil — only possible in tests that never open a store
// — every probe route answers 500 (a wiring bug, logged as such).
package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
)

const probeContentType = "text/html; charset=utf-8"

// errProbeStoreNotWired reports a probe request served with a nil Server.Store.
var errProbeStoreNotWired = errors.New("probe: store not wired")

func init() {
	register(route{pattern: "GET /probe", handler: probeGet})
	register(route{pattern: "POST /probe", handler: probePost})
}

// probeData renders partials/probe.html: the current counter value and the
// CSRF token the increment form submits.
type probeData struct {
	Count     int64
	CSRFToken string
}

// renderPartial executes a bare partial (no base.html wrapper) — the HTMX
// fragment contract depends on it. It mirrors renderPage for the partials
// map; like renderPage it buffers, so a failing template never writes a
// half-rendered fragment to the response.
func (t *templates) renderPartial(w io.Writer, name string, data any) error {
	tpl, ok := t.partials[filepath.ToSlash(name)]
	if !ok {
		return fmt.Errorf("unknown partial %s", name)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// probeCount reads the counter through Server.Store, collapsing the nil
// store into an error so handlers keep a single failure path.
func (s *Server) probeCount(ctx context.Context) (int64, error) {
	if s.Store == nil {
		return 0, errProbeStoreNotWired
	}
	return s.Store.ProbeCount(ctx)
}

// probeInc increments the counter through Server.Store, collapsing the nil
// store into an error so handlers keep a single failure path.
func (s *Server) probeInc(ctx context.Context) (int64, error) {
	if s.Store == nil {
		return 0, errProbeStoreNotWired
	}
	return s.Store.ProbeInc(ctx)
}

// probeGet renders the probe fragment: heading, current count and the
// CSRF-protected increment form. HTMX fetches it (hx-trigger="load" on the
// home mount, hx-post on the form) and swaps it into <section id="probe">.
func probeGet(w http.ResponseWriter, r *http.Request) {
	s := serverFrom(r.Context())
	n, err := s.probeCount(r.Context())
	if err != nil {
		s.Log.Error("probe: count", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	token := IssueCSRFToken(w, r)
	if token == "" {
		return // IssueCSRFToken could not mint a token and answered 500.
	}
	data := probeData{Count: n, CSRFToken: token}
	// The fragment is a full HTML document for a no-JS visitor who navigates
	// here directly; with the global nosniff header it must not be left to
	// sniffing (which reads it as text/plain and shows escaped source).
	w.Header().Set("Content-Type", probeContentType)
	if err := s.tpl.renderPartial(w, "partials/probe.html", data); err != nil {
		s.Log.Error("probe: render", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// probeSubmission is a POST /probe body parsed once, at the boundary. After
// parseProbePost returns, handlers consult only these typed fields.
type probeSubmission struct {
	CSRFToken string // submitted csrf_token form value
	HTMX      bool   // HX-Request header present (HTMX flow) or absent (no-JS form submit)
}

// parseProbePost parses the request form exactly once and lifts it into a
// typed struct; it is the only code in this feature that touches the form.
func parseProbePost(r *http.Request) (probeSubmission, error) {
	if err := r.ParseForm(); err != nil {
		return probeSubmission{}, err
	}
	return probeSubmission{
		CSRFToken: r.PostForm.Get(CSRFTokenName),
		HTMX:      r.Header.Get("HX-Request") != "",
	}, nil
}

// probePost verifies the CSRF pair, increments the counter and answers with
// the re-rendered fragment (HTMX) or a 303 back to the home page (no-JS
// submit). Rejected tokens leave the counter untouched.
func probePost(w http.ResponseWriter, r *http.Request) {
	s := serverFrom(r.Context())

	form, err := parseProbePost(r)
	if err != nil {
		s.Log.Error("probe: parse", "err", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Uniform rejection per the CSRF contract: missing, malformed and
	// mismatched tokens are indistinguishable. No increment happens here.
	if err := VerifyCSRF(r, form.CSRFToken); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	n, err := s.probeInc(r.Context())
	if err != nil {
		s.Log.Error("probe: inc", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if !form.HTMX {
		// No-JS submit: the counter is already incremented; send the browser
		// back to the home page, which shows the new count on its next render.
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	token := IssueCSRFToken(w, r)
	if token == "" {
		return // IssueCSRFToken could not mint a token and answered 500.
	}
	data := probeData{Count: n, CSRFToken: token}
	// Same nosniff reasoning as probeGet: the swapped fragment must declare
	// itself HTML, not rely on sniffing.
	w.Header().Set("Content-Type", probeContentType)
	if err := s.tpl.renderPartial(w, "partials/probe.html", data); err != nil {
		s.Log.Error("probe: render", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// probeFragment renders the probe partial server-side for the home page mount
// so JavaScript-disabled clients see the live counter and the no-JS form
// (contract A-103). JS clients get the same markup refreshed by htmx.
func probeFragment(w http.ResponseWriter, r *http.Request) (template.HTML, error) { //nolint:forbidigo // §8.4 audited door: the ONLY template.HTML return type, for server-rendered fragments
	s := serverFrom(r.Context())
	n, err := s.probeCount(r.Context())
	if err != nil {
		return "", err
	}
	token := IssueCSRFToken(w, r)
	var buf bytes.Buffer
	if err := s.tpl.renderPartial(&buf, "partials/probe.html", probeData{Count: n, CSRFToken: token}); err != nil {
		return "", err
	}
	//nolint:gosec // §8.4 audited: buf holds only our own template output; all user data is escaped upstream by html/template
	return template.HTML(buf.String()), nil //nolint:forbidigo // §8.4 audited door: sole conversion, same rationale as above
}
