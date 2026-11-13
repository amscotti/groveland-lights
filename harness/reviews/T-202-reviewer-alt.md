# T-202 — independent review (reviewer-alt): internal/img

Basis: the attached diff (`harness/reviews/T-202.diff`), packet
`harness/tasks/T-202.md`, `AGENTS.md`, `testdata/photos/gen/main.go` and the
committed photo fixtures, `acceptance/contract.json` (A-206/A-207), the inert
Go draft (`acceptance/drafts/admin_lights_go_test.go.txt`), `Makefile`, and
`harness/policy.json`.

Verification limit: the diff is **not applied** in this checkout (there is no
`internal/img/` directory and no `harness/handoffs/T-202.md` on disk), and shell
execution is denied in this session. This is therefore a static review of the
diffed bytes against the contracts I can read; the handoff's executed results
(`check-fast`, fuzz exec count, coverage) are taken on trust. Line numbers below
are post-patch `file:line`.

## Findings

### 1. [Medium — coverage theatre] The HEIC sniff has no test that can fail

`TestProcessFixtures` asserts the `heic.bin` fixture returns `ErrUnsupported`
(`internal/img/img_test.go:132-135`). `heic.bin` is a 16-byte non-image ftyp
header, so `image.DecodeConfig` fails on it and `Process` already returns
`ErrUnsupported` from that error (`internal/img/img.go:61-63`) even if the HEIC
guard is deleted or always returns false. The guard itself is
`isHEIC` (`internal/img/img.go:184-201`); no test calls it, and none feeds a
major-brand or compatible-brand variant, a non-HEIC `ftyp` box, or a short
`ftyp` buffer. Deleting `isHEIC` (or the call at `img.go:58-60`) therefore keeps
the whole suite green. This is the definition of a test that cannot fail for the
behaviour it is cited against. A small `isHEIC` table (heic/heix/mif1 major,
heic as a compatible brand, `ftyp`+`avif` negative, <12 bytes) and an
integration row that reaches the guard rather than `DecodeConfig` would pin it.
The handoff's fixture table (`harness/handoffs/T-202.md:79`) and open issue 4
(`:149`) overstate what the row proves.

### 2. [Medium — deliverable branch with no coverage at any layer] WebP decode is untested

WebP support is the blank import `_ "golang.org/x/image/webp"`
(`internal/img/img.go:18`). No test feeds WebP bytes, and `testdata/photos`
contains no WebP fixture (the generator writes only the seven listed files).
If the import is removed or the decoder registration breaks, every test in this
diff still passes and a WebP upload silently becomes `ErrUnsupported` — a direct
A-206 contract violation ("jpeg/png/webp ... re-encoded jpeg",
`acceptance/contract.json:72`). The A-206 Go draft uploads only
`portrait-orient6-gps.jpg`, so nothing downstream covers it either. The
handoff's open issue 2 (`harness/handoffs/T-202.md:139-144`) says the path is
"exercised only through the shared registered-decoder path", but no test decodes
a WebP; registration failure is precisely the regression the sentence says would
go unnoticed. The packet's test list does not name a WebP fixture, so this is a
missing test rather than a weakened one, but it is the only deliverable branch
in this diff with zero executable coverage. Add a tiny WebP fixture through
`testdata/photos/gen` and one `Process` row (dims, JPEG output, no APP1/APP2),
or at minimum a `webp`/`image.Decode` row. Close this before T-206 relies on
WebP.

### 3. [Low — format assertion is incidental] `requireJPEGOutput` does not actually assert JPEG

`requireJPEGOutput` (`internal/img/img_test.go:167-182`) calls
`image.DecodeConfig`, which succeeds for any registered decoder, then asserts
dims and no APP1/APP2. It never checks the magic bytes or the returned format
name. A non-JPEG result would currently trip `hasJPEGAPPn` only because that
helper assumes an SOI and fatal-errors when `data[2] != 0xFF`
(`internal/img/img_test.go:276-281`) — and the error message would be
"lost marker synchronisation", not "not a JPEG". A JPEG-like container that
keeps the marker chain intact would pass. Quality 85 is also only pinned for
orientation 1 by the byte-equality reference
(`internal/img/img_test.go:432-447,497-500`). Use `jpeg.DecodeConfig`/an SOI
assertion in this helper so the "re-encoded as JPEG, quality 85" contract is
asserted directly.

### 4. [Low — assertion 2x looser than its stated contract] Aspect slack is `ow+oh`, not ±0.5 px per axis

`assertPropertyDims` comments "aspect preserved within half-up rounding slack
(±0.5 px per axis)" (`internal/img/img_test.go:530`), but the condition is
`skew > ow+oh` (`:531`). The correct bound implied by ±0.5 px per axis is about
`(ow+oh)/2`; the current threshold allows roughly twice that. Example: a
2000×1499 image rendered at 1600×1200 has `skew = |1600*1499 - 1200*2000| =
1600`, which passes although the exact half-up result is 1600×1199. Gross
distortion is still caught, so this is not a gate hole, but the assertion is
weaker than the comment and the packet's "aspect preserved" wording.

### 5. [Low — observer hook not itself validated] "zero full decodes" would also hold if the hook were never called

`testHookFullDecode` (`internal/img/img.go:27-30`) is invoked after a
successful `image.Decode` (`img.go:75-77`). The pixel-bomb test sets it and
asserts zero (`internal/img/img_test.go:313-339`), but no test ever processes a
normal image with the hook set to prove the hook fires. Deleting the hook call —
or moving it out of `Process` — leaves `decodes == 0` and the test green. The
load-bearing proof is the `ErrPixelBomb` assertion itself: the header-only bomb
cannot survive a full decode, so returning `ErrPixelBomb` rather than
`ErrUnsupported` does pin the guard order. Still, a successful-decode row
asserting the hook fires once would make the hook evidence real rather than
vacuously zero.

### 6. [Info — handoff/documentation precision]

- `isHEIC` checks `mif1`/`msf1` in the compatible-brand loop as well as
  major brands (`internal/img/img.go:174-178,188-201`), but the handoff says
  "`mif1`/`msf1` are accepted as major brands only"
  (`harness/handoffs/T-202.md:149-153`). Behaviour is still `ErrUnsupported`
  either way; the statement is simply wrong.
- The broken/absent-EXIF table has 20 want-1 rows
  (`internal/img/exif_test.go:178-200`), not the "17 want-1 cases" claimed at
  `harness/handoffs/T-202.md:86`.
- `fitLongEdge`'s `cap <= 0` branch (`internal/img/img.go:147-149`) is untested.
  It is extra behaviour not required by the packet, so this is informational.

## Checked and found sound

- **No existing test was weakened, deleted, skipped or narrowed.** The diff adds
  a new package and its tests; it edits no pre-existing test file. There is no
  `t.Skip`, `test.skip`, `test.only`, `.fixme`, `TODO`/`FIXME`,
  `panic("not implemented")`, "Phase N" string, or unexplained `//nolint` in the
  added bytes.
- **No fixture-value special-casing in production code.** The literals in
  `img.go`/`exif.go` are format constants (`ftyp`, HEIC brands, `Exif\0\0`,
  `II`/`MM`, JPEG markers, tag `0x0112`); no fixture filename or acceptance
  dimension appears in non-test code.
- **No mocks bypassing the behaviour under test.** Tests call the real `Process`
  and `ReadOrientationJPEG`; the pixel-bomb hook is an observer, not a stub. The
  fixture table, corner tests and quadrant test all decode the real output.
- **The guard-order tests are load-bearing.** `TestProcessRejectsOversizeBeforeAnyDecode`
  would fail if the byte cap ran after `DecodeConfig` (64 zero bytes would be
  `ErrUnsupported`, not `ErrTooLarge`); the pixel-bomb row would be
  `ErrUnsupported` if the pixel cap ran after a full decode because the
  header-only PNG cannot decode. The size check also fires before the HEIC
  sniff as the packet requires.
- **The EXIF orientation matrices are spec-correct and independently pinned.**
  I re-derived `sourceXY` for all eight orientations from the EXIF definitions;
  `orientationQuadrants` (`internal/img/img_test.go:371-380`) matches, and the
  orient6/orient8 corner tests use the correct source corners (the 7-pixel
  offset at orient8 bottom-left is absorbed by the smooth gradient and the
  16-unit tolerance). A wrong rotation/flip changes a corner by ~255, so the
  tolerance is meaningful.
- **The fuzz invariant is the packet's** (1..8, never panic); the packet's
  "truncated input → 1" is covered by the dense prefix test
  (`internal/img/exif_test.go:265-282`), not by the fuzz body.
- **Dependency and ownership are within the packet.** Only
  `golang.org/x/image v0.47.0` is added (allowed); `go.mod`/`go.sum` contain no
  other module change. Only `internal/img/**`, `go.mod`, `go.sum` and the
  handoff are touched; no protected path, golden, acceptance or generator file
  is modified.

## Verdict rationale

The package is implemented against the frozen API and the suite genuinely
exercises the hard parts: guard ordering, pixel-bomb refusal without a full
decode, exact fixture dimensions, orientation direction with known pixels, all
eight orientation matrices, metadata stripping, truncation semantics and a fuzz
seed corpus. I found no deleted, skipped, narrowed or special-cased test, no
mock substituting for the behaviour under test, and no production branch on
fixture values.

The two Medium findings are real coverage holes: the HEIC sniff's only test is
vacuously satisfied by `DecodeConfig` failure, and WebP decode has no test at
any layer (the A-206 draft uploads only a JPEG). Neither is a weakened assertion
or a hidden special-case, and the packet's explicit test list does not require a
WebP fixture, so they do not justify a gate block; they should be closed before
T-206 relies on this package, ideally with an `isHEIC` table and a tiny WebP
fixture. The remaining findings are assertion-strength and handoff-precision
nits.

VERDICT: PASS
