# T-207 review — Activate the M2 acceptance suite (A-201..A-209)

Reviewer: primary (GLM-5.3). Scope: the attached diff (acceptance/README.md,
acceptance/e2e/admin-walk.spec.ts, acceptance/go/admin_{auth,lights}_test.go,
harness/.ratchet.json, harness/protected.sha256, harness/handoffs/T-207.md)
against harness/tasks/T-207.md and the frozen contract it activates.
Shell/exec is denied in this sandbox: exit codes, hash recomputation and the
holdout checkout could not be re-run; every claim reachable by reading the
tree was checked instead (same trust basis as the T-105 review, finding 3).

## Packet conformance

- **Writes are inside owns.** The diff touches only `acceptance/**`,
  `harness/protected.sha256`, `harness/.ratchet.json` and the handoff.
  `harness/.ratchet.json` is outside the literal owns list but is the
  sanctioned output of `make protect` (packet deliverable 2: "regenerate
  manifest + ratchet"); the guard whitelists it exactly in that pairing
  (cmd/harness/guard.go:141-143). Same disposition as the T-105 round.
- **Renames + edits match the packet's activation rules.** Rule 1's moves are
  all present; `acceptance/drafts/` is gone; the diff hunks for the two Go
  specs show comment-only header deltas and the e2e spec shows exactly two
  deltas (header note + the `scrollIntoViewIfNeeded` line at
  acceptance/e2e/admin-walk.spec.ts:332) — **no assertion line in the visible
  suite was touched**, verifiable from the diff hunks themselves.
- **Rule 5 (preserve the frozen contract)**: acceptance/README.md carries the
  contract reconciliations, the UI/HTTP contract tables, rerun-safety rules,
  fixture table, schema oracle, per-assertion visual notes and the holdout
  mirror (acceptance/README.md:26-237); the 91%-similarity rename hunk shows
  only the header prose above "## Contract reconciliations" was rewritten.
- **No skips/weakeners**: no `t.Skip`/`test.skip`/`test.only`/`.fixme`/
  `TODO`/`console.log` anywhere under acceptance/ (grepped).

## Verification of the disclosed oracle edits (visible suite)

**@A-203 `scrollIntoViewIfNeeded` (acceptance/e2e/admin-walk.spec.ts:332).**
The premise checks out against the implementation: the pin pad is
`<svg id="pinpad" viewBox="0 0 200 200" width="100%">`
(web/templates/pages/admin-walk.html:12) with the pin at `translate(100 100)`
(:16), so on desktop-chromium's default 1280×720 viewport
(playwright.config.ts:28-29) the pad renders ~1200×1200 CSS px and the pin's
bbox centre sits below the fold — the draft's raw `page.mouse` drag (which
bypasses Playwright's auto-scrolling actions) targets coordinates no compliant
build can make reachable without scrolling. The edit cannot weaken the test:
the `expect.poll ... .not.toBe(before)` drag assertion
(admin-walk.spec.ts:341-346), the arrow-key nudge poll (:352-362) and the
saved-position equality against `#light-pos` (:375) are untouched, and a
scroll cannot move a pin that ignores pointer events. This is exactly the
packet rule-3 "mechanical defect that cannot change what it asserts" class
(harness/tasks/T-207.md:41-47), it is listed with justification, hash lineage
and a ratification request (handoff edit 2 + Gaps item 2) — the same
disposition the T-105 regex pin received (harness/reviews/T-105-reviewer.md:
75-88). Correctly handled; planner ratification still owed, as disclosed.

**README header rewrite (acceptance/README.md:1-24).** Non-contract prose
describing T-207's own future work was stale post-activation; the frozen
material below is preserved. Fine.

## The routed A-208 dispute (holdout) — correctly BLOCKED

Verified both sides of the dispute in the tree: `lightEdit` carries only
`CSRFToken/Label/AssetNo` (internal/web/admin.go:380-384), `parseLightEdit`
lifts only label/asset_no (:389-402), and `adminEdit` + `EditLight` apply only
those (:423-477) — a posted `kind` is silently ignored. T-205's packet froze
the route as "edit: label, asset_no" (harness/tasks/T-205.md:70), while the
frozen README's edit row is generic ("multipart `csrf_token` + changed
fields", acceptance/README.md:93) and the holdout variant reads it to include
kind. That is a genuine frozen-contract ambiguity, and the packet's rule-3
first bullet prescribes exactly what T-207 did: do NOT edit the test, leave it
red, open the handoff with `BLOCKED: A-208 unmet — …` and route to the
planner (handoff line 1 + Gaps item 1). `make holdout` FAIL is the sanctioned
terminal state, per the T-105 precedent (harness/reviews/T-105-reviewer.md:
163-171).

## Holdout-repo edits (3-6) — unverifiable here, premises verified

The holdout checkout is outside this sandbox. Where the disclosed edits'
premises touch the main repo they hold: the row template emits exactly one
`alt="{{.ImgAlt}}"` per row (web/templates/partials/admin-rows.html:6;
`thumbAlt` at internal/web/admin.go:99-100), so the A-209 row-marker rewrite
(`alt="Boundary lamp 01"` == 1 AND `alt="Boundary lamp ` == 1) pins
exactly-one-row-and-it-is-lamp-01 — a duplicated row, an extra Boundary row,
the wrong lamp or an empty page all fail, i.e. the attempt-2 weakening is
genuinely reverted and the draft's exactly-one-row intent restored. The
session-cache edit's premise (one-time GLOBAL token, replay from any client →
403) is the frozen contract itself (acceptance/README.md:90), so a fresh
worker process per project could never re-exchange it. FindBin delegation and
bootstrap caching are assertion-neutral by construction.

## Manifest / ratchet arithmetic — exact

- Manifest: 4 drafts entries removed, 4 activated entries added, net 15 lines
  covering every file under acceptance/** (cross-checked against the tree;
  contract.json, 3 utils, 6 Go tests incl. the 2 activated, 4 e2e specs, 1
  README). Nothing else moved. The hashes themselves could not be recomputed
  (no shell); the guard verifies them at gate time.
- Ratchet pw 32→76 is exact: 19 e2e tests (home 3, home_m1 2, probe 3 incl.
  the nested no-JS test at probe.spec.ts:76, admin-walk 11 incl. the nested
  geolocation-denied test at admin-walk.spec.ts:291) × 4 projects
  (playwright.config.ts:26-48). a11y 12/12 = 3 @a11y tests × 4. Growth-only
  as the ratchet demands (cmd/harness/ratchet.go:89-98).
- Ratchet go 51→128 reflects tagless unit-test growth from T-201..T-206
  (`countGoTests` runs `go test -list` without `-tags acceptance`,
  cmd/harness/guard.go:341-353 — the counting gap disclosed since M1); the
  activated acceptance tests are guarded by the manifest instead.

## Security / architecture spot-checks (all clean)

- No app code, `go.mod`, Makefile or CI touched; no new dependencies
  (`allow_deps: []` respected).
- No `template.HTML`, no SQL, no path handling, no upload code in the diff —
  activation only. The oversize fixture is synthesised at runtime
  (acceptance/go/admin_lights_test.go:711-721), never committed.
- The test servers' hardcoded env is byte-equal to the dummy `.env.test`
  values (acceptance/go/admin_auth_test.go:281-284 vs .env.test:6-9; ditto
  admin_lights_test.go:952-955) — no secret material; `make secrets`
  (gitleaks) is in the claimed-green `make check`.
- HTMX never swaps `#map`: the activated spec only asserts the admin pager's
  fragment swap; the pager targets `#rows` with `hx-swap="outerHTML"`
  (web/templates/partials/admin-rows.html:16-17), and the walk page uses the
  SVG pin pad, not the map.
- The tests exercise real behaviour: self-spawned real binary over temp
  SQLite with /healthz readiness (admin_auth_test.go:260-295), real browsers
  on 4 projects, byte-level JPEG parsing of served photos
  (admin_lights_test.go:753-882), file-system stray checks (:723-751).
- Per-assertion table: every named proving test exists in the activated
  files; @A-207 being Go-only is disclosed and packet-conformant.

## Findings

1. **[Low] Stale "byte-identical / zero content edits" self-descriptions.**
   acceptance/README.md:15-16 says "the two Go specs are byte-identical to
   their drafts", and both Go headers say "zero content edits"
   (acceptance/go/admin_auth_test.go:6-10, acceptance/go/admin_lights_test.go:
   7-11) — but the attempt-3 header rewrites mean the current bytes (manifest
   `09506a…`/`535d4ad…`) differ from the drafts (`6a48178…`/`1c7bd11…`), as
   the handoff itself states ("the current deltas against the drafts are
   header comments only"). The claims were true only at the activation commit
   `e8ee540`. The audit trail is complete and honest in the handoff, so this
   is wording drift, not concealment (same class as the T-105 review's
   finding 1) — correct the wording when the planner ratifies the edits.
2. **[Low — disclosed, planner item] The @A-203 edit is an oracle edit, not a
   rename.** Sanctioned by packet rule 3 and verified assertion-preserving
   (see above); ratification by the planner remains open (handoff Gaps item
   2), exactly as the M1 regex pin was routed.
3. **[Info] A-208 holdout dispute correctly left red** as the BLOCKED
   terminal state; the planner must either cut a kind-edit fix task or
   re-freeze the variant (handoff Gaps item 1).
4. **[Info] Holdout-repo state unverifiable in this sandbox** (external
   directory, no shell): commits `4b64fbc`/`c8d6897`, the 48/48 e2e result
   and edits 3-6 rest on the handoff; every main-repo premise they rely on
   checked out.
5. **[Info] Prior-round review files cited by the handoff are absent from the
   tree** (harness/reviews/T-207-reviewer.md from attempt 1,
   T-207-reviewer-alt.md from attempt 3). This diff does not delete them
   (harness/reviews/** is not touched); their absence makes the retry-record
   verdicts non-reproducible in-tree, worth the scheduler's attention.
6. **[Info] Unverifiable here, consistent everywhere checked**: command exit
   codes, `make protect` hash regeneration, the actual green runs. Counts,
   manifest coverage, env bytes, template premises and the dispute's
   file:line citations all reconcile exactly.

## Verdict rationale

This is a disciplined activation in the T-105 mould: pure renames where the
drafts allowed them, one assertion-preserving mechanical oracle edit with
full hash lineage and a ratification request, a genuine contract-vs-feature
ambiguity left red and routed as BLOCKED instead of being edited to fit, the
frozen contract tables and visual notes preserved, the manifest/ratchet
regenerated only through the sanctioned tool, and nothing written outside
owns. The visible suite's assertions are byte-untouched (provable from the
diff hunks), the failing holdout assertion was never weakened — the attempt-2
weakening was caught and genuinely reverted — and no skip, TODO or secret
leaks anywhere. The remaining items are the planner's to ratify, not defects
of this activation.

VERDICT: PASS
