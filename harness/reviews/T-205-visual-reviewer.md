# T-205 visual review (round 5 retry)

Reviewed all 10 PNGs under `test-results/` (both mobile projects, five states). Compared against the handoff's visual notes and the prior round's blockers (Cancel tap target, missing screenshot evidence).

## Per-file findings

- `t205-walk-iphone-se-320.png` — 320 px walk page, accuracy 8 m. No horizontal overflow; coordinate readout `42.762700, -71.010000 accuracy 8 m` legible; single blue donut marker centered and clearly distinguishable on the ±25 m grid; `Add light here` full-width, ~48 px tall, in the lower third; footer disclaimer visible and unobstructed. OK.
- `t205-walk-pixel-9.png` — same state at Pixel 9 width. Marker centered, button ≥ 44 px, nothing clipped. OK.
- `t205-lowacc-iphone-se-320.png` — accuracy 85 m. `Add light here` visibly disabled (pale) AND the enabled `Save anyway (low accuracy)` button below states the reason in text — state is not colour-alone. Both buttons ≥ 44 px. OK.
- `t205-lowacc-pixel-9.png` — same; disabled button and text-labelled fallback both present, no overlap. OK.
- `t205-dup-iphone-se-320.png` — numbered duplicate panel at 320 px. Amber panel with ⚠ glyph + `Possible duplicate` heading (shape + text, not colour alone); body names the existing light (`Ball field lamp SLRBPH`, asset, `about 0.0 m away`); `Merge` ≥ 44 px; `Cancel` link has the surrounding whitespace consistent with its 44 px spacer box (matches the measured 44 px evidence); form fields, `Confirm pin` ≥ 44 px; no clipping or overflow. OK.
- `t205-dup-pixel-9.png` — same state; panel, Merge, Cancel all clean at width. OK.
- `t205-list-iphone-se-320.png` — 320 px list. Both rows show the 48×36 thumbnail (visible, matches handoff), asset + bold label as a two-line link (large tap area), status as gray dot glyph + the word `unknown` — never colour alone. No overflow. OK.
- `t205-list-pixel-9.png` — shows only the PLRAPR row vs two rows at 320. Explained by run order (pixel-9 project ran first against the fresh DB, before SLRBPH was seeded), not a rendering defect; the single row renders correctly. OK.
- `t205-detail-iphone-se-320.png` — detail/edit at 320 px. Title wraps cleanly to two lines (no clipping); photo full width; `Position:` / asset / `Kind: park · Status: ● unknown · Recorded accuracy: 8 m` all legible with dot + text status; `Save changes` full-width ≥ 44 px; `Delete light` outlined with ⚠ glyph, visually separated; footer visible. OK.
- `t205-detail-pixel-9.png` — same at width; one-line title, everything legible, no overlap. OK.

## Checklist synthesis

- 320 px layout: no horizontal overflow, no clipped or overlapping text in any of the five states on either project.
- Tap targets: all task-owned controls (`Add light here`, `Save anyway (low accuracy)`, `Merge`, `Cancel`, `Confirm pin`, `Save changes`, `Delete light`, list row links) are ≥ 44 px or measured 44 px per the handoff's Playwright evidence; screenshots show proportionate boxes.
- Contrast: body/link/button text passes. The disabled `Add light here` is low-contrast white-on-pale-blue, which is exempt for disabled controls and redundant with the textual low-accuracy fallback.
- Status semantics: every state uses a glyph/shape plus text (gray dot + `unknown`, ⚠ + `Possible duplicate`, `(low accuracy)` label, ⚠ + `Delete light`). No colour-only signalling anywhere.
- Markers: single blue donut marker visible and distinguishable on the walk/lowacc/dup grids on both viewports. No clustered-marker UI and no bottom sheet exists on these admin pages (they use the static ±25 m picker), so cluster counts and marker/sheet interplay are N/A for this set. The grid placeholders carry no tile attribution (none required); the footer disclaimer is legible and unobstructed on all 10 captures.

## Prior blockers

- Cancel tap target: resolved (spacer box; measured 44 px on both projects; visual spacing corroborates).
- Missing screenshot evidence: resolved (all 10 PNGs present and current with the committed templates).

## Minor observations (non-blocking)

1. Header nav `Map List About` links are ~20 px tall on mobile — shared `base.html` chrome outside this task's owned files; flag for the owning/base-template task.
2. Native `Choose File` button is ~32 px tall at 320 px (browser-drawn control; minor).
3. The `±25 m` scale label is small gray-on-gray but legible and decorative.
4. Rainbow-gradient blocks in photos/thumbnails are the test fixture image, not a rendering bug.

VERDICT: PASS
