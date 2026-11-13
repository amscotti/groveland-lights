# M1 validation report (operator-compiled, artifacts on disk)

- make validate MILESTONE=M1: 20/20 passed, all four Playwright projects
  (desktop-chromium, pixel-9, iphone-16 webkit, iphone-se-320). Artifacts:
  harness/runs/20261113T121419Z/validate/ (screenshots, traces).
- A-101 healthz 200 <2s: Go acceptance + e2e @smoke, green.
- A-102 CSP + zero console/CSP violations: home_m1.spec.ts, green all projects.
- A-103 probe persists (SQLite) + no-JS 303 fallback: probe.spec.ts green,
  including javaScriptEnabled:false project runs; restart persistence proven by
  acceptance/go spawn-based tests (T-101) and the e2e reload flow.
- A-104 migration checksum guard refuses boot on edited applied migration:
  internal/store tests (TestMigrationChecksumGuard family), green in check.
- A-105 JSON logs with route pattern/status/duration, no IP: acceptance/go
  spawn test with X-Forwarded-For probe, green.
- Reviews: T-100..T-105 all reviewer + reviewer-alt VERDICT: PASS (harness/reviews/).
- Gate: every task merged via serial gate (full make check + holdout) — see
  harness/progress.log (merged lines).
- BLOCKED (human): Fly deploy + deployed smoke (no Fly account; H4). Local
  Docker proven: image 29.25MB, boots, healthz 200.
- Holdout suite ran in every gate (hidden repo, T-100 variants): green.
