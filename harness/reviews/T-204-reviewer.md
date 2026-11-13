# T-204 review — internal/geo: Haversine DistanceM + duplicate classification (A-204/A-205)

Review basis: the attached diff (`harness/reviews/T-204.diff`, identical to the diff
supplied with this request) against `harness/tasks/T-204.md` (the packet, incl. the
frozen API block at lines 29–65 and the required test list at lines 67–79),
`acceptance/contract.json:57–68` (A-204/A-205), `acceptance/drafts/README.md`
(frozen reconciliation: lines 100–101 "Duplicate radii", the A-205 panel contract
lines 64/68, and the per-assertion notes lines 202–210),
`acceptance/drafts/admin_lights_go_test.go.txt` (`TestA204OutsideBBox` 129–149,
`TestA205Duplicates` 151–257 — the future oracle), `docs/spec/architecture.md`,
`harness/tasks/graph.json:373–397` (owns `internal/geo/**`, claims A-204/A-205,
`allow_deps: []`), `internal/config/config.go:22–29` (the pre-existing
`BBox.Contains` the packet says not to duplicate), `Makefile:111–116` (`e2e-task`),
`.golangci.yml`, and `go.mod`.

Verification limits: `internal/geo/` does not exist in this checkout (the task ran in
its worktree; external directories are denied for this role) and shell execution is
denied in this sandbox, so I could not re-run `go test`/`make check-fast` — the
handoff's exit codes are taken on trust where marked. Everything below is a static
check of the diffed bytes. I independently re-derived every hand-computed test value
(see "Math verification") rather than trusting the diff's own comments.

## Contract conformance (packet's frozen API)

- **Signatures and constants verbatim**: `NumberedRadiusM = 25.0`,
  `UnnumberedRadiusM = 8.0` (geo.go:15–16), `meanEarthRadiusM = 6371008.8`
  (geo.go:21), `Point struct{ Lat, Lon float64 }` (geo.go:29),
  `DistanceM(a, b Point) float64` (geo.go:33), `Candidate{LightID int64; AssetNo
  string; Point Point}` (duplicate.go:6–10), `DupDecision{Numbered bool;
  ExistingID int64; DistanceM float64}` (duplicate.go:13–17), and
  `ClassifyDuplicate(newAssetNo string, newPt Point, existing []Candidate)
  (DupDecision, bool)` (duplicate.go:28) match the packet's frozen block character
  for character. The handoff's frozen-API restatement for T-205 also matches, and
  its module path (`github.com/amscotti/groveland-lights/internal/geo`) matches
  go.mod:1.
- **Bbox non-duplication honoured**: the packet says "Do NOT duplicate or
  re-implement bbox logic" — geo contains no bbox code; the package doc
  (geo.go:4–5) points at `config.BBox.Contains`, which I confirmed exists
  (config.go:27–29). Ownership respected: only `internal/geo/**` plus the handoff
  file touched (the handoff is the sanctioned exception).
- **No new dependencies**: `allow_deps: []` respected — geo.go imports only
  `math`; duplicate.go imports nothing; tests import `math`, `math/rand`,
  `testing` (all stdlib). go.mod/go.sum untouched in the diff, consistent with
  main's go.mod.
- **Claims vs scope**: A-204's bbox half pre-exists in config and A-205's panel is
  T-205's; the packet scopes this task to "the metric half". The handoff
  correctly reports `make e2e-task TASK=T-204` as a designed no-op — I verified
  the mechanism: the Makefile variable is `TASK ?=` (Makefile:8, and the usage
  message at Makefile:113 says `TASK=T-###`; AGENTS.md's `T=<id>` shorthand is
  the stale one), the target greps the task's claims with
  `--pass-with-no-tests` (Makefile:114–116), and no spec under
  `acceptance/e2e/` carries an `@A-204`/`@A-205` tag (grep: zero matches; the
  oracle lives in the inert drafts until T-207 renames them).

## Math verification (independent re-derivation)

With R = 6371008.8 m:

- **Formula**: geo.go:34–44 is the standard haversine
  `2R·asin(√(sin²(Δφ/2)+cosφ₁·cosφ₂·sin²(Δλ/2)))`. For lat ∈ [−90,90] every term
  is non-negative, so `s ≥ 0` and `Asin(Sqrt(s))` cannot NaN on valid input; the
  `s > 1` ulp clamp (geo.go:41–43) makes near-antipodal pairs degrade to exactly
  πR instead of NaN. Correct.
- **R·π/180 = 111195.0802 m/deg** and **πR = 20015114.442 m** — both re-derived
  by hand to all printed digits; match geo_test.go's table (111195.080,
  111.195, 20015114.442) and duplicate_test.go:8's `metresPerDegLat`.
- **cos(42.76°) = 0.734204** (re-derived via Taylor: 0.7342044) → pure-longitude
  0.01° at Groveland = 1111.9508 × 0.734204 = 816.399 m ✓ (geo_test.go table).
- **Diagonal 0.005° S + 0.005° E**: NS = 555.975 m; EW with the exact
  √(cosφ₁·cosφ₂) factor = 408.214 m; √(555.975² + 408.214²) = 689.744 m vs the
  table's 689.745 — 1 mm apart, far inside the packet's 0.5 m tolerance.
- **Antipodes**: s rounds to exactly 1 (or within an ulp below) → distance
  πR ± ~0.13 m worst case, inside 0.5 m.
- **Fixture construction** (duplicate_test.go `northOf`): a pure-latitude
  haversine reduces to R·Δφ with < 1e-9 m correction at street scale, and the
  rounded `metresPerDegLat` constant adds ~3e-10 relative error — the 0.05 m
  classify tolerances have ~7 orders of magnitude of headroom.
- **Exact-float symmetry assertion** (geo_test.go, `ab != DistanceM(b, a)`):
  sound — Go's `math.Sin` is exactly odd and `math.Cos` even (sign-bit handling
  in the implementations), squares kill the remaining sign, and IEEE
  multiplication is commutative, so s(a,b) is bit-identical to s(b,a).
- **Triangle-inequality slacks**: 1e-6 m for ordinary triples (haversine absolute
  error at 2e7 m scale is ~2e-9 m) and 2.0 m only when a leg exceeds 1.9e7 m
  (near-antipodal condition ~√ε·R ≈ 0.06–0.13 m observed/derived) — every triple
  is still asserted; nothing is skipped. Honest and correctly documented in the
  test comments.

## Semantics vs the frozen oracle (what T-205 will build on)

- **Numbered, distance-independent** (duplicate.go:32–43): first candidate in
  slice order with an equal non-empty asset_no wins, `DistanceM` reported
  regardless of distance. Matches drafts/README.md:100–101 ("same non-empty
  asset_no anywhere → numbered duplicate; 25 m only bounds the candidate
  window") and the packet's own frozen test ("same asset_no anywhere", 2 km).
  The contract.json:66 shorthand "same asset_no within 25m" is satisfied via the
  documented reconciliation; the handoff flags this openly (open issue 2) —
  correct call, the drafts are the oracle.
- **No cross-kind**: a numbered submission never falls through to the unnumbered
  rule (duplicate.go:42 early return), and the unnumbered loop skips non-empty
  candidate asset numbers (duplicate.go:50–52). The empty-new/empty-candidate
  case is structurally prevented from matching in the numbered branch by the
  `newAssetNo != ""` guard (duplicate.go:32) — the comment at duplicate.go:29–31
  explains why. Matches the packet's "NOT unnumbered duplicate" test names.
- **Unnumbered radius inclusive at 8.0**: skip only when `d > UnnumberedRadiusM`
  (duplicate.go:54), i.e. d == 8.0 qualifies — consistent with the doc "within".
  Bracketed 7.9 in / 8.1 out; exactly-8.0 not constructed (a degrees
  representability limit the handoff honestly notes). The oracle's fixture pair
  (~4.45 m apart) is comfortably inside.
- **Nearest wins, earlier on ties**: strict `<` (duplicate.go:57) keeps the first
  candidate at equal distance; tested with identical construction points
  (bit-identical distances, so the tie is real). Matches the packet's "first
  match wins with nearest preferred".
- **Caller contract documented**: duplicate.go:3–5 tells T-205 the slice must
  combine the 25 m window with the asset_no lookup — the exact shape the
  distance-independent numbered rule requires. Good API hygiene.

## Test integrity

- All five packet-mandated classify cases exist under assertion-style names
  (duplicate_test.go: "same asset_no anywhere…", "unnumbered within 8 m…",
  "numbered new fixture near unnumbered existing is NOT…", "empty candidate list
  never duplicates" ×2, nearest-of-three within 8 m), plus extras: numbered
  nearby, first-numbered-on-slice-order, numbered-over-proximity precedence,
  unnumbered-new-near-numbered, exact tie.
- Distance table covers the packet's asks: equator degree, Groveland-local pairs
  (N, W, diagonal), zero, antipodes ×2. Property loops are seeded
  (`rand.New(rand.NewSource(204))` — reproducible, as the packet demands) and
  cover symmetry, non-negativity and the triangle inequality over 2000 random
  triples plus 9³=729 pole/antipode/dateline triples, all asserted, none
  skipped.
- No `t.Skip`, no `.only`, no special-casing, no golden files touched.
  `TestThresholdsAreFrozen` pins both radii so a silent threshold bump fails
  loudly.
- Every test expectation is derived from a closed form independent of the
  implementation (I re-derived them; see above) — the values are not
  self-generated.

## Security

Pure geometry, no attack surface: no HTTP, no SQL, no templates (no
`template.HTML` — forbidigo clean), no file I/O, no paths, no secrets, no
logging, no CGO. `math/rand` is confined to `_test.go` and is not used for
anything security-relevant (property fixtures only; gosec is excluded for tests
by .golangci.yml:34–37 anyway). Nothing to flag.

## Architecture

- Ownership map: diff touches only `internal/geo/**` + the handoff file. HTMX /
  `#map` untouched (no web/ files). Parse-don't-validate is N/A (no handlers;
  the package takes already-typed inputs). Migrations untouched;
  `status.Derive` unaffected.
- **Observation (non-blocking)**: docs/spec/architecture.md:23 lists
  `internal/geo` as owning "bbox check, Haversine, duplicate distance", but the
  packet (and pre-existing reality) keeps bbox in `internal/config.BBox.Contains`
  and explicitly forbids geo from re-implementing it. The worker followed the
  packet (correct — the spec file is outside `owns`) and documented the
  divergence in geo.go:4–5. Spec-vs-packet drift for the planner to reconcile;
  not a task defect.
- **Observation (non-blocking)**: `Candidate.LightID int64` is an untyped ID,
  which sits loosely against AGENTS.md rule 6 — but it is the packet's frozen
  API verbatim, and no typed `LightID` exists anywhere in `internal/` yet
  (grep: none; store/gen has only raw int64). T-205 should convert at the
  boundary when store types land. Following the packet was the right call.

## Minor notes (no action required for this task)

1. Handoff bookkeeping: "25/25 subtests" is loose — the suite has 4 top-level
   tests with 20 `t.Run` subtests (7 distance + 13 classify); the property
   loops and the threshold pin are not subtests. Cosmetic; the substance of the
   claim (what runs, and that it all passes) is accurate.
2. `DistanceM` can return NaN for out-of-range latitudes (|Lat| > 90 makes a cos
   negative → s < 0). GIGO for invalid input; T-205's boundary parse plus the
   bbox check guard this before any geo call. Not worth code in this package.
3. Pre-existing untracked `node_modules` file on the branch noted in the handoff
   — correctly not touched; worth the chore task the handoff suggests if the
   gate's clean-check ever counts untracked paths.

## Verdict

The frozen API is implemented verbatim, the math is right (independently
re-derived, including the antipodal clamp and the symmetry argument), the
classify semantics match the frozen reconciliation the A-205 oracle will
enforce, tests are honest and exercise every packet-mandated behaviour with
hand-computed expectations, and there is no security or architecture concern.
Non-blocking observations above are for the planner/T-205, not rework.

VERDICT: PASS
