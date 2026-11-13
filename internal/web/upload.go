// Photo HTTP side (A-206/A-207): multipart intake through the img pipeline,
// persistence under PHOTO_DIR, and the GET /photos/ file route. T-205's form
// handler calls into this file's frozen API (LimitBody, Photo, PhotoProblem)
// without a prefix; the parse-order contract it compiles against lives on
// Photo.
package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/amscotti/groveland-lights/internal/img"
	"github.com/amscotti/groveland-lights/internal/store"
)

// MaxPhotoBytes is the upload size limit (A-206/A-207). It caps the whole
// request body (via LimitBody) and, redundantly, the photo bytes inside the
// img pipeline.
const MaxPhotoBytes = 15 << 20 // 15 MB

// Frozen pixel and edge caps for the pipeline (A-206: long edge <= 1600,
// >40 MP bombs refused before decode).
const (
	photoMaxPixels = 40_000_000
	photoMaxEdge   = 1600
)

// photoField is the multipart field Photo parses; photoMaxMemory is passed to
// ReadForm. The body is already capped at MaxPhotoBytes (15 MB) by LimitBody,
// so a 32 MB memory limit keeps every compliant upload fully in RAM — no
// part ever spills to a temp file.
const (
	photoField     = "photo"
	photoMaxMemory = 32 << 20
)

// Photo cache header: names under PHOTO_DIR are server-minted and never
// rewritten, so stored photos may be cached forever.
const photoCacheControl = "public, max-age=31536000, immutable"

var (
	ErrPhotoTooLarge    = errors.New("photo: above the size limit")
	ErrPhotoUnsupported = errors.New("photo: unsupported image format")
	ErrPhotoTooManyPx   = errors.New("photo: above the pixel limit")
)

// heicUnsupported marks ErrPhotoUnsupported whose source bytes sniffed as
// HEIC (ftyp box carrying a heic-family brand), so PhotoProblem can answer
// with the frozen "please choose JPEG instead of HEIC" wording.
type heicUnsupported struct{ err error }

func (e heicUnsupported) Error() string { return e.err.Error() }

func (e heicUnsupported) Unwrap() error { return e.err }

// LimitBody wraps the request body with http.MaxBytesReader(MaxPhotoBytes).
// Call it BEFORE any multipart parsing; the overflow surfaces as
// ErrPhotoTooLarge from Photo.
func LimitBody(r *http.Request) {
	r.Body = http.MaxBytesReader(nil, r.Body, MaxPhotoBytes)
}

// UploadedPhoto is one persisted, processed photo. Path is relative to
// PHOTO_DIR and doubles as the URL path under /photos/ (URL = "/photos/" +
// Path).
type UploadedPhoto struct {
	ID            store.PhotoID
	Path          string // relative to PHOTO_DIR; URL path under /photos/
	Width, Height int
}

// Photo parses the optional "photo" file part through the img pipeline and
// persists it: PHOTO_DIR/YYYY/MM/<16-hex-rand>.jpg plus a photos row
// (relative path, dimensions, re-encoded byte count, sha256 of the
// RE-ENCODED bytes). It returns (nil, nil) when the field is absent or empty
// (photo is optional). On ANY error nothing is left on disk and no row is
// written. Error mapping: img.ErrTooLarge → ErrPhotoTooLarge;
// img.ErrUnsupported → ErrPhotoUnsupported (HEIC included);
// img.ErrPixelBomb → ErrPhotoTooManyPx.
//
// Parse-order contract for callers (T-205):
//
//  1. LimitBody(r) FIRST, before anything reads the body.
//  2. Call s.Photo(r). It performs the ONE multipart parse of the request
//     (MultipartReader + ReadForm — a request allows only one) and stores
//     the parsed form on r.MultipartForm.
//  3. THEN read your own fields (csrf_token, label, ...) from
//     r.MultipartForm.Value. A later r.ParseMultipartForm call is a no-op
//     that returns nil; never call ParseMultipartForm / FormValue /
//     ParseForm BEFORE Photo, or Photo would see an already-consumed body.
//
// If the caller has already parsed the form (r.MultipartForm != nil), Photo
// reuses it instead of parsing again; in that ordering a body-overflow error
// surfaces at the caller's parse as *http.MaxBytesError (PhotoProblem maps
// it too).
func (s *Server) Photo(r *http.Request) (*UploadedPhoto, error) {
	form, err := photoForm(r)
	if err != nil {
		return nil, err
	}
	fhs := form.File[photoField]
	if len(fhs) == 0 {
		// Absent field, or a part without a filename (e.g. a file input
		// left empty): photo is optional.
		return nil, nil
	}
	f, err := fhs[0].Open()
	if err != nil {
		return nil, fmt.Errorf("photo: open upload: %w", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		// The bytes were fully read by ReadForm already, so this cannot be
		// a body-size error — treat it as a broken upload part.
		return nil, fmt.Errorf("photo: read upload: %w", err)
	}
	if len(data) == 0 {
		// An empty part stores nothing and is not an error.
		return nil, nil
	}
	return s.persistPhoto(r.Context(), data)
}

// photoForm returns the request's parsed multipart form, performing the
// single parse if the caller has not already done it. A body over the
// LimitBody cap surfaces here as ErrPhotoTooLarge.
func photoForm(r *http.Request) (*multipart.Form, error) {
	if r.MultipartForm != nil {
		return r.MultipartForm, nil // caller parsed first; reuse it
	}
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf("photo: request is not multipart form data: %w", err)
	}
	form, err := mr.ReadForm(photoMaxMemory)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return nil, ErrPhotoTooLarge
		}
		return nil, fmt.Errorf("photo: read multipart body: %w", err)
	}
	// Hand the single parse to the caller for its own fields.
	r.MultipartForm = form
	return form, nil
}

// persistPhoto runs the img pipeline and writes the file + row. On any error
// nothing is left on disk and no row is written (A-207).
func (s *Server) persistPhoto(ctx context.Context, data []byte) (*UploadedPhoto, error) {
	if s.Store == nil {
		return nil, errors.New("photo: store not wired")
	}
	res, err := img.Process(data, img.Options{
		MaxBytes:  MaxPhotoBytes,
		MaxPixels: photoMaxPixels,
		MaxEdge:   photoMaxEdge,
	})
	if err != nil {
		return nil, mapImgError(err, data)
	}
	rel, err := s.writePhotoFile(res.JPEG)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(res.JPEG) // hash the RE-ENCODED bytes, never the source
	row, err := s.Store.CreatePhoto(ctx, store.NewPhoto{
		Path:   rel,
		Width:  res.Width,
		Height: res.Height,
		Bytes:  int64(len(res.JPEG)),
		SHA256: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		// The row is the source of truth: a failed insert must not leave
		// the file behind (no stray files, A-207).
		_ = os.Remove(filepath.Join(s.Cfg.PhotoDir, filepath.FromSlash(rel)))
		return nil, fmt.Errorf("photo: store: %w", err)
	}
	return &UploadedPhoto{ID: row.ID, Path: rel, Width: res.Width, Height: res.Height}, nil
}

// mapImgError translates the img pipeline's sentinels onto the photo API's
// frozen ones. HEIC input is detected by an independent sniff of the source
// bytes (the img package's sniff is private by design), so PhotoProblem can
// answer with the frozen HEIC wording while the refusal itself stays
// img.ErrUnsupported.
func mapImgError(err error, src []byte) error {
	switch {
	case errors.Is(err, img.ErrTooLarge):
		return ErrPhotoTooLarge
	case errors.Is(err, img.ErrPixelBomb):
		return ErrPhotoTooManyPx
	case errors.Is(err, img.ErrUnsupported):
		if sniffHEIC(src) {
			return heicUnsupported{ErrPhotoUnsupported}
		}
		return ErrPhotoUnsupported
	default:
		return fmt.Errorf("photo: process: %w", err)
	}
}

// writePhotoFile writes the re-encoded JPEG to
// PHOTO_DIR/YYYY/MM/<16-hex-rand>.jpg (directories 0755, file 0644) and
// returns the slash-separated path relative to PHOTO_DIR. The name is
// server-generated; the client controls nothing about it.
func (s *Server) writePhotoFile(jpeg []byte) (string, error) {
	if s.Cfg.PhotoDir == "" {
		return "", errors.New("photo: PHOTO_DIR is not configured")
	}
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", fmt.Errorf("photo: rand: %w", err)
	}
	now := time.Now().UTC()
	rel := path.Join(now.Format("2006"), now.Format("01"), hex.EncodeToString(rnd[:])+".jpg")
	abs := filepath.Join(s.Cfg.PhotoDir, filepath.FromSlash(rel))
	//nolint:gosec // G301: 0755 directories are frozen by the A-206/A-207 contract (PHOTO_DIR/YYYY/MM layout)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("photo: mkdir %s: %w", filepath.Dir(abs), err)
	}
	//nolint:gosec // G306: 0644 files are frozen by the A-206/A-207 contract; content is a server-generated JPEG
	if err := os.WriteFile(abs, jpeg, 0o644); err != nil {
		return "", fmt.Errorf("photo: write %s: %w", abs, err)
	}
	return rel, nil
}

// PhotoProblem maps an upload error to (httpStatus, userMessage):
//
//	ErrPhotoTooLarge    → 413, "Photo must be at most 15 MB"
//	ErrPhotoUnsupported → 415, "Photo must be JPEG, PNG or WebP — please
//	                       choose JPEG instead of HEIC" when the bytes
//	                       sniffed as HEIC, else 415, "Photo must be JPEG,
//	                       PNG or WebP"
//	ErrPhotoTooManyPx   → 415, "Photo has too many pixels"
//
// Anything else (wiring bugs, store failures) is a 500 "internal error";
// a *http.MaxBytesError is reported as the size problem it is.
func PhotoProblem(err error) (int, string) {
	var mbe *http.MaxBytesError
	var heic heicUnsupported
	switch {
	case errors.Is(err, ErrPhotoTooLarge), errors.As(err, &mbe):
		return http.StatusRequestEntityTooLarge, "Photo must be at most 15 MB"
	case errors.Is(err, ErrPhotoTooManyPx):
		return http.StatusUnsupportedMediaType, "Photo has too many pixels"
	case errors.Is(err, ErrPhotoUnsupported):
		if errors.As(err, &heic) {
			return http.StatusUnsupportedMediaType,
				"Photo must be JPEG, PNG or WebP — please choose JPEG instead of HEIC"
		}
		return http.StatusUnsupportedMediaType, "Photo must be JPEG, PNG or WebP"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}

// heicMajorBrands are the ISO-BMFF major brands that identify HEIC/HEIF
// still images; heicCompatibleBrands the subset meaningful in the
// compatible-brand list. Kept in step with the img package's sniff so the
// frozen wording and the pipeline's refusal always agree.
var (
	heicMajorBrands = map[string]bool{
		"heic": true, "heix": true, "hevc": true, "hevx": true,
		"heim": true, "heis": true, "hevm": true, "hevs": true,
		"mif1": true, "msf1": true,
	}
	heicCompatibleBrands = map[string]bool{
		"heic": true, "heix": true, "hevc": true, "hevx": true,
		"heim": true, "heis": true, "hevm": true, "hevs": true,
	}
)

// sniffHEIC reports whether b sniffs as ISO-BMFF content carrying a
// heic-family brand: the ftyp box type sits at bytes 4..8, the major brand
// at 8..12, followed by the 4-byte minor version and the compatible-brand
// list. Only the first ftyp box is inspected.
func sniffHEIC(b []byte) bool {
	if len(b) < 12 || string(b[4:8]) != "ftyp" {
		return false
	}
	if heicMajorBrands[string(b[8:12])] {
		return true
	}
	end := len(b)
	if bl := int(binary.BigEndian.Uint32(b[0:4])); bl >= 16 && bl < end {
		end = bl
	}
	for off := 16; off+4 <= end; off += 4 {
		if heicCompatibleBrands[string(b[off:off+4])] {
			return true
		}
	}
	return false
}

func init() {
	register(route{pattern: "GET /photos/{path...}", handler: photoServe})
}

// photoServe serves stored, re-encoded photos from PHOTO_DIR ONLY. Every
// stored file is a JPEG produced by the upload pipeline, so the response is
// always Content-Type image/jpeg with the frozen immutable cache header;
// paths under PHOTO_DIR are server-minted and never rewritten, which is what
// the header promises. Missing files — and any path that climbs out of
// PHOTO_DIR, which the resolver clamps to a guaranteed miss — answer 404.
func photoServe(w http.ResponseWriter, r *http.Request) {
	s := serverFrom(r.Context())
	abs, ok := photoAbsPath(s.Cfg.PhotoDir, r.PathValue("path"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(abs) //nolint:gosec // abs comes from PHOTO_DIR joined with a cleaned rooted-relative name; photoAbsPath proves it cannot leave PHOTO_DIR
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", photoCacheControl)
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

// photoAbsPath resolves a /photos/{path...} value to a file path inside
// photoDir. The value is cleaned against a rooted form first, so dot-dot
// segments can never climb above PHOTO_DIR — a traversal attempt just
// resolves to a name inside photoDir that misses and 404s. ok is false for
// empty or unusable values.
func photoAbsPath(photoDir, urlPath string) (abs string, ok bool) {
	if photoDir == "" || strings.ContainsRune(urlPath, 0) {
		return "", false
	}
	// Rooted clean: path.Clean of "/x/../../y" is "/y" — every ".." above
	// the root is dropped, so no traversal survives.
	clean := path.Clean("/" + urlPath)
	rel := strings.TrimPrefix(clean, "/")
	if rel == "" {
		return "", false
	}
	abs = filepath.Join(photoDir, filepath.FromSlash(rel))
	// Belt and braces on top of the rooted clean: the joined path must
	// still be photoDir itself or below it.
	under, err := filepath.Rel(photoDir, abs)
	if err != nil || under == ".." || strings.HasPrefix(under, ".."+string(filepath.Separator)) {
		return "", false
	}
	return abs, true
}
