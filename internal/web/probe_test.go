package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/amscotti/groveland-lights/internal/config"
	"github.com/amscotti/groveland-lights/internal/store"
)

// probeReCount pins the frozen fragment markup: the digits of the counter
// sit alone inside #probe-count (`<p>Count: <span id="probe-count">N</span>
// </p>`). The count is read from the span, not from a "Count: N" text node —
// the span sits between the prefix and the digits in the HTML source.
var probeReCount = regexp.MustCompile(`<span id="probe-count">(\d+)</span>`)

// newProbeServer builds a Server whose probe handlers run against a real
// store (store.Open on a temp-file SQLite DB, migrations included) wired
// through the same SetProbeStore call main uses. Cleanup closes the store
// and unsets the package-level wiring so rows stay independent.
func newProbeServer(t *testing.T) *Server {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	SetProbeStore(db)
	t.Cleanup(func() { SetProbeStore(nil) })
	return newTestServer(t, config.EnvTest)
}

// probeMint GETs /probe through the full middleware chain once: it returns
// the response body, the minted CSRF cookie (the jar's future value) and the
// count shown. Every caller asserts on THIS response, so the minted token
// and the rendered fragment cannot diverge.
func probeMint(t *testing.T, s *Server) (string, *http.Cookie, int64) {
	t.Helper()
	rec := s.probeGet(t)
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()

	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == CSRFTokenName {
			cookie = c
		}
	}
	if cookie == nil || cookie.Value == "" {
		t.Fatalf("GET /probe minted no %s cookie; headers: %v", CSRFTokenName, rec.Header())
	}
	m := probeReCount.FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatalf("GET /probe fragment shows no count in #probe-count; body:\n%s", rec.Body.String())
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		t.Fatalf("GET /probe count %q: %v", m[1], err)
	}
	return rec.Body.String(), cookie, n
}

// probeGet performs GET /probe through the full chain and asserts 200.
func (s *Server) probeGet(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/probe", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /probe: status %d, want 200; body:\n%s", rec.Code, rec.Body.String())
	}
	return rec
}

// probePostForm POSTs to /probe through the full chain. form == nil sends an
// empty body with no content type (the "missing token" shape); hx decides
// whether the HTMX HX-Request header rides along (the no-JS path omits it).
func probePostForm(t *testing.T, s *Server, cookie *http.Cookie, form url.Values, hx bool) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/probe", body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// probeCountNow reads the counter straight from the wired store.
func probeCountNow(t *testing.T) int64 {
	t.Helper()
	if probeStore == nil {
		t.Fatal("probe store not wired")
	}
	n, err := probeStore.ProbeCount(context.Background())
	if err != nil {
		t.Fatalf("ProbeCount: %v", err)
	}
	return n
}

// probeTamper returns a token of the same shape with a wrong value (first
// character flipped) — the tampered-submission row of the contract.
func probeTamper(token string) string {
	if token == "" {
		return "tampered"
	}
	b := []byte(token)
	if b[0] == 'A' {
		b[0] = 'B'
	} else {
		b[0] = 'A'
	}
	return string(b)
}

func TestProbeGet(t *testing.T) {
	t.Run("renders the contract fragment", func(t *testing.T) {
		s := newProbeServer(t)
		body, cookie, n := probeMint(t, s)

		// Frozen fragment contract: h2 heading exactly "Probe"; the count
		// line with the digits alone in #probe-count; the form with BOTH
		// transports — the no-JS plain action/method and the HTMX attributes
		// — the hidden csrf_token field carrying the minted token, and the
		// submit button with its exact accessible name.
		for _, want := range []string{
			"<h2>Probe</h2>",
			`<p>Count: <span id="probe-count">` + strconv.FormatInt(n, 10) + `</span></p>`,
			`<form action="/probe" method="post"`,
			`hx-post="/probe"`,
			`hx-target="#probe"`,
			`hx-swap="innerHTML"`,
			`<input type="hidden" name="csrf_token" value="` + cookie.Value + `">`,
			`<button type="submit">Increment counter</button>`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("fragment missing %q; body:\n%s", want, body)
			}
		}
		if m := probeReCount.FindStringSubmatch(body); m == nil {
			t.Errorf("#probe-count does not hold the bare digits; body:\n%s", body)
		}
	})

	t.Run(`fragment has no html/body and no id="map"`, func(t *testing.T) {
		s := newProbeServer(t)
		body := s.probeGet(t).Body.String()
		// The fragment is swapped INTO the home page: a full document here
		// would nest html/body tags, and id="map" would collide with the
		// map container the HTMX contract forbids swapping.
		for _, banned := range []string{"<html", "<body", `id="map"`} {
			if strings.Contains(body, banned) {
				t.Errorf("fragment contains %q; body:\n%s", banned, body)
			}
		}
		if !strings.Contains(body, "<h2>Probe</h2>") {
			t.Errorf("fragment is not the probe partial; body:\n%s", body)
		}
	})

	t.Run("store unset answers 500", func(t *testing.T) {
		s := newProbeServer(t)
		SetProbeStore(nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/probe", nil))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("GET /probe with store unset: status %d, want 500", rec.Code)
		}
	})

	t.Run("store failure answers 500", func(t *testing.T) {
		s := newProbeServer(t)
		if err := probeStore.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/probe", nil))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("GET /probe with closed store: status %d, want 500", rec.Code)
		}
	})
}

func TestProbePost(t *testing.T) {
	t.Run("valid token increments", func(t *testing.T) {
		s := newProbeServer(t)
		_, cookie, n := probeMint(t, s)

		form := url.Values{CSRFTokenName: {cookie.Value}}
		rec := probePostForm(t, s, cookie, form, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /probe with valid token: status %d, want 200; body:\n%s", rec.Code, rec.Body.String())
		}
		m := probeReCount.FindStringSubmatch(rec.Body.String())
		if m == nil {
			t.Fatalf("POST /probe fragment has no \"Count: N\" text; body:\n%s", rec.Body.String())
		}
		got, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || got != n+1 {
			t.Fatalf("POST /probe fragment count = %v, want %d; body:\n%s", m[1], n+1, rec.Body.String())
		}
		if got := probeCountNow(t); got != n+1 {
			t.Fatalf("store count after valid POST = %d, want %d", got, n+1)
		}
	})

	t.Run("missing token 403 leaves count unchanged", func(t *testing.T) {
		s := newProbeServer(t)
		_, cookie, n := probeMint(t, s)

		// Empty body: no csrf_token field at all (HTMX-shaped header, as the
		// contract's rejected flow describes).
		rec := probePostForm(t, s, cookie, nil, true)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("POST /probe without csrf_token: status %d, want 403; body:\n%s", rec.Code, rec.Body.String())
		}
		if got := probeCountNow(t); got != n {
			t.Fatalf("count changed after rejected POST: %d, want %d", got, n)
		}
	})

	t.Run("tampered token 403", func(t *testing.T) {
		s := newProbeServer(t)
		_, cookie, n := probeMint(t, s)

		form := url.Values{CSRFTokenName: {probeTamper(cookie.Value)}}
		rec := probePostForm(t, s, cookie, form, true)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("POST /probe with tampered csrf_token: status %d, want 403; body:\n%s", rec.Code, rec.Body.String())
		}
		if got := probeCountNow(t); got != n {
			t.Fatalf("count changed after tampered POST: %d, want %d", got, n)
		}
	})

	t.Run("non-HTMX post redirects 303 to /", func(t *testing.T) {
		s := newProbeServer(t)
		_, cookie, n := probeMint(t, s)

		// The no-JS submit: valid token, no HX-Request header. The counter
		// increments first, then the browser is sent home to see it.
		form := url.Values{CSRFTokenName: {cookie.Value}}
		rec := probePostForm(t, s, cookie, form, false)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("POST /probe without HX-Request: status %d, want 303; body:\n%s", rec.Code, rec.Body.String())
		}
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Fatalf("POST /probe without HX-Request: Location = %q, want \"/\"", loc)
		}
		if got := probeCountNow(t); got != n+1 {
			t.Fatalf("count after no-JS POST = %d, want %d", got, n+1)
		}
	})

	t.Run("unparseable body 400", func(t *testing.T) {
		s := newProbeServer(t)
		_, cookie, n := probeMint(t, s)

		// An invalid media type is the one POST shape ParseForm itself
		// rejects (multipart bodies parse lazily and defer to
		// ParseMultipartForm), so it is the 400 path of the boundary parser.
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/probe", strings.NewReader("csrf_token=x"))
		req.Header.Set("Content-Type", "broken;;;===")
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("POST /probe with unparseable body: status %d, want 400", rec.Code)
		}
		if got := probeCountNow(t); got != n {
			t.Fatalf("count changed after unparseable POST: %d, want %d", got, n)
		}
	})
}

// TestProbeHomeMount pins the home page's probe mount: the empty shell
// section with the HTMX load attributes and the plain link no-JS visitors
// follow to reach the fragment's form.
func TestProbeHomeMount(t *testing.T) {
	s := newProbeServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<section id="probe"`,
		`hx-get="/probe"`,
		`hx-trigger="load"`,
		`hx-target="#probe"`,
		`hx-swap="innerHTML"`,
		`<a href="/probe">Probe</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET / missing probe mount markup %q; body:\n%s", want, body)
		}
	}
}
