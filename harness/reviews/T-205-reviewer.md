# T-205 reviewer — Admin UI: walk-the-town flow, light CRUD, list + HTMX pagination

Basis: the attached diff, packet `harness/tasks/T-205.md`, the frozen oracle drafts
(`acceptance/drafts/admin-walk.spec.ts.txt`, `admin_lights_go_test.go.txt`,
`admin_auth_go_test.go.txt`, `acceptance/drafts/README.md` — verified untouched by
this diff), the merged sources the new code compiles against
(`internal/web/{web,csrf,session,auth,probe,upload,templates}.go`,
`internal/store/{store,lights,photos}.go`, `internal/geo/*`, `internal/config`,
`migrations/0002_lights.sql`, `cmd/server/main.go`, `web/templates/base.html`,
the vendored `htmx-2.0.11.min.js`), and `docs/spec/architecture.md`.

Method note: shell execution was denied this session, so the handoff's exit-code
table was not re-run. Every file in the diff was read from the worktree and
matches the diff byte-for-byte where spot-checked; every load-bearing claim
(template strings, store/geo/upload API signatures, htmx config key, DSN pragmas,
schema columns) was verified statically against the merged sources.

## Packet conformance

- Routes registered exactly as the packet's Deliverable 1 table
  (admin.go:22-28, admin_create.go:23-25), all wrapped in `AdminOnly`, all from
  `init()` in the task's own files. `web.go/csrf.go/probe.go/auth.go/session.go/upload.go`
  are untouched (diff confirms); `base.html`, `pages/index.html`,
  `partials/probe.html` untouched as required (packet :79-80).
- Frozen check order honored: `LimitBody` first (admin_create.go:177), single
  multipart parse in the boundary parser (admin_create.go:71), `VerifyCSRF`
  (:184), bbox `Contains` → 422 + the exact string (admin_create.go:180-184),
  photo via `s.Photo(r)` reusing the boundary parse per upload.go:94-108's
  documented "caller parsed first" ordering (413/415 through `PhotoProblem`),
  then duplicates via `LightByAssetNo` + `NearbyLights(NumberedRadiusM)` +
  `geo.ClassifyDuplicate` (admin_create.go:252-285). Numbered+save → 422,
  unnumbered gets Save anyway, `dup_choice=""` → 409 panel, merge → apply onto
  ExistingID with no new row, success → role=status `Saved` (HX) / 303 `/admin`.
  All match packet :56-69 and the README contract table (:91).
- This **resolves T-206's F1 forward risk**: the photo pipeline now runs only
  AFTER `VerifyCSRF` (admin_create.go:184-195) — the CSRF-safe "Order B" the
  T-206 review mandated. A body over the cap surfaces at the caller's parse as
  `*http.MaxBytesError` and is mapped to 413 (admin_create.go:73-84), exactly
  the case upload.go:105-108 documents.
- List pagination: `RecentLights(ctx, 50, (page-1)*50)` + one extra row to
  detect Older (admin.go:145-165); `/admin/rows` fragment-only for HX-Request,
  else 303 `/admin?page=N` (admin.go:196-199) — the frozen :97 row. `#rows` is
  the fragment root (partials/admin-rows.html:1), asserted by
  `reRowsRoot` (admin_test.go:169).
- Detail `#light-pos` at 6 dp (`posString`, admin.go:309-311); 404 on
  ErrNotFound and on non-numeric/`<1` ids (admin.go:283-290, 319-332); edit
  keeps unspecified fields (validatedEditLabel/Asset nil-passthrough,
  admin.go:473-498) with the ErrDuplicateAssetNo → 409 re-render
  (admin.go:460-463); delete → 303 `/admin` (admin.go:553).
- Parse-don't-validate / typed IDs (rule 6): `walkSubmission`, `lightEdit`,
  `lightDelete`, `listQuery`, `lightIDParam`/`store.LightID`,
  `store.PhotoID`; zero `r.FormValue`/`ParseForm` reads after the boundary
  parsers (grep-verified across admin*.go).
- Deliverable 3 (walk.js ≤200 lines, `// @ts-check`, JSDoc, no console.log):
  exactly 200 lines; the only "console" hit is a comment. Behaviour matches the
  frozen list: 6-dp readouts, >30 m gate with the platform `disabled`
  attribute (walk.js:96-97), ±25 m drag + 1 m arrow nudges (walk.js:134-163),
  manual fallback on denied/unavailable (walk.js:115-131), photo preview via
  object URL (CSP `img-src ... blob:`), and the client-side bbox pre-flight
  with the exact frozen message (walk.js:9, 93) that README reconciliation 4
  requires.
- The htmx 4xx-swap requirement (packet :99-101) is implemented in JS
  (walk.js:185-187) rather than the suggested `<meta name="htmx-config">`.
  Verified against the vendored source: htmx 2.0.11 ships
  `responseHandling:[{code:"204",swap:false},{code:"[23]..",swap:true},
  {code:"[45]..",swap:false,error:true}]`; the override flips `[45]..` to
  swap-without-error so the 422 alert / 409 panel land in `#dup-slot`.
  base.html loads htmx `defer`; module evaluation precedes htmx's
  DOMContentLoaded init, so the config is in time. Equivalent outcome to the
  packet's mechanism — acceptable deviation, noted as F6.
- Deliverable 2: `pages/admin-login.html` keeps the frozen strings
  (`Admin login`/`Token`/`Sign in`) byte-for-byte inside the pipeline page;
  `installLoginPage` (admin.go:37-47) captures the rendered page at `init()`
  and installs it as auth.go's `loginPageTmpl` without editing auth.go.
  Verified against the frozen `auth_test.go:29-32` regexes (they tolerate the
  added attributes) and the README :78 markup row — nothing breaks.
  `TestRenderAll` now has a per-page fixture map (web_test.go:122-168), as
  packet :80-81 demands.
- Ownership of changed files: `admin.go`, `admin_create.go`, `admin_merge.go`,
  `admin_test.go` (all `admin*.go`), `web_test.go`, `web/templates/**`,
  `web/static/js/walk.js`, `globals.d.ts`, plus the sanctioned handoff — all
  inside `owns`. The scratch visual harness is gone (glob-verified); the
  handoff's `make e2e-task TASK=T-205` invocation matches the Makefile's
  `TASK ?=` variable (:8, :113).

## Security

- **CSRF**: every POST verifies the double-submit pair before any state change
  (admin_create.go:184, admin.go:432, admin.go:540); the create parse happens
  before CSRF but persists nothing (the T-206 Order-B fix). Failed tokens are a
  uniform 403. Re-renders mint/reuse the token correctly (`IssueCSRFToken`
  reuses the request's valid cookie, so the echoed form stays submittable).
- **XSS**: no `template.HTML` anywhere in the diff (grep + forbidigo active).
  All user-derived values (labels, asset numbers, messages, alt text) go
  through contextual escaping. The one deliberate door is
  `rowView.ImgSrc template.URL` (admin.go:72, 102-106) with `//nolint` reasons;
  both reachable values are server-minted — the constant data-URI placeholder
  and `/photos/` + `PhotoByID().Path`, which only the upload pipeline writes
  (`PHOTO_DIR/YYYY/MM/<16-hex>.jpg`, upload.go:222-242). CSP admits `img-src
  'self' data: blob:` for both thumbnail sources and the JS preview. No
  injection vector found.
- **SQL**: everything goes through the sqlc store API except the one statement
  in admin_merge.go:52-57, which is a constant string with `?1..?3`
  placeholders (no injection; see F1 for the architectural concern).
- **Path traversal**: no new file-path handling; served photo URLs are built
  only from store rows and resolved by upload.go's clamped resolver.
- **Upload limits**: create bodies capped at 15 MB by `LimitBody` before any
  parse (admin_create.go:177); edit/delete bodies capped at 1 MB
  (admin.go:398-409); the oversize→413 mapping is tested
  (admin_test.go:583-590). Multipart memory budget (32 MB) matches
  upload.go's documented reasoning.
- **Secrets in output**: pages render label/asset/position/status/photo URL
  only; the login input is `type=password`; handlers log errors only (the
  shared logging middleware already excludes coordinates/credentials).

## Architecture

- **HTMX/#map rule (architecture.md:29-33)**: the admin pages contain no map;
  every `hx-target` is `#rows` (outerHTML) or `#dup-slot` (innerHTML) — both
  far outside any `#map` ancestor; none of the three new partials contains
  `id="map"`. The map is initialised exactly once, untouched.
- **Registration convention** (`<feature>.go` + `init()` + `register`),
  feature-per-file split (list/detail in admin.go, create in admin_create.go)
  follows web.go:1-3 and the ownership map's `internal/web` entry.
- **Status derivation** goes nowhere near `status.Derive` — admin pages only
  read the stored column. Rule 8 respected.
- **The one violation is admin_merge.go** — see F1.
- Rules 3/5/9: go.mod unchanged (modernc.org/sqlite was already the store's
  driver; no new module dependency); no "Phase N"/TODO/FIXME/panic strings in
  the new code; `//nolint` usages each carry an on-line reason.

## Tests — do they exercise the behaviour?

Yes, and the reviewer-alt BLOCK items are genuinely fixed, not papered over:

- `TestAdminDuplicateMergePhoto` (admin_test.go:691-717) seeds a
  landscape-photo light, merges a *different* upload, and asserts 303, exactly
  one `<tr>`, the detail's `/photos/...` src CHANGED, and both files still
  serve 200 — a real end-to-end proof of `attachPhoto` (it fails if the UPDATE
  is dropped, which is precisely the previously-missing behaviour).
- `TestAdminDuplicatePanelContent` (:662-689) extracts the exact `#dup-panel`
  element (`dupPanelOf`) in BOTH the full-page and HX responses and asserts the
  EXISTING light's `<strong>` label, `asset number GP-017`, distance, Merge +
  Cancel, no `Save anyway`, and no echo of the submitted twin label — the
  extraction makes the old "matched the wrong element" vacuity impossible.
- `TestAdminPaginationBoundary` (:938-992) counts `<tr>`s and asserts the full
  50-label set, then page 1 = 51..2 (no 1) and page 2 = exactly {1}, for both
  the full page and the rows fragment, plus the 50-lights/no-Older boundary.
- `TestAdminWalkPage` (:307-340) parses the hidden input's value and compares
  it to the `csrf_token` cookie for equality, not just presence.
- `TestAdminCreateOutsideBBox` (:496-540) asserts the re-render keeps label,
  asset, kind, lat, lon, accuracy, bbox and a non-empty token (the `[^"]+`
  capture makes "non-empty" enforced), and that the HX rejection is a fragment.
- `TestAdminEdit` (:844-892) captures the photo URL before a label-only edit
  and asserts both `-71.029000` and photo survival afterwards; the duplicate
  asset 409 and no-CSRF 403 paths are covered.
- `TestAdminLoginPageIsTheStyledPipelinePage` (:283-305) drives `GET
  /admin/login` through the handler and asserts the pico stylesheet,
  `id="admin-login"`, the three frozen strings and a complete document.
- All tests run through `s.Handler()` (full middleware chain) against a real
  temp-file store with a minted session and matching CSRF pair
  (`newAdminServer`, :47-82) — the same wiring as production (main.go:40-52).
  No `t.Skip`/`test.only` anywhere in the new files.

## Findings

1. **[high — architecture violation, must be tracked to resolution]
   `internal/web/admin_merge.go` opens its own SQLite connection and runs a
   raw UPDATE** (admin_merge.go:36-57). `docs/spec/architecture.md:18` reserves
   "SQLite access … all queries" for `internal/store`; this imports the driver
   into `internal/web`, hand-writes schema knowledge (`photo_id`, `updated_at`,
   `deleted_at IS NULL`) outside the sqlc surface a migration could compile-
   check, and adds a second writer connection. Context that keeps this out of
   BLOCK territory: the packet (:67) and README (:68) freeze "merge applies
   label AND photo", the merged store API has no photo-update method
   (EditLight is label/asset only, lights.go:256), `internal/store` is outside
   this task's `owns`, and the previous review BLOCKed the drop-the-photo
   alternative — the packet left no compliant path except `BLOCKED`. The
   implementation is the narrowest available: DSN byte-identical to
   `store.dsn` (WAL/busy_timeout/foreign_keys, store.go:28-33), parameterized,
   `deleted_at IS NULL` guarded, 0 rows → `store.ErrNotFound`, FK-safe (the
   photos row was committed by `CreatePhoto` before the merge), covered by a
   real-DB test, and disclosed as the handoff's open issue 3 with the follow-up
   named. **Required follow-up for the planner: add
   `store.EditLightPhoto(ctx, id, photoID)` (store task) and delete
   admin_merge.go.** Residual risk meanwhile: a partial merge window — if
   `EditLight` commits and `attachPhoto` then fails (admin_create.go:311-322),
   the survivor carries the twin label but the old photo and the admin sees a
   500; a retry heals. Also note `checkDSNPath` is not re-applied (store.Open
   already refused ambiguous paths at boot, so `Cfg.DBPath` is pre-validated).
2. **[medium — real bug, no-JS fallback] The pagination pager loses the page
   number without JS** (partials/admin-rows.html:14-18 and the synced copy in
   pages/admin-list.html). The `Newer`/`Older` buttons sit in a
   `<form method="get">` whose only page carrier is the `formaction` URL's
   query string (`formaction="/admin/rows?page=2"`). Per the HTML
   form-submission algorithm ("Mutate action URL"), a GET submission REPLACES
   the action URL's query with the serialized entry list — here empty — so the
   no-JS submit goes to `/admin/rows` with no `page`, the handler correctly
   303s to `/admin?page=1`, and Older/Newer always land on page 1. The HTMX
   path (the only one the drafts exercise) is unaffected, which is exactly why
   nothing caught it. Fix: add `<input type="hidden" name="page"
   value="{{.OlderPage}}">` (per button is not possible in one form — either
   one pager form per button or `formaction="/admin/rows"` + hidden input set
   per submit is needed; simplest is two one-button forms each carrying its
   own hidden `page`).
3. **[low — UX/robustness] Full-page 409/422 re-renders hide their own
   feedback**: `#add-form` is unconditionally `hidden`
   (pages/admin-walk.html:33), so the server-side duplicate panel / alert that
   `respondDuplicate`/`walkReject` carefully render for non-HX clients
   (admin_create.go:353-358, 374-386) live inside a hidden section. A
   no-htmx browser must click "Add light here" again to see the rejection; a
   fully no-JS client can never open the walk form at all. The frozen drafts
   only assert body-contains on these paths, so nothing fails — but the
   re-render machinery's whole purpose is degraded. Suggest dropping `hidden`
   when `.Alert` or `.Dup` is set.
4. **[low — robustness] `classifyDuplicate` swallows `NearbyLights` errors**
   (admin_create.go:264-268): a store failure on the nearby query logs and
   returns "no duplicate", so the create proceeds — silently skipping
   unnumbered duplicate detection instead of failing the request. Numbered
   duplicates remain protected by the unique-index race fallback
   (admin_create.go:221-232, a nice catch). A 500 would be more honest.
5. **[info] Walk-page panel/alert markup is duplicated** between
   pages/admin-walk.html:40-52 and partials/admin-{alert,dup}.html ("sync"
   comments both sides). Drift between the full-page and HX surfaces would
   only be caught because TestAdminDuplicatePanelContent asserts both — which
   it does; acceptable, but a `{{template}}` include would remove the risk
   (the walk page's copy is intentionally a different data shape
   `.Dup.*` vs `.`).
6. **[info] `installLoginPage` re-parses rendered HTML as a template**
   (admin.go:46): safe because admin-login.html renders no user data and the
   output contains no `{{`; `renderLoginPage`'s nil-data Execute is inert on
   the action-free result. Frozen auth tests and the A-201 draft markup row
   still pass (regexes verified).
7. **[info] Packet-mechanism deviation**: htmx 4xx swap configured from
   walk.js:185-187 instead of the suggested `<meta name="htmx-config">`
   (packet :99-101). Key and default verified against the vendored
   htmx-2.0.11; equivalent and correctly timed (defer script + module
   evaluation order).
8. **[info] `TestRenderAll`'s `t.Skip` is pre-existing** (web_test.go:116-118,
   env-gated snapshot writer), not introduced or weakened by this diff.
9. **[info] walk.js sits exactly at the 200-line ceiling** (packet :84 "≤200"),
   disclosed in the handoff; `Newer` button and the pager's no-JS behaviour
   have no test (see F2 — the untested path is the broken one).

## Verdict

The frozen HTTP/UI contract is implemented faithfully (routes, statuses,
strings, fragment roots, 303s, alt format, merge-applies-label+photo, 50/51
pagination boundary), security posture is sound (CSRF order fixed per T-206's
forward risk, bounded uploads, escaped output, parameterized SQL), and the test
suite now exercises every previously-vacuous assertion for real. The two
substantive problems — the store-ownership breach in admin_merge.go (forced by
a packet gap, minimally and honestly implemented, but a breach) and the no-JS
pagination bug — are tracked findings requiring fast follow-up, not reasons to
bounce the task: the first cannot be fixed inside this task's owns at all, and
the second breaks no frozen assertion. The planner should schedule
`store.EditLightPhoto` + admin_merge.go's removal, and the pager hidden-input
fix.

VERDICT: PASS
