// Package img turns arbitrary uploaded image bytes into a clean, display
// JPEG: size and pixel guards first, sniffed decode, EXIF orientation
// applied, long-edge capped resize, metadata-free re-encode. The package is
// pure bytes-in/bytes-out — no HTTP, no filesystem, no database; the upload
// handler maps the returned sentinel errors onto HTTP status codes.
package img

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // registers PNG decoding with image.Decode/DecodeConfig

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers WebP decoding with image.Decode/DecodeConfig
)

var (
	ErrTooLarge    = errors.New("img: above the size limit")
	ErrPixelBomb   = errors.New("img: above the pixel limit")
	ErrUnsupported = errors.New("img: unsupported image format")
)

// testHookFullDecode, when non-nil, runs after every successful full decode.
// Production code leaves it nil; tests use it to prove the pixel-bomb guard
// fires from DecodeConfig alone, before any raster is touched.
var testHookFullDecode func()

type Options struct {
	MaxBytes  int64 // bytes, before decode
	MaxPixels int   // width*height cap from DecodeConfig, before full decode
	MaxEdge   int   // long-edge cap of the output; <= 0 leaves dimensions untouched
}

type Result struct {
	JPEG   []byte // re-encoded, orientation applied, metadata-free
	Width  int
	Height int
}

// Process validates, decodes, applies EXIF orientation, scales so the long
// edge is <= MaxEdge (never upscales), and re-encodes as JPEG (quality 85).
// Every input pixel must survive the transform except the resize itself.
//
// Guards run in this exact order: byte size first (ErrTooLarge), then the
// DecodeConfig pixel cap (ErrPixelBomb — a bomb is never fully decoded; the
// decision is made from the image header alone), then content sniffing and
// full decode. HEIC-family content, undecodable bytes and every format
// outside JPEG/PNG/WebP yield ErrUnsupported. Only magic bytes and content
// are trusted — never a filename, which this package never sees.
func Process(b []byte, o Options) (Result, error) {
	if int64(len(b)) > o.MaxBytes {
		return Result{}, ErrTooLarge
	}
	if isHEIC(b) {
		return Result{}, ErrUnsupported
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return Result{}, ErrUnsupported
	}
	// int64 arithmetic: DecodeConfig dims can be as large as 2^31 each, and
	// the product of two of those still fits a signed 64-bit int. A header
	// with negative dims cannot decode at all and is rejected downstream.
	if int64(cfg.Width)*int64(cfg.Height) > int64(o.MaxPixels) {
		return Result{}, ErrPixelBomb
	}
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return Result{}, ErrUnsupported
	}
	if testHookFullDecode != nil {
		testHookFullDecode()
	}

	out := applyOrientation(src, ReadOrientationJPEG(b))
	w, h := out.Bounds().Dx(), out.Bounds().Dy()

	if nw, nh, scaled := fitLongEdge(w, h, o.MaxEdge); scaled {
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), out, out.Bounds(), xdraw.Src, nil)
		out, w, h = dst, nw, nh
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 85}); err != nil {
		return Result{}, fmt.Errorf("img: re-encode: %w", err)
	}
	return Result{JPEG: buf.Bytes(), Width: w, Height: h}, nil
}

// applyOrientation permutes pixels so the image displays upright per the
// given EXIF orientation. It is a pure index permutation — no pixel is
// dropped or interpolated — so values outside 2..8 (orientation 1 and any
// corrupt reading) are returned as the untouched source.
func applyOrientation(src image.Image, o int) image.Image {
	if o < 2 || o > 8 {
		return src
	}
	sb := src.Bounds()
	w, h := sb.Dx(), sb.Dy()
	dw, dh := w, h
	if o >= 5 { // the quarter-turn orientations swap width and height
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			sx, sy := sourceXY(x, y, w, h, o)
			dst.Set(x, y, src.At(sb.Min.X+sx, sb.Min.Y+sy))
		}
	}
	return dst
}

// sourceXY maps an output pixel of the oriented image back to its source
// pixel, for each EXIF orientation. Example, orientation 6 (rotate 90°
// clockwise) on [A B / C D]: the source's top-left A lands at the output's
// top-right, giving [C A / D B].
func sourceXY(x, y, w, h, o int) (int, int) {
	switch o {
	case 2: // flip horizontal
		return w - 1 - x, y
	case 3: // rotate 180
		return w - 1 - x, h - 1 - y
	case 4: // flip vertical
		return x, h - 1 - y
	case 5: // transpose (flip about the main diagonal)
		return y, x
	case 6: // rotate 90° clockwise
		return y, h - 1 - x
	case 7: // transverse (flip about the anti-diagonal)
		return w - 1 - y, h - 1 - x
	case 8: // rotate 90° counter-clockwise
		return w - 1 - y, x
	}
	return x, y
}

// fitLongEdge reports the scaled dimensions when the long edge exceeds cap:
// aspect preserved, half-up rounding, never below 1 pixel, long edge exactly
// cap. scaled is false when the image already fits (or cap <= 0, which
// disables the cap entirely — a caller omitting MaxEdge never resizes).
func fitLongEdge(w, h, cap int) (nw, nh int, scaled bool) {
	if cap <= 0 {
		return w, h, false
	}
	long := max(w, h)
	if long <= cap {
		return w, h, false
	}
	nw = scaleDim(w, cap, long)
	nh = scaleDim(h, cap, long)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	return nw, nh, true
}

// scaleDim rounds d*cap/long half-up, in integer arithmetic so no float
// error can push the long edge past cap.
func scaleDim(d, cap, long int) int {
	return int((int64(d)*int64(cap) + int64(long)/2) / int64(long))
}

// heicBrands are the ISO-BMFF brands that identify HEIC/HEIF still images
// (major brand or any compatible brand of the ftyp box).
var heicBrands = map[string]bool{
	"heic": true, "heix": true, "hevc": true, "hevx": true,
	"heim": true, "heis": true, "hevm": true, "hevs": true,
	"mif1": true, "msf1": true,
}

// isHEIC reports whether b sniffs as ISO-BMFF content carrying a
// heic-family brand: the ftyp box type sits at bytes 4..8, the major brand
// at 8..12, followed by the 4-byte minor version and the compatible-brand
// list. Only the first ftyp box is inspected.
func isHEIC(b []byte) bool {
	if len(b) < 12 || string(b[4:8]) != "ftyp" {
		return false
	}
	if heicBrands[string(b[8:12])] {
		return true
	}
	end := len(b)
	if bl := int(binary.BigEndian.Uint32(b[0:4])); bl >= 16 && bl < end {
		end = bl
	}
	for off := 16; off+4 <= end; off += 4 {
		if heicBrands[string(b[off:off+4])] {
			return true
		}
	}
	return false
}
