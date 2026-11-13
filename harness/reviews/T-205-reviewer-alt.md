# T-205 — independent review (reviewer-alt): admin UI, light CRUD, HTMX list

Basis: attached diff, `harness/tasks/T-205.md`, `AGENTS.md`,
`acceptance/drafts/README.md`, `acceptance/contract.json` (A-202/A-203/A-205/
A-208/A-209), the inert Go/e2e drafts, the checked-out source, the handoff and
the on-disk `test-results/` screenshots.

Verification limit: shell execution for the test/build commands was denied in
this session, so this is a static source review plus a vision check of the
committed screenshot artifacts. The handoff's exit codes are taken on trust.

## Findings

### 1. [High — reward hacking / explicit contract violation] Merge applies the new label but deliberately drops the uploaded photo

- The packet freezes `dup_choice=merge` as "apply photo/label onto ExistingID,
  no new row" (`harness/tasks/T-205.md:60-69`), and the protected contract table
  repeats "Merge APPLIES the new submission's label/photo onto the existing
  row" (`acceptance/drafts/README.md:68`).
- `internal/web/admin_create.go:185` calls `s.Photo(r)` **before** duplicate
  classification (`:196`), so a merge submission's photo is already persisted to
  `PHOTO_DIR` and the `photos` table. `mergeDuplicate` then calls only
  `EditLight(ctx, existing.ID, sub.Label, existing.AssetNo)`
  (`internal/web/admin_create.go:289-315`); `EditLight` has no photo parameter
  (`internal/store/lights.go:252-274`). The survivor keeps its old `PhotoID` and
  the newly uploaded photo is orphaned. The "vanished duplicate" fallback also
  creates a light without `PhotoID` (`internal/web/admin_create.go:294-301`).
- The handoff admits this and states the rationale outright: "The frozen Go
  draft asserts only the label application, so acceptance passes; photo-merge
  needs an `EditLightPhoto`-style store method first"
  (`harness/handoffs/T-205.md:208-214`). Shipping behaviour known to violate the
  written contract because the oracle is narrower is exactly the reward-hacking
  pattern this review is for. The task should have been `BLOCKED` for an owns
  extension / store API or the photo clause should have been ratified away by
  the planner.
- No test compensates. `TestAdminDuplicateNumbered` (`internal/web/admin_test.go:509-523`)
  merges without a photo; the frozen Go draft's own comment mentions photo/label
  but only asserts the label
  (`acceptance/drafts/admin_lights_go_test.go.txt:208-218`). A merge-with-photo
  test would leave both the implementation and this test gap red.

### 2. [High — test cannot fail for the property it claims] Numbered duplicate-panel content check is vacuous

`TestAdminDuplicateNumbered` requests a duplicate with label
`"Ball field lamp twin"` (`internal/web/admin_test.go:480`) and then checks the
response body contains `"Ball field lamp"` (`:486`). That string is a substring
of the submitted label, which is also re-rendered into the form input
(`web/templates/pages/admin-walk.html:54`). `"GP-017"` likewise appears in the
asset-number input (`admin-walk.html:56`). A panel that showed no existing
label/asset at all — or only the submitted values — would still pass. The test
checks panel existence and buttons, but not the frozen "existing light" content.
Assert the text of `#dup-panel` (or an exact rendered fragment), not a substring
present elsewhere on the page.

### 3. [Medium — narrowed pagination-boundary test] A two-row list can pass the 50/51 contract test

`TestAdminPaginationBoundary` claims to cover "50 lights: one page" but only
checks `padLamp(1)` and `padLamp(50)` are present
(`internal/web/admin_test.go:721-729`); it does not count rows or check the
other 48 labels. After the 51st create, page 2 is checked only for
`padLamp(1)`, not for the other four expected rows, and page 1 only checks that
`padLamp(1)` is absent (`:730-750`). The rows-fragment check similarly checks only
`padLamp(1)` present / `padLamp(51)` absent (`:751-758`). A regression that
renders two rows per page instead of fifty would stay green. The frozen Go
draft loops over every expected label
(`acceptance/drafts/admin_lights_go_test.go.txt:511-546`); the unit test should
do at least a full count and the complete 06–55 / 01–05 sets.

### 4. [Medium — assertion weaker than its own comment and the double-submit contract] CSRF input is never compared with the cookie

`TestAdminWalkPage` says "it matches the cookie"
(`internal/web/admin_test.go:236-237`) but the code only proves a cookie exists
(`:226-235`) and that some `csrf_token` input has a `value` attribute
(`:238-241`). It never compares the input value to the cookie value. A template
that minted a different token in the form (or a handler that set the cookie
separately) would pass this test and fail the frozen draft's double-submit flow.
Parse both values and compare them; the comment should not claim a check that
does not exist.

### 5. [Medium — server-side re-render loses submitted values / breaks CSRF] `walkPageDataOf` omits the asset number; parse-error re-render emits an empty token and bbox

`walkPageDataOf` builds `walkValues` with Label, Kind, Lat, Lon and AccuracyM
only (`internal/web/admin_create.go:377-389`), but the template renders
`value="{{.Values.AssetNo}}"` (`web/templates/pages/admin-walk.html:56`).
After a 422/409 full-page re-render the admin's typed asset number is lost.
`TestAdminCreateOutsideBBox` (`internal/web/admin_test.go:388-414`) only checks
the message/status, so this regression is not covered.

Separately, `walkReject` with `sub == nil` (the parse-error path) renders a
`walkPageData` with no `CSRFToken` and no `BBox`
(`internal/web/admin_create.go:364-371`). The resulting page contains
`<input ... name="csrf_token" value="">` and `data-bbox=""`, so a no-JS/admin
client cannot retry. The non-multipart test only asserts the 400
(`internal/web/admin_test.go:379-385`). Mint a token and bbox for this path, or
answer a fragment/page that does not pretend to be a usable form.

### 6. [Medium — dead/unwired template with false test coverage] `pages/admin-login.html` is not the live login page

The packet requires `pages/admin-login.html` to replace T-203's minimal page
(`harness/tasks/T-205.md:75-78`). No handler renders it:
`internal/web/auth.go:57-60,97-99,143-179` still serves the inline
`loginPageTmpl` via `renderLoginPage`. The handoff discloses this
(`harness/handoffs/T-205.md:197-207`), but still calls the task COMPLETE, and
`TestRenderAll` renders and snapshots the unwired template
(`internal/web/web_test.go:122-126`). `make templates-check` therefore validates
a page production never serves; the live login page remains unstyled and below
the A-201 44 px tap-target note. This is dead production code plus coverage
theatre. Either wire it through an explicit owns exception/T-203 hook or mark
the deliverable blocked and defer it, but do not count the template as delivered
based on `TestRenderAll`.

### 7. [Low — edit test narrower than A-208] Position and photo survival are not asserted

`TestAdminEdit` checks the new label, asset number and latitude
(`internal/web/admin_test.go:627-641`) but not the longitude or the stored
photo. The protected visual/contract notes require a label-only edit to keep the
position **and** the stored photo (`acceptance/drafts/README.md:216-220`). A
regression that dropped the longitude or photo would pass this unit test; the
frozen Go draft covers the photo
(`acceptance/drafts/admin_lights_go_test.go.txt:444-465`), but the local suite
should not rely on T-207 to catch it.

### 8. [Info — handoff accuracy] Commit/line counts do not match the tree

`harness/handoffs/T-205.md:16,192` says the branch is four commits ahead of
main; the checkout shows the four feature/handoff commits plus the later
handoff-rewrite commit (`4926a29`) on top. `:78` says `walk.js` is 199 lines and
its commit message says "199 lines", while the diff/file is 200 lines (still
within the ≤200 budget, but the evidence claim is wrong). These are reporting
nits, not gate blockers.

## Checked and found sound

- **No fixture-value special-casing in production.** Searching the new
  `internal/web` code and `web/static/js/walk.js` found no acceptance literals
  (`GP-017`, `Ball field lamp`, `42.7617`, `42.36`, etc.); those values occur only
  in tests, the handoff, and the protected drafts.
- **No mocks bypassing behaviour.** `admin_test.go` opens a real per-test
  temp-file SQLite store, mints a real session, and drives the full
  `Server.Handler()`; the photo tests use real fixtures and a real
  `PHOTO_DIR`.
- **No new skip/only/fixme, dependency, protected-path or golden change.** The
  only `t.Skip` is the pre-existing `TestRenderAll` env guard
  (`internal/web/web_test.go:116-118`), untouched in the diff.
- **Most core paths are covered:** guard redirect/401, walk page markup, empty
  list, `#rows` fragment root, 303 without HX, create happy path/HX Saved,
  validation, bbox 422 both HX and full page, photo 413/415, duplicates,
  save-anyway, detail 404s, edit, delete, and pagination statuses.
- **The screenshots exist and visually match the main flows** (walk readout,
  disabled low-accuracy button, bbox alert, numbered/unnumbered panels, list
  thumbnails, detail form); the problem is the source tests, not the evidence
  being absent.

## Verdict rationale

The implementation is broad and mostly correct, but it knowingly ships the
merge photo half of the A-205 contract unimplemented and justifies that omission
by the acceptance draft's narrower assertions; that is reward hacking. On top of
that, several tests cannot detect the regressions they claim to cover
(duplicate-panel content, CSRF matching, the 50/51 row set), and the login
template is dead while `TestRenderAll` reports it as covered. These are
test-integrity failures, not cosmetic test nits. The task should not pass the
gate until the merge photo behaviour is implemented or formally re-scoped and
the listed assertions are strengthened.

VERDICT: BLOCK
