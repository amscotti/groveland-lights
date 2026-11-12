# Groveland Lights — agent rules

Public map + reporting app for the town's park/building light fixtures (60–150 to start;
street lights later). Go stdlib + SQLite + htmx + Alpine (CSP build) + MapLibre. One admin
(Anthony). See `docs/spec/` for detail; `docs/PLAN.md` is the planner's input only.

## Repo map

- `cmd/server` — the app binary. `cmd/harness` — scheduler/guard/gate (protected).
- `internal/{config,store,web,img,geo,status,jobs}` — app code.
- `web/templates`, `web/static/{css,js,vendor}` — UI. Vendor files carry versions in names.
- `migrations/` — append-only SQL, one file per change. Never edit a merged migration.
- `acceptance/` — PROTECTED contract tests. `harness/` — PROTECTED orchestration state.
- `testdata/` — fixtures. `*.golden*` files are protected.
- `scripts/` — worktree + ops scripts. `docs/` — specs, ADRs, runbook.

## Commands

- `make check-fast` — fmt-check, vet, lint, unit tests. Run constantly.
- `make e2e-task T=<id>` — only the Playwright specs for your task's claims.
- `make check` — everything. The gate runs this; don't hand it anything red.

## Hard rules

1. Never edit `acceptance/`, `harness/` (except your handoff file), `.opencode/`, the
   `Makefile`, CI, `.golangci.yml`, or golden files. If a spec seems wrong, write
   `BLOCKED: <reason>` in your handoff and stop — never edit the spec.
2. Never weaken, skip, delete or special-case a test. No `t.Skip`, `test.skip`,
   `test.only`, `.fixme`. New code needs new tests.
3. Never add a dependency not listed in your task packet's `allow_deps`.
4. HTMX never swaps `#map` or its ancestors. The map is initialised once.
5. No "Phase N" strings in code under `cmd/server`, `internal`, `web`, `migrations`.
6. Parse inputs at the boundary into typed structs; handlers never read `r.FormValue`
   after parsing. Typed IDs (`LightID`, `ReportID`) — the compiler is the reviewer.
7. Every handler has a table test. Every UI change has a Playwright spec that runs on
   the mobile projects.
8. Migrations are append-only. Status derivation goes through `status.Derive` only.
9. No `TODO`/`FIXME`/`panic("not implemented")` in merged code. `//nolint` needs a
   reason on the same line. No `console.log` in `web/static/js`.
10. Commit everything your task needs. The guard fails on a dirty worktree.

## Handoff

Before finishing, write `harness/handoffs/T-###.md`: what changed, how verified, commands
run with exit codes, open issues. If blocked: `BLOCKED: <reason>` as the first line.
