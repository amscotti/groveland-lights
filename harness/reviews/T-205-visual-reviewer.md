# T-205 visual review — Admin UI: walk-the-town flow, light CRUD, list + HTMX pagination

Review basis: the attached screenshot (iphone-se-320 walk page with the numbered
duplicate panel open — the state the handoff calls `dup-numbered-se.png`), plus the
diff `harness/reviews/T-205.diff`, checked against `harness/tasks/T-205.md`,
`acceptance/drafts/README.md` (frozen contract table + per-assertion visual notes,
lines 178–226), `web/templates/base.html`, `web/static/css/app.css` and the vendored
`pico-2.1.1.min.css`. The worktree path `/Users/ascotti/Documents/git_repos/wt/T-205/test-results`
was permission-denied for this role (external directory), so the remaining PNGs could
not be opened directly; per instructions I reviewed the attached screenshot and audited
the diff for layout/CSS correctness. The handoff claims programmatic no-overflow and
≥44 px button assertions green (16/16 scratch specs, re-run after the final two changes)
— taken on trust where marked, as the earlier reviewers did.

## Screenshot findings (iphone-se-320, 320 px, numbered dup panel open)

- **320 px layout**: clean. Header nav, `Walk the town` h1, readout strip
  `42.761700, -71.030300 accuracy 12 m` (6 dp + frozen accuracy format, readable),
  pin pad with grid lines, `±25 m` scale label top-left and the pin centred and
  clearly visible, `#pin-pos` mirroring the identical 6 dp string (A-203's
  "equals `#light-pos`" property), full-width `Add light here`, then the
  `New light at the pin` section. No horizontal overflow, nothing clipped; the long
  per-run asset number `GP-017-mwffhxmn-se-dupvis` wraps cleanly inside the dup
  panel text and the asset input.
- **A-205 numbered panel**: bordered `<article>` (pico) with an amber warning
  triangle, heading `Possible duplicate`, existing label bold (`Dup vis lamp`),
  asset number, `about 0.0 m away`, full-width `Merge` + a `Cancel` link, and NO
  `Save anyway` — exactly the frozen reconciliation 2 / A-205 note. Status is
  conveyed by shape+text, never colour alone.
- **Form**: labels `Label` (prefilled with the twin label — the HTMX 409 swapped
  into `#dup-slot` without losing typed values), `Asset number`, `Kind` select
  showing `Park`, `Photo` native input, full-width `Confirm pin`. Footer
  attribution line legible and unobstructed.
- **Tap targets**: `Add light here`, `Merge`, `Confirm pin` are pico default
  buttons ≈ 48 px tall and full-width at 320 px (the screenshot also empirically
  confirms pico `.grid` collapses to one column at 320 px, as the low-accuracy
  note requires). One miss below the 44 px floor: the `Cancel` link (~20 px).
  Documented in the handoff as open issue 3 with a real constraint chain: the
  frozen oracle requires a *link* named `Cancel` (`getByRole('link')`), styling it
  as a button would change its ARIA role, `app.css` is outside this task's owns,
  and CSP (`style-src 'self'`) blocks inline styles. Not asserted by any visual
  note for A-205 → non-blocking, routed to the planner.
- **Map checks**: N/A for this task — T-205 adds no MapLibre map, no markers,
  clusters or attribution overlay; the walk "map" is a self-contained SVG pin pad
  (`#pinpad` in `web/templates/pages/admin-walk.html`), and no bottom sheet exists
  on any admin page (`base.html` and the public map files are untouched by the
  diff — verified: no hunks against `base.html`/`pages/index.html`/`partials/probe.html`).

## Diff-level layout/CSS correctness (file by file)

- `web/templates/pages/admin-walk.html` — `#walk-alert` is a bordered
  `<article role="alert">` sitting directly under `#walk-readout`, matching the
  A-204 note ("red alert box … near the position readout"); the retry's
  alert-box fix is visible in the structure (icon SVG + `#walk-alert-text`). The
  icons use SVG presentation attributes (`fill="#b3261e"`), not CSS, so CSP-safe.
  `#pinpad` is `width="100%"` over a square viewBox → ~316 px tall at 320 px, no
  overflow; the pin is `role="button" tabindex="0"` with arrow-key nudges (A-203
  keyboard requirement, and `NUDGE_STEPS` in `web/static/js/walk.js`). `data-bbox`
  on the section feeds the client-side pre-flight whose message mirrors the frozen
  server string (reconciliation 4). Hidden lat/lon/accuracy inputs keep server
  values so the no-JS error re-render (via `walkPageDataOf` in
  `internal/web/admin_create.go`) keeps the admin's typed values.
- `web/templates/partials/admin-dup.html` and the sync'd inline copy in
  `admin-walk.html` — numbered case renders `Merge` + `<a href="/admin/walk">Cancel</a>`,
  unnumbered adds `Save anyway`; both full-width pico buttons in the screenshots'
  rendering. The distance is always text ("m away"), asserted in
  `internal/web/admin_test.go`. The page/fragment duplication is kept honest by
  explicit `<!-- sync: … -->` comments; both paths are needed (no-JS full page vs
  htmx fragment).
- `web/templates/partials/admin-alert.html` — `role="alert"` gets the red
  triangle, `role="status"` a green check + `Saved`; the message is always the
  text label, never colour-only. `web/static/js/walk.js` overrides htmx 2.0.11
  `config.responseHandling` with `{ code: '[45]..', swap: true, error: false }` —
  the correct key/shape for that vendored version, so 422/409 bodies swap into
  `#dup-slot` (frozen `hx-target`/`hx-swap` on the form). File is 199 lines
  (≤ 200 budget), no console.log.
- `web/templates/pages/admin-detail.html` — the photo `width="320"` relies on
  pico's `img { max-width: 100% }` clamping; the handoff's programmatic
  no-overflow assertions on both projects cover this empirically. `#light-pos`
  carries the 6 dp string from `posString` (`internal/web/admin.go`), matching
  `#pin-pos`. `Delete light` is a separate form + outline button with a red
  warning icon → separated from `Save changes` per A-208, and destruction is
  signalled by shape+text (not colour-only), but the button itself is not red —
  partial vs the note "visually destructive (red) action"; documented as open
  issue 4 (`app.css` outside owns, CSP blocks the alternative). Non-blocking.
- `web/templates/pages/admin-list.html` + `partials/admin-rows.html` — rows are
  a 48×48 thumb + asset + label + status chip where the chip is dot **and** word
  (`status-unknown` grey dot + "unknown", asserted in `admin_test.go`); `Older`/
  `Newer` are pico buttons (full-width, ≥ 44 px) with both `formaction` (no-JS)
  and `hx-get`→`hx-target="#rows"` `hx-swap="outerHTML"`; the fragment root
  `id="rows"` matches the frozen contract, and `TestAdminRowsRedirectAndFragment`
  pins it. Empty state text present; no `Older` when ≤ 50 rows (boundary tested).
- `web/templates/pages/admin-login.html` — the styled login card exists and
  renders in `TestRenderAll` fixtures, but the live `/admin/login` still serves
  auth.go's unstyled minimal page (~21 px `Sign in`, below the A-201 44 px note).
  That file is frozen T-203 property and A-201 is T-203's claim — correctly not
  touched by T-205 and flagged as open issue 1 for the planner.

## Checklist summary

| Check | Result |
|---|---|
| 320 px + mobile layout | Pass (screenshot; overflow assertions claimed green ×2 runs) |
| Tap targets ≥ 44 px | Primary buttons pass; `Cancel` link ~20 px (documented, constraint-bound, non-asserted); pin handle ~34 px with keyboard alternative; live login button out of scope (T-203) |
| Contrast | Pass — dark-on-white; pad label `#5b6470` on `#f4f5f6` ≈ 5:1; primary buttons white-on-blue |
| Status = text + shape, not colour alone | Pass — alert/dup icons always with text; list chips dot + word |
| Markers / clusters / attribution | N/A (no MapLibre in this task); footer legible |
| Bottom sheet vs marker | N/A — no bottom sheet on admin pages |
| Clipped / overlapping text | None seen; long asset numbers wrap cleanly |

## Non-blocking notes

1. `Cancel` link below the 44 px floor — needs an `app.css` (or a11y-pattern
   reconciliation) decision by whoever owns CSS/a11y tasks; the frozen
   `role=link` requirement and the 44 px guidance pull in opposite directions.
2. `Delete light` not a red button — CSS-owns gap, needs a style task (handoff
   issue 4).
3. Dup-panel/alert markup duplicated between page template and partials with
   only comments as sync — acceptable now, fragile later; consider a partial
   include mechanism when templates are next touched.
4. Out of scope but flagged for the planner: live login page styling (handoff
   issue 1) and merge-not-applying-photo (handoff issue 2, a store-API gap —
   contract-functional, not visual).

The attached screenshot matches the A-202/A-203/A-205 visual notes; the diff is
internally consistent with the pico-based styling actually available, keeps all
status signalling text+shape, and every deviation from the ideal rendering is a
documented, constraint-bound exception outside this task's owns rather than a
layout bug.

VERDICT: PASS
