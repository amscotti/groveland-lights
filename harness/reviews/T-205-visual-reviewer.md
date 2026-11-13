# T-205 visual review (round-4 retry)

Reviewer: visual-reviewer (GLM-5.3-Flash) · Date: 2026-11-13
Input: 8 screenshots under `test-results/` (all present, none empty) + cross-check
against `web/templates/pages/admin-{walk,list,detail}.html`,
`partials/admin-{dup,alert}.html`, `web/static/css/app.css`, and the handoff's
visual-evidence claims.

## Per-screenshot findings

### t205-walk-iphone-se-320.png / t205-walk-pixel-9.png
- 320 px: no horizontal overflow; header ("Groveland Lights | Map List About")
  fits; `#walk-pos` + accuracy readout ("42.761700, -71.030300 accuracy 8 m") on
  one line, not clipped; footer wraps cleanly.
- Pin pad renders as the synthetic SVG `#pinpad` (200×200 viewBox, grey grid,
  `±25 m` label top-left in `#5b6470` on `#f4f5f6` — legible). The pin (dark
  blue disc, white ring + centre dot, `role="button"`, `aria-label="position
  pin"`) is visible, centred, and distinguishable at both viewports.
- `#pin-pos` echoes 6-dp coords; `Add light here` is full-width and tall
  (visually ≥ 44 px) in the lower third as claimed.
- This page is a pin pad, not MapLibre tiles → no tile attribution, no markers,
  no clusters on these screens (see nit 3 below).

### t205-dup-iphone-se-320.png / t205-dup-pixel-9.png
- Amber warning triangle (`#b45309`) + "Possible duplicate" heading: status is
  conveyed by shape + heading text, not colour alone. ✔
- Panel names the EXISTING light ("Visual lamp … (asset number VIS-…), about
  0.0 m away") — no echo of the submitted twin label. ✔
- Numbered case: full-width `Merge` button + `Cancel` link, no `Save anyway`. ✔
  Matches both screenshots.
- The add form below the panel is revealed (Label/Asset/Kind/Photo/Confirm pin),
  matching the 409/422 reveal behaviour. All inputs full-width, legible.
- 320 px: all text wraps inside the panel, nothing clipped or overlapping.

### t205-list-iphone-se-320.png / t205-list-pixel-9.png
- Two rows: photo-thumbnail row (landscape gradient thumb, 48×48) and a
  placeholder row (grey bulb icon) — both visible and distinguishable.
- Each row: asset number + bold label as one link, then `● unknown` — the grey
  dot is `aria-hidden` and always paired with the word "unknown"
  (`.status .dot` + `{{.Status}}` in `admin-rows`/`app.css`). Never
  colour-only. ✔ Confirmed in all four list/detail screenshots.
- 320 px: label wraps under the asset number without clipping or overlap;
  row separators fine.

### t205-detail-iphone-se-320.png / t205-detail-pixel-9.png
- The portrait-orientation upload renders landscape (image clearly wider than
  tall at both viewports), matching the handoff's `naturalWidth > naturalHeight`
  assertion.
- `Position:` shows 6 dp; Kind/Status/accuracy line wraps cleanly at 320 px
  ("Kind: park · Status: ● unknown · Recorded accuracy: 8 m").
- Edit form labelled (`Label`, `Asset number`), `Save changes` full-width
  filled, `Delete light` a separate form with an outline button + red warning
  icon — visually separated from Save, destructive state signalled by icon +
  text, not fill colour alone. ✔
- h1 "Photo lamp 577168" wraps to two lines at 320 px — intentional wrap, no
  clipping.

## Checklist results
- Layout at 320 px + mobile viewports: PASS (no overflow, no clipping, wraps
  cleanly on all four pages at both sizes).
- Tap targets ≥ 44 px: PASS for every button (Add light here, Confirm pin,
  Merge, Save changes, Delete light, Save anyway) at both viewports. Two
  sub-44 px exceptions, both pre-declared: (1) the `Cancel` text link in the
  dup panel — frozen by the contract as a link named `Cancel` (handoff open
  issue 2); (2) header nav links live in `base.html`, outside this task's
  `owns`.
- Contrast: PASS. Body text dark-on-white; button text white on dark blue;
  `±25 m` label readable; muted footer (opacity 0.7) still legible.
- Status via text + shape, not colour alone: PASS everywhere (`● unknown` with
  the word always present; alert/dup panels pair icons with headings/text).
- Map markers / clusters / attribution: N/A on these admin pages — the walk
  page is a synthetic pin pad (no tiles). The one visible pin is labelled and
  distinguishable. Public-map attribution/clusters are outside this task's
  screenshots.
- Bottom sheet vs selected marker: N/A (`.bottom-sheet` defined in app.css but
  not rendered on any of these pages).
- Clipped / overlapping text: none found in any of the 8 screenshots.

## Minor notes (non-blocking)
1. `Cancel` link in the dup panel is < 44 px tall — accepted deviation, frozen
   by contract and documented in the handoff; flagged for any future
   links-vs-buttons contract change.
2. Native `Choose File` file-input row is visually a bit shorter than 44 px;
   the handoff's programmatic 44×44 check over `#main` covers it, and it is a
   native control. Monitor only.
3. Handoff wording nit: it says the walk screenshots show "attribution
   visible" — strictly, no attribution renders because the pin pad is an SVG,
   not a map with tiles. Cosmetic documentation inaccuracy, not a visual
   defect.
4. Fixture IDs differ between the two Playwright runs (577168 on
   iphone-se-320, 573081 on pixel-9) — different seeded runs, not a defect.

## Assertion cross-check
Every claim in the handoff's "Visual evidence" section is observable in the
corresponding screenshot: walk (±25 m pad, labelled pin, 6-dp readout,
full-width button), dup (amber icon + heading, existing light's data, Merge +
Cancel, no Save anyway), list (landscape photo thumb + placeholder row,
asset + label, dot + word), detail (landscape render of portrait upload, 6-dp
position, labelled form, separated outline delete, no 320 px overflow).

VERDICT: PASS
