package img

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	xdraw "golang.org/x/image/draw"
)

// photoLimits mirrors the A-206 upload contract: 15 MB byte cap, 40 MP pixel
// bomb cap, 1600 px long edge.
func photoLimits() Options {
	return Options{MaxBytes: 15 << 20, MaxPixels: 40_000_000, MaxEdge: 1600}
}

// fixture reads testdata/photos/<name>, walking up from the package
// directory — the same fixtures the acceptance drafts and the HTTP layer use.
func fixture(tb testing.TB, name string) []byte {
	tb.Helper()
	dir, err := os.Getwd()
	if err != nil {
		tb.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		cand := filepath.Join(dir, "testdata", "photos", name)
		if b, err := os.ReadFile(cand); err == nil {
			return b
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	tb.Fatalf("fixture testdata/photos/%s not found (run from the repo)", name)
	return nil
}

// gradientPNG encodes a deterministic w×h PNG whose red channel grows
// left-to-right and green channel top-to-bottom (the fixture generator's
// exact scheme, so orientation is visually detectable).
func gradientPNG(tb testing.TB, w, h int) []byte {
	tb.Helper()
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
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		tb.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

// Quadrant colors for the orientation-matrix test: a 64×64 PNG with four
// 32×32 solid quadrants, indexed 0=A top-left, 1=B top-right, 2=C
// bottom-left, 3=D bottom-right. Quadrants are far larger than any 4:2:0
// chroma block, so subsampling cannot smear across them.
var quadrantColors = [4]color.RGBA{
	{R: 250, G: 16, B: 16, A: 255},  // A red
	{R: 16, G: 250, B: 16, A: 255},  // B green
	{R: 16, G: 16, B: 250, A: 255},  // C blue
	{R: 250, G: 250, B: 16, A: 255}, // D yellow
}

func quadrantPNG(tb testing.TB) []byte {
	tb.Helper()
	const q = 32
	img := image.NewRGBA(image.Rect(0, 0, 2*q, 2*q))
	for qi, c := range quadrantColors {
		x0, y0 := (qi%2)*q, (qi/2)*q
		for y := y0; y < y0+q; y++ {
			for x := x0; x < x0+q; x++ {
				img.SetRGBA(x, y, c)
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		tb.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func TestProcessFixtures(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		wantW   int
		wantH   int
		wantErr error
	}{
		{
			name:  "portrait orient6 gps serves as exactly 800x600",
			file:  "portrait-orient6-gps.jpg",
			wantW: 800, wantH: 600,
		},
		{
			name:  "portrait orient8 gps serves as exactly 800x600",
			file:  "portrait-orient8-gps.jpg",
			wantW: 800, wantH: 600,
		},
		{
			name:  "landscape without exif keeps its dimensions",
			file:  "landscape.jpg",
			wantW: 320, wantH: 200,
		},
		{
			name:  "tiny png decodes and re-encodes as a 1x1 jpeg",
			file:  "tiny.png",
			wantW: 1, wantH: 1,
		},
		{
			name:    "heic ftyp content is unsupported",
			file:    "heic.bin",
			wantErr: ErrUnsupported,
		},
		{
			name:    "text bytes wearing a jpg name are unsupported",
			file:    "text-as.jpg",
			wantErr: ErrUnsupported,
		},
		{
			name:    "pixel bomb header is refused as a pixel bomb",
			file:    "pixelbomb.png",
			wantErr: ErrPixelBomb,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Process(fixture(t, tc.file), photoLimits())
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Process(%s): error %v, want %v", tc.file, err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			if res.Width != tc.wantW || res.Height != tc.wantH {
				t.Errorf("Process(%s): Result dims %dx%d, want %dx%d", tc.file, res.Width, res.Height, tc.wantW, tc.wantH)
			}
			requireJPEGOutput(t, res, tc.wantW, tc.wantH)
		})
	}
}

// requireJPEGOutput asserts the Result bytes decode as a JPEG of exactly the
// wanted dimensions, agree with the Result fields, and carry no APP1/APP2
// metadata segment at all.
func requireJPEGOutput(t *testing.T, res Result, wantW, wantH int) {
	t.Helper()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(res.JPEG))
	if err != nil {
		t.Fatalf("output does not decode as JPEG: %v", err)
	}
	if cfg.Width != wantW || cfg.Height != wantH {
		t.Errorf("output JPEG is %dx%d, want %dx%d", cfg.Width, cfg.Height, wantW, wantH)
	}
	if res.Width != cfg.Width || res.Height != cfg.Height {
		t.Errorf("Result dims %dx%d disagree with the encoded JPEG %dx%d", res.Width, res.Height, cfg.Width, cfg.Height)
	}
	if hasJPEGAPPn(t, res.JPEG, 0xE1, 0xE2) {
		t.Errorf("output JPEG carries an APP1/APP2 segment; all metadata must be stripped")
	}
}

// TestProcessOrient6FixtureCornersComeFromTheRotatedSourceCorners pins the
// rotation direction with known pixels: orientation 6 means "rotate 90°
// clockwise", so [A B / C D] becomes [C A / D B] — the stored 600×800
// gradient's corners land in the oriented output's corners accordingly.
func TestProcessOrient6FixtureCornersComeFromTheRotatedSourceCorners(t *testing.T) {
	srcBytes := fixture(t, "portrait-orient6-gps.jpg")
	res, err := Process(srcBytes, photoLimits())
	if err != nil {
		t.Fatalf("Process(portrait-orient6-gps.jpg): %v", err)
	}
	src := mustDecode(t, srcBytes) // stored orientation: 600×800
	out := mustDecode(t, res.JPEG) // oriented: 800×600

	// The four corners of the gradient differ by ≥96 in at least one
	// channel; a tolerance of 16 absorbs JPEG noise on the smooth gradient
	// yet fails on any mis-rotation, flip or crop.
	// orient6 = rotate 90° clockwise: [A B / C D] -> [C A / D B].
	assertCornerMatches(t, out, 0, 0, src, 0, 792, "orient6 output top-left must be the stored bottom-left")
	assertCornerMatches(t, out, 792, 0, src, 0, 0, "orient6 output top-right must be the stored top-left")
	assertCornerMatches(t, out, 0, 592, src, 592, 792, "orient6 output bottom-left must be the stored bottom-right")
	assertCornerMatches(t, out, 792, 592, src, 592, 0, "orient6 output bottom-right must be the stored top-right")
}

// TestProcessOrient8FixtureCornersComeFromTheRotatedSourceCorners pins the
// opposite rotation direction: orientation 8 ("rotate 90° counter-clockwise")
// sends [A B / C D] to [B D / A C].
func TestProcessOrient8FixtureCornersComeFromTheRotatedSourceCorners(t *testing.T) {
	srcBytes := fixture(t, "portrait-orient8-gps.jpg")
	res, err := Process(srcBytes, photoLimits())
	if err != nil {
		t.Fatalf("Process(portrait-orient8-gps.jpg): %v", err)
	}
	src := mustDecode(t, srcBytes)
	out := mustDecode(t, res.JPEG)

	assertCornerMatches(t, out, 0, 0, src, 592, 0, "orient8 output top-left must be the stored top-right")
	assertCornerMatches(t, out, 792, 0, src, 592, 792, "orient8 output top-right must be the stored bottom-right")
	assertCornerMatches(t, out, 0, 592, src, 0, 0, "orient8 output bottom-left must be the stored top-left")
	assertCornerMatches(t, out, 792, 592, src, 0, 792, "orient8 output bottom-right must be the stored bottom-left")
}

// assertCornerMatches compares the mean of an 8×8 block of out at (ox, oy)
// against the mean of the corresponding 8×8 block of src at (sx, sy).
func assertCornerMatches(t *testing.T, out image.Image, ox, oy int, src image.Image, sx, sy int, label string) {
	t.Helper()
	const block = 8
	const tol = 16.0
	or, og, ob := meanRGB(t, out, ox, oy, block, block)
	sr, sg, sb := meanRGB(t, src, sx, sy, block, block)
	channels := []struct {
		name string
		o, s float64
	}{
		{"R", or, sr},
		{"G", og, sg},
		{"B", ob, sb},
	}
	for _, ch := range channels {
		if diff := math.Abs(ch.o - ch.s); diff > tol {
			t.Errorf("%s: channel %s differs by %.1f (output block at (%d,%d) mean %.1f, source block at (%d,%d) mean %.1f, tolerance %.0f)",
				label, ch.name, diff, ox, oy, ch.o, sx, sy, ch.s, tol)
		}
	}
}

func meanRGB(tb testing.TB, m image.Image, x0, y0, bw, bh int) (float64, float64, float64) {
	tb.Helper()
	var rs, gs, bs float64
	for y := y0; y < y0+bh; y++ {
		for x := x0; x < x0+bw; x++ {
			r, g, b, _ := m.At(x, y).RGBA()
			rs += float64(r >> 8)
			gs += float64(g >> 8)
			bs += float64(b >> 8)
		}
	}
	n := float64(bw * bh)
	return rs / n, gs / n, bs / n
}

func mustDecode(tb testing.TB, b []byte) image.Image {
	tb.Helper()
	m, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		tb.Fatalf("image decode: %v", err)
	}
	return m
}

// hasJPEGAPPn byte-scans the segment chain before the first scan data and
// reports whether any APPn marker in [lo, hi] is present. The inputs are our
// own encoder's output, so a malformed chain fails the test outright.
func hasJPEGAPPn(tb testing.TB, data []byte, lo, hi byte) bool {
	tb.Helper()
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			tb.Fatalf("appn scan: lost marker synchronisation at offset %d", i)
		}
		m := data[i+1]
		if m == 0xFF { // fill byte
			i++
			continue
		}
		if m == 0x01 || (m >= 0xD0 && m <= 0xD8) { // standalone markers
			i += 2
			continue
		}
		if m == 0xD9 || m == 0xDA { // metadata must precede the scan
			return false
		}
		segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if segLen < 2 {
			tb.Fatalf("appn scan: zero segment length at offset %d", i)
		}
		next := i + 2 + segLen
		if next > len(data) {
			tb.Fatalf("appn scan: segment at offset %d overruns the file", i)
		}
		if m >= lo && m <= hi {
			return true
		}
		i = next
	}
	return false
}

// TestProcessPixelBombIsRefusedWithoutAFullDecode: the >40 MP decision must
// come from DecodeConfig alone — a bomb's raster is never allocated.
func TestProcessPixelBombIsRefusedWithoutAFullDecode(t *testing.T) {
	bomb := fixture(t, "pixelbomb.png")

	// Mechanism sanity: the crafted IHDR alone reports > 40 MP.
	cfg, err := png.DecodeConfig(bytes.NewReader(bomb))
	if err != nil {
		t.Fatalf("png DecodeConfig must read the crafted IHDR: %v", err)
	}
	if px := uint64(cfg.Width) * uint64(cfg.Height); px <= 40_000_000 {
		t.Fatalf("DecodeConfig reports %d px; the bomb fixture must exceed the 40 MP cap", px)
	}

	decodes := 0
	testHookFullDecode = func() { decodes++ }
	t.Cleanup(func() { testHookFullDecode = nil })

	res, err := Process(bomb, photoLimits())
	if !errors.Is(err, ErrPixelBomb) {
		t.Fatalf("Process(pixelbomb.png): error %v, want ErrPixelBomb", err)
	}
	if res.JPEG != nil {
		t.Errorf("a refused pixel bomb must not produce output bytes")
	}
	if decodes != 0 {
		t.Fatalf("full decode ran %d times for a pixel bomb; the guard must fire from DecodeConfig before any full decode", decodes)
	}
}

// TestProcessRejectsOversizeBeforeAnyDecode: the byte cap fires first — even
// for bytes that would not decode as anything — and trailing junk past EOI
// counts toward the cap.
func TestProcessRejectsOversizeBeforeAnyDecode(t *testing.T) {
	landscape := fixture(t, "landscape.jpg")
	padded := append(bytes.Clone(landscape), bytes.Repeat([]byte{0x55}, 128)...)

	opts := photoLimits()
	opts.MaxBytes = int64(len(landscape)) // padded input is 128 bytes over
	res, err := Process(padded, opts)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Process(landscape + 128 junk bytes, cap %d): error %v, want ErrTooLarge", opts.MaxBytes, err)
	}
	if res.JPEG != nil {
		t.Errorf("a refused oversize input must not produce output bytes")
	}

	res, err = Process(bytes.Repeat([]byte{0x00}, 64), Options{MaxBytes: 8, MaxPixels: 1, MaxEdge: 1600})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Process(64 undecodable bytes, cap 8): error %v, want ErrTooLarge (the size guard runs first)", err)
	}
	if res.JPEG != nil {
		t.Errorf("a refused oversize input must not produce output bytes")
	}
}

// orientationQuadrants maps each EXIF orientation to the resulting 2×2
// arrangement of the source quadrants (0=A top-left, 1=B top-right, 2=C
// bottom-left, 3=D bottom-right), hand-derived from the EXIF spec matrices —
// e.g. orientation 6 (rotate 90° clockwise) turns [A B / C D] into [C A / D B].
var orientationQuadrants = [9][2][2]int{
	1: {{0, 1}, {2, 3}}, // A B / C D
	2: {{1, 0}, {3, 2}}, // B A / D C
	3: {{3, 2}, {1, 0}}, // D C / B A
	4: {{2, 3}, {0, 1}}, // C D / A B
	5: {{0, 2}, {1, 3}}, // A C / B D
	6: {{2, 0}, {3, 1}}, // C A / D B
	7: {{3, 1}, {2, 0}}, // D B / C A
	8: {{1, 3}, {0, 2}}, // B D / A C
}

// TestProcessAppliesEveryOrientationMatrix: a four-quadrant image tagged with
// each EXIF orientation 1..8 and pushed through the full pipeline decodes
// back with the quadrants arranged exactly as the spec demands.
func TestProcessAppliesEveryOrientationMatrix(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, mustDecode(t, quadrantPNG(t)), &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	baseJPEG := buf.Bytes()
	const q = 32 // output quadrant size
	samples := [4][2]int{{16, 16}, {48, 16}, {16, 48}, {48, 48}}

	for o := 1; o <= 8; o++ {
		t.Run(fmt.Sprintf("orientation %d quadrants match the exif matrix", o), func(t *testing.T) {
			input := insertAPP1(t, baseJPEG, exifAPP1Segment(t, uint16(o), true, 3))
			res, err := Process(input, photoLimits())
			if err != nil {
				t.Fatalf("Process(orientation %d): %v", o, err)
			}
			out := mustDecode(t, res.JPEG)
			want := orientationQuadrants[o]
			for i, sp := range samples {
				qx, qy := sp[0]/q, sp[1]/q
				wantIdx := want[qy][qx]
				wantC := quadrantColors[wantIdx]
				r, g, b, _ := out.At(sp[0], sp[1]).RGBA()
				gr, gg, gb := uint8(r>>8), uint8(g>>8), uint8(b>>8)
				const tol = 24
				if absDiff(gr, wantC.R) > tol || absDiff(gg, wantC.G) > tol || absDiff(gb, wantC.B) > tol {
					t.Errorf("sample %d at (%d,%d): got RGB(%d,%d,%d), want quadrant %d RGB(%d,%d,%d)",
						i, sp[0], sp[1], gr, gg, gb, wantIdx, wantC.R, wantC.G, wantC.B)
				}
			}
			if hasJPEGAPPn(t, res.JPEG, 0xE1, 0xE2) {
				t.Errorf("orientation %d output still carries APP1/APP2 metadata", o)
			}
		})
	}
}

func absDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

// referenceOrientation1 is the plain pipeline for upright inputs: decode,
// fit the long edge, re-encode at quality 85. Orientation 1 must be a no-op
// transform, so Process's output is byte-identical to this.
func referenceOrientation1(t *testing.T, input []byte, maxEdge int) []byte {
	t.Helper()
	src := mustDecode(t, input)
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	out := src
	if nw, nh, scaled := fitLongEdge(w, h, maxEdge); scaled {
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
		out = dst
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	return buf.Bytes()
}

// TestProcessPropertiesHoldAcrossSizesOrientationsAndAspects: over a
// deterministic corpus spanning sizes 1..2000, all 8 orientations and mixed
// aspects (flat, tall, square, extreme) — every decodable input yields dims
// ≥ 1 with long edge ≤ MaxEdge and no metadata; nothing is ever upscaled;
// aspect survives within rounding slack; orientation 1 output equals the
// plain resize pipeline byte for byte.
func TestProcessPropertiesHoldAcrossSizesOrientationsAndAspects(t *testing.T) {
	const maxEdge = 1600
	rng := rand.New(rand.NewSource(20261113)) // deterministic corpus

	var cases [][3]int // w, h, orientation
	for o := 1; o <= 8; o++ {
		cases = append(cases,
			[3]int{601, 400, o}, [3]int{400, 601, o},
			[3]int{1, 1, o}, [3]int{1, 9, o}, [3]int{9, 1, o},
		)
	}
	// Resize-path cases around and above the cap, in all rotation shapes.
	cases = append(cases,
		[3]int{2000, 1499, 1}, [3]int{1999, 1500, 6},
		[3]int{1500, 1999, 8}, [3]int{1703, 1277, 3},
	)
	for i := 0; i < 4; i++ {
		cases = append(cases, [3]int{1 + rng.Intn(2000), 1 + rng.Intn(2000), 1 + rng.Intn(8)})
	}
	for i := 0; i < 4; i++ {
		cases = append(cases, [3]int{1 + rng.Intn(60), 1 + rng.Intn(60), 1 + rng.Intn(8)})
	}

	for _, tc := range cases {
		w, h, o := tc[0], tc[1], tc[2]
		t.Run(fmt.Sprintf("%dx%d orientation %d", w, h, o), func(t *testing.T) {
			// EXIF orientation lives in a JPEG APP1, so every case above
			// orientation 1 is fed as a tagged JPEG; orientation 1 stays a
			// plain PNG (also covering the PNG decode path).
			input := gradientPNG(t, w, h)
			if o != 1 {
				var buf bytes.Buffer
				if err := jpeg.Encode(&buf, mustDecode(t, input), &jpeg.Options{Quality: 95}); err != nil {
					t.Fatalf("jpeg encode: %v", err)
				}
				input = insertAPP1(t, buf.Bytes(), exifAPP1Segment(t, uint16(o), true, 3))
			}
			res, err := Process(input, Options{MaxBytes: 64 << 20, MaxPixels: 64_000_000, MaxEdge: maxEdge})
			if err != nil {
				t.Fatalf("Process: %v", err)
			}
			assertPropertyDims(t, res, w, h, o, maxEdge)
			if o == 1 {
				want := referenceOrientation1(t, input, maxEdge)
				if !bytes.Equal(res.JPEG, want) {
					t.Errorf("orientation 1 output differs from the plain decode-fit-encode pipeline (%d vs %d bytes)", len(res.JPEG), len(want))
				}
			}
			if hasJPEGAPPn(t, res.JPEG, 0xE1, 0xE2) {
				t.Errorf("output carries APP1/APP2 metadata")
			}
		})
	}
}

// assertPropertyDims pins the dimension contract of one property case: dims
// ≥ 1, long edge ≤ MaxEdge, never upscaled, aspect preserved within rounding
// slack, and (when the oriented input already fits) the oriented dims
// unchanged.
func assertPropertyDims(t *testing.T, res Result, w, h, o, maxEdge int) {
	t.Helper()
	// Expected oriented dims (before any resize).
	ow, oh := w, h
	if o >= 5 {
		ow, oh = h, w
	}
	if res.Width < 1 || res.Height < 1 {
		t.Fatalf("output dims %dx%d, both must be at least 1", res.Width, res.Height)
	}
	if long := max(res.Width, res.Height); long > maxEdge {
		t.Errorf("output long edge %d exceeds the %d px cap", long, maxEdge)
	}
	if long := max(res.Width, res.Height); long > max(ow, oh) {
		t.Errorf("output %dx%d upscales the %dx%d input", res.Width, res.Height, ow, oh)
	}
	// Aspect preserved within half-up rounding slack (±0.5 px per axis).
	if skew := abs(res.Width*oh - res.Height*ow); skew > ow+oh {
		t.Errorf("output %dx%d does not preserve the %dx%d aspect (skew %d > slack %d)",
			res.Width, res.Height, ow, oh, skew, ow+oh)
	}
	if max(ow, oh) <= maxEdge && (res.Width != ow || res.Height != oh) {
		t.Errorf("input already within the cap: output %dx%d, want the oriented dims %dx%d unchanged",
			res.Width, res.Height, ow, oh)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
