package img

import (
	"encoding/binary"
	"fmt"
	"testing"
)

// exifAPP1Segment builds a complete APP1 segment (marker, length, "Exif\0\0"
// header, TIFF block) whose IFD0 carries a single Orientation entry of the
// requested value type, in either TIFF byte order.
func exifAPP1Segment(tb testing.TB, orientation uint16, le bool, typ uint16) []byte {
	tb.Helper()
	tiff := make([]byte, 26) // 8-byte header, 2-byte entry count, one 12-byte entry, 4-byte next-IFD
	put16 := func(p []byte, v uint16) {
		if le {
			binary.LittleEndian.PutUint16(p, v)
		} else {
			binary.BigEndian.PutUint16(p, v)
		}
	}
	put32 := func(p []byte, v uint32) {
		if le {
			binary.LittleEndian.PutUint32(p, v)
		} else {
			binary.BigEndian.PutUint32(p, v)
		}
	}
	if le {
		copy(tiff[0:2], "II")
	} else {
		copy(tiff[0:2], "MM")
	}
	put16(tiff[2:4], 42) // TIFF magic
	put32(tiff[4:8], 8)  // IFD0 offset
	put16(tiff[8:10], 1) // one entry
	e := tiff[10:22]
	put16(e[0:2], 0x0112) // Orientation tag
	put16(e[2:4], typ)
	put32(e[4:8], 1) // count
	if typ == 3 {    // SHORT: in-place value in the first two bytes
		put16(e[8:10], orientation)
	} else { // LONG: in-place value in all four bytes
		put32(e[8:12], uint32(orientation))
	}
	put32(tiff[22:26], 0) // no next IFD

	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := make([]byte, 4, 4+len(payload))
	seg[0], seg[1] = 0xFF, 0xE1
	// The JPEG segment length is ALWAYS big-endian, independent of the
	// TIFF byte order inside the payload.
	binary.BigEndian.PutUint16(seg[2:4], uint16(len(payload)+2))
	return append(seg, payload...)
}

// exifAPP1SegmentEntries builds an APP1 whose IFD0 (little-endian) holds the
// given entries in order; each entry is tag, type, count and the 4-byte
// in-place value field encoded little-endian.
func exifAPP1SegmentEntries(tb testing.TB, entries [][4]uint32) []byte {
	tb.Helper()
	tiff := make([]byte, 8+2+12*len(entries)+4)
	copy(tiff[0:2], "II")
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8) // IFD0 offset
	binary.LittleEndian.PutUint16(tiff[8:10], uint16(len(entries)))
	for i, ent := range entries {
		e := tiff[10+12*i : 22+12*i]
		binary.LittleEndian.PutUint16(e[0:2], uint16(ent[0]))
		binary.LittleEndian.PutUint16(e[2:4], uint16(ent[1]))
		binary.LittleEndian.PutUint32(e[4:8], ent[2])
		binary.LittleEndian.PutUint32(e[8:12], ent[3])
	}
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := make([]byte, 4, 4+len(payload))
	seg[0], seg[1] = 0xFF, 0xE1
	binary.BigEndian.PutUint16(seg[2:4], uint16(len(payload)+2))
	return append(seg, payload...)
}

// wrapExifPayload wraps an Exif payload (header + TIFF block) into a full
// APP1 segment.
func wrapExifPayload(payload []byte) []byte {
	seg := make([]byte, 4, 4+len(payload))
	seg[0], seg[1] = 0xFF, 0xE1
	binary.BigEndian.PutUint16(seg[2:4], uint16(len(payload)+2))
	return append(seg, payload...)
}

// xmpAPP1 builds an APP1 segment carrying an XMP header (not Exif) — the
// common "first APP1 is not the EXIF one" real-world layout.
func xmpAPP1() []byte {
	payload := append([]byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta/>"), make([]byte, 8)...)
	return wrapExifPayload(payload)
}

// insertAPP1 returns jpegBytes with one APP1 segment inserted directly after
// the SOI marker (the layout of the committed fixtures).
func insertAPP1(tb testing.TB, jpegBytes, seg []byte) []byte {
	tb.Helper()
	if len(jpegBytes) < 2 || jpegBytes[0] != 0xFF || jpegBytes[1] != 0xD8 {
		tb.Fatalf("insertAPP1: input is not a JPEG (no SOI)")
	}
	out := make([]byte, 0, len(jpegBytes)+len(seg))
	out = append(out, jpegBytes[:2]...)
	out = append(out, seg...)
	out = append(out, jpegBytes[2:]...)
	return out
}

// jpegWithSegments assembles SOI + the given segments + EOI.
func jpegWithSegments(segs ...[]byte) []byte {
	var out []byte
	out = append(out, 0xFF, 0xD8)
	for _, s := range segs {
		out = append(out, s...)
	}
	out = append(out, 0xFF, 0xD9)
	return out
}

// TestReadOrientationJPEGParsesEveryOrientationBothByteOrdersAndValueTypes:
// orientations 1..8 parse back exactly, written either little- or big-endian
// with the value stored as SHORT (the spec form) or LONG (a tolerated
// real-world variant).
func TestReadOrientationJPEGParsesEveryOrientationBothByteOrdersAndValueTypes(t *testing.T) {
	for o := 1; o <= 8; o++ {
		for _, le := range []bool{true, false} {
			endian := "big-endian"
			if le {
				endian = "little-endian"
			}
			for _, typ := range []uint16{3, 4} {
				typName := "SHORT"
				if typ == 4 {
					typName = "LONG"
				}
				t.Run(fmt.Sprintf("orientation %d %s %s", o, endian, typName), func(t *testing.T) {
					j := jpegWithSegments(exifAPP1Segment(t, uint16(o), le, typ))
					if got := ReadOrientationJPEG(j); got != o {
						t.Errorf("ReadOrientationJPEG = %d, want %d", got, o)
					}
				})
			}
		}
	}
}

// TestReadOrientationJPEGReadsUprightWhenExifIsAbsentOrBroken: absent,
// truncated or corrupt EXIF must read as orientation 1, never an error or a
// wild value.
func TestReadOrientationJPEGReadsUprightWhenExifIsAbsentOrBroken(t *testing.T) {
	brokenAPP1 := wrapExifPayload([]byte("Exif\x00\x00XX")) // Exif header, no usable TIFF
	orientationFirst := exifAPP1SegmentEntries(t, [][4]uint32{
		{0x0112, 3, 1, 6},  // Orientation SHORT=6 (in-place low bytes)
		{0x8825, 4, 1, 38}, // GPSInfo pointer, like the fixtures
	})
	orientationSecond := exifAPP1SegmentEntries(t, [][4]uint32{
		{0x0100, 4, 1, 600}, // ImageWidth before the orientation tag
		{0x0112, 3, 1, 8},   // Orientation SHORT=8
	})
	noOrientation := exifAPP1SegmentEntries(t, [][4]uint32{
		{0x010E, 2, 4, 0x64657363}, // ImageDescription ASCII
	})
	unknownByteOrder := func() []byte {
		tiff := make([]byte, 8)
		copy(tiff[0:2], "XX") // neither II nor MM
		binary.LittleEndian.PutUint16(tiff[2:4], 42)
		binary.LittleEndian.PutUint32(tiff[4:8], 8)
		return append([]byte("Exif\x00\x00"), tiff...)
	}()

	tests := []struct {
		name string
		b    []byte
		want int
	}{
		{"nil bytes read upright", nil, 1},
		{"empty bytes read upright", []byte{}, 1},
		{"bytes without soi read upright", []byte{0x00, 0x01, 0xFF, 0xE1}, 1},
		{"bare soi reads upright", []byte{0xFF, 0xD8}, 1},
		{"jpeg without exif reads upright", jpegWithSegments(), 1},
		{"png bytes read upright", fixture(t, "tiny.png"), 1},
		{"heic bytes read upright", fixture(t, "heic.bin"), 1},
		{"non-exif app1 first is skipped", jpegWithSegments(xmpAPP1()), 1},
		{"app1 without orientation tag reads upright", jpegWithSegments(noOrientation), 1},
		{"broken exif app1 without tiff reads upright", jpegWithSegments(brokenAPP1), 1},
		{"orientation value 0 is corrupt", jpegWithSegments(exifAPP1Segment(t, 0, true, 3)), 1},
		{"orientation value 9 is corrupt", jpegWithSegments(exifAPP1Segment(t, 9, true, 3)), 1},
		{"orientation value 65535 is corrupt", jpegWithSegments(exifAPP1Segment(t, 65535, false, 3)), 1},
		{"orientation tag with ascii type is corrupt", jpegWithSegments(exifAPP1Segment(t, 6, true, 2)), 1},
		{"orientation tag with count 2 is corrupt", jpegWithSegments(exifAPP1SegmentEntries(t, [][4]uint32{{0x0112, 3, 2, 0}})), 1},
		{"tiff with unknown byte order is corrupt", jpegWithSegments(wrapExifPayload(unknownByteOrder)), 1},
		{"exif payload shorter than a tiff header reads upright", jpegWithSegments([]byte{0xFF, 0xE1, 0x00, 0x08, 'E', 'x', 'i', 'f', 0x00, 0x00}), 1},
		{"zero segment length reads upright", jpegWithSegments([]byte{0xFF, 0xE1, 0x00, 0x00}), 1},
		{"segment running past the end reads upright", func() []byte {
			j := jpegWithSegments(exifAPP1Segment(t, 6, true, 3))
			return j[:len(j)-8] // cut into the APP1 segment
		}(), 1},
		{"metadata after sos reads upright", jpegWithSegments([]byte{0xFF, 0xDA, 0x00, 0x02}, exifAPP1Segment(t, 6, true, 3)), 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReadOrientationJPEG(tc.b); got != tc.want {
				t.Errorf("ReadOrientationJPEG = %d, want %d", got, tc.want)
			}
		})
	}

	// Entries around the orientation tag do not hide it.
	t.Run("orientation entry preceded by another entry is found", func(t *testing.T) {
		if got := ReadOrientationJPEG(jpegWithSegments(orientationSecond)); got != 8 {
			t.Errorf("ReadOrientationJPEG = %d, want 8", got)
		}
	})
	t.Run("gps pointer entry after the orientation tag is tolerated", func(t *testing.T) {
		if got := ReadOrientationJPEG(jpegWithSegments(orientationFirst)); got != 6 {
			t.Errorf("ReadOrientationJPEG = %d, want 6", got)
		}
	})
	t.Run("a valid exif app1 after a broken one is still found", func(t *testing.T) {
		j := jpegWithSegments(brokenAPP1, exifAPP1Segment(t, 8, true, 3))
		if got := ReadOrientationJPEG(j); got != 8 {
			t.Errorf("ReadOrientationJPEG = %d, want 8", got)
		}
	})
	t.Run("fill bytes before the marker are tolerated", func(t *testing.T) {
		j := jpegWithSegments([]byte{0xFF, 0xFF, 0xFF}, exifAPP1Segment(t, 6, true, 3))
		if got := ReadOrientationJPEG(j); got != 6 {
			t.Errorf("ReadOrientationJPEG = %d, want 6", got)
		}
	})
	t.Run("orientation survives a comment segment before app1", func(t *testing.T) {
		j := jpegWithSegments([]byte{0xFF, 0xFE, 0x00, 0x04, 'h', 'i'}, exifAPP1Segment(t, 6, true, 3))
		if got := ReadOrientationJPEG(j); got != 6 {
			t.Errorf("ReadOrientationJPEG = %d, want 6", got)
		}
	})
}

// TestReadOrientationJPEGReadsTheCommittedFixtures: the real fixtures
// produced by testdata/photos/gen parse back to their tagged values.
func TestReadOrientationJPEGReadsTheCommittedFixtures(t *testing.T) {
	tests := []struct {
		file string
		want int
	}{
		{"portrait-orient6-gps.jpg", 6},
		{"portrait-orient8-gps.jpg", 8},
		{"landscape.jpg", 1},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s reads orientation %d", tc.file, tc.want), func(t *testing.T) {
			if got := ReadOrientationJPEG(fixture(t, tc.file)); got != tc.want {
				t.Errorf("ReadOrientationJPEG(%s) = %d, want %d", tc.file, got, tc.want)
			}
		})
	}
}

// TestReadOrientationJPEGReadsUprightOnTruncatedFixtures: every prefix that
// cuts inside the APP1 segment reads as orientation 1 — never panics, never
// a stale value. Once the segment is complete the value reads back even
// though the rest of the file is missing.
func TestReadOrientationJPEGReadsUprightOnTruncatedFixtures(t *testing.T) {
	portrait := fixture(t, "portrait-orient6-gps.jpg")
	if portrait[0] != 0xFF || portrait[1] != 0xD8 || portrait[2] != 0xFF || portrait[3] != 0xE1 {
		t.Fatalf("fixture does not start with SOI + APP1")
	}
	app1End := 4 + int(binary.BigEndian.Uint16(portrait[4:6])) // one past the segment

	for n := 0; n < app1End; n++ {
		if got := ReadOrientationJPEG(portrait[:n]); got != 1 {
			t.Fatalf("prefix of %d bytes (APP1 incomplete) returned orientation %d, want 1", n, got)
		}
	}
	for n := app1End; n < len(portrait); n += 97 {
		if got := ReadOrientationJPEG(portrait[:n]); got != 6 {
			t.Fatalf("prefix of %d bytes (EXIF complete) returned orientation %d, want 6", n, got)
		}
	}
}

// FuzzReadOrientationJPEG: the parser must never panic and must always
// answer within 1..8, for arbitrary bytes. Seed corpus: the committed
// fixtures, their truncations, hand-built EXIF variants and garbage.
func FuzzReadOrientationJPEG(f *testing.F) {
	f.Add([]byte(nil))
	f.Add([]byte{0xFF, 0xD8})
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x02})
	for _, name := range []string{
		"portrait-orient6-gps.jpg", "portrait-orient8-gps.jpg", "landscape.jpg",
		"tiny.png", "pixelbomb.png", "text-as.jpg", "heic.bin",
	} {
		b := fixture(f, name)
		f.Add(b)
	}
	portrait := fixture(f, "portrait-orient6-gps.jpg")
	for _, n := range []int{3, 11, 40, 97, 512} {
		f.Add(portrait[:n])
	}
	for o := uint16(1); o <= 8; o++ {
		f.Add(jpegWithSegments(exifAPP1Segment(f, o, true, 3)))
	}
	f.Add(jpegWithSegments(exifAPP1Segment(f, 6, false, 4)))
	f.Add(jpegWithSegments(xmpAPP1()))
	f.Add([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'})
	f.Add([]byte{0x00, 0x00, 0x00, 0x10, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c', 0, 0, 0, 0})

	f.Fuzz(func(t *testing.T, b []byte) {
		if o := ReadOrientationJPEG(b); o < 1 || o > 8 {
			preview := b
			if len(preview) > 24 {
				preview = preview[:24]
			}
			t.Fatalf("ReadOrientationJPEG(%x...) = %d, want 1..8", preview, o)
		}
	})
}
