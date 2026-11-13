# T-200 — reviewer-alt (test integrity / reward hacking), attempt 4

Review basis: the attached diff (`harness/reviews/T-200.diff`), the operative
packet `harness/tasks/T-200.md` (including the appended attempt-1/2 reviews),
the downstream packets `T-201..T-207`, `acceptance/contract.json`,
`playwright.config.ts`, `Makefile`, `.gitignore`, `cmd/harness/{guard,ratchet}.go`,
`harness/policy.json`, `harness/protected.sha256`, `migrations/0001_init.sql`,
`internal/web/csrf.go`, `.env.test`, and the checked-in acceptance suite.

Verification limits: the diff is **not applied** in this checkout (there is no
`acceptance/drafts/`, `testdata/photos/*` is absent and `testdata/schema.expected.sql`
is absent), shell access is limited to `ls`/`pwd` (git, `head`, `cat` and
`python3` are denied), and the holdout repo is outside the sandbox. Every
finding below is a static check of the diffed bytes against the contracts and
repo facts I could read. The handoff's executed red/green results, the manifest
sha256s and the holdout commit are taken on trust; the gate re-verifies the
manifest (`VerifyManifest`) and the fixture generator self-verifies its bytes.

## Findings

### 1. [Low — assertion weaker than the frozen contract] `Path=/` cannot fail for `Path=/admin`

`acceptance/drafts/admin_auth_go_test.go.txt:126-130` asserts the cookie
attributes with:

```go
for _, want := range []string{"httponly", "samesite=lax", "path=/"} {
    if !strings.Contains(strings.ToLower(sessionCookie), want) { ... }
}
```

`"path=/"` is a substring of `"path=/admin"`, so a cookie scoped to `/admin`
passes the check. The README freezes `Path=/` (`acceptance/drafts/README.md:96`),
and T-203 freezes `Path=/` and `MaxAge` 24 h (`harness/tasks/T-203.md:62-64`);
the handoff explicitly claims this closes the attempt-3 finding
(`harness/handoffs/T-200.md` fix-map row "alt 7 / reviewer 1"). It does not.
The same loop also never asserts `Max-Age`/`Expires`, so a session cookie with
no 24 h lifetime passes the acceptance oracle even though T-203 requires it.
This is a coverage-theatre/weaker-assertion issue, not an unsatisfiable test:
the functional exchange and guard are still enforced, and the cookie's path
does not affect the tested `/admin` flows. It is worth a one-line
strengthening while the author still owns the oracle; fix by parsing the
`Set-Cookie` value (e.g. `http.ParseSetCookie` on Go 1.23+) and asserting
`Path == "/"` and `MaxAge == 86400`, or at minimum matching the attribute
terminator (`; path=/;` / end-of-string) and checking `max-age=86400`.

### 2. [Low] Blank-token 200 is status-only

`acceptance/drafts/admin_auth_go_test.go.txt:104-107` asserts only
`status == 200` for `GET /admin/login?token=` (blank). The README freezes that
blank renders the form page (`acceptance/drafts/README.md:96`), and the
no-token branch checks the four markup regexes
(`admin_auth_go_test.go.txt:92-96`). An implementation that answers an empty
200 for a blank `token` passes this draft while violating the frozen form-page
contract. Re-run the markup regexes on the blank-token body.

### 2b. [Low] `#walk-pos` is frozen at 6 dp but asserted only as `\d+\.\d+`

The README freezes `#walk-pos` as "rendered at 6 decimal places"
(`acceptance/drafts/README.md:66`), but every `#walk-pos` assertion in the e2e
draft uses the loose packet regex `\d+\.\d+, -?\d+\.\d+`
(`admin-walk.spec.ts.txt:188-194, 223-224, 466-467, 607`; the `openWalkForm`
wait is the first of these). A walk page that renders `42.76, -71.03` (2 dp)
passes the wait and test 1; the `#pin-pos` and `#light-pos` checks separately
use six decimals. Use `\d+\.\d{6}, -?\d+\.\d{6}` (or assert the exact rendered
text) so the README's 6 dp clause is actually frozen.

### 3. [Low] `@A-206` preview can be a static landscape placeholder

`acceptance/drafts/admin-walk.spec.ts.txt:510-521` polls
`naturalWidth > 0` and then asserts only `naturalW > naturalH`. A static
landscape placeholder `img#photo-preview` (320×200) passes without showing the
uploaded portrait fixture at all; the test never checks the preview's
`src`/object URL or its expected dimensions. The Go test does prove the served
photo's exact 800×600 (`acceptance/drafts/admin_lights_go_test.go.txt:298-302`),
but the preview claim is weaker than the README's "in-form preview ... renders
EXIF orientation applied" (`README.md:82`). Assert `naturalW === 800 &&
naturalH === 600` for this fixture (or assert the preview `src` is a
blob/data URL derived from the selected file).

### 4. [Low — flake] `@A-208` clicks Delete and immediately `page.goto`s

`acceptance/drafts/admin-walk.spec.ts.txt:551-554`:

```ts
await page.getByRole('button', { name: 'Delete light' }).click();
await page.goto('/admin');
```

`click()` does not wait for a navigation triggered by the button; the
immediate `goto` can cancel the in-flight delete POST/303. The frozen contract
is a 303 to `/admin` (`README.md:100`); wait for the URL/navigation (or the
row's disappearance) before asserting the list. This is a mechanical
`expect.poll`/`waitForURL` fix, but it should not ship frozen.

### 5. [Low] Edit "unspecified fields keep values" only proves asset + position

`acceptance/drafts/admin_lights_go_test.go.txt:434-443` edits only `label`
and checks that the asset number and position survive. The light has no photo,
and `kind` is already the default `park`, so a server that resets
`kind`/`photo` on every edit still passes. The README freezes "unspecified
fields keep values" (`README.md:99`). Create the edit fixture with a photo
(and/or assert the kind round-trip) so the preservation clause is actually
proven.

### 6. [Low] Merge only proves one row, not that the pair was merged

`acceptance/drafts/admin_lights_go_test.go.txt:204-207` asserts exactly one row
whose alt starts `GP-017 ` after `dup_choice=merge`. A server that receives
`dup_choice=merge` and does nothing (keeps the original row) also passes. The
e2e twin is the same (`admin-walk.spec.ts.txt:653-661`). T-205 freezes
"dup_choice==merge → apply photo/label onto ExistingID"
(`harness/tasks/T-205.md:67-68`); assert the merged row carries the twin's
label (or the photo) to prove the merge, not just the row count.

### 7. [Low] Low-accuracy test never attempts a save before the choice

`acceptance/drafts/admin-walk.spec.ts.txt:263-274` asserts the button is
disabled, clicks `Save anyway (low accuracy)`, then completes a real save. It
never attempts a submit while the gate is active, so an implementation that
disables the button but still accepts a form submit by another path (Enter,
another button, direct POST) passes. The frozen contract is "save gated at
≤30 m or explicit save-anyway" (`acceptance/contract.json`; `README.md:194-197`).
This is the same class the attempt-2 review raised, only partially addressed.

### 8. [Info] Client-side bbox alert is a new reconciliation, not a T-205 behaviour

`acceptance/drafts/admin-walk.spec.ts.txt:361-368` expects a `role=alert`
immediately on `setGeolocation` outside `TOWN_BBOX`, without submitting. T-205's
walk.js behaviour list only freezes the server-side 422
(`harness/tasks/T-205.md:60-62,89-101`). The draft is the declared oracle and
the README now records reconciliation 4 (`README.md:46-53`), and T-205 is
instructed to read the draft, so this is satisfiable; it is still the most
likely activation-time surprise. Planner ratification is the right path (as the
handoff says).

### 9. [Info] `@A-209` cleanup depends on `GET /admin?page=2`

`acceptance/drafts/admin-walk.spec.ts.txt:141-154` sweeps `/admin?page=2` to
find `Scroll lamp`/`Edge lamp` rows. The frozen HTTP table
(`README.md:94-103`) explicitly freezes `GET /admin`, the 50-newest list, and
`GET /admin/rows?page=N` (303 target `/admin?page=N`), but not the full-page
`GET /admin?page=N`. T-205's `GET /admin` query-page implementation makes this
fine; the table could state it to remove the ambiguity.

### 10. [Info] `harness/tasks/graph.json:275` status change

The diff changes T-200 from `review` to `merging`. That path is outside the
packet's `owns` (though `acceptanceAllowed` permits `harness/tasks/**` for an
acceptance task). The handoff attributes the transition to the scheduler; if it
is not scheduler-generated it should be dropped. No test-integrity impact.

## What I checked and found sound

- **No weakened/deleted/skipped/narrowed existing test.** The diff adds new
  drafts, fixtures and two active utils only. No `t.Skip`/`test.skip`/
  `test.only`/`.fixme` appears; no existing acceptance file is edited; the
  marker scan exempts `acceptance/` but the added non-test code (the fixture
  generator) is clean.
- **No special-casing in production code.** No app code is touched. The
  `GP-017`/`GP-018`/coordinate literals live in the test drafts and the
  fixture generator, not in handlers/templates/store. The fixture generator's
  filename literals are data generation, not a branch on fixture values.
- **No mocks bypassing the behaviour.** The Go drafts spawn the real
  `bin/server` with a fresh `t.TempDir()` DB and use real HTTP clients with
  `ErrUseLastResponse`; the e2e drafts drive a real browser. `loginAsAdmin`
  injects the real session cookie obtained from the real one-time exchange; it
  does not mock the server, and A-201's exchange/replay/cookie/guard contract
  is asserted against a fresh server in the Go draft.
- **Attempt-4 fixes are present:** `openWalkForm` now waits for `#walk-pos` and
  `#walk-accuracy` before opening every walk form
  (`admin-walk.spec.ts.txt:188-210`, used in all six @A-205 sites and @A-206);
  the preview polls `naturalWidth > 0` (`:510-521`); the blank-token case is
  asserted 200 (`admin_auth_go_test.go.txt:104-107`); the served photo is
  asserted exactly 800×600 (`admin_lights_go_test.go.txt:298-302`); `jpegSize`
  has the truncated-SOF guard (`images.ts:98-100`); @A-209 sweeps the stable
  `Scroll lamp` prefix (`admin-walk.spec.ts.txt:557-559`); and the heading
  regexes tolerate whitespace (`admin_lights_go_test.go.txt:329-331`).
- **Rerun safety under the real topology is coherent.** `playwright.config.ts`
  pins `workers: 1`/`fullyParallel: false`; one webServer shares
  `./test-run.db`; `admin.ts` exchanges the one-time token at most once per DB
  (memo + `test-run.db.session.json`, proven with a `sessionWorks` GET) and
  reuses the 24 h cookie across the e2e→a11y invocations. Unique per-test assets,
  jittered unnumbered coordinates, fixed pagination lanes, `maxRedirects: 0`
  on every `page.request.post`, and href dedupe are all present. The residual
  point-vs-point jitter collision is disclosed and fails loudly; the
  stable-prefix sweep covers the cross-run seed case.
- **Architecture/ownership:** test/fixture paths only; the drafts never swap
  `#map`; the manifest adds exactly the six new acceptance files in hash order
  and leaves the nine pre-existing entries unchanged; the ratchet is unchanged
  (drafts are inert); the `testdata` oracle and fixtures are the owned,
  generator-verified artifacts. No dependencies were added (`@axe-core/playwright`
  and `golang.org/x/image` already exist), no `console.log`, no TODO/FIXME, no
  dead code introduced merely to satisfy a linter.
- **Schema oracle and fixtures:** `testdata/schema.expected.sql` is line-for-line
  the packet's frozen product schema plus the probe `CREATE TABLE` copied from
  `migrations/0001_init.sql:2-5`; the generator is stdlib-only, deterministic,
  self-verifying (APP1/TIFF layout, CRC-correct pixel bomb, HEIC sniff box,
  15 MB cap) and the README records the expected sha256 for every fixture.

## Verdict rationale

The attempt-4 diff is a genuine hardening pass: it fixes the flaky waits, the
preview decode race, the blank-token contradiction, the exact processed
dimensions, the SOF guard, the cross-run pagination sweep and the heading
tolerance, and the remaining probes are satisfiable against the frozen
contracts. The findings above are test-quality/coverage issues, not an
unsatisfiable oracle: the strongest (`Path=/` / MaxAge, `#walk-pos` 6 dp) are
weak attribute/format assertions worth a one-line strengthening while the
author still owns the oracle, but the functional exchange, replay, guard, CRUD,
duplicate, photo and pagination behaviours are all exercised against real
servers and a real browser.
None of the findings is a deleted/weakened existing test, a skip, a mock of the
behaviour under test, or a production special-case. The two contract
reconciliations (client-side bbox alert; numbered/un-numbered duplicate buttons)
are recorded in the README and routed for planner ratification, as the earlier
reviews required.

VERDICT: PASS
