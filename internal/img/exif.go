package img

import "encoding/binary"

// ReadOrientationJPEG returns the EXIF orientation 1..8 of a JPEG, or 1 when
// absent, truncated or corrupt. Hand-written APP1/TIFF parser (decision: no
// EXIF library dependency).
//
// The parser walks the segment chain after SOI up to the first scan data:
// every metadata segment must precede the entropy-coded scan, so a marker
// stream that runs off the rails at any point simply reads as "no usable
// EXIF" (orientation 1). Both TIFF byte orders (II/MM) are parsed; SHORT and
// LONG in-place values are accepted for tag 0x0112; values outside 1..8 are
// corrupt and read as 1. The function never panics on arbitrary input.
func ReadOrientationJPEG(b []byte) int {
	if len(b) < 2 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1 // no SOI: not a JPEG at all
	}
	i := 2
	for i+4 <= len(b) {
		next, stop, app1 := scanSegment(b, i)
		if stop {
			return 1 // metadata must precede the scan / malformed segment
		}
		if next < 0 {
			return 1 // lost marker synchronisation
		}
		if app1 {
			if o := exifAPP1Orientation(b[i:]); o != 0 {
				return o
			}
			// A broken Exif payload does not end the search: a later APP1
			// may still carry a readable IFD0.
		}
		i = next
	}
	return 1
}

// scanSegment classifies the JPEG marker at offset i. It returns the offset
// of the next marker (-1 when the byte stream has lost synchronisation),
// stop=true once no further metadata can follow (SOS/EOI, malformed or
// truncated segment), and app1=true when the segment at i is an APP1.
func scanSegment(b []byte, i int) (next int, stop, app1 bool) {
	if b[i] != 0xFF {
		return -1, false, false
	}
	m := b[i+1]
	if m == 0xFF { // fill byte before the real marker
		return i + 1, false, false
	}
	if m == 0x01 || (m >= 0xD0 && m <= 0xD8) { // TEM, RSTn, SOI: standalone
		return i + 2, false, false
	}
	if m == 0xD9 || m == 0xDA { // EOI / SOS
		return -1, true, false
	}
	segLen := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
	if segLen < 2 {
		return -1, true, false // malformed segment length
	}
	nextOff := i + 2 + segLen
	if nextOff > len(b) {
		return -1, true, false // truncated segment
	}
	return nextOff, false, m == 0xE1
}

// exifAPP1Orientation inspects the APP1 segment starting at b (its 0xFF 0xE1
// marker): when it carries an "Exif\0\0" payload, the IFD0 Orientation value
// is returned (0 when absent, malformed or out of range).
func exifAPP1Orientation(b []byte) int {
	segLen := int(binary.BigEndian.Uint16(b[2:4]))
	if segLen < 8 || string(b[4:10]) != "Exif\x00\x00" {
		return 0
	}
	return tiffOrientation(b[10 : 2+segLen])
}

// tiffOrientation parses the Orientation tag (0x0112) out of IFD0 of a TIFF
// block (the payload of an Exif APP1, after its 6-byte header). It returns
// the orientation 1..8, or 0 when the block is malformed, the tag is absent,
// or the tag carries an unusable type or value.
func tiffOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 0
	}
	var u16 func([]byte) int
	var u32 func([]byte) int64
	switch string(tiff[:2]) {
	case "II": // little-endian
		u16 = func(p []byte) int { return int(binary.LittleEndian.Uint16(p)) }
		u32 = func(p []byte) int64 { return int64(binary.LittleEndian.Uint32(p)) }
	case "MM": // big-endian
		u16 = func(p []byte) int { return int(binary.BigEndian.Uint16(p)) }
		u32 = func(p []byte) int64 { return int64(binary.BigEndian.Uint32(p)) }
	default:
		return 0
	}

	ifd0 := u32(tiff[4:8]) // IFD0 offset from the start of the TIFF block
	if ifd0 < 8 || ifd0+2 > int64(len(tiff)) {
		return 0
	}
	entries := u16(tiff[ifd0 : ifd0+2])
	for e := 0; e < entries; e++ {
		base := ifd0 + 2 + int64(e)*12
		if base+12 > int64(len(tiff)) {
			return 0 // truncated IFD
		}
		rec := tiff[base : base+12]
		if u16(rec[0:2]) != 0x0112 { // not the Orientation tag
			continue
		}
		switch u16(rec[2:4]) {
		case 3: // SHORT
			if u32(rec[4:8]) != 1 {
				return 0
			}
			return validOrientation(u16(rec[8:10])) // in-place SHORT value
		case 4: // LONG
			if u32(rec[4:8]) != 1 {
				return 0
			}
			return validOrientation(int(u32(rec[8:12]))) // in-place LONG value
		}
		return 0 // orientation tag with an unexpected type: corrupt
	}
	return 0 // no Orientation tag in IFD0
}

// validOrientation narrows v to the EXIF range, 0 when out of range.
func validOrientation(v int) int {
	if v >= 1 && v <= 8 {
		return v
	}
	return 0
}
