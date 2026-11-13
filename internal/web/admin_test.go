package web

// Admin UI handler tests (A-202/A-203/A-208/A-209). Every route is exercised
// through the full middleware chain against a real temp-file store (the same
// wiring as production), with a minted admin session cookie and matching
// double-submit CSRF pair. Assertions mirror the frozen contract in
// acceptance/drafts (status codes, frozen strings, the #rows fragment root,
// the 303s without HX-Request).

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/amscotti/groveland-lights/internal/config"
	"github.com/amscotti/groveland-lights/internal/store"
)

// adminTownBBox mirrors the frozen TOWN_BBOX the drafts use.
var adminTownBBox = config.BBox{MinLat: 42.70, MinLon: -71.09, MaxLat: 42.82, MaxLon: -70.96}

// newAdminServer builds a Server on a real temp DB with an admin session
// already minted; it returns the server and the session cookie value.
func newAdminServer(t *testing.T) (*Server, string) {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "admin.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg := testConfig(config.EnvTest)
	cfg.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	cfg.TownBBox = adminTownBBox
	cfg.PhotoDir = filepath.Join(t.TempDir(), "photos")
	s, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	uid, err := db.EnsureAdminUser(context.Background())
	if err != nil {
		t.Fatalf("EnsureAdminUser: %v", err)
	}
	sid, err := db.CreateSession(context.Background(), uid, "admin")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return s, sessionValue(cfg.SessionKey, sid)
}

// adminDo runs one request through the full chain with the session cookie.
func adminDo(t *testing.T, s *Server, method, target, session string, hx bool, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, target, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	if session != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: session})
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// adminGet is adminDo for GETs (optionally HX).
func adminGet(t *testing.T, s *Server, target, session string, hx bool) *httptest.ResponseRecorder {
	t.Helper()
	return adminDo(t, s, http.MethodGet, target, session, hx, nil, "")
}

// adminPostMultipart builds and runs one multipart POST. csrfCookie ("" for
// none) completes the double-submit pair with the csrf_token form field.
func adminPostMultipart(t *testing.T, s *Server, target string, fields map[string]string, fileField, fileName string, fileData []byte, session, csrfCookie string, hx bool) *httptest.ResponseRecorder {
	t.Helper()
	var body strings.Builder
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("field %s: %v", k, err)
		}
	}
	if fileField != "" {
		fw, err := w.CreateFormFile(fileField, fileName)
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		if _, err := fw.Write(fileData); err != nil {
			t.Fatalf("file write: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, target, strings.NewReader(body.String()))
	req.Header.Set("Content-Type", w.FormDataContentType())
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	if session != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: session})
	}
	if csrfCookie != "" {
		req.AddCookie(&http.Cookie{Name: CSRFTokenName, Value: csrfCookie})
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// adminCSRF mints a fresh CSRF token (cookie value == form value).
func adminCSRF(t *testing.T) string {
	t.Helper()
	return mustCSRFToken(t)
}

// adminFixture reads one of the testdata photo fixtures.
func adminFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "photos", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return b
}

// postCreate is the common create call (label/kind/lat/lon + optional asset);
// the CSRF pair matches.
func postCreate(t *testing.T, s *Server, session, csrf string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	all := map[string]string{CSRFTokenName: csrf}
	for k, v := range fields {
		all[k] = v
	}
	return adminPostMultipart(t, s, "/admin/lights", all, "", "", nil, session, csrf, false)
}

// createLight asserts a 303 create and returns nothing (list assertions follow).
func createLight(t *testing.T, s *Server, session, csrf string, fields map[string]string) {
	t.Helper()
	rec := postCreate(t, s, session, csrf, fields)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %v: status %d, want 303; body:\n%s", fields, rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/admin" {
		t.Fatalf("create Location = %q, want /admin", loc)
	}
}

// reRowsRoot matches a fragment whose ROOT element carries id="rows" (frozen).
var reRowsRoot = regexp.MustCompile(`\A\s*<[a-zA-Z][^>]*\bid="rows"`)

// --- guard ---

func TestAdminRoutesGuard(t *testing.T) {
	tests := []struct {
		name   string
		target string
		hx     bool
		want   int
	}{
		{"anon list redirects to login", "/admin", false, http.StatusFound},
		{"anon walk redirects to login", "/admin/walk", false, http.StatusFound},
		{"anon rows redirects to login", "/admin/rows", false, http.StatusFound},
		{"anon detail redirects to login", "/admin/lights/1", false, http.StatusFound},
		{"anon list HX gets 401", "/admin", true, http.StatusUnauthorized},
		{"anon rows HX gets 401", "/admin/rows", true, http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newAdminServer(t)
			rec := adminGet(t, s, tc.target, "", tc.hx)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d; body:\n%s", rec.Code, tc.want, rec.Body.String())
			}
			if tc.hx {
				if loc := rec.Header().Get("HX-Redirect"); loc != "/admin/login" {
					t.Fatalf("HX-Redirect = %q, want /admin/login", loc)
				}
			} else if loc := rec.Header().Get("Location"); loc != "/admin/login" {
				t.Fatalf("Location = %q, want /admin/login", loc)
			}
		})
	}
	t.Run("guard anonymous create 302", func(t *testing.T) {
		s, _ := newAdminServer(t)
		rec := postCreate(t, s, "", "x", map[string]string{"label": "x", "kind": "park", "lat": "42.76", "lon": "-71.03"})
		if rec.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302", rec.Code)
		}
	})
}

// --- walk page ---

func TestAdminWalkPage(t *testing.T) {
	s, session := newAdminServer(t)
	rec := adminGet(t, s, "/admin/walk", session, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body:\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`name="csrf_token"`, `Add light here`, `Confirm pin`, `Save anyway (low accuracy)`,
		`Asset number`, `Label`, `Kind`, `Park`, `Building`, `Photo`,
		`aria-label="position pin"`, `id="walk-pos"`, `id="walk-accuracy"`, `id="pin-pos"`,
		`id="manual-lat"`, `id="manual-lon"`, `type="number"`, `hx-post="/admin/lights"`,
		`id="dup-slot"`, `data-bbox="42.7,-71.09,42.82,-70.96"`, `/static/js/walk.js`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("walk page missing %q", want)
		}
	}
	// The double-submit cookie accompanies the rendered token.
	sawCSRF := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == CSRFTokenName && c.Value != "" {
			sawCSRF = true
		}
	}
	if !sawCSRF {
		t.Error("walk page set no csrf_token cookie")
	}
	// The rendered hidden csrf input carries a value attribute (the drafts
	// parse it), and it matches the cookie.
	reInput := regexp.MustCompile(`<input[^>]*name="csrf_token"[^>]*value="([^"]+)"`)
	if reInput.FindStringSubmatch(body) == nil {
		t.Errorf("csrf_token input has no value attribute; body:\n%s", body)
	}
}

// --- list page + rows fragment ---

func TestAdminListEmpty(t *testing.T) {
	s, session := newAdminServer(t)
	rec := adminGet(t, s, "/admin", session, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<h1>Lights</h1>") {
		t.Errorf("list missing the exact Lights heading; body:\n%s", body)
	}
	if !strings.Contains(body, "No lights recorded yet") {
		t.Error("empty list missing the empty message")
	}
	if strings.Contains(body, "Older") {
		t.Error("empty list shows an Older control")
	}
}

func TestAdminRowsRedirectAndFragment(t *testing.T) {
	s, session := newAdminServer(t)
	csrf := adminCSRF(t)
	createLight(t, s, session, csrf, map[string]string{
		"label": "Rows lamp", "kind": "park", "lat": "42.7617", "lon": "-71.0303",
	})
	t.Run("no HX redirects to the full page", func(t *testing.T) {
		rec := adminGet(t, s, "/admin/rows?page=2", session, false)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/admin?page=2" {
			t.Fatalf("Location = %q, want /admin?page=2", loc)
		}
	})
	t.Run("HX returns the rows fragment", func(t *testing.T) {
		rec := adminGet(t, s, "/admin/rows?page=1", session, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if !reRowsRoot.MatchString(rec.Body.String()) {
			t.Errorf("fragment root lacks id=\"rows\"; body:\n%s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Rows lamp") {
			t.Error("fragment lacks the row")
		}
	})
}

// --- create ---

func TestAdminCreateHappyPath(t *testing.T) {
	s, session := newAdminServer(t)
	csrf := adminCSRF(t)
	t.Run("no HX 303 to /admin", func(t *testing.T) {
		createLight(t, s, session, csrf, map[string]string{
			"label": "Happy lamp", "kind": "park", "lat": "42.7617", "lon": "-71.0303",
		})
	})
	t.Run("HX answers the Saved status fragment", func(t *testing.T) {
		rec := adminPostMultipart(t, s, "/admin/lights", map[string]string{
			CSRFTokenName: csrf, "label": "HX lamp", "kind": "park",
			"lat": "42.7700", "lon": "-71.0400",
		}, "", "", nil, session, csrf, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `role="status"`) || !strings.Contains(rec.Body.String(), "Saved") {
			t.Errorf("fragment lacks role=status Saved; body:\n%s", rec.Body.String())
		}
	})
	t.Run("building kind accepted and listed with plain alt", func(t *testing.T) {
		createLight(t, s, session, csrf, map[string]string{
			"label": "Building lamp", "kind": "building", "lat": "42.7750", "lon": "-71.0200",
			"asset_no": "GP-900", "accuracy_m": "12.5",
		})
		list := adminGet(t, s, "/admin", session, false).Body.String()
		if !strings.Contains(list, `alt="GP-900 Building lamp"`) {
			t.Errorf("list missing the numbered alt format; body:\n%s", list)
		}
		if !strings.Contains(list, `alt="Happy lamp"`) {
			t.Errorf("list missing the unnumbered alt format; body:\n%s", list)
		}
		if !strings.Contains(list, "unknown") {
			t.Error("list rows lack the status text label")
		}
	})
}

func TestAdminCreateValidation(t *testing.T) {
	s, session := newAdminServer(t)
	csrf := adminCSRF(t)
	long := strings.Repeat("x", 121)
	tests := []struct {
		name   string
		fields map[string]string
		want   string
	}{
		{"empty label", map[string]string{"label": "", "kind": "park", "lat": "42.76", "lon": "-71.03"}, "Label must be"},
		{"over-long label", map[string]string{"label": long, "kind": "park", "lat": "42.76", "lon": "-71.03"}, "Label must be"},
		{"bad asset", map[string]string{"label": "L", "kind": "park", "lat": "42.76", "lon": "-71.03", "asset_no": "bad space"}, "Asset number"},
		{"street kind reserved", map[string]string{"label": "L", "kind": "street", "lat": "42.76", "lon": "-71.03"}, "park or building"},
		{"missing kind", map[string]string{"label": "L", "lat": "42.76", "lon": "-71.03"}, "park or building"},
		{"bad lat", map[string]string{"label": "L", "kind": "park", "lat": "abc", "lon": "-71.03"}, "position is required"},
		{"bad lon", map[string]string{"label": "L", "kind": "park", "lat": "42.76", "lon": "1e999"}, "position is required"},
		{"negative accuracy", map[string]string{"label": "L", "kind": "park", "lat": "42.76", "lon": "-71.03", "accuracy_m": "-1"}, "Accuracy"},
		{"unknown dup_choice", map[string]string{"label": "L", "kind": "park", "lat": "42.76", "lon": "-71.03", "dup_choice": "nope"}, "Unknown duplicate choice"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := postCreate(t, s, session, csrf, tc.fields)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body:\n%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Errorf("body lacks %q; body:\n%s", tc.want, rec.Body.String())
			}
		})
	}
	t.Run("missing csrf 403", func(t *testing.T) {
		rec := adminPostMultipart(t, s, "/admin/lights", map[string]string{
			"label": "L", "kind": "park", "lat": "42.76", "lon": "-71.03",
		}, "", "", nil, session, "", false)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
	})
	t.Run("mismatched csrf 403", func(t *testing.T) {
		rec := adminPostMultipart(t, s, "/admin/lights", map[string]string{
			CSRFTokenName: csrf, "label": "L", "kind": "park", "lat": "42.76", "lon": "-71.03",
		}, "", "", nil, session, mustCSRFToken(t), false)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
	})
	t.Run("non-multipart body 400", func(t *testing.T) {
		rec := adminDo(t, s, http.MethodPost, "/admin/lights", session, false,
			strings.NewReader("label=x"), "application/x-www-form-urlencoded")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})
}

func TestAdminCreateOutsideBBox(t *testing.T) {
	s, session := newAdminServer(t)
	csrf := adminCSRF(t)
	fields := map[string]string{"label": "Out lamp", "kind": "park", "lat": "42.36", "lon": "-71.06"}
	rec := postCreate(t, s, session, csrf, fields)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body:\n%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "outside Groveland town bounds") {
		t.Errorf("body lacks the frozen message; body:\n%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `role="alert"`) {
		t.Error("rejection page lacks role=alert")
	}
	hx := adminPostMultipart(t, s, "/admin/lights", map[string]string{
		CSRFTokenName: csrf, "label": "Out lamp", "kind": "park", "lat": "42.36", "lon": "-71.06",
	}, "", "", nil, session, csrf, true)
	if hx.Code != http.StatusUnprocessableEntity {
		t.Fatalf("HX status = %d, want 422", hx.Code)
	}
	if !strings.Contains(hx.Body.String(), "outside Groveland town bounds") {
		t.Errorf("HX fragment lacks the frozen message; body:\n%s", hx.Body.String())
	}
	if strings.Contains(hx.Body.String(), "<!doctype") || strings.Contains(hx.Body.String(), "<html") {
		t.Error("HX rejection must be a fragment, not a full page")
	}
}

func TestAdminCreatePhotoPipeline(t *testing.T) {
	s, session := newAdminServer(t)
	csrf := adminCSRF(t)
	base := map[string]string{CSRFTokenName: csrf, "label": "Photo lamp", "kind": "park", "lat": "42.7617", "lon": "-71.0303"}
	t.Run("happy path stores and serves the photo", func(t *testing.T) {
		rec := adminPostMultipart(t, s, "/admin/lights", base, "photo", "landscape.jpg", adminFixture(t, "landscape.jpg"), session, csrf, false)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303; body:\n%s", rec.Code, rec.Body.String())
		}
		list := adminGet(t, s, "/admin", session, false).Body.String()
		re := regexp.MustCompile(`src="(/photos/[^"]+)"`)
		m := re.FindStringSubmatch(list)
		if m == nil {
			t.Fatalf("list has no /photos src; body:\n%s", list)
		}
		served := adminGet(t, s, m[1], session, false)
		if served.Code != http.StatusOK {
			t.Fatalf("photo status = %d, want 200", served.Code)
		}
		if ct := served.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/jpeg") {
			t.Errorf("photo Content-Type = %q", ct)
		}
	})
	t.Run("HEIC refused 415 with frozen wording", func(t *testing.T) {
		rec := adminPostMultipart(t, s, "/admin/lights", base, "photo", "heic.bin", adminFixture(t, "heic.bin"), session, csrf, false)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d, want 415", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "please choose JPEG") {
			t.Errorf("body lacks the HEIC wording; body:\n%s", rec.Body.String())
		}
	})
	t.Run("text-as-jpg refused 415", func(t *testing.T) {
		rec := adminPostMultipart(t, s, "/admin/lights", base, "photo", "text-as.jpg", adminFixture(t, "text-as.jpg"), session, csrf, false)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d, want 415", rec.Code)
		}
	})
	t.Run("pixel bomb refused 415", func(t *testing.T) {
		rec := adminPostMultipart(t, s, "/admin/lights", base, "photo", "pixelbomb.png", adminFixture(t, "pixelbomb.png"), session, csrf, false)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d, want 415", rec.Code)
		}
	})
	t.Run("oversize body refused 413", func(t *testing.T) {
		jpeg := adminFixture(t, "landscape.jpg")
		big := append(append([]byte{}, jpeg...), make([]byte, 16<<20)...)
		rec := adminPostMultipart(t, s, "/admin/lights", base, "photo", "big.jpg", big, session, csrf, false)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413", rec.Code)
		}
	})
}

// --- duplicates ---

func TestAdminDuplicateNumbered(t *testing.T) {
	s, session := newAdminServer(t)
	csrf := adminCSRF(t)
	createLight(t, s, session, csrf, map[string]string{
		"asset_no": "GP-017", "label": "Ball field lamp", "kind": "park", "lat": "42.7617", "lon": "-71.0303",
	})
	t.Run("same asset raises the panel (409)", func(t *testing.T) {
		rec := postCreate(t, s, session, csrf, map[string]string{
			"asset_no": "GP-017", "label": "Ball field lamp twin", "kind": "park", "lat": "42.76175", "lon": "-71.03035",
		})
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body:\n%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		for _, want := range []string{"Possible duplicate", "Merge", "Cancel", `id="dup-panel"`, "Ball field lamp", "GP-017"} {
			if !strings.Contains(body, want) {
				t.Errorf("panel missing %q; body:\n%s", want, body)
			}
		}
		if strings.Contains(body, `value="save"`) {
			t.Error("numbered panel offers Save anyway")
		}
	})
	t.Run("nothing saved while the panel is up", func(t *testing.T) {
		list := adminGet(t, s, "/admin", session, false).Body.String()
		if strings.Contains(list, "Ball field lamp twin") {
			t.Error("list shows the twin before any dup_choice")
		}
	})
	t.Run("numbered save is 422", func(t *testing.T) {
		rec := postCreate(t, s, session, csrf, map[string]string{
			"asset_no": "GP-017", "label": "Ball field lamp twin", "kind": "park", "lat": "42.76175", "lon": "-71.03035", "dup_choice": "save",
		})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", rec.Code)
		}
	})
	t.Run("merge applies the twin label onto the survivor", func(t *testing.T) {
		rec := postCreate(t, s, session, csrf, map[string]string{
			"asset_no": "GP-017", "label": "Ball field lamp twin", "kind": "park", "lat": "42.76175", "lon": "-71.03035", "dup_choice": "merge",
		})
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303; body:\n%s", rec.Code, rec.Body.String())
		}
		list := adminGet(t, s, "/admin", session, false).Body.String()
		if got := strings.Count(list, `alt="GP-017 Ball field lamp twin"`); got != 1 {
			t.Errorf("survivor rows with the twin label = %d, want 1; body:\n%s", got, list)
		}
		if strings.Contains(list, `alt="GP-017 Ball field lamp"`) {
			t.Error("survivor still carries the original label")
		}
	})
}

func TestAdminDuplicateUnnumbered(t *testing.T) {
	s, session := newAdminServer(t)
	csrf := adminCSRF(t)
	createLight(t, s, session, csrf, map[string]string{
		"label": "Path light one", "kind": "park", "lat": "42.7625", "lon": "-71.0295",
	})
	t.Run("second unnumbered within 8 m raises the panel with Save anyway", func(t *testing.T) {
		rec := postCreate(t, s, session, csrf, map[string]string{
			"label": "Path light two", "kind": "park", "lat": "42.76254", "lon": "-71.0295",
		})
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body:\n%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, `value="save"`) || !strings.Contains(body, "Save anyway") {
			t.Errorf("unnumbered panel lacks Save anyway; body:\n%s", body)
		}
		if !strings.Contains(body, "Possible duplicate") {
			t.Error("panel lacks the frozen heading")
		}
		if !strings.Contains(body, "m away") {
			t.Error("panel lacks the distance")
		}
	})
	t.Run("save-anyway keeps both lights", func(t *testing.T) {
		rec := postCreate(t, s, session, csrf, map[string]string{
			"label": "Path light two", "kind": "park", "lat": "42.76254", "lon": "-71.0295", "dup_choice": "save",
		})
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303; body:\n%s", rec.Code, rec.Body.String())
		}
		list := adminGet(t, s, "/admin", session, false).Body.String()
		for _, alt := range []string{`alt="Path light one"`, `alt="Path light two"`} {
			if got := strings.Count(list, alt); got != 1 {
				t.Errorf("%s count = %d, want 1; body:\n%s", alt, got, list)
			}
		}
	})
	t.Run("beyond 8 m no panel fires", func(t *testing.T) {
		rec := postCreate(t, s, session, csrf, map[string]string{
			"label": "Path light three", "kind": "park", "lat": "42.7627", "lon": "-71.0295",
		})
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303; body:\n%s", rec.Code, rec.Body.String())
		}
	})
}

// --- detail / edit / delete ---

// adminDetailFixture creates one numbered light and returns its detail URL.
func adminDetailFixture(t *testing.T, s *Server, session, csrf string) string {
	t.Helper()
	createLight(t, s, session, csrf, map[string]string{
		"asset_no": "GP-018", "label": "Old label", "kind": "park", "lat": "42.7620", "lon": "-71.0290",
	})
	list := adminGet(t, s, "/admin", session, false).Body.String()
	re := regexp.MustCompile(`href="(/admin/lights/\d+)"`)
	m := re.FindStringSubmatch(list)
	if m == nil {
		t.Fatalf("list has no detail link; body:\n%s", list)
	}
	return m[1]
}

func TestAdminDetailPage(t *testing.T) {
	s, session := newAdminServer(t)
	csrf := adminCSRF(t)
	detailURL := adminDetailFixture(t, s, session, csrf)
	t.Run("detail shows position, asset and 200", func(t *testing.T) {
		rec := adminGet(t, s, detailURL, session, false)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `id="light-pos"`) || !strings.Contains(body, "42.762000") || !strings.Contains(body, "-71.029000") {
			t.Errorf("detail lacks the 6 dp position; body:\n%s", body)
		}
		if !strings.Contains(body, "GP-018") {
			t.Error("detail lacks the asset number")
		}
		if !strings.Contains(body, "Save changes") || !strings.Contains(body, "Delete light") {
			t.Error("detail lacks the frozen buttons")
		}
	})
	t.Run("unknown id 404", func(t *testing.T) {
		if rec := adminGet(t, s, "/admin/lights/99999", session, false); rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
	t.Run("non-numeric id 404", func(t *testing.T) {
		if rec := adminGet(t, s, "/admin/lights/notanumber", session, false); rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

func TestAdminEdit(t *testing.T) {
	s, session := newAdminServer(t)
	csrf := adminCSRF(t)
	detailURL := adminDetailFixture(t, s, session, csrf)
	t.Run("label-only edit keeps asset and position", func(t *testing.T) {
		rec := adminPostMultipart(t, s, detailURL, map[string]string{
			CSRFTokenName: csrf, "label": "New label",
		}, "", "", nil, session, csrf, false)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303; body:\n%s", rec.Code, rec.Body.String())
		}
		if loc := rec.Header().Get("Location"); loc != detailURL {
			t.Fatalf("Location = %q, want %q", loc, detailURL)
		}
		body := adminGet(t, s, detailURL, session, false).Body.String()
		if !strings.Contains(body, "New label") || !strings.Contains(body, "GP-018") || !strings.Contains(body, "42.762000") {
			t.Errorf("detail after edit lost values; body:\n%s", body)
		}
	})
	t.Run("edit reusing another asset 409", func(t *testing.T) {
		createLight(t, s, session, csrf, map[string]string{
			"asset_no": "GP-019", "label": "Other lamp", "kind": "park", "lat": "42.7800", "lon": "-71.0100",
		})
		rec := adminPostMultipart(t, s, detailURL, map[string]string{
			CSRFTokenName: csrf, "asset_no": "GP-019",
		}, "", "", nil, session, csrf, false)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body:\n%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "already in use") {
			t.Errorf("body lacks the duplicate message; body:\n%s", rec.Body.String())
		}
	})
	t.Run("edit without csrf 403", func(t *testing.T) {
		rec := adminPostMultipart(t, s, detailURL, map[string]string{"label": "Nope"}, "", "", nil, session, "", false)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
	})
}

func TestAdminDelete(t *testing.T) {
	s, session := newAdminServer(t)
	csrf := adminCSRF(t)
	createLight(t, s, session, csrf, map[string]string{
		"label": "Doomed lamp", "kind": "park", "lat": "42.7617", "lon": "-71.0303",
	})
	list := adminGet(t, s, "/admin", session, false).Body.String()
	re := regexp.MustCompile(`href="(/admin/lights/\d+)"`)
	detailURL := re.FindStringSubmatch(list)[1]
	t.Run("delete 303 to /admin", func(t *testing.T) {
		rec := adminPostMultipart(t, s, detailURL+"/delete", map[string]string{CSRFTokenName: csrf}, "", "", nil, session, csrf, false)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/admin" {
			t.Fatalf("Location = %q, want /admin", loc)
		}
	})
	t.Run("detail 404 after delete", func(t *testing.T) {
		if rec := adminGet(t, s, detailURL, session, false); rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
	t.Run("list no longer mentions the light", func(t *testing.T) {
		if list := adminGet(t, s, "/admin", session, false).Body.String(); strings.Contains(list, "Doomed lamp") {
			t.Error("list still shows the deleted light")
		}
	})
	t.Run("deleting again 404s", func(t *testing.T) {
		rec := adminPostMultipart(t, s, detailURL+"/delete", map[string]string{CSRFTokenName: csrf}, "", "", nil, session, csrf, false)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
	t.Run("delete without csrf 403", func(t *testing.T) {
		rec := adminPostMultipart(t, s, "/admin/lights/1/delete", map[string]string{}, "", "", nil, session, "", false)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
	})
}

// --- pagination boundary (frozen 50/51) ---

func TestAdminPaginationBoundary(t *testing.T) {
	s, session := newAdminServer(t)
	csrf := adminCSRF(t)
	seed := func(n int) {
		for i := 1; i <= n; i++ {
			createLight(t, s, session, csrf, map[string]string{
				"label": padLamp(i), "kind": "park",
				"lat": strconv.FormatFloat(42.76+float64(i)*0.0005, 'f', 7, 64),
				"lon": strconv.FormatFloat(-71.045+float64(i)*0.0005, 'f', 7, 64),
			})
		}
	}
	seed(50)
	t.Run("50 lights: one page, no Older", func(t *testing.T) {
		body := adminGet(t, s, "/admin", session, false).Body.String()
		if strings.Contains(body, "Older") {
			t.Error("page 1 with exactly 50 rows shows Older")
		}
		if !strings.Contains(body, padLamp(50)) || !strings.Contains(body, padLamp(1)) {
			t.Errorf("page 1 lacks all 50 rows; body:\n%s", body)
		}
	})
	t.Run("51st light: Older appears, page 2 holds the oldest", func(t *testing.T) {
		createLight(t, s, session, csrf, map[string]string{
			"label": padLamp(51), "kind": "park",
			"lat": strconv.FormatFloat(42.76+51*0.0005, 'f', 7, 64),
			"lon": strconv.FormatFloat(-71.045+51*0.0005, 'f', 7, 64),
		})
		body := adminGet(t, s, "/admin", session, false).Body.String()
		if !strings.Contains(body, "Older") {
			t.Error("page 1 with 51 lights lacks Older")
		}
		if strings.Contains(body, padLamp(1)) {
			t.Error("page 1 shows the oldest light")
		}
		page2 := adminGet(t, s, "/admin?page=2", session, false)
		if page2.Code != http.StatusOK {
			t.Fatalf("page 2 status = %d, want 200", page2.Code)
		}
		if !strings.Contains(page2.Body.String(), padLamp(1)) {
			t.Errorf("page 2 lacks the oldest light; body:\n%s", page2.Body.String())
		}
	})
	t.Run("rows fragment page 2 carries the oldest only", func(t *testing.T) {
		frag := adminGet(t, s, "/admin/rows?page=2", session, true)
		if frag.Code != http.StatusOK || !reRowsRoot.MatchString(frag.Body.String()) {
			t.Fatalf("fragment status = %d", frag.Code)
		}
		if !strings.Contains(frag.Body.String(), padLamp(1)) || strings.Contains(frag.Body.String(), padLamp(51)) {
			t.Errorf("fragment page 2 content wrong; body:\n%s", frag.Body.String())
		}
	})
}

func padLamp(i int) string {
	return fmt.Sprintf("Boundary lamp %02d", i)
}
