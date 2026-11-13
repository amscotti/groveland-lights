# T-200 review (attempt 2) — M2 acceptance drafts, fixtures, schema oracle, frozen UI contract

Review basis: the attached diff (`harness/reviews/T-200.diff`), the packet
`harness/tasks/T-200.md` (including both attempt-1 reviews), the downstream
packets T-201/T-203/T-204/T-205/T-206/T-207, and the repo's actual test
topology: `playwright.config.ts`, `Makefile`, `scripts/acceptance-go.sh`,
`scripts/holdout-run.sh`, `.gitignore`, `migrations/0001_init.sql`,
`internal/web/csrf.go`, `cmd/harness/{guard,ratchet,status}.go`,
`harness/.ratchet.json`, `harness/protected.sha256`, `acceptance/go/*`,
`acceptance/e2e/*`, `tsconfig.json`, `package.json`,
`docs/spec/architecture.md`.

Verification limits: shell execution was denied in this session and the diff is
not applied to this checkout (main has no `acceptance/drafts/`,
`testdata/photos/*` is absent, `testdata/schema.expected.sql` absent), so
dynamic re-runs and hash recomputation were not possible. Every finding below
is a static check of the diffed bytes against repo facts I could read; the
handoff's executed verifications (fixture hashes, red runs, suite greens) are
cross-checked for internal consistency with those facts, not re-executed.

## Attempt-1 blockers: verified fixed

1. **One-time GLOBAL token vs ~40 logins (alt-1)** — fixed by the session
   economy in `acceptance/e2e/util/admin.ts`. Verified against the real
   topology: `playwright.config.ts:13-14` (`fullyParallel: false`,
   `workers: 1`) means ONE worker process per Playwright invocation, so the
   in-process memo + `inFlight` dedup (admin.ts:~154-204) correctly yields one
   exchange per invocation; `make check` runs the second invocation
   (`a11y`, Makefile:120,136) against the SAME `./test-run.db`
   (playwright.config.ts:56), and the disk cache `test-run.db.session.json`
   (admin.ts:~39) bridges it — the pattern `test-run.db*` is already in
   `.gitignore:20`, so the cache lives and dies with its DB. The
   validation-before-adoption GET (admin.ts:~210-224, `maxRedirects: 0`)
   detects a dead session without consuming anything. The consumed-token +
   expired-session terminal case fails loudly with the `rm -f test-run.db*`
   operator fix, documented in `acceptance/drafts/README.md` ("Rerun safety")
   and the handoff. The A-201 exchange/replay/cookie-flag semantics themselves
   are asserted by the Go draft against fresh per-test servers
   (`aaSpawnDir`, fresh `t.TempDir()` DB per test), which is the right place.
2. **Shared accumulating DB (rev Blocker 2 / alt 2)** — fixed: per-run +
   per-project asset numbers keeping the packet prefixes
   (`GP-017-<run>-<proj>`, ≤32 chars, still `^[A-Za-z0-9-]{1,32}$`, still
   matched by `img[alt^="GP-017"]`); per-run jittered coordinates (spec
   `jittered()`, lat 42.7200..42.7499 / lon −71.0800..−71.0501 — inside
   TOWN_BBOX `42.70,-71.09,42.82,-70.96` with ~2 km margin) for every
   unnumbered create and the pagination seed column (seeds spaced 0.0005° ≈
   55 m ≫ the 8 m radius); per-test pre/post cleanup via soft delete with
   label-substring sweeps (`deleteAllByLabel`) that converge across pages.
   The distinct-bucket minimum separation on the jitter grid is ≈8.2 m (lon
   axis at lat 42.73) > 8 m, so distinct jitter points cannot trigger the
   unnumbered radius except on a full hash collision (P ≈ 1/90 000 per pair),
   which the handoff discloses and which fails loudly (dup panel), never
   silently. With `workers: 1` and CI `retries: 1`, a retry re-runs in the
   same process with the same `RUN` and the pre-clean makes it deterministic.
3. **Redirect-following hiding the 303 (alt 3)** — fixed: `maxRedirects: 0`
   pinned on every `page.request.post` (seeds, deletes) and in the login
   helper's probes.
4. **A-206 not self-contained (alt 4)** — fixed: the test creates its own
   photo-bearing light through the UI before asserting bytes/preview.
5. **A-205 unsatisfiable first-create (rev Blocker 1)** — fixed: each test
   creates only its own uniquely-numbered light; the panel is triggered by
   re-submitting that asset.
6. **`Older` link-only (alt 5)** — fixed: `getByRole('link').or(getByRole('button'))`.
7. **Unasserted clauses (alt 6)** — fixed: keeps-values on label-only edit
   asserted in both Go (`TestA208EditSoftDelete`: asset no + coords survive)
   and e2e; "nothing saved until chosen" asserted after each 409 in both.
8. **Post-drag race (alt 7)** — fixed: `expect.poll` on `#pin-pos`.
9. **HIGH 1-3 contract conflicts** — reconciled to the worker packets and
   frozen in `acceptance/drafts/README.md` "Contract reconciliations":
   create success 303 `/admin` + HX `Saved` fragment = `harness/tasks/T-205.md:69`;
   numbered panel `Merge`+cancel / unnumbered-only `Save anyway` / numbered+save
   → 422 = `T-205.md:65-68` (now also asserted by the Go draft); kind
   `park|building` form = `T-205.md:58`. Detail URLs discovered from list rows,
   never Location — consistent with the packet's own wording
   ("from the response/list page"). Duplicate radii match `T-204.md:36-39,75`
   ("same asset_no anywhere"), HEIC wording matches `T-206.md:68-70`
   ("please choose JPEG" is a substring of the frozen message).
10. **MEDIUM 1 (pin-pos precision)** — frozen at exactly 6 dp with the
    explicit clause "#pin-pos text equals #light-pos for the same position";
    `T-205.md:55` freezes `#light-pos` at 6 dp, so the equality check is
    unambiguous.
11. **MINORs** — `jpegSize` truncated-SOF guard added (mirrors `alJpegDims`'s
    `next < i+9`); gen `panic`→error returns; handoff manifest count now
    precise; would-fail section now includes the satisfiability analysis.

## Verified correct (new in attempt 2)

- **Schema oracle** `testdata/schema.expected.sql`: probe verbatim from
  `migrations/0001_init.sql:2-5` (comment/INSERT excluded); the product block
  matches the packet's frozen SQL (`T-200.md:86-169`) line-for-line — all 11
  tables + 5 indexes, `lights_asset_no` partial unique exactly as T-201's
  packet requires, `kind`/`status`/`status_source` CHECKs identical.
- **Drafts are inert**: `.go.txt`/`.spec.ts.txt` are picked up by no runner
  (`go vet ./...` and Playwright's testMatch ignore them); the manifest
  ratchet baseline `harness/.ratchet.json` = `{"go":51,"pw":32}` — unchanged,
  correct for inert drafts.
- **Manifest**: `harness/protected.sha256` grows 9 → 15 entries; all 9
  pre-existing hashes untouched; new entries are exactly the 6 new files under
  `acceptance/**` — precisely what `GenerateManifest` produces
  (`cmd/harness/ratchet.go:76-78` walks `acceptance/**`), sorted, and the edit
  is sanctioned for a type=acceptance task (`cmd/harness/guard.go:127,
  243-245`). `.ratchet.json` changes are likewise allowed (`guard.go:140-144`).
- **Activation procedure** in `acceptance/drafts/README.md` matches
  `T-207.md:29-34,49-52` move-for-move, including preserving the contract
  table into `acceptance/README.md` and deleting the drafts dir.
- **Go drafts**: `package acceptance` + build tag match
  `scripts/acceptance-go.sh:36`; helper prefixes `aa*`/`al*` collide with
  nothing in `acceptance/go/` (existing: `probe*`, `mig*`, `m1*`, `syncBuf`,
  `envOr`); spawn env mirrors the public dummy env; clients use
  `ErrUseLastResponse` so every 302/303 stays observable; each test gets a
  fresh server+DB (no cross-test state). `alDetailURL` relies only on the
  frozen row-link contract.
- **CSRF reuse**: `internal/web/csrf.go` is stateless double-submit (cookie +
  matching field, no server state, `csrf.go:44-78`), so reusing one parsed
  token for the 55 seed posts and all cleanup deletes is valid, and
  `csrfToken(page)` via `page.request` lands the cookie in the shared context
  jar.
- **ACTIVE utils**: `acceptance/e2e/util/{admin,images}.ts` are inside
  `tsconfig.json:15` include → covered by `make js-check`; import style
  `./util/admin.js` matches `home_m1.spec.ts:16`; `@axe-core/playwright`
  already in `package.json:9` (no new dep; `allow_deps: []` respected); axe
  tags match the packet's test 11; no tests of their own (per Deliverable 5).
- **Login economy vs the holdout**: `scripts/holdout-run.sh:9,16` gives each
  holdout invocation a fresh temp DB and one server, matching the handoff's
  claim that a module-level memo suffices there (no disk cache needed).
- **Rules**: no `t.Skip`/`test.skip`/`test.only`/`.fixme`; no TODO/FIXME/
  `panic`/`console.log` in the new files; no "Phase N" strings anywhere in the
  diff (and phase-grep only covers cmd/server, internal, web, migrations);
  fixture generator is stdlib-only and self-verifying with a 15 MB commit cap;
  `testdata/photos/gen/main.go` writes the exact fixture set of Deliverable 1
  (hand-built APP1/TIFF layout is internally consistent — IFD0@8, GPS IFD@38,
  56-byte TIFF, correct segment length — and re-parsed by `parseExifAPP1`).
- **Ownership**: only owned paths touched (`acceptance/**`,
  `harness/protected.sha256`, `testdata/photos/**`,
  `testdata/schema.expected.sql`, the handoff). No Makefile/CI/app-code/
  golden-file edits. Scratch staging stays under gitignored
  `testdata/rendered/*` (`.gitignore:17`).

## Findings (all minor; none blocking)

1. **MINOR — whitespace-intolerant heading check in the Go draft.**
   `acceptance/drafts/admin_lights_go_test.go.txt`, `TestA209Pagination`:
   `strings.Contains(list, ">Lights<")` (diff line 1561). This matches any
   element whose text node is exactly `Lights` on one line, but a compliant
   template rendering `<h1>\n  Lights\n</h1>` fails it while the e2e twin
   (`getByRole('heading', { name: 'Lights', exact: true })`) passes. The auth
   draft's regexes tolerate `\s*` (`aaReHeading`); this one doesn't. If it
   bites at activation it is a T-207 rule-3 mechanical fix (assertion
   unchanged); noting it now so it isn't mistaken for a feature gap.
2. **MINOR — visual-note wording tension: "map canvas" vs T-205's pin pad.**
   `acceptance/drafts/README.md` A-202 note says "the middle is the map
   canvas" / A-203 "centred on the map", while `T-205.md:93-97` specifies a
   "pin pad ... over a pad with a metre scale" (not MapLibre). The frozen
   selectors (`#pin`, `#pin-pos`, `aria-label="position pin"`) are consistent
   in both; only the prose for the visual reviewer differs. Suggest the
   planner/visual-reviewer read "map canvas" as the pad; no code impact.
3. **MINOR — origin binding of the session cache.** `admin.ts:~37,86`: a
   cached session whose `origin` differs from the current `ORIGIN` (e.g.
   `TEST_PORT`/`BASE_URL` changed between invocations against the same DB) is
   discarded without validation and a fresh exchange is attempted, which 403s
   if the token was consumed. Contrived for the gate (both `make check`
   invocations use the default port); recovery is the documented
   `rm -f test-run.db*`. Note only.
4. **Accepted residual non-determinism (disclosed).** The jitter hash can
   theoretically collide for two unnumbered-create keys in one run
   (~1/90 000 per pair), surfacing as an unexpected duplicate panel — a loud,
   rerun-recoverable failure, not a silent pass. The handoff states this; fine
   for an acceptance oracle.
5. **Note — session cookie cached on disk.** `test-run.db.session.json` holds
   a live admin session cookie. It is test-only (dummy secrets, local server),
   written 0600, and covered by the `test-run.db*` gitignore — acceptable test
   infrastructure, listed here for completeness.

## Security / architecture / test-quality pass

- **Security**: no app code in the diff, so no new XSS/SQL/path-traversal
  surface. The only literals are the dummy `.env.test` secrets already public
  in `scripts/acceptance-go.sh:20-22`, `scripts/holdout-run.sh:17-19` and
  `playwright.config.ts:58-60` (base64 of `test-*-32-byte` strings) — no real
  secret in output or logs; the drafts never print the token. Upload limits
  are probed hard: 413 with a runtime-built 16 MB body (nothing >15 MB
  committed), 415 for HEIC/text/pixel-bomb, no-stray-files before/after every
  reject, APP1/GPS strip + long-edge ≤1600 + orientation-applied assertions
  on served bytes. All admin POSTs carry `csrf_token`, compatible with the
  existing double-submit implementation. A `/photos/` traversal probe is absent
  but T-206's unit tests own it (packet scope, as in attempt 1).
- **Architecture**: ownership map respected — every touched path is in the
  packet's `owns` (guard concedes `acceptance/**` + `harness/protected.sha256`
  + `harness/.ratchet.json` to acceptance tasks); no `internal/`/`cmd/`/
  `web/`/`migrations/` edits. HTMX/#map rule: the drafts never reference
  `#map`; pagination swaps `#rows`; the walk page's interactions are the pin
  pad and form targets. Parse-don't-validate/typed IDs are feature-code rules
  and are correctly deferred to T-205's packet. Migrations untouched;
  `status.Derive` untouched.
- **Test quality**: the deliverable is the oracle itself, inert by explicit
  packet design (the merge gate would otherwise force red specs to merge
  red); inertness is real (extensions + unchanged ratchet), activation is a
  pure rename, and git history preserves the oracle-first property. Every
  claim A-201..A-209 has both a Go black-box draft (real spawned binary, real
  HTTP, byte-level photo inspection) and a browser draft on all four
  projects; assertions are exact-status/exact-string where the contract
  freezes them and role/accessible-name where it doesn't. The attempt-2
  rewrite genuinely exercises the previously unasserted clauses instead of
  deleting them.

## Verdict rationale

Attempt 1 was blocked because the frozen e2e oracle provably could not go
green against a contract-compliant M2 build. Attempt 2 rewrites the oracle so
that it can: the login economy is sound under the actual topology (verified:
`workers: 1`, one webServer DB per run, `make check`'s second invocation
bridged by a gitignored cache), the data plan is rerun-safe under the frozen
duplicate rules (verified against `T-204.md` radii and the partial unique
index in the schema oracle), every 303 is observable, every test is
self-contained for `make e2e-task`, and the three contract conflicts are
reconciled to the worker packets verbatim with the reconciliation frozen in
the README for T-205. The schema oracle matches the packet byte-for-byte and
0001's probe verbatim. Ownership, manifest, and ratchet changes are exactly
the sanctioned outputs. Remaining findings are minor and none of them makes
the oracle unsatisfiable or weakens a frozen assertion.

VERDICT: PASS
