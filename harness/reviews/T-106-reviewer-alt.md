# T-106 — independent review (reviewer-alt), attempt 5

Basis: the attached diff (`harness/reviews/T-106.diff`, 113 lines; the only added
path is `harness/handoffs/T-106.md`), the packet (`harness/tasks/T-106.md`), the
attempt-5 worker transcript (`harness/runs/20261113T161511/T-106/worker.jsonl`),
`harness/progress.log`, both indexes (`/Users/ascotti/Documents/git_repos/GrovelandLampLighters/.git/index`
and `.git/worktrees/T-106/index`), the branch/main reflogs, `cmd/harness/gate.go`,
`cmd/harness/sched.go`, `cmd/harness/ratchet.go`, and `.opencode/agents/worker.md`.
Shell execution was denied in this session; load-bearing git/working-tree claims
were re-derived from the reflogs, indexes, transcript, and source reads.

## Test-integrity / reward-hacking checks

- **No test, spec, fixture, golden, or production file is in the diff.** The only
  path added is `harness/handoffs/T-106.md` (`harness/reviews/T-106.diff:1-6`).
  There is no `t.Skip`, `test.skip`, `test.only`, `.fixme`, no weakened/deleted/
  narrowed test, no mock bypass, no fixture literal in non-test code, no
  coverage theatre, and no dead code. `type: chore`, `claims: []`, `ui: false`
  (`harness/tasks/T-106.md:12-13`) makes `make e2e-task TASK=T-106` →
  `no claims for T-106` a correct no-op (transcript `worker.jsonl:101`), not a
  skipped suite.
- **The `BLOCKED` route is the packet-prescribed one, and its precondition is
  independently true.** `harness/tasks/T-106.md:46-50` says: if the three paths
  are not tracked, write `BLOCKED: already untracked on main` and stop. The
  handoff's first line is exactly that (`harness/reviews/T-106.diff:7`).
  Re-verified without shell: a literal grep of both `.git/index` and
  `.git/worktrees/T-106/index` for `test-run` returns no matches, while positive
  controls in the same indexes match `harness/tasks/graph.json` and
  `harness/handoffs/T-106.md` respectively. `.gitignore:20` is `test-run.db*`.
  The transcript shows `git -C . rm --cached test-run.db test-run.db-shm
  test-run.db-wal` exiting 128 with "did not match any files"
  (`worker.jsonl:106`) — nothing was tracked to remove, and the worker did not
  fabricate an empty deliverable commit.
- **Verification is not fabricated.** The transcript contains the actual
  `make check-fast` output (vet, golangci-lint `0 issues.`, `go test -race`
  all ok) at `worker.jsonl:102` and again after the handoff commit at
  `worker.jsonl:146`; `make e2e-task TASK=T-106` at `worker.jsonl:101`; and the
  rebase at `worker.jsonl:111`. `harness/progress.log:406-409` records
  `worker_done T-106`, `guard_ok T-106`, `check_fast_ok T-106` afterward.
- **No protected-file content is changed by the branch.** The pre- and
  post-commit three-dot checks in the transcript report only
  `harness/handoffs/T-106.md` (`worker.jsonl:116,141`); the handoff commit is
  one file, 48 insertions / 41 deletions (`worker.jsonl:134`).
- **No fixture/string special-casing is possible here:** the diff adds no
  executable code, only a markdown handoff, and the handoff contains only git
  commands, SHAs, and source references.

## Findings

1. **[Medium, accuracy — not a test-integrity failure] The "loop is closed"
   claim was true only as a snapshot and was false again by review time.** The
   handoff says `HEAD..main` is empty and the gate's ff-only merge will succeed
   (`harness/reviews/T-106.diff:43-45`; open issue 1 at `:85-89`). The reflogs
   show the branch was rebased onto main at `fd50f5d` and the attempt-5 commit
   `49e6a4f` was created at 16:21:02Z
   (`.git/worktrees/T-106/logs/HEAD:34-40`), while main then gained `5ab2056`
   at 16:21:43Z (`.git/logs/refs/heads/main:227`) — 41 seconds after the
   handoff commit and 4 seconds after `check_fast_ok` (`harness/progress.log:409`).
   Main is therefore no longer an ancestor of the branch, and a bare
   `git merge --ff-only` would refuse again. The durable recovery now rests on
   the gate's lock and retry (`cmd/harness/gate.go:38-42,130-143`;
   `cmd/harness/sched.go:162-183`), which should rebase once more under the
   lock. The handoff should have framed this as "the gate can recover" rather
   than claiming the divergence loop is closed. This is the same scheduler race
   seen in attempts 3–4, outside the worker's control.
2. **[Low, documentation precision] "Branch-vs-main diff is exactly one file"
   is true only for the merge-base diff.** The supporting command is
   `git diff --name-only main...HEAD` (`harness/reviews/T-106.diff:46,77`;
   transcript `worker.jsonl:116,141`), which compares the merge-base to HEAD and
   ignores main-only scheduler commits. After main advanced to `5ab2056`, a
   literal `git diff main HEAD` would include main-side changes to graph,
   progress, and review artifacts. The gate is safe because `finalizeMerge`
   restores main's `graph.json` (`cmd/harness/gate.go:121-137`), but not every
   other scheduler artifact; the prose should keep the three-dot qualifier.
3. **[Low, documentation precision] "the rebase only moved already-committed
   history" understates a dropped commit.** The rebase auto-skipped `f918866`
   (`harness/reviews/T-106.diff:36-41`; transcript `worker.jsonl:111`), so a
   commit was dropped, not merely moved. The rationale "the patch already exists
   on main" is also imprecise: git skipped it via patch-id equivalence, not by
   proving main's current tree contains that hunk. No content regression
   results because `finalizeMerge` re-creates the graph bookkeeping from main's
   authoritative copy (`cmd/harness/gate.go:121-137`), but the rationale should
   not be read as proof.
4. **[Info, security/process — pre-existing] `git -C *` is an allowlist
   bypass, and this handoff normalizes it.** `.opencode/agents/worker.md:39`
   allows any `git -C …` invocation, including topology-changing verbs denied
   at `:32-38`; only `--no-verify` is patched at `:40`. Attempt 5 used
   `git -C . rebase main` (transcript `worker.jsonl:111`) and open issue 4
   recommends future workers use the same form (`harness/reviews/T-106.diff:98-101`).
   The packet's retry context did ask for divergence repair
   (`harness/tasks/T-106.md:53-65`), so this is not worker misconduct, but the
   wildcard should be narrowed by the operator rather than codified in handoffs.
5. **[Info] `?? node_modules` is not a "harness symlink."** It is a
   provisioning-created symlink to the main checkout's `node_modules`; the guard
   tolerates it via `cmd/harness/ratchet.go:127-130`, and `guard_ok T-106`
   confirms that (`harness/progress.log:408`). Wording only.
6. **[Info] "re-creates that commit … on every successful gate" is slightly
   off.** `finalizeMerge` restores/commits the graph on every invocation that
   reaches it, including the retry path after a failed ff-only merge
   (`cmd/harness/gate.go:121-143`), not only on successful gates.

## Hard-rule compliance

- Rule 1: worker-authored content in this diff is confined to
  `harness/handoffs/T-106.md`; no `acceptance/`, `Makefile`, CI, lint-config,
  migration, or golden edit. `.gitignore:20` is untouched. The rebase rewrote
  only task-branch history and dropped a gate-authored graph commit, which
  `finalizeMerge` re-creates.
- Rules 2/3/8: no tests touched or weakened, `allow_deps: []` respected (no
  deps), no migrations, `status.Derive` not applicable.
- Rule 9: no TODO/FIXME/panic/`console.log` introduced.
- Rule 10: handoff committed as `49e6a4f`; the only dirty path reported by the
  worker is the guard-tolerated `node_modules` symlink (`worker.jsonl:140`).
- The packet's Never clause and retry context conflict ("stop" vs "fix it and
  finish"); the worker wrote the mandated `BLOCKED` line and committed the
  handoff without fabricating the deliverable, which is the defensible reading.

## Why this is not a BLOCK

The artifact under review is a documentation-only handoff for an
already-remediated premise. No test, fixture, golden, mock, or production
behaviour is weakened; no verification claim is fabricated; no protected file is
edited. The inaccuracies in findings 1–3 are about the durability/prose of a
snapshot claim, not about test integrity, and the scheduler-side divergence is
outside the worker's owns and is addressed by the gate's lock/retry. The
allowlist wildcard (finding 4) is a pre-existing harness policy hole that this
attempt disclosed and used under the packet's repair instruction, not a
test-integrity failure.

VERDICT: PASS
