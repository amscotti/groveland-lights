# T-205 — visual review (visual-reviewer, round-4 retry)

Scope: the 8 kept PNGs under `test-results/` (4 states × iphone-se-320 / pixel-9),
each inspected with vision, compared against the per-assertion visual notes in
`acceptance/drafts/README.md` (A-202/203/205/206/208/209) and the packet's
320 px / ≥44 px / no-overflow requirements. Claims cross-checked against the
owned templates (`pages/admin-{walk,detail}.html`, `partials/admin-dup.html`)
and `web/static/css/app.css` where a screenshot alone was ambiguous. This file
supersedes the round-3 review (which covered the earlier, richer 22-shot scratch
suite; the round-4 retry regenerated the kept set).

## Per-file findings

### `t205-detail-iphone-se-320.png` / `t205-detail-pixel-9.png` (A-208, A-206)
- Photo renders **landscape** (wider than tall) at both widths — orientation
  applied, matching the portrait-orient6 requirement (intrinsic landscape is
  asserted in-suite; the visible box corroborates it).
- `Position: 42.761700, -71.030300` — `#light-pos` at 6 dp ✓.
- `Kind: park · Status: ● unknown · Recorded accuracy: 8 m` — status is dot
  **plus the word "unknown"**, never colour alone ✓.
- Labelled edit form (`Label`, `Asset number` filled), full-width `Save changes`.
- `Delete light`: its own form, below `Save changes`, outline style with a red
  warning triangle. **Deviation (minor, disclosed in handoff open issue 3):** the
  note says "visually destructive (red)"; here only the 16 px icon is red and the
  text/border are blue. Confirmed not fixable in-scope: `app.css` has no
  destructive class (only an htmx-indicator comment) and is outside this task's
  `owns`; CSP blocks inline styles, so the template cannot redden it. Separation
  + icon carry the destructive cue. Flag for the CSS owner.
- No clipped or overlapping text; title wraps cleanly at 320 px; no horizontal
  overflow (photo fits edge-to-edge).

### `t205-dup-iphone-se-320.png` / `t205-dup-pixel-9.png` (A-205)
- Amber `#dup-panel` with warning-triangle icon and `Possible duplicate` heading;
  shows the **EXISTING** light: `Visual lamp 577168 (asset number VIS-577168),
  about 0.0 m away` — label + asset + distance ✓; no echo of the submitted twin
  label ✓.
- Numbered case: full-width `Merge` button + `Cancel` link, **no `Save anyway`** ✓
  (matches reconciliation 2).
- Warning conveyed by icon + heading text, not colour alone ✓.
- `Cancel` is a small text link, under the 44 px floor. The contract freezes it as
  a *link named `Cancel`*; padding it would need CSS outside `owns`. Documented
  (handoff open issue 2). Note: the handoff's rationale (changing it to a button
  would change the ARIA role) is imprecise — a padded link keeps `role=link` —
  but the conclusion stands because the stylesheet is not owned. Meets WCAG 2.5.8
  AA (24 px), not AAA.
- Rest of the walk form (Label / Asset number / Kind select `Park` / Photo / full-
  width `Confirm pin`) renders in order, nothing clipped at 320 px.

### `t205-walk-iphone-se-320.png` / `t205-walk-pixel-9.png` (A-202, A-203)
- Top strip: `42.761700, -71.030300` + `accuracy 8 m` in readable type ✓.
- Pin pad spans the middle with the `±25 m` scale label top-left; the pin (blue
  donut, white stroke/ring) is centred, high-contrast and clearly the draggable/
  focusable target ✓ (`aria-label="position pin"` confirmed in template).
- `#pin-pos` mirrors the coordinates at 6 dp directly under the pad ✓.
- `Add light here` is full-width; on both viewports the page fits one screen and
  the button sits in the bottom third — thumb-reachable ✓.
- Footer legible and unobstructed; no horizontal overflow at 320 px.
- The pin's *painted* glyph is ~22 px at 320 px (r=11 in the 200-unit viewBox);
  the ≥44 px claim is a hit-area claim, asserted programmatically per the
  handoff — plausible, not disprovable from a still.

### `t205-list-iphone-se-320.png` / `t205-list-pixel-9.png` (A-209)
- `Lights` heading ✓; one row per light; small square thumbnails — the photo
  light shows its landscape re-encoded thumb, the photo-less light a neutral bulb
  placeholder (allowed) ✓.
- Asset number + label text, label linking to detail; `● unknown` status chip is
  dot + word ✓.
- No horizontal scroll at 320 px; rows wrap cleanly.
- No `Older` control visible — correct here (2 rows < 50; the pager only renders
  when there is a next page). The round-3 suite visually verified `Older`/`Newer`;
  the current kept set does not show them (test-asserted). Minor evidence gap,
  not a layout defect.

## Checklist

- **320 px / mobile layout**: no horizontal overflow or clipped content in any of
  the 8 captures; headings wrap cleanly; footer visible everywhere ✓.
- **Tap targets ≥ 44 px**: all buttons qualify (`Add light here`, `Confirm pin`,
  `Merge`, `Save changes`, `Delete light`, file input row). Under the floor:
  dup-panel `Cancel` (contract-frozen link, CSS out of `owns`) and the global
  header nav (base.html, untouched by this task). Minor, disclosed.
- **Contrast**: dark body text on white, blue links ≥ 4.5:1, grey status text
  paired with the dot — all readable on both viewports ✓.
- **Status ≠ colour alone**: status dot + "unknown" word; dup/bbox warnings are
  icon + text ✓.
- **Map markers / clusters / attribution / bottom sheet**: N/A — the admin pages
  contain no MapLibre map (the pin pad is a local control); no attribution to
  obscure; no bottom sheet to cover a marker.
- **Clipped/overlapping text**: none observed in any capture.
- **States not in the kept set**: low-accuracy disabled + `Save anyway (low
  accuracy)`, geolocation-denied `#manual-lat/lon`, outside-bbox role=alert,
  in-form `#photo-preview`, `Saved` toast, HTMX pager — all covered by the
  handler/e2e tests and (except the toast and pager) were visually verified in
  the round-3 suite; the kept 8 omit them. Evidence gap only.

## Verdict

Layout, orientation, status encoding, contrast and overflow checks pass on both
mobile projects. The two deviations (blue-texted destructive button, sub-44 px
`Cancel` link) are minor, disclosed in the handoff, and forced by ownership
boundaries (`app.css` not owned; `Cancel` frozen as a link by the acceptance
contract) — neither violates the frozen contract table, which is the oracle.
Route the Delete-light redden to the CSS-owning task as a follow-up.

VERDICT: PASS
