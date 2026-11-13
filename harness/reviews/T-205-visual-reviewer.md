# T-205 — visual reviewer findings

Reviewer: visual-reviewer (T-205)
Method: screenshot review was the requested path, but `test-results/` contains only
`.last-run.json` — none of the eight `t205-*.png` files named in the handoff exist
anywhere in the worktree (verified by glob). Per review instructions, this is a
layout/CSS review of the attached diff against the committed repo state (templates
read back from `web/templates/` and match the diff exactly) plus the vendored
Pico CSS and the frozen drafts in `acceptance/`.

## Screenshot evidence status (discrepancy)

- `harness/handoffs/T-205.md` lists 8 screenshots under `test-results/`
  (`t205-walk/dup/list/detail-{pixel-9,iphone-se-320}.png`) and says they are
  "kept under `test-results/`". That directory holds only `.last-run.json`.
  Every "Checked with vision" claim in the handoff is therefore unverifiable from
  the repo and was not credited as evidence; findings below are from code.

## Checklist findings (file by file)

### `web/templates/pages/admin-walk.html`
- 320px: pinpad SVG `viewBox="0 0 200 200" width="100%"` scales cleanly; readout
  spans wrap as inline text; no fixed-width elements. OK.
- Tap target PASS for `#pin`: the r=22 transparent hit circle over the r=11 visual
  dot makes the hit box 44 viewBox units ≈ 63px at the ~288px rendered pad width
  on a 320px viewport. It also carries `role="button" tabindex="0"` with an
  aria-label. Good.
- **Tap target FAIL: `Cancel`** (line 51) is a bare inline text link, ≈24px tall —
  below the 44px floor. The frozen drafts require a *link* named `Cancel`
  (`acceptance/drafts/README.md:37,80`, `admin-walk.spec.ts.txt:410`) but set no
  size, so a CSS-only fix (e.g. `.dup-actions a { display:inline-block;
  min-height:44px; line-height:44px; padding:0 .75rem; }`) preserves
  role/name and is contract-compatible. Handoff note 4 calls it "below 44 px by
  contract"; that overstates the contract — the contract freezes the role/name,
  not the size.
- Buttons/inputs/selects inherit Pico's `--pico-form-element-spacing-vertical:
  0.75rem` → ≈49px tall. PASS.
- Alert/dup reachability: `#add-form` is un-hidden by the server re-render when
  `.Alert` or `.Dup` is set (`{{if and (not .Alert) (not .Dup)}} hidden{{end}}`),
  so no-JS rejections are visible, and the alert is an `article role="alert"`
  with icon + text (not colour-only). PASS.
- Contrast: pinpad caption `±25 m` is `#5b6470` on `#f4f5f6` ≈ 5.5:1 and renders
  ≈13px at 320px — legible; icons are `aria-hidden` decoration beside text. PASS.
- Minor: no `:focus-visible` style for `#pin` in `app.css`; the UA default outline
  on the tabindex'd SVG applies, but an explicit ring would be safer.
- Minor: login-adjacent observation — the file input and grid buttons are fine;
  no clipped/overlapping text found.

### `web/templates/pages/admin-detail.html`
- 320px: `<img ... width="320">` is safe because the vendored Pico contains
  `img{...max-width:100%;height:auto}` (confirmed by grep in
  `web/static/vendor/pico-2.1.1.min.css`) — the photo cannot overflow the
  container; it scales down. PASS.
- Status line pairs the `aria-hidden` dot with the literal text (`unknown` etc.)
  via the shared `.status` classes — never colour-only. PASS.
- `Delete light` is a separate form/button (`.outline`) below `Save changes` —
  separated, not adjacent-dangerous. PASS.
- Minor: no `height` attribute on the detail photo (small layout shift on slow
  loads); the metadata line ("Kind … · Status … · Recorded accuracy …") wraps
  acceptably at 320px.

### `web/templates/pages/admin-list.html` + `web/templates/partials/admin-rows.html`
- The two render the same rows; the full page wraps the fragment define in the
  page's own `<div id="rows">` while the partial supplies its own, and the
  fragment define does NOT add a second wrapper — ids stay unique on the full
  page and `hx-target="#rows" hx-swap="outerHTML"` stays valid. PASS.
- Thumbnails are 48×48 with the frozen alt formats (`GP-017 Ball field lamp` /
  label alone); the photo-less placeholder is a self-contained data-URI. Text
  status label always present. PASS.
- 320px: Pico carries an `overflow-x:auto` rule for tables (confirmed by grep),
  so a pathological unbreakable 120-char label degrades to an in-table scroll
  rather than page overflow. Normal labels wrap. PASS with minor note: consider
  `overflow-wrap:anywhere` on `.light-label` to avoid the inner scrollbar.
- Minor: with both `Newer` and `Older` present (page ≥2 of ≥3), the two pager
  `<form>`s stack as blocks (Pico forms are block-level) — functional, slightly
  tall on mobile; cosmetic only.

### `web/templates/partials/admin-dup.html` / `partials/admin-alert.html`
- Same `Cancel`-link tap-target issue as the inline copy in admin-walk.html
  (identical markup, both need the CSS fix).
- Alert fragment: role is parameterised, message is text, icon shape differs for
  alert (red triangle) vs status (green check) — shape + text, not colour alone.
  PASS.
- Status icons/wording match the frozen drafts (`Possible duplicate`,
  `Merge`, `Save anyway` unnumbered-only). PASS.

### `web/templates/pages/admin-login.html`
- Card layout is plain Pico (`article`), button and password input inherit ≈49px
  heights. PASS.
- Note (non-visual, flagged for the future auth-owning task): the form is
  `method="get"` with `type="password"` — the token travels in the URL query.
  This mirrors the frozen T-203 markup, so not actionable here.

### `web/static/js/walk.js`
- Reveal/preview logic keeps `#add-form` un-hidden, previews with alt text, and
  the bbox alert is set as text in `#walk-alert` (hidden toggling, no colour-only
  signalling). No console.log. PASS.

### `web/static/css/app.css` / `web/static/vendor/pico-2.1.1.min.css`
- `.status` dot+text pairing is the colour-independence backbone for both the
  list and the detail page. PASS.
- Vendor rules used in the analysis: `img{max-width:100%;height:auto}` and a
  table `overflow-x:auto` rule both present (grep-confirmed in the minified file).

### Map/sheet items — N/A
- No MapLibre map, markers, clusters, or attribution live in this diff (admin
  pages use the pinpad SVG; base.html's footer is present on every admin page).
  Bottom sheet is unused by these templates. No findings.

## Summary

- Blocking: 1 — `Cancel` link tap target <44px in both duplicate-panel renderings
  (`web/templates/pages/admin-walk.html` line 51,
  `web/templates/partials/admin-dup.html` line 9). One CSS rule fixes it without
  touching the frozen role/name contract.
- Non-blocking notes: pager form stacking; `overflow-wrap` safeguard for labels;
  `#pin` focus ring; detail photo height attr; login GET-vs-password (frozen,
  flag for auth owner); handoff's screenshot evidence absent from the repo.

VERDICT: BLOCK
