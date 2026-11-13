# T-205 — visual review (visual-reviewer)

Scope: all 22 PNGs under `test-results/` (16 visual scenarios × iphone-se-320 / pixel-9,
2 shots for several scenarios), compared against the per-assertion visual notes in
`acceptance/drafts/README.md` (A-201..A-209) and the task packet's scratch-suite
requirements. Cross-checked against `internal/web/admin.go` (row limit/pager) and the
template strings where the screenshots were too dense to OCR reliably.

## Per-file findings

### `visual-login-card-at-320-{iphone-se-320,pixel-9}/login.png` (A-201)
Single centred card; `<h1>Admin login</h1>`; labelled `Token` input (password-style
masked box); full-width `Sign in` button, ≥44 px tall; attribution footer in frame;
no horizontal scroll at 320 px or 412 px. Matches the note exactly.

### `visual-walk-page-layout-and-tap-targets-{iphone-se-320,pixel-9}/walk.png` (A-202/A-203)
`#walk-pos` renders `42.761700, -71.030300` at 6 dp with `accuracy 12 m` inline;
pin pad fills the middle with the `±25 m` scale label top-left; pin marker centred,
high-contrast, clearly a draggable target; `#pin-pos` mirrors the coords at 6 dp
under the pad; `Add light here` is full-width in the bottom third of the viewport
(iPhone capture is viewport-height and the button sits below the 2/3 line). Footer
visible, nothing clipped.

### `visual-low-accuracy-gate-and-save-anyway-{iphone-se-320,pixel-9}/walk-low-accuracy.png` (A-202)
At `accuracy 80 m`: `Add light here` is muted (platform `disabled`, asserted in-suite)
and `Save anyway (low accuracy)` appears as a full-width enabled button below. Gate
state is conveyed by the accuracy readout text plus the labelled unlock button, not
colour alone. Both buttons ≥44 px.

### `visual-geolocation-denied-manual-position-fallback-{iphone-se-320,pixel-9}/walk-manual.png` (A-202)
Pad hidden; `Latitude` / `Longitude` number inputs shown full-width with labels;
save held disabled; no overflow. Minor copy nit: the readout placeholders render as
"waiting for a position… manual position" / "waiting for a position…" — readable but
awkward phrasing; harmless, no action required for this task.

### `visual-outside-bbox-alert-{iphone-se-320,pixel-9}/walk-bbox-alert.png` (A-204)
Bordered role=alert panel with red triangle icon directly under the position readout:
"That position is outside Groveland town bounds" (frozen string). Status conveyed by
icon + text, not colour alone. Save stays disabled; the form does not advance. ✓

### `visual-photo-preview-and-detail-{iphone-se-320,pixel-9}/walk-photo-preview.png` (A-206)
`#photo-preview` renders the portrait-orient6 fixture **landscape** (wider than tall),
i.e. orientation applied, not a stretched box. Filename shows inline next to
`Choose File` — truncated at 320 px by the browser's native ellipsis, no overlap.
`Confirm pin` full-width at the bottom. ✓

### `visual-photo-preview-and-detail-{iphone-se-320,pixel-9}/detail-photo.png` (A-208)
Served photo landscape; `Position:` at 6 dp (`#light-pos`); `Kind: park · Status:
● unknown · Recorded accuracy: 12 m` — status chip is dot + text, not colour alone;
labelled edit form (Label, Asset number filled); `Save changes` primary; `Delete
light` separated below the save with a red warning triangle. **Deviation (minor,
documented in handoff §Open issues 2):** the Delete action is an outline button with
blue text + red icon, not a red button as the note sketches; `web/static/css/app.css`
is outside this task's `owns`, so the red icon + separation carry the destructive cue.
Accepted; flag for the CSS owner to redden later.

### `visual-duplicate-panel-and-merge-{iphone-se-320,pixel-9}/dup-panel-numbered.png` (A-205)
Amber-bordered `#dup-panel` with warning icon, heading `Possible duplicate`, the
EXISTING light (`Dup lamp`, asset `V-…-pixe-dup`, "about 0.0 m away"), full-width
`Merge`, `Cancel` link, **no `Save anyway`** — numbered case per reconciliation 2.
Panel appears in-flow above the form fields; nothing saved until chosen. ✓

### `visual-duplicate-panel-and-merge-{iphone-se-320,pixel-9}/list-after-merge.png` (A-205)
List shows exactly ONE row for the merged pair; survivor carries the submitted twin
label ("Dup lamp twin") with thumbnail alt in the frozen `<asset_no> <label>` format;
the photo light renders its re-encoded thumbnail. Merge applied, not discarded. ✓

### `visual-admin-list-paginates-with-htmx-{iphone-se-320,pixel-9}/list-page1.png`, `list-page2.png` (A-209)
Heading `Lights`; small square thumbnail per row (bulb placeholder for photo-less
lights — allowed); label links to detail; `● unknown` status chip (text + shape);
full-width `Older`; page 2 adds `Newer` above it (exact frozen strings verified in
`partials/admin-rows.html` / `pages/admin-list.html`). Pixel-9 page 1 is a clean
50-row page (pixe 55–06); pixel-9 post-merge page is dup/photo + ipho 55–08 = 50;
page 2 continues gapless (pixe 05–01 → dup/photo → ipho …). No horizontal scroll at
320 px; 50-row cap cross-checked in `internal/web/admin.go` (`adminPageSize = 50`,
51-row probe trimmed at `rows[:adminPageSize]`) and pinned by
`TestRecentLights` / `TestAdminPaginationBoundary`. Note: the iphone page-1/page-2
pair shows the dup/photo fixture pair on BOTH pages — leftovers from an earlier
scratch invocation against the persistent `test-run.db` (the drafts' rerun-safety
sweep only covers the drafts' own seed prefixes), not a rendering/ordering bug; the
scratch DB is throwaway and gitignored.

## Checklist

- **320 px / mobile layout**: no horizontal overflow on any capture; all text wraps;
  footer disclaimer legible and unobstructed in every shot. ✓
- **Tap targets ≥44 px**: all buttons qualify (`Sign in`, `Add light here`,
  `Save anyway (low accuracy)`, `Merge`, `Confirm pin`, `Save changes`,
  `Delete light`, `Newer`, `Older`, `Choose File`). Text links under the floor:
  header nav (~28 px), dup-panel `Cancel` (~24 px, frozen as a link by the contract —
  handoff open issue 1), per-row label links (row pitch ~44–48 px, the link line
  itself smaller). Meets WCAG 2.5.8 AA (24 px), not AAA (44 px). Minor.
- **Contrast**: body #1f2937-ish on white ✓; teal/blue links ≥4.5:1 ✓; grey
  "unknown" chip ≈4.6:1, paired with the dot ✓; disabled `Add light here` is low
  contrast but is a disabled control with a text label and an adjacent enabled
  alternative. Acceptable.
- **Status ≠ colour alone**: dot + "unknown" text; alert = icon + text; accuracy
  gate = readout + labelled unlock button. ✓
- **Map markers / clusters / attribution / bottom sheet**: N/A — T-205 admin pages
  contain no MapLibre map (pin pad is a local control); no attribution to obscure.
- **Clipped/overlapping text**: none observed; filename ellipsis is native and
  non-overlapping; headings wrap cleanly at 320 px.
- **`Saved` success toast**: not among the captures (asserted by Go/e2e tests
  instead); the visual evidence covers the 409/422 and list states. Evidence gap
  only, contract is test-asserted.

## Verdict

All frozen strings, formats, panel/button permutations, orientation and layout
requirements verified visually on both mobile projects; the two minor deviations
(blue-texted destructive button, sub-44 px contract-frozen links) are documented in
the handoff and none violate the frozen contract.

VERDICT: PASS
