# T-106 review — BLOCKED disposition on "already untracked on main"

Reviewed artifact: `harness/reviews/T-106.diff` (the only change is the new
handoff `harness/handoffs/T-106.md`; no other file touched). Independently
re-verified against the main checkout at HEAD `83778e2`, which shares history
with the task worktree base `4d6f347` (worktree base is an ancestor; app code
byte-identical, see F6).

## Packet-conformance (harness/tasks/T-106.md)

- Never clause (T-106.md:48-50) prescribes exactly this route when the files
  are not tracked: first line `BLOCKED: already untracked on main`, then stop.
  Handoff line 1 is that exact string; the deliverable commit was correctly
  NOT created — packet step 1 (`git rm --cached …`, T-106.md:28) is a no-op git
  rejects with exit 128, and step 2's commit (T-106.md:31) would be empty.
- Step 3 respected: `.gitignore` untouched (diff contains no `.gitignore`
  hunk; on-disk `.gitignore:20-21` = `test-run.db*` / `test-run-photos/`,
  matching the handoff's exact line citations).
- Owns respected: no file outside the three owned paths (which were not edited
  — they exist in neither index nor disk) plus the handoff. Nothing in
  `acceptance/`, no deps added (`allow_deps: []`), no migrations, no goldens.
  Hard rules 1, 2, 3, 8, 10 all compliant.

## Independent re-verification of the BLOCKED condition (it holds)

- All three owned paths are untracked: `git log --oneline -- test-run.db` (and
  `-shm`/`-wal`) shows exactly `30be90d` ("chore: untrack e2e runtime
  artifacts") then `7f07df2` (the original accidental add) — nothing after.
  `30be90d` (2026-11-13 07:12:53) is an ancestor of both the worktree base
  `4d6f347` and main HEAD `83778e2`. `git log --oneline -- '*test-run*'`
  confirms no other tracked test-run path anywhere in history.
- `git status --porcelain` on main is EMPTY — no `D` entries, consistent with
  the handoff's third proof.
- The remediated finding is real and matches: harness/findings/judge-M1.md:93-95
  ("operator commit `7f07df2` committed `test-run.db{,-shm,-wal}` … `.gitignore`
  fix landed later in `f901627` but the tracked blobs remain"), routed to T-106
  in harness/state.md:7-8,20. `30be90d` closed it before this worktree was cut.
- `?? node_modules` tolerance: correct — `cmd/harness/ratchet.go:127-129`
  (DirtyLines) exempts `node_modules`, `.env.test`, `.merge.lock`,
  `harness/handoffs/**`, `harness/runs/**`; and `scripts/worktree-new.sh:23-24`
  creates the worktree's `node_modules` as a `ln -s` symlink, which
  `.gitignore:2` (`node_modules/`, directory-only pattern) cannot cover. The
  handoff's "pre-existing symlink" explanation (open issue 2) is precise.
- `make e2e-task TASK=T-106` → exit 0 "no claims for T-106": consistent with
  Makefile:112-116 (empty grep ids → echo + `exit 0` before any `npx`
  invocation, so it cannot mint `test-run.db*` dirt) and graph.json T-106
  `"claims": []` (harness/tasks/graph.json:237). AGENTS.md flag drift
  (`T=<id>` vs Makefile:113's `TASK=`) — open issue 3 confirmed against the
  Makefile's own usage string.

## Security / architecture

No code, templates, handlers, SQL or static assets changed — CSRF, XSS via
template.HTML, SQLi, path traversal, upload limits, secrets-in-output are all
N/A for this diff. The handoff emits no secret material (`.env.test` is named
only, never shown). Architecture rules (HTMX/#map, parse-don't-validate, typed
IDs, status.Derive) untouched. No "Phase N" strings introduced into app code
(handoff lives in `harness/` anyway). Test rules: `type: chore`, `claims: []`,
no UI — no new tests required; nothing was weakened.

## Findings

- **F1 (low, prose accuracy):** The handoff attributes the `.gitignore`
  coverage to `30be90d` — "removed … from the index and landed the `.gitignore`
  coverage" and "was itself part of the `30be90d` fix" (Confirmation section of
  T-106.diff). Incorrect: `git log --oneline -- .gitignore` shows the coverage
  landed in `f901627` (06:46:18, "build: ignore e2e runtime artifacts") and
  `30be90d` never touched `.gitignore`. Judge-M1.md:94 and the packet itself
  (T-106.md:23-24) both credit `f901627`. The error does not affect the BLOCKED
  conclusion (the paths are independently proven untracked, and the
  "ignored" evidence — `.gitignore:20` — is correct on its own).
- **F2 (informational):** "pre-registered as operator pre-step #1 in
  `harness/state.md` per the T-105 handoff" — current state.md (rewritten at
  `4d6f347`) no longer carries that pre-step text; it survives only at
  harness/handoffs/T-105.md:292-299. Accurate only as cited "per the T-105
  handoff"; the direct state.md pointer is dead.
- **F3 (informational):** `make check-fast` and `make e2e-task` exit-0 claims
  could not be re-executed in this review session (`make` denied by the review
  shell allowlist — mirroring the handoff's own note 4). Verified statically
  instead: see F6; the e2e-task path is proven from Makefile:112-116 +
  graph.json:237.
- **F4 (no action):** Handoff open issue 2 (`node_modules` symlink cosmetics)
  is real and correctly left outside the task's owns; durable fix is an
  operator `.gitignore` tweak or dropping the symlink in worktree-new.sh.
- **F5 (no action):** Handoff open issue 3 (AGENTS.md `T=` vs Makefile `TASK=`)
  confirmed; doc-side fix belongs to an operator/docs task.
- **F6 (evidence):** `git log --oneline 4d6f347..HEAD -- cmd internal web
  migrations` is EMPTY — zero app-code delta between the worktree base and
  main HEAD, so the packet's "make check-fast still passes (nothing changed in
  code)" verify (T-106.md:39) holds by construction given the previously
  recorded green runs at these commits (T-105 attempt-6 handoff,
  harness/handoffs/T-105.md:205).

## Verdict

The BLOCKED route is the packet-prescribed correct behaviour for the actual
repo state, the worker stopped exactly where told, the diff is minimal and
clean, and every substantive claim re-verified (two prose-level inaccuracies,
F1/F2, neither load-bearing). No security, architecture or test concerns.

VERDICT: PASS
