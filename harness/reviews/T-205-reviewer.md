# T-205 review — Admin UI: walk-the-town flow, light CRUD, list + HTMX pagination (A-202/A-203/A-208/A-209)

Review basis: the attached diff (`harness/reviews/T-205.diff`, identical to the diff supplied
with this request; byte-checked against the extracted copies `.review-t205-admin.go` /
`.review-t205-admin_create.go` left by a prior pass) against `harness/tasks/T-205.md` (the
packet), `acceptance/drafts/README.md` (frozen UI/HTTP contract + reconciliations 1–4 and the
per-assertion visual notes), `acceptance/drafts/admin-walk.spec.ts.txt` and
`acceptance/drafts/admin_lights_go_test.go.txt` (the future oracle), `docs/spec/architecture.md`
(ownership map, HTMX/map rule, parse-don't-validate, typed IDs), AGENTS.md hard rules, and the
merged building blocks this diff compiles against: `internal/web/{web,csrf,session,auth,upload,
probe,templates}.go`, `internal/store/{lights,photos}.go`, `internal/geo/{geo,duplicate}.go`,
`internal/config/config.go`, `web/templates/base.html`, `web/static/vendor/htmx-2.0.11.min.js`,
`Makefile`, `.golangci.yml`, `tsconfig.json`.

Verification limits: the task's worktree is not mounted in this checkout (only the diff is
available) and shell execution is denied, so I could not re-run `make check-fast` /
`templates-check` / `js-check` / the scratch Playwright suite — the handoff's exit codes are
taken on trust where marked. Everything below is a static check of the diffed bytes against
the surrounding main-branch code, including independent re-derivations (pagination geometry,
duplicate radii, htmx config semantics, template.URL data flow) rather than trusting the
diff's own comments.

## Routes and frozen HTTP contract

All seven routes are registered from the task's own `init()`s (admin.go:21-28,
admin_create.go:22-24), every one wrapped in `AdminOnly` (session.go:83-97 provides the 302 /
401+`HX-Redirect` guard the drafts freeze). Verified against the packet and the frozen table:

- `GET /admin` — list page, 1-based `page`, heading `Lights` (admin.go:149-167; template
  admin-list.html:3). `RecentLights(ctx, adminPageSize+1, (page-1)*50)` (admin.go:127) —
  the packet's literal `50` plus one extra row that is trimmed off (admin.go:138-141) and
  used only as the `HasOlder` detector. Observable behaviour is identical to
  `RecentLights(50, …)` (max 50 rows per page, `ORDER BY created_at DESC, id DESC` via the
  store), and the +1 is the only way to render the frozen `Older` control without a second
  COUNT query. Letter deviation, spirit satisfied; documented in the handoff.
- `GET /admin/rows` — HX-only fragment (root element `id="rows"`, partials/admin-rows.html:1),
  else 303 `/admin?page=N` (admin.go:177-180). Matches the frozen contract exactly.
- `GET /admin/walk` — fresh/reused CSRF via `IssueCSRFToken`, bbox pre-rendered for the
  client pre-flight (admin.go:235-251, `formatBBox` :255-257 mirrors `config.ParseBBox`'s
  input form; test asserts `data-bbox="42.7,-71.09,42.82,-70.96"`).
- `GET /admin/lights/{id}` — 404 on `ErrNotFound` and on malformed/non-positive ids
  (admin.go:264-271, 300-314); `#light-pos` renders "lat, lon" at exactly 6 dp
  (`posString`, admin.go:290-292) — the same string `#pin-pos` shows client-side.
- `POST /admin/lights` — frozen check order preserved verbatim (admin_create.go:164-238):
  `LimitBody` FIRST (:170, before any body read) → parse (:171) → CSRF (:176, uniform 403) →
  bbox 422 + the frozen message (:180-184) → photo 413/415 via `PhotoProblem` (:185-195) →
  duplicates (409 panel / merge / numbered+save→422 / save-anyway, :196-211) → create.
  Success: role=status `Saved` fragment for HX, 303 `/admin` otherwise
  (`respondWalkOK`, :402-412) — reconciliation 1 honoured. Non-multipart bodies 400
  (parse error mapped at :74-80; tested). Oversize bodies surface as `*http.MaxBytesError`
  at the parse and map to 413 through `PhotoProblem` — matches upload.go's documented
  caller-parsed-first contract (upload.go:105-108) and the draft's 413.
- `POST /admin/lights/{id}` (edit) — nil fields keep stored values
  (`validatedEditLabel/Asset`, admin.go:453-479), 303 back to the detail (admin.go:450);
  duplicate asset re-rendered with an alert.
- `POST /admin/lights/{id}/delete` — soft delete → 303 `/admin`; repeat delete 404
  (admin.go:505-535); CSRF 403 before any store touch (tested).

The frozen duplicate semantics are correct: `classifyDuplicate` (admin_create.go:252-285)
feeds `LightByAssetNo` (distance-independent) + `NearbyLights(geo.NumberedRadiusM)` into
`geo.ClassifyDuplicate`, which implements numbered-anywhere / unnumbered-within-8 m
(geo/duplicate.go) exactly as reconciled in README:100-101. The numbered panel shows
`Merge` + a `Cancel` link and NO `Save anyway`; unnumbered shows both (partials/admin-dup.html
+ the sync'd inline copy in admin-walk.html) — reconciliation 2 honoured. The
lost-race branch (admin_create.go:221-231) converts a concurrent-create
`ErrDuplicateAssetNo` into the numbered panel instead of a 500 — good defensive design.

## Security

- **CSRF**: every POST verifies the double-submit pair after the boundary parse
  (admin_create.go:176, admin.go:413-416, :521-524); uniform 403, no oracle about which half
  failed. Re-rendered error pages (409 dup panel, 422s) mint/reuse a fresh token via
  `walkPageDataOf` (admin_create.go:380) so the follow-up Merge/Save-anyway/corrected submit
  still verifies. Anon POSTs never reach a parser (AdminOnly wraps the handlers).
- **XSS / template.HTML**: no `template.HTML` anywhere in the diff. The one escaping-bypass
  type is `rowView.ImgSrc template.URL` (admin.go:53), used for (a) the constant data-URI
  placeholder (admin.go:36, :83) and (b) `"/photos/" + p.Path` (admin.go:86). I traced the
  provenance of `p.Path`: it is written exclusively by the upload pipeline
  (`writePhotoFile`, upload.go:222-242 → `CreatePhoto`, store/photos.go:59-72) as
  `YYYY/MM/<16-hex-rand>.jpg` — server-minted, never client-influenced. No user input can
  reach `ImgSrc`. `template.URL` is genuinely required for the `data:` placeholder
  (html/template's URL filter rewrites `data:` srcs to `#ZgotmplZ`; CSP
  `img-src 'self' data: blob:` allows it). Both conversions carry same-line nolint reasons
  (AGENTS rule 9); note forbidigo's pattern (.golangci.yml:30) only forbids
  `template.HTML`, so the `//nolint:forbidigo` on admin.go:53 is redundant but harmless.
  All other user text (labels, asset numbers, messages, `Role`, `Status`) flows through
  html/template contextual auto-escaping.
- **SQL**: no SQL in the diff; everything goes through the typed store methods.
- **Path traversal**: no file-path handling in the new code; `/photos/...` serving is
  T-206's already-reviewed clamped route.
- **Upload limits**: `LimitBody` (15 MB) before any parse on create; edit/delete bodies
  capped at 1 MiB via `http.MaxBytesReader` (`parseFormValues`, admin.go:383-395). The
  multipart memory budget (32 MB, admin_create.go:28) can never spill since the body is
  already ≤15 MB — same reasoning as upload.go:44-48.
- **Secrets**: no token/key material in any response or log; slog calls carry errors and
  pattern names only; redirects are fixed strings or int64-derived (`/admin?page=%d`,
  `/admin/lights/%d`) — no open redirect. CSRF cookie remains HttpOnly; the form value is
  the double-submit half by design (frozen T-103 pattern).
- **DoS**: `?page=` values large enough to overflow `(page-1)*50` into a negative offset
  produce a store "offset must be >= 0" error → 500 "internal error" (no crash, admin-only,
  no data effect) — minor nit, see findings.

## Architecture (docs/spec/architecture.md)

- **Ownership map**: diff touches only `internal/web/admin*.go` (incl. admin_test.go),
  `internal/web/web_test.go`, `web/templates/**`, `web/static/js/walk.js`,
  `web/static/js/globals.d.ts` and the handoff — all inside the packet's `owns`. No
  protected path, no dependency-file change (`allow_deps: []` respected; imports are stdlib
  + repo-internal only), no migration, no `base.html`/`pages/index.html`/`partials/probe.html`
  edit. Feature registration via `init()`/`register` exactly as the ownership map demands.
- **HTMX never swaps `#map`**: no `id="map"` exists anywhere in the tree yet (the map is a
  later milestone; verified by grep) and none of the new partials introduce one; the only
  `hx-target`s are `#rows` and `#dup-slot`, both inside `main > section` — nowhere near any
  map ancestor.
- **Parse-don't-validate**: `walkSubmission` / `lightEdit` / `lightDelete` / `listQuery` /
  `lightIDParam` are parsed once at the boundary; I verified no handler reads
  `r.FormValue`/the form after its parse (adminCreate uses only `sub`, adminEdit only
  `edit`, adminDelete only `del`, adminList only `q`). Floats are range- and
  NaN/Inf-checked (`parseLatLon`, admin_create.go:143-157; `finite`, :159-162).
- **Typed IDs**: `lightIDParam.ID` is `store.LightID`; geo's `int64` `ExistingID` is
  converted at the store boundary (`store.LightID(decision.ExistingID)`,
  admin_create.go:290, :321) — exactly the conversion T-204's review asked T-205 to make.
- **Status derivation**: admin code reads `l.Status` verbatim from the store and never
  derives it — rule 8 respected.

## walk.js and the templates against the frozen UI contract

- Frozen ids/strings all present and asserted by tests: `#walk-pos`/`#pin-pos` at 6 dp
  (`posText`), `#walk-accuracy` (`accuracy N m`), `#pin` with `aria-label="position pin"`,
  focusable (`tabindex="0"`) and keyboard-nudged ±1 m (`NUDGE_STEPS`, walk.js:28 — packet
  "nudge by 1 m"), pad spanning ±25 m (`PAD_SPAN_M = 50`, clamp at ±25, walk.js:8-9,39),
  `Add light here` / `Save anyway (low accuracy)` / `Confirm pin` / `Merge` / `Save anyway`
  / `Cancel`, labels `Asset number`/`Label`/`Kind`(Park|Building only — `KindStreet`
  refused with 422, admin_create.go:104-109, reconciliation 3)/`Photo`,
  `#manual-lat`/`#manual-lon` number inputs on denied/unavailable geolocation
  (`enterManual`, walk.js:103-111), `#photo-preview` object-URL img (browsers honour EXIF
  orientation in intrinsic dimensions — satisfies README's landscape assertion),
  `#dup-slot` with `#dup-panel`, `#light-pos` 6 dp.
- **Accuracy gate is real, not painted**: `add.disabled = !pt || needChoice ||
  outsideBBox(pt)` (walk.js:95) sets the platform `disabled` attribute the e2e draft
  asserts; `Save anyway (low accuracy)` both un-hides and clears `lowAccAccepted`,
  unlocking a genuine 80 m save. The gate covers exactly `> 30` (strictly), per the packet.
- **Client bbox pre-flight** (reconciliation 4): `setAlert(outsideBBox(pt) ? BBOX_MESSAGE
  : '')` on every position update (walk.js:90), mirroring the frozen server message
  byte-for-byte into the `role="alert"` article next to the readout — the no-submit A-204
  behaviour the drafts freeze.
- **htmx 4xx swapping**: the packet suggested a `<meta name="htmx-config">` on the walk
  page; the implementation instead assigns `window.htmx.config.responseHandling` in walk.js
  init (walk.js:184-188), flipping the vendored default (`[45].. → swap:false,error:true`,
  confirmed in htmx-2.0.11.min.js) to `swap:true,error:false`. I verified the mechanics:
  htmx (defer, head, base.html:10) executes before the body module script; deferred
  classic + module scripts run in document order before DOMContentLoaded, so the override
  lands before htmx processes the form and before any request; htmx reads
  `config.responseHandling` per response, and its `HX-Redirect` handling early-returns
  before any swap, so the AdminOnly 401+`HX-Redirect` guard still redirects correctly on
  the walk page. Functionally equivalent to the meta tag (a second meta on the walk page
  would have merged via htmx's multi-meta config). Letter deviation, spirit satisfied,
  documented in the handoff.
- **Pagination markup**: full page wraps `<div id="rows">` around an inline
  `admin-rows-fragment` (admin-list.html:4,7); the HTMX partial is the same content with
  the wrapping div (partials/admin-rows.html, sync-commented both sides). The duplication
  is forced by templates.go's parsing model (pages are parsed as base+page only, so a page
  cannot `{{template}}` a partial); sync comments mitigate drift. `Older`/`Newer` work both
  via htmx (`hx-get` + `outerHTML` swap, no navigation) and no-JS (`formaction` → 303 to
  the full page). Row alt format `"<asset_no> <label>"` / `"<label>"` (admin.go:72-78)
  matches the frozen reconciliation; status chips carry dot AND text (never colour-only).
- **`web/templates/scratch/`**: the packet's `scratch/` could not live at the repo root
  (outside owns); the handoff explains the `web/templates/scratch/` workaround and its
  pre-commit deletion (via a `go run` helper since `rm` is not allowlisted). Nothing
  scratch-related remains in the diff. Note: while it existed, any stray `.html` there
  would have been picked up by `parseTemplates` — the risk is past, but worth remembering
  for future tasks.

## Tests (do they exercise the behaviour?)

`internal/web/admin_test.go` (764 lines) runs every route through the full middleware chain
(`s.Handler().ServeHTTP`) against a real temp SQLite store with a real minted session cookie
and a matching double-submit CSRF pair — the same wiring as production, not handler-unit
shortcuts. Coverage matches the packet's mandated list: guard × (anon 302 / HX 401+
`HX-Redirect`) (TestAdminRoutesGuard, :168); walk page frozen strings + CSRF cookie + value
attribute (:209); empty list (:247); rows 303-without-HX and fragment-root-with-HX using the
draft's own `\A\s*<[a-zA-Z][^>]*\bid="rows"` selector (:265); create happy paths both ways
(303 `/admin` + HX role=status `Saved`) (:296); a 9-case validation table + missing/mismatched
CSRF 403 + non-multipart 400 (:334); bbox 422 full-page and HX-fragment, including a
fragment-is-not-a-page assertion (:389); photo pipeline happy-path (stores, lists, serves
`image/jpeg`) + HEIC/text/pixelbomb 415s (HEIC wording) + 16 MB 413 (:417); numbered
duplicate (409 panel strings, nothing-saved-while-panel-up, numbered+save 422, merge applies
the twin label with the frozen alt asserted positively and negatively) (:473); unnumbered
duplicate (panel with Save anyway, save-anyway keeps both, beyond-8 m no panel) (:527);
detail (200/6 dp/404/non-numeric 404) (:592); edit (label-only keeps asset+position, dup
asset 409, no-CSRF 403) (:624); delete (303, 404 after, list clean, re-delete 404, no-CSRF
403) (:665); pagination 50/51 boundary incl. fragment page 2 content (:709).

I independently re-derived the test geometry: the pagination seeds (lat 42.76+i·0.0005,
lon −71.045+i·0.0005, i≤51) are all inside the test bbox and mutually ≥~69 m apart (>8 m
unnumbered radius), so no seed can trip a duplicate panel; the unnumbered dup fixtures
(42.7625 vs 42.76254 ≈ 4.4 m; "beyond" 42.7627 ≈ 22 m) bracket the 8 m radius correctly.
Expectations are not self-generated — they mirror the frozen drafts' values.

`TestRenderAll` (web_test.go) now renders per-page fixtures and FAILS on any page without
one — the packet's "admin pages render with their own data, not homeData" is enforced, not
just styled. No `t.Skip` added (the pre-existing GL_RENDER_SNAPSHOTS gate is untouched,
main-branch behaviour), no `.only`/`fixme`, no golden edits. Every handler has a table test
(AGENTS rule 7); the mobile-project Playwright evidence is the scratch suite (deleted per
packet) plus the kept `test-results/` screenshots — taken on trust here, with the prior
attempt's guard failure (stale screenshots) explained plausibly in the handoff.

## Findings

### Cross-task gaps the worker could not close inside `owns` (planner items, not rework)

1. **`pages/admin-login.html` exists but is not wired** — the live `GET /admin/login` still
   renders auth.go's inline minimal page (auth.go:143-179). The packet asks the template to
   "replace T-203's minimal page" while simultaneously freezing auth.go ("never edit
   web.go/csrf.go/probe.go/auth.go/session.go/upload.go"), and re-registering
   `GET /admin/login` from admin*.go would panic the mux on duplicate patterns — the
   contradiction is real and visible in auth.go's own doc comment (:14-15, :143-146), which
   expects T-205 to do the swap. A-201 is T-203's claim, and the inline page carries every
   frozen A-201 string (`<h1>Admin login</h1>`, label `Token`, input `name="token"`,
   button `Sign in` — verified against the auth draft's regexes), so the oracle is green;
   only the A-201 *visual* note (pico card, 44 px button) is unmet, and that page is
   outside this task's claims. The template itself is complete, renders through the
   pipeline (TestRenderAll fixture), and keeps the frozen strings byte-for-byte. Correctly
   documented as open issue 1 with the exact 3-line fix for T-203's owner. Follow-up
   needed, not a blocker for T-205.
2. **Merge applies the label but not the photo** — the packet freezes
   "dup_choice==merge → apply photo/label onto ExistingID" (task line 67; README:68
   repeats "label/photo"), but `EditLight(id, label, assetNo *string)` is T-201's whole
   edit surface (store/lights.go:256) and nothing in the frozen store API can set a light's
   photo post-create. `mergeDuplicate` (admin_create.go:289-316) applies the label onto the
   survivor, which is ALL both acceptance suites assert (Go draft lines 202-218; e2e lines
   426-429 — alt-format only). Documented as open issue 2 with the needed store shape. A
   follow-up task (store `EditLightPhoto`-style method + one-line wiring) is required to
   honour the packet's letter.
3. **Consequence of the frozen check order: orphan photos on dup paths** — `s.Photo(r)`
   persists (file + photos row) BEFORE duplicate handling (admin_create.go:185 precedes
   :196), so every 409 dup-panel response and every merge leaves an unreferenced photo, and
   an HTMX merge (the file input still holds the file) persists a SECOND copy. A-207's
   no-stray-files contract only covers *rejected* uploads, which leave nothing — I verified
   the rejects in admin_test.go:417 assert 4xx and the draft only counts PHOTO_DIR on
   rejects/happy-path. This is inherent to the packet's frozen order plus gap 2, not a
   worker error; worth a planner note when the photo-merge API lands (it could also link or
   GC these orphans).

### Minor code issues (non-blocking)

4. `walkReject` with `sub == nil` (parse failures: label length, bad kind, bad position,
   non-multipart, oversize) re-renders the no-JS walk page with an EMPTY CSRF token and
   empty field values (admin_create.go:364-367 builds bare `walkPageData`); a no-JS client
   resubmitting from that page gets 403 until it reloads `/admin/walk`. The HX path (the
   only path the drafts exercise) is unaffected — it answers the alert fragment. Also,
   `walkPageDataOf` (:377-390) does not echo `Values.AssetNo`, so even the sub≠nil no-JS
   re-render loses the typed asset number. Cosmetic-to-minor UX; untested edge.
5. `classifyDuplicate` fails open on a `NearbyLights` store error (admin_create.go:264-268:
   log + "no duplicates" → proceeds to create). Fail-closed (500) would block saving on a
   transient DB error instead; either is defensible, but silently bypassing the unnumbered
   8 m check on error deserves a comment at minimum.
6. `mergeDuplicate`'s vanished-existing fallback `CreateLight` (admin_create.go:294-301)
   drops `PhotoID` — inconsistent with the happy path (the race window makes it rare, and
   gap 2 means merges don't apply photos anyway).
7. Geolocation TIMEOUT (error code 3) never falls back to the manual fields
   (walk.js:126-130 handles codes 1/2 only); the packet says "Denied/unavailable" →
   manual. A pure timeout (10 s, no fix) leaves `Add light here` disabled with no
   recourse short of reload. Minor UX.
8. `<article role="alert">` / `role="status"` (admin-walk.html:8; partials/admin-alert.html:1)
   — per ARIA-in-HTML, `article`'s allowed roles do not include the live-region roles, so
   an axe scan of a *visible* alert would flag `aria-allowed-role`. The frozen a11y spec
   passes only because `#walk-alert` is `hidden` (excluded from the a11y tree) when the
   walk page is scanned, and the `role=status "Saved"` fragment is never scanned. A `<div>`
   with the same pico styling would be fully conforming; recommend swapping in a follow-up
   so a future axe pass over an active alert can't regress the a11y claim.
9. Edit rejections always answer 409 (`adminEditRejected`, admin.go:486) even for input
   validation (label length / asset pattern); the contract only freezes success-303, and no
   draft exercises a failing edit, but 422 would be the semantically correct status.
10. `GET /admin?page=<huge>`: `(page-1)*50` overflows to a negative offset → store error →
    500 (admin.go:127 + parseListQuery:106-113 bounds only `page ≥ 1`). Clamping the
    offset (or capping page) would turn this into an empty page. Admin-only, no data
    effect.
11. Handoff bookkeeping: walk.js is exactly 200 lines (`@@ -0,0 +1,200 @@`), the handoff
    says 199 — the ≤200 budget is met either way; likewise "50/page +1" vs the packet's
    literal `RecentLights(50, …)` phrasing. Cosmetic.

## Verdict

The frozen HTTP/UI contract is implemented faithfully — routes, statuses, frozen strings,
the frozen check order, both duplicate panels, the HTMX fragment/redirect split, pagination
50/51 — and is verified by full-chain table tests on a real store whose expectations mirror
the oracle. Security is clean (CSRF everywhere, no `template.HTML`, the single
`template.URL` provably confined to server-minted values, upload caps enforced, no secret
leakage, no traversal/SQL surface). Architecture rules are respected (ownership, init
registration, parse-don't-validate, typed IDs incl. the geo→store conversion, no `#map`
contact, status never derived, no new deps). The two packet clauses that cannot be
satisfied inside this task's frozen boundaries (wiring the styled login page; applying the
photo on merge) are delivered as far as possible, pass the acceptance oracle, and are
documented precisely with the fixes each needs — planner follow-ups, not rework. Findings
4-11 are minor and none touches an asserted behaviour.

VERDICT: PASS
