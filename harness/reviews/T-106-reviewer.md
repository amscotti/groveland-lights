# T-106 review (attempt 2) — BLOCKED disposition on "already untracked on main"

Reviewed artifact: `harness/reviews/T-106.diff` — the branch-introduced changes
(merge-base `77945ff` → `task/T-106`): the attempt-2 handoff
`harness/handoffs/T-106.md` (new) and a one-line `harness/tasks/graph.json`
status hunk. Verified against the packet `harness/tasks/T-106.md`, the main
checkout, `harness/progress.log`, `cmd/harness/gate.go`, and `cmd/harness/ratchet.go`.

Method note: this review session's shell allowlist permits read-only git verbs
only (`log/show/diff/status` — the same restriction the handoff reports); all
git claims below were re-executed directly, not taken from the handoff or the
worker transcript (which is not present in this checkout).

## Packet conformance — the BLOCKED route is prescribed and was followed exactly

- Never clause (T-106.md:46-50): "If the files are NOT tracked in your
  worktree (someone already fixed it), write `BLOCKED: already untracked on
  main` in the handoff and stop." Handoff line 1 is that exact string.
- The precondition genuinely holds. `git log --oneline -- test-run.db
  test-run.db-shm test-run.db-wal` → exactly `30be90d` (untrack) then
  `7f07df2` (accidental add), nothing after; `30be90d` is an ancestor of both
  `main` and `task/T-106`. So packet step 1 (`git rm --cached …`, T-106.md:28)
  cannot succeed and step 2's commit would be empty — correctly not fabricated.
- Step 3 respected: no `.gitignore` hunk anywhere in the branch-introduced
  diff; `.gitignore:20` = `test-run.db*` is already correct. All three files
  exist on disk as ignored runtime artifacts.
- Owns respected: the worker's only commit content is the handoff
  (`b0a671a`, `4f971ee`). Nothing under `acceptance/`, no deps
  (`allow_deps: []`), no migrations, no goldens. Hard rules 1, 2, 3, 8, 10
  compliant.

## The graph.json hunk is gate-authored, not a worker edit of protected state

The diff's only non-handoff change (`status: review → merging`, T-106 diff
hunk at graph.json:240) is commit `194e9f2`. Three independent proofs that
this is the harness's own commit, not an AGENTS.md rule-1 violation:

1. Its message exists verbatim only in the gate: `cmd/harness/gate.go:127`
   (`finalizeMerge` restores main's authoritative graph.json onto the branch
   precisely "so the merge cannot regress task status/attempts", gate.go:122-123).
2. Timing: `194e9f2` was committed 2026-11-13T13:56:35Z — the exact second of
   `gate_fail T-106` (harness/progress.log:245), five minutes after
   `worker_done T-106` at 13:51:23Z (progress.log:239) and 7 seconds before the
   next worker claim at 13:56:42Z (progress.log:256-257). No worker session
   was active.
3. Content: exactly main's graph.json blob (`ce4141e`, produced by scheduler
   commit `cdaadab` at 13:55:30Z) restored onto the branch.

The handoff openly identifies `194e9f2` as a harness commit — accurate.

## Divergence diagnosis and operator guidance are correct

- Re-verified: branch-only = `b0a671a`, `194e9f2`, `4f971ee`; main-only =
  `cdaadab`, `02f6cdc`, `1dfd88c`, `37a571f`, `f148c82` (all scheduler
  "update task graph…" commits). Sole file overlap: `harness/tasks/graph.json`.
  Main currently carries `status: "review", attempts: 1` (graph.json:240-241).
- Option 2's rebase mechanics check out: replaying `194e9f2` (which only
  touched the status line) onto main via git's 3-way merge is conflict-free
  and keeps main's `attempts: 1` — the handoff's "rebase auto-keeps attempts"
  claim is correct. `b0a671a`/`4f971ee` replay cleanly (main lacks the file).
- Harness-side defect, correctly not fixable by the worker: there is NO
  `gate_conflict T-106` event in progress.log, i.e. the gate's rebase step
  (`gate.go:88`) returned success without integrating main, then
  `finalizeMerge`'s ff-only merge (gate.go:130) refused. Re-running the gate
  without an effective rebase will loop; harness is protected, and every
  topology verb the worker could have used (`rebase`/`merge`/`reset`/
  `cherry-pick`) is denied by the session allowlist — a restriction this
  review session reproduces. The handoff's recommendation (option 1: close
  T-106 as already-done, drop the branch) is the right call: the branch
  carries zero code changes and the deliverable already exists on main as
  `30be90d`.

## Verification claims corroborated by the harness's own log

- `make check-fast` exit 0 (attempt 2): `check_fast_ok T-106` at
  2026-11-13T14:05:52Z (progress.log:261), preceded by `guard_ok T-106`
  (progress.log:260).
- `make e2e-task TASK=T-106` → "no claims for T-106", exit 0: statically
  proven — Makefile `e2e-task` echoes and exits 0 on empty claim ids before
  any `npx` invocation, and graph.json:237 has `"claims": []`.
- `?? node_modules` tolerance: `cmd/harness/ratchet.go:127-128` (DirtyLines)
  exempts `node_modules` et al. Real, guard-tolerated, outside owns.
- Task premise: the judge finding is real (harness/findings/judge-M1.md:93-95)
  but was already remediated by `30be90d` before any T-106 worker ran — the
  packet was stale on arrival and its Never clause anticipated exactly this.

## Findings (all minor; none load-bearing)

- **F1 (low, prose):** Handoff open-issue 2 says `30be90d` was "merged via
  T-200". Wrong: progress.log:236 records `merged T-105 30be90d…`, and
  `30be90d`'s own stat includes the T-105 handoff edit. Cosmetic
  mis-attribution; the untracked-on-main conclusion is unaffected.
- **F2 (info):** "State at HEAD `194e9f2`" and the command table's branch-tip
  row describe the pre-handoff snapshot; the handoff itself landed as
  `4f971ee`. Clearly labelled as attempt-2 state; not misleading in context.
- **F3 (info, confirmed):** AGENTS.md flag drift — AGENTS.md says
  `make e2e-task T=<id>` but the Makefile variable is `TASK=` (usage line:
  "usage: make e2e-task TASK=T-###"). Doc-side fix for an operator/docs task.
- **F4 (info, confirmed):** Allowlist gap — topology-changing git verbs are
  denied for task sessions (reproduced here); if retries are expected to
  self-heal divergence, `git rebase` or a sanctioned `scripts/` helper must
  be allowlisted. See also the gate-side rebase defect above.

## Security / architecture / tests

No code, templates, handlers, SQL, static assets, or migrations are touched —
CSRF, XSS via template.HTML, SQLi, path traversal, upload limits, HTMX/#map,
parse-don't-validate, typed IDs, `status.Derive` are all out of scope of this
diff by construction. The handoff contains no secret material (commands and
exit codes only). No "Phase N" strings, no deps added, no test weakened,
skipped, or special-cased; `type: chore`, `claims: []`, `ui: false` — no new
test obligations. Nothing re-added to the index (`git diff --cached` empty at
handoff; branch diff contains no blob additions under the three owned paths).

## Verdict

The worker found the deliverable already on main, took the packet-prescribed
BLOCKED route verbatim, proved it with re-runnable git evidence, ran the
required checks, and — when the gate's own topology defect blocked the merge —
diagnosed it precisely and reported it honestly instead of fabricating a
commit or touching protected state. The only non-handoff hunk in the diff is
the gate's own scheduler-state commit. Recommended operator action is the
handoff's option 1: close T-106 as already-done and drop the branch (no merge
needed); if the history must land, rebase per option 2, which is verified
conflict-free.

VERDICT: PASS
