# Judge findings — Milestone M2 (A-201..A-209)

Verdict: **DONE**. Every gating assertion has passing evidence I can point to;
the pipeline claims in the milestone package check out against the repo; the two
adjudicated merges are documented with retained transcripts; and the only
human-gated candidate (the H9b field test) is sequenced *after* "Judge: DONE" in
`docs/PLAN.md`'s per-phase checklist and softened to "before judge **if
possible**" in the governing `harness/state.md` — it gates no A-2xx assertion.
Open items are recorded below; none undermines a contract gate.

## Contract gates — all A-201..A-209 are `gate:true`, all green

No `gate:false` assertions in M2 (nothing owed as recorded numbers); no
`verified:human` flags in `acceptance/contract.json`.

| Gate | Visible evidence (pointable) | Holdout evidence |
|---|---|---|
| A-201 | `TestA201AdminLogin` (acceptance/go, green in the final gate's `make check`, T-207 handoff attempt-6 table) + e2e `@A-201 unauthenticated /admin lands on the login page` ×4 projects | `TestHoldoutA201OneTimeTokenAcrossClients` green |
| A-202 | 3 e2e specs ×4 (`add a light from current position`, `low accuracy gates saving`, `geolocation denied shows manual pin fields`) + `@A-202 @a11y` axe ×4 | walk / low-acc (55 m) / a11y variants green |
| A-203 | e2e `@A-203 dragged pin position is what gets saved` ×4 (drag + keyboard nudge + `#light-pos` equality) | `@A-203 keyboard-moved pin` variant green ×4 |
| A-204 | `TestA204OutsideBBox` (422 + frozen message) + e2e `@A-204` ×4 | north-of-bbox Go + east-of-bbox e2e green |
| A-205 | `TestA205Duplicates` (409/422/merge-applies-label/save-anyway) + e2e `@A-205 duplicate panels` ×4 | `TestHoldoutA205MergeLeavesExactlyOneRow` green |
| A-206 | `TestA206PhotoPipeline` (no APP1, exact dims, ≤1600, HEIC 415, 413) + e2e `@A-206 served photo is re-encoded…` ×4 | Orientation=8 Go + e2e variants green |
| A-207 | `TestA207NoStrayFiles` — I read it on disk: HEIC/text-as-jpg/pixel-bomb → 415, oversize → 413, PHOTO_DIR file-count unchanged on every reject, exactly one `.jpg` on success (`acceptance/go/admin_lights_test.go:362-415`). No e2e spec exists **by frozen design** (T-200's Deliverable-4 list has no `@A-207` e2e entry); its gate evidence is the Go acceptance test, green inside the final gate's `make check` | `TestHoldoutA207SixteenMBUploadLeavesNoStrayFile` green |
| A-208 | `TestA208EditSoftDelete` + e2e `@A-208 edit relabels and delete removes` ×4, running against the adjudicated kind-edit fix (verified present on main: `validatedEditKind`, `internal/web/admin.go:473-513`) | `TestHoldoutA208EditKindAndSoftDelete` — the gap the hidden holdout caught at the final gate; fixed through store/handler/template on main `2c93380`; green after |
| A-209 | `TestA209Pagination` (50-newest, rows fragment) + e2e `@A-209 admin list paginates with HTMX` (no navigation event) ×4 | `TestHoldoutA209FiftyOneBoundary` (row-marker exactly-one-row pin, after the reviewer-caught attempt-2 weakening was restored) green |

**Validate artifact is real this time** (M1's blocker #2 does not recur):
`harness/runs/20261114T021227Z/validate/results.json` — machine-readable,
`expected: 44, unexpected: 0, flaky: 0, skipped: 0`, 11 specs × 4 projects,
grep `@A-2[0-9]`, webServer `bin/server` booted 02:12:28Z — **12 seconds after
T-207 merged** (02:12:16Z), so it validated the fully merged tree.

## Pipeline claims — verified

- **Merges:** `progress.log` carries `gate_ok`+`merged` for all nine M2 tasks —
  T-200 16:50:07Z, T-208 16:51:53Z, T-202 17:33:14Z, T-204 17:56:37Z,
  T-201 18:11:07Z, T-206 18:47:23Z, T-203 18:50:53Z, T-205 23:59:06Z,
  T-207 02:12:16Z (Nov 14) — plus T-106 16:26:26Z (the folded judge-M1 chore
  packet). `graph.json`: all 16 task statuses `done`, none `blocked`.
- **Contract-first held:** T-200 (acceptance) merged before the first feature
  claim (T-201 claimed 16:50:18Z); the guard's ancestry rule visibly blocked
  T-205 at 18:51:03Z when its acceptance dep wasn't done.
- **Reviews:** double PASS (reviewer + reviewer-alt) for T-106, T-200, T-201
  (after one round), T-202, T-203, T-204, T-206, T-208 — including T-200, the
  oracle-authoring task (M1's T-100 hole does not recur; `T-200-reviewer-alt.md`
  ends `VERDICT: PASS`). T-205 merged via round-5 operator adjudication:
  reviewer PASS + visual-reviewer PASS (10/10 screenshots), reviewer-alt's stale
  owns objection overruled — transcript retained and verified at
  `harness/runs/20261113T233402/T-205/reviewer-alt.jsonl`. T-207 merged via
  harness/operator adjudication over four reviewer-alt BLOCKs; the primary
  review (`harness/reviews/T-207-reviewer.md`, `VERDICT: PASS`) is a substantive
  152-line verification that re-checked the frozen bytes, the ratchet math and
  the skip-marker grep.
- **Test integrity:** my own grep over `acceptance/` for
  `t.Skip|test.skip|test.only|.fixme|fixme(`: zero matches. The one behaviour-
  bearing oracle edit (`@A-203` `scrollIntoViewIfNeeded()`) is disclosed with
  hash lineage, verified by the reviewer as assertion-preserving against the
  frozen draft bytes, and is independently corroborated by the holdout's
  keyboard-moved-pin variant — the worker cannot have gamed the visible spec.
  Oracle-first demonstrably worked: the holdout caught the A-208 kind-edit gap
  and refused T-207's gate until the feature (not the test) moved.

## DoD scope — the milestone package's claims verified

- **No deploy dependency in M2:** confirmed. `harness/state.md` operator
  pre-steps: "H4 (two deploys + smoke evidence) closes M1, **independent of
  Phase 2**". PLAN.md's Phase-2 DoD block is `make check && make holdout` +
  `make validate MILESTONE=M2` (+ optional exiftool). The first two are green on
  the merged tree (T-207 handoff attempt-6: `make check` exit 0, `make holdout`
  exit 0 — "holdout: PASS", go `ok` incl. the kind-edit test, e2e 48/48 ×4);
  validate artifact above. M1 itself remains NOT DONE pending H4 — recorded, no
  effect on M2.
- **Field test (H9b) is not a gate for this verdict:** PLAN.md's per-phase
  checklist orders it *after* "Judge: DONE" (§13: … Validator report →
  Visual-reviewer PASS → **Judge: DONE** → *Human field test (H9b)*), and
  `harness/state.md` softens it to "before judge **if possible**". No A-2xx
  assertion references it. It is owed work, not missing evidence (see below).
- **Visual-reviewer:** PASS verdict on record
  (`harness/reviews/T-205-visual-reviewer.md`, 10 screenshots, both mobile
  projects, five states; 8 verdict lines in `progress.log`), covering the frozen
  per-assertion visual notes. M1's blocker #3 does not recur.

## Non-blocking gaps (record; route; none gates A-2xx)

1. **T-207 reviewer-alt file absent on main.** `harness/reviews/` has
   `T-207-reviewer.md` (PASS) but no `T-207-reviewer-alt.md`; the four BLOCK
   verdicts live in `progress.log`, and the attempt-3 five findings are quoted
   and answered point-by-point in the handoff (the A-209 row-pin restoration is
   verified in the holdout tree). The validate report's "adjudicated with
   transcripts retained" is slightly overstated for this one task — correct the
   record or archive the review file from git history.
2. **Visual PASS predates the last UI delta.** The final visual PASS
   (23:42:20Z Nov 13) precedes the validate run and the adjudicated kind-select
   addition to the edit form (main `2c93380`, ~01:25–02:12 Nov 14). The frozen
   A-208 visual note constrains the detail page's photo/`#light-pos`/"Label"
   form/"Save changes"/separated "Delete light" — all reviewed — and does not
   constrain the edit-form field set, so the frozen visual contract is fully
   covered; the kind select is functionally proven by visible e2e + holdout.
   Refresh the detail-page screenshot at M3's first visual pass.
3. **Planner ratifications still pending:** (a) the `@A-203` scroll oracle edit
   (disclosed, hash-lineaged, reviewer-PASSed, holdout-corroborated); (b) the
   six contract reconciliations frozen in `acceptance/README.md`. Both survive
   fine if ratified; if refused, the disclosed fallbacks are on record in
   T-207's handoff.
4. **Field test (H9b) owed:** five real fixtures in one park (photos upright,
   pins < 15 m). It decides HEIC (R6/C18) and exercises the GPS-under-tree-cover
   risk (R9) before real inventory collection begins. T-207's handoff flags it
   "still owed to the judge" — schedule it before M3's field data depends on
   the walk flow.
5. **T-207 provenance wording nit:** three comment blocks still say "byte-
   identical/zero content edits" (true only at the activation commit); prepared
   comment-only patches (`harness/handoffs/t207-a5-readme.patch`,
   `t207-a6-provenance.patch`) await a session with `acceptance/**` write
   rights. Comment-accuracy only.
6. **M1 carry-over:** M1 stays NOT DONE pending H4 (Fly deploy + 2× smoke) —
   independent of M2 per the pre-registered plan; do not let it drift.

## Why DONE despite the M1 precedent's strictness

M1's four blockers were: an assertion half (deploy persistence) with *no*
evidence anywhere; a *missing* validation artifact chain; *zero* visual-reviewer
output; and a factually false "all reviewed" claim for the task that authored
the oracle suite. M2 has none of these failure modes: the validation artifact is
real and machine-readable, the visual review is substantive and PASS, every task
including the oracle author went through live double review or a documented
adjudication with retained transcripts, and the one human-gated candidate
(field test) is pre-sequenced after the judge by the plan itself. The gaps above
are record-keeping and process-hygiene items, each with the substance preserved
elsewhere on the record.
