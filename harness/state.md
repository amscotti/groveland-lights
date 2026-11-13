# Harness state

**M1 merged and green on main; judge: NOT DONE pending H4** (Fly deploy +
deployed smoke ×2 — human; Docker redeploy proof in runs/20261113T122251Z).
Record: T-100 merged WITHOUT reviews; T-105's stale BLOCKED handoff line is
superseded by the runs/20261113T115949 transcript; ratchet go 51 / pw 32; CI
blocked on GitHub billing (BLOCKED-H1b). Judge-M1 gaps routed: Content-Type →
T-208, db blobs → T-106, T-100 reviews → operator narrative.

## Phase 2 (M2) planned — planner output 2026-11-13

Goal: admin "walk the town" — geolocation-gated add-light flow, photo pipeline
(orient/resize/strip), duplicate detection, edit + soft delete, admin list.
Contract delta A-201..A-209 (scheduler applies to acceptance/contract.json and
harness/tasks/graph.json). Packets: harness/tasks/T-2*.md + T-106.md.

- T-200 acceptance: ALL A-2xx specs as INERT DRAFTS + photo fixtures
  (testdata/photos, committed generator) + schema oracle testdata/
  schema.expected.sql + frozen UI contract table.
- T-106 ‖ T-208 chores (alongside T-200): untrack test-run.db blobs;
  Server.Store refactor (drops SetProbeStore; freezes `New(cfg,log,db)` +
  `s.Store`) + probe fragment Content-Type fix (judge-M1 gaps).
- T-201 ‖ T-202 ‖ T-204 (after T-200): migration 0002 + store (schema must
  equal the expected file), internal/img (only new dep: golang.org/x/image),
  internal/geo (Haversine + duplicate classification).
- T-203 ‖ T-206: auth/session slices (one-time ADMIN_TOKEN via login_tokens;
  hand-written database/sql so sqlc gen stays whole with T-201); photo intake
  + /photos serving, building on T-201/T-202/T-208 frozen APIs.
- T-205 ui-worker (last feature): admin pages + walk.js + HTMX pagination;
  visual self-check via a scratch Playwright config, deleted before commit.
- T-207 acceptance LAST: activates drafts verbatim; disputes → BLOCKED to
  planner (T-105 lesson — never worker edits to oracles).

## Deviations from plan §7 table (why)

- T-207/T-208/T-106 added: per-task gates forbid red specs (drafts pattern);
  Phase-1 close-out pinned the store-onto-Server refactor here; judge-M1
  findings folded in as micro-tasks.
- T-203 deps +T-201/+T-208, owns internal/store/{session,user}*.go (needs the
  0002 tables + Server plumbing). T-205 deps +T-206/+T-208, owns
  web/templates/** + web_test.go (parser only treats pages/* as full pages →
  flat pages/admin-*.html; TestRenderAll needs per-page fixtures).
- Schema oracle is schema.expected.sql, NOT .golden.sql (`*.golden*` is
  protected AND edit-denied even for test-author); ownership still locks it
  to acceptance tasks. A-205 precision: same asset_no → numbered duplicate
  (partial unique index makes distance moot; 25 m only bounds the near
  query); unnumbered within 8 m → Merge or Save anyway.

## Run notes (M2 close-out)

- Order: {T-200, T-106, T-208} (2 slots) → {T-201, T-202, T-204} → {T-203,
  T-206} → T-205 → T-207 → `make validate MILESTONE=M2` → visual-reviewer →
  judge; after T-207 run `make build` (validator webServer needs bin/server).
- Operator pre-steps: H4 (two deploys + smoke evidence) closes M1, independent
  of Phase 2; optional post-M2 exiftool check; spot-check TOWN_BBOX (§5.1).
- Field test (H9b) before judge if possible: five real fixtures in one park,
  photos upright, pins < 15 m; HEIC (R6/C18) is decided from that result.
- Risks: T-205 is the largest packet (split walk-flow from list/edit if it
  thrashes); x/image transitives beyond allow_deps → BLOCKED → planner extends.
- Next: Phase 3 (M3 public map) — T-300 acceptance first; MapLibre vendored
  then + CSP worker ADR (E11); base.html only via T-305; testdata/map/ style.
