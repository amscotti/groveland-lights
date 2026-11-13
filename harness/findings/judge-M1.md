# Judge findings — Milestone M1 (A-101..A-105)

Verdict: **NOT DONE**. The local engineering is real and largely green, but the
milestone's pre-registered definition of done includes a deployed persistence
proof that does not exist, the milestone validation evidence chain is missing,
and one process claim in the milestone package is contradicted by the repo.

## What I verified (positive evidence, pointable)

- Contract: `acceptance/contract.json` carries A-101..A-105, all `gate:true`.
  No `gate:false` assertions (no recorded numbers owed). No `verified:human`
  flags in the contract — but see §B1: the Fly deploy is a human-gated item of
  M1's DoD per `harness/state.md` (H4), which is exactly the kind of item that
  needs the named human's evidence.
- Merges: `harness/progress.log` shows `gate_ok`+`merged` for all six tasks
  (T-100 04:52:24Z, T-101 08:01:11Z, T-102 08:01:50Z, T-104 08:02:30Z,
  T-103 09:47:28Z, T-105 12:14:04Z).
- Reviews: T-101..T-105 each have `harness/reviews/T-10x-reviewer.md` and
  `-reviewer-alt.md` with `VERDICT: PASS`.
- Local green on the merged tree: the final T-105 gate session
  (`harness/runs/20261113T115949/T-105/integrator.jsonl`, lines 161/194/198/231)
  recorded real runs: `npx playwright test` 32/32 across all four projects
  (including the frozen no-JS A-103 test), `make holdout` PASS, `make check`
  exit 0 (acceptance/go ok, coverage 81.1%/86.2%). The 12:14:04Z `gate_ok` ran
  the gate's own check on this tree.
- A-103 no-JS dispute resolved legitimately: the frozen oracle is byte-exact
  (`probe.spec.ts` on disk matches manifest hash `4ec1052d…` in
  `harness/protected.sha256`; assertions at :83-93 intact) and the FEATURE was
  fixed to meet it — operator (human, Anthony Scotti) commit `7f07df2`
  "fix(web): server-render the probe fragment for no-JS clients" +
  `{{.ProbeHTML}}` in `web/templates/pages/index.html`. Zero test edits; the
  oracle-first property survives.
- Per-assertion local status: A-101 ✓, A-102 ✓, A-103 restart-half ✓
  (acceptance/go SIGTERM+respawn test, holdout two-session restart, e2e reload),
  A-104 ✓ (acceptance/go migrations tests + holdout variant), A-105 ✓
  (acceptance/go spawn test with X-Forwarded-For probe). All inside the
  recorded green `make check`.

## Blocking gaps

1. **A-103 "…across restarts AND DEPLOYS" is half-unevidenced, and M1's
   deployed DoD is unmet.** `harness/state.md` (planner/operator close-out,
   written before the run): M1 goal is "skeleton deployed on Fly with a
   volume; a SQLite-backed probe counter proves persistence across restart and
   deploys", and instructs the judge to use "BOTH deploys' scripts/smoke.sh
   outputs as persistence evidence". No deploy has ever happened (Fly
   BLOCKED-H4, no account — documented, but documentation is not evidence).
   No `smoke.sh` output exists anywhere in the repo. The offered substitute
   ("Local Docker proven: image 29.25MB, boots, healthz 200") is the Phase-0
   image figure and evidences boot+healthz only — it contains no probe
   increment, no restart, no redeploy boundary, so it cannot stand in for the
   deploy-persistence proof even locally. The human-gated item has no human
   evidence on record.
2. **Milestone validation evidence chain is absent.** The report
   (`harness/findings/validate-M1.md`) claims artifacts at
   `harness/runs/20261113T121419Z/validate/ (screenshots, traces)` — that
   directory is EMPTY (verified; zero files). No validator-role transcript
   exists anywhere (every other run dir has one). The "20/20 passed" figure is
   operator-compiled narrative; it is *indirectly corroborated* (the gate
   session's 32/32 superset), but the formal `make validate MILESTONE=M1`
   artifact set does not exist, so nothing validates the validation.
3. **No visual-reviewer verdict exists.** The pre-registered close-out
   (`harness/state.md` run notes) requires "visual-reviewer on the validator
   screenshots"; T-103 was `ui:false` with "visual check at validation"
   deferred to exactly this step. With the screenshot directory empty, no
   visual review could or did occur — grep of `harness/` finds no
   visual-reviewer output at all. The M1 UI visual check was never performed.
4. **The milestone claim "All tasks T-100..T-105 merged through
   guard+reviews+gate" is false for T-100, and the report repeats it.**
   `harness/reviews/` contains review files only for T-101..T-105; there are
   no T-100 review files and no `review_pass`/`review_block` lines for T-100 in
   `harness/progress.log` (it went guard_fail → rework ×2 → gate_ok → merged).
   `validate-M1.md`'s "Reviews: T-100..T-105 all reviewer + reviewer-alt
   VERDICT: PASS" is therefore factually wrong for the task that authored the
   entire M1 oracle suite (the frozen A-101..A-105 specs and drafts). The
   milestone's test-integrity story has an unreviewed link at its base.

## Non-blocking open issues (record, don't gate)

- **Gap (b) from T-105's handoff remains unfixed:** `probeGet`/`renderPartial`
  set no `Content-Type`, so `GET /probe` is sniffed `text/plain` + global
  `nosniff` and the shipped `<noscript><a href="/probe">Probe</a>` link shows
  escaped source text to no-JS visitors. T-105's handoff calls the one-line
  fix "Required … unconditional"; the operator fixed only gap (a); no fix task
  exists. Not covered by the frozen suite (so not a contract gate), but it
  contradicts T-103's own contract bullet "non-JS visitors can reach the
  fragment page".
- **Merged T-105 handoff is stale:** its first line still reads "BLOCKED:
  A-103 unmet …" from the pre-`7f07df2` state; the final session hit max
  steps before rewriting it. The audit trail in `harness/handoffs/T-105.md`
  no longer describes the merged reality (the transcript summary in
  `runs/20261113T115949` does).
- **Hygiene:** operator commit `7f07df2` committed `test-run.db{,-shm,-wal}`
  binaries into the repo (tracked ever since); `.gitignore` fix landed later
  in `f901627` but the tracked blobs remain.

## Path to DONE

1. Human (Anthony) performs H4: `fly launch`/deploy with the `data` volume,
   runs `scripts/smoke.sh <url>` before and after a second deploy; both
   outputs filed (e.g. `harness/runs/<id>/smoke/`) proving the counter
   continues across the deploy boundary — this is A-103's missing half and
   the judge's pre-registered evidence. (A local two-container redeploy with a
   shared volume + smoke.sh would be weaker but materially better than
   nothing; it still needs artifacts, not narrative.)
2. Re-run `make validate MILESTONE=M1` with artifacts actually written to the
   run directory (screenshots/traces/report), then visual-reviewer over those
   screenshots with a recorded verdict — closes the deferred T-103 visual
   check.
3. Reconcile the record: either run the missing T-100 reviews (retroactive,
   against the frozen drafts in git history) or correct the milestone claim
   and report so they don't assert reviews that don't exist.
4. Cut the small fix task for the `/probe` Content-Type defect (gap b) with a
   header assertion in the probe table tests.

---

## Post-verdict addendum (operator, 2026-11-13T12:25Z)

Judge verdict NOT DONE accepted. Both fixable findings addressed:

1. **Artifact chain**: `make validate` now writes a machine-readable results
   file (PLAYWRIGHT_JSON_OUTPUT_NAME → results.json) alongside failure
   artifacts; `harness/runs/20261113T122251Z/validate/results.json` records the
   4 unique green specs (20 project-runs) for M1.
2. **Redeploy-boundary persistence (local)**: docker proof on record —
   container A (volume `gl-probe-proof`): POST /probe → 303, count=1; container
   A destroyed; fresh container B on the same volume boots with count=1. The
   Fly-deployed equivalent remains BLOCKED-H4 (human account) and M1 stays
   NOT DONE until that deploy + `make smoke` evidence exists.

Verdict stands: **NOT DONE (pending H4: Fly deploy)**.
