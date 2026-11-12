# Architecture

Single Go binary, stdlib `net/http` (method/pattern routing), `html/template` embedded,
SQLite via modernc (WAL, busy_timeout, one writer), htmx + Alpine CSP build + Pico +
MapLibre vendored under `web/static/vendor` (versions in filenames). Fly.io, one machine,
one volume, region ewr, auto-stop off. Nightly ticker (03:15 America/New_York) does
backup + privacy rounding + retention.

## Request flow

browser → Fly proxy → Go binary → ServeMux + middleware (reqID, slog, recover, CSP,
CSRF, rate limit) → templates (full pages + HTMX partials) → SQLite /data/lights.db,
photos /data/photos/YYYY/MM/<id>.jpg.

## Ownership map (which package owns what)

- `internal/config` — env parsing into typed Config; fail fast, list all missing vars.
- `internal/store` — SQLite access (sqlc-generated), migrations runner, all queries.
- `internal/web` — handlers, middleware, sessions, CSRF, rate limit, templates.
  Features register via `Register(mux)` from `internal/web/<feature>.go`; only one task
  per phase touches `routes.go`/`base.html`.
- `internal/img` — decode/EXIF-orient/resize/re-encode; strips all metadata.
- `internal/geo` — bbox check, Haversine, duplicate distance.
- `internal/status` — `Derive(reports, now)` — the ONLY status derivation path.
- `internal/jobs` — nightly ticker jobs (backup, rounding, retention).

## The HTMX/map rule

The map lives in `<div id="map">`, initialised once (Alpine component). HTMX never
swaps `#map` or ancestors; status changes reach the map via
`map.getSource('lights').setData(...)`. Enforced by a Go contract test (no partial
contains `id="map"`; every `hx-target` resolves outside `#map`'s ancestors) and a
Playwright test (`window.__glMapInits === 1` after 10 swaps, test env only).

## CSP

`default-src 'self'; script-src 'self' https://challenges.cloudflare.com; style-src
'self'; img-src 'self' data: blob:; worker-src 'self'; connect-src 'self'
https://tiles.openfreemap.org; frame-src https://challenges.cloudflare.com; base-uri
'none'; form-action 'self'; frame-ancestors 'none'` + nosniff + Referrer-Policy +
Permissions-Policy. Proven by a Playwright CSP-violation test, not assertion.

## Status rules (corroboration)

- Report under rate limits → `status_source='single'`.
- Same status, different session AND different IP HMAC, within 24h → `'confirmed'`,
  confirming reporter +3 pts.
- Admin edit → `'admin'`. Staleness (>30d) computed at read time; renders grey with
  "Stale — last report N days ago".
- UI never shows bare status: "On — confirmed 3 days ago by 4 people".

## Dependencies policy

Allow-listed per task (`allow_deps`). Currently: modernc.org/sqlite v1.62.0,
golang.org/x/image v0.48.0, sqlc v1.31.1 (codegen), AWS SDK Go v2 (Phase 5 backups).
No JS dependencies beyond vendored files; no bundler.
