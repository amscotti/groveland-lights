# T-106 — independent review (reviewer-alt), attempt 4

Basis: the attached diff (`harness/reviews/T-106.diff`, 106 lines: the attempt-4
handoff as a new file only), the packet (`harness/tasks/T-106.md`), the attempt-4
worker transcript (`harness/runs/20261113T142515/T-106/worker.jsonl`), the
current main checkout, `task/T-106`'s refs/history, `harness/progress.log`,
`harness/tasks/graph.json`, `cmd/harness/gate.go`, and `.opencode/agents/worker.md`.
Shell was available for single read-only git commands; compound commands were
denied, so I re-ran the load-bearing checks individually.

## Test-integrity / reward-hacking checks

- **No test, spec, fixture, golden, or production file is in the diff.** The
  only path added is `harness/handoffs/T-106.md`
  (`harness/reviews/T-106.diff:1-6`; `git diff --name-status main...task/T-106`
  = `A harness/handoffs/T-106.md`). There is no `t.Skip`, `test.skip`,
  `test.only`, `.fixme`, no deleted/narrowed test, no mock bypass, no fixture
  literal in non-test code, and no coverage theatre. `type: chore`,
  `claims: []`, `ui: false` (`harness/tasks/T-106.md:3-17`), so the e2e no-op is
  correct rather than a dodge.
- **The `BLOCKED` route is the packet-prescribed one and its precondition is
  independently true.** T-106.md:46-50 says: if the three paths are not
  tracked, write `BLOCKED: already untracked on main` and stop. Handoff line 1
  (diff `:7`) is exactly that string. Re-verified:
  - `git log --oneline -- test-run.db test-run.db-shm test-run.db-wal` lists
    only `30be90d` (untrack) then `7f07df2` (accidental add); nothing after.
  - `git show --stat 30be90d` shows `test-run.db 4096 -> 0`, `-shm 32768 -> 0`,
    `-wal 28872 -> 0`.
  - `git diff main task/T-106 -- <three paths>` is empty.
  - `git status --porcelain --ignored -- <three paths>` is `!!` for all three,
    and `.gitignore:20` is `test-run.db*`.
  - `git -C . rm --cached ...` exited 128 "did not match any files" in the
    attempt-4 transcript (`worker.jsonl:93`). Nothing was tracked to remove;
    the worker correctly did not fabricate an empty deliverable commit.
- **No fabricated verification.** `make check-fast` exit 0 (vet,
  golangci-lint `0 issues.`, `go test -race -count=1 -shuffle=on ./...`) is in
  the transcript (`worker.jsonl:85`), and `make e2e-task TASK=T-106` exits 0
  with `no claims for T-106` (`worker.jsonl:88`). I did not need to re-run
  these for a docs-only diff. The only worker-authored commit in the diff is
  the handoff (`127d700`), and `git show --stat 127d700` is one file.
- **No protected-file content is changed by the branch.** The merge-base diff
  is handoff-only; the worker's four commits (`e199507`, `9aceb4d`, `3d04606`,
  `127d700`) each touch only `harness/handoffs/T-106.md`.

## Findings

1. **[Medium, accuracy/robustness — not a test-integrity failure] The handoff's
   central new claim, "the recurring gate failure is fixed", is already false
   at review time.** The handoff says `HEAD..main` is empty and the gate's
   `merge --ff-only task/T-106` will succeed (diff `:38-40`). That was true for
   only ~25 seconds: `127d700` was committed at 14:31:43Z, and main gained
   `8e68e81` ("chore(harness): update task graph, progress and review
   artifacts") at 14:32:08Z (`git show -s --format='%ci' 8e68e81`). Current
   state is `git log --oneline task/T-106..main` = `8e68e81`, so main is no
   longer an ancestor of the branch and a bare ff-only merge refuses. The
   gate does rebase first (`cmd/harness/gate.go:47`, :88), so the branch can be
   re-integrated; but the gate's rebase happens before `make check`
   (`gate.go:100-117`) and the ff-only merge (`gate.go:130`), which is exactly
   the window in which the scheduler advanced main during attempt 3
   (`172a8df` 14:24:24Z, `3cfcadd` 14:25:15Z, `6040d7c` 14:25:25Z around the
   14:25:05Z `gate_fail`). A worker-side pre-rebase is therefore not a durable
   fix for the race. The operator should close T-106 as already remediated on
   main (`30be90d`) rather than rely on another gate cycle; the handoff's
   "fixed" wording overstates the result even though its `BLOCKED` disposition
   remains correct.

2. **[Low, documentation precision] "Branch-vs-main diff is exactly one file"
   is true only for the merge-base diff.** `git diff --name-status main
   task/T-106` currently reports three paths: `A harness/handoffs/T-106.md`,
   `M harness/reviews/T-106.diff`, `M harness/tasks/graph.json` (the latter two
   are older branch-side scheduler state versus main's newer versions). The
   handoff's command row uses `git diff --stat main...HEAD`, which correctly
   gives one file (diff `:62`), but the prose at `:39-40` drops the three-dot
   qualifier. This matters if an operator ever merges the branch literally
   (rather than through the gate's rebase + `finalizeMerge` graph restore):
   main's newer review artifact and graph status would be replaced by stale
   branch copies. `finalizeMerge` restores `graph.json` from main
   (`cmd/harness/gate.go:122-128`) but not the review artifact.

3. **[Low, documentation precision] "the rebase only moved already-committed
   history" understates a dropped commit.** The same rebase auto-skipped
   `c095e38` (diff `:30-37`), so it did not merely move history. The handoff
   does disclose the skip and the graph-only content, and the practical outcome
   is safe: `finalizeMerge` re-creates main's authoritative graph state
   (`cmd/harness/gate.go:122-128`), and the branch graph now differs from main
   only by `status: "running"` vs `"review"` (same attempts/model). But the
   blanket "no edits outside ... the rebase only moved history" sentence at
   `:96-104` is literally inaccurate. The additional claim that `c095e38`'s
   "patch already exists on main" is also imprecise: main at the rebase base
   (`f968619`) had T-106 `status: "running", attempts: 3`, not the commit's
   `review -> merging, attempts: 2`; git skipped it via patch-id equivalence
   with an earlier main-history commit, not because the current main tree
   contains that exact hunk. No content regression results because the gate
   restores the graph, but the rationale should not be read as proof.

4. **[Info, harness policy] The allowlist bypass is real and is now
   normalized.** `worker.md:39` unconditionally allows `git -C *`, so
   `git -C . rebase main` executes even though bare `git rebase` is denied by
   the default-deny table (the attempt-4 transcript shows bare `git reset`,
   `git rebase`, and `git merge` all rejected, then `git -C . rebase main`
   succeeding, `worker.jsonl:38-46,74`). The handoff openly documents this and
   recommends the `git -C .` form for future retries (diff `:91-94`). That is
   a harness permission-model ambiguity, not a test-integrity violation, but
   the operator should either explicitly sanction topology repair via `git -C`
   or tighten the wildcard.

5. **[Info] A zero-width space sits in the handoff's allowlist sentence**
   (`harness/reviews/T-106.diff:24`, between `(` and `.opencode`). It is
   harmless for the handoff content but makes copy/paste of that path
   unreliable.

6. **[Info, carried] `AGENTS.md` flag drift is accurate:** AGENTS.md says
   `make e2e-task T=<id>`, while the Makefile variable is `TASK=`. Doc-side
   fix, outside this task's owns.

## What I did not find

No weakened, deleted, skipped, or narrowed tests; no test-only shortcuts; no
mocks substituting for the behaviour under test; no coverage theatre; no dead
code to satisfy a linter; no production code or fixture special-casing. The
branch's net effect is a single BLOCKED handoff document, which is the packet's
prescribed outcome for an already-remediated premise.

## Conclusion

The deliverable itself is already on main (`30be90d`), the `BLOCKED` disposition
is exactly what the packet's Never clause requires, and the diff touches no
tests or code. The attempt-4 handoff's claim that the gate failure is durably
fixed is contradicted by main's `8e68e81` 25 seconds after the handoff commit;
that is a harness race and a documentation overclaim, not a test-integrity or
reward-hacking failure. The operator action remains: close T-106 as already
done on main and stop cycling the branch through the gate.

VERDICT: PASS
