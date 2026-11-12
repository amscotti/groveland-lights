# Harness state

**Phase 0 done** (bootstrap + selftest green; CI blocked on GitHub billing —
BLOCKED-H1b; Fly untested — BLOCKED-H4 human). Skeleton serves `/healthz`
(`ok`) and `/` (Pico + htmx 2.0.11 + Alpine CSP 3.17.5 vendored, §3.4 CSP,
slog, graceful shutdown, config typed). `make check` green locally: 29 Go
tests, 12 Playwright specs over 4 projects, axe clean, image 29.25 MB.
Contract has A-000 only; graph empty; ratchet baseline go=28/pw=12.

## Phase 1 (M1) planned — planner output 2026-11-12

Goal: skeleton deployed on Fly with a volume; a SQLite-backed probe counter
proves persistence across restart and deploys. Contract delta A-101..A-105
(see planner report; scheduler applies to `acceptance/contract.json` and
`harness/tasks/graph.json`).

Tasks (packets in `harness/tasks/T-10*.md`):
- T-100 acceptance: green guard specs (A-101/A-102/A-105) + INERT DRAFTS of
  A-103/A-104 specs (`acceptance/drafts/*.txt`, holdout `drafts/`).
- T-101 ‖ store: `modernc.org/sqlite` (v1.62.0), migrations/0001 (probe table
  ONLY — product schema is Phase 2's 0002), runner + schema_migrations
  checksum refusal; sqlc queries committed. allow_deps incl. transitives.
- T-102 ‖ web: CSRF double-submit (`IssueCSRFToken`/`VerifyCSRF`/`ErrCSRF`)
  + middleware logging guard tests. No `web.go` edits.
- T-104 ‖ infra: Dockerfile entrypoint (su-exec chown /data), fly.toml [env]
  (DB_PATH=/data/lights.db…), scripts/smoke.sh (deployed persistence
  evidence). Claims none; docker/fly NOT runnable by workers — CI/human prove.
- T-103 serial (after 100,101,102): main.go opens store (boot refusal on
  checksum mismatch), `probe.SetStore`, GET/POST /probe per frozen UI contract
  (section#probe, h2 "Probe", #probe-count "Count: N", button "Increment
  counter", hidden csrf_token; 403 on bad token; non-HTMX POST → 303 /),
  home mount in web/templates/**. ui:false (visual check at validation).
- T-105 acceptance (last): activates drafts BY RENAME; green required;
  feature-vs-test disputes → BLOCKED to planner.

## Why drafts + T-105 (structural finding)

The gate runs full `make check` per task, so red specs can never merge; yet
contract-first requires acceptance before features. Resolution: specs for
not-yet-buildable assertions are committed as inert drafts (no runner picks
them up) and activated verbatim after the features land — oracle-first stays
auditable in git history. Deviations from the plan's Phase-1 table: T-105
added; owns extended (`go.mod,go.sum`→T-101; `web/templates/**`→T-103;
`scripts/docker-entrypoint.sh`→T-104; acceptance owns include
`harness/protected.sha256`); `testdata/schema.golden.sql` deferred (protected
glob blocks feature tasks from creating it — Phase 2 must route it through
its acceptance task).

## REQUIRED operator pre-steps before `make run`

1. `.gitignore` lacks the e2e runtime artifacts; any agent running Playwright
   would leave an untracked `test-run.db` → guard exit 9 with no way to clean
   it (no rm/clean permission). Fix on main and commit:
   `printf 'test-run.db\ntest-run-photos/\n' >> .gitignore`
2. H4 (Fly app/volume/secrets) before the deployed DoD; `fly launch` picks up
   fly.toml; volume `data` ewr 1 GB `--snapshot-retention 14`.

## Run notes (M1 close-out)

- Order the scheduler will produce: T-100 → {T-101,T-102,T-104} (max_parallel
  2) → T-103 → T-105.
- After T-105: `make build` on main (bin/ is gitignored; validator's
  Playwright webServer needs bin/server), then validator
  `make validate MILESTONE=M1` + `make holdout`, visual-reviewer on the
  validator screenshots, judge with both deploys' `scripts/smoke.sh` outputs
  as persistence evidence.
- Known risks: modernc transitive deps beyond allow_deps (worker BLOCKS —
  planner extends list); docker path unverifiable until CI billing is fixed;
  `T-102`'s A-102/A-105 specs are green-on-arrival guards by design.

## Next phase preview

Phase 2 (M2, admin walk-the-town) per plan §7: T-200 acceptance first;
0002 migration owns the product schema (incl. users/sessions for T-203);
`web.go` Server-field refactor (store onto Server) lands there.
