# T-205 visual review (round 5 retry)

Reviewer: visual-reviewer agent. Evidence: the 10 PNGs under `test-results/`
(all read with vision), compared against the per-assertion visual notes in
`acceptance/drafts/README.md`, the frozen contract in
`acceptance/drafts/admin-walk.spec.ts.txt` / `README.md`, and the committed
templates `web/templates/pages/admin-walk.html`, `pages/admin-detail.html`,
`pages/admin-list.html`, `partials/admin-dup.html`, `partials/admin-rows.html`,
`base.html`, plus `web/static/css/app.css` and the vendored `pico-2.1.1.min.css`.

Prior BLOCK (Cancel tap target < 44 px; missing screenshot evidence) is
addressed: 10 screenshots now exist, and `partials/admin-dup.html` /
`pages/admin-walk.html` render Cancel as a block-level anchor containing a
`<svg width="1" height="44">` spacer (44 px border box, visible text
unchanged, role still `link`).

## Findings by file

### t205-walk-iphone-se-320.png / t205-walk-pixel-9.png (A-202/A-203)
- No 320 px overflow; header, readout, pad, readout repeat and button all fit.
- `#walk-pos` (`42.762700, -71.010000`) and `#walk-accuracy`
  (`accuracy 8 m`) readable at 320 px; 6 dp form matches the frozen contract.
- Pin pad SVG: `±25 m` scale text (#5b6470 on #f4f5f6) legible; pin marker
  (#0b5c8f + white ring/centre) clearly visible and distinguishable at both
  viewports, centred per A-203. `#pin-pos` mirrors the coords under the pad.
- `Add light here` is full-width and in the bottom third at 320×640 —
  thumb-reachable, ≥ 44 px tall.
- No real MapLibre map on admin pages (pin pad by design), so map attribution
  is not applicable here; the footer disclaimer is legible and unobstructed on
  every shot.

### t205-lowacc-iphone-se-320.png / t205-lowacc-pixel-9.png (A-202 gate)
- `Add light here` renders washed out (Pico `opacity:.5` disabled treatment)
  and — per template + handoff measurement — carries the platform `disabled`
  attribute; the frozen contract requires the attribute, not a look. ✓
- `Save anyway (low accuracy)` appears below it, full blue, enabled, ≥ 44 px.
  The gated state is conveyed by the distinct labelled second button, i.e.
  text, not colour alone. White-on-light-blue text of the disabled button is
  low contrast, but inactive controls are exempt and the gate has a fully
  readable alternative path — acceptable.
- `accuracy 85 m` readout legible; no overflow at 320 px.

### t205-dup-iphone-se-320.png / t205-dup-pixel-9.png (A-205 numbered case)
- Amber-bordered panel with warning-triangle SVG (#b45309 — colour AND shape),
  heading `Possible duplicate`, existing light named in bold with asset number
  and distance. Matches the visual note.
- Buttons: `Merge` (primary, ≥ 44 px) + `Cancel` link and NO `Save anyway` —
  exactly the frozen numbered-case contract.
- `Cancel` now shows a generous hit area (44 px spacer verified at 44 px on
  both projects per handoff; visually the row has clear whitespace below the
  link). Prior blocker resolved.
- Form below (Label / Asset number / Kind / Photo / `Confirm pin`) intact,
  nothing clipped or overlapped at 320 px.

### t205-list-iphone-se-320.png / t205-list-pixel-9.png (A-209)
- Heading `Lights`; rows with thumbnails that are clearly visible (~48×36)
  with the asset number + label as the link text, and a status chip pairing a
  grey dot with the word `unknown` — status is never colour-only. ✓
- No horizontal scroll at 320 px; two rows (320 project) / one row (pixel
  project) per each project's own seeded DB — not a defect.
- No `Older` pager: < 50 rows in the runtime DB, so the conditional pager is
  correctly absent (A-209's HTMX path is exercised by the frozen specs, not by
  this fixture).
- Nit: thumbnails are 48×36 landscape, not the "small square" of the prose
  note. Pico's `height:auto` forces this and the packet's `owns` excludes
  `web/static/css/**`; documented as handoff open issue 4. The frozen
  selector table only requires a visible thumbnail with the alt format, which
  is met. Not a blocker.

### t205-detail-iphone-se-320.png / t205-detail-pixel-9.png (A-208)
- Photo renders (fixture gradient, landscape 800×600); `Position:` shows the
  6 dp `#light-pos`; asset number; `Kind: park · Status: ● unknown · Recorded
  accuracy: 8 m` with dot+text pairing. ✓
- `Save changes` (primary) and `Delete light` (outlined, red warning-triangle
  icon, separate form below) — visually separated from the save button. ✓
  Nit: the delete label text itself is Pico primary blue, not red; only the
  icon is red. The task owns no CSS and CSP is `style-src 'self'` (no inline
  styles), so a red-labelled button is not reachable within ownership; the
  frozen contract table does not require red text, the icon provides colour +
  shape, and the destructive choice is separated. Acceptable; noted for a
  future CSS-owning task.
- Title `Ball field twin SLRBPH` wraps to two clean lines at 320 px — no
  clipping, no overlap.

## Checklist sweep
- 320 px layout: pass on all five 320 px shots (no overflow, no clipped or
  overlapping text anywhere).
- Tap targets ≥ 44 px: all task-owned buttons pass; `Cancel` passes via the
  44 px spacer. Header nav links (Map/List/About) are sub-44 px text links in
  `base.html`, which this task is forbidden to touch — pre-existing chrome,
  out of scope here, worth a note to the base owner. The list thumbnail link
  is ~36 px tall but the row's primary link is the adjacent label link with
  cell padding; minor.
- Contrast: status dots always paired with text; warning icons ≥ 3:1 plus
  text; `±25 m` scale text readable; disabled button exempt and doubly
  conveyed.
- Status/shape over colour: pass everywhere (dot+text, icon+heading,
  disabled attribute + alternate button).
- Map markers/clusters/attribution/bottom sheet: N/A on admin pages (pin pad,
  no MapLibre, no bottom sheet); nothing obstructs the footer on any shot.
- Not visually captured (residual evidence gap, not a blocker): the
  unnumbered duplicate panel (`Merge` + `Save anyway`) and the A-206 in-form
  photo preview; the handoff's instrumented flow run asserts the preview's
  intrinsic 800×600 (orientation applied) and the Go drafts pin the rest.

The prior blockers are resolved, every frozen-contract element visible in the
screenshots matches, and the remaining nits are ownership-constrained,
documented in the handoff, and none breaks the frozen contract or harms the
mobile experience.

VERDICT: PASS
