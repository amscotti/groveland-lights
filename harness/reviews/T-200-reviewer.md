# T-200 review — M2 acceptance drafts, fixtures, schema oracle, frozen UI contract

Reviewer scope: the attached diff (also at `harness/reviews/T-200.diff`) against
`harness/tasks/T-200.md`, `acceptance/contract.json`, the downstream packets
(T-201..T-208), `docs/spec/architecture.md`, and the repo's actual test
topology (`playwright.config.ts`, `scripts/acceptance-go.sh`,
`scripts/holdout-run.sh`, `.gitignore`, `.env.test`, `migrations/0001_init.sql`).
Note: shell execution was denied in this session, so dynamic re-runs were not
possible; all findings are from static analysis of the diff plus the repo
facts, which is sufficient because the blockers are logical, not environmental.

## What is correct (verified)

- **Schema oracle** `testdata/schema.expected.sql` is byte-faithful to the
  packet's frozen block (T-200.md:86-169): all 11 tables/indexes compared
  line-by-line, probe statement copied verbatim from `migrations/0001_init.sql:2-5`
  (comment/INSERT correctly excluded). 86 lines, no comments.
- **Fixtures generator** `testdata/photos/gen/main.go`: stdlib only, all 7
  fixtures per Deliverable 1, hand-built EXIF APP1 (TIFF layout verified
  byte-by-byte: IFD0@8, 2 entries, GPS IFD@38 with one GPSLatitudeRef entry —
  consistent with `alHasExifGps`/`hasExifGps`/`parseExifAPP1`), correct APP1
  length field, CRC-correct 45-byte pixel bomb, 16-byte ftyp/heic sniff box,
  15 MB commit cap enforced, self-verifying on every run. Deterministic design.
- **Go drafts** follow `acceptance/go/probe_test.go` conventions (spawn, BIN
  walk-up, free port, kill via t.Cleanup, waitHealthy with context deadline);
  each test spawns its own server on a fresh `t.TempDir()` DB, so the Go suite
  has no cross-test state contamination. `aa*`/`al*` prefixes collide with
  nothing in `acceptance/go/` (probe*/mig*/m1*/syncBuf checked). All imports
  used; JPEG APP1/SOF byte parsers are correct (segment length semantics,
  SOF w/h offsets, both TIFF endiannesses). TestA205's geometry (~4.4 m
  unnumbered pair, ~7 m numbered pair) matches the 8 m/25 m rules. TestA209's
  55 seeds are inside TOWN_BBOX and >8 m apart. Login/CSRF/one-time-token flow
  matches T-203's packet exactly.
- **Active utils** (`acceptance/e2e/util/{admin,images}.ts`): typecheck under
  the repo tsconfig (`acceptance/e2e/**/*.ts` is included), import style
  `./util/*.js` matches `home_m1.spec.ts:16`, axe tags/import match the green
  existing specs, no `console.log`, no tests of their own (per Deliverable 5).
- **Ownership/guard**: only owned paths touched (`acceptance/**`,
  `testdata/photos/**`, `testdata/schema.expected.sql`,
  `harness/protected.sha256`, handoff). The manifest diff adds entries only;
  all 9 pre-existing hashes unchanged and sorted. No `*.golden*`, Makefile, CI,
  or existing-file edits. Scratch verification area is under gitignored
  `testdata/rendered/*` (.gitignore:17) and not committed. Ratchet unchanged
  (correct — drafts are inert). No `t.Skip`/`test.skip`/`.only`/`.fixme`/TODO/
  FIXME. Dummy test secrets only (already in `.env.test`/`playwright.config.ts`).

## BLOCKER 1 — the e2e draft freezes contradictory behaviour for the same action (A-205 unsatisfiable)

`acceptance/drafts/admin-walk.spec.ts.txt`:

- Test 1 (`@A-202 @A-203`, line 50) creates asset `GP-017` "Ball field lamp"
  and expects `Saved` (line 64, 70). The light is never deleted.
- Test `@A-205` (line 152) then creates asset `GP-017` "Dup panel lamp" via
  `addLightFromForm` (line 154) and expects `role=status` `Saved` (line 155),
  before creating the twin that triggers the panel (lines 157-168).

Same asset_no is a **numbered duplicate regardless of distance** — frozen by
this same diff (`acceptance/drafts/README.md` HTTP table, line 60: "duplicate →
409"; see also `harness/tasks/T-204.md:75` "same asset_no anywhere is numbered
duplicate", the partial unique index in `testdata/schema.expected.sql:48`, and
`acceptance/contract.json:66`). Both tests run in one file against one server
(`playwright.config.ts:14` workers=1). Therefore test `@A-205`'s first create
must be refused with the 409 dup panel ("nothing is saved until one is chosen" —
README visual note A-205), `Saved` never appears, and the test fails **on every
project, against a fully contract-compliant implementation**. Conversely, an
implementation that lets it save fails the Go draft `TestA205Duplicates`
(`acceptance/drafts/admin_lights_go_test.go.txt:903` expects 409 for the same
asset_no). The two draft files (and test 1 within the e2e file itself) freeze
mutually exclusive behaviour for the same request. This is exactly the
"unsatisfiable oracle" case, knowable at authoring time from the packet's own
test list (T-200.md:232-236 vs :245-246) plus contract.json; the packet's Never
clause ("If a contract above seems wrong, `BLOCKED: <reason>` and stop")
applied. At T-207 this is a guaranteed BLOCKED (T-207.md:36-40), poisoning the
terminal M2 task.

Note the packet's test 6 wording ("create `GP-017`, then add same asset_no
nearby") is satisfiable read as "reuse test 1's GP-017"; the draft's added
first-create-with-`Saved` step is what makes it unsatisfiable even on project 1.

## BLOCKER 2 — the e2e draft ignores the shared, never-reset server DB across tests and projects

`playwright.config.ts:49-68` starts ONE webServer (`DB_PATH=./test-run.db`,
`reuseExistingServer: false`) for the entire run; all four projects
(desktop-chromium, pixel-9, iphone-16, iphone-se-320) execute every test
against that one accumulating database (T-200.md:213 "All tests on all four
projects"; T-207.md:35 "Expected: all green, all four Playwright projects").
Given the frozen duplicate rules, the drafted data cannot pass:

- Test 1 (GP-017) fails on projects 2-4: the asset already exists from project
  1 → dup panel → no `Saved` (lines 50-70).
- Test `@A-209` (line 220) seeds 60 **unnumbered** "Scroll lamp NN" at fixed
  coordinates (lines 232-243). On projects 2-4 each seed lands 0 m from the
  previous project's identical seed → unnumbered-within-8 m → 409, but the test
  demands 303 (line 242). Deterministic failure.
- Test `@A-203` (drag, line 105) creates an unnumbered light near the previous
  project's dragged position (drag distance in metres is viewport-dependent) →
  panel instead of `Saved` on repeats.
- Test 7's `img[alt^="GP-017"]` (line 177) also depends on which GP-017 rows
  exist/sort first — brittle given tests 1 and 6 both mint GP-017.

The draft even acknowledges the shared DB (lines 224-227: "regardless of the
few lights earlier tests left in this server's database") but only solved the
pagination-boundary half of the problem, not the duplicate-contract half.
`test-run.db` also persists across `make e2e` invocations on disk. The fix is
cheap and entirely in T-200's owns: per-run/per-project unique asset numbers and
seed coordinates (e.g. suffix `testInfo.project.name` + timestamp), or
soft-delete cleanup at test end (test `@A-208` already shows deletion frees the
asset). The same defect is mirrored in the holdout e2e variants (GP-042 +
51-light boundary on the single `scripts/holdout-run.sh` server), which I could
not read (external_directory) but which the handoff describes with the same
shape.

## HIGH 1 — frozen create-success redirect contradicts T-205's packet

The drafts freeze `POST /admin/lights` success → `303 Location:
/admin/lights/{id}` (`acceptance/drafts/README.md:60`;
`admin_lights_go_test.go.txt:22` header and the `detailURL :=
header.Get("Location")` assertions in TestA206/TestA208 at diff lines
998/1145). But `harness/tasks/T-205.md:69` freezes "Success → role=status
`Saved` (fragment for HX; **303 `/admin` without HX**)". One of the two must
lose; workers are told the drafts are the oracle (T-205.md:127-128) while T-205's
reviewer will check the packet. This guarantees an activation-time dispute or a
wrong build. T-200 should either align to `/admin` or escalate to the planner
now, and record it in the handoff's open issues (currently absent).

## HIGH 2 — numbered-duplicate panel buttons contradict T-205's packet

`admin-walk.spec.ts.txt:168` requires a visible `Save anyway` button in the
dup panel for the **numbered** (same asset_no) case, and README line 34 lists
`Merge`, `Save anyway` unconditionally. `harness/tasks/T-205.md:65-68` freezes
the opposite: numbered case shows `Merge` + a cancel link, `Save anyway` is
unnumbered-only, and `dup_choice=save` on a numbered duplicate → 422 (also
implied by `harness/state.md:45-47`). A worker following its packet fails the
e2e test; a worker following the draft violates the packet and the Go draft's
own semantics. Needs the same reconciliation before the drafts freeze.

## HIGH 3 — kind enum drift

`README.md:60` and `admin_lights_go_test.go.txt:19-21` freeze
`kind(park|building|street)` for the create form; `harness/tasks/T-205.md:58`
freezes `kind (park|building)` (schema allows all three). No test posts
`street`, so it is satisfiable either way, but the oracle should not contradict
the worker packet it governs — pick one and note it.

## MEDIUM 1 — `#pin-pos` precision under-specified for the 6 dp equality check

`admin-walk.spec.ts.txt:124-139`: the test recomputes `expected` from the
`#pin-pos` text via `Number(x).toFixed(6)` and requires `#light-pos` to equal
it exactly. README line 33 freezes `#pin-pos` only as "same numeric format"
(the loose `\d+\.\d+, -?\d+\.\d+` regex), so a compliant worker rendering
`#pin-pos` at < 6 dp with lossy rounding breaks the equality while still
matching the frozen format. Freeze `#pin-pos` at ≥ 6 dp (or make both elements
render the identical 6 dp string) to keep the oracle unambiguous.

## MINOR

- `acceptance/e2e/util/images.ts` `jpegSize` reads `bytes[i+5..i+8]` without
  the truncated-SOF guard the Go twin has (`alJpegDims` checks `next < i+9`);
  a malformed short SOF yields `{w: NaN, h: NaN}` instead of a loud throw. The
  Go version is the better contract; mirror it.
- `testdata/photos/gen/main.go` uses `panic(...)` on encode errors
  (`encodeJPEG`, `tinyPNG`) where the file's own `fail()` convention would be
  consistent. Not rule-9 panics (real messages, unreachable paths), style only.
- Handoff says "5 new entries (3 drafts + README + 2 utils)" — the manifest
  adds 6 entries (that list itself enumerates six). Trivial, but handoffs are
  evidence.
- Handoff's "would-fail reasoning" covers only current-main 404s. The packet
  asked T-200 to reason about failure; the analysis that mattered —
  satisfiability against a *compliant* build under the real Playwright
  topology — is what's missing, and that gap is precisely how Blockers 1-2
  shipped.

## Security / architecture / test-quality pass

- Security: no app code, no `template.HTML`, no SQL, no path handling in this
  diff. Upload limits are well-covered (413 over 15 MB with a runtime-built
  body, 415 HEIC/text/pixel-bomb, no-stray-files, APP1/GPS strip, long-edge
  cap). The only literals are the dummy `.env.test` secrets already public in
  the repo (`probe_test.go:297`, `playwright.config.ts:60`). The drafts
  correctly require `csrf_token` on every admin POST. A `/photos/` traversal
  probe would have been nice (T-206 covers it in unit tests; not required by
  the packet) — not blocking.
- Architecture: ownership map respected (test-only paths, all in `owns`);
  drafts never swap `#map` (admin pagination targets `#rows`; the walk page
  uses a pin pad, not the map instance); the auth drafts pin HttpOnly +
  SameSite=Lax on `gl_session` and one-time tokens, matching T-203. No
  "Phase N" strings inside phase-grep scope (the e2e header comment says
  "Phase-2" but lives in `acceptance/`, outside the grep scope per D7).
- Coverage: every claim A-201..A-209 has at least one Go and one e2e draft
  test; assertions are black-box (HTTP status/body/headers/bytes, real
  browser), matching the repo's oracle-first style. The tests do exercise the
  behaviour — the problem is that three of them exercise it into a corner no
  compliant build can escape.

## Required before merge

1. Make the e2e draft rerun-safe and self-consistent: unique asset numbers /
   seed coordinates per test AND per project (or cleanup via soft delete);
   remove or repair the `@A-205` first-create-with-`Saved` step (Blockers 1-2).
2. Reconcile the three frozen-contract conflicts with the worker packets —
   create-success redirect target, numbered-dup panel buttons, kind enum —
   either by amending the drafts (still pre-freeze) or by escalating
   `BLOCKED: <contract conflict T-200 vs T-205>` for the planner, and record
   the decision in the handoff (HIGH 1-3).
3. Mirror the fixes in the holdout drafts (same shapes per the handoff).
4. Sharpen `#pin-pos` precision (MEDIUM 1) and the handoff's manifest count /
   would-fail analysis (MINOR).

The Go drafts, schema oracle, fixtures, utils, and manifest work are ready;
the e2e oracle is not. Because these bytes are about to be frozen
(protected manifest) and six downstream tasks build against them, an oracle
that provably cannot go green on all four projects must be fixed now, not at
T-207 activation.

VERDICT: BLOCK
