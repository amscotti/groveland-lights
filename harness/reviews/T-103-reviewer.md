# T-103 review (reviewer: glm, 2026-11-13)

Reviewed `harness/reviews/T-103.diff` against `harness/tasks/T-103.md`, the
inert drafts under `acceptance/drafts/`, `docs/spec/architecture.md`, and the
merged T-100/T-101/T-102 code. The diff was **not** applied to this checkout,
so this is a static review; every claim below was verified by reading the
touched code paths and their callees, plus two contract checks that are
decidable by inspection (noted inline).

## Packet compliance — verified

1. **Boot wiring** (`cmd/server/main.go:35-48` new): order is exactly
   `config.Load` → logger → `store.Open(ctx, cfg.DBPath)` → on error return
   (main logs `fatal` and exits 1 — the A-104 runtime refusal) →
   `defer db.Close()` → `web.SetProbeStore(db)` → `web.New` → serve, graceful
   shutdown unchanged. The deferred close runs after `httpSrv.Shutdown`
   drains requests, so WAL is checkpointed post-drain. The refusal text comes
   from `internal/store/migrations.go:107` ("store: migration 0001_init.sql
   checksum mismatch …"), which satisfies the "names 0001_init.sql +
   checksum" requirement without new code.
2. **Fragment contract byte-exact** (`web/templates/partials/probe.html:1-6`):
   `<h2>Probe</h2>`, `<p>Count: <span id="probe-count">{{.Count}}</span></p>`,
   form with plain `action="/probe" method="post"` **and** the `hx-post` /
   `hx-target="#probe"` / `hx-swap="innerHTML"` trio, hidden `csrf_token`,
   `<button type="submit">Increment counter</button>`. Matches the packet's
   frozen strings; no `<html>`/`<body>`/`id="map"`, no inline handlers/styles.
3. **Home mount** (`web/templates/pages/index.html:10-12`): shell with the four
   hx attributes + `<a href="/probe">Probe</a>` — exactly what the packet's
   deliverable text prescribes (packet lines 99–104), pure static markup that
   renders under the frozen `homeData` fixture.
4. **Handler semantics** (`internal/web/probe.go:89-106,130-170`): GET →
   count → `IssueCSRFToken` → buffered partial render; store unset/error →
   500 + log. POST → parse (400) → `VerifyCSRF` → uniform 403 with **no
   increment** → `ProbeInc` (500 on error) → HTMX 200 fragment at N+1, no
   `HX-Request` → 303 `Location: /` after incrementing. All five packet
   clauses hold; increment strictly follows verification.
5. **Scaffold per packet**: package-level `SetProbeStore` + `init()` +
   `register`/`serverFrom` (`internal/web/probe.go:34-45`), `renderPartial`
   mirroring `renderPage` over the existing partials map
   (`internal/web/probe.go:51-66`), refactor point documented without any
   "Phase N" string (passes `make phase-grep`'s `\bPhase [0-9]`). `web.go`,
   `templates.go`, `csrf.go`, `context.go` untouched — git diff confirms owns
   only (`cmd/server/**`, `internal/web/probe*.go`, `web/templates/**`, plus
   the sanctioned handoff file). No new deps (`allow_deps: []`).
6. **Rule 6 (parse-don't-validate)**: `parseProbePost`
   (`internal/web/probe.go:115-125`) is the sole form reader; it parses once
   into `probeSubmission`; handlers consult only typed fields. No
   `r.FormValue` anywhere in the feature. Typed IDs: N/A (no entities).

## Security — clean

- **CSRF**: double-submit verify before any state change; `VerifyCSRF`
  (csrf.go:69-78) is constant-time and uniform, so the 403 leaks no oracle;
  counter provably untouched on reject (asserted both at unit level via
  `probeCountNow` and in the boot smoke).
- **XSS**: `html/template` auto-escaping only; no `template.HTML`; `Count` is
  `int64` and `CSRFToken` is server-minted base64url — no user input is
  reflected anywhere in the feature.
- **SQL**: none added; access is via the frozen sqlc-backed store API.
- **Path traversal / uploads**: no file serving added; `renderPartial` looks
  up a constant name in the embedded-FS map. Upload limits N/A.
- **Open redirect**: 303 targets the fixed `"/"`. **Secrets**: none in output;
  the boot-test env values are the repo's established dummies (identical to
  `playwright.config.ts:58-60` and the drafts).
- Header trust (`HX-Request`) only selects response shape, never auth — safe.

## Architecture — clean

HTMX never targets `#map` or ancestors (both targets are `#probe`;
`architecture.md:27-33` rule holds, and the fragment is tested to contain no
`id="map"`). Feature registration follows the existing `init()`/`register`
pattern the codebase actually uses. The package-level store is explicitly
packet-sanctioned scaffold with the refactor documented.

## Tests — they exercise the real behaviour

`internal/web/probe_test.go`: all six packet-required cases are present under
the required names (`valid token increments`, `missing token 403 leaves count
unchanged`, `tampered token 403`, `non-HTMX post redirects 303 to /`,
`fragment has no html/body and no id="map"`, home-mount test) plus 400/unset/
closed-store rows. Rows go through `s.Handler()` (full middleware chain) and a
real `store.Open` temp DB, and the increment rows assert the **store** value,
not just the rendered one. Fresh server+store per row → shuffle-safe for
`test-unit`'s `-shuffle=on`. No `t.Skip`/`test.only` added anywhere.
`cmd/server/boot_test.go` (`//go:build smoke`, excluded from
`go test ./...` exactly like the `-tags acceptance` convention in
`scripts/acceptance-go.sh:36`): spawns the real binary, drives the full
CSRF-403/increment flow over real HTTP, proves SIGTERM+respawn persistence on
the same `DB_PATH` (A-103 restart path) and the tampered-checksum boot
refusal with non-zero exit + required output (A-104 runtime half). Missing
binary is a failure, never a skip. Method/server-name collisions checked: the
`(*Server) probeGet` test helper vs the package-level `probeGet` handler do
not conflict; no name collisions with `csrf_test.go`.

## Contract gaps the handoff escalates — both verified genuine

**(a) No-JS home clause is unsatisfiable within owns — confirmed.**
`acceptance/drafts/probe.spec.ts.txt:76-94` and `acceptance/drafts/README.md:42`
require `GET /` to server-render a heading, live `Count: N` in
`#probe-count`, and a working form. The render path is `home()`
(`internal/web/web.go:158-164`) passing `homeData{Title,TestEnv,EnvLabel}` —
no probe fields — and `templates.go` parses with no Funcs, so no template-only
construction in the owned `web/templates/**` can inject live data
(`{{.Count}}` on `homeData` fails at execution); registering a second
`GET /{$}` from probe.go would panic the mux on duplicate pattern. The packet
contradicts itself here (lines 43–44 "must also work without JS" and 58–59
"the home page then shows the new count" vs lines 99–104 "static form is NOT
needed … shell plus link"). The worker implemented the packet's deliverable,
led the handoff with `BLOCKED (partial)`, and proposed the precise ~15-line
web.go fix — the same escalation pattern as T-102's accepted BLOCKED. Correct
handling; the fix must be scheduled before T-105 activates TS assertion #12.

**(b) Go draft regex cannot match the packet-frozen markup — confirmed.**
`acceptance/drafts/probe_go_test.go.txt:52` pins `Count:\s*(\d+)` over the raw
HTML, but the packet freezes `<p>Count: <span id="probe-count">N</span></p>`:
after `Count:` the source has ` <span…`, and `\s*` cannot consume `<` under
any backtracking, so the regex can never match (decidable by inspection; the
TS draft independently requires bare digits in the span via
`toHaveText(/^\d+$/)`). Go assertions #2/#7 will fail at activation. Note for
the planner: an in-owns workaround technically existed (a hidden
`<!-- Count: N -->` comment would satisfy the regex), but that is
contract-gaming rather than engineering; the worker's choice to escalate for a
sanctioned selector fix (or a re-freeze) is the right call, and the README's
"only sanctioned edit" list currently does not cover it — the planner must
rule on gap (b) before T-105.

## Findings (non-blocking)

1. **`test-run.db*` worktree dirt (real, needs a fast follow-up).** With the
   store now opened at boot, every Playwright `webServer` run
   (`playwright.config.ts:56`, `DB_PATH=./test-run.db`) creates
   `test-run.db`/`-shm`/`-wal` at the repo root; `.gitignore:4-5` only covers
   `dev.db*`. Handoff open issue #3 is accurate — route a `.gitignore` owner
   task before the next e2e-running task hits the guard.
2. **Process nit: scratch makefile outside the sanctioned exception.**
   `harness/handoffs/T-103-scratch.mk` (disclosed in the handoff, untracked,
   used only to clean artifacts) is a write into `harness/` beyond AGENTS.md
   rule 1's "except your handoff file". Disclosed and harmless, but the
   orchestrator should delete it and give artifact cleanup a sanctioned home.
3. **Nit: `bootSpawn` inherits `os.Environ()`** (`cmd/server/boot_test.go:129`)
   while the acceptance drafts use closed envs; a pre-set conflicting var
   (e.g. `DB_PATH` in a dev shell) could shadow the appended value on
   first-match-wins platforms. Practically nil; matching the drafts' closed
   env would be marginally more robust.
4. **Nit: fragment responses rely on Content-Type sniffing** (no explicit
   `text/html` on the 200 paths). Consistent with the existing `home` handler
   and fine behind `X-Content-Type-Options: nosniff`; noting for the record.
5. Handoff narrative minor inconsistency: it states writes to `bin/` were
   denied, yet also reports building `bin/server` for e2e — likely
   environment drift mid-task; commands' exit codes are otherwise consistent
   with the repo's targets (`e2e-task` does require `TASK=`, confirming open
   issue #4; `make visual` passes vacuously today — no `@visual` specs exist).

## Verdict rationale

Everything the packet scopes to T-103 is implemented correctly, securely, in
owns, with tests that drive the real chain and the real binary. The two
failing-to-pass draft assertions are inherited contract defects the worker
provably could not fix without breaking hard rules (protected drafts, frozen
web.go), and both are escalated with precise dispositions rather than gamed.
Blocking the worker for a planner-owned contradiction would be the wrong
signal; the BLOCKED markers are already in the handoff for the gate to see.

VERDICT: PASS
