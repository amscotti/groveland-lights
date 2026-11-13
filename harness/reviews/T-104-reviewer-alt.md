# T-104 — reviewer-alt (test integrity / reward hacking)

Scope: attached `harness/reviews/T-104.diff` and the current worktree, read
against `harness/tasks/T-104.md`, `harness/tasks/T-103.md`,
`harness/tasks/graph.json`, `Makefile`, and the proposed files. Shell execution
is permission-denied in this review environment, so I traced the artifacts
instead of re-running the commands. The diff adds exactly one file,
`harness/handoffs/T-104.md` (`T-104.diff:1-6`).

## Blocking: the diff delivers none of the task

- All five owned paths are absent or unchanged. Current worktree:
  `Dockerfile:14,17` still has `USER appuser` and
  `ENTRYPOINT ["/app/server"]`; `fly.toml:6-7` has `[build]` with no `[env]`;
  `.dockerignore:13` stops at `harness/runs`; `scripts/smoke.sh` and
  `scripts/docker-entrypoint.sh` do not exist (`scripts/*` contains only the
  pre-existing five scripts). The handoff itself says "The task owns five files;
  none could be created or edited" (`T-104.diff:18-19`) and "Nothing on disk
  outside this handoff" (`T-104.diff:24`).
- A handoff describing code is not a deliverable. The proposed
  Dockerfile/entrypoint/fly/.dockerignore/smoke blocks (`T-104.diff:34-310`)
  are not in the tree, so they are proposals rather than mergeable code; they
  cannot be built, linted, or run. The guard has no change to the owned paths
  and a later validator has nothing to deploy. "Ready to apply verbatim"
  (`T-104.diff:19-20`) is an instruction to a human, not a completed task.

## Blocking: the two green commands cannot fail for T-104

- `T-104.diff:316-317` reports `make e2e-task TASK=T-104` and `make
  check-fast` as verification. Both are vacuous:
  - `make e2e-task` reads `claims` from `harness/tasks/graph.json`; T-104 has
    `"claims": []` (`harness/tasks/graph.json:164`;
    `harness/tasks/T-104.md:12`), so the target prints "no claims for T-104"
    and exits 0 by construction (`Makefile:112-116`). It exercises nothing.
  - `make check-fast` runs fmt/vet/lint/Go unit tests; the packet itself says
    it is "green trivially — you touch no Go"
    (`harness/tasks/T-104.md:30-31`). The new files are shell/Docker/TOML, none
    of which check-fast covers. The handoff admits "Because no deliverable
    landed, the checks above are the untouched baseline" (`T-104.diff:321`).
    Presenting them as verification is coverage theatre: no assertion exists
    that can fail if `smoke.sh` or the entrypoint is wrong.
- The only meaningful checks are explicitly deferred: `bash -n`,
  `shellcheck`, `docker build`, `docker run`, `flyctl deploy`, and the smoke
  run are all in "Machine-unverified paths" (`T-104.diff:324-345`). The CI
  fallback is not live either: `harness/state.md:3,68` records "CI blocked on
  GitHub billing — BLOCKED-H1b". So "CI runs this via make ci"
  (`T-104.diff:333`) is not a current verification path.

## Blocking: the proposed smoke assertion is weaker than the frozen contract

- If the code block were applied, the valid POST at `T-104.diff:273-278` sends
  `HX-Request: true` but accepts both `200` and `303`. T-103's frozen contract
  says a valid HTMX POST returns 200 and only a no-JS POST returns 303
  (`harness/tasks/T-103.md:55-59`). Accepting 303 on an HTMX request lets a
  regression in HTMX detection pass silently. The task packet's step 4 also
  says "accept either" (`harness/tasks/T-104.md:103-104`); that leniency is the
  weakness, but the smoke script is the sole evidence collector and should not
  accept a status the contract forbids for its request shape.
- The script's parsing/date logic is the part most likely to fail and has no
  static or unit check: `extract_csrf`/`extract_count`
  (`T-104.diff:193-211`) and `epoch_of` (`T-104.diff:222-229`). The handoff's
  "No unit tests added" (`T-104.diff:357-360`) defers all logic to a deployed
  run that cannot happen until after merge and T-103 deploy. A test that can
  only be validated after merge is not evidence at review time.
- `--smoke-suite` (`T-104.diff:298-307`) invokes `make smoke`, whose target
  uses `--pass-with-no-tests` (`Makefile:194-197`); it can print
  `smoke: suite OK` with zero matched specs and does not assert a minimum
  count. Secondary, but another check that cannot fail.

## Process findings

- The block marker is not the first line: `T-104.diff:1` is `# T-104 — ...`;
  `BLOCKED:` starts at `T-104.diff:9`. AGENTS.md requires
  `BLOCKED: <reason>` as the first line.
- The block claim is unverifiable from the artifact: no tool logs, no partial
  files, and the diff contains no failed-write evidence. Even if the sandbox
  boundary is real, the remedy is to re-run with write access, not to accept a
  handoff as completion.
- The "apply verbatim" blocks are an end-run around the denied writes: if a
  human pastes them, the task can be marked done without the guard ever seeing
  a committed diff to the five owned paths. That is the reward-hacking shape to
  reject here.

## Checked and not found

- No weakened/deleted/skipped/narrowed test, no
  `t.Skip`/`test.skip`/`test.only`/`.fixme`, no fixture special-casing, no mock
  bypass, no protected-path edit, no new dependency, no `console.log`, no
  TODO/FIXME in the diff — because the diff contains no production or test code
  at all. The omission is the failure.
- Positive verification-by-reading only: the proposed env names match
  `internal/config/config.go` (`PORT`, `APP_ENV`, `BASE_URL`, `DB_PATH`,
  `PHOTO_DIR`, `TOWN_BBOX`, `TZ`; `internal/config/config.go:75-110`), and the
  proposed `.dockerignore` preserves the existing entries while adding
  `test-run.db`, `test-run-photos/`, `selftest*`, and `harness`
  (`T-104.diff:118-134`; current `.dockerignore:1-13`). Those blocks are not in
  the tree.
- Secondary, outside T-104's owns: the handoff says the secret names are
  "consumed by `config.secret`" (`T-104.diff:110-112`), but `config.Load` does
  not add the three secrets to `missing` and `secret()` returns `(nil, nil)`
  when unset (`internal/config/config.go:91-102,112-126`). A deploy that
  forgets `fly secrets set` can boot with nil keys; the fly `[env]` block
  correctly omits secrets, but the deploy runbook should make the failure mode
  explicit.

VERDICT: BLOCK
