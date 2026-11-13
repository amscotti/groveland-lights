# T-205 review — Admin UI: walk-the-town flow, light CRUD, list + HTMX pagination

Review basis: the attached diff (`harness/handoffs/T-205.md` + `internal/web/admin.go`
+ `internal/web/admin_create.go` + `internal/web/admin_test.go` + `internal/web/web_test.go`
+ `web/static/js/walk.js` + `web/static/js/globals.d.ts` + 7 templates), checked out and
read in this worktree against: the packet `harness/tasks/T-205.md`, the frozen oracle
(`acceptance/drafts/README.md`, `acceptance/drafts/admin-walk.spec.ts.txt`,
`acceptance/drafts/admin_lights_go_test.go.txt`), `acceptance/contract.json`,
`harness/tasks/graph.json` (owns/claims), the merged sources the diff builds on
(`internal/web/{web,csrf,auth,session,upload,probe,templates}.go`,
`internal/store/lights.go`, `internal/geo/{geo,duplicate}.go`,
`internal/config/config.go`, `web/templates/base.html`,
`web/static/vendor/htmx-2.0.11.min.js`), and the on-disk evidence under `test-results/`.

Verification limits: shell execution was denied this session, so the handoff's command
exit codes are trust-marked (cross-checked for internal consistency against the tree).
Everything load-bearing was verified statically; the committed files were read directly
and match the diff byte-for-byte in every spot checked. The visual evidence itself was
inspected structurally (16 `visual-*` result dirs + `test-results/.last-run.json`
`"status": "passed"` — the handoff's claimed paths exist, unlike the previous attempt).

## Claimed assertions — all four satisfied against the frozen oracle

- **A-202 (walk page, accuracy gate)**: `#walk-pos`/`#pin-pos` render 6 dp via
  `toFixed(6)` (`web/static/js/walk.js:20`); `#walk-accuracy` "accuracy N m"
  (`walk.js:90`); gate at >30 m uses the platform `disabled` attribute with the
  `Save anyway (low accuracy)` unlock (`walk.js:96-98,195`) and a real 80 m save is
  wired (scratch suite `visual-low-accuracy-gate-and-real-save-at-80-m-*`, passed);
  denied geolocation shows `#manual-lat`/`#manual-lon` number inputs
  (`walk.js:104-113`, `admin-walk.html:22-28`). The mid-watch `setGeolocation`
  transient-error hijack that an earlier scratch iteration hit (see
  `web/test-results/walk--scratch-A-202-*/error-context.md`, "manual position" at
  accuracy 8) is fixed by the `state.baseLat === null` guard (`walk.js:126-130`).
- **A-203 (pin)**: `#pin` `role=button` `tabindex=0` `aria-label="position pin"`
  (`admin-walk.html:16`); pointer drag ±25 m clamped (`walk.js:139-155`) + arrow-key
  1 m nudges (`walk.js:156-162`); server renders the same 6 dp string
  (`posString`, `internal/web/admin.go:290-292`), so `#light-pos` equals `#pin-pos`.
- **A-208 (detail/edit/delete)**: `#light-pos` 6 dp, photo `src="/photos/..."`,
  labelled edit form, `Save changes` in a separate form from `Delete light`
  (`admin-detail.html:4-21`); nil fields keep stored values
  (`parseLightEdit`/`validatedEdit*`, `admin.go:363-379,453-479` — an empty-string
  post is a pointer-to-empty, so label→422 and asset_no→clears, correctly); edit
  303→detail, delete 303→/admin, 404 after soft delete (all tested through the full
  middleware chain on a real temp DB).
- **A-209 (list + HTMX pagination)**: heading `Lights`, 50/page + 1-row lookahead
  (`recentRows`, `admin.go:126-146`), fragment root `id="rows"`
  (`partials/admin-rows.html:1`), `hx-get`/`hx-target="#rows"`/`hx-swap="outerHTML"`
  on `Older`/`Newer`, 303 to `/admin?page=N` without `HX-Request` (`admin.go:177-180`),
  50/51 boundary table-tested (`admin_test.go:708-760`).

Cross-cutting frozen clauses verified: create check order parse → CSRF → bbox → photo
→ duplicates (`admin_create.go:170-211`, with `LimitBody` first and the single multipart
parse reused by `s.Photo(r)` per `upload.go:94-108`); 422 frozen bbox message client
(pre-flight mirroring `config.BBox.Contains`'s inclusive edges, `walk.js:60-64`) and
server; 413/415 through `PhotoProblem` incl. the HEIC wording; numbered dup = `Merge` +
`Cancel` link only, numbered+save → 422, unnumbered = `Merge` + `Save anyway`,
nothing saved while the panel is up, merge applies the twin label with no new row;
thumbnail alt `"<asset_no> <label>"` / `"<label>"` (`thumbAlt`, `admin.go:73-78`);
HX create → role=status `Saved` fragment, non-HX → 303 `/admin` (`respondWalkOK`).
The htmx override is the correct mechanism for the frozen "422/409 swap the form
target": the vendored 2.0.11 default is
`responseHandling:[{code:"204",swap:false},{code:"[23]..",swap:true},{code:"[45]..",swap:false,error:true}]`
(`web/static/vendor/htmx-2.0.11.min.js:1`) and `walk.js:183-188` replaces it before
htmx boots (deferred htmx in `base.html:10` executes before the body-end module).

## Security — no issues found

- **CSRF**: verified on all three POST routes before any state change
  (`admin_create.go:176-179`, `admin.go:413-416`, `admin.go:521-524`); missing and
  mismatched tokens are both 403 and both table-tested.
- **XSS**: no `template.HTML` anywhere in the diff. Every template value is
  auto-escaped; `Status` is schema-CHECK-constrained; `alertView.Role` is a server
  constant. `rowView.ImgSrc` is the only `template.URL` and is provably server-minted:
  the constant data-URI placeholder or `"/photos/" + p.Path` where `p.Path` comes from
  the upload pipeline's server-generated names (`upload.go:222-241`) — no user input
  reaches it. Both `//nolint` uses carry reasons on the same line (rule 9).
- **SQL**: none in this package — all queries via `internal/store` (architecture map).
- **Path traversal**: photo serving is `upload.go`'s clamped resolver (untouched,
  T-206-reviewed); no new filesystem paths in this diff.
- **Upload limits**: `LimitBody(r)` first (15 MB cap) before the single multipart
  parse; the >15 MB body surfaces as `*http.MaxBytesError` → 413 (tested at
  `admin_test.go:460-467` with a real ~16 MB body); edit/delete bodies additionally
  capped at 1 MB (`parseFormValues`, `admin.go:379-395`); multipart memory 32 MB only
  ever holds ≤15 MB bodies.
- **Secrets**: no credential, token or coordinate values in any log line; handler
  errors log `err` from store calls only.

## Architecture — conforms

- Ownership: every changed file lands inside `owns` ∪ handoff
  (`graph.json:412-418`); `web.go`/`csrf.go`/`auth.go`/`session.go`/`upload.go`/
  `probe.go`/`base.html`/`index.html`/`probe.html` untouched; routes registered from
  `init()` in the task's own files (`admin.go:21-28`, `admin_create.go:22-24`), no
  pattern collisions with `auth.go`'s `/admin/login`.
- HTMX/#map rule: no `#map` in any admin template; `hx-target`s are `#rows` and
  `#dup-slot`, both outside any map ancestry (there is no map on these pages).
- Parse-don't-validate: `walkSubmission`/`lightEdit`/`lightDelete`/`listQuery`/
  `lightIDParam` lifted once at the boundary; grep confirms no `r.FormValue` after
  parsing in `admin*.go`. Typed `store.LightID` throughout (`parseLightID`,
  `admin.go:261-271`).
- Hygiene: no TODO/FIXME/`panic(`/`console.log`/"Phase N" in added lines; no deps
  added (`allow_deps: []`); migrations untouched; status derivation untouched (list
  shows stored status verbatim). `TestRenderAll`'s `t.Skip` is pre-existing context,
  not an added line (diff confirms).
- The new `TestRenderAll` fixture map turns "every page has data" into an enforced
  invariant (`web_test.go:169-174`) — a genuine improvement over `homeData`-everywhere.

## Tests — real behaviour, a few loose assertions

`admin_test.go` drives every route through `s.Handler()` (full middleware) against a
real temp-file SQLite store with a minted admin session and matching double-submit
pair: guard matrix (302/401+`HX-Redirect`), walk page frozen strings, empty list,
`#rows` root regex `\A\s*<[a-zA-Z][^>]*\bid="rows"` + 303 without HX, create happy
path (303 + HX `Saved` fragment), 9 validation cases, bbox 422 (fragment AND full
page, fragment asserted not to be a full document), photo pipeline incl. HEIC wording
and a real >15 MB 413, numbered/unnumbered duplicates incl. nothing-saved and
merge-applies-label, detail 404s, edit (label-only keeps asset+position; duplicate
asset 409), delete (303/404/list-gone/re-delete 404/no-CSRF 403), pagination 50/51
boundary with fragment content split. No mocks, no fixture-value special-casing in
production code, no skips added. The `e2e-task` no-op matches the packet ("inert
drafts — T-207 activates them") and the durable mobile-project Playwright coverage is
sanctioned to arrive with T-207; this task's mobile evidence is the deleted-per-packet
scratch battery whose artifacts I verified on disk (16/16 dirs + `.last-run.json`
passed).

## Findings

1. **[High — planner-bound, disclosed] Merge applies the label but not the photo.**
   The packet freezes `dup_choice=merge → "apply photo/label onto ExistingID"`
   (`harness/tasks/T-205.md:67`) and the contract table repeats "label/photo"
   (`acceptance/drafts/README.md:68`). `mergeDuplicate` calls only
   `EditLight(ctx, existing.ID, sub.Label, existing.AssetNo)`
   (`internal/web/admin_create.go:310`); `EditLight` has no photo parameter
   (`internal/store/lights.go:256`) and `internal/store` is outside this task's owns —
   I confirmed there is no in-owns path to set `photo_id` (create-then-soft-delete
   would violate the frozen "no new row" and change the id). The photo uploaded with a
   merge submission is persisted at `admin_create.go:185` (frozen order: photo
   validation precedes duplicate classification) and then orphaned. The frozen oracle
   asserts only the label (both draft suites merge without a photo part), so the gate
   passes. This is a genuine product gap that ONLY the planner can resolve (an
   `EditLightPhoto`-style store task, or ratify the re-scope); the handoff discloses
   it as open issue 2. Not reward hacking — the omitted work is architecturally
   impossible here and the omission is documented — but the handoff's "Status:
   COMPLETE" overstates the deliverable; per AGENTS.md the clean state was
   `BLOCKED:` for an owns extension.
2. **[High — planner-bound, disclosed] `pages/admin-login.html` is dead code; the
   live login page is still T-203's unstyled inline template.** The packet asks for
   the template to "replace T-203's minimal page" (`harness/tasks/T-205.md:76-77`)
   while the same packet forbids editing `auth.go` (`:42`), whose
   `renderLoginPage` (`internal/web/auth.go:170-179`) serves the live page — and
   ServeMux panics on a duplicate exact pattern, so no override route is possible.
   Packet-internal contradiction, resolved the only way owns allows (template
   created, frozen strings kept, `TestRenderAll` renders it, live page untouched and
   still satisfying A-201's frozen markup). A-201 is T-203's claim, not T-205's, and
   its assertions pass; the A-201 *visual* note (44 px tap target) remains unmet —
   correctly routed (handoff open issue 1, with the exact 3-line fix for whoever owns
   `auth.go`).
3. **[Medium] Full-page re-renders lose the typed asset number.** `walkPageDataOf`
   builds `walkValues` without `AssetNo` (`internal/web/admin_create.go:377-390`)
   while the template renders `value="{{.Values.AssetNo}}"`
   (`web/templates/pages/admin-walk.html:56`) — after a 409/422/415 no-JS re-render
   the admin's asset number is gone (label/kind/lat/lon survive). One-line fix plus a
   test; the HX path (the real product path) is unaffected.
4. **[Medium] `walkReject` on the parse-failure path renders an unusable form.**
   With `sub == nil` (`internal/web/admin_create.go:364-367`) the page carries no
   CSRF token (`value=""` at `admin-walk.html:36`) and no bbox (`data-bbox=""`,
   which also kills `walk.js`'s client pre-flight, `walk.js:53-58`). Reachable via
   malformed multipart / the 413-before-parse path for non-HX clients. Mint a token
   + bbox as `walkPageDataOf` does, or answer a plain status page.
5. **[Medium-low] Numbered duplicate-panel content assertions are weak.**
   `admin_test.go:486` checks the 409 body contains `"Ball field lamp"` and
   `"GP-017"` — but the submitted label "Ball field lamp twin" is re-rendered into
   the form input (`admin-walk.html:54`) and matches as a substring, and "GP-017"
   likewise appears in the asset input (`:56`). A panel that omitted the existing
   light entirely would still pass. The exact-alt merge assertions
   (`admin_test.go:517-521`) are well-anchored; strengthen the panel case to assert
   text unique to the panel (e.g. `asset number GP-017` / `m away` inside
   `#dup-panel`, or the exact `partials/admin-dup.html` fragment for the HX path).
6. **[Low] `TestAdminWalkPage` claims more than it checks.** The comment says the
   rendered csrf input "matches the cookie" (`admin_test.go:236-237`) but the code
   only proves a cookie exists and the input has *a* value (`:226-241`) — no
   comparison. Parse both and compare; the frozen drafts depend on the double-submit
   pair matching.
7. **[Low] The no-JS pager lands on page 1.** `method="get"` +
   `formaction="/admin/rows?page=N"` with zero form controls
   (`partials/admin-rows.html:15-18`, mirrored in `pages/admin-list.html:21-24`): a
   native GET submission replaces the action URL's query with the (empty) form data,
   so the no-JS fallback hits `/admin/rows` (page 1) and the 303 goes to
   `/admin?page=1`. The frozen A-209 path is the htmx one (which passes the query
   verbatim and works, asserted no-navigation in the scratch suite); a hidden `page`
   input or plain links would fix the fallback. Not contract-asserted anywhere.
8. **[Low] `mergeDuplicate`'s vanished-existing fallback drops the uploaded photo and
   500s on a duplicate race** (`internal/web/admin_create.go:290-308`: `CreateLight`
   without `PhotoID`, and an `ErrDuplicateAssetNo` there falls through to 500).
   Edge case (the duplicate was deleted between check and merge).
9. **[Low] `classifyDuplicate` fails open** on a `NearbyLights` DB error
   (`internal/web/admin_create.go:264-268`: logs and proceeds to create). Single-admin
   scale; a DB failure will surface on the create anyway.
10. **[Info] Forced template duplication carries a drift risk.** The rows fragment
    exists twice (`pages/admin-list.html:7-26` define vs `partials/admin-rows.html`)
    and the dup panel likewise (`admin-walk.html:40-52` vs `partials/admin-dup.html`)
    because `templates.go:48-53` parses pages without partials and `templates.go` is
    outside owns. Both copies verified in sync at review time; the `<!-- sync -->`
    comments are the mitigation. A future task could let pages include partials.
11. **[Info] Stale scratch-failure artifacts under `web/test-results/`** (gitignored,
    so guard-clean): `walk--scratch-A-202-*/error-context.md` etc. document an
    earlier harness iteration's failure (the manual-position hijack since fixed at
    `walk.js:126-130`). Not mentioned in the handoff; harmless, but the handoff's
    screenshot inventory should have noted that only the root `test-results/`
    battery is the final evidence.
12. **[Info — correction to reviewer-alt] Finding 3 in
    `harness/reviews/T-205-reviewer-alt.md` overstates.** The claim "a regression
    that renders two rows per page instead of fifty would stay green" is false:
    `admin_test.go:726-728` requires BOTH `Boundary lamp 01` and `Boundary lamp 50`
    on page 1, which a 2-rows-per-page pagination fails. The middle-48 labels are
    indeed not individually asserted (worth tightening to match the frozen draft's
    full loop), but the 50/51 boundary itself is pinned by the extremes plus the
    page-1/page-2/fragment content split.
13. **[Info] Edit test narrower than A-208's letter**: longitude and stored-photo
    survival after a label-only edit are not asserted locally
    (`admin_test.go:613-641` asserts label/asset/latitude only); the frozen Go draft
    covers both at T-207 activation. Cheap to add.

## Verdict rationale

Against the four assertions T-205 claims (A-202/A-203/A-208/A-209) the implementation
is correct and honestly tested: I traced every frozen clause in the drafts to its
implementing line and found no divergence; security and architecture are clean; the
evidence on disk is real and green. The two unmet packet clauses (merge-photo,
login-page wiring) are both impossible within `owns` — I verified the store API and
the ServeMux constraints directly — and both are disclosed in the handoff with
concrete remediations for the planner; neither is exercised by any frozen, gated
assertion. That matches this repo's review precedent (T-203's dual-encoding
deviation: sound within owns, disclosed, "not a block — the planner should resolve
it"). I do not accept reviewer-alt's "reward hacking" framing: the omitted work is
structurally out of scope, not dodged, and nothing was special-cased to a test.
However, the rework that reviewer-alt's BLOCK already triggers should take the cheap
in-owns fixes above (findings 3-7, 13) and re-state the two planner-bound gaps as
explicit `BLOCKED:`-style escalations rather than "COMPLETE with open issues" — and
the planner must act on findings 1 and 2 before M2 closes, or the merge-photo half of
A-205's written contract will ship silently unimplemented.

VERDICT: PASS
