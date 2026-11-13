# T-106 review (attempt 3) — BLOCKED disposition on "already untracked on main"

Reviewed artifact: `harness/reviews/T-106.diff` — the branch-introduced changes
(merge-base `06e5b8a` → `task/T-106` = `43159a9`): the attempt-3 handoff
`harness/handoffs/T-106.md` (new, 130 lines) and a one-line
`harness/tasks/graph.json` status hunk. Verified against the packet
`harness/tasks/T-106.md`, the main checkout, the attempt-3 worker transcript
`harness/runs/20261113T141329/T-106/worker.jsonl`, `harness/progress.log`,
`cmd/harness/gate.go`, and `.gitignore`.

Method note: this review session's allowlist permits a subset of read-only git
verbs only (`log/show/diff/grep`-style); every load-bearing git claim below was
either re-executed directly or corroborated against the harness's own log and
the worker transcript. The same allowlist wall the handoff reports was
reproduced here (`git ls-files`, `git branch`, compound commands: denied).

## Packet conformance — BLOCKED route prescribed, precondition re-proven

- Never clause (harness/tasks/T-106.md:46-50): "If the files are NOT tracked
  in your worktree (someone already fixed it), write `BLOCKED: already
  untracked on main` in the handoff and stop." Handoff line 1 is that exact
  string.
- The precondition genuinely holds — re-verified independently, not taken from
  the handoff:
  - `git log --oneline -- test-run.db test-run.db-shm test-run.db-wal` →
    exactly `30be90d` (untrack) then `7f07df2` (accidental add), nothing
    after; `30be90d` is on main's first-parent line and reachable from
    `task/T-106` (it appears in the branch's path history).
  - `git diff main task/T-106 -- test-run.db test-run.db-shm test-run.db-wal`
    → EMPTY: identical (untracked) tree state for all three paths at both
    tips. Nothing re-added on either side.
  - `.gitignore:20` = `test-run.db*`; all three files exist on disk as runtime
    artifacts (this checkout: test-run.db 4 KB, -shm 32 KB, -wal 124 KB) and
    are invisible to `git status --porcelain` (clean here).
- Packet step 1 therefore has nothing to remove; the handoff accurately
  reports that `git rm --cached …` was attempted and denied this session
  (worker.jsonl:46), with attempt 2's executed run (exit 128, "did not match
  any files") as the definitive prior proof. Correctly, no empty commit
  wearing the prescribed message was fabricated.
- Required checks ran and passed: `make check-fast` exit 0 with full output —
  vet, golangci-lint `0 issues.`, `go test -race -shuffle=on` all ok
  (worker.jsonl:72; corroborated by `check_fast_ok T-106` at
  harness/progress.log:280 and `guard_ok T-106` at progress.log:279).
  `make e2e-task TASK=T-106` → `no claims for T-106`, exit 0 (worker.jsonl:76)
  — correct, not dodged: `claims: []`, `ui: false`
  (harness/tasks/graph.json:237-238).

## The graph.json hunk is gate-authored, not a worker edit of protected state

The diff's only non-handoff change (`status: review → merging`, attempts
untouched) is commit `ba1a039`. Three independent proofs it is the gate's own
commit, per the pattern verified for attempt 2's `194e9f2`:

1. Message verbatim from the gate: `chore(harness): keep scheduler graph
   state on merge` exists only at `cmd/harness/gate.go:127` —
   `finalizeMerge` restores main's authoritative graph.json onto the branch
   "so the merge cannot regress task status/attempts" (gate.go:122-123).
2. Timing: `ba1a039` CommitDate 2026-11-13T14:13:21Z is the exact second of
   `gate_fail T-106` (harness/progress.log:264), 8 seconds BEFORE the
   attempt-3 worker claim at 14:13:29Z (progress.log:276). No worker session
   was active when it was created.
3. Content: exactly the one status line, matching main's graph state at gate
   time; the worker's own commits are handoff-only (see below).

The handoff openly labels `ba1a039` as harness graph state — accurate.

## Hard-rule compliance

- Rule 1 (protected paths): the worker's writes/commits across all three
  attempts touch ONLY `harness/handoffs/T-106.md` — verified by `--stat` on
  `fdab4ed` (81+), `525e743` (rewrite), `43159a9` (90+/88-, attempt 3). The
  full branch diff is exactly two files (handoff + the gate's graph hunk):
  `git diff --stat 06e5b8a task/T-106` → 2 files, 131 insertions. Nothing
  under `acceptance/`, no Makefile/CI/lint-config/golden edits.
- Rules 2/3/8: no tests touched or weakened, no deps added (`allow_deps: []`
  respected), no migrations, N/A for status.Derive.
- Rule 9: no TODO/FIXME/panic/console.log introduced (handoff-only content).
- Rule 10: handoff committed (`43159a9`), passing gitleaks ("no leaks found")
  and the conventional-commit hook (worker.jsonl:87). `git diff --cached`
  empty at both transcript checkpoints (worker.jsonl:66, 91).

## Handoff accuracy — spot-checked claims, all reproduce

- `git show main:harness/handoffs/T-106.md` → exit 128, "does not exist in
  'main'" — reproduced; handoff commits replay conflict-free as claimed.
- Main's graph state at handoff-write time: `running/2` at `b9b70be` —
  reproduced (`git diff b9b70be main -- harness/tasks/graph.json` shows only
  `running → review`, attempts 2 both sides; main has since gained review
  artifacts `b61e168`, `c890647`).
- Divergence 3-vs-3 at write time — reproduced from the graph (branch-only
  `fdab4ed`/`525e743`/`ba1a039` vs main-only `3a12cc1`/`dd03624`/`b9b70be`);
  sole file overlap `harness/tasks/graph.json`.
- Denials table — corroborated verbatim in the transcript: `git rm --cached
  test-run.db test-run.db-shm test-run.db-wal` (worker.jsonl:46), `git rebase
  main` (worker.jsonl:50), `git merge main` (worker.jsonl:54), all
  permission.rejected. The handoff's note that this session is narrower than
  attempt 2's is also corroborated (attempt 2 could run `rm --cached`; several
  read-only invocations were additionally denied here, worker.jsonl:2,6,7,11,14).
- The incorrect "merged via T-200" provenance for `30be90d` from attempt 2's
  handoff (prior review F1) has been dropped in attempt 3 — open issue 2 now
  states the facts correctly (`30be90d` untracked the blobs; `.gitignore:20`
  guards re-tracking).

## Security / architecture / tests

No code, templates, handlers, SQL, static assets, or migrations are touched —
CSRF, XSS via template.HTML, SQLi, path traversal, upload limits, HTMX/#map,
parse-don't-validate, typed IDs, `status.Derive` are out of scope of this diff
by construction. No secret material in the handoff (commands and exit codes
only; gitleaks clean on the commit). No "Phase N" strings in the rule-5 scoped
dirs. No test obligations (`type: chore`, `claims: []`, `ui: false`). Nothing
re-added to the index.

## Findings (all minor; none load-bearing)

- **F1 (info): stale self-reference.** "Branch tip is now `ba1a039`" and
  "3 vs 3" describe the pre-handoff snapshot; the handoff itself landed as
  `43159a9` (4th branch-only commit) and main has since advanced
  (`b61e168`, `c890647`). Same cosmetic pattern as attempt 2's F2; clearly
  labelled as fresh-at-verification state, not misleading.
- **F2 (info): operator-merge regression risk.** If an operator hand-merges
  per option 2 and blindly takes the branch's graph blob, main's T-106 state
  would regress `attempts: 2 → 1` and `status: review → merging`. The
  handoff's resolution note ("keep the branch's `status: "merging"` and take
  main's `attempts` value") covers exactly this; the ff-only gate refuses the
  naive merge anyway. Read option 2's resolution literally.
- **F3 (info, carried):** AGENTS.md flag drift — `make e2e-task T=<id>` vs
  the Makefile's `TASK=`. Confirmed real in attempt 2's review; doc-side fix,
  outside this task's owns.
- **F4 (info, carried — harness-side, third occurrence):** the retry loop is
  not fixable by a worker: the gate's rebase step (gate.go:88) does not
  actually integrate main (no `gate_conflict T-106` event exists in
  progress.log), `finalizeMerge`'s ff-only merge (gate.go:130) then refuses,
  and every topology-changing git verb is denied in task sessions. The worker
  has now documented this precisely three times with fresh evidence each
  time. Recommended operator action remains the handoff's option 1: close
  T-106 as already-done (deliverable exists on main as `30be90d`; the branch
  carries zero code changes) and drop the branch.

## Verdict

The task's premise is stale — the judge-M1 hygiene finding was remediated on
main by `30be90d` before any T-106 worker ran, and the packet's Never clause
prescribes exactly the disposition the worker took, verbatim, with re-runnable
git evidence re-verified fresh this attempt rather than carried over. The only
non-handoff hunk in the diff is the gate's own scheduler-state commit. Checks
ran green, nothing was fabricated, nothing re-added, no protected file was
edited by the worker, and the residual merge divergence is a harness-side
defect outside the worker's power, correctly escalated with operator options.

VERDICT: PASS
