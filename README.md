# Groveland Lights

A public map and reporting app for the Town of Groveland's park and building
light fixtures. One Go binary, SQLite, htmx + Alpine (CSP build) + MapLibre, on
Fly.io. Built — tests first, reviews, validation and all — by a multi-agent
harness in `cmd/harness` + `.opencode/agents/`. `docs/PLAN.md` is the plan.

## Quick start

    mise install
    npm install
    npx playwright install chromium webkit
    make doctor
    make check-fast

Run locally:

    make build
    PORT=8080 APP_ENV=dev BASE_URL=http://localhost:8080 DB_PATH=./dev.db \
      PHOTO_DIR=./dev-photos SESSION_KEY=dev IP_HASH_KEY=dev ADMIN_TOKEN=dev \
      TOWN_BBOX=42.70,-71.09,42.82,-70.96 TZ=America/New_York ./bin/server

## Harness

    make status        # task states + recent progress
    make plan PHASE=1  # planner expands a phase into packets
    make run           # scheduler: workers in worktrees -> reviews -> gate
    make stop          # halt the scheduler

Agents live in `.opencode/agents/` (deny-all permission baselines). The guard,
the serial merge gate and the hidden holdout suite are the real enforcement —
see `docs/PLAN.md` §6 and §9.
