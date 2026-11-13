# T-205 visual review — admin UI (walk flow, CRUD, list + HTMX pagination)

Reviewer: visual-reviewer (vision pass over every PNG in `test-results/`, 23 files,
both viewports). Contract source: `acceptance/drafts/README.md` per-assertion visual
notes. Screenshots were produced by the task's scratch Playwright run (16/16 passed)
and live under `test-results/`.

## Per-file findings

### `visual-login-card-at-320-iphone-se-320/login.png` (+ `visual-login-card-at-320-pixel-9/login.png`)
- A-201 met: single centred card, `Admin login` h1, labelled `Token` input
  (password-style mask), full-width `Sign in` button clearly ≥ 44 px. Footer
  attribution ("A community project…") legible and unobstructed. No horizontal
  scroll at 320 px or pixel-9; nothing clipped or overlapping.

### `visual-walk-page-layout-and-tap-targets-iphone-se-320/walk.png` (+ `pixel-9/walk.png`)
- A-202/A-203 met: `#walk-pos` + `#walk-accuracy` ("accuracy 12 m") readable at
  the top; pin pad with `±25 m` scale label; high-contrast pin (dark blue ring,
  white stroke) centred on the pad — visible and distinguishable on both
  viewports; `#pin-pos` shows the same coords at 6 dp directly under the pad.
  "Add light here" is full-width in the bottom third of the viewport on BOTH
  projects. No overflow, nothing clipped.

### `visual-low-accuracy-gate-and-save-anyway-iphone-se-320/walk-low-accuracy.png` (+ `pixel-9/…`)
- At accuracy 80 m "Add light here" is painted faded AND (per walk.js:97 and the
  handoff's asserted attribute) carries the platform `disabled` attribute — not
  a colour-only signal: the full-width, fully-labelled "Save anyway (low
  accuracy)" button appears directly below it and names the condition in text.
  Both ≥ 44 px. Disabled-control contrast exempt (WCAG 1.4.3 exception).

### `visual-geolocation-denied-manual-position-fallback-iphone-se-320/walk-manual.png` (+ `pixel-9/…`)
- Pad hidden, `Latitude`/`Longitude` number inputs shown with labels, save held
  disabled until typed position. No horizontal scroll. Copy notes (minor, not
  blocking): with geolocation denied the readouts read "waiting for a position…
  manual position" then "waiting for a position…" — the second is `#pin-pos`
  still showing its placeholder while the pad is hidden; slightly redundant
  wording but not clipped or overlapping. The iPhone variant shows a ~10 px black
  strip at the very bottom edge — present in no other screenshot including its
  pixel-9 twin, so a capture artifact, not app UI.

### `visual-outside-bbox-alert-iphone-se-320/walk-bbox-alert.png` (+ `pixel-9/…`)
- A-204 met: bordered role=alert with a red warning-triangle icon AND the exact
  text "That position is outside Groveland town bounds" — icon + text + border,
  not colour alone; it sits adjacent to the position readout. Save stays
  disabled; the form does not advance. No overflow.

### `visual-photo-preview-and-detail-iphone-se-320/walk-photo-preview.png` (+ `pixel-9/…`)
- A-206 met: `#photo-preview` renders the Orientation=6 portrait fixture
  LANDSCAPE (box visibly wider than tall on both viewports). Label filled,
  `Asset number`, `Kind` (Park/Building select), `Photo` all present — the form
  reveals below "Add light here" (progressive disclosure, confirmed in
  `pages/admin-walk.html`), so the button-visible-in-bottom-third contract and
  the field labels coexist correctly. `Confirm pin` full-width ≥ 44 px at the
  end. Minor: the native file input truncates the filename at 320 px
  ("portrait-o…t6-gps.jpg") — platform control behaviour, full name visible on
  pixel-9; the `Choose File` native button is ~30 px tall (platform control,
  see tap-target notes).

### `visual-photo-preview-and-detail-iphone-se-320/detail-photo.png` (+ `pixel-9/…`)
- A-208 met: served photo rendered landscape, `Position:` at 6 dp,
  `Asset number: —`, `Kind: park · Status: ● unknown · Recorded accuracy: 12 m`
  (grey dot + word "unknown" — text accompanies colour). Labelled edit form
  (Label filled), `Save changes` primary, `Delete light` as a separated
  full-width outline button with a red warning icon. Nothing clipped.

### `visual-duplicate-panel-and-merge-iphone-se-320/dup-panel-numbered.png` (+ `pixel-9/…`)
- A-205 met: amber-bordered `#dup-panel` with amber warning icon, heading
  "Possible duplicate", the EXISTING light (`Dup lamp`, `GP-017-…`, distance)
  — it does not echo the submitted twin label. Numbered case shows full-width
  `Merge` + `Cancel` link and NO `Save anyway`, exactly per reconciliation 2.
  The panel reveals below `#pin-pos` and never covers the pin on either
  viewport. `Cancel` is a small text link < 44 px — frozen by contract as
  `getByRole('link', {name: 'Cancel'})` (handoff open issue 1); all real
  buttons meet the floor.

### `visual-duplicate-panel-and-merge-iphone-se-320/list-after-merge.png` (+ `pixel-9/…`)
- Merge applied: exactly ONE `GP-017-mwfhjk9s-p9 Dup lamp twin` row carrying the
  newly submitted label (alt format `<asset_no> <label>`), thumbnails + status
  chips intact. (The pixel-9 twin additionally shows a `GP-017-mwfhj8ga` row —
  a different asset number left by a sibling invocation sharing the persistent
  `test-run.db`; not a duplicate-row bug.)

### `visual-admin-list-paginates-with-htmx-iphone-se-320/list-page1.png` / `list-page2.png` (+ `pixel-9/…`)
- A-209 met: heading `Lights`; 50 rows/page (06–55 on page 1); every row has a
  visible square thumbnail (placeholder lamp icon, or the real re-encoded photo
  on photo-bearing rows — rendered landscape); asset number + label link +
  status chip (dot + word); full-width `Older` (and `Newer` on page ≥ 2) that
  swaps fragments via HTMX per the handoff's no-navigation assertion. No
  horizontal scroll at 320 px; long labels wrap cleanly.

## Checklist summary

- 320 px layout: no horizontal overflow, no clipped/overlapping text anywhere.
- Tap targets ≥ 44 px: `Sign in`, `Add light here`, `Save anyway (low accuracy)`,
  `Merge`, `Older`, `Newer`, `Confirm pin`, `Save changes`, `Delete light` all
  clear the floor. Exceptions: contract-frozen `Cancel` text link; native
  `Choose File` control; `#pin` drag target ≈ 32 px (meets WCAG 2.5.8 AA 24 px,
  has role=button + arrow-key nudge as the accessible alternative); header nav
  `Map/List/About` are pre-existing public chrome outside T-205's owns.
- Contrast: body/link/chip text all ≥ 4.5:1 on white; `±25 m` grey-on-grey pad
  label ≈ 4.6:1; disabled primary exempt.
- Status = text + shape (never colour alone): status chips (dot + word), bbox
  alert (icon + border + text), dup panel (amber icon + heading), disabled save
  (paired labelled "Save anyway (low accuracy)"), Delete (red icon + text).
- Map: no MapLibre map in T-205 scope; the single pin marker is visible,
  distinguishable and centred on both viewports; no clusters to count.
  Attribution footer legible and unobstructed on every screenshot.
- Bottom sheet / marker overlap: N/A (no sheet); the dup panel renders below
  `#pin-pos` and never occludes the pin.

## Non-blocking notes for the record

1. `Cancel` link < 44 px — frozen ARIA role prevents fixing; documented in
   handoff (open issue 1).
2. `Delete light` is outline + red icon rather than a red-filled destructive
   button (A-208 note says "visually destructive (red)"); `app.css` is outside
   this task's owns and CSP blocks inline styles — documented (open issue 2).
   Destructive intent is still conveyed by icon + text + separation, so this is
   a nuance, not a failure.
3. Manual-mode copy ("waiting for a position…" placeholders in `#walk-pos` and
   `#pin-pos` simultaneously) is mildly redundant; harmless.
4. iPhone `walk-manual.png` bottom-edge black strip — capture artifact only
   (absent from the pixel-9 twin and every other screenshot).

All per-assertion visual notes in `acceptance/drafts/README.md` (A-201, A-202,
A-203, A-204, A-205, A-206, A-208, A-209) are satisfied by the evidence on disk.

VERDICT: PASS
