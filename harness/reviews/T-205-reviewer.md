# T-205 review (reviewer)

Scope: the attached diff (5 commits, 5847295..6e118c9) vs `harness/tasks/T-205.md`,
`acceptance/drafts/{README.md,admin-walk.spec.ts.txt,admin_lights_go_test.go.txt}`,
`docs/spec/architecture.md`, AGENTS.md, and the merged dependency code
(`internal/store/*`, `internal/web/{upload,auth,session,csrf,templates,probe}.go`,
`internal/geo/*`).

Method note: shell execution was denied in this review session after the initial
`git log`/`git status` (worktree clean at HEAD 6e118c9 except the pre-existing
untracked `node_modules` symlink), so `make check-fast` / `templates-check` /
`js-check` could NOT be re-run by the reviewer; the handoff's exit-code table is
unverified here. The 10 screenshots exist under `test-results/` but this
reviewer session has no image input, so visual claims (44 px Cancel, 48×36
thumbnails, bottom-third placement) rest on the worker and the visual-reviewer.

## Findings

### 1. BLOCKER — new files outside the task's owns (`internal/store/`)

- `internal/store/lights_photo.go:1-74` and `internal/store/lights_photo_test.go:1-140`
  are new files, but T-205's owns are exactly `internal/web/admin*.go`,
  `internal/web/web_test.go`, `web/templates/**`, `web/static/js/walk.js`,
  `web/static/js/globals.d.ts` (`harness/tasks/T-205.md:11`, `harness/tasks/graph.json:412-418`).
  The packet's own rule: "File names must match your owns globs" (`harness/tasks/T-205.md:124-125`).
  `internal/store/lights*.go` is T-201's owns (`docs/PLAN.md:874`).
- The worker demonstrably knew: commit 5847295 is "revert out-of-owns store
  API", then 8845c6e re-introduced it. The sanctioned move when a packet cannot
  meet its oracle in-owns is `BLOCKED: <reason>` (AGENTS.md handoff rule; the
  T-105 lesson recorded in `acceptance/drafts/README.md:7-12`).
- Root cause is a genuine planner gap: `EditLightPhoto` is needed because the
  frozen A-205 clause "Merge APPLIES the new submission's label/photo onto the
  existing row" (`acceptance/drafts/README.md:68`; packet line 67-68) requires a
  photo_id write, and no API in T-205's building-blocks list
  (`harness/tasks/T-205.md:26-28`) updates a light's photo. The choice was
  between breaking the frozen contract, breaking the architecture map (SQL in
  `internal/web`), or breaking owns — and the correct fourth option (BLOCKED →
  planner extends owns or adds the API to a store-owned task) was not taken.
- The code itself is store-quality (single transaction, `ErrNotFound` /
  `ErrDuplicateAssetNo` wrapping consistent with `internal/store/lights.go`,
  FK-enforced photo id — `migrations/0002_lights.sql:27` + `_pragma=foreign_keys(1)`
  in `internal/store/store.go:28-33` — and every failure branch tested). If the
  planner/integrator ratifies the addition (e.g. adopts the files under a
  T-201-owned follow-up), this finding closes; as shipped, it must not merge as
  T-205 work.

Remedy: remove the two store files and re-submit the (fully working) in-owns
remainder with `BLOCKED: packet gap — no owned store API updates a light's
photo_id (A-205 merge-photo)`, OR have the planner/integrator explicitly extend
owns / reassign the API before merge.

### 2. Medium — runtime reach-across into frozen auth.go state

- `internal/web/admin.go:42-56` (`installLoginPage`) reassigns `loginPageTmpl`
  (declared at `internal/web/auth.go:147`) from `init()`, capturing a rendered
  page and re-parsing it as a template. It honours the letter of "never edit
  auth.go" and panics loudly on parse/render failure (build-defect semantics),
  and T-203's frozen tests still pass (`assertLoginMarkup` regexes at
  `internal/web/auth_test.go:30-32` match the styled page). Still: it reaches
  into another task's file state at runtime, depends on Go init ordering
  (package vars before init funcs — currently safe), and duplicates template
  parsing at startup. Documented in the handoff (open issue 2); acceptable only
  as a flagged, temporary bridge.

### 3. Low — `template.URL` door for thumbnails (safe today, pinned never-user-input)

- `internal/web/admin.go:79-86` and `admin.go:109-114`: `ImgSrc` is
  `template.URL`, bypassing html/template's URL scheme filter so the constant
  data-URI placeholder survives (`img-src 'self' data: blob:` allows it). Values
  are (a) the constant `photoPlaceholder` and (b) `"/photos/" + p.Path` where
  `p.Path` can only originate from the pipeline's minted
  `YYYY/MM/<16-hex>.jpg` (`internal/web/upload.go:222-242`) — never user input.
  nolint reasons are on the same line per rule 9. This is the one door in the
  diff that must never receive user-controlled data; the invariants holding it
  shut live in another package's file (upload.go), so it deserves a watch-item.

### 4. Low — photos persisted before the duplicate decision (retention leak)

- `internal/web/admin_create.go:193-199`: `s.Photo(r)` writes the file + photos
  row before duplicate classification. On a 409 panel followed by Cancel
  (`/admin/walk` link) the photo is orphaned; on every re-post of the panel
  (including the Merge click) the pipeline stores ANOTHER copy. A-207's
  no-stray-file oracle only covers rejected uploads (413/415), so no contract is
  breached, and the handoff documents the replaced-photo GC question (open issue
  3) — but not the cancelled-panel / merge-re-post leak. Retention/GC belongs to
  the nightly jobs slice later; worth recording as an open issue there.

### 5. Low — handoff inaccuracy

- `harness/handoffs/T-205.md:12-13` claims the untracked
  `harness/reviews/T-205-visual-reviewer.md` was "left in place"; no such file
  exists in the worktree (`harness/reviews/T-205*` → no match). Evidence
  references should be verifiable.

### 6. Notes (no action)

- Fragment markup is duplicated between `pages/admin-list.html` and
  `partials/admin-rows.html` (and the dup/alert panels inline) with `sync:`
  comments — forced by `templates.go`'s per-page parse design (T-205 does not
  own templates.go); acceptable with the comments.
- HTMX/map rule: no partial contains `id="map"`; every hx-target
  (`#dup-slot`, `#rows`) is outside any map ancestor. The pager uses
  `hx-target="#rows" hx-swap="outerHTML"`, which re-renders the pager itself —
  correct.
- Parse-don't-validate / typed IDs: `parseLightID` → `store.LightID`
  (`admin.go:290-297`); `walkSubmission`/`lightEdit`/`lightDelete`/`listQuery`
  parsed once at the boundary; no `r.FormValue` anywhere in the new files
  (grep-verified). Upload limits: `LimitBody` first (15 MB,
  `admin_create.go:175`), 1 MB `MaxBytesReader` on edit/delete
  (`admin.go:409-421`). CSRF double-submit verified on every POST before any
  semantic validation (`admin_create.go:181-211`, `admin.go:439-442,547-550`);
  the 403-before-422 order is tested (`admin_test.go` `csrf is checked before
  field validation`). SQL is parameterised (sqlc + `?1/?2` in lights_photo.go).
  No secrets in output; error logs carry no token values.
- Frozen-contract spot-checks all line up: list heading/alt format/pager
  (`admin.go:99-105,152-172`), rows-fragment root id + 303 without HX
  (`admin.go:197-218`), walk-page ids/strings and `data-bbox`
  (`admin-walk.html:2-71`), numbered panel Merge+Cancel/no-Save-anyway and
  unnumbered Merge+Save-anyway (`admin-dup.html:1-10`), numbered+save → 422
  (`admin_create.go:246-250`), success role=status `Saved` (HX) vs 303 `/admin`
  (`admin_create.go:436-446`), bbox 422 message (`admin_create.go:188-192`),
  `#light-pos` at 6 dp (`admin.go:316-318`), edit keeps unspecified fields and
  303s to detail (`admin.go:457-476`), delete → 303 `/admin`
  (`admin.go:551-561`), guard 302/401+`HX-Redirect` via AdminOnly
  (`session.go:83-97`). walk.js: 30 m gate with the platform `disabled`
  attribute, client bbox pre-flight mirroring the frozen message (drafts
  README reconciliation 4), keyboard-nudgeable `#pin` (aria-label
  `position pin`), manual fallback, 200 lines exactly, no `console.log`.
- Tests are genuinely behavioural: every route through the full middleware
  chain against a real temp store; the pagination 50/51 boundary incl. the
  no-JS hidden-input pager contract; the dup panel scoped to `#dup-panel` (so
  the page's hidden placeholders can't satisfy assertions); the merge-photo
  clause end-to-end over HTTP; the no-swallow 500 on duplicate-lookup failure
  (DROP TABLE lights with sessions intact). `TestRenderAll` was de-vacuoused
  (per-page fixtures, render errors and empty documents now fail the test).

## Verdict rationale

Functionally and security-wise the diff is solid and, apart from the visual
claims I could not re-inspect, everything the frozen drafts assert is present
and tested. But the packet's owns rule is a hard coordination boundary, the
breach is deliberate and documented in the worker's own commit history, and the
review is the only enforcement point left (the worktree guard's write policy is
broader than task owns). The planner gap behind it is real and needs an owner's
decision, which a PASS would bury.

VERDICT: BLOCK
