# T-200 — reviewer-alt (test integrity / reward hacking), attempt 3

Review basis: the attached diff (`harness/reviews/T-200.diff`, also the file
under review), the operative packet `harness/tasks/T-200.md` (including the
attempt-1/2 reviews appended to it), the downstream packets
`T-201/T-203/T-204/T-205/T-206/T-207`, `acceptance/contract.json`,
`playwright.config.ts`, `Makefile`, `.gitignore`,
`cmd/harness/{guard,ratchet}.go`, `harness/.ratchet.json`,
`harness/protected.sha256`, `internal/web/csrf.go`,
`migrations/0001_init.sql`, the checked-in `acceptance/` suite and the
installed Playwright types.

Verification limits: the diff is **not applied** in this checkout (there is no
`acceptance/drafts/`, `testdata/photos/*` is empty and
`testdata/schema.expected.sql` is absent), shell execution is denied in this
session, and the holdout repo is outside the sandbox. Every finding below is a
static check of the diffed bytes against the contracts and repo facts I could
read. The handoff's executed red/green results and the holdout commits are
taken on trust; the line references are to the activated draft paths (the
files as T-207 will `git mv` them).

## Attempt-2 findings: verified fixed in the diff

I re-checked all twelve attempt-2 alt findings against the bytes:

1. **@A-205 missing `Confirm pin`** — the first create now clicks
   `Confirm pin` and waits for `Saved`
   (`acceptance/drafts/admin-walk.spec.ts.txt:372-374`), then re-submits the
   same asset to raise the panel (`:397-404`). The whole numbered →
   `Merge` → unnumbered → `Save anyway` flow is present (`:372-444`).
2. **Detail-link discovery assumed one link per row** — the e2e helper
   dedupes with a `Set` (`admin-walk.spec.ts.txt:136-150`) and the Go helper
   with a `seen` map (`acceptance/drafts/admin_lights_go_test.go.txt:109-124`);
   the README freezes that a row may repeat its detail href
   (`acceptance/drafts/README.md:67`).
3. **Token replay only from the same client** — the replay now comes from
   `bare`, a second cookie-less client
   (`acceptance/drafts/admin_auth_go_test.go.txt:72,131-134`).
4. **Asset numbers not unique per test** — `uniqAsset` appends a per-test tag
   (`admin-walk.spec.ts.txt:98-104`) and every numbered create uses one
   (`:218`, `:372`, `:462`, `:511`); the values remain within
   `^[A-Za-z0-9-]{1,32}$` and keep the `GP-017`/`GP-018` prefixes.
5. **Low-accuracy test only proved button state** — it now completes a real
   save at 80 m after the save-anyway choice
   (`admin-walk.spec.ts.txt:237-263`).
6. **Preview assertion could pass on a CSS-stretched portrait** — it now
   asserts `naturalWidth > naturalHeight`
   (`admin-walk.spec.ts.txt:494-500`), matching the README's intrinsic-dims
   clause (`acceptance/drafts/README.md:68`).
7. **Numbered panel negatives unasserted** — `Save anyway` is asserted absent
   inside `#dup-panel` and the `Cancel` link present
   (`admin-walk.spec.ts.txt:389-390`).
8. **e2e purity checked only GPS** — `hasAPP1` was added and both
   `hasAPP1 === false` and `hasExifGps === false` are asserted
   (`admin-walk.spec.ts.txt:479-481`; `acceptance/e2e/util/images.ts:22-36`).
9. **Keyboard-movable pin never exercised** — the test focuses `#pin`,
   presses `ArrowRight` three times, polls `#pin-pos` and saves the
   keyboard-moved position (`admin-walk.spec.ts.txt:319-342`).
10. **Handoff overstated what was proven** — the handoff now separates the
    executed red-against-main run from the satisfiability walk
    (`harness/handoffs/T-200.md`, "Would-fail reasoning — EXECUTED" vs
    "SATISFIABILITY").
11. **`admin.ts` cache-write fallback comment wrong** — the comment now states
    the one-time-global consequence and `writeCache` surfaces the failure on
    stderr (`acceptance/e2e/util/admin.ts:170-187`).
12. **`TestA209` did not assert the `#rows` fragment root** —
    `alReRowsRoot` is asserted on the page-2 fragment
    (`admin_lights_go_test.go.txt:85,505-507`).

The rerun-safety rewrite also holds up: `maxRedirects: 0` is on every
`page.request.post` (`admin-walk.spec.ts.txt:153-159,555-568`), the logins use
one session (`admin.ts:56-98`), and the cleanup sweeps by label
(`admin-walk.spec.ts.txt:166-178`). No new test is skipped, disabled or
narrowed, and no production code is touched.

## Findings (none makes the oracle deterministically unsatisfiable)

### 1. [Medium — flaky wait] `addLightFromForm` never waits for the geolocation fix

`addLightFromForm` opens `/admin/walk`, clicks `Add light here` and fills the
form (`admin-walk.spec.ts.txt:182-196`) without waiting for `#walk-pos` or
`#walk-accuracy`. Test 1 shows the intended wait
(`admin-walk.spec.ts.txt:209-210`), but every caller of the helper does not:

- @A-208 creates its light this way and then asserts the detail's
  `#light-pos` contains the default `42.7617`/`-71.0303`
  (`admin-walk.spec.ts.txt:511-513,526-527`). If the first `watchPosition`
  callback has not populated the hidden lat/lon before `Confirm pin`, the
  POST carries empty/default coordinates and the create fails at the `Saved`
  assertion.
- @A-205's unnumbered half sets `context.setGeolocation` to `near` and
  `near + 0.00004` and relies on the form submitting those coordinates
  (`admin-walk.spec.ts.txt:410-426`). If the second submission reads a stale
  position, the light is not within the 8 m radius and the expected
  `#dup-panel` never appears.
- @A-206's photo create uses the same helper (`:462-468`).

This is a race, not a contract gap: adding
`await expect(page.locator('#walk-accuracy')).toHaveText(/accuracy \d+ m/)`
(or a `#walk-pos` wait) before filling would make it deterministic. T-207
rule 3 permits a mechanical flaky-wait fix, but the frozen oracle should not
ship with this race; the handoff's satisfiability walk does not mention it.

### 2. [Medium — flaky/oracle gap] The preview `naturalWidth` read happens before the image load

@A-206 sets the file, asserts `#photo-preview` is visible and immediately reads
`naturalWidth`/`naturalHeight` (`admin-walk.spec.ts.txt:494-500`). An `<img>`
can be visible via its CSS box before the object URL/blob has loaded; the
read then returns `0/0` and a compliant, correctly oriented preview fails on
a correct screenshot. The test needs an `expect.poll`/wait for
`naturalWidth > 0` before comparing the dimensions.

Secondarily, the README now freezes intrinsic dimensions as the proof
(`acceptance/drafts/README.md:68`) but the WebKit project is part of the
matrix. If a worker previews the raw file and WebKit reports the *unrotated*
intrinsic size, the test fails on `iphone-16` while the rendered preview is
correct. The worker can avoid this by previewing a processed image, but the
contract does not say so. At minimum the wait above should be added; the
WebKit assumption should be validated at activation (T-207) or the preview
contract should state "processed or browser-oriented intrinsic dimensions".

### 3. [Medium-High — contract gap vs T-205] @A-204 expects a client-side bbox alert on geolocation change

The e2e test only changes the context geolocation outside the bbox and then
expects `role=alert` containing `outside Groveland town bounds`
(`admin-walk.spec.ts.txt:347-354`); it never submits the form. That requires
the walk page to display the bbox alert on a position change.

T-205's packet only freezes the **server-side** 422 on POST
(`harness/tasks/T-205.md:60-62`); its walk.js behaviour list has no bbox check
(`harness/tasks/T-205.md:89-101`). A worker that implements exactly the packet
(server-side validation only) fails @A-204, and the README's visual note says
only "near the position readout; the form does NOT advance"
(`acceptance/drafts/README.md:182-183`). The draft's test 5 wording in
T-200.md:244 does require the alert on `setGeolocation`, so this is satisfiable
by a worker who reads the draft, but it is an unreconciled contract gap like
the three the README already lists. Add it to the README reconciliation table
and the handoff's planner-ratify list (or make the e2e submit the form and
assert the server 422). I do not block on it because the draft is the declared
oracle and T-205 is instructed to read it, but it is the most likely
activation-time surprise.

### 4. [Low-Medium — frozen-contract contradiction] README says empty token → 403; T-203 says no/blank token → 200

`acceptance/drafts/README.md:82` freezes `wrong/empty token → 403` for
`GET /admin/login`, but T-203's packet freezes `GET /admin/login`
(no/blank token) → 200 (`harness/tasks/T-203.md:53-60`). The Go draft tests
only no token → 200 and a wrong token → 403
(`acceptance/drafts/admin_auth_go_test.go.txt:68-...`); the empty-token case is
unasserted, so a worker following the README can implement 403 for `?token=`
and still pass every drafted test, while a T-203-packet-compliant build returns
200. Either reconcile the wording to T-203 or add the empty-token assertion so
the oracle is unambiguous.

### 5. [Low — assertion weaker than the README] The Go photo test does not assert the exact 800×600 output

`TestA206PhotoPipeline` asserts only `w > h` and long edge ≤ 1600
(`acceptance/drafts/admin_lights_go_test.go.txt:285-294`), while the fixture
table freezes "processed output must be 800×600"
(`acceptance/drafts/README.md:141`) and T-202's packet tests exact dims
(`harness/tasks/T-202.md:83-86`). A pipeline that upscales the 600×800
Orientation=6 fixture to 1600×1200 would satisfy the acceptance draft while
violating the fixture table and T-202's "never upscales" rule. Add
`w == 800 && h == 600` for this fixture (the failure text already says
800×600).

### 6. [Low — helper parser gap] `images.ts` `jpegSize` still lacks the Go twin's SOF length guard

`jpegSize` checks `i + 9 > bytes.length` but not `next < i + 9` after the
segment length is read (`acceptance/e2e/util/images.ts:95-100`), whereas
`alJpegDims` checks `next < i+9` (`admin_lights_go_test.go.txt:791-821`). A
malformed SOF with a declared length shorter than the minimal SOF can read the
following bytes as width/height. Not reachable against a compliant re-encode,
but the handoff's "mirrors the Go twin" claim is not exact and the helper is
the byte-level proof for the e2e claim.

### 7. [Low — unasserted frozen cookie attribute] `Path=/` is not checked

The README freezes `Path=/` on `gl_session`
(`acceptance/drafts/README.md:88`), but the Go auth draft only checks
`HttpOnly` and `SameSite=Lax`
(`acceptance/drafts/admin_auth_go_test.go.txt:116-124`). The T-203 packet does
not require Path, so this is not a feature blocker; it is a one-line
assertion if the README is the oracle.

### 8. [Low — rerun safety across invocations] @A-209's pre-clean matches only the current RUN label

`@A-209`'s pre-clean is `deleteAllByLabel(page, baseLabel)` with
`baseLabel = "Scroll lamp ${RUN}-${code}"`
(`admin-walk.spec.ts.txt:538-540`). Seeds from a previous invocation carry a
different `RUN`, so the pre-clean does not remove them. They ride the same
fixed per-project longitude lane; the run-jittered base lat is on a
`0.0001°` lattice (~11.1 m buckets), so when two runs draw the same bucket
(~1/300 per project per run) every current seed collides with a leftover
seed at the same coordinate and the seed POST returns 409 instead of 303.
The handoff's "≈0.07% point-vs-point" count does not include this
grid-vs-grid repeat, and the README's "can never fire across ... runs"
(`acceptance/drafts/README.md:116-121`) is stronger than the code. Sweep a
stable prefix (e.g. `Scroll lamp `) or all `Scroll lamp ${code}` labels to
make the pagination test rerun-safe after a failed run.

### 9. [Low — whitespace-intolerant heading assertion] `>Lights<` can fail a compliant template

`TestA209Pagination` checks `strings.Contains(list, ">Lights<")`
(`admin_lights_go_test.go.txt:477`), while the e2e uses
`getByRole('heading', { name: 'Lights', exact: true })`
(`admin-walk.spec.ts.txt:570`). A compliant template rendering
`<h1>\n  Lights\n</h1>` fails the Go check; the auth regexes already tolerate
`\s*` elsewhere. This is a T-207 mechanical fix if it bites, but it is a
narrower oracle than the frozen heading contract.

### 10. [Info — accepted residual] Jittered point-vs-point collisions

The handoff already discloses the ~1/90 000-per-pair jitter birthday for the
unnumbered creates. It fails loudly (unexpected dup panel) and the pre-clean
covers retries; I agree it is acceptable for an acceptance oracle. The
pagination-lane repeat in finding 8 is the one that is not disclosed.

## Checked and not found

- **No weakened, deleted, skipped or narrowed test.** All changes are new
  drafts/fixtures/utils. The attempt-3 edits strengthen the @A-205, @A-206,
  @A-203, @A-201 and @A-209 assertions; nothing was removed. No
  `t.Skip`/`test.skip`/`test.only`/`.fixme` appears in the diff.
- **No mocks bypassing the behaviour.** The Go drafts spawn the real
  `bin/server` with a fresh `t.TempDir()` DB and use real HTTP clients with
  `ErrUseLastResponse`; the e2e drives a real browser. `loginAsAdmin` injects
  the real session cookie obtained from the real one-time exchange; it does
  not mock the server. The A-201 exchange/replay/cookie flags are asserted by
  the Go draft against a fresh server.
- **No fixture-value special-casing in production code.** No app code is
  touched. The fixture names/labels in `testdata/photos/gen/main.go` are
  test-data generation, and the `GP-017`/`GP-018` literals are confined to
  the test drafts and their fixture generator, not production branches.
- **No coverage theatre.** The positive and negative halves are both present:
  the numbered panel asserts `Save anyway` absent, the unnumbered panel
  asserts it present; the low-accuracy test completes a real save; the
  duplicate tests assert "nothing saved" between 409 and the explicit choice;
  the photo test asserts APP1/GPS absence and orientation, not just a 200.
- **Schema oracle and fixtures.** `testdata/schema.expected.sql` matches the
  packet's frozen product schema and copies the probe `CREATE TABLE` from
  `migrations/0001_init.sql:2-5`; the generator is stdlib-only,
  deterministic and self-verifying (APP1/TIFF layout, CRC-correct pixel
  bomb, HEIC sniff box, 15 MB cap).
- **Ownership/manifest.** Only owned paths are touched (`acceptance/**`,
  `harness/protected.sha256`, `testdata/photos/**`,
  `testdata/schema.expected.sql`, the handoff). The manifest adds exactly the
  6 new acceptance files and leaves the pre-existing hashes unchanged; the
  ratchet `{"go":51,"pw":32}` is correct for inert drafts.

## Verdict rationale

The attempt-2 blockers are fixed: the e2e oracle is self-consistent and
rerun-safe under the real Playwright topology, the one-time token is consumed
once and reused through a gitignored cache, the 303s are observable, the
duplicate/merge/save-anyway flows are complete, and the photo and keyboard
assertions are stronger than before. The remaining findings are robustness and
contract-clarity items: two waits (`addLightFromForm`, preview image load), one
unreconciled client-side bbox gap against T-205, one README empty-token
contradiction, and several low-strength assertions. None of them is a
deterministic unsatisfiable oracle against a build that follows the frozen
draft, and the two waits are exactly the mechanical `expect.poll`/wait class
T-207 rule 3 permits. I would expect the planner to ratify findings 3 and 4
and T-207 to apply findings 1-2 and 5-9 without changing what the tests assert.

VERDICT: PASS
