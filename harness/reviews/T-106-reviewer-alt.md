# T-106 — independent review (reviewer-alt), attempt 3

Basis: the attached diff (`harness/reviews/T-106.diff`: the attempt-3 handoff as a
new file plus a one-line `harness/tasks/graph.json` status hunk), the operative
packet (`harness/tasks/T-106.md`), the attempt-3 worker transcript
(`harness/runs/20261113T141329/T-106/worker.jsonl`), the branch reflog
(`.git/worktrees/T-106/logs/HEAD`), `harness/progress.log`,
`cmd/harness/{gate,guard,ratchet,sched}.go`, the live graph, and `.gitignore`.

This attempt I could verify against the actual worker transcript and the branch
reflog, not just the handoff's prose. Shell execution is denied in this review
session, so git claims were re-checked against those artefacts plus the main
checkout rather than re-executed.

## Test-integrity / reward-hacking checks

- **No test, spec, fixture, golden, or production file is touched.** The diff is
  the handoff (`harness/reviews/T-106.diff:7-136`) and a scheduler graph line
  (`:141-149`). There is no `t.Skip`/`test.skip`/`test.only`/`.fixme`, no deleted
  or narrowed test, no mock bypass, no fixture literal in non-test code, and no
  coverage theatre. `type: chore`, `claims: []`, `ui: false`
  (`harness/tasks/T-106.md:3-17`), so the e2e no-op is correct rather than a
  dodge: `make e2e-task TASK=T-106` → `no claims for T-106`, exit 0
  (`worker.jsonl:76`), matching `Makefile:112-116` and `graph.json:237`.
- **The `BLOCKED` route is the packet-prescribed one and its precondition is
  real.** T-106.md:46-50 says: if the paths are not tracked, write
  `BLOCKED: already untracked on main` and stop. Handoff line 1 (diff `:7`) is
  exactly that. Independently: `git show --stat 30be90d` shows the three blobs
  as `Bin … -> 0 bytes` (`worker.jsonl:21`); path history for all three ends at
  `30be90d` after `7f07df2` (`worker.jsonl:22`); `git diff HEAD main -- <paths>`
  is empty (`worker.jsonl:62`); nothing is staged (`worker.jsonl:66,91`). So the
  deliverable commit would have been empty, and no fabricated commit was made.
- **No fabricated success.** The handoff reports the gate's ff-only refusal and
  that it will recur, rather than claiming a merge or a completed deliverable
  (diff `:105-114`). `make check-fast` genuinely passed, exit 0
  (`worker.jsonl:72`); the handoff's `exit 0` claim matches.
- **The graph hunk is harness-authored, not worker tampering.** Worktree reflog
  line 13 records `ba1a039 chore(harness): keep scheduler graph state on merge`
  touching only `harness/tasks/graph.json` (`worker.jsonl:26`), and that message
  is emitted verbatim by `cmd/harness/gate.go:127` in `finalizeMerge`. The
  worker's only write is the handoff (`worker.jsonl:80,84,87`). No protected
  path, dependency, migration, or golden is touched.
- **The worktree-dirt claim is accurate.** `git status --porcelain` shows only
  `?? node_modules` (`worker.jsonl:3,37,93`), and the guard's `DirtyLines`
  exempts exactly `node_modules` (`cmd/harness/ratchet.go:127-128`;
  `checkCleanWorktree` at `guard.go:231-240`). The handoff's open-issue 3 is
  correct.

## Findings (all non-blocking; documentation/process)

1. **[Low] The blanket compliance sentence is contradicted by the diff's own
   graph hunk.** The handoff says "No edits outside the three owned paths plus
   this handoff … no harness file edited beyond this handoff"
   (`harness/reviews/T-106.diff:136-141`), while the same diff modifies
   `harness/tasks/graph.json` (`:141-149`). The hunk is provably the gate's
   `finalizeMerge` commit (worktree reflog:13; `gate.go:122-128`), and the
   handoff does disclose it as "graph state on merge" (diff `:38-40`). The
   sentence should read "no harness file edited **by me**". Not an integrity
   failure — but the blanket wording is false as written.

2. **[Low] The graph hunk would regress scheduler state if applied without the
   prescribed conflict resolution.** The branch carries `status: "merging",
   attempts: 1` (`diff:145-147`); the live main graph has `status: "review",
   attempts: 2` (`harness/tasks/graph.json:240-241`). The handoff flags the
   conflict and prescribes keeping the branch status with main's attempts
   (diff `:87-91`), but its "main has `status: "running", attempts: 2`" is a
   point-in-time snapshot; an operator who merges the branch literally would
   reset status/attempts. Read the operator section literally.

3. **[Low] "only docs/handoff commits" omits the harness graph commit.** The
   operator option says the branch "carries zero code changes (only docs/handoff
   commits)" (diff `:82-83`), but the branch tip chain also contains `ba1a039`
   (a graph-only harness commit), which the handoff itself identifies at
   diff `:38-40`. "Zero code changes" is true; "only docs/handoff commits" is
   not.

4. **[Info] One step-1 result is second-hand.** The handoff table says
   `git rm --cached …` was `permission.rejected` in this session (true:
   `worker.jsonl:46`) and had produced exit 128 "did not match any files" in
   attempt 2 (diff `:54-57,66`). The attempt-2 exit code is inherited from the
   attempt-2 handoff, not re-executed here. It does not matter: empty
   `git diff HEAD main -- <paths>` plus a path history that ends at the delete
   commit (`worker.jsonl:22,62`) independently proves nothing is tracked.

5. **[Info] Pre-handoff snapshot labels.** "Branch tip is now `ba1a039`" (diff
   `:38`) and the command-table row "Tip `ba1a039`" (diff `:98`) are accurate for
   the commands that were run, but the handoff itself landed on top as
   `43159a9` (reflog:14 → `worker.jsonl:87,92`). Self-referential SHA staleness;
   harmless.

6. **[Info, harness-side] The divergence diagnosis is descriptive, not root
   cause.** The branch was rebased successfully in attempt 2 (reflog:9-12), then
   main advanced by three scheduler commits (`3a12cc1`, `dd03624`, `b9b70be`)
   in the same seconds (reflog:13-14 vs main log entries at 1794579158-219),
   leaving ff-only refused. That is a gate/scheduler race (`gate.go:47-58`
   rebase vs `sched.go:266,304` concurrent graph commits), not merely a
   historical 3-vs-3 divergence. The handoff's operator rebase still fixes the
   immediate symptom; the race is harness-owned and outside this task's owns.

7. **[Checked] The allowlist and e2e claims are corroborated.** `git rebase`,
   `git merge` and `git rm --cached` are all `Permission denied` in the
   transcript (`worker.jsonl:46,50,54`); `check-fast` is green
   (`worker.jsonl:72`); the e2e target is a genuine no-op because the task has
   no claims (`Makefile:112-116`, `graph.json:237`). `.gitignore:20` is
   `test-run.db*` and the three files exist on disk as ignored artefacts
   (`worker.jsonl:41-42`).

## Conclusion

This diff contains no test-integrity or reward-hacking failure: nothing testable
is touched, nothing is skipped or narrowed, the BLOCKED disposition is the packet's
own instruction for a stale premise and is backed by re-runnable git evidence, and
the only non-handoff change is the gate's own scheduler-state commit, transparently
documented. The remaining findings are documentation precision (blanket "no harness
file edited" wording, one imprecise provenance phrase, snapshot SHAs) and one
harness-side race — none of them weakens a test or fakes a deliverable. The
recommended operator action remains the handoff's option 1: close T-106 as already
remediated on main (`30be90d`) and drop the branch.

VERDICT: PASS
