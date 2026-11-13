// Photo intake and serving tests (A-206/A-207): the multipart upload flow
// through a real temp PHOTO_DIR and a real store, the /photos file route,
// the no-stray-files rule and the PhotoProblem mapping table.
package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/amscotti/groveland-lights/internal/config"
	"github.com/amscotti/groveland-lights/internal/store"
)

// uploadRePath pins the frozen storage layout PHOTO_DIR/YYYY/MM/<16 hex>.jpg.
var uploadRePath = regexp.MustCompile(`\A\d{4}/\d{2}/[0-9a-f]{16}\.jpg\z`)

// newUploadServer builds a Server with a real temp store and a real temp
// PHOTO_DIR, wired exactly as production: Server.Store from store.Open,
// Server.Cfg.PhotoDir under its own t.TempDir. Returns the server, the store
// and the photo directory path.
func newUploadServer(t *testing.T) (*Server, *store.DB, string) {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "upload.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := newTestServer(t, config.EnvTest)
	dir := filepath.Join(t.TempDir(), "photos")
	s.Cfg.PhotoDir = dir
	s.Store = db
	return s, db, dir
}

// uploadFixture reads a photo fixture from the repo's testdata/photos.
func uploadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "photos", name))
	if err != nil {
		t.Fatalf("fixture testdata/photos/%s: %v", name, err)
	}
	return b
}

// uploadOversizeJPEG builds, in memory, a body past the 15 MB cap: a valid
// fixture JPEG with junk padded after its EOI marker.
func uploadOversizeJPEG(t *testing.T, want int) []byte {
	t.Helper()
	base := uploadFixture(t, "landscape.jpg")
	if len(base) >= want {
		t.Fatalf("fixture landscape.jpg is already %d bytes; pick a larger target", len(base))
	}
	out := make([]byte, 0, want)
	out = append(out, base...)
	out = append(out, bytes.Repeat([]byte{0x55}, want-len(base))...)
	return out
}

// uploadRequest builds a multipart POST carrying the given fields and an
// optional "photo" file part (photoName "" means no file part), with
// LimitBody applied first — the frozen parse-order contract Photo documents.
func uploadRequest(t *testing.T, fields map[string]string, photoName string, photoData []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("multipart field %s: %v", k, err)
		}
	}
	if photoName != "" {
		fw, err := mw.CreateFormFile(photoField, photoName)
		if err != nil {
			t.Fatalf("multipart file %s: %v", photoName, err)
		}
		if _, err := fw.Write(photoData); err != nil {
			t.Fatalf("multipart write %s: %v", photoName, err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/admin/lights", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	LimitBody(req)
	return req
}

// uploadPhoto runs one happy-path upload and fails the test on error.
func uploadPhoto(t *testing.T, s *Server, name string, data []byte) *UploadedPhoto {
	t.Helper()
	req := uploadRequest(t, map[string]string{
		"csrf_token": "tok", "label": "Upload lamp", "kind": "park", "lat": "42.76", "lon": "-71.03",
	}, name, data)
	up, err := s.Photo(req)
	if err != nil {
		t.Fatalf("Photo(%s): %v", name, err)
	}
	if up == nil {
		t.Fatalf("Photo(%s): got nil, want a stored photo", name)
	}
	return up
}

// uploadCountFiles counts regular files under dir; a missing directory holds
// 0 files.
func uploadCountFiles(t *testing.T, dir string) int {
	t.Helper()
	var n int
	err := filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return n
}

// uploadHasJpegMarker walks the JPEG marker segments from SOI until the scan
// starts and reports whether the requested marker segment is present.
func uploadHasJpegMarker(data []byte, want byte) bool {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return false
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return false
		}
		m := data[i+1]
		if m == 0x01 || (m >= 0xD0 && m <= 0xD7) { // no payload length
			i += 2
			continue
		}
		if m == 0xD9 || m == 0xDA { // EOI / SOS: metadata only precedes the scan
			return false
		}
		segLen := int(data[i+2])<<8 | int(data[i+3])
		if segLen < 2 || i+2+segLen > len(data) {
			return false
		}
		if m == want {
			return true
		}
		i += 2 + segLen
	}
	return false
}

func TestPhotoPortraitOrient6Becomes1600EdgeJPEGWithoutEXIF(t *testing.T) {
	s, db, dir := newUploadServer(t)
	src := uploadFixture(t, "portrait-orient6-gps.jpg")
	if !uploadHasJpegMarker(src, 0xE1) {
		t.Fatal("the source fixture itself carries no APP1 segment; the strip assertion would prove nothing")
	}

	up := uploadPhoto(t, s, "portrait-orient6-gps.jpg", src)
	uploadAssertStoredLayout(t, dir, up)
	raw := uploadAssertReencodedJPEG(t, dir, up)
	uploadAssertRowMatchesFile(t, db, up, raw)

	// A second upload mints a different path (server-generated names).
	up2 := uploadPhoto(t, s, "portrait-orient6-gps.jpg", src)
	if up2.Path == up.Path {
		t.Errorf("second upload reused path %q; names must be unique per upload", up.Path)
	}
}

// uploadAssertStoredLayout pins the frozen storage contract for one stored
// photo: exact processed dims (T-202: 800x600 for the Orientation=6 fixture),
// the YYYY/MM/<16-hex>.jpg layout, and plain 0755/0644 permissions (asserted
// modulo any umask: never group/world-writable).
func uploadAssertStoredLayout(t *testing.T, dir string, up *UploadedPhoto) {
	t.Helper()
	if up.Width != 800 || up.Height != 600 {
		t.Fatalf("stored photo is %dx%d, want exactly 800x600", up.Width, up.Height)
	}
	if !uploadRePath.MatchString(up.Path) {
		t.Fatalf("stored path %q does not match YYYY/MM/<16-hex>.jpg", up.Path)
	}
	file := filepath.Join(dir, filepath.FromSlash(up.Path))
	if st, err := os.Stat(file); err != nil {
		t.Fatalf("stat stored photo: %v", err)
	} else if st.Mode().Perm()&0o022 != 0 {
		t.Errorf("stored photo mode %s is group/world-writable, want plain 0644", st.Mode())
	}
	if st, err := os.Stat(filepath.Dir(file)); err != nil {
		t.Fatalf("stat year dir: %v", err)
	} else if st.Mode().Perm()&0o022 != 0 {
		t.Errorf("photo dir mode %s is group/world-writable, want plain 0755", st.Mode())
	}
}

// uploadAssertReencodedJPEG reads the stored file back and pins the
// re-encoding contract: JPEG from SOI, no APP1 segment at all (so no EXIF
// and no GPS), exactly 800x600. Returns the stored bytes.
func uploadAssertReencodedJPEG(t *testing.T, dir string, up *UploadedPhoto) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(up.Path)))
	if err != nil {
		t.Fatalf("read stored photo: %v", err)
	}
	if len(raw) < 2 || raw[0] != 0xFF || raw[1] != 0xD8 {
		t.Error("stored photo does not start with the JPEG SOI marker")
	}
	if uploadHasJpegMarker(raw, 0xE1) {
		t.Error("stored photo still carries an APP1 segment: metadata must be stripped")
	}
	if bytes.Contains(raw, []byte("Exif")) {
		t.Error("stored photo contains the string Exif")
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode stored photo: %v", err)
	}
	if cfg.Width != 800 || cfg.Height != 600 {
		t.Errorf("stored photo decodes as %dx%d, want 800x600", cfg.Width, cfg.Height)
	}
	return raw
}

// uploadAssertRowMatchesFile pins the photos row against the stored file:
// path, dimensions, byte count and the sha256 of the RE-ENCODED bytes.
func uploadAssertRowMatchesFile(t *testing.T, db *store.DB, up *UploadedPhoto, raw []byte) {
	t.Helper()
	sum := sha256.Sum256(raw)
	row, err := db.PhotoByID(context.Background(), up.ID)
	if err != nil {
		t.Fatalf("PhotoByID(%d): %v", up.ID, err)
	}
	if row.Path != up.Path {
		t.Errorf("row path %q, want %q", row.Path, up.Path)
	}
	if row.Width != up.Width || row.Height != up.Height {
		t.Errorf("row dims %dx%d, want %dx%d", row.Width, row.Height, up.Width, up.Height)
	}
	if row.Bytes != int64(len(raw)) {
		t.Errorf("row bytes %d, want %d", row.Bytes, len(raw))
	}
	if row.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("row sha256 %s is not the digest of the re-encoded stored bytes", row.SHA256)
	}
}

func TestPhotoAbsentFieldIsNotAnErrorAndStoresNothing(t *testing.T) {
	s, db, dir := newUploadServer(t)
	fields := map[string]string{"csrf_token": "tok", "label": "No photo lamp", "kind": "park"}

	// No photo part at all.
	req := uploadRequest(t, fields, "", nil)
	up, err := s.Photo(req)
	if err != nil {
		t.Fatalf("Photo without a photo field: %v", err)
	}
	if up != nil {
		t.Fatalf("Photo without a photo field returned %+v, want nil", up)
	}

	// An empty file part (a file input left empty) is equally absent.
	req = uploadRequest(t, fields, "empty.jpg", nil)
	if up, err = s.Photo(req); err != nil || up != nil {
		t.Fatalf("Photo with an empty file part: photo=%v err=%v, want nil nil", up, err)
	}

	// A plain value named photo (no filename) is not an upload either.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField(photoField, ""); err != nil {
		t.Fatalf("write field: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/x", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	LimitBody(req)
	if up, err = s.Photo(req); err != nil || up != nil {
		t.Fatalf("Photo with an empty value field: photo=%v err=%v, want nil nil", up, err)
	}

	if got := uploadCountFiles(t, dir); got != 0 {
		t.Errorf("photo dir holds %d files, want 0", got)
	}
	if rows, err := db.PhotoList(context.Background(), 100); err != nil {
		t.Fatalf("PhotoList: %v", err)
	} else if len(rows) != 0 {
		t.Errorf("photo table holds %d rows, want 0", len(rows))
	}
}

func TestPhotoOversizeBodyYieldsErrTooLargeAndLeavesNoFile(t *testing.T) {
	s, db, dir := newUploadServer(t)
	// Valid JPEG + padding after EOI, past the 15 MB cap — built in memory.
	oversize := uploadOversizeJPEG(t, 16<<20)

	req := uploadRequest(t, map[string]string{"label": "Big lamp"}, "big.jpg", oversize)
	up, err := s.Photo(req)
	if up != nil {
		t.Fatalf("oversize upload returned %+v, want nil", up)
	}
	if !errors.Is(err, ErrPhotoTooLarge) {
		t.Fatalf("oversize upload err = %v, want ErrPhotoTooLarge", err)
	}
	if got := uploadCountFiles(t, dir); got != 0 {
		t.Errorf("photo dir holds %d files after the rejected upload, want 0", got)
	}
	if rows, err := db.PhotoList(context.Background(), 100); err != nil {
		t.Fatalf("PhotoList: %v", err)
	} else if len(rows) != 0 {
		t.Errorf("photo table holds %d rows, want 0", len(rows))
	}
	status, msg := PhotoProblem(err)
	if status != http.StatusRequestEntityTooLarge {
		t.Errorf("PhotoProblem status = %d, want 413", status)
	}
	if msg != "Photo must be at most 15 MB" {
		t.Errorf("PhotoProblem message = %q, want the frozen size message", msg)
	}
}

func TestPhotoTextAsJPEGYieldsErrUnsupportedAndLeavesNoFile(t *testing.T) {
	s, _, dir := newUploadServer(t)
	textAs := uploadFixture(t, "text-as.jpg")

	req := uploadRequest(t, map[string]string{"label": "Text lamp"}, "text-as.jpg", textAs)
	up, err := s.Photo(req)
	if up != nil {
		t.Fatalf("text upload returned %+v, want nil", up)
	}
	if !errors.Is(err, ErrPhotoUnsupported) {
		t.Fatalf("text upload err = %v, want ErrPhotoUnsupported", err)
	}
	if got := uploadCountFiles(t, dir); got != 0 {
		t.Errorf("photo dir holds %d files after the rejected upload, want 0", got)
	}
	status, msg := PhotoProblem(err)
	if status != http.StatusUnsupportedMediaType {
		t.Errorf("PhotoProblem status = %d, want 415", status)
	}
	if msg != "Photo must be JPEG, PNG or WebP" {
		t.Errorf("PhotoProblem message = %q, want the plain unsupported message (not the HEIC one)", msg)
	}
}

func TestPhotoPixelBombYieldsErrPixelBombWithoutDecoding(t *testing.T) {
	s, _, dir := newUploadServer(t)
	// The 45-byte bomb claims 50000x50000 (2.5e9 px > the 40 MP cap); the
	// guard must refuse it from the header alone — a full decode would try
	// to allocate ~10 GB of raster and the refusal must still be the
	// pixel error, never a decode failure.
	bomb := uploadFixture(t, "pixelbomb.png")

	req := uploadRequest(t, map[string]string{"label": "Bomb lamp"}, "pixelbomb.png", bomb)
	up, err := s.Photo(req)
	if up != nil {
		t.Fatalf("bomb upload returned %+v, want nil", up)
	}
	if !errors.Is(err, ErrPhotoTooManyPx) {
		t.Fatalf("bomb upload err = %v, want ErrPhotoTooManyPx", err)
	}
	if got := uploadCountFiles(t, dir); got != 0 {
		t.Errorf("photo dir holds %d files after the rejected upload, want 0", got)
	}
	status, msg := PhotoProblem(err)
	if status != http.StatusUnsupportedMediaType {
		t.Errorf("PhotoProblem status = %d, want 415", status)
	}
	if msg != "Photo has too many pixels" {
		t.Errorf("PhotoProblem message = %q, want the frozen pixel message", msg)
	}
}

func TestPhotoHEICYieldsErrUnsupportedWithTheHEICMessage(t *testing.T) {
	s, _, dir := newUploadServer(t)
	heic := uploadFixture(t, "heic.bin")

	req := uploadRequest(t, map[string]string{"label": "Heic lamp"}, "heic.bin", heic)
	up, err := s.Photo(req)
	if up != nil {
		t.Fatalf("HEIC upload returned %+v, want nil", up)
	}
	if !errors.Is(err, ErrPhotoUnsupported) {
		t.Fatalf("HEIC upload err = %v, want ErrPhotoUnsupported", err)
	}
	if got := uploadCountFiles(t, dir); got != 0 {
		t.Errorf("photo dir holds %d files after the rejected upload, want 0", got)
	}
	status, msg := PhotoProblem(err)
	if status != http.StatusUnsupportedMediaType {
		t.Errorf("PhotoProblem status = %d, want 415", status)
	}
	const want = "Photo must be JPEG, PNG or WebP — please choose JPEG instead of HEIC"
	if msg != want {
		t.Errorf("PhotoProblem message = %q, want the frozen HEIC wording %q", msg, want)
	}
}

func TestPhotoDirCountsUnchangedOnEveryError(t *testing.T) {
	s, db, dir := newUploadServer(t)
	oversize := uploadOversizeJPEG(t, 16<<20)
	rejects := []struct {
		name string
		file string
		data []byte
	}{
		{"heic", "heic.bin", uploadFixture(t, "heic.bin")},
		{"text-as-jpg", "text-as.jpg", uploadFixture(t, "text-as.jpg")},
		{"pixelbomb", "pixelbomb.png", uploadFixture(t, "pixelbomb.png")},
		{"oversize", "big.jpg", oversize},
		{"garbage", "junk.jpg", []byte("this is not an image at all, really")},
		{"empty", "empty.jpg", nil},
	}
	if before := uploadCountFiles(t, dir); before != 0 {
		t.Fatalf("photo dir holds %d files before any upload, want 0", before)
	}
	for _, r := range rejects {
		req := uploadRequest(t, map[string]string{"label": "Stray " + r.name}, r.file, r.data)
		up, err := s.Photo(req)
		if up != nil || err == nil && r.file != "empty.jpg" {
			t.Fatalf("%s upload: photo=%v err=%v, want a rejection", r.name, up, err)
		}
		if got := uploadCountFiles(t, dir); got != 0 {
			t.Fatalf("%s upload left stray files: photo dir held 0, now %d", r.name, got)
		}
		if rows, err := db.PhotoList(context.Background(), 100); err != nil {
			t.Fatalf("PhotoList: %v", err)
		} else if len(rows) != 0 {
			t.Fatalf("%s upload left %d photo rows, want 0", r.name, len(rows))
		}
	}
	// The happy path still works after all the rejections.
	uploadPhoto(t, s, "landscape.jpg", uploadFixture(t, "landscape.jpg"))
	if got := uploadCountFiles(t, dir); got != 1 {
		t.Errorf("after the happy-path upload the photo dir holds %d files, want exactly 1", got)
	}
}

func TestServedPhotoHasContentTypeJPEGAndImmutableCacheHeader(t *testing.T) {
	s, _, dir := newUploadServer(t)
	up := uploadPhoto(t, s, "landscape.jpg", uploadFixture(t, "landscape.jpg"))
	stored, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(up.Path)))
	if err != nil {
		t.Fatalf("read stored photo: %v", err)
	}

	h := s.Handler()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/photos/"+up.Path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /photos/%s: status %d, want 200; body: %.200s", up.Path, rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q, want exactly image/jpeg", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want the frozen immutable header", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), stored) {
		t.Error("served bytes differ from the stored file")
	}

	// HEAD works through the same route (mux: GET patterns match HEAD).
	req = httptest.NewRequestWithContext(context.Background(), http.MethodHead, "/photos/"+up.Path, nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("HEAD /photos/%s: status %d, want 200", up.Path, rec.Code)
	}

	// A missing photo answers 404.
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/photos/2026/01/ffffffffffffffff.jpg", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing photo: status %d, want 404", rec.Code)
	}
}

func TestPhotoTraversalAttempt404s(t *testing.T) {
	s, _, dir := newUploadServer(t)
	// A secret beside PHOTO_DIR (inside its temp parent, outside the photo
	// root) that a traversal must never serve.
	secret := filepath.Join(dir, "..", "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatalf("plant secret: %v", err)
	}
	h := s.Handler()

	// Percent-encoded dot-dot values DO reach the handler (the mux only
	// cleans literal dots); the resolver must clamp them inside PHOTO_DIR.
	for _, target := range []string{
		"/photos/%2e%2e%2fsecret.txt",
		"/photos/%2e%2e%2f%2e%2e%2fgo.mod",
		"/photos/sub/%2e%2e%2f%2e%2e%2fsecret.txt",
		"/photos/..%5c..%5cgo.mod", // backslashes are plain bytes here
		"/photos/.%2e/secret.txt",
		"/photos/",      // bare root
		"/photos/..%00", // NUL byte
	} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Errorf("GET %s: status 200, want a 404 family answer; body: %.100s", target, rec.Body.String())
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("top secret")) {
			t.Errorf("GET %s served the secret outside PHOTO_DIR", target)
		}
	}

	// White-box: the resolver clamps every dot-dot climb inside the photo
	// root — the resolved path can never name the outside secret.
	for _, val := range []string{"../../secret.txt", "a/../../../secret.txt", "/../../secret.txt", "\x00"} {
		abs, ok := photoAbsPath(dir, val)
		if !ok {
			continue // rejected outright (NUL) — also fine
		}
		if abs != dir && !strings.HasPrefix(abs, dir+string(filepath.Separator)) {
			t.Errorf("photoAbsPath(%q) = %q, which escapes %q", val, abs, dir)
		}
		if _, err := os.Stat(abs); err == nil {
			t.Errorf("photoAbsPath(%q) = %q, which exists — traversal may have escaped", val, abs)
		}
	}

	// Literal dot-dot through the mux: Go's mux redirects non-canonical
	// paths to their cleaned form, so the traversal request resolves to a
	// 404 — never the file.
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	client := &http.Client{}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/photos/../../go.mod", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET traversal: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("traversal /photos/../../go.mod resolves to status %d, want a final 404; body: %.100s", resp.StatusCode, body)
	}
	if bytes.Contains(body, []byte("module ")) {
		t.Error("traversal served the repo's go.mod")
	}
}

func TestPhotoProblemMappingTable(t *testing.T) {
	heic := heicUnsupported{ErrPhotoUnsupported}
	cases := []struct {
		name   string
		err    error
		status int
		msg    string
	}{
		{"too large sentinel", ErrPhotoTooLarge, http.StatusRequestEntityTooLarge, "Photo must be at most 15 MB"},
		{"too large wrapped", fmt.Errorf("photo: body: %w", ErrPhotoTooLarge), http.StatusRequestEntityTooLarge, "Photo must be at most 15 MB"},
		{"unsupported sentinel", ErrPhotoUnsupported, http.StatusUnsupportedMediaType, "Photo must be JPEG, PNG or WebP"},
		{"unsupported wrapped", fmt.Errorf("read: %w", ErrPhotoUnsupported), http.StatusUnsupportedMediaType, "Photo must be JPEG, PNG or WebP"},
		{"heic marked", heic, http.StatusUnsupportedMediaType, "Photo must be JPEG, PNG or WebP — please choose JPEG instead of HEIC"},
		{"heic deep wrapped", fmt.Errorf("a: %w", fmt.Errorf("b: %w", heic)), http.StatusUnsupportedMediaType, "Photo must be JPEG, PNG or WebP — please choose JPEG instead of HEIC"},
		{"too many px sentinel", ErrPhotoTooManyPx, http.StatusUnsupportedMediaType, "Photo has too many pixels"},
		{"too many px wrapped", fmt.Errorf("cfg: %w", ErrPhotoTooManyPx), http.StatusUnsupportedMediaType, "Photo has too many pixels"},
		{"max bytes raw", &http.MaxBytesError{Limit: MaxPhotoBytes}, http.StatusRequestEntityTooLarge, "Photo must be at most 15 MB"},
		{"unknown error", errors.New("boom"), http.StatusInternalServerError, "internal error"},
		{"nil error", nil, http.StatusInternalServerError, "internal error"},
	}
	for _, c := range cases {
		status, msg := PhotoProblem(c.err)
		if status != c.status || msg != c.msg {
			t.Errorf("%s: PhotoProblem = (%d, %q), want (%d, %q)", c.name, status, msg, c.status, c.msg)
		}
	}
}

// TestPhotoFieldsReadableAfterPhotoParses pins the parse-order contract
// T-205 compiles against: Photo performs the request's ONE multipart parse,
// and the caller's scalar fields are readable from r.MultipartForm.Value
// afterwards.
func TestPhotoFieldsReadableAfterPhotoParses(t *testing.T) {
	s, _, _ := newUploadServer(t)
	fields := map[string]string{
		"csrf_token": "tok-123", "label": "Contract lamp", "kind": "building",
		"lat": "42.7617", "lon": "-71.0303", "asset_no": "GP-9001",
	}
	req := uploadRequest(t, fields, "landscape.jpg", uploadFixture(t, "landscape.jpg"))
	up, err := s.Photo(req)
	if err != nil {
		t.Fatalf("Photo: %v", err)
	}
	if up == nil {
		t.Fatal("Photo returned nil for a valid upload")
	}
	for k, want := range fields {
		got := req.MultipartForm.Value[k]
		if len(got) != 1 || got[0] != want {
			t.Errorf("field %s = %v after Photo, want [%q]", k, got, want)
		}
	}
	// A caller-side ParseMultipartForm after Photo is a no-op returning nil.
	if err := req.ParseMultipartForm(photoMaxMemory); err != nil {
		t.Errorf("ParseMultipartForm after Photo: %v, want a no-op nil", err)
	}
}

// TestPhotoReusesFormTheCallerParsed covers the reverse order: a handler
// that parsed the form before calling Photo still gets the photo processed
// from the already-parsed field.
func TestPhotoReusesFormTheCallerParsed(t *testing.T) {
	s, _, dir := newUploadServer(t)
	req := uploadRequest(t, map[string]string{"label": "Pre-parsed lamp"}, "tiny.png", uploadFixture(t, "tiny.png"))
	if err := req.ParseMultipartForm(photoMaxMemory); err != nil {
		t.Fatalf("caller ParseMultipartForm: %v", err)
	}
	up, err := s.Photo(req)
	if err != nil {
		t.Fatalf("Photo: %v", err)
	}
	if up == nil {
		t.Fatal("Photo returned nil for a valid pre-parsed upload")
	}
	if got := uploadCountFiles(t, dir); got != 1 {
		t.Errorf("photo dir holds %d files, want exactly 1", got)
	}
}

// TestPhotoNonMultipartIsAWiringErrorNotASentinel: a body that never was
// multipart must not be confused with a photo-format problem.
func TestPhotoNonMultipartIsAWiringErrorNotASentinel(t *testing.T) {
	s, _, dir := newUploadServer(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/x", strings.NewReader("label=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	LimitBody(req)
	up, err := s.Photo(req)
	if up != nil || err == nil {
		t.Fatalf("Photo on a non-multipart request: photo=%v err=%v, want a wrapped error", up, err)
	}
	for _, sentinel := range []error{ErrPhotoTooLarge, ErrPhotoUnsupported, ErrPhotoTooManyPx} {
		if errors.Is(err, sentinel) {
			t.Errorf("non-multipart request err = %v, which wrongly matches %v", err, sentinel)
		}
	}
	if status, _ := PhotoProblem(err); status != http.StatusInternalServerError {
		t.Errorf("PhotoProblem status = %d, want 500 for a wiring error", status)
	}
	if got := uploadCountFiles(t, dir); got != 0 {
		t.Errorf("photo dir holds %d files, want 0", got)
	}
}
