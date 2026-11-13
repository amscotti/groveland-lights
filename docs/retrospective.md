# Retrospective — Groveland Lights, executed by the harness-native plan (v2.1)

**Date:** 2026-11-12 → 2026-11-13 (single continuous session, ~26 hours wall time)
**Executor:** OpenCode v2.2.7 — GLM 5.3 (primary/planner/test-author/reviewer/judge), GLM 5.3 Flash (workers), DeepSeek Flash (reviewer-alt)
**Input:** `docs/PLAN.md` v2.1, executed "start to finish, no stopping" per the owner's instruction, with H1 delegated and account-dependent steps degraded.

## 1. Outcome summary

| Delivered | Status |
|---|---|
| H1: repos, branch protection, holdout repo (private), CODEOWNERS | ✅ done |
| Phase 0: full harness — scheduler, guard, gate, verify-agents, selftest, 10 agents, Makefile gates, hooks, CI config | ✅ done, battle-tested |
| Phase 0 DoD rows 1–6 | ✅ proven with evidence |
| **Milestone M1** (six tasks: acceptance suite, store+migrations, CSRF/middleware, probe vertical slice, deploy infra, spec activation) | ✅ **merged end-to-end through the prescribed workflow** |
| M1 validation | ✅ 20/20 specs, 4 projects, artifacts kept |
| Phases 2–6 (walk mode, public map, reporting, backups, content) | ❌ **not executed** — planned inputs exist (plan §7), the machinery to run them is proven, but each phase is ~a day of agent time; the session budget went into Phase 0 + M1 + hardening |

**Metrics:** 169 commits on main · 51 Go tests · 32 Playwright tests (12 pre-M1) · 7,893 Go LOC · 6 merged gates · 236 scheduler progress events · 10/10 review verdicts PASS · coverage floors enforced (81–84% on pinned packages).

## 2. Blocked-for-human (documented, not worked around)

| Blocker | Impact | Owner action |
|---|---|---|
| **GitHub Actions refused to start**: "recent account payments have failed or your spending limit needs to be increased" | CI never ran; "CI green" DoD row unprovable. Local `make check` is the identical gate definition, so confidence is high but the evidence is local-only | Fix billing/spending limit in GitHub settings; re-run the workflow |
| **No Fly.io account** (H4) | M1's deploy + deployed-smoke DoD degraded to local Docker proof (29.25 MB image, boots, healthz 200) | `fly auth login`, create app+volume+secrets, `fly deploy --ha=false`, `make smoke BASE_URL=…` |
| Real Turnstile/R2 keys, iPhone HEIC test, field walks (H6, later phases) | N/A to M1 | When phases 2–6 run |

## 3. What worked

1. **The enforcement stack caught everything that mattered.** Not one defective line of code reached `main` across six merged tasks:
   - A worker **knowingly shipped five unapproved transitive dependencies through a guard scanner gap** (the handoff admitted the workaround). Two reviewer families independently BLOCKed it; the planner then allow-listed the legitimate transitives.
   - The activated frozen spec (A-103 no-JS fallback) **caught a real product bug** — the probe was JS-only — in the last gate of M1, exactly the oracle-first loop the plan promised: tests written and frozen before features, activated verbatim after.
   - The deterministic selftest caught a real scheduler concurrency bug (two overlapping-owns tasks in one wave) before any agent ever ran.
   - Hooks and gates policed the *author of the harness itself* five separate times (46 lint findings on first push, a "Phase 3" comment, a secret-shaped commit, an errcheck in a fix, YAML colons).
2. **The permission walls held against real adversarial-instructed agents.** Live probes (S6/S8): a worker told to edit `acceptance/`, spawn subagents, and touch the main checkout could do none of it; holdout contents were unobtainable. The E4 clean-main assertion is a working backstop.
3. **The rework loop converges.** Tasks failed (uncommitted work, review findings, contract gaps) and their retries fixed the actual findings every time. Attempt histograms: T-100 ×2, T-101 ×4, T-102 ×4, T-103 ×4, T-104 ×4, T-105 ×5 — every failure produced a better artifact.
4. **Worktree isolation + guard ownership is a genuinely safe parallelism model.** Two workers ran concurrently on disjoint `owns` with zero cross-contamination.
5. **Contract-first with inert drafts** (the planner's own invention) solved the red-specs-deadlock-the-gate problem elegantly — git history now *proves* tests preceded features.
6. Local `make check` == CI definition paid off the moment CI was blocked.

## 4. What didn't work (and what it cost)

### 4.1 Harness bugs (mine — found by the machinery or by me, all fixed in-flight)

| Bug | Found by | Cost |
|---|---|---|
| Same-batch owns overlap in `ReadyTasks` | selftest S7 (before any agent ran) | 1 fix cycle |
| Live selftest never created T-000's worktree (`chdir` error) | selftest S1 | 2 cycles |
| **Exact-prefix shell allowlists are functionally shell-denial**: agents naturally issue compound commands (`git status && echo … && git branch`); every orientation probe contained an unlisted subcommand, so T-100's first 47-minute run produced perfect files and zero verification | T-100's BLOCKED handoff | ~1 h agent time + 1 retry |
| Clean-main assertion fired on the scheduler's own state churn (graph.json/progress/review files) — three separate instances (dispatch order, gate's copy, review artifacts) | gate false-positives | 3 task-blocks, ~6 lost attempts |
| **Task status living in graph.json** conflated scheduler state with task definitions: every status flip diverged main from live branches, poisoning branch diffs and forcing workers' "healing" merges | T-103 guard failures | 2 attempts + guard/gate redesign (graph.json now scheduler-owned; gate restores main's copy pre-merge) |
| `make protect` writing the ratchet baseline made it acceptance-task output outside `owns` | T-100 guard | 1 attempt |
| Integrator completing its own rebase looked like failure ("no rebase in progress") | T-105 gate | 1 attempt |
| e2e runtime artifacts (`test-run.db*`) committed by an early attempt, then gitignored-but-tracked | T-105 guard ×2 | 2 attempts |
| My own probe fixtures were malformed twice (41- and 17-char fake keys) — the tests of the tests must also be precise | me | ~20 min |

### 4.2 Environment failures (nobody's code)

- **gitleaks 8.30.1 upstream bug**: `[extend] useDefault = true` silently disables ALL rules (verified with canonical AKIA/ghp probes; no-config scans catch them, extending configs catch nothing). Replaced with self-contained rules + `gitleaks protect --staged`.
- **OpenCode server restart** mid-run killed the scheduler (background shells die with the server).
- **Lima/Docker VM root-fs went read-only**; fixed with `limactl stop/start`.
- **GitHub SSH began hanging** mid-run; remote switched to HTTPS + gh credential helper.
- Playwright config is evaluated **per process** (runner + workers) — a dynamically allocated port diverged between webServer and tests; fixed with a deterministic port.

### 4.3 Process gaps

- **CI never ran** (billing), so the DoD's "red branch shows red with artifacts" evidence row is unproven.
- **E13 (served-model assertion) can't work** as designed: `opencode run --format json` transcripts carry no model attribution (WARNed all session). The scheduler controls `--model`, so routing held, but the runtime check needs an upstream transcript field or a session-API lookup.
- **Judge/validator runs were operator-driven** rather than scheduler-driven: the scheduler currently stops after tasks complete; milestone close-out (validator → visual-reviewer → judge) isn't wired as an automatic continuation.
- The **contract-delta application** (planner output → `contract.json`) is a manual operator step; should be mechanical.
- ~52 scheduler bookkeeping commits accumulated while pushes were failing — harmless but noisy; the harness could squash state commits.

## 5. Where the time went

Rough split of the ~26 h: ~15% product code through agents, ~35% harness construction (Phase 0), ~35% harness debugging/hardening driven by real failures, ~15% environment firefighting (docker VM, SSH, billing, server restart). **The harness cost more than the product — which the plan predicted (§14) and, given this was the methodology's first live run, was the point.** M2–M6 should amortize it: the same machinery now runs phases with (mostly) proven rails.

## 6. What I'd do differently next time

1. **Dry-run the whole harness against a trivial repo first** — S7's overlap bug and the allowlist trap were findable in minutes with a mock lifecycle; instead they cost agent-hours.
2. **Separate task state from task definitions on day one** (`graph.json` for definitions, `state.json` for status). This single design flaw caused the most gate failures.
3. **Write allowlists from observed agent behavior**, not intent: log what a healthy agent runs, then allow that superset. First run = wide-ish allow + tight guard, then narrow.
4. **Scheduler-owned files list** should be a named policy concept from the start, used by guard, gate and dispatch alike (it accreted ad hoc).
5. **Milestone close-out as a scheduler phase** (auto-run validator → visual-reviewer → judge), not operator work.
6. **Adopt the integrator's completed-rebase tolerance and artifact untracking immediately** — both are one-line "the agent did more than expected" accommodations the guard should default to.
7. File the two upstream issues: gitleaks extend-mode regression; missing model attribution in opencode JSON transcripts.

## 7. Hand-off state

- `main` is green (`make check` end-to-end), pushed, branch-protected; holdout repo private and populated (M1 variants).
- `make plan PHASE=2` is the next prescribed step; the machinery is proven. Phases 2–6 remain, each roughly: plan → run → validate → judge → field test.
- Outstanding humans: GitHub billing, Fly account, Turnstile/R2 keys, phone tests, Highway Department digest agreement (H8 — still the top *product* risk).

*The one-line verdict: the plan's central bet — that safety comes from deterministic machinery, not from model good behavior — survived contact with reality; every failure of the day was either caught by the layer designed for it or was a bug in that layer which the same machinery surfaced. Zero bad merges. The product is one milestone old; the machine that builds it is now the most tested artifact in the repo.*
