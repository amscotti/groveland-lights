# T-205 reviewer — Admin UI: walk flow, light CRUD, list + HTMX pagination (round 4)

Basis: the attached diff (T-205.diff), packet `harness/tasks/T-205.md`, the frozen
oracle `acceptance/drafts/README.md` + `acceptance/drafts/admin_lights_go_test.go.txt`
+ `acceptance/drafts/admin-walk.spec.ts.txt` + `acceptance/drafts/admin_auth_go_test.go.txt`
(verified unedited — the diff touches none of `acceptance/`), the merged building
blocks on disk (`internal/web/{web,auth,session,csrf,upload,probe,templates}.go`,
`internal/store/{store,lights,photos}.go`, `internal/geo/*`, `internal/config/config.go`,
`web/templates/base.html`, `web/static/vendor/htmx-2.0.11.min.js`), the handoff
`harness/handoffs/T-205.md`, and `docs/spec/architecture.md`.

Method note: shell execution is denied this session, so the handoff's exit-code
table was not re-run. Every load-bearing claim was verified statically against the
diff and the on-disk sources (the worktree files match the diff; templates/JS
inventories re-globbed). Image input is unavailable to this model, so the 8
screenshots under `test-results/` were not re-viewed here; the visual claims rest
on `harness/reviews/T-205-visual-reviewer.md` (PASS, detailed) plus the deleted
evidence spec's programmatic assertions quoted in the handoff.

## Packet conformance — routes, frozen order, frozen strings

- All 8 routes registered from this task's `init()`s and wrapped in `AdminOnly`
  (admin.go:29-36, admin_create.go:22-24). No `routes.go`/`web.go`/`auth.go`/
  `base.html`/`index.html`/`probe.html` edits — verified by glob and diff.
- `POST /admin/lights` implements the frozen check order exactly:
  `LimitBody` first (admin_create.go:170), single multipart parse into the typed
  `walkSubmission` (:171, :63-88), CSRF (:176), town bbox 422 with the frozen
  string (:180-183), photo 413/415 via `PhotoProblem` (:185-195), then
  `LightByAssetNo` OR `NearbyLights(NumberedRadiusM)` + `geo.ClassifyDuplicate`
  (:196-211, :252-285). This is T-206's reviewer-mandated "Order B": the photo
  persists only after CSRF and bbox pass, so the previous round's
  photo-before-CSRF forward risk is closed.
- Duplicate contract per reconciliation 2: `dup_choice` ""→409 panel; numbered
  shows Merge + a Cancel link and never `Save anyway`; unnumbered shows Merge +
  Save anyway; numbered+save → 422 (admin_create.go:198-210;
  partials/admin-dup.html:4-9 and the synced copy in pages/admin-walk.html).
  `merge` applies the twin label via `EditLight` and the photo via `attachPhoto`,
  creating no row (admin_create.go:289-326); the ErrDuplicateAssetNo race falls
  back to the numbered panel for the row that won (:221-231).
- `kind=street` refused (admin_create.go:99-110) — reconciliation 3; the form
  offers Park/Building only.
- Client-side bbox pre-flight mirrors the frozen server message, role=alert, on
  every position update (walk.js:9, :93, :60-64; `#walk-alert` in
  pages/admin-walk.html:7-11) — reconciliation 4, matching the e2e draft
  (`setGeolocation` outside TOWN_BBOX → alert without submitting).
- `GET /admin/rows`: fragment only for `HX-Request` (root element `id="rows"`,
  partials/admin-rows.html:1), else 303 `/admin?page=N` (admin.go:196-200) —
  matches the frozen HTTP table.
- List: 50/page via `RecentLights(51, (page-1)*50)` with the limit+1 Older
  detector (admin.go:147-167); heading `Lights`; frozen alt format
  `<asset_no> <label>` / `<label>` (admin.go:96-99); placeholder thumbnails.
- `#light-pos` renders "lat, lon" at exactly 6 dp (admin.go posString, :317-320
  region); the detail 404s on ErrNotFound and non-numeric ids (admin.go:301-313).
- Edit/delete: parse-once (`lightEdit`/`lightDelete`), unspecified fields keep
  values, 303 detail / 303 `/admin` (admin.go:366-464, :516-559).
- Login page: `installLoginPage` (admin.go:37-49) renders `pages/admin-login.html`
  through the pipeline and installs it as `loginPageTmpl` without touching frozen
  auth.go. All frozen login strings survive, and I re-checked them against the
  auth draft's regexes (admin_auth_go_test.go.txt:61-64) and T-203's own
  auth_test.go regexes — every one still matches the styled page.
- htmx: the walk form carries `hx-post`/`hx-target="#dup-slot"`/
  `hx-encoding="multipart/form-data"`; walk.js:180-188 overrides
  `htmx.config.responseHandling` — I verified against the vendored
  htmx-2.0.11.min.js default (`responseHandling:[{code:"204",swap:false},
  {code:"[23]..",swap:true},{code:"[45]..",swap:false,error:true}]`): the key and
  array shape are right and `[45]..` is correctly flipped to `swap:true,
  error:false`, so the 422 alert and 409 panel swap. The packet's
  `<meta name="htmx-config">` suggestion lives in frozen `base.html`
  (allowEval only), so the JS override is the only owned-file route to the same
  behaviour; the drafts assert behaviour, not mechanism.
- No-JS: the pager controls are standalone GET forms to `/admin/rows` with the
  hidden `page` input inside the form (pages/admin-list.html:22-23 — a GET form
  submission replaces the action URL's query, so this is the correct no-JS
  contract); `#add-form` is server-unhidden on any 409/422 re-render
  (pages/admin-walk.html:35).
- walk.js is exactly 200 lines (budget ≤200), `// @ts-check`, ES module, no
  console/eval/inline. Accuracy gate >30 m sets the platform `disabled`
  attribute (walk.js:96-98) as the contract demands; denied geolocation reveals
  `#manual-lat`/`#manual-lon` (walk.js:126-130, code 1/2 before any fix);
  pin drag (pointer) + 1 m arrow nudges clamp to ±25 m; `#pin-pos` mirrors at
  6 dp; photo preview via object URL (CSP `img-src blob:` admitted).

## Security

- CSRF: double-submit verified on all three POSTs (admin_create.go:176,
  admin.go:415-419, :536-540); missing/mismatched tokens → 403, tested
  (admin_test.go TestAdminCreateValidation, TestAdminEdit, TestAdminDelete).
  `SameSite=Lax` on `gl_session` (session.go:72) plus double-submit is sound.
- XSS: every user string goes through html/template escaping; no `template.HTML`
  anywhere in the diff. `template.URL` is used solely for the constant SVG
  placeholder and pipeline-minted `/photos/<path>` values (admin.go:70-77,
  :101-125) with on-line nolint reasons — server-minted today, but see note in
  F2-adjacent: the URL-door would become a vector only if the photos table were
  polluted by something other than the upload pipeline (writePhotoFile mints
  names from crypto/rand).
- SQL: the one handwritten statement is fully parameterized (admin_merge.go:65-69).
- Path traversal: no new file serving; photos are served by T-206's clamping
  resolver. The auxiliary DSN path is validated against the same `?#%` set the
  store refuses (admin_merge.go:34-44 vs store.go:43-50).
- Upload limits: `LimitBody` (15 MB) before any read on create; edit/delete
  bodies capped at 1 MB (admin.go:371, :402-404). Multipart memory budget 32 MB
  > 15 MB cap, so no temp-file spill. >15 MB surfaces as 413 through the
  `*http.MaxBytesError` mapping (admin_create.go:64-76), tested with a runtime
  ~16 MB body.
- Secrets: no token/key/coordinate values reach responses or logs; rejection
  messages are fixed strings; the logging middleware logs the route pattern only.
- Handlers parse once at the boundary into typed structs
  (`walkSubmission`, `lightEdit`, `lightDelete`, `lightIDParam`) and never touch
  `r.FormValue` (grep: zero uses in the new code); IDs are `store.LightID`/
  `store.PhotoID` — rules 6 satisfied.

## Architecture

- HTMX/#map: no admin page or partial contains `id="map"`; the only hx-targets
  are `#dup-slot` and `#rows`, both far outside any map ancestry. Map rule safe.
- Ownership of files: diff touches exactly `internal/web/admin{,_create,_merge,
  _test}.go`, `internal/web/web_test.go`, `web/templates/**` (4 pages + 3
  partials), `web/static/js/{walk.js,globals.d.ts}` and the sanctioned handoff —
  all inside `owns`. No `internal/store` change remains (verified: the store's
  public surface is exactly T-201's). No new dependency (modernc.org/sqlite is
  the repo's existing allow-listed driver). No protected path touched.
- **One real ownership-map breach — see F1** (`internal/web/admin_merge.go`).

## Tests — do they exercise the behaviour?

Yes, at the right level. `newAdminServer` wires a real temp-file store, a real
admin session and matching CSRF pair, and every request goes through
`s.Handler()` (full middleware chain). Coverage walked assertion by assertion:

- Guard: anon GET ×4 (302/HX-401+`HX-Redirect`) and anon POST create/edit/delete
  → 302 `/admin/login` (admin_test.go:314-363).
- Create: happy path (303/Saved-status fragment), a 9-case validation table with
  the frozen messages, 403s both ways, non-multipart 400 whose re-render still
  carries a CSRF pair + bbox + park default, photo pipeline incl. HEIC wording,
  text-as-jpg, pixel bomb, runtime 16 MB → 413, and the served photo 200/jpeg.
- Bbox: full-page 422 asserts the visible role=alert via a non-vacuous helper —
  `alertWithMessage` extracts the specific `<article>` carrying the message
  (admin_test.go:188-200), so it cannot match the page's permanently hidden
  `#walk-alert` placeholder, and `assertVisibleAlert` (:202-216) proves
  role=alert + not hidden. The HX variant additionally rejects a full page.
  Typed values (asset/label/lat/lon/accuracy/kind) all survive the re-render.
- Duplicates: numbered 409 → nothing saved → numbered+save 422 → merge applies
  the twin label (exactly one survivor alt, original gone); panel content is
  asserted against the EXISTING light inside the extracted `#dup-panel`
  (dupPanelOf + assertNumberedPanel), including the no-`Save anyway` negative,
  in both full-page and HX fragment responses; unnumbered panel shows Save
  anyway, save keeps both, >8 m creates no panel; photo-merge pinned end-to-end
  (TestAdminDuplicateMergePhoto:781 — one row survives, the detail's
  `/photos/...` src changes, both files serve 200); the auxiliary write's five
  branches (unset/ambiguous DB_PATH, unknown id, soft-deleted id, canceled
  context) each assert the right error class (TestMergeAttachPhotoBranches:814).
- Edit: label-only keeps asset/position/photo (photo URL compared before/after),
  duplicate asset 409 with re-render, 403 without CSRF. Delete: 303, detail 404,
  list clean, second delete 404, 403 without CSRF.
- Pagination: exact row-set assertions at 50/51 (page 1 holds exactly
  51..02 and lacks 1; page 2/fragment hold exactly 1), plus the no-JS pager
  contract scoped to the pager form (assertPagerPageInput:241-257 checks the
  hidden `page` input AND the matching `hx-get` live inside the
  `method="get" action="/admin/rows"` form carrying the button).
- TestRenderAll now renders each page with its own fixture, fails on a missing
  fixture, a render error, or an empty document (web_test.go:113-197) — the
  previous empty-snapshot hole is closed.
- No `t.Skip`/`test.skip`/`test.only`/`.fixme` anywhere in the new test code
  (the one `t.Skip` in TestRenderAll is pre-existing, unchanged context).

I could not execute the suite (shell denied); every assertion was traced
statically against the implementation and none is vacuous or self-fulfilling.
The committed-Playwright-spec question is settled by the packet itself: the
drafts are the specs (inert until T-207), the scratch evidence harness must be
deleted (it is — only the 4 pages + 3 partials remain under `web/templates`),
and `make e2e-task` is a documented no-op (`playwright.config.ts` testDir is
`acceptance/e2e`; the drafts live in `acceptance/drafts` as `.txt`).

## Findings

1. **[major, architecture debt — needs planner routing] `internal/web` now owns
   a raw SQLite connection and a handwritten query.** admin_merge.go:50-81 opens
   a per-call `sql.Open("sqlite", dsn)` and runs `UPDATE lights SET photo_id…
   WHERE id=? AND deleted_at IS NULL`, violating `docs/spec/architecture.md`'s
   ownership map ("internal/store — SQLite access (sqlc-generated), migrations
   runner, all queries"). Context, to be fair to the worker: the frozen store
   API genuinely cannot express A-205's photo-merge (`UpdateLight` writes
   label/asset_no/updated_at only — internal/store/queries/lights.sql), the
   packet freezes "apply photo/label onto ExistingID, no new row", and
   `internal/store` is outside `owns` (the round-3 `EditLightPhoto` attempt died
   on guard exit 4). So the packet is internally unsatisfiable without either
   this breach or a `BLOCKED`. The implementation manually reproduces every
   store invariant that applies (DSN path rule, WAL/busy_timeout/foreign_keys
   pragmas, 5 s bound, live-row guard, `ErrNotFound` only on 0 rows, FK-safe
   photo id) and is branch-tested plus exercised over real HTTP. Still: raw SQL
   in the web layer, a second connection pool, and a schema dependency invisible
   to sqlc are exactly what the ownership map exists to prevent, and PASSing
   sets a precedent for every future task that finds a store method missing.
   The handoff honestly records the follow-up; the planner should schedule the
   store-scoped `EditLightPhoto` absorption (or bless the pattern explicitly)
   rather than let this linger as prose.
2. **[medium, inherited — must be disclosed/routed] valid photos are orphaned by
   the duplicate-panel flow.** admin_create.go:185-210: `s.Photo(r)` persists the
   row+file before duplicate classification; when the panel is raised
   (`dup_choice=""`) and the admin clicks `Cancel` (a plain link), or when
   numbered+save 422s after a photo upload, the photo row and PHOTO_DIR file
   remain referenced by nothing, permanently. This is forced by the frozen check
   order (photo strictly before duplicates) plus upload.go's atomic
   validate-and-persist API (no validate-only entry point), so it is not fixable
   inside this task's owns — but neither the handoff nor open issues mentions
   it. A-207's no-stray-files contract only covers rejected uploads, so no test
   catches it. Needs a planner decision (GC of unreferenced photos, or a
   validate/persist split in a store/upload-scoped task).
3. **[low] merge-with-photo is two non-transactional writes.**
   admin_create.go:289-326: `EditLight` (label) then `attachPhoto` (photo). A
   failure between them 500s with the label already applied and the photo
   orphaned; there is no rollback path (impossible without F1's store method).
   Single-admin app, short window — acceptable, but the 500 gives the admin no
   hint the merge half-applied. Disclose in the follow-up.
4. **[low] edit's empty-asset semantics differ by transport.** admin.go:486-498:
   a posted `asset_no=""` clears the stored asset number while an omitted field
   keeps it. The real form always posts the field pre-filled, so the UI is safe,
   but any client that posts the empty field silently drops the asset number.
   Defensible ("empty clears"), undocumented in the handoff.
5. **[low, handoff accuracy] two handoff claims are wrong.** (a) "The three
   `harness/reviews/T-205-*.md` files are still in the worktree" — exactly one
   exists (`T-205-visual-reviewer.md`; the round-3/4 review artifacts the
   handoff responds to are not in the worktree). (b) "attribution visible" on
   the walk screenshots — the visual reviewer's own note 3 contradicts this (the
   walk page is an SVG pin pad, no tiles, no attribution). Neither affects the
   merge, but the handoff should not assert artifacts that aren't there.
6. **[info] unnumbered-merge drops a submitted asset number.**
   admin_create.go:~310: merge passes `existing.AssetNo` to `EditLight`, so on
   the unnumbered-duplicate path a submitted asset number is discarded (the
   numbered case is identity). Consistent with "apply label/photo onto the
   existing row"; note for T-207 dispute handling.
7. **[info] parse-failure re-render carries `value="0"` in the hidden lat/lon.**
   admin_create.go:365-389 with `sub == nil` → zero-lat/lon echo. Harmless
   (`Add light here` stays disabled until a fix; the server re-validates), noted
   only because every other field's echo is carefully preserved.
8. **[info] the responseHandling override also swaps 5xx plain-text bodies into
   `#dup-slot`** (walk.js:185-187) — a rare 500 renders as "internal error" text
   in the form rather than an error event. Cosmetic.
9. **[info] worktree debris**: ~15 `test-run.db-visual*` (-shm/-wal) files at the
   root from the visual runs. Gitignored (`test-run.db*`), so the guard and gate
   are unaffected; cosmetic clutter with the usual shared-DB session caveat.
10. **[info] visual evidence**: 8 screenshots exist under `test-results/` and a
    separate visual-reviewer PASS artifact is on disk with per-screenshot
    findings (including the two accepted sub-44px deviations, both pre-declared).
    This reviewer could not open the images (no image input this session), so the
    visual claims are accepted on that artifact plus the evidence spec's
    programmatic assertions (44×44 boxes, no 320 px overflow,
    `naturalWidth > naturalHeight`) quoted in the handoff; the spec itself was
    deleted as the packet mandates.

## Verdict

Every frozen route, status code, string and reconciliation in the packet and the
draft oracle is implemented and pinned by tests that exercise the real chain
against a real store; the security posture (CSRF-before-photo-persist, escaping,
limits, parameterized SQL, parse-don't-validate) is correct; the map rule and all
repo rules hold; ownership of files is exactly `owns`. The one genuine
architecture breach (raw SQL for the merge photo, F1) is the consequence of a
packet that froze a behaviour its own frozen store API cannot express, is
narrowly scoped, defensively implemented, honestly disclosed with a concrete
follow-up, and is the alternative to discarding four correctly-delivered claims —
so it is routed to the planner rather than blocking. F2 is the same shape
(inherited from the frozen check order) and must be disclosed likewise.

VERDICT: PASS
