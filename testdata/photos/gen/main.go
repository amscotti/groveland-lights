// Command gen writes (and re-verifies) the M2 photo fixtures used by the
// A-206/A-207 acceptance drafts. Run from the repo root:
//
//	go run ./testdata/photos/gen
//
// Everything is deterministic: the committed bytes are exactly what a fresh
// run reproduces, so the fixtures are auditable and reproducible. The program
// exits non-zero if any self-check fails, so a run is its own audit.
//
// Why a generator: the standard library's image/jpeg writes no EXIF, so the
// flagship fixtures (portrait JPEGs carrying EXIF Orientation 6/8 plus a GPS
// IFD) are built by hand — APP1 segment bytes appended right after the SOI
// marker of an ordinarily encoded JPEG. The pixel bomb is a PNG whose IHDR
// claims 50000x50000 while the file never carries a raster at all.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
)

// fixture geometry: the flagship portrait fixtures are 600x800; with EXIF
// Orientation 6 (or 8) applied the processed output must come out 800x600 —
// wider than tall — which is what the photo pipeline contract asserts.
const (
	portraitW, portraitH   = 600, 800
	landscapeW, landscapeH = 320, 200
	bombDim                = 50000 // IHDR-claimed width AND height
	maxFixtureBytes        = 15 << 20
)

func main() {
	outDir, err := resolveOutDir(os.Args[1:])
	if err != nil {
		fail("%v", err)
	}
	written, err := writeFixtures(outDir)
	if err != nil {
		fail("write fixtures: %v", err)
	}
	if err := verifyFixtures(outDir); err != nil {
		fail("verify: %v", err)
	}
	fmt.Printf("gen: %d fixtures verified in %s\n", written, outDir)
}

// resolveOutDir finds the fixtures directory: an explicit argument wins
// (used to mirror the fixtures into the holdout repo); otherwise the repo
// root (the directory that owns migrations/) is found by walking up from the
// working directory, so the command works from the repo root and anywhere
// inside it.
func resolveOutDir(args []string) (string, error) {
	if len(args) > 0 {
		if err := os.MkdirAll(args[0], 0o755); err != nil {
			return "", err
		}
		return args[0], nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 8; i++ {
		if st, err := os.Stat(filepath.Join(dir, "migrations")); err == nil && st.IsDir() {
			return filepath.Join(dir, "testdata", "photos"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("repo root not found (no migrations/ directory on any parent; pass the output dir as an argument)")
}

// --- fixture builders ---

// gradientImage returns a deterministic RGB image whose channels are derived
// from the coordinates, so orientation is visually detectable: the red channel
// grows left-to-right, the green channel top-to-bottom.
func gradientImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x * 255 / max(w-1, 1)),
				G: uint8(y * 255 / max(h-1, 1)),
				B: 96,
				A: 255,
			})
		}
	}
	return img
}

// encodeJPEG encodes img at a fixed quality; the encoder is deterministic.
func encodeJPEG(img image.Image) []byte {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		panic("fixture build: jpeg encode: " + err.Error())
	}
	return buf.Bytes()
}

// withExifAPP1 returns jpegBytes with one EXIF APP1 segment inserted directly
// after the SOI marker. The segment carries the given Orientation value and a
// minimal GPS IFD (a single GPSLatitudeRef = "N" entry) — enough for any
// scanner to report "GPS data present".
func withExifAPP1(jpegBytes []byte, orientation uint16) []byte {
	seg := buildExifAPP1(orientation)
	out := make([]byte, 0, len(jpegBytes)+len(seg))
	out = append(out, jpegBytes[:2]...) // SOI
	out = append(out, seg...)
	out = append(out, jpegBytes[2:]...)
	return out
}

// buildExifAPP1 hand-assembles the APP1 segment. TIFF block (little-endian):
//
//	 0  "II"  0x002A  IFD0-offset = 8
//	 8  IFD0, 2 entries:
//	     0x0112 Orientation  SHORT count 1 = orientation
//	     0x8825 GPSInfo      LONG  count 1 = GPS IFD offset
//	38  GPS IFD, 1 entry:
//	     0x0001 GPSLatitudeRef ASCII count 2 = "N\x00"
//
// 8 + (2 + 2*12 + 4) = 38; GPS IFD adds 2 + 1*12 + 4 = 18 bytes -> 56 total.
func buildExifAPP1(orientation uint16) []byte {
	const ifd0 = 8
	const gps = 38
	tiff := make([]byte, 56)

	copy(tiff[0:], "II")
	binary.LittleEndian.PutUint16(tiff[2:], 0x002A)
	binary.LittleEndian.PutUint32(tiff[4:], ifd0)

	binary.LittleEndian.PutUint16(tiff[ifd0:], 2) // two IFD0 entries
	e0 := tiff[10:22]
	binary.LittleEndian.PutUint16(e0[0:], 0x0112) // Orientation
	binary.LittleEndian.PutUint16(e0[2:], 3)      // SHORT
	binary.LittleEndian.PutUint32(e0[4:], 1)      // count 1
	binary.LittleEndian.PutUint16(e0[8:], orientation)
	// e0[10:12] zero padding of the in-place value field.
	e1 := tiff[22:34]
	binary.LittleEndian.PutUint16(e1[0:], 0x8825) // GPSInfo pointer
	binary.LittleEndian.PutUint16(e1[2:], 4)      // LONG
	binary.LittleEndian.PutUint32(e1[4:], 1)      // count 1
	binary.LittleEndian.PutUint32(e1[8:], gps)
	binary.LittleEndian.PutUint32(tiff[34:], 0) // no next IFD

	binary.LittleEndian.PutUint16(tiff[gps:], 1) // one GPS entry
	g := tiff[gps+2 : gps+2+12]
	binary.LittleEndian.PutUint16(g[0:], 0x0001) // GPSLatitudeRef
	binary.LittleEndian.PutUint16(g[2:], 2)      // ASCII
	binary.LittleEndian.PutUint32(g[4:], 2)      // count 2: "N\x00"
	g[8] = 'N'
	// g[9:12] zero padding.
	binary.LittleEndian.PutUint32(tiff[gps+2+12:], 0) // no next IFD

	payload := make([]byte, 0, 6+len(tiff))
	payload = append(payload, "Exif\x00\x00"...)
	payload = append(payload, tiff...)
	seg := make([]byte, 0, 4+len(payload))
	seg = append(seg, 0xFF, 0xE1)
	seg = append(seg, byte(len(payload)+2)>>8, byte(len(payload)+2)) // segment length incl. itself
	seg = append(seg, payload...)
	return seg
}

// buildPixelBombPNG crafts a PNG whose IHDR claims bombDim x bombDim pixels
// while the file carries no raster data at all — 8 (signature) + 25 (IHDR) +
// 12 (IEND) bytes. png.DecodeConfig reads only the IHDR, so it reports a
// >40 MP image; a full decode never gets a chance to allocate.
func buildPixelBombPNG() []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'})

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], bombDim) // width
	binary.BigEndian.PutUint32(ihdr[4:], bombDim) // height
	ihdr[8] = 8                                   // bit depth
	ihdr[9] = 2                                   // color type: truecolor RGB
	ihdr[10] = 0                                  // compression: deflate
	ihdr[11] = 0                                  // filter
	ihdr[12] = 0                                  // no interlace
	writeChunk(&buf, "IHDR", ihdr)
	writeChunk(&buf, "IEND", nil)
	return buf.Bytes()
}

func writeChunk(buf *bytes.Buffer, typ string, data []byte) {
	var lenb [4]byte
	binary.BigEndian.PutUint32(lenb[:], uint32(len(data)))
	buf.Write(lenb[:])
	buf.WriteString(typ)
	buf.Write(data)
	var crcb [4]byte
	binary.BigEndian.PutUint32(crcb[:], crc32.ChecksumIEEE(append([]byte(typ), data...)))
	buf.Write(crcb[:])
}

// tinyPNG is an ordinary 1x1 RGBA PNG.
func tinyPNG() []byte {
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 0x7F, G: 0xB2, B: 0x33, A: 0xFF})
	if err := png.Encode(&buf, img); err != nil {
		panic("fixture build: png encode: " + err.Error())
	}
	return buf.Bytes()
}

// heicSniff carries an ISO-BMFF ftyp box with the brand "heic" at offset 8
// (box type "ftyp" at offset 4). It is NOT a real HEIC — the server must
// reject it on content sniffing, which is exactly what A-207 exercises.
func heicSniff() []byte {
	b := make([]byte, 16)
	binary.BigEndian.PutUint32(b[0:], 16) // box size
	copy(b[4:], "ftyp")
	copy(b[8:], "heic")
	binary.BigEndian.PutUint32(b[12:], 0) // minor version
	return b
}

// --- writers ---

func writeFixtures(outDir string) (int, error) {
	portrait := encodeJPEG(gradientImage(portraitW, portraitH))
	landscape := encodeJPEG(gradientImage(landscapeW, landscapeH))
	textAs := []byte("This is not a JPEG. It is plain text wearing a .jpg name, so the\nserver must reject it on sniffing (A-207).\n")
	files := map[string][]byte{
		"portrait-orient6-gps.jpg": withExifAPP1(portrait, 6),
		"portrait-orient8-gps.jpg": withExifAPP1(portrait, 8),
		"landscape.jpg":            landscape,
		"tiny.png":                 tinyPNG(),
		"pixelbomb.png":            buildPixelBombPNG(),
		"text-as.jpg":              textAs,
		"heic.bin":                 heicSniff(),
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, err
	}
	n := 0
	for _, name := range []string{
		"portrait-orient6-gps.jpg", "portrait-orient8-gps.jpg", "landscape.jpg",
		"tiny.png", "pixelbomb.png", "text-as.jpg", "heic.bin",
	} {
		data := files[name]
		if len(data) > maxFixtureBytes {
			return n, fmt.Errorf("%s: %d bytes exceeds the %d-byte commit cap", name, len(data), maxFixtureBytes)
		}
		if err := os.WriteFile(filepath.Join(outDir, name), data, 0o644); err != nil {
			return n, err
		}
		sum := sha256.Sum256(data)
		fmt.Printf("gen: wrote %s (%d bytes, sha256 %s)\n", name, len(data), hex.EncodeToString(sum[:]))
		n++
	}
	return n, nil
}

// --- self-verification (every run audits the bytes it just wrote) ---

func verifyFixtures(outDir string) error {
	for name, check := range map[string]func([]byte) error{
		"portrait-orient6-gps.jpg": portraitCheck(6),
		"portrait-orient8-gps.jpg": portraitCheck(8),
		"landscape.jpg":            landscapeCheck,
		"tiny.png":                 tinyCheck,
		"pixelbomb.png":            bombCheck,
		"text-as.jpg":              textAsCheck,
		"heic.bin":                 heicCheck,
	} {
		data, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			return err
		}
		if err := check(data); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		fmt.Printf("gen: verified %s\n", name)
	}
	return nil
}

func portraitCheck(wantOrientation uint16) func([]byte) error {
	return func(data []byte) error {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("does not decode as JPEG: %w", err)
		}
		if cfg.Width != portraitW || cfg.Height != portraitH {
			return fmt.Errorf("dims %dx%d, want %dx%d (EXIF orientation must NOT change DecodeConfig)", cfg.Width, cfg.Height, portraitW, portraitH)
		}
		orient, hasGPS, err := parseExifAPP1(data)
		if err != nil {
			return err
		}
		if orient != wantOrientation {
			return fmt.Errorf("EXIF Orientation = %d, want %d", orient, wantOrientation)
		}
		if !hasGPS {
			return errors.New("no GPS IFD found in the APP1 segment")
		}
		return nil
	}
}

func landscapeCheck(data []byte) error {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("does not decode as JPEG: %w", err)
	}
	if cfg.Width != landscapeW || cfg.Height != landscapeH {
		return fmt.Errorf("dims %dx%d, want %dx%d", cfg.Width, cfg.Height, landscapeW, landscapeH)
	}
	if bytes.Contains(data, []byte("Exif\x00\x00")) {
		return errors.New("ordinary JPEG must carry no EXIF APP1")
	}
	return nil
}

func tinyCheck(data []byte) error {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("does not decode as PNG: %w", err)
	}
	if cfg.Width != 1 || cfg.Height != 1 {
		return fmt.Errorf("dims %dx%d, want 1x1", cfg.Width, cfg.Height)
	}
	return nil
}

func bombCheck(data []byte) error {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("DecodeConfig must read the crafted IHDR: %w", err)
	}
	px := uint64(cfg.Width) * uint64(cfg.Height)
	if px <= 40_000_000 {
		return fmt.Errorf("DecodeConfig sees %d px, want > 40 MP", px)
	}
	if len(data) > 4096 {
		return fmt.Errorf("file is %d bytes; a pixel bomb must stay tiny (headers only)", len(data))
	}
	return nil
}

func textAsCheck(data []byte) error {
	if _, err := jpeg.DecodeConfig(bytes.NewReader(data)); err == nil {
		return errors.New("plain text must not decode as JPEG")
	}
	if !bytes.Contains(data, []byte("not a JPEG")) {
		return errors.New("content marker missing")
	}
	return nil
}

func heicCheck(data []byte) error {
	if len(data) < 16 {
		return fmt.Errorf("only %d bytes", len(data))
	}
	if string(data[4:8]) != "ftyp" {
		return errors.New("no ftyp box type at offset 4")
	}
	if string(data[8:12]) != "heic" {
		return errors.New("brand at offset 8 is not heic")
	}
	return nil
}

// parseExifAPP1 reads back what withExifAPP1 wrote: walks to the first APP1
// segment, checks the Exif header, then parses the little-endian TIFF IFD0
// for the Orientation value and the GPSInfo pointer (a GPS IFD with at least
// one entry counts as "GPS present").
func parseExifAPP1(data []byte) (orientation uint16, hasGPS bool, err error) {
	if len(data) < 6 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, false, errors.New("not a JPEG (no SOI)")
	}
	if data[2] != 0xFF || data[3] != 0xE1 {
		return 0, false, errors.New("no APP1 directly after SOI")
	}
	segLen := int(binary.BigEndian.Uint16(data[4:6]))
	payload := data[6 : 4+segLen]
	if len(payload) < 6 || string(payload[:6]) != "Exif\x00\x00" {
		return 0, false, errors.New("APP1 is not an Exif segment")
	}
	tiff := payload[6:]
	if len(tiff) < 8 || string(tiff[:2]) != "II" {
		return 0, false, errors.New("no little-endian TIFF header")
	}
	ifd0 := int(binary.LittleEndian.Uint32(tiff[4:8]))
	if ifd0+2 > len(tiff) {
		return 0, false, errors.New("IFD0 offset out of bounds")
	}
	entries := int(binary.LittleEndian.Uint16(tiff[ifd0 : ifd0+2]))
	for i := 0; i < entries; i++ {
		base := ifd0 + 2 + i*12
		if base+12 > len(tiff) {
			return 0, false, errors.New("truncated IFD0 entry")
		}
		e := tiff[base : base+12]
		switch binary.LittleEndian.Uint16(e[0:2]) {
		case 0x0112:
			orientation = binary.LittleEndian.Uint16(e[8:10])
		case 0x8825:
			gps := int(binary.LittleEndian.Uint32(e[8:12]))
			if gps+2 <= len(tiff) && binary.LittleEndian.Uint16(tiff[gps:gps+2]) > 0 {
				hasGPS = true
			}
		}
	}
	return orientation, hasGPS, nil
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gen: "+format+"\n", args...)
	os.Exit(1)
}
