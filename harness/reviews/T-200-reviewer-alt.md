# T-200 — reviewer-alt (test integrity / reward hacking)

Review basis: the attached diff, the operative packet `harness/tasks/T-200.md`,
the downstream contracts `harness/tasks/T-203.md` / `T-207.md`, `playwright.config.ts`,
`Makefile`, `scripts/holdout-run.sh`, `cmd/harness/guard.go`, and the installed
Playwright API types.

Verification limits: the diff is **not applied** in this checkout (there is no
`acceptance/drafts/`, `testdata/photos/*` is empty), and shell execution is
restricted to `ls`. The holdout repo is outside the sandbox. The handoff's exit
codes are therefore taken on trust. Every finding below is a static check of the
drafted bytes and the contracts they must satisfy.

## Findings

### 1. [Blocker] A one-time token is used as a per-test login, 10 times per project, against one server and one persistent DB

`acceptance/e2e/util/admin.ts:15-18` performs the token exchange on every call:
it navigates to `/admin/login?token=…` and `waitForURL(/\/admin(?:\?|$)/)`. Its
own comment (`admin.ts:16-17`) says “a wrong or replayed token would land on 403
instead and this wait would fail.”

The draft calls it ten times per project:
`acceptance/drafts/admin-walk.spec.ts.txt:51,78,95,106,143,153,172,201,221,270`.
Playwright creates a fresh browser context (and therefore a fresh cookie jar)
for each test, so the session from test 1 is not reused.

The contract makes the token *globally* one-time:
`acceptance/drafts/README.md:59` (“one-time: replay → 403”);
`harness/tasks/T-203.md:37-40` implements `ConsumeAdminToken` as a DB-backed
`login_tokens` row whose `used_at` makes a second call return `ErrTokenReused`;
the holdout variant explicitly requires “one-time token is GLOBAL — client B
(fresh jar) cannot replay client A's consumed token” (`harness/handoffs/T-200.md:29`).
The visible e2e helper is therefore incompatible with the visible A-201 Go draft
and the holdout.

Consequences, all fatal to T-207:

- Within one Playwright run, the first admin test (test 1) consumes the token;
  every later `loginAsAdmin` gets 403 and fails. Only 1 of the 10 admin logins
  per project can succeed.
- `playwright.config.ts:49-67` starts one webServer for the whole run with
  `DB_PATH=./test-run.db`. The `login_tokens` row persists, so a second
  `npx playwright test` run has **zero** valid logins, and even the same
  `make check` fails: `make check` runs `e2e` and then `a11y` as separate
  Playwright invocations (`Makefile:136`); the `e2e` run consumes the token and
  the `a11y` run's `@a11y` test (`admin-walk.spec.ts.txt:270`) then gets 403.
- The only ways to make this green are to make the token reusable in test mode
  (a test-only special case that breaks the frozen A-201 replay assertion and
  the holdout) or to rewrite the e2e helper/oracle. That is exactly the
  reward-hacking / oracle-editing path this task exists to prevent.

The handoff's “red against main” proof does not exercise a contract-compliant
server, so it does not establish satisfiability.

### 2. [Blocker] E2E test 5 creates `GP-017` after test 1 already created `GP-017`; the frozen unique index makes the second insert a 409

Test 1 saves `GP-017` (`acceptance/drafts/admin-walk.spec.ts.txt:64`). Test 5
then calls `addLightFromForm(page, { assetNo: 'GP-017', … })`
(`admin-walk.spec.ts.txt:154`) and immediately expects `Saved`
(`admin-walk.spec.ts.txt:155`), then posts another `GP-017`
(`admin-walk.spec.ts.txt:160`).

The frozen schema has
`CREATE UNIQUE INDEX lights_asset_no ON lights(asset_no) WHERE asset_no IS NOT NULL AND deleted_at IS NULL`
(`testdata/schema.expected.sql:43`). Test 1’s light is still live (test 7 deletes
only `GP-018`; test 6 depends on test 1’s `GP-017` still existing), so the insert
at test 5 cannot succeed: the server must return the frozen duplicate response
(409 / `Possible duplicate`), not `Saved`. The test fails at its first assertion.

This is independent of finding 1 and persists across runs: `./test-run.db` is
not deleted by `playwright.config.ts` or the Makefile, so on a second run test 1
itself also hits the already-present `GP-017` and cannot save. The e2e admin
suite is not idempotent. The packet specifies `GP-017` for both tests, but the
draft had to reconcile that shared-state collision (e.g. a distinct asset number
for test 5’s baseline or a cleanup); it does not.

### 3. [Blocker] The pagination seed’s `page.request.post` follows the create 303, so `expect(res.status()).toBe(303)` can never pass

`acceptance/drafts/admin-walk.spec.ts.txt:233` posts each seed with
`page.request.post('/admin/lights', { … })` and asserts `303` at
`admin-walk.spec.ts.txt:242`. Playwright’s `APIRequestContext` follows redirects
by default (`maxRedirects` defaults to 20; see
`node_modules/playwright-core/types/types.d.ts:19624-19628`), and neither
`playwright.config.ts` nor the request overrides it. The server’s 303
`Location: /admin/lights/{id}` is followed, so the observed status is `200`.
The loop fails on seed 1 and the pagination test never reaches `Older`.

The Go `TestA209Pagination` uses a `CheckRedirect: ErrUseLastResponse` client, so
only the e2e draft has this bug. A `maxRedirects: 0` request option (or an
assertion on the followed 200) is required; the current test cannot pass.

### 4. [High] The A-206 e2e test is not self-contained and its claim cannot be run with `make e2e-task`

`admin-walk.spec.ts.txt:177` fetches `img[alt^="GP-017"]`, relying on test 1
(`admin-walk.spec.ts.txt:64`) having run first. The handoff discloses the file
order dependency (`harness/handoffs/T-200.md:130`). But the task workflow is
`make e2e-task TASK=T-206` → `npx playwright test --grep "@A-206"` (`Makefile:111-116`);
that selects only the `@A-206`-tagged test, excludes test 1 (`@A-202 @A-203`), and
the test fails on a fresh DB because no `GP-017` light exists. The test should
create its own photo-bearing light (via the API or the form) rather than depend
on an earlier test, or the claim cannot be proven in isolation.

### 5. [Medium] The `Older` selector narrows the frozen “link/button” contract to a link

`acceptance/drafts/README.md:39` freezes “link/button `Older`”, but
`admin-walk.spec.ts.txt:257` uses `page.getByRole('link', { name: 'Older' })`.
A fully compliant implementation that renders `<button>Older</button>` fails the
test. The selector needs `.or(getByRole('button', …))` (or the contract must be
tightened to “link”); this is a narrowed test against the frozen table.

### 6. [Low — contract clauses left unasserted]

- The README freezes `POST /admin/lights/{id}` as “edit (multipart `csrf_token` +
  changed fields; **unspecified fields keep values**)”
  (`acceptance/drafts/README.md:62`), but `TestA208EditSoftDelete` posts only
  `csrf_token` + `label` and checks only the new label
  (`acceptance/drafts/admin_lights_go_test.go.txt:702-708`). Asset number, kind,
  coordinates and photo could all be cleared and the test would pass.
- The duplicate visual note says “nothing is saved until one is chosen”
  (`acceptance/drafts/README.md:121-123`), but `TestA205Duplicates` does not
  inspect the list between the 409 and the merge
  (`admin_lights_go_test.go.txt:475-483`), and the e2e A-205 test never chooses
  `Merge`/`Save anyway`. A premature save that the later merge collapses would
  still leave the asserted one-row count.

### 7. [Low — flakiness] A-203 reads `#pin-pos` immediately after `mouse.up`

`acceptance/drafts/admin-walk.spec.ts.txt:122-124` reads the post-drag text with
no wait for the map/position update. If the pin position is committed on a
`moveend`/animation frame, `after` can still equal `before`. `expect.poll` is the
appropriate wait (T-207 rule 3 explicitly permits a flaky-wait fix, but the
frozen oracle should not ship flaky).

### 8. [Checked — no integrity failure]

- No existing test is deleted, skipped or narrowed by the diff; no
  `t.Skip`/`test.skip`/`test.only`/`.fixme` is added; the only new “panic” calls
  are encode-error guards in the fixture generator, not production code.
- No production code is touched, so there is no fixture-value special-case in
  app code and no mock bypassing the behaviour under test. The Go drafts spawn
  the real `bin/server`; the e2e drafts drive a real browser.
- The schema oracle matches the packet’s product schema and copies the probe
  `CREATE TABLE` verbatim from `migrations/0001_init.sql:2-5`
  (`testdata/schema.expected.sql:1-86`).
- The fixture generator’s APP1/TIFF construction, CRC-correct pixel bomb,
  HEIC sniff box and deterministic writes check out statically; the fixture
  files are not in the manifest, but ownership (`testdata/**` is T-200’s) and
  the frozen hashes in `acceptance/drafts/README.md:81-87` are the protection.
- The alt-format rule (`acceptance/drafts/README.md:50`, `alt="<asset_no>
  <label>"` for numbered lights) is a necessary reconciliation of the table’s
  `img[alt=<label>]` with test 7’s `img[alt^="GP-017"]`. It is not a weakening
  (it adds the asset number) but it is a frozen-contract amendment and should be
  ratified by the planner, as was T-105’s regex pin.
- The fixture-name string literals in `testdata/photos/gen/main.go` will trip
  the guard’s warning-only special-case heuristic (`cmd/harness/guard.go:250-265`);
  that is a benign false positive, since the generator must name its outputs.
- Bookkeeping: the handoff says “5 new entries” in `harness/protected.sha256`,
  but the diff adds six (README + three drafts + two utils)
  (`harness/reviews/T-200.diff:2057-2072`); harmless.

## Verdict rationale

The drafts are indeed red against current main, but red is not the same as
satisfiable. Findings 1–3 independently make the frozen e2e oracle impossible to
pass against any contract-compliant M2 implementation: a global one-time token
is used as a per-test login across one shared server and persistent DB; a fixed
asset number collides with the schema’s unique index; and Playwright’s default
redirect-following makes the `303` assertion unobservable. Together with the
claim-specific dependency on test 1, T-207 would be forced either to edit the
frozen oracle or to add test-only token/DB special cases, destroying the
contract-first property the task is meant to establish. The Go drafts, schema
oracle and fixtures are sound; the e2e draft needs rework before activation.

VERDICT: BLOCK
