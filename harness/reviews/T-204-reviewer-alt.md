# T-204 — independent review (reviewer-alt): internal/geo

Basis: the attached diff (`harness/reviews/T-204.diff`), packet
`harness/tasks/T-204.md`, `AGENTS.md`, `acceptance/contract.json` (A-204/A-205),
`acceptance/drafts/README.md`, the inert drafts (`admin-walk.spec.ts.txt`,
`admin_lights_go_test.go.txt`), `harness/state.md` and the `Makefile`.

Verification limit: the diff is **not applied** in this checkout (`internal/geo/`
does not exist), and shell execution is denied in this session. This is a static
review of the diffed bytes. The handoff's executed results (test counts, exit
codes) are therefore taken on trust, and one of those claims is internally
inconsistent (finding 1). Line numbers below are post-patch `file:line`.

## Test-integrity / reward-hacking checks

- **No pre-existing test was weakened, deleted, skipped or narrowed.** The diff
  adds a new package and its tests; it edits no existing test file. No
  `t.Skip`/`test.skip`/`test.only`/`.fixme` appears in the added bytes.
- **No fixture-value special-casing in production.** The only new production
  literals are `25.0`, `8.0`, `6371008.8` and `math.Pi/180`
  (`internal/geo/geo.go:14-26`); the acceptance fixture strings (`GP-017`,
  `GP-018`, `GP-030`) appear only in `internal/geo/duplicate_test.go`.
- **No mocks bypassing the behaviour under test.** The tests call the real
  `DistanceM`/`ClassifyDuplicate`; the package has no I/O to stub.
- **The classify suite is load-bearing, not coverage theatre.** The 2 km row
  (`internal/geo/duplicate_test.go:39-46`) fails if numbered matching is gated
  on `NumberedRadiusM`; the 8.1 m row (`:104-109`) fails if the unnumbered
  radius is widened to 25 m; the cross-kind rows (`:88-93`, `:111-116`) fail if
  the branch split leaks; the nearest row (`:132-143`) fails if the first
  candidate is reported instead of the closest; the tie row (`:145-155`) pins
  the strict `<` at `duplicate.go:57`; the empty-slice rows (`:118-130`) pin the
  false return.
- **The distance table is independently derived.** The expected values are
  closed-form reductions, not outputs of the implementation under test: one
  degree `R*pi/180`, a pure-meridian 0.001 deg arc, a pure-longitude arc at
  `cos(42.76)`, a diagonal from the exact haversine terms, and `pi*R` for the
  antipodes. A formula error in `DistanceM` cannot satisfy them by construction.
- **No dead code added to satisfy a linter.** `NumberedRadiusM` has no consumer
  yet because the packet freezes it for T-205; it is an exported API constant,
  not a lint workaround.
- **No dependency or protected-path change.** Only `internal/geo/**` and the
  handoff are added; `acceptance/**`, `harness/` (other than the handoff),
  `Makefile`, CI, goldens and `go.mod`/`go.sum` are untouched. Imports are
  stdlib `math`/`math/rand` only.

## Findings

### 1. [Low — handoff reporting is inconsistent with the test tree] "25/25 subtests" and "4 property checks"

`harness/handoffs/T-204.md:51` claims the run reported "25/25 subtests: 7
distance-table cases, 4 property checks × 2000 seeded random triples + 729 nasty
triples, 13 classify cases, threshold pin". The Go tree has **20 `t.Run`
subtests** — 7 distance rows (`internal/geo/geo_test.go:30-69`) and 13 classify
rows (`internal/geo/duplicate_test.go:39-155`) — plus two top-level tests. The
property test (`geo_test.go:93-142`) contains no `t.Run`; its `check` closure
(`:104-123`) has three assertions (symmetry, non-negative, triangle), not four
checks. The 25 figure only works if one counts logical assertions and top-level
tests as "subtests". The claimed `ok, exit 0` may well be true, but the evidence
narrative is not the test tree and no captured output is attached. This is a
reporting problem rather than a code/test hole; correct the count or attach the
raw `go test` output.

### 2. [Low-Medium — A-205 contract shorthand vs the frozen reconciliation; planner must ratify] "same asset_no within 25m" is implemented as distance-independent

`acceptance/contract.json:64-67` says A-205 fires for "same asset_no within
25m", while `internal/geo/duplicate.go:32-43` treats any equal non-empty
asset_no as a numbered duplicate at any distance, and
`internal/geo/duplicate_test.go:39-46` freezes a 2 km match. This matches the
protected reconciliation in `acceptance/drafts/README.md:100-101` ("same
non-empty asset_no anywhere → numbered duplicate; 25 m only bounds the candidate
window") and `harness/state.md:45-47`, and no active spec asserts a negative
beyond 25 m (`acceptance/e2e/` has no A-204/A-205 tests). The contract sentence
is a one-way "shall", so the implementation is a superset rather than a
violation, but the protected wording is stale and the worker cannot amend it.
The handoff already discloses this at `:73`. Planner should ratify or amend
A-205 before T-205/T-207; not a gate block.

### 3. [Low — property assertion relaxed in the one regime it matters] 2 m antipodal slack

`internal/geo/geo_test.go:98-103` defines `antipodalZoneM = 1.9e7` and
`antipodalSlackM = 2.0`; `:113-118` applies the 2 m slack to any triple with a
leg beyond 1.9e7 m. The handoff's own observation is 0.048 m (`:72`; test
comment `geo_test.go:89-92`) and the analytic bound for plain haversine near
`s = 1` is ~0.1 m, so 2 m is roughly 20-40× looser than needed. It is disclosed,
bounded to near-antipodal legs (irrelevant to the 8 m town-scale decision), and
the table's antipodal rows (`geo_test.go:54-63`) plus the symmetry/NaN check
still pin the clamp. But the packet's triangle-inequality property is the one
assertion that is relaxed, and it is relaxed exactly where the implementation is
known to lose precision. Tightening to ≤0.25 m, or comparing against the
complementary-angle form `π/2 − asin(√(1−s))` for those legs, would keep the
property meaningful without changing production.

### 4. [Low — inclusive boundary bracketed, not pinned] exactly 8.0 m is untested

`internal/geo/duplicate.go:54` implements `d > UnnumberedRadiusM` → no match,
i.e. inclusive at 8.0. Tests use 7.9 m in (`duplicate_test.go:95-102`) and 8.1 m
out (`:104-109`); no row constructs exactly 8.0 m, so a `<`-for-`≤` regression
or a 1-ulp boundary change would stay green. The handoff acknowledges this at
`:74`, and the packet only asks 7.9/8.1, so it is not a requirement failure.
A test placing `8.0 / metresPerDegLat` degrees north along a meridian
(construction error ~1e-9 m) would pin the "within" wording.

### 5. [Low — defensive branch only indirectly exercised] the `s > 1` clamp

`internal/geo/geo.go:40-42` clamps `sin² + cos²` to 1 so `Asin(√s)` stays
finite. No test directly forces the terms to round above 1: the antipodal table
rows (`geo_test.go:54-63`) and the 729 nasty triples may or may not overshoot;
if none does, deleting the clamp leaves the suite green. The clamp is not part
of the 8 m contract, but the handoff calls it out (`:15`), so a known-rounding
antipodal pair or a dense antipodal sweep asserting "no NaN" would make the
branch evidence direct rather than incidental.

### 6. [Info — integration precondition for T-205] the numbered rule needs the caller's asset_no lookup, not just the 25 m window

`internal/geo/duplicate.go:3-5,24-27` and handoff `:43` state that the caller
must union the asset_no lookup with the near-candidate window. `ClassifyDuplicate`
only classifies the slice it is given; `duplicate_test.go:39-46` builds the 2 km
candidate itself, so it does not prove a T-205 caller will fetch it. A T-205
implementation that feeds only `NumberedRadiusM`-window candidates would miss
the 2 km case and rely on the DB unique index to produce the 409. Flag for
T-205, not a T-204 defect. `NumberedRadiusM` having no consumer here is per the
packet, not dead code for a linter.

### 7. [Info] `TestThresholdsAreFrozen` is a change-detector, not behavioural coverage

`internal/geo/duplicate_test.go:18-25` compares the two exported constants to
literals. It legitimately freezes the A-205 radii, but it proves nothing about
classification; the real radius coverage is the 7.9/8.1 rows.

## Checked and found sound

- **Frozen API matches the packet exactly**: `Point`, `DistanceM`, the two
  constants, `Candidate`, `DupDecision`, `ClassifyDuplicate` (packet
  `harness/tasks/T-204.md:29-65`; code `internal/geo/geo.go:14-32`,
  `internal/geo/duplicate.go:6-28`).
- **The distance math is correct.** I re-derived the table independently:
  `R*pi/180 = 111195.0803 m`; 0.001° of latitude = 111.1951 m; 0.01° of
  longitude at 42.76° ≈ 816.40 m; the 0.005° diagonal ≈ 689.74 m (Pythagoras on
  555.98 m north and 408.20 m east); `pi*R` ≈ 20015114.44 m. All are inside the
  0.5 m tolerance. The classify points are pure-meridian reductions through
  `8.0 / 111195.0802` degrees, so construction error is far below the 0.05 m /
  0.5 m tolerances.
- **Decision semantics are coherent.** The numbered branch requires a non-empty
  new asset number and exact equality (`duplicate.go:32-43`); the unnumbered
  branch requires both sides empty, `≤ 8 m`, selects the nearest, and the strict
  `<` keeps the earlier candidate on an exact tie (`:54-59`); cross-kind matches
  are impossible because the branches are disjoint; both empty-slice paths
  return `(DupDecision{}, false)`.
- **The property test is honest.** Seeded `rand.NewSource(204)`
  (`geo_test.go:94`), 2000 random triples plus all 729 nasty triples
  (`:124-133`), asserting symmetry, non-negativity and the triangle inequality;
  nothing is skipped and the 2 m regime is documented rather than hidden.
- **The clamp cannot produce NaN.** `math.Sqrt(s)` is safe because `s` is a sum
  of squares and is clamped above; `Asin` receives `[0,1]`.
- **No protected path, dependency, dead lint-code, `TODO`/`FIXME`,
  `panic("not implemented")`, "Phase N" string or unexplained `//nolint`** is in
  the added bytes.

## Verdict rationale

The package matches the frozen API and the protected drafts reconciliation, and
the suite genuinely pins the decision rules with independent closed-form
reference values. I found no deleted, skipped, narrowed or special-cased test,
no fixture branch in non-test code, no mock substituting for the behaviour under
test, and no coverage theatre in the classifier. The findings are a stale A-205
contract wording for planner ratification, one over-generous but disclosed
antipodal slack, two small missing boundary/branch tests, an integration
precondition for T-205, and an inaccurate handoff test count. None lets a
broken deliverable through the gate.

VERDICT: PASS
