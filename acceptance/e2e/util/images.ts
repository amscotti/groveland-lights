/**
 * JPEG byte-inspection helpers for the M2 photo contract (A-206/A-207).
 *
 * ACTIVE helper (no tests of its own): the frozen drafts import these to
 * prove, black-box, that a served photo was re-encoded — no EXIF GPS block
 * survives and the long edge is within the 1600 px cap. Byte-level parsing
 * only; no image decoding needed.
 */

/** Dimensions parsed from a JPEG SOF marker. */
export interface JpegSize {
  w: number;
  h: number;
}

/**
 * hasExifGps reports whether the JPEG bytes carry an EXIF APP1 segment that
 * points at a GPS IFD (tag 0x8825 resolving to a non-empty IFD). A correct
 * pipeline strips the whole APP1 segment, so this must be false for any
 * served photo.
 */
export function hasExifGps(bytes: Uint8Array): boolean {
  if (bytes.length < 4 || bytes[0] !== 0xff || bytes[1] !== 0xd8) {
    return false; // not a JPEG
  }
  let i = 2;
  while (i + 4 <= bytes.length) {
    if (bytes[i] !== 0xff) {
      return false; // desynchronised; no metadata worth trusting
    }
    const marker = bytes[i + 1];
    if (marker === 0x01 || (marker >= 0xd0 && marker <= 0xd7)) {
      i += 2; // standalone marker, no length field
      continue;
    }
    if (marker === 0xd9 || marker === 0xda) {
      return false; // EOI / start of scan: metadata only precedes these
    }
    const len = (bytes[i + 2] << 8) | bytes[i + 3];
    if (len < 2 || i + 2 + len > bytes.length) {
      return false; // malformed segment length
    }
    if (marker === 0xe1) {
      const seg = bytes.subarray(i + 4, i + 2 + len);
      if (startsWith(seg, exifHeader) && tiffHasGpsIfd(seg.subarray(6))) {
        return true;
      }
    }
    i += 2 + len;
  }
  return false;
}

/**
 * jpegSize parses the frame dimensions from the first SOF marker (SOF0, SOF1,
 * SOF2, ... — progressive JPEGs included). Throws when no SOF is found: a
 * caller asserting on a served photo wants a loud failure, not {0, 0}.
 */
export function jpegSize(bytes: Uint8Array): JpegSize {
  if (bytes.length < 4 || bytes[0] !== 0xff || bytes[1] !== 0xd8) {
    throw new Error('jpegSize: not a JPEG (missing SOI)');
  }
  let i = 2;
  while (i + 4 <= bytes.length) {
    if (bytes[i] !== 0xff) {
      throw new Error('jpegSize: lost marker synchronisation');
    }
    const marker = bytes[i + 1];
    if (marker === 0x01 || (marker >= 0xd0 && marker <= 0xd7)) {
      i += 2;
      continue;
    }
    if (marker === 0xd9 || marker === 0xda) {
      throw new Error('jpegSize: reached end of headers without a SOF marker');
    }
    const len = (bytes[i + 2] << 8) | bytes[i + 3];
    if (len < 2 || i + 2 + len > bytes.length) {
      throw new Error('jpegSize: malformed segment length');
    }
    // SOF0..SOF15 except DHT (0xc4), JPG (0xc8) and DAC (0xcc).
    if (marker >= 0xc0 && marker <= 0xcf && marker !== 0xc4 && marker !== 0xc8 && marker !== 0xcc) {
      if (i + 9 > bytes.length) {
        throw new Error('jpegSize: truncated SOF segment');
      }
      const h = (bytes[i + 5] << 8) | bytes[i + 6];
      const w = (bytes[i + 7] << 8) | bytes[i + 8];
      return { w, h };
    }
    i += 2 + len;
  }
  throw new Error('jpegSize: no SOF marker found');
}

const exifHeader = ascii('Exif\x00\x00');

function ascii(s: string): Uint8Array {
  return Uint8Array.from(s, (c) => c.charCodeAt(0));
}

function startsWith(b: Uint8Array, prefix: Uint8Array): boolean {
  if (b.length < prefix.length) {
    return false;
  }
  for (let i = 0; i < prefix.length; i++) {
    if (b[i] !== prefix[i]) {
      return false;
    }
  }
  return true;
}

/**
 * tiffHasGpsIfd parses a TIFF header (either endianness), walks the first IFD
 * and reports whether the GPSInfo pointer tag (0x8825) resolves to an IFD
 * holding at least one entry. Any malformed input reads as "no GPS".
 */
function tiffHasGpsIfd(tiff: Uint8Array): boolean {
  if (tiff.length < 8) {
    return false;
  }
  const little = tiff[0] === 0x49 && tiff[1] === 0x49;
  const big = tiff[0] === 0x4d && tiff[1] === 0x4d;
  if (!little && !big) {
    return false;
  }
  const u16 = (o: number): number =>
    little ? tiff[o] | (tiff[o + 1] << 8) : (tiff[o] << 8) | tiff[o + 1];
  const u32 = (o: number): number =>
    little
      ? (tiff[o] | (tiff[o + 1] << 8) | (tiff[o + 2] << 16) | (tiff[o + 3] << 24)) >>> 0
      : ((tiff[o] << 24) | (tiff[o + 1] << 16) | (tiff[o + 2] << 8) | tiff[o + 3]) >>> 0;
  const ifd0 = u32(4);
  if (ifd0 + 2 > tiff.length) {
    return false;
  }
  const entries = u16(ifd0);
  for (let e = 0; e < entries; e++) {
    const base = ifd0 + 2 + e * 12;
    if (base + 12 > tiff.length) {
      return false;
    }
    if (u16(base) === 0x8825) {
      const gps = u32(base + 8);
      return gps + 2 <= tiff.length && u16(gps) > 0;
    }
  }
  return false;
}
