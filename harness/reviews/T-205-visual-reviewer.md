# T-205 — visual review (all 24 screenshots under `test-results/` inspected)

Reviewed every PNG on disk against `acceptance/drafts/README.md` per-assertion
visual notes (A-202, A-203, A-204, A-205, A-206, A-208, A-209) at both mobile
viewports (pixel-9 `*-p9`, iphone-se-320 `*-se`). The narrowest layout (320 px)
shows no horizontal overflow anywhere: long asset numbers wrap at hyphens, list
rows wrap to two lines, nothing is clipped or overlapping.

## Walk page — `visual-walk-page-layout-p9/walk-page-layout/walk-p9.png`, `visual-walk-page-layout-se/walk-page-layout/walk-se.png`
- `#walk-pos` "42.761700, -71.030300" + "accuracy 12 m" readable at 6 dp; pin
  pad with "±25 m" scale label and a clearly visible, centred pin marker;
  `#pin-pos` mirrors the coords below the pad; "Add light here" is full-width,
  in the bottom third, ≥ 44 px tall at both viewports. Footer text legible. ✓
- The pad is a schematic SVG scale (no third-party tiles), so no map
  attribution applies; no bottom sheet on this page, so nothing covers the pin.

## Low-accuracy gate — `visual-low-accuracy-gate-and-real-save-at-80-m-p9/…/walk-lowacc-p9.png`, `…-se/…/walk-lowacc-se.png`
- "accuracy 80 m" stated in text; "Add light here" rendered muted/disabled
  while "Save anyway (low accuracy)" sits full-width below it — the gated state
  is conveyed by text + an explicit labelled alternative, not colour alone. ✓
- Note: the disabled button's white-on-pale-blue fill is low contrast, but
  disabled controls are contrast-exempt and the platform `disabled` attribute +
  the alternative button carry the meaning. Acceptable.

## Geolocation denied — `visual-geolocation-denied-shows-manual-pin-fields-p9/…/walk-denied-p9.png`, `…-se/…/walk-denied-se.png`
- Pad hidden, `#manual-lat` / `#manual-lon` labelled number inputs shown,
  button correctly held disabled; no overflow at 320 px. ✓
- Cosmetic: the strip reads "waiting for a position… manual position" and
  `#pin-pos` still prints "waiting for a position…" beneath — honest, legible,
  non-overlapping, but slightly redundant copy. Not a defect.

## Outside-bbox alert — `visual-outside-bbox-shows-the-frozen-alert-p9/…/walk-bbox-alert-p9.png`, `…-se/…/walk-bbox-alert-se.png`
- Bordered alert near the readout: red warning-triangle icon + "That position
  is outside Groveland town bounds" — shape AND text, not colour alone; wraps
  cleanly at 320 px; form does not advance (button disabled). Matches A-204. ✓

## Duplicate panels — `visual-numbered-duplicate-panel-merge-and-cancel-p9/…/dup-numbered-p9.png`, `…-se/…/dup-numbered-se.png`, `…/dup-merged-list-*.png`; `visual-unnumbered-duplicate-panel-save-anyway-p9/…/dup-unnumbered-p9.png`, `…-se/…/dup-unnumbered-se.png`
- Amber-icon bordered `#dup-panel`, heading "Possible duplicate", existing
  label + asset number + distance shown. Numbered case: `Merge` + `Cancel`
  link, NO "Save anyway" (frozen contract honoured). Unnumbered: `Merge` +
  `Save anyway`, both full-width ≥ 44 px. Post-merge list shows exactly one
  row carrying the merged label. Matches A-205. ✓
- Minor: the `Cancel` link is a ~24 px text link, below the 44 px floor. The
  frozen contract freezes it as `role=link` named `Cancel`, so styling it as a
  button would break the oracle; it is a secondary escape hatch adjacent to the
  44 px `Merge` button. Flagged in the handoff (open issue 3). Acceptable.

## Photo preview / detail — `visual-photo-preview-is-landscape-detail-page-intact-p9/…/{walk-preview-p9,list-p9,detail-p9}.png`, `…-se/…/{walk-preview-se,list-se,detail-se}.png`
- `#photo-preview` renders the portrait Orientation=6 fixture landscape
  (wider than tall) at both viewports — A-206 satisfied. List rows carry a
  small square thumbnail with the frozen alt format + status chip (dot AND
  "unknown" text). Detail shows the landscape served photo, 6-dp position,
  labelled edit form, `Save changes` ≥ 44 px separated from `Delete light`
  (outline button with red warning-triangle icon). File-input filename
  truncates with ellipsis without colliding with "Choose File". ✓
- Deviation (noted, not blocking): the A-208 visual note asks for a
  *visually destructive (red)* delete action; the button is an outline with a
  red warning icon instead. `app.css` is outside this task's owns and CSP
  blocks inline styles, so a red variant needs a CSS task — already flagged in
  the handoff (open issue 4). Destructiveness is still conveyed by icon + text
  + separation, which is stronger than colour-alone anyway.

## List pagination — `visual-admin-list-paginates-with-htmx-p9/…/list-page{1,2}-p9.png`, `…-se/…/list-page{1,2}-se.png`
- Page 1: 50 seeded rows (55→06), visible thumbnails, status chips with dot +
  text, no 320 px overflow, full-width `Older` ≥ 44 px. Page 2: rows 05→01
  swapped in with `Newer` offered back. Matches A-209. ✓

## Checklist summary
- 320 px / mobile layouts: no clipping, no overlap, no horizontal scroll. ✓
- Tap targets ≥ 44 px: all buttons pass; exceptions are the frozen `Cancel`
  text link (handoff issue 3) and the pre-existing small header nav links from
  `base.html` (unchanged by this diff). Noted.
- Contrast: dark-on-white body, pico-blue links, legible muted labels;
  disabled-state contrast exempt and textually backed. ✓
- Status via text + shape, never colour alone: alert icon + text, dot + text
  chips, delete warning icon + text, gate = disabled attribute + labelled
  alternative button. ✓
- Map markers/clusters/attribution: public map not in this task; walk pad pin
  visible and centred everywhere; no clusters, no tile attribution required. ✓
- Bottom sheet: not rendered on these pages; nothing occludes the pin. ✓

Findings are notes only; every one is either frozen-contract-mandated or
outside the task's owns and already flagged in `harness/handoffs/T-205.md`.

VERDICT: PASS
