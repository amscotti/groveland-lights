# T-205 review (reviewer, round 4) — Admin UI: walk-the-town flow, light CRUD, list + HTMX pagination

Reviewed: the attached diff (the round-4 retry) against `harness/tasks/T-205.md`,
`acceptance/drafts/{README.md,admin-walk.spec.ts.txt,admin_lights_go_test.go.txt,
admin_auth_go_test.go.txt}`, the frozen packages it builds on (`web.go`, `csrf.go`,
`auth.go`, `session.go`, `upload.go`, `templates.go`, `probe.go`, `context.go`,
`internal/store/*`, `internal/geo/*`, `internal/config`), and
`docs/spec/architecture.md`. Every file in the diff was re-read on disk and matches
the diff byte-for-byte in the sections cited below. This review supersedes the
round-3 `T-205-reviewer.md` (same diff except the `internal/store` revert and the
new merge-branch/panel-fragment tests, both re-verified here from scratch).
Every frozen symbol the new code calls (`AdminOnly`, `IssueCSRFToken`, `VerifyCSRF`,
`CSRFTokenName`, `LimitBody`, `s.Photo`, `PhotoProblem`, `MaxPhotoBytes`,
`renderPartial`/`renderPage`, `RecentLights`, `GetLight`, `LightByAssetNo`,
`EditLight`, `SoftDeleteLight`, `NearbyLights`, `CreateLight`, `PhotoByID`,
`ErrNotFound`, `ErrDuplicateAssetNo`, `store.ParseKind`, `geo.ClassifyDuplicate`,
`geo.DupDecision`, `geo.NumberedRadiusM`, `geo.DistanceM`, `config.BBox.Contains`)
exists with the assumed signature. The on-disk `walk.js` is exactly 200 lines; the
vendored `htmx-2.0.11.min.js` default `responseHandling` was checked against the
override. Caveats: this sandbox rejects `go`/`git`/`make`, so execution claims
(`make check-fast`, `templates-check`, `js-check`, `e2e-task` exit 0) are the
handoff's; and this reviewer instance has no image input, so the 8 kept
screenshots were not re-inspected here (the round-4 visual-reviewer PASS, which I
read in full, covers them).

## Round-4 delta: every prior BLOCK/fix re-verified as genuinely fixed

1. **No harness deletion.** The three `harness/reviews/T-205-*.md` files are
   present and unmodified; the handoff makes no cleanup claim. (reviewer-alt
   BLOCK #1 — resolved.)
2. **Bbox alert assertion is not vacuous.** `alertWithMessage`/`assertVisibleAlert`
   (admin_test.go:186-216) extract the `<article>` that carries the frozen message
   and prove it is `role="alert"` and not `hidden`, in BOTH the full-page 422 and
   the HTMX fragment (admin_test.go:529, 554-561). The walk page's permanently
   hidden empty `#walk-alert` (pages/admin-walk.html:8) can no longer satisfy the
   check. (BLOCK #2 — resolved.)
3. **`TestRenderAll` fails on failed render.** web_test.go:176-187 checks
   `renderPage`'s error per page and rejects an empty document before snapshotting;
   a missing fixture is an explicit error (web_test.go:170-173). (BLOCK #3 —
   resolved.)
4. **Dead CSRF branch removed.** `csrfInputValue` returns `(value, ok)`
   (admin_test.go:172-180); callers assert `ok` (admin_test.go:520-522, 576-578),
   and `TestAdminWalkPage` compares the form token against the Set-Cookie value
   (admin_test.go:617-631). (Additional #4 — resolved.)
5. **Pager assertions scoped to the pager form.** `pagerFormOf` extracts the
   `method="get" action="/admin/rows"` form carrying the Older/Newer button, then
   `assertPagerPageInput` proves the hidden `page` input AND the matching `hx-get`
   are inside that form (admin_test.go:239-266, used at 1135, 1141, 1149).
   (Additional #5 — resolved.)
6. **Anonymous edit/delete routes covered.** admin_test.go:349-364 POSTs
   `/admin/lights/1` and `/admin/lights/1/delete` without a session and asserts the
   302 to `/admin/login`. (Additional #6 — resolved.)
7. **Merge-write branches covered.** `TestMergeAttachPhotoBranches`
   (admin_test.go:811-880) covers unset `DB_PATH`, `?`/`#`/`%` DSN-ambiguous
   paths, unknown id → `ErrNotFound`, soft-deleted id → `ErrNotFound`, and a
   canceled context surfacing a DB error (not a masquerading `ErrNotFound`).
   (Additional #7's minimum — met.)
8. **Guard ownership restored.** No `EditLightPhoto` remains under
   `internal/store` (grep-verified); the only mention is admin_merge.go:7's
   follow-up note. The diff touches exclusively owns-glob files
   (`internal/web/admin*.go`, `web_test.go`, `web/templates/**`, `walk.js`,
   `globals.d.ts`) plus the handoff.

## Correctness against the packet and the frozen drafts — verified

- All seven routes registered via `register(route{...})` from this task's own
  `init()`s, every one wrapped in `AdminOnly` (admin.go:22-30, admin_create.go:22-24).
  `base.html`, `pages/index.html`, `partials/probe.html` untouched; template
  inventory matches the packet (4 flat admin pages + 3 partials; no `scratch/`).
- Frozen check order parse → CSRF → bbox → photo → duplicates implemented verbatim
  (admin_create.go:170-211); `LimitBody(r)` is the first body-touching call; the
  single multipart parse happens in the boundary parser and `s.Photo(r)` reuses
  `r.MultipartForm` exactly per upload.go:94-108's documented contract.
- Frozen responses all present and asserted: 422 "That position is outside
  Groveland town bounds" in a visible role=alert; 409 `#dup-panel` (heading
  `Possible duplicate`, EXISTING light's label/asset/distance); numbered = Merge +
  Cancel link, no Save anyway (numbered+save → 422); unnumbered = Merge + Save
  anyway; merge applies label AND photo with no new row; success = role=status
  `Saved` fragment for HX / 303 `/admin` otherwise; edit → 303 detail; delete →
  303 `/admin`; `#light-pos` at 6 dp (`posString`, admin.go:309-311); `/admin/rows`
  fragment with root `id="rows"` only under `HX-Request`, else 303 `/admin?page=N`
  (admin.go:196-198); 50/page list with the Older detector at exactly 51
  (admin.go:144-165); alt format `"<asset_no> <label>"` / `"<label>"`
  (admin.go:92-97) matching the README's frozen table; kind select offers
  park|building only with street 422'd (admin_create.go:102-107, reconciliation 3);
  list ordering delegated to `RecentLights` (`created_at DESC, id DESC`).
- The `installLoginPage` hook (admin.go:37-47) replaces auth.go's `loginPageTmpl`
  var without editing the frozen file — the move auth.go:143-146 itself anticipates.
  Go's init ordering (package var initializers before any `init()`) makes the
  override sound; the styled page keeps all four frozen markup elements, which I
  checked against BOTH auth_test.go:29-32's regexes and the A-201 draft's
  (aaReHeading/aaReLabel/aaReInput/aaReButton) — both suites keep passing.
- `walk.js`: 6-dp readouts; accuracy>30 gate using the platform `disabled`
  attribute (walk.js:94-98, exactly what e2e asserts); ±25 m pointer drag + 1 m
  arrow nudges on a focusable `#pin` (tabindex=0, aria-label="position pin");
  manual fallback on denied geolocation before any fix (walk.js:126-130);
  object-URL photo preview (`#photo-preview`, src changes per pick); htmx 4xx-swap
  override (walk.js:180-188) whose shape I verified against the vendored
  htmx-2.0.11 default (`[45]..`: `swap:false,error:true` → `swap:true,error:false`);
  `// @ts-check`, ES module, no `console.log`, exactly 200 lines (the ceiling).
- Deviation from the letter of deliverable 3, judged equivalent: the packet asked
  for a `<meta name="htmx-config">`; instead `walk.js` overrides
  `htmx.config.responseHandling` before htmx's DOMContentLoaded init (deferred
  classic htmx in `<head>`, module script in `<body>`, both pre-DOMContentLoaded).
  Functionally identical for this key; base.html's frozen meta is untouched.
- No-JS flows hold together: each pager control is its own GET form with the page
  number in a hidden input; error re-renders unhide `#add-form` when an alert or
  dup panel is present (pages/admin-walk.html:33); the dup panel's Merge/Save
  buttons are in-form submits carrying `dup_choice`.

## Security — verified

- **CSRF**: create/edit/delete verify the double-submit pair after the boundary
  parse and before any state change (admin_create.go:176-179, admin.go:443-446,
  522-525); missing and mismatched tokens are 403-tested for all three. Parsing
  precedes CSRF (frozen order) but is size-bounded (15 MB create via `LimitBody`;
  1 MB `MaxBytesReader` on edit/delete, admin.go:398-414).
- **XSS**: no `template.HTML` in the new code. `template.URL` appears only for the
  constant data-URI placeholder and `/photos/` + server-minted paths with reasoned
  `//nolint` on the same line (admin.go:72, 102-105) — see finding 3 below for the
  minor widening. All user text (labels, asset numbers, alerts, panel content, alt
  text) goes through html/template auto-escaping; `Status`/`Kind` in class
  attributes are DB-CHECK-constrained values.
- **SQL**: the one raw statement (admin_merge.go:65-69) is fully parameterised; no
  string interpolation anywhere.
- **Path traversal**: nothing new touches the filesystem from user input;
  `/photos/` serving stays behind upload.go's clamped resolver.
- **Upload limits**: `LimitBody` first; oversize body → 413 via `MaxBytesError`
  through `PhotoProblem` at the parse (admin_create.go:64-73), tested with a 16 MB
  body; HEIC/text/pixelbomb 415s tested with the frozen HEIC wording.
- **Secrets**: none in responses or logs; `withLogging` (frozen) logs no paths or
  coordinates; `http.Error` bodies are constants.
- Redirects are fixed targets (no open redirect); anon access is 302/401+HX-Redirect
  per the frozen guard, tested including the edit/delete POSTs.

## Architecture

- **[Major — not a blocker this round; integrator follow-up REQUIRED] Raw SQL in
  `internal/web` (internal/web/admin_merge.go:50-81).** The ownership map gives
  `internal/store` "SQLite access …, all queries" (docs/spec/architecture.md:18)
  and store.go:1-4 repeats it ("Package store owns the SQLite database: … and
  every query"). `attachPhoto` opens its own `database/sql` pool per merge call
  (DSN duplicated at admin_merge.go:54-57), blank-imports the driver
  (admin_merge.go:18), and runs `UPDATE lights SET photo_id…, updated_at… WHERE
  id=? AND deleted_at IS NULL` — schema knowledge now living in two packages, so a
  future migration silently breaks this path as a 500. This is a real ownership-map
  deviation and must be absorbed into the store (`store.EditLightPhoto(ctx, id,
  photoID)`) by a store-owning task before M2 close; the planner should also note
  the packet defect that produced it (a frozen merge-photo clause with no frozen
  write API). Why not a BLOCK, after weighing it independently: (a) the packet
  (T-205.md:67-68) AND the frozen README table (README.md:68) both freeze "apply
  photo/label onto ExistingID, no new row" — the only in-scope alternatives each
  break a different frozen clause (delete+recreate breaks "no new row"/ExistingID;
  label-only merge breaks the contract table); (b) the store route is provably
  outside `owns` — round 3 demonstrated the guard rejects it (exit 4), and this
  retry correctly reverted rather than special-casing the guard; (c) the workaround
  is minimal, parameterised, `deleted_at IS NULL`-guarded, mirrors the store's DSN
  pragmas and RFC3339 `updated_at` exactly (store.go:28-33, lights.go:260), maps
  0 rows to `store.ErrNotFound` with every driver failure wrapped as a DB error,
  validates the DSN path the way `store.checkDSNPath` does, and is branch-tested;
  (d) it is disclosed in the file header and the handoff with the named follow-up.
  Related follow-up noted by round 3 and still true: the merge is two transactions
  (EditLight at admin_create.go:311 then attachPhoto at admin_create.go:316-322),
  so a failure between them leaves the label applied without the photo (500, admin
  retries; single-admin scale, acceptable).
- **HTMX/map rule** ✓: no new template contains `id="map"` (grep-verified); the
  only new hx-targets are `#rows` (pages/admin-list.html:22-23,
  partials/admin-rows.html:16-17) and `#dup-slot`
  (pages/admin-walk.html:35) — none is `#map` or an ancestor of it; the admin pages
  contain no map at all.
- **Parse-don't-validate** ✓: every boundary parse yields a typed struct
  (`walkSubmission`, `lightEdit`, `lightDelete`, `listQuery`, `lightIDParam`);
  no `r.FormValue`/form access after parsing anywhere in the new code.
  **Typed IDs** ✓: `store.LightID`/`store.PhotoID` throughout; bare `int64` only
  where the frozen `geo.DupDecision.ExistingID` forces it, converted immediately
  (admin_create.go:290, 329).
- Feature registration via `init()` in feature files; owns globs respected.
  No new module dependency (`modernc.org/sqlite` is already in go.mod; see finding
  1 for why the import is still regrettable). No "Phase N" strings, TODO/FIXME,
  panics, or `console.log` (grep-verified).

## Tests — genuinely exercise the behaviour

- Everything drives the full middleware chain (`s.Handler()`) against a real
  temp-file store with a minted admin session and matching CSRF pair — production
  wiring. Coverage verified on disk: guard matrix (anon 302/401+HX-Redirect,
  including anon edit/delete POSTs), styled login page, walk page (incl. form
  token == cookie), empty list, rows 303/fragment-root, create happy path + HX
  status fragment, a 9-case validation table, outside-bbox re-render (values kept,
  form unhidden, fresh token, visible role=alert in both transports), photo
  pipeline (happy/HEIC/text/pixelbomb/413 over the real multipart path), numbered
  dup (409 → nothing-saved → 422 → merge label), panel content extracted from the
  exact `#dup-panel` in BOTH the full page and the HTMX fragment (no echo of the
  submitted twin label, no Save-anyway), merge-photo over real HTTP (one row
  survives, photo URL changes, old and new files serve 200 — this exercises
  admin_merge.go end to end), the merge-write branch tests, unnumbered dup
  (panel/save-anyway/beyond-8 m), detail (200/404/non-numeric 404), edit
  (label-only keeps asset, 6-dp position AND photo by URL; dup-asset 409; CSRF
  403), delete (303/404/gone/idempotent-404/CSRF 403), pagination 50/51 with
  exact row counts on page 1, page 2 and the fragment, plus the pager-form
  structure assertions.
- `TestRenderAll` now fails on a missing fixture, a render error, or an empty
  document (web_test.go:169-187) — the packet's per-page fixture ask, with the
  round-3 vacuity closed.
- Remaining gaps (minor, none contract-bearing): the lost-race
  `ErrDuplicateAssetNo` re-route (admin_create.go:221-232) and
  `classifyDuplicate`'s `NearbyLights`-error path are untested through HTTP (hard
  to trigger); `partials/admin-dup.html`/`admin-alert.html` are not rendered by
  `TestRenderAll` (pages only), so html-validate never sees the exact fragments
  the HTMX path serves — they are close copies of validated page markup kept in
  sync by hand (finding 5).

## Minor findings / notes

1. `classifyDuplicate` fails OPEN on a `NearbyLights` store error
   (admin_create.go:264-268): a transient DB error silently skips the unnumbered
   duplicate check and the create proceeds (numbered dups stay backstopped by the
   partial unique index). Prefer failing closed (500) so a duplicate row is never
   created without the panel.
2. `*sub.AssetNo` at admin_create.go:224 is dereferenced only after
   `ErrDuplicateAssetNo`, which the partial unique index (0002_lights.sql:39)
   guarantees implies a non-nil asset — safe by invariant, but a nil guard would
   make it locally robust; the fallback `respondDuplicate` there would also 500 if
   its `GetLight` fails.
3. `template.URL` for the `/photos/` path (admin.go:105) is an unnecessary
   widening — a plain relative-URL string passes html/template's filter; only the
   data-URI placeholder needs the type. Values are server-minted
   (upload.go:222-242), so no user input can reach it today.
4. Duplicated markup kept in sync by `<!-- sync: -->` comments only: the
   rows/pager block exists twice (pages/admin-list.html:7-26 vs
   partials/admin-rows.html:3-20) and the dup panel/alert inline copies
   (pages/admin-walk.html:40-52) mirror partials/admin-{dup,alert}.html. This diff
   already had to apply the pager fix twice; drift risk for every future edit.
5. The frozen check order persists the photo BEFORE duplicate detection
   (admin_create.go:185-196), so an abandoned 409 panel orphans a photos row+file
   — packet-mandated; future retention job's problem.
6. `parseListQuery` has no upper bound on `page` (huge offsets render an empty
   page) — harmless.
7. Visual evidence: the 8 kept PNGs exist under `test-results/`; I could not view
   them (no image input this session), so the visual claims rest on the round-4
   visual-reviewer PASS (which independently flagged and accepted the two
   disclosed deviations: the blue-toned Delete button and the sub-44 px Cancel
   link, both forced by `app.css` being outside `owns`/the frozen link contract).
8. Execution claims (all exit 0) are the handoff's; this sandbox cannot run
   `go`/`git`/`make`. Everything below was verified by reading, and the code
   type/symbol-checks against the frozen packages by inspection.

## Verdict

Round 4 does exactly what a retry should: it reverts the guard-violating
`internal/store` edit (proving the ownership boundary holds), keeps the
in-scope merge write confined to an owned file with branch tests, and every
remaining item from the two prior review cycles is fixed for real — the alert
assertions extract the visible element, `TestRenderAll` polices its own renders,
the pager assertions are form-scoped, the anon edit/delete routes are covered,
and the harness artifacts are intact. No correctness, security, or test-integrity
defect remains that I can find. The single architecture deviation (raw SQL in
internal/web) is real and must be tracked to a store-scoped absorption
(`store.EditLightPhoto`) before M2 close, but it is the minimal, parameterised,
tested and disclosed implementation of a frozen contract clause that the frozen
store API cannot express and that this task may not add to the store — re-blocking
it would demand the worker either break a different frozen clause or write
"BLOCKED" over a behaviour two prior reviews already adjudicated as
follow-up-not-blocker.

VERDICT: PASS
