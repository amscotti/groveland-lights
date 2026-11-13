# T-200 review (attempt 4) — M2 acceptance drafts, fixtures, schema oracle, frozen UI contract

Review basis: the attached diff (`harness/reviews/T-200.diff`) against
`harness/tasks/T-200.md`, `acceptance/contract.json`, the downstream packets
(T-201, T-202, T-203, T-204, T-205, T-206, T-207), `docs/spec/architecture.md`,
and the repo's actual test topology (`playwright.config.ts`, `Makefile`,
`scripts/acceptance-go.sh`, `scripts/holdout-run.sh`, `.gitignore`,
`tsconfig.json`, `migrations/0001_init.sql`, `harness/.ratchet.json`,
`harness/policy.json`, and the existing `acceptance/go/*` helpers).

Verification limits: the diff is not applied in this checkout (no
`acceptance/drafts/` on disk) and shell execution is denied, so dynamic
re-runs, sha256 verification of the fixture table, and holdout inspection
(`<holdout_dir>` from harness/policy.json is outside the workspace) were not
possible — same constraints as the three prior review rounds. Every finding
below is a static check of the drafted bytes against the contracts and the
real topology; the handoff's exit codes are taken on trust where marked.

## Prior findings — all closed (re-verified in the final bytes)

Every blocker/high/medium from attempts 1–3 was re-checked in this diff:

- **Attempt-1 B1 / B2, attempt-2 #1/#2 (unsatisfiable e2e oracle)**: closed.
  `uniqAsset` (admin-walk.spec.ts.txt:97) yields `GP-017-<run>-<proj>-<tag>`
  (≤32 chars, budget verified; `projectCode` default branch maps
  `iphone-se-320`→`ipho`); every unnumbered create rides `jittered()`
  (grid ≥8.2 m lon / 11.1 m lat apart, inside TOWN_BBOX — bounds verified);
  pre/post-clean by label via soft delete; @A-206 is self-contained (creates
  its own light — `make e2e-task TASK=T-206` works, Makefile:111-116);
  @A-205's first create now clicks `Confirm pin` before expecting `Saved`.
- **One-time token economy**: closed. `acceptance/e2e/util/admin.ts`
  exchanges at most once per `test-run.db` lifetime (in-process memo +
  gitignored `test-run.db.session.json`, covered by `.gitignore:20`
  `test-run.db*` — verified) with a validation GET before adoption
  (admin.ts:108-125) and a documented recovery path when the 24 h TTL lapses.
  The e2e→a11y bridge is real: `make check` runs `e2e` then `a11y`
  (Makefile:136, 118-120) as two Playwright invocations against the same
  persistent `DB_PATH=./test-run.db` with `reuseExistingServer: false`
  (playwright.config.ts:56,66). `workers: 1`/`fullyParallel: false`
  (playwright.config.ts:13-14) make the `inFlight` serialisation sufficient.
  The A-201 one-time/GLOBAL semantics themselves are asserted by the Go draft
  against fresh servers — the helper does not weaken the oracle.
- **Redirect observability**: closed — `maxRedirects: 0` on every
  `page.request.post` (deleteLight, pagination seeds).
- **HIGH 1–3 (packet contradictions)**: closed via README reconciliations
  1–3, all verified against the worker packets: 303 `/admin` without HX =
  T-205.md:69; numbered panel `Merge`+`Cancel`, `Save anyway`
  unnumbered-only, numbered+save 422 = T-205.md:65-68 (both asserted: Go
  422 at admin_lights_go_test.go.txt TestA205Duplicates; e2e negative
  `toHaveCount(0)` + `Cancel` link in @A-205); kind park|building =
  T-205.md:58. "Same asset_no anywhere" matches T-204.md:75-76 verbatim
  (contract.json's 25 m is only the candidate window per T-204.md:32-35 — no
  conflict).
- **MEDIUM 1 (`#pin-pos` precision)**: closed — 6 dp frozen in the README
  additional-selectors table and asserted as `^\d+\.\d{6}, -?\d+\.\d{6}$`,
  with the @A-203 detail check comparing the exact same string.
- **All Lows**: replay now from the cookie-less `bare` client
  (admin_auth_go_test.go.txt, `bare` created at ~:70, replay ~:122);
  blank token → 200 asserted (matches T-203.md:53); low-accuracy test now
  completes a REAL save at 80 m; preview uses intrinsic dims with a decoded-
  frame poll; `hasAPP1` asserted in e2e too; keyboard pin exercised and
  saved; `jpegSize` truncated-SOF guard mirrors `alJpegDims`; detail-href
  dedupe in both `lightHrefsByLabel` and `alDetailURL`; `#rows` root
  asserted (`\A\s*<[a-zA-Z][^>]*\bid="rows"`); whitespace-tolerant
  `Lights` heading; `path=/` asserted on the Set-Cookie (T-203.md:62-64);
  exact 800×600 asserted (T-202.md:83's own freeze; no-upscale rule
  T-202.md:79 makes it satisfiable); `Older` is link-or-button; HEIC body
  substring "please choose JPEG" ⊂ T-206.md:69's frozen message.

## Correctness spot-checks that passed

- Spawn helpers are byte-identical in env to the proven `probeSpawnDir`
  (probe_test.go:289-301); health-wait, free port, BIN walk-up, kill on
  cleanup all follow the established style. No top-level identifier
  collisions with `probe*`/`mig*`/`m1*`/`syncBuf`/`envOr` (grep-verified) —
  the `aa*`/`al*` prefixes are disjoint and each draft file is
  self-contained as required for pure-rename activation.
- Drafts are genuinely inert: `.go.txt` is not compiled by
  `scripts/acceptance-go.sh:36` (`./acceptance/go/...`), `go vet ./...`,
  `gofmt`, or golangci-lint; `.spec.ts.txt` sits outside `testDir:
  'acceptance/e2e'` and fails the default testMatch. Ratchet
  `{"go":51,"pw":32}` unchanged (harness/.ratchet.json) — correct for inert
  drafts. New ACTIVE utils match no spec pattern and are covered by
  `make js-check` via tsconfig.json:15.
- @A-209 geometry: seeds at lat `grid.lat + i*0.0005` (≤42.7775, in-bbox),
  55 m apart, on per-project fixed lon lanes −71.0480..−71.0495 (outside the
  jitter band, 41 m between lanes) — deterministic within a run; the
  cross-run leftover risk is swept by the stable `Scroll lamp` prefix
  pre/post-clean (pages 1–2 always cover the ≤65 matching rows; 6 sweeps
  converge). "Seeds are the 55 newest regardless of leftovers" holds because
  `created_at DESC, id DESC` is frozen with the id tie-break — the exact
  same-second hazard rapid creates hit.
- Login-order discipline: every test calls `loginAsAdmin` before any
  label-sweep (which needs the session for `page.request.get('/admin')`).
- Constant mocked geolocation during @A-203 means watchPosition ticks cannot
  drift the pin between reading `#pin-pos` and saving — the equality check
  against `#light-pos` is deterministic.
- Go tests each spawn a fresh server (fresh `t.TempDir()` DB) — no
  cross-test state, per-server token consumption is correct.
- Fixture generator: stdlib-only, deterministic, self-verifying, ≤15 MB cap
  enforced, APP1/TIFF layout consistent with all three independent parsers
  (`parseExifAPP1`, `alHasExifGps`, images.ts `tiffHasGpsIfd`), CRC-correct
  bomb, 16-byte ftyp/heic box matching T-206.md:71's sniff.

## Findings (residual, none blocking)

1. **[Low] ~1e-4-probability duplicate collision for offset unnumbered
   creates.** README ("Rerun safety") proves distinct `jittered` KEYS land
   ≥8.2 m apart, but @A-203 saves the pin position at a continuous offset
   (drag ≤±25 m + 3×1 m arrows) from its base cell, which can land within
   8 m of a NEIGHBOURING grid cell if that cell holds one of the ≤11 other
   live jitter lights (P ≈ 1e-4 per run; requires both a specific occupied
   neighbour and the offset landing in the <8 m window). Same class as the
   acknowledged cross-run seed risk; not worth restructuring a frozen oracle
   over, but T-207 should know a rare 409-instead-of-Saved failure in @A-203
   is a coordinate-collision artifact, rerun-safe by construction.
2. **[Low] Reconciliation 4 (client-side bbox alert on every position
   update) remains an addition beyond T-205.md:84-101's walk.js behaviour
   list.** Satisfiable, documented in README reconciliation 4, and T-205's
   packet directs the worker to the drafts as the primary oracle
   (T-205.md:21-24) — but it is still pending planner ratification (handoff
   note 1, sixth item). If unratified, T-207 hits an avoidable dispute.
3. **[Low] `deleteAllByLabel` over-matches label prefixes** — sweeping
   'Dup panel lamp' also matches 'Dup panel lamp twin' rows. Harmless (both
   are swept anyway; nothing asserts exact-match semantics on cleanup), and
   the Go twin `alDetailURL` is only used with prefix-unique labels on fresh
   DBs. Noted so nobody "fixes" it into an under-match later.
4. **[Low] README says the pagination lane sits "≈41 m clear of the band";
   the actual gap is 0.0006° ≈ 49 m** (band edge −71.0501 vs lane
   −71.0495). The claim is conservative in the safe direction; no impact.
5. **[Low] `harness/tasks/graph.json` status flip (`review`→`merging`) is
   outside T-200's owns.** The handoff attributes it to the scheduler after
   the attempt-3 PASS reviews; content is a pure lifecycle transition (no
   budget/attempts/claims drift in the hunk). Flagging for the guard's
   audit trail only.
6. **[Low] A-207 has no e2e coverage** (Go-only: TestA207NoStrayFiles + the
   413/415 matrix inside TestA206PhotoPipeline). This matches the packet's
   own Deliverable-4 test list (tests 1–11 include no @A-207 e2e), so it is
   packet-conformant; recorded for the judge's coverage map.
7. **[Info] Unverifiable in this session**: fixture sha256 table (generator
   self-verifies every run and the handoff records byte-identical
   regeneration in both repos), the holdout drafts (external repo, commit
   `f523f06` per handoff), and all handoff exit codes. The manifest's six
   new entries are additions only, sorted, with the nine pre-existing
   hashes untouched; `make protect` output trusted per handoff (guard
   re-verifies at merge).

## Security / architecture / test-quality pass

- **Security**: no app code touched — no `template.HTML`, no SQL, no path
  handling, no new dependencies (`allow_deps: []` respected; generator and
  utils are stdlib/`@playwright/test`/`@axe-core/playwright` only, the
  latter two already in use). Upload abuse is frozen thoroughly: 413 over
  15 MB with a runtime-built body, 415 HEIC/text/bomb (sniffing, never
  filenames), >40 MP guard, no-stray-files on every reject, whole-APP1
  strip (no EXIF/GPS/maker notes), long-edge ≤1600. CSRF `csrf_token` is
  required on every admin POST in both suites. The only literals are the
  dummy `.env.test` secrets already public in `playwright.config.ts:60` and
  `scripts/*.sh`. The session cache file is written 0600 next to a
  gitignored test DB and holds only a localhost test-session cookie, with
  the write-failure path surfaced on stderr (admin.ts `writeCache`) rather
  than silently degrading. A `/photos/` traversal probe is absent from the
  drafts but is T-206's unit-test territory per its packet (T-206.md:76-78,
  94) — not required here.
- **Architecture**: ownership respected — only `acceptance/**`,
  `testdata/photos/**`, `testdata/schema.expected.sql`,
  `harness/protected.sha256`, the handoff (plus the scheduler's graph flip,
  finding 5). HTMX never swaps `#map` or ancestors: the drafts freeze
  `#rows` as the pagination swap root, `#dup-panel`/form targets for the
  walk flow, and the walk page uses a pin pad — @A-209 additionally proves
  no full navigation occurs. Parse-don't-validate and typed IDs are N/A
  (no app code); no "Phase N" strings in the drafts or generator. The
  drafts enforce the architecture's own rules rather than bypassing them.
- **Tests actually exercise the behaviour**: every claim A-201..A-209 has
  black-box coverage (real spawned `bin/server` over HTTP/bytes; real
  browser on all four projects), including the negative halves that
  previously went unasserted (no `Save anyway` in the numbered panel,
  nothing saved while the panel is up, list absence after delete, blank
  token → 200 form). Assertions are behaviour-level (status codes, headers,
  cookie flags, byte scans, roles/accessible names), not implementation
  echoes. Red-against-main is documented per suite (7/7 Go, 11/11 e2e) at
  the frozen first assertions, consistent with the no-/admin-routes state
  of main.

## Verdict rationale

This attempt closes every blocker, high and medium from the three prior
reviews, plus all the low findings, and the remaining deltas are honestly
documented (reconciliation list, ratchet nuance, node_modules artifact,
pending human evidence). The e2e oracle is now rerun-safe and satisfiable
under the real topology (one webServer, one persistent DB, four projects,
e2e→a11y double invocation, one-time global token), the Go drafts match the
worker packets line-for-line where they overlap, the schema oracle is
byte-faithful to the packet's frozen block with the probe statement copied
verbatim from migrations/0001_init.sql:2-5, and the inertness mechanism
holds (nothing compiles or loads the drafts; ratchet unchanged). The six
residual findings are low-severity, mostly documentation-precision items,
none of which can produce a false green or an unsatisfiable oracle. The
frozen bytes are ready for T-207's pure-rename activation.

VERDICT: PASS
