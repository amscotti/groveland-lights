# T-202 review — internal/img: decode, orient, resize, strip (A-206/A-207, package level)

Review basis: the attached diff (`harness/reviews/T-202.diff`, identical to the diff
supplied with this request) against `harness/tasks/T-202.md` (the packet, incl. its
frozen API block and frozen guard-order rules), `harness/tasks/T-206.md` (the
downstream consumer of the API), `acceptance/drafts/admin_lights_go_test.go.txt`
(A-206/A-207 oracle, esp. `TestA206PhotoPipeline` lines 259–358 and
`TestA207NoStrayFiles` lines 360–415), `testdata/photos/gen/main.go` (fixture
ground truth), `docs/spec/architecture.md`, `harness/tasks/graph.json:316–342`,
`cmd/harness/guard.go`/`scan.go` (what the guard actually checks), `Makefile`,
`harness/policy.json`, `.gitignore`, and main's `go.mod`/`go.sum`.

Verification limits: `internal/img/` does not exist in this checkout (the task ran
in its worktree) and shell execution is denied in this sandbox, so I could not
re-run `go test`, lint, or the fuzzer — the handoff's exit codes are taken on trust
where marked. Everything below is a static check of the diffed bytes, cross-checked
against the fixture generator and the oracle draft, both of which I read in full. I
independently re-derived the EXIF orientation math, the resize arithmetic, and the
parser bounds rather than trusting the diff's own comments.

## Contract conformance (packet's frozen API and rules)

- **Signatures verbatim**: `Process(b []byte, o Options) (Result, error)`,
  `ReadOrientationJPEG(b []byte) int`, `Options{MaxBytes int64; MaxPixels int;
  MaxEdge int}`, `Result{JPEG []byte; Width, Height int}` and the three sentinel
  error strings match `harness/tasks/T-202.md:29–61` character for character
  (internal/img/img.go:21–42). T-206's frozen mapping table (T-206.md:60–72)
  compiles against this.
- **Guard order frozen and test-proven**: byte size first (img.go:55–57, fires even
  for undecodable bytes and counts trailing junk after EOI — exactly what
  `alOversizeJPEG` constructs in the draft, lines 709–721), then HEIC sniff
  (img.go:58–60), then `DecodeConfig` pixel cap in int64 (img.go:68–70), only then
  full decode (img.go:71). A bomb is never decoded — proven by the packet-sanctioned
  package-internal hook (`testHookFullDecode`, img.go:27–30, 75–77; nil in
  production), with a mechanism-sanity precheck that the crafted IHDR really
  reports >40 MP (img_test.go, `TestProcessPixelBombIsRefusedWithoutAFullDecode`).
  The HEIC sniff sitting between size and DecodeConfig does not change any
  observable outcome (HEIC bytes fail DecodeConfig with the same `ErrUnsupported`),
  and an oversize HEIC correctly stays `ErrTooLarge`/413, matching the draft's
  status table (lines 30–32).
- **Orientation math verified against the EXIF spec**: I re-derived all 8 matrices
  for `sourceXY` (img.go:123–141) and `applyOrientation`'s dim swap for o≥5
  (img.go:99–117). All correct: 6 = 90° CW → [A B/C D]→[C A/D B], 8 = 90° CCW →
  [B D/A C], 2/4 flips, 3 = 180°, 5/7 transpositions. The pipeline yields exactly
  800×600 for both 600×800 portrait fixtures — the frozen dims in the oracle
  (draft:306–314) and anticipated by the generator's own comment
  (gen/main.go:33–35). The test-side `orientationQuadrants` table
  (img_test.go) is an independent encoding of the same spec, and the corner tests
  hand-derive correspondences from the fixture gradient — I verified each corner
  coordinate pair (e.g. orient6 output top-left ← stored (0,792..799)) is the
  mapping the code computes.
- **Resize**: CatmullRom via `golang.org/x/image/draw` (img.go:82–86), aspect
  preserved, never upscales, long edge exactly the cap — `scaleDim`'s half-up
  integer arithmetic (img.go:168–170) provably yields exactly `cap` for the long
  edge and can never exceed the short edge's own length (I checked the algebra,
  incl. the extreme-aspect cases). `MaxEdge <= 0` disables the cap — additive
  semantic, documented; T-206 always passes 1600.
- **Metadata stripping**: stdlib `jpeg.Encode` (img.go:89) writes no
  APP1/APP2/EXIF/XMP; every success case in the suite byte-scans the output for
  0xE1/0xE2 (`hasJPEGAPPn`), including all 8 orientations and the whole property
  corpus. Draft A-206's `alHasExifGps` therefore passes trivially (no APP1 at all).
- **HEIC**: `isHEIC` (img.go:184–201) implements the packet's "bytes 4..8 == ftyp
  with a heic-family brand" sniff; the 16-byte `heic.bin` fixture (verified in
  gen/main.go:227–234) hits it → `ErrUnsupported`. Trust is content-only; the
  package never sees a filename.
- **Error paths return zero-value Results** (nil JPEG) at every refusal
  (img.go:56, 59, 63, 69, 73, 90) — the property A-207's "no stray files" leans on.

## Security

No HTTP, filesystem, DB, templates, or secrets in the package (pure bytes in/out,
as the packet demands). Upload-limit review: the two DoS guards (bytes, pixels)
fire before any raster allocation and in the frozen order; TIFF/IFD offsets are
bounds-checked in int64 throughout (exif.go:90–130 — I re-checked every slice
expression against its guard); the fuzzer's invariant is 1..8 with no panic.
Worst-case legitimate memory (~2×160 MB for a 40 MP upload: full decode plus the
oriented RGBA copy at img.go:79 before downscale) is inherent to the packet's
frozen 40 MP cap and orient-before-resize order — noted as info, not a defect.
CSRF/XSS/SQL/path traversal: not applicable to this package's surface.

## Architecture (docs/spec/architecture.md)

Ownership map line 22 (`internal/img` — decode/EXIF-orient/resize/re-encode; strips
all metadata) is exactly what the package is and nothing more. No HTMX/`#map`
surface (ui: false). Parse-don't-validate: bytes are parsed once at the boundary
into a typed `Result`; typed IDs N/A. Diff touches only owned paths
(`internal/img/**`, `go.mod`, `go.sum`) plus the sanctioned handoff file — matches
`owns` in graph.json:324–328. No `TODO`/`FIXME`/`panic("not implemented")`/`nolint`
markers and no "Phase N" strings anywhere in the diff (grep-verified, incl. the
handoff); no test skips/`.only`.

## Tests — do they exercise the behaviour?

Yes, against real ground truth. `TestProcessFixtures` runs the actual committed
fixtures and pins the exact frozen dims (orient6 → 800×600, orient8 → 800×600,
landscape 320×200 unchanged, tiny.png → 1×1 JPEG, heic/text-as → `ErrUnsupported`,
bomb → `ErrPixelBomb`), re-decoding every output and byte-scanning for metadata; I
verified the fixture assumptions (APP1 directly after SOI, segLen layout incl.
itself, GPS entry, 50000×50000 IHDR, 16-byte ftyp/heic) against the generator.
`TestProcessOrient6/8FixtureCorners...` are genuine KNOWN-pixel assertions
(packet T-202.md:83–86) with tight tolerances vs ≥96-unit corner deltas and
failure modes that swap whole channels. `TestProcessAppliesEveryOrientationMatrix`
pushes a four-quadrant image through the full pipeline for all 8 orientations and
checks the exact spec arrangement at quadrant centers. The property test (52
deterministic cases, seeded) covers long-edge ≤ cap, no upscale, dims ≥ 1, aspect
slack, no APP1/APP2, and byte-identity of orientation-1 output vs a plain
decode→fit→encode reference — the packet's "orientation 1 output equals plain
resize". Ordering is proven (`TestProcessRejectsOversizeBeforeAnyDecode`: junk
after EOI counts; undecodable oversize bytes still yield `ErrTooLarge`).
`ReadOrientationJPEG` gets a 32-case SHORT/LONG × II/MM table, a 22-case
absent/corpus/corrupt table (XMP-first APP1, GPS pointer entries, metadata after
SOS, zero/overrun segment lengths, unknown BOM, bad types/counts/values), the
committed fixtures, a dense truncation sweep over every prefix inside the fixture's
own APP1, and a seeded fuzz target with the 1..8 invariant. WebP is the one
under-exercised path (registration only) — the packet's test list does not require
a WebP case and the handoff escalates it (open issue 2).

## Findings

1. **(Low — planner/integrator decision needed) x/image pinned to v0.47.0, while
   `docs/spec/architecture.md:55` names v0.48.0 in the dependency policy.** The
   packet pins no version (allow_deps is a module path, T-202.md:14) and forbids
   anything but the x/image addition in go.mod/go.sum (T-202.md:101–102); per the
   handoff, v0.48.0's requirements would bump `golang.org/x/sys` — already an
   indirect dep at v0.49.0 on main (go.mod:17, go.sum verified) — which would put a
   second module's version change in the diff. The guard matches deps by path only
   (`cmd/harness/scan.go:141–153`, `guard.go:196–216`), so v0.47.0 passes
   mechanically, and no new transitive module enters the graph (exactly the
   trigger the packet reserves BLOCKED for, T-202.md:68–70 — not hit; state.md:59
   anticipated this risk). The worker surfaced the conflict prominently (handoff
   open issue 1). Resolution is planner-owned: accept v0.47.0 + update the
   architecture doc, or deliberately take the x/sys bump. Note `make check` runs
   govulncheck (Makefile:92,136) at the gate; a one-release-old x/image is not
   expected to trip it, and the gate will say so if it does.
2. **(Info) Handoff note 4 mis-describes `isHEIC`**: it says `mif1`/`msf1` are
   accepted "as major brands only", but img.go:174–178 puts them in the same map,
   so they are accepted anywhere in the brand list (major or compatible).
   Behaviourally irrelevant — every match lands in `ErrUnsupported`, and any
   ISO-BMFF input is undecodable by `image.Decode` regardless — but the handoff
   text should not be relied on by T-206 (which the packet already has doing its
   own HEIC sniff for the frozen wording, T-206.md:68–71).
3. **(Info) `fitLongEdge`'s `< 1` clamp (img.go:157–162) is unreachable in the
   committed corpus** — triggering it needs a long edge > ~2×cap with a 1-px short
   edge (corpus max is 2000 vs cap 1600). The code is correct (1×5000 → 1×1600);
   pure coverage nit, listed here only for completeness.
4. **(Info) WebP decode path has no fixture-backed test** (decode-only in
   x/image; no committed .webp). Covered via the registered-decoder path only;
   packet's test list does not require it; handoff open issue 2 gives T-206 the
   right remedy.
5. **(Info) Peak memory ~2×160 MB for a maximal legitimate (≤40 MP) upload** —
   `applyOrientation` copies at full size before the downscale (img.go:79–86).
   Inherent to the packet's frozen 40 MP cap and orient-before-encode order
   (T-202.md:74–77); single-admin upload path. Not a defect.
6. **(Verification note)** Handoff exit codes (`make check-fast`, `-race
   -shuffle`, 25 s fuzz at 5 M execs, 85.8 % cover, lint clean) are taken on
   trust — not re-runnable here. The handoff's `make e2e-task TASK=T-202` matches
   the Makefile's variable (Makefile:113); AGENTS.md's `T=` spelling is stale, not
   a worker error. The no-op e2e result matches the repo: no active `@A-206`/
   `@A-207` Playwright specs exist (grep-verified; the oracle is an inert
   `.go.txt` draft).

Nothing found that breaches the packet's frozen API, guard order, ownership,
marker or dependency rules; the orientation/resize/strip behaviour the A-206/A-207
oracle freezes is implemented correctly and pinned by tests that exercise it
against real fixtures.

VERDICT: PASS
