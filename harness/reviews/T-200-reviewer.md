# T-200 review (attempt 3) — M2 acceptance drafts, fixtures, schema oracle, frozen UI contract

Review basis: the attached diff (`harness/reviews/T-200.diff`), the operative
packet `harness/tasks/T-200.md` (including the attempt-1 and attempt-2
reviews), the downstream packets T-203/T-205 (read in full), T-204/T-207 (via
the prior reviews' citations), `acceptance/contract.json`, and the repo's
actual topology: `playwright.config.ts`, `Makefile`, `scripts/acceptance-go.sh`,
`scripts/holdout-run.sh`, `.gitignore`, `.env.test`, `migrations/0001_init.sql`,
`cmd/harness/{guard,ratchet,scan}.go`, `harness/protected.sha256`,
`acceptance/go/*`, `acceptance/e2e/*`, `tsconfig.json`, `package.json`,
`docs/spec/architecture.md`.

Verification limits: shell execution was denied and the diff is not applied to
this checkout (no `acceptance/drafts/`, no `testdata/photos/`), so sha256
recomputation and suite re-runs were not possible; the holdout repo is outside
the sandbox. Everything below is a static check of the diff bytes against the
repo and packets — the same basis as the two prior reviews. The gate itself
re-verifies the two things I could not: `VerifyManifest`
(`cmd/harness/ratchet.go:15-47`) fails loudly on any manifest mismatch, and
`go run ./testdata/photos/gen` self-verifies every fixture byte on each run.

## Attempt-2 findings — all twelve closed (verified in the diff)

1. **BLOCKER (A-205 first create never submitted)** — fixed:
   `acceptance/drafts/admin-walk.spec.ts.txt` `@A-205` now calls
   `addLightFromForm(...)` then clicks `Confirm pin` and waits for `Saved`
   before re-posting the same asset to raise the panel. The full numbered →
   Merge → unnumbered → Save-anyway flow is walked, including the
   nothing-saved checks between panels (`img[alt*="Dup panel lamp twin"]`
   count 0; `img[alt="Path light two"]` count 0).
2. **HIGH (one-link-per-row assumption)** — fixed: e2e `lightHrefsByLabel`
   collects hrefs into a `Set`; Go `alDetailURL` dedupes via a `seen` map
   before requiring exactly one distinct href; README list contract now
   freezes "a row may repeat its detail href — href consumers dedupe".
3. **MED (replay only from same client)** — fixed: the replay probe in
   `admin_auth_go_test.go.txt` uses `bare`, the second cookie-less client;
   the HTMX 401 guard probe also uses `bare`; the header comment and README
   freeze "one-time, GLOBAL: replay from any client → 403", matching
   T-203's DB-backed `ConsumeAdminToken`/`ErrTokenReused`.
4. **MED (asset numbers shared across tests)** — fixed:
   `uniqAsset(prefix, testInfo, tag)` adds a per-test tag
   (`save|dup|photo|edit`); max length ≈24 chars, still
   `^[A-Za-z0-9-]{1,32}$`, still matched by `img[alt^="GP-017"]`.
5. **MED (low-accuracy gate proved only button state)** — fixed: the test
   completes a real save at 80 m (choose save-anyway → enabled → form →
   `Confirm pin` → `Saved` → row visible on `/admin`) on jittered per-run
   coordinates. Rationale for no server-side negative probe is correct:
   T-205's frozen check order (`harness/tasks/T-205.md:60-68`) has no
   accuracy gate server-side; the gate is the UI control, so a wired save is
   the strongest compliant assertion.
6. **MED (CSS-stretched preview could pass)** — fixed: preview asserted on
   intrinsic dimensions via `locator.evaluate` (`naturalWidth >
   `naturalHeight`), which honours EXIF orientation.
7. **LOW (numbered-panel negatives)** — fixed: `Save anyway` `toHaveCount(0)`
   inside `#dup-panel` plus a visible `Cancel` link; the `Cancel` name is
   newly frozen (README reconciliation 2 + selectors table) and flagged for
   planner ratification in the handoff.
8. **LOW (e2e purity GPS-only)** — fixed: `images.ts` gains `hasAPP1`
   (shared `app1Segment` walker; `hasExifGps` semantics unchanged); the e2e
   asserts `hasAPP1 === false` AND `hasExifGps === false`, matching the Go
   `alFindApp1` byte-scan.
9. **LOW (keyboard pin unexercised)** — fixed: after the poll-verified drag,
   the test focuses `#pin`, presses `ArrowRight` ×3, polls `#pin-pos` for a
   further change, saves THAT position and requires `#light-pos` to equal
   the final 6 dp string.
10. **LOW (handoff overstatement)** — fixed: the handoff separates the
    red-against-main run (scoped as such) from a dedicated satisfiability
    section walking each fix against a compliant build.
11. **LOW (cache-write fallback comment wrong)** — fixed: `writeCache`
    surfaces the failure on stderr with the one-time-token consequence and
    the operator fix; README states `make check`'s e2e→a11y sequence requires
    a writable cache path.
12. **LOW (`#rows` root unasserted)** — fixed: `alReRowsRoot`
    (`\A\s*<[a-zA-Z][^>]*\bid="rows"`) asserted on the page-2 fragment.

The attempt-1 fixes (unique assets/coords, `maxRedirects: 0`, the three
T-205 reconciliations, `#pin-pos` at exactly 6 dp, `jpegSize` truncation
guard, error-returns instead of panics in the generator, manifest count)
are all still present in the attempt-3 bytes.

## What is correct (verified against the repo, not just the handoff)

- **Packet deliverables**: all six present. Fixtures + deterministic
  self-verifying generator (`testdata/photos/gen/main.go`, stdlib only, all
  7 fixtures, 15 MB commit cap, APP1/TIFF layout re-parsed by
  `parseExifAPP1`, CRC-correct 45-byte pixel bomb, 16-byte ftyp/heic box).
  Schema oracle `testdata/schema.expected.sql` is line-for-line the packet's
  frozen block (T-200.md:86-169) with the probe statement verbatim from
  `migrations/0001_init.sql:2-5`. Go drafts, e2e draft, the two ACTIVE utils,
  the README contract tables, and holdout drafts (per handoff; outside
  sandbox) all present.
- **Reconciliations now match the worker packets exactly** (this was the
  attempt-1 HIGH 1-3 conflict; I diffed the README against T-205.md
  directly): create success → `role=status` `Saved` for HX / `303 /admin`
  without HX and detail discovery via list rows (T-205.md:69);
  numbered panel `Merge` + cancel link, `Save anyway` unnumbered-only,
  numbered+save → 422 (T-205.md:64-68); kind `park|building` (T-205.md:58);
  auth 302 exchange + `gl_session` HttpOnly/SameSite=Lax (T-203.md:62-69).
  All five amendments are recorded for planner ratification (handoff note 1)
  — the right escalation path short of BLOCKED, since none of them
  contradicts a worker packet.
- **Satisfiability of the tricky probes**: T-205.md:60-68 freezes
  parse → CSRF → bbox → photo (413/415) → duplicates, so
  `TestA206PhotoPipeline`'s HEIC/text/bomb creates at the same coordinates
  as an existing unnumbered light correctly expect 415 (photo validation
  precedes the duplicate check), and the 16 MB probe dies at `LimitBody`
  first. `TestA207NoStrayFiles` runs its rejects on a fresh server before
  any successful create. Both are satisfiable against a compliant build.
- **Rerun safety is coherent under the real topology**: I verified
  `playwright.config.ts` has `workers: 1` and `fullyParallel: false`
  (lines 13-14), one webServer with `DB_PATH=./test-run.db`,
  `reuseExistingServer: false`, port 8931 — so a single worker process runs
  all projects sequentially. `admin.ts`'s in-process memo/`inFlight` then
  covers the whole e2e invocation; the gitignored
  `test-run.db.session.json` (`.gitignore:20` covers `test-run.db*`) hands
  the still-valid DB-backed session to `make check`'s second (`a11y`)
  Playwright invocation, which boots a fresh server on the same DB —
  exactly the design the sessions table (schema oracle) supports. The
  ORIGIN default (`TEST_PORT ?? 8931`) matches the config. Coordinates:
  jitter band lat 42.72-42.75 / lon -71.08..-71.0501 and the fixed
  per-project pagination lanes at -71.0480..-71.0495 are ≥ ~49 m apart
  (0.0006° lon) and ≥ ~41 m lane-to-lane, so grid/point and grid/grid
  8 m collisions are deterministic impossibilities; the default walk
  position (42.7617, -71.0303) is >1.3 km from the band and only ever hosts
  numbered lights with unique assets. The residual point-vs-point birthday
  (≈0.07%/run) is disclosed and fails loudly (dup panel), not silently.
- **Inertness is structural, not conventional**: `.go.txt` is compiled by
  nothing (`scripts/acceptance-go.sh:36` runs `./acceptance/go/...` only);
  the spec draft lives in `acceptance/drafts/`, outside `testDir:
  'acceptance/e2e'` and with an extension Playwright's testMatch cannot
  match; `tsconfig.json` includes only `acceptance/e2e/**/*.ts`, so
  `make js-check` type-checks the two ACTIVE utils and ignores the draft.
  `countGoTests` runs `go test -list` without the acceptance tag and
  `countPwTests` lists via Playwright, so the ratchet stays `go=51 pw=32` —
  correctly unchanged (handoff note 6 discloses the pre-existing nuance
  that activated Go drafts will not raise the count; the manifest remains
  their protection).
- **Ownership/guard**: every touched path is in the packet's `owns`
  (`acceptance/**`, `harness/protected.sha256`, `testdata/photos/**`,
  `testdata/schema.expected.sql`) plus the handoff file; the guard
  explicitly allows `acceptance/**` and `harness/protected.sha256` for
  type-acceptance tasks (`cmd/harness/guard.go:243-246`). The manifest diff
  is add-only: all 9 pre-existing entries byte-identical, 6 new sorted
  entries — exactly the shape `GenerateManifest` (ratchet.go:49-78)
  produces over `acceptance/**`. No existing test edited, no skip markers
  (the marker scan exempts `acceptance/` and `*_test.go`, and the only
  non-exempt added file, the generator, is clean), no deps changed
  (`@axe-core/playwright` 4.14.0 already in package.json; Go side stdlib
  only), scratch staging under gitignored `testdata/rendered/*`
  (.gitignore:17). No "Phase N" strings in scope, no TODO/FIXME/panic, no
  `console.log`.
- **Go draft conventions**: `aa*`/`al*` helper families collide with nothing
  in the `acceptance` package (`probe*`/`mig*`/`m1*` verified by grep), each
  file is self-contained for the pure-rename activation, spawn/env/kill/
  waitHealthy mirror `probeSpawnDir` (probe_test.go:274-306) including the
  `.env.test` dummy secrets (verified identical in `.env.test` and
  `playwright.config.ts:58-61`). JPEG byte parsers (APP1 walker, SOF dims
  with truncation guards, TIFF both endiannesses) are correct.

## Findings (all low — none block)

1. **LOW — `Path=/` frozen but unasserted.** README freezes the
   `gl_session` cookie with `Path=/` (and T-203.md:63 specifies it), but
   `TestA201AdminLogin` asserts only `HttpOnly` and `SameSite=Lax`. A
   cookie scoped to `/admin` would pass the visible oracle. One-line
   strengthening for T-207 to consider (or the holdout may already cover
   it); not worth refreezing the drafts over.
2. **LOW — strict-mode single-match assumptions on live regions.** The e2e
   draft uses `page.getByRole('status')` / `getByRole('alert')` without
   `.filter()`/`.first()`. A compliant page that renders a second
   `role=status` or `role=alert` region would trip Playwright strict-mode
   rather than mismatch. The frozen contract designates exactly one of
   each, and workers build to the drafts, so this is brittleness, not
   unsatisfiability.
3. **LOW — token-exchange robustness leans on `workers: 1`.** `admin.ts`'s
   memo is per-process; two concurrent worker processes against a fresh DB
   could race the one-time exchange (loser gets 403). The protected config
   pins `workers: 1`/`fullyParallel: false` today, which makes the design
   sound; anyone raising workers later (config is protected, so it would be
   deliberate) needs to know this. Worth a line in T-207's notes.
4. **INFO — verification limits.** Manifest/fixture sha256s and the suite
   exit codes in the handoff are taken on trust (shell denied); the gate
   re-verifies the manifest and the generator self-verifies, so a wrong
   hash fails loudly rather than silently. The holdout drafts remain
   outside the sandbox; the handoff's mirroring claims are consistent with
   `scripts/holdout-run.sh` only ever loading `go/*_test.go` and
   `e2e/*.spec.ts` (drafts stay inert there too).
5. **INFO — cleanup-on-failure.** Tests clean up at the end, so a mid-test
   failure leaves rows; recovery is by pre-clean on rerun (fixed labels)
   plus per-run unique assets/coordinates, so leftovers cannot poison a
   later test's assertions. Disclosed in the README's rerun-safety section.

## Security / architecture / test-quality pass

- No app code in the diff: no `template.HTML`, no SQL, no path handling, no
  secrets beyond the dummy `.env.test` values already public in
  `playwright.config.ts`/`scripts/*.sh` (gitleaks-equivalent exposure
  unchanged). The session cache file is 0600, gitignored, and dies with its
  DB.
- The oracle itself enforces the security posture the packets freeze: CSRF
  token on every admin POST (multipart, double-submit), one-time GLOBAL
  token with cross-client replay 403, HttpOnly + SameSite=Lax on
  `gl_session`, 15 MB cap (413, runtime-synthesised body), type sniff +
  HEIC wording + >40 MP bomb (415), no-stray-files on every reject path,
  full APP1 strip (stronger than GPS-only), long-edge ≤ 1600, soft delete
  freeing the partial-unique index, unspecified-field preservation on edit.
- Architecture: ownership map untouched (test/fixture paths only); the
  drafts never swap or wrap `#map` — pagination swaps `#rows`, the walk
  page uses a pin pad, and the e2e asserts no full navigation on the HTMX
  swap; no parse/typed-ID surface is introduced (tests are black-box over
  HTTP/DOM, the correct level for acceptance).
- Tests exercise behaviour, not mocks: Go drafts spawn the real binary over
  HTTP per test (fresh `t.TempDir()` DBs, `ErrUseLastResponse` clients,
  observable 303/409/413/415/422/302/401 and response bodies/headers/
  bytes); the e2e drives a real browser through the real UI on all four
  projects, including the mobile 320×640 primary target, with an axe pass.
  Every claim A-201..A-209 has at least one Go and one e2e draft test.

## Verdict rationale

The attempt-3 diff closes all twelve attempt-2 findings (each verified in
the bytes above), preserves the attempt-1 fixes, and — the part that
mattered — the previously contradictory freezes now match the worker
packets verbatim (checked against T-203.md and T-205.md directly), the
shared-DB rerun-safety scheme is sound under the actual Playwright topology
(`workers: 1`, persistent `test-run.db`, sequential e2e→a11y invocations
bridged by the gitignored session cache), and the remaining probes are
satisfiable against T-205's frozen check order. Ownership, inertness,
manifest and ratchet handling are all exactly what the packet and guard
sanction. The residual findings are low-severity brittleness notes for
T-207, none of which justifies refreezing the oracle. The drafts are ready
to freeze.

VERDICT: PASS
