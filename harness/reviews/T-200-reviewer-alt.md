# T-200 — reviewer-alt (test integrity / reward hacking), attempt 5

Review basis: the attached diff (`harness/reviews/T-200.diff`), the operative
packet `harness/tasks/T-200.md` (including the appended attempt-1/2 reviews),
the downstream packets `T-201..T-207`, `acceptance/contract.json`,
`playwright.config.ts`, `Makefile`, `.gitleaks.toml`, `.gitignore`,
`cmd/harness/{guard,ratchet}.go`, `harness/policy.json`,
`harness/protected.sha256`, and `migrations/0001_init.sql`.

Verification limits: the diff is **not applied** in this checkout (there is no
`acceptance/drafts/`, `testdata/photos/*` is absent and
`testdata/schema.expected.sql` is absent), and shell execution is denied in
this session. The holdout repo is outside the sandbox. Every finding below is
therefore a static check of the diffed bytes against the contracts I can read;
the handoff's executed red/green results, manifest hashes and holdout commit are
taken on trust. The gate re-verifies the manifest, and the fixture generator is
self-verifying, so the static scope is sufficient to decide the integrity
question.

## Findings

### 1. [Low — assertion weaker than the stated gate] `@A-202` low-accuracy test proves the button attribute, not a blocked save

`acceptance/drafts/admin-walk.spec.ts.txt:269-270` asserts
`toBeDisabled()` and then the presence of the `disabled` attribute. That is
enough to reject an `aria-disabled`/CSS-only fake (the intended attempt-4 fix),
but it still never attempts a save while the gate is active. The test's own
comment (`:266-268`) claims "while it is disabled NOTHING can be saved", which
is stronger than what is exercised: an implementation that disables the visible
button but still submits the form on Enter (or accepts a direct POST at
accuracy 80) passes. The packet's test 2 only requires "`Add light here`
disabled until chosen" (`harness/tasks/T-200.md:237-238`), and the draft does
perform a real save **after** choosing save-anyway (`:275-282`), so this is a
coverage gap rather than an unsatisfiable or weakened contract. A stronger
draft would submit while gated (e.g. press Enter with the form filled) and
assert no `Saved`/no row, then choose save-anyway and save.

### 2. [Low — README clause unasserted] `@A-204` never opens or submits the form

`acceptance/drafts/admin-walk.spec.ts.txt:372-376` only waits for the
client-side `role=alert` after `setGeolocation` outside the bbox. The README
visual note also freezes "the form does NOT advance" (`acceptance/drafts/README.md:127-129`),
but the test never opens the add-light form and never checks that no form is
present or that nothing was saved. The packet test 5 only requires the alert
(`harness/tasks/T-200.md:244`), so this is a minor coverage gap, not a
contract failure. The server-side 422 is independently asserted in
`acceptance/drafts/admin_lights_go_test.go.txt:185-199`.

### 3. [Low — panel detail clauses unasserted] `@A-205` checks the panel chrome, not its content or the cancel escape

`acceptance/drafts/admin-walk.spec.ts.txt:402-410` asserts the `#dup-panel`
heading, `Merge`, the absence of `Save anyway` in the numbered case and the
presence of the `Cancel` link. It never checks that the panel shows the
existing light's label, asset number and distance (README visual note
`acceptance/drafts/README.md:118-125`), nor that `Cancel` closes the panel or
leaves the light untouched. The packet test 6 only freezes "`#dup-panel`
appears with `Merge` and `Save anyway`" (`harness/tasks/T-200.md:245-246`), so
this is coverage, not a broken oracle. The Go draft independently checks the
409 body contains `Possible duplicate`.

### 4. [Low — policy clause half-proven] `@A-205` merge proves the label but not the photo

The README freezes `Merge` as applying "the new submission's label/photo onto
the existing row" (`acceptance/drafts/README.md:68-69`), and T-205 says
`dup_choice==merge → apply photo/label onto ExistingID`
(`harness/tasks/T-205.md:67-68`). The e2e merge check
(`acceptance/drafts/admin-walk.spec.ts.txt:428-429`) and the Go merge check
(`acceptance/drafts/admin_lights_go_test.go.txt:213-218`) both create the
merge fixtures **without a photo**, so only the label half is proven. A merge
that re-labels the survivor but drops the new photo passes. The attempt-4
review asked for "the twin's label (or the photo)"; this attempt chose the
label, which is defensible, but the README's "label/photo" wording is now
stronger than the assertions. Either narrow the README or add a photo-bearing
merge check.

### 5. [Low — assertion weaker than the frozen format] `@A-208` checks substring coordinates

`acceptance/drafts/admin-walk.spec.ts.txt:568-569` asserts
`#light-pos` contains `42.7617` and `-71.0303` only. The README freezes
`#light-pos` as `lat, lon` at 6 decimals (`acceptance/drafts/README.md:83`),
and `@A-203` does assert the full 6 dp string against `#pin-pos`
(`acceptance/drafts/admin-walk.spec.ts.txt:351`), so the format is covered by
the sibling test. Still, the `@A-208` check would pass a detail page that
renders `142.7617` or a 2 dp position; use the same 6 dp string/assertion to
keep the claim local and self-contained.

### 6. [Low — narrowed selector] `Save changes` and `Delete light` are required to be `button`s

`acceptance/drafts/admin-walk.spec.ts.txt:562` and `:572` use
`getByRole('button', { name: 'Save changes' })` / `{ name: 'Delete light' }`.
The README's frozen button/label table only freezes the walk/duplicate buttons
(`acceptance/drafts/README.md:70-72`); the edit note only says "with `Save
changes`" and "`Delete light` is a visually destructive (red) action"
(`acceptance/drafts/README.md:133-136`). A compliant implementation that
renders these as links (or link-styled controls) would fail. The packet's test
8 uses backticked strings without a role (`harness/tasks/T-200.md:251-252`), so
the oracle should either freeze the roles in the README or use a
`link`-or-`button` locator as `Older` already does (`:630-632`).

### 7. [Low — preview tracking is only checked when a preview pre-exists] `@A-206`

`acceptance/drafts/admin-walk.spec.ts.txt:517-518` captures the pre-pick
`#photo-preview` source and `:539-541` only asserts a source change inside
`if (hadPreview)`. If the page inserts a **static landscape preview only when
a file is picked**, `hadPreview` is false, `expect(preview).toBeVisible()`
passes, and `naturalW > naturalH` passes without the selected portrait being
rendered. The README's wording ("must appear/change", `README.md:76`) is
satisfied literally by appearance, but the packet's intent is that the preview
renders the picked portrait with orientation applied. This is the same class as
the attempt-4 static-placeholder finding, now narrowed to the insert-on-pick
case; the Go test proves the served bytes, not the preview. Worth closing while
T-207 still cannot edit the oracle.

### 8. [Low — flake] `@A-203` captures `final` as soon as any arrow-key change lands

`acceptance/drafts/admin-walk.spec.ts.txt:344-351` polls `#pin-pos` until it
differs from the post-drag value, then uses the captured `final` as the expected
detail position (`:351`, `:612`). If the three `ArrowRight` presses update the
effective position through a debounce/animation frame, the poll can succeed on
the first nudge while the submitted form uses the later third nudge; the detail
will then legitimately differ from `final` and the test flakes. `keyboard.press`
awaits each event, so a synchronous handler makes this unlikely, but the frozen
oracle should wait for the third nudge (or read the position again immediately
before `Confirm pin`) rather than accept the first change.

### 9. [Info — navigation-event interpretation] `@A-209` treats any `framenavigated` as a full reload

`acceptance/drafts/admin-walk.spec.ts.txt:624-629` sets `navigated` on any
main-frame `framenavigated`; `:636` requires it false. The packet's test 9 says
"without a full reload (no navigation event)" (`harness/tasks/T-200.md:253-254`).
If a compliant HTMX pagination adds `hx-push-url` (a common pattern), the URL
changes via `history.pushState`; depending on the Playwright/Chromium version
this can surface as a `framenavigated` event even though there is no document
reload, failing the test. The frozen contract should say "no document
navigation" and/or assert on `page.on('load')`/document request instead. T-205
does not currently require push-url, so this is an activation-time risk rather
than a present blocker.

### 10. [Info — operational] `admin.ts` session cache is keyed to the origin string, not the DB

`acceptance/e2e/util/admin.ts:40-41` fixes the cache path to
`test-run.db.session.json`, and `currentSession()` rejects a cached session
whose `origin` differs (`:80-91`). The Makefile and `playwright.config.ts` use
the same `TEST_PORT` for the e2e and a11y invocations, so the normal path is
fine. If an operator changes `TEST_PORT` between the two invocations while the
same `./test-run.db` persists, the cached (still valid) session is rejected
because its origin string no longer matches, the token is already consumed, and
the helper fails; the README's `rm -f test-run.db*` fix is documented
(`acceptance/drafts/README.md:160-165`), but the origin check could instead
accept the same DB/port-independent path. This is an operational edge, not a
test-integrity failure.

### 11. [Info — scheduler-owned] `harness/tasks/graph.json` status flip

The diff changes T-200 from `review` to `merging`
(`harness/tasks/graph.json:275`). The handoff attributes this to the scheduler
and `checkProtectedPaths` exempts graph.json (`cmd/harness/guard.go:117-147`).
No test-integrity impact; noted only because the path is outside T-200's packet
`owns`.

## What I checked and found sound

- **No weakened/deleted/skipped/narrowed existing test.** The diff adds new
  draft files, fixtures and two active helpers; it does not edit any existing
  acceptance test. No `t.Skip`/`test.skip`/`test.only`/`.fixme`/`TODO`/`FIXME`
  appears in the diff (grep over the diff: no matches), and no `console.log` or
  `panic("not implemented")` is introduced.
- **No special-casing in production code.** No `internal/`, `web/`,
  `cmd/server/` or `migrations/` file is touched. The `GP-017`/`GP-018`/
  coordinate literals live in the test drafts and the fixture generator, not
  in handlers/templates/store. The fixture generator's filename/geometry
  literals are data generation, not a branch on fixture values.
- **No mocks bypassing the behaviour under test.** The Go drafts spawn the real
  `bin/server` with fresh `t.TempDir()` DBs, real HTTP clients and
  `ErrUseLastResponse`; the e2e drafts drive a real browser. `loginAsAdmin`
  injects a real cookie obtained from the real one-time exchange and validates
  it with a real `GET /admin`; it does not mock the server, and A-201's
  exchange/replay/cookie/guard contract is asserted against a fresh server in
  `admin_auth_go_test.go.txt`.
- **The attempt-5 fixes are present and stronger than their predecessors.**
  The cookie attributes are parsed into a map and compared exactly
  (`admin_auth_go_test.go.txt:138-157`), including `Path=/`, `Max-Age=86400`,
  `SameSite=lax` and the `HttpOnly` flag; the blank-token case re-runs the four
  frozen markup regexes (`:110-114`); both `#walk-pos` waits assert the 6 dp
  form (`admin-walk.spec.ts.txt:226, 441`); the delete flow waits for the
  frozen 303 target (`:572-575`); the edit fixture carries a photo and asserts
  the same `src` survives (`admin_lights_go_test.go.txt:459-464`); the merge
  alt is closing-quote anchored and checks the twin's label
  (`:213-218`); the numbered panel asserts `Save anyway` is absent and
  `Cancel` is present (`admin-walk.spec.ts.txt:409-410`); the rows fragment
  root is asserted (`admin_lights_go_test.go.txt:536-537`).
- **Rerun safety is coherent.** `playwright.config.ts:13-14` pins
  `workers: 1`/`fullyParallel: false`; one webServer shares `./test-run.db`;
  `admin.ts` exchanges the one-time token at most once per DB (memo + cache +
  `sessionWorks`) and reuses the cookie across e2e→a11y; per-test asset
  numbers, jittered unnumbered coordinates, fixed pagination lanes,
  `maxRedirects: 0` on every `page.request.post`, and href dedupe are all
  present. The residual point-vs-point jitter collision is disclosed and fails
  loudly; the stable-prefix sweep covers cross-run seeds.
- **Schema oracle and fixtures.** `testdata/schema.expected.sql` matches the
  packet's frozen block line-for-line and copies the probe `CREATE TABLE`
  verbatim from `migrations/0001_init.sql:2-5`; the generator is stdlib-only,
  deterministic, self-verifying (APP1/TIFF layout, CRC-correct pixel bomb,
  HEIC `ftyp` box, 15 MB cap), and the README records every fixture's sha256.
  The pixel bomb, HEIC sniff, text-as-jpg and oversize fixtures are the right
  shapes for the A-206/A-207 assertions.
- **Ownership/manifest.** Only owned paths are touched (`acceptance/**`,
  `testdata/photos/**`, `testdata/schema.expected.sql`,
  `harness/protected.sha256`, handoff). The manifest adds the six new
  `acceptance/**` files in hash order and leaves the nine pre-existing entries
  unchanged; the ratchet is unchanged (drafts are inert). No new dependency is
  added; the only external imports (`@axe-core/playwright`, Playwright) already
  exist.

## Verdict rationale

The attempt-5 diff is a genuine hardening pass: it closes the attempt-4
findings (exact cookie attributes, blank-token markup, 6 dp `#walk-pos`,
preview source tracking, delete navigation wait, photo-preserving edit,
merge-label application, `disabled` attribute, `#rows` root) and the earlier
blockers (shared-DB rerun safety, one-time-token economy, `maxRedirects: 0`,
detail-href dedupe, per-test assets, merge/save-anyway flows, keyboard pin).
The remaining findings are Low/Info coverage gaps and one operational edge, not
deleted/weakened existing tests, skips, mocks or production special-cases. No
draft is provably unsatisfiable against the frozen contracts, and the two
contract reconciliations (client-side bbox alert; numbered/un-numbered
duplicate buttons) are recorded in the README and routed for planner
ratification. The oracle is ready to freeze; the Low findings are worth closing
while T-207 still may not edit it, but none justifies blocking.

VERDICT: PASS
