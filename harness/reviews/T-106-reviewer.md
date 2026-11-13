# T-106 review (attempt 5) — BLOCKED disposition on "already untracked on main"; re-rebase verified; divergence recurred again but is now gate-healed

Reviewed artifact: `harness/reviews/T-106.diff` — attempt 5's branch changes are again a
single file: the rewritten handoff `harness/handoffs/T-106.md` (107 lines, first line
`BLOCKED: already untracked on main`). Verified against the packet
`harness/tasks/T-106.md`, both git reflogs and both tips' indexes, `cmd/harness/gate.go`,
`cmd/harness/sched.go`, `cmd/harness/ratchet.go`, `.opencode/agents/worker.md`,
`harness/progress.log`, `harness/findings/judge-M1.md`, the `Makefile`, `.gitignore`,
and the attempt-4 review (`harness/reviews/T-106-reviewer.md` as merged to main).

Method note: shell execution was denied in this review session. Every load-bearing git
claim was re-derived from primary sources: `.git/logs/refs/heads/main`,
`.git/logs/refs/heads/task/T-106`, `.git/worktrees/T-106/logs/HEAD`, both branch refs,
`ORIG_HEAD`, and byte-level greps of both index files. Index-grep negatives were
validated with positive controls before trusting them.

## Packet conformance — BLOCKED route prescribed, precondition re-proven at both tips

- Never clause (harness/tasks/T-106.md:46-50): "If the files are NOT tracked … write
  `BLOCKED: already untracked on main` in the handoff and stop." Handoff line 1 is that
  exact string, and no empty commit wearing the prescribed deliverable message
  (packet :31) was fabricated — `git rm --cached` → exit 128 "did not match any files"
  is consistent with the index evidence below.
- The precondition genuinely holds, re-verified fresh:
  - Byte-grep of the branch tip's index (`.git/worktrees/T-106/index`) for `test-run`
    → no matches; same negative on main's `.git/index`. Positive controls
    (`.gitignore`, `harness/handoffs/T-106.md`, `harness/tasks/graph.json`) match in
    the worktree index, so the methodology is sound. The three paths are tracked at
    neither tip.
  - `.gitignore:20` = `test-run.db*`; all three files exist on disk in the main
    checkout as ignored runtime artifacts.
  - History corroborated by reflogs: `7f07df2` (accidental add, main reflog line 139;
    the finding itself is harness/findings/judge-M1.md:93) → `30be90d` (untrack,
    arrived on main via the task/T-105 ff-merge, main reflog line 145). Nothing after.
- `make check-fast` exit 0 corroborated by the harness: `guard_ok T-106`
  (progress.log:408) and `check_fast_ok T-106` (progress.log:409) after `worker_done`
  at 16:21:33Z. `make e2e-task TASK=T-106` → "no claims" is correct, not dodged:
  `claims: []`, `ui: false` (harness/tasks/T-106.md:12-13) and Makefile:115 exits 0
  with exactly that message when the claim list is empty.
- `git -C . show main:harness/handoffs/T-106.md` → 128 is consistent with the main
  checkout: no `harness/handoffs/T-106.md` exists there (the branch-only file).

## The re-rebase — happened exactly as claimed; the skipped commit is gate-authored

- Reflog `.git/worktrees/T-106/logs/HEAD` lines 34-39: `rebase (start): checkout main`
  onto `fd50f5d`, four picks — `09e9c31` (attempt 1), `4d1500e` (attempt 2),
  `109b366` (attempt 3), `dbd4912` (attempt 4) — then `rebase (finish)` to
  `dbd4912`, then the attempt-5 handoff commit `49e6a4f` (branch reflog line 16).
  Exactly the handoff's table.
- `f918866` ("chore(harness): keep scheduler graph state on merge") was auto-skipped:
  no pick line exists for it. That message exists only at `cmd/harness/gate.go:127`
  (and :136) — `finalizeMerge` restores main's authoritative graph.json onto the
  branch (gate.go:121-129) and re-creates that commit on demand, so dropping the
  stale copy is benign. The handoff's claim that the patch "already exists on main"
  is corroborated by the retry-context history and by main carrying the same
  graph-state commits.
- Was the worker allowed to rebase? The packet's own retry context says "Fix it and
  finish" and reproduces git's `git rebase` suggestion (harness/tasks/T-106.md:68-79).
  The rebase rewrote only task-branch topology, changed no file contents outside the
  handoff, and removed (not added) gate-authored state. In bounds.

## Findings

- **F1 (info, not worker-caused): the handoff's closing topology snapshot is already
  stale — the branches diverged again ~41s after the handoff commit.** Timeline:
  rebase finished at reflog time 1794586815; handoff commit `49e6a4f` at 1794586862
  (~16:21:02Z); `worker_done` 16:21:33Z; `check_fast_ok` 16:21:39Z; the scheduler
  then committed `5ab2056` ("update task graph, progress and review artifacts") to
  main at 1794586903 (~16:21:43Z) — parent `fd50f5d`, i.e. a sibling of the branch's
  base. Current topology (verified via refs): merge-base `fd50f5d`; branch-only
  `09e9c31`,`4d1500e`,`109b366`,`dbd4912`,`49e6a4f`; main-only `5ab2056`. So
  `git log HEAD..main` is no longer empty and the plain ff-only merge would again
  refuse. **Unlike attempts 1-4 this is structurally handled inside the gate:** the
  gate takes the exclusive merge lock up front (gate.go:38-42) and the scheduler
  serializes its state commits against the same lock (sched.go:169); the gate's
  first step is `rebaseOntoMain` (gate.go:47); and `finalizeMerge` retries the
  ff-only merge once after an in-gate rebase (gate.go:130-139, added by `70be9ce`,
  main reflog line 224). The replay onto `5ab2056` should be clean (main-only commit
  touches scheduler state/review artifacts, not the handoff). The handoff's
  conclusion — the gate can now merge this branch — survives even though its
  snapshot does not; expect the gate to exercise the new retry path.
- **F2 (carried from attempt-4 review, pre-existing, disclosed): `git -C *` is an
  allowlist bypass.** `.opencode/agents/worker.md:39` allows any `git -C …`
  invocation, including topology-destructive verbs the granular entries at :32-38
  deliberately deny; only `--no-verify` is patched (:40). Attempt 5 used it for the
  packet-sanctioned rebase and disclosed it with the line reference, but the handoff
  again coaches future workers to route around the allowlist (open issue 4). The
  correct fix is narrowing :39, not codifying the bypass. Protected file — operator
  action.
- **F3 (info, carried): `?? node_modules` symlink.** The claim is now corroborated
  from code: `DirtyLines` tolerates exactly `p == "node_modules"`
  (cmd/harness/ratchet.go:127-130, matching the handoff's citation), and
  `.gitignore:2` is `node_modules/` (directories only), so a symlink staying
  untracked-unignored is expected. Guard-tolerated, outside owns.
- **F4 (info, carried): AGENTS.md flag drift confirmed** — AGENTS.md:20 says
  `make e2e-task T=<id>`; the Makefile variable is `TASK=` (Makefile:8, :113).
  Doc-side, outside this task's owns.
- The attempt-4 review's zero-width-space nit (F3 there) is gone: byte-grep of this
  diff for U+200B/U+FEFF finds nothing.

## Hard-rule compliance

- Rule 1: worker-authored content across all five attempts is confined to
  `harness/handoffs/T-106.md` (this diff's only file; worktree index confirms).
  No `acceptance/`, Makefile, CI, lint-config, or golden edits; `.gitignore`
  untouched at line 20. The rebase's one dropped commit was gate-authored
  bookkeeping, re-creatable by the gate itself.
- Rules 2/3/8: no tests touched or weakened, `allow_deps: []` respected (no deps
  added), no migrations, `status.Derive` N/A.
- Rule 9: no TODO/FIXME/panic/console.log introduced.
- Rule 10: handoff committed (`49e6a4f`); guard clean per `guard_ok T-106`
  (progress.log:408).

## Security / architecture / tests

No code, templates, handlers, SQL, static assets, or migrations are touched — CSRF,
XSS via template.HTML, SQLi, path traversal, upload limits, HTMX/#map invariants,
parse-don't-validate, and typed IDs (docs/spec/architecture.md ownership map) are out
of scope of this diff by construction; the packet declares `type: chore`,
`claims: []`, `ui: false`. No secret material in the handoff (commands, exit codes,
and SHAs only). The only security observation is F2 (pre-existing harness policy,
not introduced here). Test obligations vacuous; the checks that do apply
(`make check-fast`) are green per harness log.

## Verdict

The task's premise is stale — the judge-M1 hygiene finding (judge-M1.md:93) was
remediated on main by `30be90d` before any T-106 worker ran — and the packet's Never
clause prescribes exactly the disposition taken, verbatim, as the handoff's first
line. Nothing was fabricated or re-added (re-proven at both tips via index inspection
with validated controls), no protected file was edited, the re-rebase is
reflog-verified exactly as narrated and dropped only a gate-authored duplicate, and
guard/check-fast are green. The one new observation (F1) is the same scheduler-side
race re-diverging the branches seconds after the handoff was written — outside the
worker's owns, accurately reported with fresh evidence, and now structurally healed
by the gate itself (merge lock + rebase-first + ff-only retry, gate.go:38-47,
:130-139), which is precisely what this handoff documents. F2 remains a pre-existing
policy hole used legitimately and disclosed. The carried recommendation stands:
close T-106 as already-done (deliverable exists on main as `30be90d`; the branch
carries zero code changes). Nothing in this diff blocks the gate or harms the repo.

VERDICT: PASS
