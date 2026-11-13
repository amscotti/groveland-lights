// Admin UI (A-202/A-203/A-208/A-209): the walk-the-town page, the light
// detail/edit page, the admin list with HTMX pagination and the soft-delete
// flow. Every route is wrapped in AdminOnly; every request is parsed once at
// the boundary into a typed struct and the handlers consult only those fields.
// The create endpoint (POST /admin/lights) lives in admin_create.go.
package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/amscotti/groveland-lights/internal/config"
	"github.com/amscotti/groveland-lights/internal/store"
)

func init() {
	register(route{pattern: "GET /admin", handler: AdminOnly(adminList)})
	register(route{pattern: "GET /admin/rows", handler: AdminOnly(adminRows)})
	register(route{pattern: "GET /admin/walk", handler: AdminOnly(adminWalk)})
	register(route{pattern: "GET /admin/lights/{id}", handler: AdminOnly(adminDetail)})
	register(route{pattern: "POST /admin/lights/{id}", handler: AdminOnly(adminEdit)})
	register(route{pattern: "POST /admin/lights/{id}/delete", handler: AdminOnly(adminDelete)})
	installLoginPage()
}

// installLoginPage makes pages/admin-login.html the live login page without
// touching auth.go (frozen for T-203): auth.go renders loginPageTmpl with nil
// data, so the styled pipeline output is captured once at startup and installed
// as that template. The frozen strings are unchanged; the served page now uses
// base.html (pico.css, 44 px button) instead of auth.go's minimal markup.
func installLoginPage() {
	tpl, err := parseTemplates()
	if err != nil {
		return // New will report the parse failure; keep the minimal page.
	}
	var buf bytes.Buffer
	if err := tpl.renderPage(&buf, "admin-login.html", pageData{Title: "Admin login"}); err != nil {
		return
	}
	loginPageTmpl = template.Must(template.New("admin-login").Parse(buf.String()))
}

// adminPageSize is the frozen admin list page size: the 50 most recent
// lights per page (A-209).
const adminPageSize = 50

// photoPlaceholder is the thumbnail for photo-less lights (frozen: placeholder
// thumbnails are fine); a data URI keeps the row self-contained.
const photoPlaceholder = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 48 48'%3E%3Crect width='48' height='48' fill='%23e8eaec'/%3E%3Ccircle cx='24' cy='19' r='10' fill='%239aa2ab'/%3E%3Crect x='20' y='29' width='8' height='10' rx='2' fill='%239aa2ab'/%3E%3C/svg%3E"

// pageData is the chrome every admin page carries for base.html.
type pageData struct {
	Title   string
	TestEnv bool
}

// --- admin list + rows fragment ---

// rowView is one admin list row. ImgAlt follows the frozen format: asset
// number + label for numbered lights, label alone for unnumbered ones.
// ImgSrc is template.URL solely so the constant data-URI placeholder survives
// html/template's URL scheme filter; values are server-minted, never user
// input (photo paths come from the upload pipeline).
type rowView struct {
	DetailURL string
	ImgSrc    template.URL //nolint:forbidigo // constant placeholder + pipeline paths only, never user input
	ImgAlt    string
	AssetNo   string
	Label     string
	Status    string
}

// listData backs pages/admin-list.html and partials/admin-rows.html (the
// same data renders both: the full page and the HTMX fragment).
type listData struct {
	pageData
	Rows      []rowView
	Page      int
	OlderPage int
	NewerPage int
	HasOlder  bool
	HasNewer  bool
}

// thumbAlt renders the frozen per-row alt format.
func thumbAlt(l store.Light) string {
	if l.AssetNo != nil && *l.AssetNo != "" {
		return *l.AssetNo + " " + l.Label
	}
	return l.Label
}

// rowViewOf converts one stored light into its list row. Photo lookups are
// per-row (admin scale is tens of lights, so the extra indexed read is fine).
func (s *Server) rowViewOf(ctx context.Context, l store.Light) rowView {
	src := template.URL(photoPlaceholder) //nolint:gosec // constant server-owned placeholder; scheme filter bypass is the point
	if l.PhotoID != nil {
		if p, err := s.Store.PhotoByID(ctx, *l.PhotoID); err == nil && p.Path != "" {
			src = template.URL("/photos/" + p.Path) //nolint:gosec // server-minted pipeline path, never user input
		}
	}
	asset := ""
	if l.AssetNo != nil {
		asset = *l.AssetNo
	}
	return rowView{
		DetailURL: fmt.Sprintf("/admin/lights/%d", l.ID),
		ImgSrc:    src,
		ImgAlt:    thumbAlt(l),
		AssetNo:   asset,
		Label:     l.Label,
		Status:    l.Status,
	}
}

// listPage parses the one list query parameter: the 1-based page number.
type listQuery struct{ Page int }

func parseListQuery(r *http.Request) listQuery {
	raw := r.URL.Query().Get("page")
	page, err := strconv.Atoi(raw)
	if err != nil || page < 1 {
		page = 1
	}
	return listQuery{Page: page}
}

// storeReady guards the store-backed handlers the probe way: a nil store is
// a wiring bug and answers 500.
func (s *Server) storeReady(w http.ResponseWriter) bool {
	if s.Store == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return false
	}
	return true
}

// recentRows fetches one page of rows plus one extra to detect an older page.
func (s *Server) recentRows(ctx context.Context, page int) (listData, error) {
	rows, err := s.Store.RecentLights(ctx, adminPageSize+1, (page-1)*adminPageSize)
	if err != nil {
		return listData{}, err
	}
	data := listData{
		pageData:  pageData{Title: "Lights"},
		Page:      page,
		NewerPage: page - 1,
		OlderPage: page + 1,
		HasNewer:  page > 1,
	}
	if len(rows) > adminPageSize {
		data.HasOlder = true
		rows = rows[:adminPageSize]
	}
	for _, l := range rows {
		data.Rows = append(data.Rows, s.rowViewOf(ctx, l))
	}
	return data, nil
}

// adminList renders the full admin list page for page N.
func adminList(w http.ResponseWriter, r *http.Request) {
	s := serverFrom(r.Context())
	if !s.storeReady(w) {
		return
	}
	q := parseListQuery(r)
	data, err := s.recentRows(r.Context(), q.Page)
	if err != nil {
		s.Log.Error("admin list", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data.TestEnv = s.Cfg.IsTest()
	if err := s.tpl.renderPage(w, "admin-list.html", data); err != nil {
		s.Log.Error("admin list: render", "err", err)
		// Header not yet sent by renderPage's buffering contract.
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// adminRows serves the HTMX rows fragment; without HX-Request it redirects to
// the full page (frozen contract).
func adminRows(w http.ResponseWriter, r *http.Request) {
	s := serverFrom(r.Context())
	if !s.storeReady(w) {
		return
	}
	q := parseListQuery(r)
	if r.Header.Get("HX-Request") == "" {
		http.Redirect(w, r, fmt.Sprintf("/admin?page=%d", q.Page), http.StatusSeeOther)
		return
	}
	data, err := s.recentRows(r.Context(), q.Page)
	if err != nil {
		s.Log.Error("admin rows", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.renderPartial(w, "partials/admin-rows.html", data); err != nil {
		s.Log.Error("admin rows: render", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// --- walk page ---

// walkValues re-renders a submitted form after an error; strings because the
// inputs carry strings.
type walkValues struct {
	Label     string
	AssetNo   string
	Kind      string
	Lat       string
	Lon       string
	AccuracyM string
}

// dupView is the duplicate panel's view of the existing light.
type dupView struct {
	Label    string
	AssetNo  string
	Distance string
	Numbered bool
}

// alertView backs partials/admin-alert.html — the small fragment the walk
// form swaps into #dup-slot for successes (role=status "Saved") and for
// server-side rejections (role=alert).
type alertView struct {
	Role    string
	Message string
}

// walkPageData backs pages/admin-walk.html for the fresh render and for the
// full-page error re-renders (no-JS clients).
type walkPageData struct {
	pageData
	CSRFToken string
	BBox      string
	Values    walkValues
	Alert     string
	Dup       *dupView
}

// adminWalk renders the walk page with a fresh CSRF token.
func adminWalk(w http.ResponseWriter, r *http.Request) {
	s := serverFrom(r.Context())
	token := IssueCSRFToken(w, r)
	if token == "" {
		return // IssueCSRFToken could not mint a token and answered 500.
	}
	data := walkPageData{
		pageData:  pageData{Title: "Walk the town", TestEnv: s.Cfg.IsTest()},
		CSRFToken: token,
		BBox:      formatBBox(s.Cfg.TownBBox),
		Values:    walkValues{Kind: string(store.KindPark)},
	}
	if err := s.tpl.renderPage(w, "admin-walk.html", data); err != nil {
		s.Log.Error("admin walk: render", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// formatBBox renders the town bbox as "minLat,minLon,maxLat,maxLon" for the
// walk page's client-side pre-flight (mirrors config.ParseBBox's input form).
func formatBBox(b config.BBox) string {
	return fmt.Sprintf("%g,%g,%g,%g", b.MinLat, b.MinLon, b.MaxLat, b.MaxLon)
}

// --- detail / edit / delete ---

// lightIDParam is one {id} path value parsed at the boundary.
type lightIDParam struct{ ID store.LightID }

func parseLightID(r *http.Request) (lightIDParam, bool) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		return lightIDParam{}, false
	}
	return lightIDParam{ID: store.LightID(id)}, true
}

// detailData backs pages/admin-detail.html (the detail + edit + delete page).
type detailData struct {
	pageData
	DetailURL string
	Label     string
	AssetNo   string
	Kind      string
	Status    string
	Pos       string
	Accuracy  string
	PhotoSrc  string
	CSRFToken string
	Alert     string
}

// posString renders coordinates the frozen way: "lat, lon" at 6 decimal
// places — the exact string the walk page's #pin-pos mirrors.
func posString(lat, lon float64) string {
	return strconv.FormatFloat(lat, 'f', 6, 64) + ", " + strconv.FormatFloat(lon, 'f', 6, 64)
}

// adminDetail renders one light's detail/edit page (404 once soft-deleted).
func adminDetail(w http.ResponseWriter, r *http.Request) {
	s := serverFrom(r.Context())
	if !s.storeReady(w) {
		return
	}
	id, ok := parseLightID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	light, err := s.Store.GetLight(r.Context(), id.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.Log.Error("admin detail", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	token := IssueCSRFToken(w, r)
	if token == "" {
		return
	}
	data := s.detailDataOf(r.Context(), light, token)
	if err := s.tpl.renderPage(w, "admin-detail.html", data); err != nil {
		s.Log.Error("admin detail: render", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// detailDataOf builds the detail page data for one light.
func (s *Server) detailDataOf(ctx context.Context, light store.Light, token string) detailData {
	data := detailData{
		pageData:  pageData{Title: light.Label, TestEnv: s.Cfg.IsTest()},
		DetailURL: fmt.Sprintf("/admin/lights/%d", light.ID),
		Label:     light.Label,
		Kind:      string(light.Kind),
		Status:    light.Status,
		Pos:       posString(light.Lat, light.Lon),
		CSRFToken: token,
	}
	if light.AssetNo != nil {
		data.AssetNo = *light.AssetNo
	}
	if light.AccuracyM != nil {
		data.Accuracy = strconv.FormatFloat(*light.AccuracyM, 'f', -1, 64)
	}
	if light.PhotoID != nil {
		if p, err := s.Store.PhotoByID(ctx, *light.PhotoID); err == nil {
			data.PhotoSrc = "/photos/" + p.Path
		}
	}
	return data
}

// lightEdit is a POST /admin/lights/{id} body parsed once at the boundary.
// Nil fields mean "not sent" and keep their stored values (frozen: unspecified
// fields keep values).
type lightEdit struct {
	CSRFToken string
	Label     *string
	AssetNo   *string
}

// parseLightEdit lifts the edit form into a typed struct. The admin UI posts
// urlencoded; the drafts post multipart — both are accepted, read exactly
// once. Handlers never touch the form after this parse.
func parseLightEdit(r *http.Request) (lightEdit, bool) {
	vals, ok := parseFormValues(r, adminEditMaxBody)
	if !ok {
		return lightEdit{}, false
	}
	e := lightEdit{CSRFToken: firstValue(vals, CSRFTokenName)}
	if v, ok := vals["label"]; ok && len(v) > 0 {
		e.Label = &v[0]
	}
	if v, ok := vals["asset_no"]; ok && len(v) > 0 {
		e.AssetNo = &v[0]
	}
	return e, true
}

// adminEditMaxBody bounds edit/delete bodies (no uploads there).
const adminEditMaxBody = 1 << 20

// parseFormValues reads the request's form values exactly once, for
// urlencoded (browser forms) and multipart (draft posts) bodies alike.
func parseFormValues(r *http.Request, maxBody int64) (map[string][]string, bool) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBody)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(maxBody); err != nil {
			return nil, false
		}
		return r.MultipartForm.Value, true
	}
	if err := r.ParseForm(); err != nil {
		return nil, false
	}
	return r.PostForm, true
}

// adminEdit applies a label/asset edit and redirects back to the detail page.
func adminEdit(w http.ResponseWriter, r *http.Request) {
	s := serverFrom(r.Context())
	if !s.storeReady(w) {
		return
	}
	id, ok := parseLightID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	edit, ok := parseLightEdit(r)
	if !ok {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := VerifyCSRF(r, edit.CSRFToken); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	light, err := s.Store.GetLight(r.Context(), id.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.Log.Error("admin edit", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	csrf := IssueCSRFToken(w, r)
	if csrf == "" {
		return
	}
	newLabel, bad := validatedEditLabel(edit.Label, light.Label)
	if bad != "" {
		s.adminEditRejected(w, r, light, csrf, bad)
		return
	}
	newAsset, bad := validatedEditAsset(edit.AssetNo, light.AssetNo)
	if bad != "" {
		s.adminEditRejected(w, r, light, csrf, bad)
		return
	}
	if _, err := s.Store.EditLight(r.Context(), id.ID, newLabel, newAsset); err != nil {
		if errors.Is(err, store.ErrDuplicateAssetNo) {
			s.adminEditRejected(w, r, light, csrf, "That asset number is already in use")
			return
		}
		s.Log.Error("admin edit", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/lights/%d", id.ID), http.StatusSeeOther)
}

// validatedEditLabel picks the new label: sent values must be 1..120 chars.
func validatedEditLabel(sent *string, current string) (string, string) {
	if sent == nil {
		return current, ""
	}
	label := strings.TrimSpace(*sent)
	if len(label) < 1 || len(label) > 120 {
		return "", "Label must be 1 to 120 characters"
	}
	return label, ""
}

// validatedEditAsset picks the new asset number: absent keeps the stored
// value; an empty submission clears it; otherwise the frozen pattern applies.
func validatedEditAsset(sent *string, current *string) (*string, string) {
	if sent == nil {
		return current, ""
	}
	asset := strings.TrimSpace(*sent)
	if asset == "" {
		return nil, ""
	}
	if !assetNoPattern.MatchString(asset) {
		return nil, "Asset number must be 1-32 letters, digits or dashes"
	}
	return &asset, ""
}

// adminEditRejected re-renders the detail page with an alert and a 409.
func (s *Server) adminEditRejected(w http.ResponseWriter, r *http.Request, light store.Light, csrf, message string) {
	data := s.detailDataOf(r.Context(), light, csrf)
	data.Alert = message
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusConflict)
	if err := s.tpl.renderPage(w, "admin-detail.html", data); err != nil {
		s.Log.Error("admin edit: render", "err", err)
	}
}

// lightDelete is a POST /admin/lights/{id}/delete body parsed at the boundary.
type lightDelete struct {
	CSRFToken string
}

func parseLightDelete(r *http.Request) (lightDelete, bool) {
	vals, ok := parseFormValues(r, adminEditMaxBody)
	if !ok {
		return lightDelete{}, false
	}
	return lightDelete{CSRFToken: firstValue(vals, CSRFTokenName)}, true
}

// adminDelete soft-deletes a light (frozen: 303 to /admin).
func adminDelete(w http.ResponseWriter, r *http.Request) {
	s := serverFrom(r.Context())
	if !s.storeReady(w) {
		return
	}
	id, ok := parseLightID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	del, ok := parseLightDelete(r)
	if !ok {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := VerifyCSRF(r, del.CSRFToken); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := s.Store.SoftDeleteLight(r.Context(), id.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.Log.Error("admin delete", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
