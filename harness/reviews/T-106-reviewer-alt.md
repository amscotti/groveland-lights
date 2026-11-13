# T-106 — independent review (reviewer-alt), attempt 2

Scope reviewed: `harness/reviews/T-106.diff` (two files: the attempt‑2 handoff
`harness/handoffs/T-106.md` and a one‑line `harness/tasks/graph.json` status
change), the operative packet `harness/tasks/T-106.md`, the worker transcript
`harness/runs/20261113T135642/T-106/worker.jsonl`, the attempt‑1 transcript/reviews,
`harness/progress.log`, `harness/state.md`, `.gitignore`, and the on‑disk tree.

Verification limits: shell execution is denied in this review session (same
allowlist restriction the worker reports), so I re‑verified the git claims from
the worker transcript and the checked‑out tree rather than re‑executing them.
The worktree path itself (`.../wt/T-106`) is outside this sandbox and unreadable
here; the diff is the authoritative artifact.

## Test-integrity / reward-hacking checks

- **No test, spec, fixture, golden or production file is touched.** The diff
  consists of `harness/handoffs/T-106.md` (new file in the reviewed range) and
  `harness/tasks/graph.json` (`harness/reviews/T-106.diff:1-6,135-147`).
  There is no `t.Skip`, `test.skip`, `test.only`, `.fixme`, deleted test,
  narrowed assertion, mock bypass, or fixture literal in app code. The packet is
  `type: chore`, `claims: []` (`harness/tasks/T-106.md:3-17`), so the absent e2e
  run is correct, not dodged: `make e2e-task TASK=T-106` → `no claims for T-106`,
  exit 0 (`worker.jsonl:136`), matching Makefile:112-116.
- **The BLOCKED disposition is honest and independently corroborated, not used
  to dodge a deliverable.** The packet's Never clause (`harness/tasks/T-106.md:46-50`)
  prescribes exactly this handoff first line when the paths are untracked. The
  claim holds:
  - `30be90d` deleted all three blobs (`worker.jsonl:50`), and
    `git log main -- test-run.db` / `task/T-106 -- test-run.db` show the untrack
    after the original add (`worker.jsonl:31,55,63,128-129`);
  - the packet's step‑1 command genuinely refuses at HEAD:
    `fatal: pathspec 'test-run.db' did not match any files`, exit 128
    (`worker.jsonl:122`), and nothing was staged (`worker.jsonl:125`);
  - the three files exist on disk but are ignored: `git status --porcelain
    --ignored` lists all three as `!!` (`worker.jsonl:30,141`), matching
    `.gitignore:20` (`test-run.db*`);
  - `make check-fast` is green, exit 0 (`worker.jsonl:133`).
- **No fabricated success.** The handoff does not claim the gate merge succeeded.
  It explicitly reports that the branch diverges and that the ff‑only gate will
  refuse again (`harness/reviews/T-106.diff:107-110`), and it recommends closing
  the task as already‑done rather than manufacturing an empty deliverable
  commit. That is the correct disposition for a stale packet.
- **The divergence is not self‑inflicted evasion.** The branch‑only commits are
  the harness's own `194e9f2` scheduler‑state commit and the attempt‑1 handoff
  (`worker.jsonl:36`); main‑only commits are harness graph/progress updates
  (`worker.jsonl:40`). The only overlap is the T‑106 graph line. The worker
  attempted `git rebase main`, `GIT_EDITOR=true git rebase main`, `git merge
  main`, `git reset --hard main` and `git cherry-pick 194e9f2`; every one was
  `permission.rejected` (`worker.jsonl:94,97,101,108,111`), and `scripts/` has no
  divergence helper (`worker.jsonl:115-118`). This is an allowlist/retry‑loop
  defect, not test tampering.

## Findings (all non-blocking, documentation/process)

1. **Low — the handoff overstates scope branch‑wide.** The reviewed branch
   contains a `harness/tasks/graph.json` edit (`harness/reviews/T-106.diff:135-147`:
   T‑106 `status: review` → `merging`, leaving `attempts: 0`), yet the handoff's
   compliance section says "no harness file edited beyond this handoff"
   (`harness/reviews/T-106.diff:132-133`). The statement is true of the worker's
   own writes (`worker.jsonl:145` is the only file write; `worker.jsonl:149`
   commits only the handoff), and the same handoff correctly identifies
   `194e9f2` as the harness commit that made the graph change
   (`harness/reviews/T-106.diff:31-33`). It should read "no harness file edited
   by me". Not a test‑integrity issue; the graph change is harness automation,
   not the worker.
2. **Low — the graph hunk would regress scheduler state if it landed.** Main
   currently has `"status": "review", "attempts": 1`
   (`harness/tasks/graph.json:240-241`); the branch's `194e9f2` has
   `"status": "merging", "attempts": 0`. The handoff flags the status/attempts
   conflict and prescribes keeping `merging` with main's `attempts: 1`
   (`harness/reviews/T-106.diff:38-43`). No merge occurs because ff‑only is
   refused, but an operator following only "merge the branch" would reset the
   attempt counter. The handoff's operator options should be read literally.
3. **Low — wrong provenance for `30be90d`.** The handoff says the untrack commit
   is "merged via T-200" (`harness/reviews/T-106.diff:117-118`), but
   `harness/progress.log:236` records `merged T-105 30be90d…` (T‑105's merge tip;
   `30be90d`'s own stat includes the T‑105 handoff edit, `worker.jsonl:50`).
   Cosmetic; the "untracked on main" conclusion is unaffected.
4. **Info — stale HEAD labels.** "State at HEAD `194e9f2`" and the command table's
   "branch tip `194e9f2`" (`harness/reviews/T-106.diff:35,98`) describe the
   pre‑handoff state; the handoff itself was committed as `4f971ee`
   (`worker.jsonl:149,153`). The wording makes the committed artifact read as a
   stale HEAD. Not material.
5. **Info — `?? node_modules` tolerance is correctly described.** The guard's
   `DirtyLines` exempts `node_modules` (`cmd/harness/ratchet.go:127-128`), and
   the only worktree dirt is that pre‑existing symlink (`worker.jsonl:4,154`).
   Open issue 3 in the handoff is accurate.

## Conclusion

The worker found the task already completed on main, stopped at the
packet‑prescribed `BLOCKED: already untracked on main`, proved it with git
plumbing snapshots, ran the required checks, and did not fabricate a commit,
edit a spec, skip a test, or special‑case a fixture. The only diff content
beyond the handoff is a harness scheduler commit that the handoff transparently
documents. No test-integrity or reward-hacking failure exists in this diff.

VERDICT: PASS
