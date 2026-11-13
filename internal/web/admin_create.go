// POST /admin/lights — the add-light submission (A-202/A-204/A-205/A-206).
// Frozen check order: parse → CSRF → town bbox → photo validation →
// duplicates. The one multipart parse happens in the boundary parser (after
// upload.LimitBody); the photo pipeline reuses that form (upload.go's
// documented contract). HTMX requests answer fragments the walk form swaps
// into #dup-slot; every other client gets redirects or full error pages.
package web

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/amscotti/groveland-lights/internal/geo"
	"github.com/amscotti/groveland-lights/internal/store"
)

func init() {
	register(route{pattern: "POST /admin/lights", handler: AdminOnly(adminCreate)})
}

// adminFormMemory is the multipart parse budget; the body itself is capped at
// MaxPhotoBytes by LimitBody before any parse (frozen order).
const adminFormMemory = 32 << 20

// assetNoPattern is the frozen asset-number rule: 1..32 of [A-Za-z0-9-].
var assetNoPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,32}$`)

// badSubmission is a parsed request that failed validation, carrying the
// response status and the user-facing message.
type badSubmission struct {
	status  int
	message string
}

func bad(status int, message string) *badSubmission {
	return &badSubmission{status: status, message: message}
}

// walkSubmission is a POST /admin/lights body parsed once at the boundary;
// after parseWalkSubmission no handler touches the form. The Raw fields hold
// exactly what the admin typed, so every error re-render can echo them; the
// typed fields are filled by validateWalkSubmission only after CSRF passes.
type walkSubmission struct {
	CSRFToken string
	LabelRaw  string
	AssetRaw  string
	KindRaw   string
	LatRaw    string
	LonRaw    string
	AccRaw    string
	DupRaw    string

	Label     string
	AssetNo   *string
	Kind      store.Kind
	Lat       float64
	Lon       float64
	AccuracyM *float64
	DupChoice string
}

// parseWalkSubmission lifts the multipart form into a typed submission.
// LimitBody(r) MUST have been called first (the handler does); the parse here
// is the request's single multipart parse, and the photo pipeline reuses it.
// This step is syntactic only: field validation runs after CSRF so the frozen
// parse → CSRF → bbox order always answers 403 to a bad token.
func parseWalkSubmission(r *http.Request) (walkSubmission, *badSubmission) {
	if err := r.ParseMultipartForm(adminFormMemory); err != nil {
		// A body over the LimitBody cap surfaces as *http.MaxBytesError and
		// is reported through the photo wording (413); anything else is a
		// malformed multipart request.
		status, message := PhotoProblem(err)
		if status == http.StatusInternalServerError {
			return walkSubmission{}, bad(http.StatusBadRequest, "bad request")
		}
		return walkSubmission{}, bad(status, message)
	}
	v := r.MultipartForm.Value
	val := func(key string) string { return firstValue(v, key) }
	return walkSubmission{
		CSRFToken: strings.TrimSpace(val(CSRFTokenName)),
		LabelRaw:  val("label"),
		AssetRaw:  val("asset_no"),
		KindRaw:   val("kind"),
		LatRaw:    val("lat"),
		LonRaw:    val("lon"),
		AccRaw:    val("accuracy_m"),
		DupRaw:    val("dup_choice"),
	}, nil
}

// validateWalkSubmission parses the required fields (label, asset number, kind,
// position) and the optional ones (accuracy, dup_choice) into their typed form.
// It runs after CSRF verification.
func validateWalkSubmission(sub *walkSubmission) *badSubmission {
	sub.Label = strings.TrimSpace(sub.LabelRaw)
	if len(sub.Label) < 1 || len(sub.Label) > 120 {
		return bad(http.StatusUnprocessableEntity, "Label must be 1 to 120 characters")
	}
	if raw := strings.TrimSpace(sub.AssetRaw); raw != "" {
		if !assetNoPattern.MatchString(raw) {
			return bad(http.StatusUnprocessableEntity, "Asset number must be 1-32 letters, digits or dashes")
		}
		asset := raw
		sub.AssetNo = &asset
	}
	kind, err := store.ParseKind(strings.TrimSpace(sub.KindRaw))
	if err != nil || kind == store.KindStreet {
		// street is reserved for the later import; the walk form offers
		// park|building only (frozen reconciliation 3).
		return bad(http.StatusUnprocessableEntity, "Choose a kind: park or building")
	}
	sub.Kind = kind
	lat, lon, perr := parseLatLon(sub.LatRaw, sub.LonRaw)
	if perr != nil {
		return bad(http.StatusUnprocessableEntity, "A position is required — wait for a fix or type one in")
	}
	sub.Lat, sub.Lon = lat, lon
	if raw := strings.TrimSpace(sub.AccRaw); raw != "" {
		acc, err := strconv.ParseFloat(raw, 64)
		if err != nil || acc < 0 || !finite(acc) {
			return bad(http.StatusUnprocessableEntity, "Accuracy must be a metre value of zero or more")
		}
		sub.AccuracyM = &acc
	}
	switch dc := sub.DupRaw; dc {
	case "", "merge", "save":
		sub.DupChoice = dc
	default:
		return bad(http.StatusUnprocessableEntity, "Unknown duplicate choice")
	}
	return nil
}

// firstValue returns the first value of a parsed form field ("" when absent).
func firstValue(v map[string][]string, key string) string {
	if vs := v[key]; len(vs) > 0 {
		return vs[0]
	}
	return ""
}

// parseLatLon converts the two position fields, refusing NaN/Inf.
func parseLatLon(latRaw, lonRaw string) (float64, float64, error) {
	lat, err := strconv.ParseFloat(strings.TrimSpace(latRaw), 64)
	if err != nil {
		return 0, 0, err
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(lonRaw), 64)
	if err != nil {
		return 0, 0, err
	}
	if !finite(lat) || !finite(lon) || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0, fmt.Errorf("position out of range")
	}
	return lat, lon, nil
}

// finite reports whether f is a usable (non-NaN, non-Inf) float.
func finite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

// adminCreate runs the frozen check order over one parsed submission.
func adminCreate(w http.ResponseWriter, r *http.Request) {
	s := serverFrom(r.Context())
	if !s.storeReady(w) {
		return
	}
	LimitBody(r) // FIRST: before anything reads the body
	sub, perr := parseWalkSubmission(r)
	if perr != nil {
		s.walkReject(w, r, nil, perr.status, perr.message)
		return
	}
	if !s.verifyWalkCSRF(w, r, sub) {
		return
	}
	if perr := validateWalkSubmission(&sub); perr != nil {
		s.walkReject(w, r, &sub, perr.status, perr.message)
		return
	}
	if !s.Cfg.TownBBox.Contains(sub.Lat, sub.Lon) {
		s.walkReject(w, r, &sub, http.StatusUnprocessableEntity,
			"That position is outside Groveland town bounds")
		return
	}
	photo, ok := s.walkPhoto(w, r, &sub)
	if !ok {
		return
	}
	if s.respondWalkDuplicate(w, r, sub, photo) {
		return
	}
	s.createWalkLight(w, r, sub, photo)
}

// verifyWalkCSRF answers 403 for a bad or missing token. It runs before any
// semantic field validation (frozen parse → CSRF → bbox order).
func (s *Server) verifyWalkCSRF(w http.ResponseWriter, r *http.Request, sub walkSubmission) bool {
	if err := VerifyCSRF(r, sub.CSRFToken); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

// walkPhoto runs the photo pipeline and maps its errors. ok is false when a
// response has already been written.
func (s *Server) walkPhoto(w http.ResponseWriter, r *http.Request, sub *walkSubmission) (*UploadedPhoto, bool) {
	photo, err := s.Photo(r) // reuses the boundary parse; validates the upload
	if err == nil {
		return photo, true
	}
	status, message := PhotoProblem(err)
	if status == http.StatusInternalServerError {
		s.Log.Error("admin create: photo", "err", err)
		http.Error(w, "internal error", status)
		return nil, false
	}
	s.walkReject(w, r, sub, status, message)
	return nil, false
}

// respondWalkDuplicate handles a classified duplicate. It reports true when it
// has written the response and the caller must stop.
func (s *Server) respondWalkDuplicate(w http.ResponseWriter, r *http.Request, sub walkSubmission, photo *UploadedPhoto) bool {
	decision, dup, err := s.classifyDuplicate(r, sub)
	if err != nil {
		s.Log.Error("admin create: duplicate lookup", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return true
	}
	if !dup {
		return false
	}
	switch {
	case sub.DupChoice == "merge":
		s.mergeDuplicate(w, r, sub, decision, photo)
		return true
	case sub.DupChoice == "save" && decision.Numbered:
		// Frozen: save-anyway is unnumbered-only.
		s.walkReject(w, r, &sub, http.StatusUnprocessableEntity,
			"This fixture has an asset number — merge it or cancel instead")
		return true
	case sub.DupChoice == "":
		s.respondDuplicate(w, r, sub, decision)
		return true
	default:
		// dup_choice=save on an unnumbered duplicate: fall through and create.
		return false
	}
}

// createWalkLight inserts the light and answers success (or the race panel).
func (s *Server) createWalkLight(w http.ResponseWriter, r *http.Request, sub walkSubmission, photo *UploadedPhoto) {
	_, err := s.Store.CreateLight(r.Context(), store.NewLight{
		AssetNo:   sub.AssetNo,
		Kind:      sub.Kind,
		Label:     sub.Label,
		Lat:       sub.Lat,
		Lon:       sub.Lon,
		AccuracyM: sub.AccuracyM,
		PhotoID:   photoIDOf(photo),
	})
	if err == nil {
		s.respondWalkOK(w, r)
		return
	}
	if errors.Is(err, store.ErrDuplicateAssetNo) && s.respondCreateRace(w, r, sub) {
		return
	}
	s.Log.Error("admin create", "err", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// respondCreateRace shows the numbered duplicate panel for the row that won a
// concurrent same-asset-number create.
func (s *Server) respondCreateRace(w http.ResponseWriter, r *http.Request, sub walkSubmission) bool {
	existing, err := s.Store.LightByAssetNo(r.Context(), *sub.AssetNo)
	if err != nil {
		return false
	}
	s.respondDuplicate(w, r, sub, geo.DupDecision{
		Numbered:   true,
		ExistingID: int64(existing.ID),
		DistanceM:  geo.DistanceM(geo.Point{Lat: sub.Lat, Lon: sub.Lon}, geo.Point{Lat: existing.Lat, Lon: existing.Lon}),
	})
	return true
}

// photoIDOf converts an optional uploaded photo into its typed id reference.
func photoIDOf(photo *UploadedPhoto) *store.PhotoID {
	if photo == nil {
		return nil
	}
	id := photo.ID
	return &id
}

// classifyDuplicate gathers the near candidates (plus the asset-number hit,
// which is distance-independent) and lets geo.ClassifyDuplicate decide. A
// store failure is returned to the caller, never swallowed: falling through
// would create a light with duplicate detection silently disabled.
func (s *Server) classifyDuplicate(r *http.Request, sub walkSubmission) (geo.DupDecision, bool, error) {
	newPt := geo.Point{Lat: sub.Lat, Lon: sub.Lon}
	var candidates []geo.Candidate
	if sub.AssetNo != nil {
		existing, err := s.Store.LightByAssetNo(r.Context(), *sub.AssetNo)
		switch {
		case err == nil:
			candidates = append(candidates, geo.Candidate{
				LightID: int64(existing.ID),
				AssetNo: *sub.AssetNo,
				Point:   geo.Point{Lat: existing.Lat, Lon: existing.Lon},
			})
		case errors.Is(err, store.ErrNotFound):
			// No numbered duplicate; the proximity check still runs.
		default:
			return geo.DupDecision{}, false, fmt.Errorf("light by asset_no: %w", err)
		}
	}
	nearby, err := s.Store.NearbyLights(r.Context(), sub.Lat, sub.Lon, geo.NumberedRadiusM)
	if err != nil {
		return geo.DupDecision{}, false, fmt.Errorf("nearby lights: %w", err)
	}
	for _, l := range nearby {
		asset := ""
		if l.AssetNo != nil {
			asset = *l.AssetNo
		}
		candidates = append(candidates, geo.Candidate{
			LightID: int64(l.ID),
			AssetNo: asset,
			Point:   geo.Point{Lat: l.Lat, Lon: l.Lon},
		})
	}
	newAsset := ""
	if sub.AssetNo != nil {
		newAsset = *sub.AssetNo
	}
	decision, dup := geo.ClassifyDuplicate(newAsset, newPt, candidates)
	return decision, dup, nil
}

// respondDuplicate answers 409 with the duplicate panel: the fragment for
// HTMX (the walk form swaps it into #dup-slot), the full walk page otherwise.
func (s *Server) respondDuplicate(w http.ResponseWriter, r *http.Request, sub walkSubmission, decision geo.DupDecision) {
	existing, err := s.Store.GetLight(r.Context(), store.LightID(decision.ExistingID))
	if err != nil {
		s.Log.Error("admin create: dup lookup", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	asset := ""
	if existing.AssetNo != nil {
		asset = *existing.AssetNo
	}
	view := dupView{
		Label:    existing.Label,
		AssetNo:  asset,
		Distance: humanDistance(decision.DistanceM),
		Numbered: decision.Numbered,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") != "" {
		w.WriteHeader(http.StatusConflict)
		if err := s.tpl.renderPartial(w, "partials/admin-dup.html", view); err != nil {
			s.Log.Error("admin create: dup render", "err", err)
		}
		return
	}
	data := s.walkPageDataOf(w, r, sub)
	data.Dup = &view
	w.WriteHeader(http.StatusConflict)
	if err := s.tpl.renderPage(w, "admin-walk.html", data); err != nil {
		s.Log.Error("admin create: dup page render", "err", err)
	}
}

// walkReject answers a validation failure with the frozen status: an alert
// fragment for HTMX, the re-rendered walk page (values kept) otherwise. A
// parse failure has no submission to echo, but the page it returns is still a
// usable form: it carries a fresh CSRF token and the bbox.
func (s *Server) walkReject(w http.ResponseWriter, r *http.Request, sub *walkSubmission, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") != "" {
		w.WriteHeader(status)
		if err := s.tpl.renderPartial(w, "partials/admin-alert.html", alertView{Role: "alert", Message: message}); err != nil {
			s.Log.Error("admin create: reject render", "err", err)
		}
		return
	}
	submitted := walkSubmission{Kind: store.KindPark, KindRaw: string(store.KindPark)}
	if sub != nil {
		submitted = *sub
	}
	data := s.walkPageDataOf(w, r, submitted)
	if data.CSRFToken == "" {
		return // IssueCSRFToken already answered 500.
	}
	data.Alert = message
	w.WriteHeader(status)
	if err := s.tpl.renderPage(w, "admin-walk.html", data); err != nil {
		s.Log.Error("admin create: reject page render", "err", err)
	}
}

// walkPageDataOf rebuilds the walk page data from a parsed submission so the
// no-JS error re-render keeps the admin's typed values exactly as submitted
// (Raw fields), even when validation stopped before every typed field was
// filled. A parse failure has no submission, so the caller passes park.
func (s *Server) walkPageDataOf(w http.ResponseWriter, r *http.Request, sub walkSubmission) walkPageData {
	return walkPageData{
		pageData:  pageData{Title: "Walk the town", TestEnv: s.Cfg.IsTest()},
		CSRFToken: IssueCSRFToken(w, r),
		BBox:      formatBBox(s.Cfg.TownBBox),
		Values: walkValues{
			Label:     sub.LabelRaw,
			AssetNo:   sub.AssetRaw,
			Kind:      sub.KindRaw,
			Lat:       sub.LatRaw,
			Lon:       sub.LonRaw,
			AccuracyM: sub.AccRaw,
		},
	}
}

// respondWalkOK answers success: role=status "Saved" fragment for HTMX, the
// frozen 303 to /admin for everyone else.
func (s *Server) respondWalkOK(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := s.tpl.renderPartial(w, "partials/admin-alert.html", alertView{Role: "status", Message: "Saved"}); err != nil {
			s.Log.Error("admin create: ok render", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// humanDistance renders a duplicate distance for the panel.
func humanDistance(m float64) string {
	if m < 10 {
		return fmt.Sprintf("%.1f m away", m)
	}
	return fmt.Sprintf("%.0f m away", m)
}
