# Retrospective — Groveland Lights, executed start-to-finish by the harness-native plan (v2.1)

**Date:** 2026-11-12
**Executor:** OpenCode v2.2.7, GLM 5.3 (planning/primary), GLM 5.3 Flash + DeepSeek Flash (harness agents)
**Input:** docs/PLAN.md v2.1 (the reviewed plan), executed in one uninterrupted session per the owner's "go, no stopping" instruction.

> **Status: IN PROGRESS** — this document is finalized when the session's work ends.

## 1. What was attempted

Execute the full plan: H1 (repos + protection), Phase 0 (harness bootstrap to DoD),
then Phases 1–6 through the prescribed workflow (`make plan` → `make run`, guard/gate,
reviews, validation), with every external-account step either delegated (H1 via `gh`)
or blocked-for-human (Fly, Turnstile/R2 production keys, iPhone HEIC test, field walks,
GitHub Actions billing).

## 2. Running log

| Time (EST) | Event |
|---|---|
| 11:33 | H1 delegated: `groveland-lights` (public) + `groveland-lights-holdout` (private) created; holdout cloned as sibling |
| 11:36 | Plan moved to docs/PLAN.md, first commit, pushed |
| 11:37–11:42 | mise.toml/package.json/go.mod; mise trust needed (first-run friction); node/go toolchains installed in background while harness code was written |
| 11:42–11:50 | All 10 agent files, policy.json (resolved model IDs), scripts, harness CLI (~2.3k LoC incl. tests), server skeleton, acceptance scaffold, all quality configs |
| 11:50 | First build+test pass: 6 compile errors (2-value returns), fixed; unit tests green |
| 11:52 | First selftest run: 5/6 → S7 caught a REAL scheduler bug (two overlapping-owns tasks scheduled in the same batch). Fixed in ReadyTasks |
| 11:54 | 6/6 deterministic scenarios pass; verify-agents: 10/10 conform |
| 11:56 | `make protect` (manifest + ratchet baseline: go=28, pw=12) |
| 11:58 | Pre-push hook refused my own push: lint found 46 issues (cyclop, errcheck, gosec, noctx, unused). Fixed all 46 properly (refactors, context, documented gosec exclusion for cmd/harness). The harness enforced its rules on its author — as designed |
| 12:01–12:05 | Full `make check` iterations: cover path bug; html-validate (DOCTYPE, template whitespace); tsc (NodeNext extensions, DOM types, `@types/node`); e2e failures → root-caused THREE real issues: (1) IPv6/IPv4 localhost mismatch, (2) playwright config evaluated per-process so dynamic ports diverged between webServer and tests, (3) htmx injecting inline indicator styles violating CSP + stale embedded-template binary masking the fix. All fixed properly |
| 12:05 | `make check` fully green: 29 Go tests (race+shuffle), 12/12 Playwright across 4 projects, axe zero violations, coverage floors passed (81%/84% vs 70%), govulncheck clean, gitleaks clean (after allowlisting documented dummy test values), phase-grep caught my own "Phase 3+" comment in CSS — fixed |
| 12:06 | Pushed with all hooks green |
| 12:06 | CI: 2 consecutive failures — (1) YAML colon in a step name, fixed; (2) **BLOCKED-H1b: GitHub Actions refuses to start: "recent account payments have failed or your spending limit needs to be increased"** — human-only |
| 12:08 | Branch protection initially blocked (private repo needs Pro) → repo made public per plan's allowance, protection applied (code-owner reviews on PRs, no force push) |
| 12:10 | Live selftest #1: 6/6 deterministic; **S6 permission probes PASS against a real worker agent** (acceptance/ untouchable, subagent denied, denial events in transcript); **S8 holdout PASS**; S1 FAIL (selftest harness bug: worktree never created before lifecycle) |
| 12:12 | Live selftest #2: S6 flagged main dirty — correctly: my own uncommitted edit. Operator error caught by the machine. S1 bug fixed (worktreeFor added) |
| 12:15 | Planner agent (GLM 5.3) dispatched for Phase 1 via `make plan PHASE=1` |
| 12:15 | Docker daemon recovered (Lima VM `berth-docker` root-fs went read-only; `limactl stop/start` fixed it). Image builds: **29.25 MB** (target ≤40), boots, healthz green in-container |
| 12:16 | Planner #1 killed in error by the operator: it showed 0% CPU but its transcript proved active work (55k+ cached tokens, tool calls). Lesson: agent runs are network-bound; CPU is not liveness. Planner relaunched |
| 12:19 | DoD row 6 deep-dive found a REAL upstream bug: **gitleaks 8.30.1's `[extend] useDefault=true` detects nothing** (extended configs silently lose all rules — verified with canonical AKIA/ghp probes; no-config scans catch them). Replaced with a self-contained rules file + `gitleaks protect --staged` in the hook; verified: staged-key commit refused end-to-end, repo scan clean. Also: my probe keys were twice malformed (41/17 chars) — fixture precision matters even in tests |
| 12:19 | Push 3acb6ce: GitHub notes "Bypassed rule violations: changes must be made through a pull request" — branch protection active, owner/gate pushes allowed by design |
| — | *(continues)*

## 3. What worked (preliminary)

- The deterministic selftest caught a real concurrency bug in the scheduler before any
  agent ever ran (S7).
- The pre-push hook and lint gate held the *author's* feet to the fire — 46 findings,
  a refused push, and a "Phase 3" comment rejection, all correct.
- Permission walls held against a **real adversarial-instructed worker**: S6 and S8
  passed live. Deny-all baselines + trailing denies work as the plan predicted, and the
  clean-main assertion is the backstop, not the first line.
- `make check` proved identical locally to what CI would run — which mattered exactly
  when CI was blocked by billing.

## 4. What didn't (preliminary)

- CI cannot run at all: account billing/spending limit (BLOCKED-H1b). Local-only proof
  for the "CI green" DoD row.
- The live happy path needed two selftest-harness fixes before the *product* path was
  ever exercised — the selftest itself was the weakest link, not the agents.
- Fly deploy path untested end-to-end (BLOCKED-H4: no account) — degraded to local
  Docker build + boot + healthz smoke.

*(Final sections: outcome per phase, agent-vs-operator work split, metrics, and
recommendations — completed at session end.)*
