# Groveland Lights: Implementation Plan v2 (harness-native)

**Status:** proposed (revision **v2.1**, 2026-11-12), supersedes v1 (2026-11-02) and the v2 draft of the same date
**Date:** 2026-11-12
**For:** Anthony Scotti
**Inputs:** *Groveland Lights — Research Findings* (2026-11-02), *Groveland Lights — Implementation Plan* v1 (2026-11-02), *Long-running agent methodology* (2026-11-11), and a review of the v2 draft (2026-11-12, §2.3)
**Executor:** OpenCode (v2.2.7, verified running in the execution environment) with **GLM 5.3** and **GLM 5.3 Flash** (Z.ai Coding Plan) and **DeepSeek** (DeepSeek API). Resolved model IDs, verified via `opencode models` in the execution environment on 2026-11-12: `zai-coding-plan/glm-5.3`, `zai-coding-plan/glm-5.3-flash`, `deepseek/deepseek-flash`.

Every version and platform claim was re-checked on 2026-11-12 against the upstream release feed, registry or docs page linked in §15. **[unverified]** marks claims I couldn't confirm. **[decision]** marks a judgment call, not a fact. The OpenCode platform claims in §6.1.2 and §6.1.4 were additionally verified **empirically in the execution environment** (the same OpenCode install, providers and models that will run this plan).

---

## 0. How to use this document

1. Do the **human-only steps** in §11 that Phase 0 needs: a GitHub repo, OpenCode installed with your providers signed in, and `mise` installed.
2. Open OpenCode in an empty clone of the repo and paste the **Phase 0 bootstrap block** (§6.2) into the default `build` agent. It builds the harness: rules, agent definitions, task graph, guards, scheduler, merge gate, hooks, Makefile and CI. Then it proves the harness works with the self-test in §6.3.
3. From then on you drive the project through the harness. `make plan PHASE=n` asks the planner to expand a phase into task packets. `make run` starts the scheduler, which runs workers in worktrees, sends their output through review and validation, and merges through the gate. `make status` shows where things stand. You step in only for `BLOCKED` tasks and the human-only steps.
4. A phase is done when its **Definition of Done commands** pass *and* the judge agent signs the milestone off. Neither is enough alone.

---

## 1. PM overview

**Problem.** Groveland has about 713 LED street lights, all owned by the Groveland Municipal Light Department (GMLD). The figure comes from GMLD's annual return. It sits on a page headed "Year ended December 31, 2019" and is repeated in later returns, so treat it as a stale baseline. The town also has an uncounted set of numbered fixtures in parks and on town buildings, maintained by the Highway Department. None of them appear on any map: the town's MIMAP/GIS has no lights layer, MassGIS has no streetlight dataset, and OpenStreetMap has 0 `highway=street_lamp` features in town (all per the research, verified 2026-11-02). To report an outage, a resident has to know which department to call and describe the location in words.

**Product.** A small public web app:
- a map of every known light, coloured by its *derived* status;
- three-tap reporting (on / off / degraded) with an optional photo and comment;
- an admin "walk the town" flow on a phone that records the fixture number, a confirmed GPS pin and a photo in under 30 seconds.

**v1 scope.** Parks and town buildings only, roughly 60–150 fixtures, maintained by the Highway Department. Street lights are a later, optional phase, gated on GMLD sharing its inventory.

**Users.**
- **Residents:** anonymous, with an optional handle, mostly on phones.
- **One admin:** Anthony.
- **The departments:** receive a digest later; not users in v1.

**Success criteria (product):**
1. Every in-scope fixture is on the map, with a photo and a pin accurate to under 15 m.
2. A first-time visitor on a phone reaches a light's detail panel in two taps and files a report in three.
3. A report changes what the map shows only through the corroboration rules in §4.4. No bare "ON" is ever shown.
4. Losing the disk loses at most 24 hours of data, and service is restored from the runbook in under 15 minutes.
5. It runs for about $4/month of Fly resources plus a domain. Fly waives personal-org bills under $5, so the actual charge may be $0 (§2, C7).

**Success criteria (engineering):** every phase lands through the harness. That means acceptance specs written before features, workers that can't touch them, black-box validation, a serial merge gate, and a judge sign-off. CI mirrors `make ci` exactly.

**Non-goals** (unchanged from v1): no native app; no real-time push (a 60 s GeoJSON cache is enough); residents can't create lights; no integration with GMLD systems beyond an optional digest; no dark-sky analysis.

**Product risk to keep in view:** the research's top risk was that **nobody acts on the reports**. Line up a "send me a weekly digest" agreement with the Highway Department (Arthur Markos) before launch. The harness can't fix that.

---

## 2. Changes from v1

### 2.1 Factual corrections (verified 2026-11-12)

| # | v1 said | Correct as of 2026-11-12 | Source |
|---|---|---|---|
| C1 | "Go (latest stable via mise)", `go = "latest"` | **Go 1.27.4** is the latest stable release (1.27.0 shipped 2026-08-19; 1.26.11 is the previous line). `latest` isn't a pin. Pin `go = "1.27.4"` in `mise.toml`, and use `go 1.27` plus `toolchain go1.27.4` in `go.mod`. | go.dev/dl JSON, go.dev/doc/devel/release |
| C2 | "HTMX 2.x (pinned at scaffold time)" | **htmx 2.0.11** (released 2026-11-01) is npm `latest`. **htmx 4.0.1** also exists (2026-11-01) under npm's `next` tag, and htmx.org says 4 will be marked latest "at some point in 2027". Pin **2.0.11** **[decision]**: 4.x is a new major version with a migration path, and nothing here needs it. **The npm package is `htmx.org`** — the bare `htmx` package is an unrelated stale squatter at 0.0.2; never install it (verified against registry dist-tags 2026-11-12). | npm dist-tags (`htmx.org`, `maplibre-gl`, `@playwright/test`), htmx.org, GitHub releases |
| C3 | "Alpine.js 3.17.5" | Correct: 3.17.5 (2026-10-20). **Add `@alpinejs/csp` 3.17.5.** The standard Alpine build evaluates expressions with `Function()`, which needs `unsafe-eval` and so conflicts with the strict CSP this plan adds (§3.4). | npm, GitHub releases |
| C4 | "Pico.css 2.1.1… no longer actively released" | Correct: 2.1.1 (2025-03-15) is still the latest release, about 20 months old. Vendor it. | npm, GitHub releases |
| C5 | "MapLibre GL JS 6.16.0" | **6.18.0** (2026-11-11). 6.16.0 is from 2025-10-25 and 6.17.0 from 2026-11-07. Pin 6.18.0 at scaffold time and vendor it. MapLibre 6 loads as an ES module, and its worker comes from a same-origin blob unless you call `setWorkerUrl`. This matters for CSP (§3.4). | GitHub releases, maplibre.org docs |
| C6 | OpenFreeMap: "free… no request cap" | Correct, with two additions. **Attribution is required** (MapLibre adds it automatically, so don't hide it). The ToS says the service is provided "as-is" with **no SLA**. Plan a fallback: a Massachusetts PMTiles extract, as the research suggested. | openfreemap.org, /tos |
| C7 | Fly: "volumes come with automatic daily snapshots at no plan premium; ~$4/month" | Snapshots are **billed at $0.08/GB-month after the first 10 GB free**. The default retention is **5 days**, configurable from 1 to 60. Compute is **$3.69/month** for shared-cpu-1x 512 MB always-on (ewr and iad are the cheapest regions; the smallest 256 MB machine is $2.19), and volumes are **$0.15/GB-month**. Fly **waives a personal org's monthly bill under $5** ("Free Waiver"). There's no free tier, and the trial is $1 of credit for 7 days. Expected spend is about $3.84/month, likely waived **[waiver terms are Fly's to change]**. | fly.io/docs/about/pricing, /volumes/snapshots |
| C8 | `fly volumes snapshots list data` | The command takes a **volume ID**, not a name: `fly volumes snapshots list <volume id>` (flyctl v0.5.8 help). Get the ID with `fly volumes list --json`. | flyctl v0.5.8 `--help` |
| C9 | `fly.toml`: `min_machines_running = 1` | `min_machines_running` only matters when `auto_stop_machines` is `"stop"` or `"suspend"`. v1 runs the **nightly backup as an in-process ticker**, which won't fire on a stopped machine, so set **`auto_stop_machines = "off"`** (and `auto_start_machines = false`). With a single machine and a single volume, every deploy has a short gap, because the volume can't be attached to two machines. Accept it, and note it in the runbook. | fly.io/docs/reference/configuration |
| C10 | `fly deploy --ha=false` | Valid: `--ha` defaults to true on both `fly launch` and `fly deploy`. Use it on `fly launch` so a second machine (and volume) is never created. | flyctl v0.5.8 docs |
| C11 | Restore via "print the exact `fly volumes create` command" | Use `fly volumes create <name> --snapshot-id <id> --region ewr --size N` for snapshot restores. A restore from the R2 backup needs a fresh volume plus a file copy (§9.7). Both flags confirmed in flyctl v0.5.8. | flyctl `--help` |
| C12 | Phase 1 DoD: `sqlite3 /data/lights.db …` via `fly ssh console` | The runtime image is Alpine and **has no `sqlite3` binary** unless you install it. Either `apk add sqlite` in the runtime stage (about 1–2 MB **[unverified size]**) or add a `server admin stats` subcommand to the binary. This plan uses the subcommand: no extra package, and it's testable. | Dockerfile in v1 |
| C13 | `.golangci.yml` "enables errcheck, govet, staticcheck…" (v1-style config) | golangci-lint is at **v2.15.0** (2026-10-16), and v2 needs **`version: "2"`** with the `linters.enable` / `formatters` schema. `gosimple` and `stylecheck` are now part of `staticcheck`, so a v1-style file won't load. Cap complexity with `cyclop` or `gocyclo` under `linters.settings`. | golangci-lint releases, golangci.run config reference |
| C14 | `mise.toml` "There is no Makefile" | **[decision]** Anthony asked for `make check`. `mise` still pins tools, and a **Makefile** becomes the single entry point for both the harness and CI. | Anthony's request |
| C15 | Playwright mentioned only as one smoke test in Phase 2 | **Playwright 1.65.0** (2026-11-05) is used from Phase 0 onward. **@axe-core/playwright 4.14.0** and **axe-core 4.14.0** handle accessibility. **Lighthouse 13.6.0** and **@lhci/cli 0.16.1** are optional. Device descriptors **"Pixel 9"**, **"iPhone 16"** and **"iPhone SE (3rd gen)"** exist in 1.65's device list. | npm registry, Playwright 1.65 types |
| C16 | `modernc.org/sqlite` (no version) | **v1.62.0** (2026-11-07). | proxy.golang.org |
| C17 | Research recommended Render; v1 chose Fly | Not a factual error, but a deliberate divergence. **[decision]** Keep Fly: everything is CLI-driven, so an agent can run it, and the cost is lower. Render's appeal (no Dockerfile) matters less when an agent writes the Dockerfile. | research §6, v1 §7 |
| C18 | Photo pipeline "re-encode… strip all EXIF" | Re-encoding with Go's `image/jpeg` does drop all metadata, but it **also drops the EXIF Orientation tag**. Portrait phone photos would then display rotated. **Read the orientation and apply it before stripping.** Also, Go's standard library can't decode HEIC. Whether iOS Safari converts HEIC to JPEG for `<input type=file accept="image/*">` uploads is **[unverified]**, so test it on a real iPhone, and reject undecodable formats with a 415 and a clear message. | Go stdlib behaviour; design review |
| C19 | Turnstile "free; two env vars" | Correct. The free plan allows up to 20 widgets with unlimited challenges. Cloudflare documents test keys; use them in CI so tests never call the real service. | developers.cloudflare.com/turnstile/plans |
| C20 | R2 "10 GB free, egress free, $0.015/GB-mo after" | Correct. Standard storage is $0.015/GB-month with 10 GB-month free and 1M Class A operations a month free. Egress is free. | developers.cloudflare.com/r2/pricing |

### 2.2 Design corrections (internal consistency)

| # | Issue in v1 | Fix in v2 |
|---|---|---|
| D1 | Status says a light "becomes `confirmed`", but `lights.status` only allows `on/off/degraded/unknown`. | Split it: `status` stays as the observed value, plus a new `status_source` (`single`, `confirmed` or `admin`) and `status_confirmed_at`. Staleness is computed at read time, never stored. |
| D2 | `lights` has no photo reference and no soft-delete column, though the admin list promises "soft delete". | Add `photo_id` and `deleted_at`. All public queries filter `deleted_at IS NULL`, and a test enforces it. |
| D3 | The moderation queue has nowhere to store flags. | Add a `comment_flags` table (comment, reporter session, reason, time). |
| D4 | `ip_hash` with no salt is reversible for IPv4. | Store `HMAC-SHA256(IP_HASH_KEY, ip)` truncated, with a new secret `IP_HASH_KEY`. |
| D5 | Privacy promises reporter location "rounded to 25 m after 30 days", but nothing does the rounding. | Add a nightly job in the same ticker as backups, with a test. |
| D6 | Points reward "the report that confirms", but the research says reward *corroboration, not volume*. | Keep v1's rules (+1 per report, +3 for confirming, +1 per photo), and add a daily cap per user so the leaderboard can't be farmed. |
| D7 | "Code never references this document": the CI grep scans all `*.go`, which would include harness code. | Scope the grep to `cmd/server`, `internal`, `web` and `migrations`. |
| D8 | Phase 0 deployed before any test infrastructure existed. | Phase 0 is now the harness. The v1 Phase 0 (skeleton and proven deploy path) becomes Phase 1, and all later phases shift by one. |
| D9 | Lighthouse mobile performance ≥ 90 with MapLibre on the first screen. | MapLibre is a large bundle **[size unverified]**. Initialise the map after first paint, put the list and text content first, and treat ≥ 90 as a target checked by an optional Lighthouse run. The hard gates are axe, layout and the JS-disabled checks. |

### 2.3 Review fixes (v2.1, 2026-11-12)

The v2 draft was reviewed in the execution environment. Fixes below are normative; where they touch v2 text, this revision wins.

| # | Issue in v2 | Fix in v2.1 |
|---|---|---|
| E1 | `../holdout` is relative, but the scheduler spawns every role in `../wt/T-###`; from a worktree it resolves to `../wt/holdout`, which doesn't exist (§9.2, §6.1.6 gate step 5). | The holdout path is resolved **absolutely** at scheduler start, stored in `harness/policy.json` (`holdout_dir`), exported to Make as `HOLDOUT_DIR`, and scoped into test-author's `external_directory` allow. `make holdout` uses `$(HOLDOUT_DIR)`, never a relative path (§6.1.8, §9.2). |
| E2 | Fresh worktrees have no `node_modules`; `make e2e-task` would run `npx`, silently fetching unpinned `latest` from the registry. CI also never ran `npm ci`. | `worktree-new.sh` symlinks `node_modules` from the main repo (or runs `npm ci` if linking fails). CI gains an explicit `npm ci` step (§6.1.7). |
| E3 | The guard checks committed diffs and checksums only; an **uncommitted** edit to `Makefile`, `.golangci.yml` or fixtures could weaken the gate's `make check` verdict without being caught. | Guard check 9 (exit 9): the worktree must be **clean** — `git status --porcelain` empty, ignoring `harness/handoffs/`, `harness/runs/` and `node_modules` (§6.1.5). |
| E4 | Unprobed escape vector: a session running in a linked worktree may treat the **main checkout** as internal (OpenCode's `external_directory` check skips paths inside "the active Location and its non-root project worktree"), letting a worker edit files the scheduler then executes (`cmd/harness`). | Selftest S6 gains a probe: a worker in a worktree tries to edit `<main>/cmd/harness/sched.go`. The scheduler asserts the main checkout is clean before every spawn and gate run, and stops loudly if not (§6.1.4, §6.3 row 4). |
| E5 | Orphan assertions violated the plan's own invariant ("every assertion is claimed by at least one task"): A-410 unclaimed (Phase 4), A-605 unclaimed (Phase 6), A-309 a non-gating target with an assertion ID. | A-410 → claimed by T-404. A-605 → `verified: "human"` class. A-309 → `gate: false` class, claimed by T-302. `contract.json` assertions may carry `gate: false` (recorded target) or `verified: "human"` (field check); both still need an owner, neither needs a green automated test (§6.1.3). |
| E6 | §6.1.3's example task ("T-204 Photo pipeline", owns `internal/img/**`) collided with the real Phase 2 task table, where T-204 is the geo task and the photo pipeline is T-202. | Example renamed to match the real M2 table: `T-202`, deps `T-200`, owns `internal/img/**` only. |
| E7 | Stale v1 remnants: schema comment still said "rounded to ~25 m"; Q5 re-opened the 3-dp-vs-4-dp question that D10 had already decided. | Comment fixed to 3 dp (~111 m); Q5 removed from open questions. |
| E8 | Phase 5 DoD omitted `make validate MILESTONE=M5` and judge sign-off; Phase 1 had no milestone ID. | Phase 5 DoD now validates M5; Phase 1's judge input is labelled M1. |
| E9 | Small omissions: `lefthook install` never run; the scheduler never commits packets/`graph.json` to main before creating worktrees (so `--file harness/tasks/T-###.md` wouldn't resolve in the worktree); `fly.toml` `[mounts]` keys `snapshot_retention`/`scheduled_snapshots` unverified. | Bootstrap step 10 runs `lefthook install`; the scheduler commits graph and packets before spawning (§6.1.4); the unverified fly.toml keys are dropped — snapshot retention is set at `fly volumes create` time (§11 H4, §5.3). |
| E10 | No flaky-E2E policy at the serial gate: one flaky Playwright spec blocks all merges. | Gate retry rule: a failed Playwright target is re-run once; a pass is logged as `flake` and the gate proceeds; a second failure fails the gate. Non-Playwright targets have no retry (§6.1.6, §10 #15). |
| E11 | `worker-src 'self'` + `setWorkerUrl` might not hold if MapLibre 6.18's ESM dist has no separate worker file. | The fallback is pre-decided: `worker-src 'self' blob:`, recorded in an ADR at Phase 3 scaffold time. Proven by the CSP-violation test either way (§3.4). |
| E12 | The §6.2 paste block was one 13-step mega-task; "roughly a day" was optimistic. | Two hard review checkpoints added (after steps 5 and 12); budget is two to three supervised days, with Phase 1 expected to shake out scheduler bugs (§14). |
| E13 | A session stores its selected model separately from agent selection; nothing verified *at runtime* which model actually served each run, though the vision routing depends on it. | The scheduler asserts the provider/model that served each run from the JSON transcript and stops if it differs from `policy.json` (§6.1.4). |
| E14 | 25 MB image target is unrealistic once modernc sqlite and the AWS SDK are in. | Target raised to 40 MB; Fly pricing is unaffected (§5.3, §10 #14). |
| E15 | Open questions Q2, Q3, Q5, Q7 were decidable now and were blocking planner input. | Decided: Q2 → AWS SDK for Go v2 (§10 #13); Q3 → Turnstile always-on in managed/invisible mode (§10 #11, A-406); Q5 → 3 dp (D10, already decided); Q7 → magic links post-v1 (§5.1). Remaining: Q1, Q4, Q6. |

---

## 3. Architecture and stack

### 3.1 Stack (pinned, verified 2026-11-12)

| Layer | Choice | Pin | Notes |
|---|---|---|---|
| Language | Go | **1.27.4** | `mise.toml` and `go.mod` `toolchain` |
| HTTP | `net/http` + `http.ServeMux` method/pattern routing (since Go 1.22) | stdlib | No framework |
| Templates | `html/template`, embedded with `//go:embed` | stdlib | Every template parsed and rendered in tests (§8.4) |
| Interactivity | htmx (npm package **`htmx.org`**) | **2.0.11** | Vendored to `web/static/vendor/`. `htmx.config.allowEval = false`, no `hx-on`. |
| Client state | Alpine.js **CSP build** (`@alpinejs/csp`) | **3.17.5** | Vendored. Components registered with `Alpine.data`, no inline expressions that need eval. |
| CSS | Pico.css + about 150 lines of local CSS | **2.1.1** | Vendored |
| Map | MapLibre GL JS + OpenFreeMap "liberty" style | **6.18.0** | Vendored ESM. `setWorkerUrl` points at a same-origin worker file so CSP can keep `worker-src 'self'` (per MapLibre docs). |
| DB | SQLite via `modernc.org/sqlite` (pure Go, no cgo) | **v1.62.0** | WAL mode, `busy_timeout`, one writer connection |
| Query typing | `sqlc` (SQLite engine) generating `database/sql` code **[decision]** | **v1.31.1** | Typed queries help agents; checked in and regenerated by `make generate` |
| Image | stdlib `image/jpeg` and `image/png`, plus `golang.org/x/image` (`draw`, `webp`) | x/image **v0.48.0** | Orientation applied before re-encode (C18) |
| Auth | Session cookies (anon, user, admin); admin one-time token; magic link later | — | — |
| Bot defence | Cloudflare Turnstile | — | Test keys in CI |
| Email (later) | Resend | — | Only when magic links ship |
| Hosting | Fly.io, 1 Machine (shared-cpu-1x 512 MB), 1 × 1 GB volume, region **ewr** | flyctl **v0.5.8** | `--ha=false`, `auto_stop_machines="off"` |
| Backups | Fly snapshots (14 days) plus nightly `VACUUM INTO` copied to Cloudflare R2 (30 days) | — | Litestream **v0.5.18** is the upgrade path if you ever need RPO below 24 h **[decision: not now]** |
| Object storage (Phase 5) | AWS SDK for Go v2 (`aws-sdk-go-v2` + `config`, `service/s3`, `feature/s3/manager`) | latest at T-501 | **[decision, §10 #13]** over hand-rolled SigV4: backups are where chunked-signing, clock-skew and encoding edge cases fail, and a restore drill is the wrong place to find out. |

**Dev and verification toolchain** (pinned in `mise.toml` and `package.json`):

| Tool | Pin | Role |
|---|---|---|
| golangci-lint | **v2.15.0** | Lint (v2 config). staticcheck runs inside it. |
| staticcheck (standalone, optional) | **2026.2.2** | Only if you want it outside golangci |
| govulncheck (`golang.org/x/vuln`) | **v1.9.0** | Known-vulnerability scan, wired with a Go 1.24+ `tool` directive |
| Playwright (`@playwright/test`) | **1.65.0** | E2E, mobile, geolocation, upload, traces, screenshots |
| `@axe-core/playwright` / `axe-core` | **4.14.0** | Accessibility gate |
| html-validate | **11.21.0** | Validates rendered HTML snapshots |
| TypeScript (`tsc --noEmit --checkJs`) | **7.0.2** | Type-checks `app.js` (JSDoc) and Playwright specs, with no build output |
| Lighthouse / @lhci/cli | **13.6.0 / 0.16.1** | Optional performance budget |
| lefthook | **v2.2.2** | Git hooks (single binary, installed by mise) |
| gitleaks | **v8.30.1** | Secret scan in pre-commit and CI |
| goose | not used | **[decision]** A hand-written, embedded migration runner (§4.3) keeps zero dependencies, and it gets its own tests. |
| GitHub Actions | `actions/checkout@v7`, `jdx/mise-action@v5`, `actions/upload-artifact@v7` | Latest majors as of 2026-11-12 |
| OpenCode | **v2.2.7** | Harness runtime. Agents in `.opencode/agents/*.md`, V2 permission rules. |

### 3.2 Request flow

```
browser ──HTTPS──▶ Fly proxy ──▶ Go binary (single process, always on)
                                    ├── net/http ServeMux (+ middleware: reqID, slog, recover, CSP, CSRF, rate limit)
                                    ├── html/template (full pages + HTMX partials)
                                    ├── SQLite /data/lights.db (WAL; 1 writer conn, N reader conns)
                                    ├── photos /data/photos/YYYY/MM/<id>.jpg
                                    └── ticker: 03:15 America/New_York → backup + privacy rounding + retention
```

### 3.3 The HTMX and map rule (unchanged, now enforced by tests)

The map lives in `<div id="map">` and is initialised once in an Alpine component. HTMX **never** swaps the map container or its ancestors. Status changes reach the map through an Alpine store that calls `map.getSource('lights').setData(...)`. **Enforcement:** a Go contract test asserts that no partial response contains `id="map"`, and that every `hx-target` in the templates resolves to an element that isn't an ancestor of `#map`. A Playwright test asserts the map instance survives ten HTMX swaps: `window.__glMapInits === 1` is exposed only when `APP_ENV=test`.

### 3.4 Security headers

```
Content-Security-Policy: default-src 'self'; script-src 'self' https://challenges.cloudflare.com;
  style-src 'self'; img-src 'self' data: blob:; worker-src 'self';
  connect-src 'self' https://tiles.openfreemap.org; frame-src https://challenges.cloudflare.com;
  base-uri 'none'; form-action 'self'; frame-ancestors 'none'
```

Plus `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`, and `Permissions-Policy: geolocation=(self), camera=(self)`.

The exact hosts MapLibre needs (style, tiles, glyphs and sprites may come from different OpenFreeMap paths) and the exact Turnstile CSP entries are **[unverified]**. Phase 3 ends with a Playwright test that fails on any CSP violation reported in the console, so the policy is proven by test, not by assertion.

**Pre-decided fallback (E11):** if MapLibre 6.18's ESM dist cannot serve its worker from a same-origin file via `setWorkerUrl`, relax to `worker-src 'self' blob:` (MapLibre's default worker comes from a blob URL), record it in an ADR, and move on. Don't deliberate mid-phase; the CSP-violation test decides.

---

## 4. Data model (revised)

### 4.1 Schema (`migrations/0001_init.sql`)

```sql
PRAGMA foreign_keys = ON;

CREATE TABLE users (
  id          INTEGER PRIMARY KEY,
  handle      TEXT NOT NULL UNIQUE,
  email       TEXT UNIQUE,
  role        TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin')),
  points      INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL
);

CREATE TABLE photos (
  id          INTEGER PRIMARY KEY,
  owner_id    INTEGER REFERENCES users(id),
  path        TEXT NOT NULL UNIQUE,            -- relative to PHOTO_DIR, server-generated only
  width       INTEGER NOT NULL,
  height      INTEGER NOT NULL,
  bytes       INTEGER NOT NULL,
  sha256      TEXT NOT NULL,
  created_at  TEXT NOT NULL
);

CREATE TABLE lights (
  id                  INTEGER PRIMARY KEY,
  asset_no            TEXT,
  kind                TEXT NOT NULL CHECK (kind IN ('park','building','street')),
  label               TEXT NOT NULL,
  lat                 REAL NOT NULL,
  lon                 REAL NOT NULL,
  accuracy_m          REAL,
  photo_id            INTEGER REFERENCES photos(id),
  status              TEXT NOT NULL DEFAULT 'unknown' CHECK (status IN ('on','off','degraded','unknown')),
  status_source       TEXT NOT NULL DEFAULT 'none' CHECK (status_source IN ('none','single','confirmed','admin')),
  status_at           TEXT,
  status_confirmed_at TEXT,
  gml_pole_no         TEXT,
  created_by          INTEGER REFERENCES users(id),
  created_at          TEXT NOT NULL,
  updated_at          TEXT NOT NULL,
  deleted_at          TEXT
);
CREATE INDEX lights_geo ON lights(lat, lon) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX lights_asset_no ON lights(asset_no) WHERE asset_no IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE reports (
  id          INTEGER PRIMARY KEY,
  light_id    INTEGER NOT NULL REFERENCES lights(id),
  user_id     INTEGER NOT NULL REFERENCES users(id),
  session_id  TEXT NOT NULL,
  status      TEXT NOT NULL CHECK (status IN ('on','off','degraded')),
  note        TEXT CHECK (length(note) <= 500),
  photo_id    INTEGER REFERENCES photos(id),
  lat REAL, lon REAL, accuracy_m REAL,       -- rounded to 3 dp (~111 m N–S) after 30 days (D10, A-409)
  ip_hmac     TEXT NOT NULL,                 -- HMAC(IP_HASH_KEY, ip), truncated; rate limiting only
  created_at  TEXT NOT NULL,
  rounded_at  TEXT
);
CREATE INDEX reports_light_time ON reports(light_id, created_at DESC);
CREATE INDEX reports_user_time  ON reports(user_id, created_at DESC);

CREATE TABLE comments (
  id          INTEGER PRIMARY KEY,
  light_id    INTEGER NOT NULL REFERENCES lights(id),
  user_id     INTEGER NOT NULL REFERENCES users(id),
  body        TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 2000),
  hidden      INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL
);

CREATE TABLE comment_flags (
  comment_id  INTEGER NOT NULL REFERENCES comments(id),
  session_id  TEXT NOT NULL,
  reason      TEXT,
  created_at  TEXT NOT NULL,
  PRIMARY KEY (comment_id, session_id)
);

CREATE TABLE sessions (
  id          TEXT PRIMARY KEY,               -- 32 random bytes, base64url; cookie holds signed id
  user_id     INTEGER NOT NULL REFERENCES users(id),
  role        TEXT NOT NULL CHECK (role IN ('anon','user','admin')),
  created_at  TEXT NOT NULL,
  expires_at  TEXT NOT NULL
);

CREATE TABLE login_tokens (
  token_hash  TEXT PRIMARY KEY,
  email       TEXT NOT NULL,
  expires_at  TEXT NOT NULL,
  used_at     TEXT
);
```

Changes from v1: **D1–D5**. `ON DELETE CASCADE` was dropped from `reports` and `comments` because lights are soft-deleted, and a hard delete should fail loudly rather than silently erase history.

### 4.2 Typed domain (Go)

- **Typed IDs:** `type LightID int64`, `ReportID`, `UserID`. The compiler rejects passing a `UserID` where a `LightID` is expected.
- **Enum-like types:** `type Status string` with `StatusOn`, `StatusOff`, `StatusDegraded` and `StatusUnknown`, plus `ParseStatus(string) (Status, error)`. The golangci `exhaustive` linter forces every switch to cover all values.
- **Parse, don't validate:** each handler turns `*http.Request` into a typed input struct (`ParseNewLight`, `ParseReport`) at the boundary. Handlers never read `r.FormValue` after that.
- **Status derivation is one pure function:** `func Derive(reports []Report, now time.Time) LightStatus`. It is table-tested and property-tested (§8.3), and `store.RecordReport` uses it inside the transaction.

### 4.3 Migrations

Ordered `.sql` files are embedded with `//go:embed`, applied at startup in one transaction per file, and recorded in `schema_migrations(version, applied_at, sha256)`. Startup **refuses to boot** if an applied migration's checksum no longer matches its file, so an edited migration can't go unnoticed. Migration tests are in §8.5.

### 4.4 Status rules (unchanged intent, made precise)

- A report from a session under its rate limits sets `status`, `status_at` and `status_source='single'`.
- A second report with the **same status from a different session and a different `ip_hmac`** within 24 h sets `status_source='confirmed'` and `status_confirmed_at`. The confirming reporter gets +3 points.
- Admin edits set `status_source='admin'`.
- **Staleness is computed at read time:** if `now - status_at > 30 days`, the light renders grey with the text "Stale — last report N days ago".
- The UI never shows a bare status. It shows, for example, "On — confirmed 3 days ago by 4 people".
- Points: +1 per report, +3 for confirming, +1 per photo. A user can earn at most 10 points a day. `users.points` is recomputed from `reports` inside the same transaction, and a property test asserts the invariant.

---

## 5. Configuration, layout and operations

### 5.1 Environment

| Variable | Required | Example / default (dev) | Notes |
|---|---|---|---|
| `PORT` | yes | `8080` | Bind `0.0.0.0:$PORT` |
| `APP_ENV` | yes | `dev` / `test` / `prod` | `test` exposes test hooks (`window.__glMapInits`, `__map`). In `prod` these are compiled out by a build tag, and a test checks that. |
| `BASE_URL` | yes | `http://localhost:8080` | Used for cookies and magic links |
| `DB_PATH` | yes | `./dev.db` / `/data/lights.db` | — |
| `PHOTO_DIR` | yes | `./dev-photos` / `/data/photos` | — |
| `SESSION_KEY` | yes | 32 random bytes, base64 | Signs session cookies |
| `IP_HASH_KEY` | yes | 32 random bytes, base64 | New (D4) |
| `ADMIN_TOKEN` | yes | 32 random bytes, base64 | One-time exchange for an admin session |
| `TOWN_BBOX` | yes | `42.70,-71.09,42.82,-70.96` | From the research. **Spot-check it against the town boundary in Phase 2 [unverified exact bounds].** |
| `TZ` | yes | `America/New_York` | Ticker schedule |
| `TURNSTILE_SITE_KEY`, `TURNSTILE_SECRET` | Phase 4+ | Cloudflare test keys in CI | — |
| `R2_ENDPOINT`, `R2_BUCKET`, `R2_KEY`, `R2_SECRET` | Phase 5+ | — | Backups |
| `HOLDOUT_DIR` | scheduler | absolute path to the holdout clone | Not a secret. The scheduler resolves it at start (default: `<main-repo>/../holdout`), writes it to `harness/policy.json`, and exports it to Make. **Never a relative path** — agents run from worktrees (E1). |
| `RESEND_API_KEY` | later (post-v1) | — | Magic link |

### 5.2 Repository layout (app + harness)

```
groveland-lights/
├── AGENTS.md                         # ≤120 lines: map + hard rules (harness reads this)
├── docs/
│   ├── PLAN.md                       # this document (planner input; code never references it)
│   ├── spec/product.md               # PM overview distilled
│   ├── spec/architecture.md          # layers, ownership map, dependency policy, CSP
│   ├── decisions/ADR-*.md
│   └── runbook.md                    # ops: logs, ssh, deploy, rollback, restore (Phase 5)
├── acceptance/                       # PROTECTED: written by test-author before features
│   ├── contract.json                 # assertion list A-### (WHEN … THE SYSTEM SHALL …)
│   ├── go/                           # Go black-box tests (build tag `acceptance`) against a running server
│   └── e2e/                          # Playwright specs (*.spec.ts) tagged @A-### and @smoke
├── harness/
│   ├── policy.json                   # protected globs, roles, model map (by name→ID), concurrency
│   ├── tasks/graph.json              # task DAG (§6.1.3) — only the scheduler mutates status
│   ├── tasks/T-###.md                # self-contained task packets
│   ├── handoffs/  reviews/  findings/  # agent outputs (append new files only)
│   ├── state.md                      # planner's rewritten current-state summary
│   ├── progress.log                  # append-only; scheduler writes one line per event
│   ├── protected.sha256              # checksum manifest of acceptance/** + golden files
│   └── runs/<ts>/<task>/             # opencode JSON transcripts, Playwright reports/traces
├── .opencode/agents/                 # planner, test-author, worker, ui-worker, reviewer,
│                                     # reviewer-alt, visual-reviewer, validator, judge, integrator
├── cmd/
│   ├── server/main.go                # the app
│   └── harness/                      # Go CLI: sched, guard, gate, verify-agents, selftest
├── internal/{config,store,web,img,geo,status,jobs}/
├── migrations/0001_init.sql
├── web/{templates,static/{css,js,vendor,icons}}/
├── testdata/{photos,schema.golden.sql,rendered/}/
├── scripts/{worktree-new.sh,worktree-rm.sh,restore.sh,smoke.sh}
├── Makefile  mise.toml  package.json  playwright.config.ts  tsconfig.json
├── .golangci.yml  lefthook.yml  .gitleaks.toml  sqlc.yaml
├── Dockerfile  fly.toml  .env.example
├── .github/{workflows/ci.yml,CODEOWNERS}
└── README.md
```

The **holdout suite** lives **outside the repo**, in a private sibling repo `groveland-lights-holdout/`, cloned next to the main repo. Its location is resolved to an **absolute path** by the scheduler and stored in `harness/policy.json` (`holdout_dir`), because every harness agent runs from a worktree (`../wt/T-###`), where a relative `../holdout` would resolve to the wrong place (E1, §9.2). It holds additional acceptance tests that workers never see.

### 5.3 Dockerfile and fly.toml essentials

- **Dockerfile.** Build stage `golang:1.27.4-alpine`, with `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`. Runtime stage `alpine` (pin the minor version at scaffold time **[decision]**) with `ca-certificates` and `tzdata`, running as a non-root user. Target image size under **40 MB** (E14) — 25 MB is unrealistic once modernc sqlite and the AWS SDK are in, and Fly pricing doesn't care.
- **fly.toml:**
  - `primary_region = "ewr"`
  - `[http_service]`: `internal_port = 8080`, `force_https = true`, `auto_stop_machines = "off"`, `auto_start_machines = false`
  - `[[http_service.checks]]` on `/healthz` (interval 15s, timeout 2s, grace_period 10s)
  - `[mounts]`: `source="data"`, `destination="/data"`. (Snapshot retention is configured on the **volume at creation time** — §11 H4 `--snapshot-retention 14`; `snapshot_retention`/`scheduled_snapshots` as fly.toml `[mounts]` keys are unverified and are not used, E9.)
  - `[[vm]]`: `memory="512mb"`, `cpu_kind="shared"`, `cpus=1`

### 5.4 Logging

`log/slog`: JSON in prod, text locally. Every request logs method, route pattern (not the raw path with IDs), status, duration and request ID. No IPs, emails or coordinates in logs. A test captures log output and greps for an injected email and IP to prove they aren't written.

---

## 6. Phase 0: Harness bootstrap

**Goal.** Before any product code, the repository enforces the method on its own. Every later phase runs through it, so nobody has to remember the rules:
- acceptance specs are written first and protected;
- workers are isolated in worktrees and confined to the files they own;
- review happens in a clean context;
- validation is black-box and vision-checked;
- merges go through a serial gate;
- a judge signs off each milestone.

### 6.1 Harness design

#### 6.1.1 Roles and models (by name)

| Role (agent file) | Model | Vision needed? | What it does | Never does |
|---|---|---|---|---|
| `planner` | **GLM 5.3** | No | Expands a phase into the contract delta and task packets; rewrites `harness/state.md`; turns findings into fix tasks | Write app code or tests |
| `test-author` | **GLM 5.3** | No | Writes acceptance tests (Go black-box + Playwright) and holdout tests **before** features; updates `protected.sha256` | Write app code |
| `worker` | **GLM 5.3 Flash** (default); **DeepSeek** for overflow or retry | Optional | Implements one task packet in its worktree, within `owns`; writes unit tests for its own code | Touch `acceptance/`, `harness/` (except its handoff), `.opencode/`, CI, Makefile |
| `ui-worker` | **GLM 5.3 Flash**; **DeepSeek** as alternate | **Yes**: looks at its own Playwright screenshots | Same as worker, for templates, CSS, Alpine and map code | Same as worker |
| `reviewer` | **GLM 5.3** | No | Clean-context diff review: correctness against the packet and contract, security, architecture | Edit anything |
| `reviewer-alt` | **DeepSeek** | No | Second lens from a different model family: test integrity and reward hacking, duplication | Edit anything |
| `visual-reviewer` | **GLM 5.3 Flash**; **DeepSeek** as alternate | **Yes** | Reviews Playwright screenshots, trace stills and map renders against the task's visual acceptance notes; returns a text verdict | Edit anything. Never assigned to GLM 5.3. |
| `validator` | **GLM 5.3 Flash** | **Yes**: reads screenshots and traces | Runs `make validate MILESTONE=…` black-box on the merged candidate and writes findings per assertion | Fix anything |
| `judge` | **GLM 5.3** | No (uses the visual-reviewer's text verdicts) | Decides whether a milestone is done, defaulting to "not done" | Edit anything |
| `integrator` | **GLM 5.3** | No | Resolves rebase conflicts in the gate, only inside the task's `owns` | Edit protected paths |

**Routing rules,** enforced by `harness verify-agents` and the scheduler:
1. Any role marked *vision needed* must resolve to **GLM 5.3 Flash** or **DeepSeek**, never **GLM 5.3**, which has no vision. Per Z.ai docs, GLM 5.3 takes text-only input and GLM 5.3 Flash takes image, video, text and file input. Per DeepSeek's pricing page, the Flash model supports vision; V4 Pro does not.
2. **Escalation is rule-based** (methodology §5.2):
   - **Three failed attempts:** a non-UI task is retried on **GLM 5.3**, and a UI task on **DeepSeek**, which keeps vision.
   - **Touching files outside `owns`, or a merge conflict:** `BLOCKED`, handed back to the planner.
3. Model **IDs** are never written by hand. The bootstrap runs `opencode models`, matches the three names **provider-scoped** (bare names are ambiguous: openrouter, fireworks and the built-in `opencode` provider also carry GLM 5.3-family and DeepSeek-family entries), and writes the mapping to `harness/policy.json` and to each agent's `model:` line. **Expected IDs, verified in the execution environment on 2026-11-12:** `zai-coding-plan/glm-5.3`, `zai-coding-plan/glm-5.3-flash`, `deepseek/deepseek-flash`. If a name doesn't match exactly one ID within the expected provider, bootstrap stops and asks you. Additionally, a session stores its selected model separately from agent selection, so the scheduler asserts **at runtime** which provider/model actually served each run from the JSON transcript, and stops if it differs from `policy.json` (§6.1.4, E13).

#### 6.1.2 Permissions: what I verified on OpenCode v2.2.7

I installed OpenCode v2.2.7 on a scratch box and ran the following against a test agent. The model was OpenCode's built-in default, not your providers. That doesn't matter: permission enforcement is client-side. **Re-verified in the execution environment on 2026-11-12:** all `opencode run` flags used in §6.1.4 exist in `--help`; the V2 permission actions, the `{action, resource, effect}` schema and last-match-wins are confirmed against opencode.ai/docs/permissions; and every agent's built-in base policy is allow-all with agent rules appended last — so the deny-all baseline below genuinely overrides it.

- Agent files go in `.opencode/agents/*.md`, using the V2 frontmatter fields `description`, `mode`, `model`, `steps` and `permissions`. Check what loaded with `opencode debug agents` — it emits **machine-readable JSON** (verified; no fallback parsing needed), so `verify-agents` consumes it directly.
- Agents the scheduler starts with `opencode run --agent <name>` use `mode: all`, so they can run as the primary agent.
- **An `edit: deny` rule alone is not containment.** With `shell` left at the default `allow`, the agent wrote the file with `printf … > file`. With `shell * deny`, it used the **`subagent`** tool to ask the built-in `general` agent (full permissions) to write the file, and that worked. With `subagent` denied and only `git diff*` allowed, it created a file through `git diff --stat --output=hello.txt`. It also reached a **Code Mode `execute`** tool.
- **Conclusion:** every harness agent starts with a deny-all rule, allows a short list of specific actions, and explicitly denies `subagent`, `execute`, `external_directory`, `webfetch`, `websearch` and `question` at the end, because the last matching rule wins. Even then, **permissions are a speed bump, not the enforcement boundary.** The real enforcement is the post-run guard and the merge gate, which check the worktree's diff against protected paths, ownership and checksums. This is the methodology's "defense in depth" in practice.

Example (`.opencode/agents/worker.md`, V2 schema):

```markdown
---
description: Implements exactly one task packet inside its owned paths. Used by the scheduler only.
mode: all
model: <GLM 5.3 Flash id from harness/policy.json>
steps: 200
permissions:
  - { action: "*",        resource: "*",                    effect: "deny"  }
  - { action: "read",     resource: "*",                    effect: "allow" }
  - { action: "glob",     resource: "*",                    effect: "allow" }
  - { action: "grep",     resource: "*",                    effect: "allow" }
  - { action: "edit",     resource: "cmd/server/**",        effect: "allow" }
  - { action: "edit",     resource: "internal/**",          effect: "allow" }
  - { action: "edit",     resource: "web/**",               effect: "allow" }
  - { action: "edit",     resource: "migrations/**",        effect: "allow" }
  - { action: "edit",     resource: "harness/handoffs/**",  effect: "allow" }
  - { action: "shell",    resource: "make check-fast*",     effect: "allow" }
  - { action: "shell",    resource: "make test-unit*",      effect: "allow" }
  - { action: "shell",    resource: "make e2e-task*",       effect: "allow" }
  - { action: "shell",    resource: "go test *",            effect: "allow" }
  - { action: "shell",    resource: "go build *",           effect: "allow" }
  - { action: "shell",    resource: "git status*",          effect: "allow" }
  - { action: "shell",    resource: "git diff*",            effect: "allow" }
  - { action: "shell",    resource: "git add *",            effect: "allow" }
  - { action: "shell",    resource: "git commit -m *",      effect: "allow" }
  - { action: "shell",    resource: "*--no-verify*",        effect: "deny"  }
  - { action: "edit",     resource: "acceptance/**",        effect: "deny"  }
  - { action: "edit",     resource: "**/*.golden*",         effect: "deny"  }
  - { action: "subagent", resource: "*",                    effect: "deny"  }
  - { action: "execute",  resource: "*",                    effect: "deny"  }
  - { action: "external_directory", resource: "*",          effect: "deny"  }
  - { action: "webfetch", resource: "*",                    effect: "deny"  }
  - { action: "websearch",resource: "*",                    effect: "deny"  }
  - { action: "question", resource: "*",                    effect: "deny"  }
---
(worker prompt, §6.2 step 5)
```

The scheduler runs agents with `--auto`, which approves anything not explicitly denied. That's why the deny-all baseline is mandatory.

**Read-only roles** (reviewer, reviewer-alt, visual-reviewer, judge, planner) have no `edit` allow except their own output folder: `harness/reviews/**`, `harness/findings/**` or `harness/tasks/**` for the planner. Their only shell allows are `git diff*`, `git log*` and `git show*`.

The **validator** gets `make validate*` and `make holdout*` only, and its edit allow is `harness/findings/**`. It isn't given `external_directory`. The `make holdout` target reads `$(HOLDOUT_DIR)` (absolute, from `harness/policy.json`) itself, and the validator only sees the report.

#### 6.1.3 Task graph (`harness/tasks/graph.json`)

```jsonc
{
  "milestones": [{ "id": "M2", "title": "Admin inventory", "assertions": ["A-201","A-202"] }],
  "tasks": [{
    "id": "T-202",
    "milestone": "M2",
    "type": "feature",              // feature | acceptance | fix | chore | infra
    "role": "worker",               // worker | ui-worker | test-author | integrator
    "title": "Photo pipeline: decode, orient, resize, strip, store",
    "deps": ["T-200"],
    "owns": ["internal/img/**"],
    "claims": ["A-206","A-207"],    // contract assertions this task must turn green
    "ui": false,                    // true → visual-reviewer runs
    "allow_deps": ["golang.org/x/image"],
    "status": "ready",              // ready|running|review|rework|merging|done|blocked
    "attempts": 0,
    "budget": { "steps": 200, "wall_min": 60 }
  }]
}
```

Invariants, enforced by `harness` and its tests:
- Only the scheduler changes `status`.
- Two tasks with overlapping `owns` globs never run at the same time.
- A `feature` task can only become `ready` after the `acceptance` task for its claims is `done`. That's the "contract first" rule.
- Every assertion in `acceptance/contract.json` is claimed by at least one task. Assertions may carry `"gate": false` (a **recorded target**, e.g. A-309 Lighthouse: needs an owner task and a recorded number, but no green test gates the merge) or `"verified": "human"` (a **field check**, e.g. A-605 seed coverage: verified by Anthony and recorded by the judge from evidence like the A-603 export). Neither class requires an automated test; both still require an owner (E5).

#### 6.1.4 Scheduler (`go run ./cmd/harness sched`)

This is plain code, not an LLM.

`opencode run` flags I confirmed in v2.2.7's `--help` (and re-confirmed in the execution environment): `--agent`, `--model provider/model#variant`, `--format json`, `--file`, `--title`, `--archived`, `--auto`, `--worktree`, `--session`, `--continue`, `--fork`, `--standalone`. The scheduler manages its own worktrees instead of using `--worktree`, so branch names, locations and cleanup stay deterministic **[decision]**.

```
start-up: resolve the holdout path absolutely → policy.json (holdout_dir; exported to Make as HOLDOUT_DIR)  (E1)

loop until no ready/running/review/merging tasks remain or `harness/STOP` exists:
  precondition, checked before every claim AND every gate run: `git -C <main> status --porcelain` is empty.
    A dirty main means an agent reached the main checkout — stop the world, report, ask Anthony.  (E4)
  before creating any worktree: commit harness/tasks/graph.json and any new task packets on main
    (worktrees branch from main; uncommitted packets make --file unresolvable inside the worktree)  (E9)
  pick ready tasks: deps done ∧ no owns-overlap with running|merging ∧ running < policy.max_parallel (default 2)
  for each: claim → scripts/worktree-new.sh T-### (../wt/T-###, branch task/T-###, from main,
              symlinks node_modules from the main repo — without it, npx fetches unpinned latest)  (E2)
            spawn in that dir:
              opencode run --standalone --auto --agent <role> --format json --archived \
                --title T-### --file harness/tasks/T-###.md \
                "Execute the attached task packet. Follow AGENTS.md. Write harness/handoffs/T-###.md."
            stream JSON → harness/runs/<ts>/T-###/<role>.jsonl ; kill at budget.wall_min
            on exit: assert the provider/model that actually served the run (read from the
            transcript) matches policy.json; mismatch → stop the world  (E13)
  on worker exit:
    harness guard T-###            (§6.1.5)   → fail: rework (attempts++) or blocked
    make check-fast (in worktree)              → fail: rework
    reviews in parallel: reviewer, reviewer-alt, (+ visual-reviewer if ui, with screenshots via --file)
      each writes harness/reviews/T-###-<role>.md ending in "VERDICT: PASS" or "VERDICT: BLOCK"
    any BLOCK → append findings to packet "## Review findings", status rework
    all PASS → enqueue for gate
  gate (serial, flock harness/.merge.lock): harness gate T-###   (§6.1.6)
  milestone complete → validator → visual-reviewer on validator screenshots → judge
    judge "NOT DONE" → planner turns findings into fix tasks
```

#### 6.1.5 Guard (`go run ./cmd/harness guard T-###`)

Exit code is non-zero, with one `ERROR:` line per violation (grep-friendly, per the methodology), when:

1. **Protected path touched** (exit 3): any path in `git diff --name-only main...HEAD` matches `policy.protected`. That covers `acceptance/**`, `harness/policy.json`, `harness/protected.sha256`, `harness/tasks/graph.json`, `.opencode/**`, `AGENTS.md`, `Makefile`, `.golangci.yml`, `lefthook.yml`, `.github/**`, `cmd/harness/**`, `testdata/**/*.golden*` and `acceptance/e2e/**/__screenshots__/**`. Exception: tasks of `type: acceptance`, run by `test-author`, may touch `acceptance/**` and the manifest.
2. **Checksum mismatch** (exit 3): `sha256sum -c harness/protected.sha256` fails. This catches changes made outside git's view, such as an untracked edit in the worktree.
3. **Ownership** (exit 4): a changed file isn't covered by `task.owns ∪ {harness/handoffs/T-###.md}`.
4. **Test ratchet** (exit 5):
   - the number of Go tests (`go test -list '.*' ./...`) or Playwright tests (`npx playwright test --list`) is lower than on `main`;
   - coverage of owned packages falls below `policy.coverage_floor` (default 70%, raised per milestone);
   - or a new `t.Skip(`, `test.skip(`, `test.only(` or `.fixme(` appears.
5. **Forbidden markers in added lines** (exit 6): `TODO`, `FIXME`, `panic("not implemented")`, `//nolint` without a reason, or `console.log` in `web/static/js`.
6. **Dependencies** (exit 7): `go.mod` or `package.json` gains a module that isn't in `task.allow_deps`.
7. **Handoff missing** (exit 8), or **no commit on the branch**.
8. **Special-casing heuristic** (warning only, sent to `reviewer-alt`): added non-test code contains string literals that appear in acceptance fixtures.
9. **Worktree not clean** (exit 9, E3): `git status --porcelain` in the worktree is non-empty, ignoring `harness/handoffs/`, `harness/runs/`, `node_modules` and `.env.test`. Everything a task needs must be committed — an uncommitted edit to `Makefile`, `.golangci.yml` or fixtures could otherwise weaken the gate's `make check` verdict without ever reaching the diff or checksum checks.

#### 6.1.6 Merge gate (`go run ./cmd/harness gate T-###`)

1. Take the merge lock.
2. Rebase `task/T-###` onto `main`. On conflict, run the `integrator` agent in the worktree, limited to conflicted files that are inside `owns`. If that fails, the task goes to `blocked`.
3. Re-run `harness guard` (including the clean-worktree check, exit 9).
4. Run full `make check` (§6.1.8) in the worktree. **Flake policy (E10, §10 #15):** if the failure is in a Playwright target (`e2e`, `a11y`, `visual`), re-run that target **once**; a pass on retry is logged as `flake T-### <target>` in `progress.log` and the gate proceeds. A second failure fails the gate. Non-Playwright targets have no retry.
5. Run `make holdout` if `$(HOLDOUT_DIR)` exists (the absolute path from `harness/policy.json`, never a relative `../holdout` — agents and the gate run from worktrees, E1).
6. `git merge --ff-only task/T-###` into `main`.
7. Push `main` to origin. The script pushes; agents never do.
8. Append `merged T-### <sha>` to `progress.log`, then remove the worktree.

Any failure leaves `main` unchanged. The self-test proves this with `git rev-parse main` before and after.

#### 6.1.7 Hooks, CODEOWNERS, CI

- **lefthook.yml:**
  - `pre-commit`: gofmt check on staged Go files, `golangci-lint` on changed packages, and a gitleaks scan of staged changes.
  - `commit-msg`: Conventional Commits regex.
  - `pre-push`: `make check-fast` and `sha256sum -c harness/protected.sha256`.
  - Hooks speed agents up but can be bypassed. The guard and gate re-run everything, and `--no-verify` is denied in agent permissions.
- **CODEOWNERS:** `/acceptance/ /harness/ /.opencode/ /.github/ /Makefile @amscotti`. With branch protection (a human step), this makes GitHub refuse PRs from outside the harness that touch those paths without your review. The local gate pushes as you, so CODEOWNERS is for external PRs, not the harness.
- **GitHub Actions `ci.yml`:** on `push` and `pull_request`:
  1. `jdx/mise-action@v5` installs the pinned tools.
  2. `npm ci` — mise doesn't install `package.json` devDeps, and without `node_modules`, `npx` silently fetches unpinned `latest` from the registry (E2).
  3. `npx playwright install --with-deps chromium webkit`. This must precede `make ci`, because `check` (inside `ci`) already runs the e2e targets, which need browsers installed (v2 ran this after; fixed in v2.1).
  4. `make ci` (it already includes `go build` and `docker build`; v2's separate `make e2e a11y visual` and `docker build` steps were redundant with it).
  5. Upload `playwright-report/` and traces with `actions/upload-artifact@v7` on failure.

  The deploy job is `workflow_dispatch` only, uses the `FLY_API_TOKEN` repo secret (a human step), and runs `flyctl deploy` then `make smoke BASE_URL=…`. **CI runs the same Makefile targets as local, never a parallel definition.**
- **Hook activation:** `lefthook install` is run once by the bootstrap (step 10) after `lefthook.yml` exists; hooks in the repo do nothing until then (E9).

#### 6.1.8 Makefile targets (the gate definitions)

| Target | Runs | Used by |
|---|---|---|
| `doctor` | Checks every tool version against `mise.toml` and `package.json` pins | Phase 0 DoD |
| `fmt` / `fmt-check` | `gofmt -l`, `go mod tidy -diff` | hooks, check |
| `vet` | `go vet ./...` | check-fast |
| `lint` | `golangci-lint run ./...` (v2 config: errcheck, govet, staticcheck, ineffassign, unused, bodyclose, sqlclosecheck, gosec, exhaustive, cyclop ≤ 15, errorlint, noctx) | check-fast |
| `generate-check` | `sqlc generate` + `git diff --exit-code` | check |
| `test-unit` | `go test -race -count=1 -shuffle=on ./...` | check-fast |
| `cover` | Coverage per package against `policy.coverage_floor` | check |
| `vuln` | `govulncheck ./...` | check |
| `templates-check` | Go test renders every template with fixtures into `testdata/rendered/`, then `npx html-validate testdata/rendered` | check |
| `js-check` | `npx tsc --noEmit -p tsconfig.json` (`checkJs` on `web/static/js`, plus specs) | check |
| `test-acceptance-go` | Builds the binary, starts it on a random port with a temp DB and photo dir, runs `go test -tags acceptance ./acceptance/go/...` | check |
| `e2e` | `npx playwright test` (projects: desktop-chromium, pixel-9, iphone-16 on WebKit, iphone-se-320) | check |
| `e2e-task T=…` | Only the specs tagged with the task's claims (uses the repo's pinned Playwright via the `node_modules` symlink that `worktree-new.sh` creates — never a bare `npx` fetch, E2) | workers |
| `a11y` | `npx playwright test --grep @a11y` (axe, WCAG 2.2 A/AA tags, zero violations) | check |
| `visual` | `npx playwright test --grep @visual` (`toHaveScreenshot`, map canvas masked) | check |
| `secrets` | gitleaks over the repo | check |
| `phase-grep` | `! grep -rnE '\bPhase [0-9]' cmd/server internal web migrations` | check |
| `check-fast` | fmt-check vet lint test-unit | workers, pre-push |
| `check` | check-fast + generate-check cover vuln templates-check js-check test-acceptance-go e2e a11y visual secrets phase-grep | gate |
| `ci` | check + `go build` + `docker build` | CI |
| `holdout` | Runs the holdout suites (Go + Playwright) at `$(HOLDOUT_DIR)` — the absolute path from `harness/policy.json`, never a relative `../holdout` (E1) — against a freshly built binary | gate, validator |
| `validate MILESTONE=…` | All `@A-###` specs for the milestone on all projects, with traces and screenshots kept; writes `harness/runs/<ts>/validate/` | validator |
| `lighthouse` | Optional `lhci autorun` with budgets | Phase 3+ |
| `smoke BASE_URL=…` | `npx playwright test --grep @smoke` against a deployed URL | after deploy |
| `drill-local` | Backup → restore → compare counts and checksums (§9.7) | Phase 5 |
| `plan PHASE=n` | `opencode run --agent planner …` with this plan and `state.md` | you |
| `run` / `status` / `stop` | Scheduler start, a summary from `graph.json` + `progress.log`, and `touch harness/STOP` | you |
| `harness-verify` | `harness verify-agents` + `go test ./cmd/harness/...` + `harness selftest` | Phase 0 DoD |

### 6.2 The Phase 0 paste block

Open OpenCode v2 in an empty repo with this plan saved as `docs/PLAN.md`, select the default `build` agent on **GLM 5.3**, and paste the block below. Stay at the keyboard. Bootstrap is interactive on purpose, because the harness can't police its own creation.

```text
You are bootstrapping the "Groveland Lights" repository so that it enforces a strict
multi-agent workflow BEFORE any product code exists. docs/PLAN.md is the full plan; §3–§6
and §8–§9 are normative. Work in this order, commit after each step with a Conventional
Commit message, and STOP and ask me whenever a step says ASK or a check fails twice.
PAUSE for my review after step 5 and after step 12 — do not continue until I say so.

1. TOOLCHAIN. Create mise.toml pinning: go 1.27.4, node (the current Active LTS major — check `mise ls-remote node`, ASK if unsure), golangci-lint 2.15.0,
   lefthook 2.2.2, gitleaks 8.30.1, sqlc 1.31.1, flyctl 0.5.8. Create package.json
   (private, type: module) pinning devDependencies exactly: @playwright/test 1.65.0,
   @axe-core/playwright 4.14.0, html-validate 11.21.0, typescript 7.0.2,
   @lhci/cli 0.16.1. go.mod: module github.com/amscotti/groveland-lights, go 1.27,
   toolchain go1.27.4, and a `tool golang.org/x/vuln/cmd/govulncheck` directive at v1.9.0.
   If any pinned version fails to resolve, ASK — do not substitute silently.

2. MODELS. Run `opencode models`. Match each name WITHIN ITS PROVIDER — bare names are
   ambiguous (openrouter, fireworks and the built-in opencode provider also carry
   GLM-5.3-family and DeepSeek-family entries). Expected, verified in this environment
   on 2026-11-12: "GLM 5.3" → zai-coding-plan/glm-5.3 (text-only), "GLM 5.3 Flash" →
   zai-coding-plan/glm-5.3-flash (vision), "DeepSeek" → deepseek/deepseek-flash (vision).
   If any expected ID no longer resolves, ASK. Write harness/policy.json with
   {"models":{"glm":"<id>","glm_flash":"<id>","deepseek":"<id>"},
   "vision_models":["glm_flash","deepseek"], "max_parallel":2, "coverage_floor":70,
   "holdout_dir":"<ABSOLUTE path to the holdout clone, resolved from the main repo>",
   "protected":[…§6.1.5 list…]}. If any name is ambiguous within its provider, ASK.

3. AGENTS.md (≤120 lines): one-paragraph product summary; repo map (§5.2); the commands
   (make check-fast, make e2e-task T=…, make check); hard rules: never edit acceptance/,
   harness/ (except your handoff), .opencode/, Makefile, CI, golden files; never weaken or
   skip a test; never add a dependency not listed in the task packet; HTMX never swaps
   #map; no "Phase N" strings in code; parse inputs at the boundary into typed structs;
   every handler has a table test; every UI change has a Playwright spec with mobile
   projects; write harness/handoffs/T-###.md (what changed, how verified, commands run
   with exit codes, open issues) before finishing. Point to docs/spec/* for detail.

4. AGENTS. Create .opencode/agents/{planner,test-author,worker,ui-worker,reviewer,
   reviewer-alt,visual-reviewer,validator,judge,integrator}.md using the V2 frontmatter
   (description, mode: all, model, steps, permissions). Models per §6.1.1, IDs from
   policy.json. EVERY agent's permissions start with {action:"*",resource:"*",effect:"deny"}
   and END with explicit denies for subagent, execute, external_directory, webfetch,
   websearch, question (last match wins). Allow lists per §6.1.2. Prompts:
   - worker/ui-worker: "Read AGENTS.md and the attached packet. Run the claimed acceptance
     specs first and confirm they fail. Implement only within `owns`. Add unit tests. Run
     make check-fast and make e2e-task T=<id> until green. ui-worker: open the screenshots
     in test-results/ and compare against the packet's visual notes. Write the handoff. If
     blocked or the spec seems wrong, write BLOCKED: <reason> in the handoff and stop —
     never edit the spec."
   - reviewer / reviewer-alt: "You did not write this code. Review the attached diff
     against the packet and contract. reviewer: correctness, security (CSRF, XSS via
     template.HTML, SQL, path traversal, upload limits), architecture (§3.3, §5.2).
     reviewer-alt: test integrity — weakened/deleted/skipped tests, special-casing of
     fixture values, mocks that bypass the behaviour, coverage theatre. Output findings
     with file:line, then exactly one line `VERDICT: PASS` or `VERDICT: BLOCK`."
   - visual-reviewer: "Inspect every attached screenshot. Check: layout at 320px and
     mobile viewports, tap targets ≥44px, contrast, status colours plus text labels,
     map markers visible and not overlapping UI, attribution visible, no clipped text.
     Output findings then `VERDICT: PASS|BLOCK`."
   - validator: "Run make validate MILESTONE=<id> and make holdout. For each assertion
     A-### report PASS/FAIL with evidence (test name, screenshot path, trace path). Open
     failure screenshots and describe what is wrong. You never fix anything."
   - judge: "Given contract.json, the validator report, visual verdicts and reviews,
     decide DONE or NOT DONE for the milestone. Default is NOT DONE unless every
     assertion has passing evidence. List the blocking gaps."
   - planner: "Given docs/PLAN.md, harness/state.md and findings, produce for phase N:
     contract delta (A-### items in WHEN/SHALL form), one `acceptance` task for
     test-author, then feature tasks with disjoint `owns`, deps, claims, ui flag,
     allow_deps. Rewrite harness/state.md (≤60 lines). Never write code."
   - test-author: "Write the acceptance tests for the listed assertions under
     acceptance/go and acceptance/e2e, tagged @A-###; also write 1–2 holdout variants
     per assertion into the holdout dir (the ABSOLUTE path from harness/policy.json
     holdout_dir; granted to this agent only — never a relative path, you run from a
     worktree). Tests must fail against the current code. Regenerate harness/protected.sha256."
   - integrator: "Resolve the rebase conflicts in the listed files only, preserving both
     sides' intent; run make check-fast; if not cleanly possible, write BLOCKED."
   test-author is the only agent with an allow rule for external_directory, scoped to the
   holdout path; ASK me to confirm that path.

5. HARNESS CLI. Implement cmd/harness (Go, stdlib only) with subcommands:
   sched [--dry-run], guard T, gate T, status, verify-agents, selftest [--live].
   Behaviour exactly per §6.1.3–§6.1.6, including:
   - sched: resolve the holdout dir ABSOLUTELY at startup (policy.json holdout_dir,
     exported to Make as HOLDOUT_DIR); assert `git status --porcelain` on the main
     checkout is empty before every claim and every gate run (stop the world if not);
     commit graph.json + new task packets to main BEFORE creating any worktree;
     after each agent run, assert the provider/model that served it (read from the
     JSON transcript) matches policy.json, else stop the world.
   - guard: checks 1–9, including the clean-worktree check (exit 9).
   - gate: flake policy — one retry of a failed Playwright target, logged as
     `flake T-### <target>`; second failure fails the gate.
   Unit tests for: glob overlap detection, DAG readiness and cycle detection,
   contract-first readiness rule, ratchet comparison, checksum verification, verdict
   parsing, ownership matching, clean-worktree detection, served-model assertion.
   verify-agents parses `opencode debug agents` output — it is machine-readable JSON
   (verified; parse it directly, no fallback) — and fails if any agent lacks the
   deny-all baseline or the six trailing denies, or if a vision role resolves to a
   non-vision model.

6. TASK GRAPH + PROGRESS. Create harness/tasks/graph.json (schema §6.1.3), empty
   harness/progress.log, harness/state.md, harness/{handoffs,reviews,findings,runs}/
   with .gitkeep; gitignore harness/runs/.

7. WORKTREES. scripts/worktree-new.sh T (git worktree add ../wt/T -b task/T main;
   copies .env.test; symlinks node_modules from the main repo — or runs npm ci if the
   symlink fails — so npx/Playwright use the PINNED versions, never registry latest)
   and scripts/worktree-rm.sh T. Idempotent; shellcheck-clean.

8. APP SKELETON (minimum to make gates meaningful): cmd/server with GET /healthz
   ("ok"), GET / rendering a base template (Pico, htmx, Alpine CSP build vendored into
   web/static/vendor with versions in filenames; npm packages are @picocss/pico,
   htmx.org — NOT the stale `htmx` squatter package — @alpinejs/csp, maplibre-gl),
   CSP header per §3.4, slog, graceful shutdown, config parsing with typed struct and
   table tests. No product features.

9. ACCEPTANCE SCAFFOLD. acceptance/contract.json with A-000 ("WHEN GET /healthz THE
   SYSTEM SHALL return 200 ok"), acceptance/go/healthz_test.go (build tag acceptance,
   BASE_URL from env), acceptance/e2e/home.spec.ts (@A-000 @smoke @a11y: loads /,
   no console errors, no CSP violations, axe zero violations with tags
   wcag2a,wcag2aa,wcag21a,wcag21aa,wcag22aa). playwright.config.ts: webServer starts the
   built binary with APP_ENV=test on a random port and temp dirs; projects
   desktop-chromium, pixel-9 (devices['Pixel 9']), iphone-16 (devices['iPhone 16'],
   WebKit), iphone-se-320 (Chromium, viewport 320×640, isMobile, hasTouch);
   trace 'retain-on-failure', screenshot 'only-on-failure', video 'retain-on-failure';
   expect.toHaveScreenshot {maxDiffPixelRatio: 0.01, animations: 'disabled'}.
   Generate harness/protected.sha256.

10. QUALITY CONFIG. .golangci.yml (version: "2", linters per §6.1.8), tsconfig.json
    (checkJs, strict, noEmit), .html-validate.json (recommended), .gitleaks.toml
    (default + allowlist for Turnstile test keys), lefthook.yml per §6.1.7 — then run
    `lefthook install` so the hooks actually activate — Makefile with
    every target in §6.1.8 (targets that need later phases exist and print "n/a until
    Phase N" with exit 0 — EXCEPT check, which must run everything that exists).

11. CI. .github/workflows/ci.yml per §6.1.7; .github/CODEOWNERS.

12. SELF-TEST FIXTURES. harness/selftest/ with a dummy task T-000 ("GET /version returns
    JSON {version, commit}") whose acceptance spec A-900 is written by a scripted
    test-author step, plus deterministic fake-worker scripts (plain shell, no LLM) for:
    tamper-spec, out-of-ownership, delete-test, red-check, overlapping-owns, and
    dirty-worktree (guard must exit 9). `harness selftest` runs these in a throwaway
    clone under $TMPDIR and asserts the expected exit codes and that main is unchanged;
    `--live` additionally runs the real T-000 through planner-less flow
    worker→reviewers→gate with models, and a permission probe in which the worker is
    told to (a) edit acceptance/, (b) start a subagent, and (c) edit
    <main-repo>/cmd/harness/sched.go from inside its worktree — the last one probes
    whether a linked-worktree session treats the main checkout as internal. For (c),
    denied permission events OR a subsequent clean-main assertion failure reported
    loudly both count as "gap detected, main protected"; silent success editing main
    is a selftest FAILURE that must be reported to me, not papered over.

13. Run the Phase 0 DoD (docs/PLAN.md §6.3) and paste me every command with its exit
    code and the last 20 lines of output. Do not claim done without that evidence.
```

### 6.3 Phase 0 Definition of Done

Each command must succeed. Paste the evidence into `harness/handoffs/P0.md`.

| # | Command | Must show |
|---|---|---|
| 1 | `mise install && npm ci && make doctor` | Every tool `OK` at its pinned version |
| 2 | `make harness-verify` | `verify-agents`: 10 agents, all with deny-all baseline and trailing denies; vision roles on GLM 5.3 Flash or DeepSeek. `go test ./cmd/harness/...` passes. Selftest output below. |
| 3 | `go run ./cmd/harness selftest` | **S2 tamper:** guard exits 3 with `ERROR: protected path acceptance/e2e/version.spec.ts` and checksum mismatch. **S3 out of scope:** exit 4. **S4 deleted test:** exit 5. **S5 red check:** gate refuses and `git rev-parse main` is unchanged. **S7 overlap:** dry-run schedule never runs the two overlapping tasks together. **S9 dirty worktree:** an uncommitted edit to `Makefile` in the worktree makes guard exit 9. Prints `selftest: 6/6 deterministic scenarios passed`. |
| 4 | `go run ./cmd/harness selftest --live` | **S1 happy path:** `progress.log` shows `claimed T-000 → worker_done → guard_ok → check_fast_ok → review_pass(reviewer) → review_pass(reviewer-alt) → gate_ok → merged <sha>`, and `curl localhost:$PORT/version` returns JSON. **S6 permission probe:** the worker is told to edit `acceptance/`, to start a subagent, and to edit `<main>/cmd/harness/sched.go` from its worktree; its JSON transcript shows denied permission events for the first two, and either a denied event or a loud clean-main assertion failure for the third — a silent successful edit of main is a selftest failure (E4). **S8 holdout:** the worker can't read the holdout dir (absolute path from `policy.json`). |
| 5 | `make check` | Exit 0 on the skeleton. All four Playwright projects pass; axe finds zero violations. |
| 6 | `git commit` with a staged fake AWS key, then with a badly formatted `.go` file | Both blocked by lefthook |
| 7 | Push to GitHub | CI green. A branch with a deliberately failing test goes red, and the Playwright report artifact is uploaded. |
| 8 | Judge | `opencode run --agent judge --file harness/handoffs/P0.md "Milestone P0"` → `DONE` |

**Parallel versus serial:** Phase 0 is entirely serial and interactive. Do not run the scheduler until row 4 passes.

---

## 7. Product phases as task graphs

**How each phase runs:**
1. `make plan PHASE=n`. The planner writes the contract delta and packets, using the task tables below as a starting point.
2. You skim `contract.json` (2 minutes; it's the one review that pays off most).
3. `make run` executes the phase.
4. The phase ends with validator → visual-reviewer → judge.

**Common rules for every task table:**
- **IDs:** T-n0x is the phase's acceptance task. It's always first and always run by `test-author`.
- **"‖" means the task can run in parallel** with the others marked ‖ in that phase, because their `owns` are disjoint. Unmarked tasks are serial.
- **Shared files** (`internal/web/routes.go`, `web/templates/base.html`, `migrations/`) are owned by **one** task per phase. Other tasks add their routes through `internal/web/<feature>.go` files that register with a `Register(mux)` function, so they never touch the shared file.

### Phase 1: Skeleton deployed (was v1 Phase 0)

**Goal:** the Phase 0 skeleton runs on Fly with a volume. A persistent counter proves the data survives a deploy.

**Contract (A-1xx):**
- A-101 `/healthz` returns 200 within 2 s.
- A-102 `/` renders with the CSP header and no console errors.
- A-103 `POST /probe` increments a counter stored in SQLite (CSRF-protected). The value survives a process restart (local) and a `fly deploy` (deployed smoke test).
- A-104 Startup applies migrations and refuses to boot when a migration checksum doesn't match.
- A-105 Requests are logged as JSON with route pattern, status and duration, and never with the IP.

| Task | Role | Owns | Deps | Claims |
|---|---|---|---|---|
| T-100 | test-author | `acceptance/**` | — | A-101..105 |
| T-101 | worker | `internal/store/**`, `migrations/**` | T-100 | A-103, A-104 |
| T-102 ‖ | worker | `internal/web/middleware*.go`, `internal/web/csrf*.go` | T-100 | A-102, A-105 |
| T-103 | worker | `cmd/server/**`, `internal/web/probe.go` | T-101, T-102 | A-103 |
| T-104 ‖ | worker | `Dockerfile`, `fly.toml`, `.dockerignore`, `scripts/smoke.sh` | T-100 | — (infra; checked by `docker build` and `make smoke`) |

**Tests written first:**
- **Go:** an acceptance test restarts the binary against the same `DB_PATH` and asserts the counter persists. A migration test edits a copy of an applied migration and expects a boot failure with a clear message. A log-capture test injects an `X-Forwarded-For` IP and asserts it's absent from the logs.
- **Playwright** `@smoke`: load `/`, click "probe", see the count increase, reload, see it persist.

**Human steps:** create the Fly app, volume and secrets (§11).

**DoD:**

```
make check
docker build -t gl:dev . && docker run --rm -e ... gl:dev   # /healthz 200
fly deploy --ha=false
make smoke BASE_URL=https://<app>.fly.dev                  # @smoke incl. counter persisted across this deploy
fly deploy --ha=false && make smoke BASE_URL=…             # second deploy: counter still increments from prior value
fly status   # exactly 1 machine, started
fly volumes list   # 1 volume, ewr, snapshot retention 14
```

**Judge input (Milestone M1):** CI link, smoke output from both deploys, and the `fly status` output.

### Phase 2: Admin inventory, "walk the town" (was v1 Phase 1)

**Goal:** you walk a park on your phone, stand at a fixture, tap "Add light", and confirm the pin. The photo is captured, oriented, resized and stripped of metadata. Duplicates are caught.

**Contract (A-2xx):**
- A-201 `GET /admin/login?token=` with a valid `ADMIN_TOKEN` sets an admin session and redirects (302) to `/admin`. Wrong or reused tokens get 403. Non-admins on `/admin/*` get 302 to login, and HTMX requests get 401 with `HX-Redirect`.
- A-202 The "walk mode" page asks for geolocation, shows live position and accuracy, and lets you save only when accuracy is ≤ 30 m **[decision: v1 had no threshold]**, or after an explicit "save anyway (low accuracy)" choice.
- A-203 You can drag the pin to adjust it before saving; the saved lat/lon equals the confirmed pin.
- A-204 Positions outside `TOWN_BBOX` are rejected with a message.
- A-205 Duplicates: the same `asset_no` within 25 m, or an unnumbered fixture within 8 m, triggers a "possible duplicate" panel with Merge or Save anyway.
- A-206 Photo upload: JPEG, PNG or WebP, ≤ 15 MB. HEIC gets 415 with a "please choose JPEG" message unless R6 is resolved otherwise. The server re-encodes to JPEG with a long edge ≤ 1600 px, applies EXIF orientation, and strips all metadata. The served photo has no EXIF GPS tag.
- A-207 Uploads over the size limit, of the wrong type, or decompression bombs (pixel count > 40 MP) are rejected with 413 or 415 and leave no stray file.
- A-208 Edit and soft delete; a deleted light disappears from lists and the map.
- A-209 The admin list shows the 50 most recent lights with thumbnails (HTMX pagination).

| Task | Role | Owns | Deps | Claims |
|---|---|---|---|---|
| T-200 | test-author | `acceptance/**`, `testdata/photos/**` | — | A-201..209 |
| T-201 | worker | `migrations/0002_*.sql`, `internal/store/lights*.go`, `internal/store/photos*.go` | T-200 | A-205, A-208 (store side) |
| T-202 ‖ | worker | `internal/img/**` | T-200 | A-206, A-207 |
| T-203 ‖ | worker | `internal/web/auth*.go`, `internal/web/session*.go` | T-200 | A-201 |
| T-204 ‖ | worker | `internal/geo/**` | T-200 | A-204, A-205 (distance and bbox logic) |
| T-205 | ui-worker | `internal/web/admin*.go`, `web/templates/admin/**`, `web/static/js/walk.js` | T-201..204 | A-202, A-203, A-208, A-209 |
| T-206 | worker | `internal/web/upload*.go` | T-202, T-201 | A-206, A-207 (HTTP side) |

**Acceptance tests written first:**
- **Fixtures** (`testdata/photos/`): a portrait JPEG with Orientation=6 and GPS tags; a 1×1 PNG; a 20 MB JPEG; a "pixel bomb" PNG (small file, 50k×50k header); a text file renamed `.jpg`; a HEIC sample **if** HEIC support is decided.
- **Go:**
  - `internal/img` tests check the output long edge is ≤ 1600 px, the orientation is applied (the output is wider than tall for the rotated portrait test image, checked against a known pixel), and no EXIF APP1 segment remains.
  - The store test for duplicate detection is table-driven and uses distances derived from the Haversine formula.
- **Playwright** (`acceptance/e2e/admin-walk.spec.ts`; projects pixel-9, iphone-16, iphone-se-320):

```ts
test.use({ geolocation: { latitude: 42.7617, longitude: -71.0303, accuracy: 12 },
           permissions: ['geolocation'] });           // Playwright BrowserContext options
test('@A-202 @A-203 add a light from current position', async ({ page, context }) => {
  await loginAsAdmin(page);
  await page.goto('/admin/walk');
  await expect(page.getByText(/accuracy 12 m/)).toBeVisible();
  await context.setGeolocation({ latitude: 42.76175, longitude: -71.03035, accuracy: 8 });  // walk a few metres
  await expect(page.getByText(/accuracy 8 m/)).toBeVisible();
  await page.getByRole('button', { name: 'Add light here' }).click();
  await page.getByLabel('Photo').setInputFiles('testdata/photos/portrait-orient6-gps.jpg');
  await page.getByLabel('Asset number').fill('GP-017');
  await page.getByRole('button', { name: 'Confirm pin' }).click();
  await expect(page.getByRole('status')).toContainText('Saved');
  // black-box check: served photo has no GPS EXIF, ≤1600px
  const src = await page.getByRole('img', { name: /GP-017/ }).getAttribute('src');
  const bytes = await (await page.request.get(src!)).body();
  expect(hasExifGps(bytes)).toBe(false);   // tiny helper in acceptance/e2e/util
});
```

The coordinates are illustrative points inside `TOWN_BBOX`, not a verified park location. Further specs:
- low accuracy (`accuracy: 80`) forces the "save anyway" path;
- an outside-bbox position is rejected;
- duplicate panel appears;
- denied geolocation permission (a context without `permissions`) shows the manual pin-drop fallback;
- `@a11y` axe runs on `/admin/walk` and `/admin`.

**Validation:**
- `make validate MILESTONE=M2` captures screenshots of walk mode on all mobile projects.
- The visual-reviewer checks that the "Add light here" button is reachable by thumb (bottom third), the accuracy text is readable, and the photo preview is oriented correctly.
- Holdout covers different coordinates and asset numbers, an Orientation=8 image, and a 16 MB file.

**DoD:**

```
make check && make holdout
make validate MILESTONE=M2      # validator report: A-201..209 PASS with evidence
exiftool -a -G1 <downloaded served photo>   # manual spot check: no GPS, no EXIF block   [optional tool]
```

Then the **field test (human):** add five real fixtures in one park on your phone. All five appear, with correct photos, within the right building or path.

**Parallel versus serial:** T-202, T-203 and T-204 are parallel. T-205 is the UI integration and runs serially after them.

### Phase 3: Public map (was v1 Phase 2)

**Goal:** anyone opens the site on a phone and sees every light, coloured by status, with clustering. Tapping one shows a detail sheet with a shareable `/l/<id>` link.

**Contract (A-3xx):**
- A-301 `/` shows a full-viewport map centred on Groveland, with OpenFreeMap attribution visible.
- A-302 `GET /lights.geojson` returns a FeatureCollection of non-deleted lights with `id`, `status`, `status_source`, `status_at`, `label` and `kind`. It sends `Cache-Control: public, max-age=60` and an ETag, and answers `If-None-Match` with 304.
- A-303 Markers are coloured by status. Below zoom 16 they cluster (MapLibre GeoJSON source `cluster: true`, `clusterMaxZoom: 15`). Each status has a text and shape cue as well as colour.
- A-304 Tapping a marker opens a bottom sheet (HTMX partial `/l/<id>/sheet`) with label, status sentence, photo and last 5 reports. The map instance is not recreated.
- A-305 `/l/<id>` is a full server-rendered page (works without JS) with `<meta property="og:*">` tags.
- A-306 A list view (`/list`) is an accessible, non-map alternative with the same data, filterable by status. Its whole flow is keyboard-operable.
- A-307 At 320 px wide nothing overflows horizontally, and tap targets are ≥ 44×44 CSS px.
- A-308 No CSP violations across all public pages.
- A-309 Performance target (`gate: false`, E5): Lighthouse mobile Performance ≥ 85 and LCP < 2.5 s on `/l/<id>`, recorded per release in `docs/perf.md`. Not a merge gate.

| Task | Role | Owns | Deps | Claims |
|---|---|---|---|---|
| T-300 | test-author | `acceptance/**`, `testdata/map/**` | — | A-301..309 |
| T-301 ‖ | worker | `internal/web/geojson*.go`, `internal/store/query_public*.go` | T-300 | A-302 |
| T-302 ‖ | ui-worker | `web/static/js/map.js`, `web/static/vendor/maplibre*`, `web/templates/index.html` | T-300 | A-301, A-303, A-308, A-309 (`gate: false` — records the Lighthouse number in `docs/perf.md`) |
| T-303 ‖ | ui-worker | `internal/web/light*.go`, `web/templates/light/**` | T-300 | A-304, A-305 |
| T-304 ‖ | ui-worker | `internal/web/list*.go`, `web/templates/list.html`, `web/static/css/app.css` | T-300 | A-306, A-307 |
| T-305 | ui-worker | `web/static/js/sheet.js`, `web/templates/base.html` | T-302, T-303 | A-304 (HTMX ↔ map integration) |

**Acceptance tests written first:**
- **Offline, deterministic map:** Playwright `page.route('https://tiles.openfreemap.org/**', …)` serves `testdata/map/style.json`, a minimal MapLibre style with a background layer only and no tile or glyph requests, so marker tests run in CI without the network. One nightly `@external` spec hits real OpenFreeMap and isn't a merge gate.
- **Marker assertions without pixel-peeping:** when `APP_ENV=test`, `map.js` exposes `window.__map`, and the spec calls `__map.queryRenderedFeatures({layers:['lights']})` to assert counts per status at a given zoom and that clusters exist at z14. The same spec checks `window.__glMapInits === 1` after opening and closing ten sheets (§3.3).
- **Visual:** `toHaveScreenshot` of `/list` and `/l/<id>` on all projects. For `/`, take the screenshot with `mask: [page.locator('canvas')]` so baselines don't depend on WebGL rasterisation, and leave the map's content to the vision review (below).
- **Go:** GeoJSON contract test (schema, 304 path); a template test that `/l/<id>` renders with JS disabled; the HTMX contract (§9.4).
- **a11y:** axe on `/`, `/list` and `/l/<id>`, plus a keyboard-only Playwright walk through `/list`.

**Validation:** the validator captures unmasked map screenshots at z13, z16 and z18 on each mobile project. The visual-reviewer (vision model) checks that markers are visible and distinguishable, clusters show counts, attribution is legible and unobstructed, and the bottom sheet doesn't cover the selected marker. Holdout uses a seeded dataset of 150 lights with a different status mix, and checks marker counts at z15 and z16.

**DoD:**

```
make check && make holdout
make validate MILESTONE=M3
make lighthouse            # record scores in docs/perf.md; not a gate
```

On a real phone (human), on mobile data: the map loads, a tap opens the sheet, and the share link opens the same light.

**Parallel versus serial:** T-301 to T-304 are parallel. `base.html` is touched only by T-305, which goes last.

### Phase 4: Reporting, comments, points, moderation (was v1 Phase 3)

**Goal:** a resident taps a light, then "It's out", "Flickering" or "It's on", optionally adds a photo or note, and the map updates. Repeat and abusive reporting is limited, and you can moderate.

**Contract (A-4xx):**
- A-401 First interaction creates an anonymous user with a generated handle (adjective + noun + 2 digits) and a session cookie (`HttpOnly`, `Secure` in prod, `SameSite=Lax`).
- A-402 Submitting a report updates the light's status per §4.4 and returns an HTMX partial that refreshes the sheet. The map marker changes colour without reloading the map.
- A-403 Confirmation: a second matching report from a different session and IP within 24 h sets "confirmed". The same session or IP does **not** confirm.
- A-404 Limits: one report per user per light per 12 h (the second attempt gets 429 and a friendly message), and 200 reports per day per IP HMAC.
- A-405 Points: +1 per report, +3 for confirming, +1 per photo, capped at 10 per day. The leaderboard shows the top 20 by handle only.
- A-406 Turnstile is required on **every** report and comment POST, in Cloudflare's **managed** mode (invisible for most users, so the three-tap goal is preserved) **[decision v2.1, §10 #11]**. The server verifies the token, and CI uses Cloudflare's test keys.
- A-407 Comments are plain text rendered escaped, at most 2000 characters. Three distinct-session flags auto-hide a comment pending review.
- A-408 `/admin/moderation` lists flagged comments and recent reports, with hide/unhide and "reset light status".
- A-409 After 30 days, a nightly job rounds report coordinates to 3 decimal places (≈ 111 m north–south) **[decision D10: 3 dp; 4 dp would be ~11 m — settled in v2.1, E7]**, and the job is idempotent.
- A-410 `/about` and `/privacy` exist (content in Phase 6), and every report form links to them.

| Task | Role | Owns | Deps | Claims |
|---|---|---|---|---|
| T-400 | test-author | `acceptance/**` | — | A-401..410 |
| T-401 | worker | `migrations/0003_*.sql`, `internal/store/reports*.go`, `internal/status/**` | T-400 | A-402 (store), A-403, A-405 |
| T-402 ‖ | worker | `internal/web/ratelimit*.go`, `internal/web/turnstile*.go` | T-400 | A-404, A-406 |
| T-403 ‖ | worker | `internal/web/anon*.go`, `internal/store/users*.go` | T-400 | A-401 |
| T-404 | ui-worker | `internal/web/report*.go`, `web/templates/light/report*.html`, `web/static/js/report.js` | T-401..403 | A-402, A-410 |
| T-405 ‖ | worker | `internal/web/comments*.go`, `internal/store/comments*.go`, `web/templates/comments/**` | T-401 | A-407 |
| T-406 ‖ | ui-worker | `internal/web/moderation*.go`, `web/templates/admin/moderation*.html`, `internal/web/leaderboard*.go`, `web/templates/leaderboard.html` | T-401 | A-405 (UI), A-408 |
| T-407 ‖ | worker | `internal/jobs/**` | T-401 | A-409 |

**Tests written first:**
- **Go:**
  - Table and property tests for `status.Derive`. For any sequence of reports, the status equals the latest report's status. Confirmed holds only if two distinct (session, IP) pairs agree within 24 h. Points equal the recomputed sum, and no user exceeds 10 per day.
  - A rate-limit test uses an injected clock.
  - Turnstile verification uses an `httptest` stub; the `siteverify` response shape is per Cloudflare docs.
  - XSS tests post `<script>` in comments and notes and assert it's escaped.
  - A job idempotency test.
- **Playwright:**
  - Two browser contexts, A and B, play two residents. A reports "out", and the marker is orange/"single" via `queryRenderedFeatures`. B reports "out", and the marker becomes "confirmed".
  - A re-reports → 429 message.
  - Photo upload on a report, on the iphone-16 project.
  - Comment flag ×3 → hidden.
  - Admin unhides.
  - `@a11y` on the sheet with the report form open. Status buttons must be real `<button>` elements with text labels.

**Validation:** the visual-reviewer checks the report sheet on 320 px (all three buttons visible without scrolling, labelled with text and not colour only) and the leaderboard. Holdout covers the 12 h boundary at 11:59 vs 12:01, an IP-only confirmation attempt that must not confirm, and the points cap.

**DoD:**

```
make check && make holdout && make validate MILESTONE=M4
```

**Parallel versus serial:** T-402 and T-403 run in parallel first. T-405, T-406 and T-407 run in parallel after T-401. T-404 is serial.

### Phase 5: Backups, restore drill, monitoring (was v1 Phase 4)

**Goal:** losing the volume, the machine or the region costs at most 24 h of data, and you have **practised** recovering.

**Contract (A-5xx):**
- A-501 Daily at 03:15 America/New_York, the server runs `VACUUM INTO '/data/backup/lights-YYYYMMDD.db'`, runs `PRAGMA integrity_check` on the copy, gzips it, uploads it to R2 under `db/`, uploads new photos under `photos/` (by sha256, skipping existing ones), writes a `manifest.json` (counts per table, sha256), and deletes local copies older than 2 days.
- A-502 R2 retention: daily copies kept 30 days, plus monthly copies (the 1st of the month) kept 12 months. This is done by the job's own pruning, so it doesn't depend on bucket lifecycle rules **[decision]**.
- A-503 `/healthz` returns 200 only if the DB answers `SELECT 1` within 500 ms. `/healthz/backup` returns 200 only if the last successful backup is less than 26 h old. That endpoint is what the external uptime monitor polls.
- A-504 `scripts/restore.sh <manifest-date> <target-dir>` downloads, verifies the sha256, decompresses, runs the integrity check, and prints row counts that must match the manifest.
- A-505 `server admin stats` prints row counts and the last backup time, so you don't need `sqlite3` in the image (C12).
- A-506 Backup failure logs `level=ERROR msg=backup_failed` and the backup health check fails.

| Task | Role | Owns | Deps | Claims |
|---|---|---|---|---|
| T-500 | test-author | `acceptance/**` | — | A-501..506 |
| T-501 ‖ | worker | `internal/backup/**` (R2 via S3 API) | T-500 | A-501, A-502, A-506 |
| T-502 ‖ | worker | `scripts/restore.sh`, `cmd/server/admin*.go` | T-500 | A-504, A-505 |
| T-503 ‖ | worker | `internal/web/health*.go` | T-500 | A-503 |
| T-504 | worker | `docs/runbook.md` | T-501..503 | — (reviewed by judge against §9.7) |

**Dependency note:** the one new dependency here is an S3 client. **[decision v2.1, §10 #13]** `allow_deps` for T-501 is the **AWS SDK for Go v2** (`github.com/aws/aws-sdk-go-v2` plus `config`, `service/s3`, `feature/s3/manager`). A minimal hand-rolled SigV4 PUT/LIST/DELETE was rejected: backups are the one path where chunked-content signing, clock skew and list-encoding edge cases fail silently, and a restore drill is the wrong place to discover that. The SDK costs several MB of binary (E14 accepts this).

**Tests written first:**
- **Go:** run against a local S3-compatible fake in tests **[which fake: decide at T-500; MinIO in a container, or an in-process httptest stub of the four calls used]**:
  - backup → manifest → restore round trip with counts compared;
  - a corrupted upload makes restore fail on sha256;
  - pruning keeps exactly the right set over a simulated 400 days (injected clock);
  - the health endpoint goes stale at 26 h.

**Drill (DoD, §9.7):**

```
make check && make holdout && make validate MILESTONE=M5
make drill-local                                   # seeds 150 lights+reports, backs up to fake S3, wipes, restores, diffs counts & sha256 → PASS
fly ssh console -C "/app/server admin backup-now"  # human: real R2 upload
scripts/restore.sh $(date +%Y%m%d) /tmp/restore    # human, on laptop: counts match manifest
```

**Judge input (Milestone M5):** the validator report for A-501..506, the drill-local output, and both human restore timings.

**Fly snapshot drill (human, quarterly, in the runbook):**
1. `fly volumes list` → note the volume ID.
2. `fly volumes snapshots list <vol-id>`.
3. `fly volumes create data --snapshot-id <snap-id> --region ewr --size 1`, attached to a temporary app or machine.
4. Boot it and run `server admin stats`.
5. Destroy the temporary resources.

Record the date and timings in the runbook.

**Monitoring (human):** an external uptime check on `/healthz` and `/healthz/backup`, alerting your email or phone. The service is your choice and isn't specified here.

**Parallel versus serial:** T-501 to T-503 run in parallel; the runbook is last.

### Phase 6: Seed data and launch (was v1 Phase 5)

**Goal:** the named parks and town buildings are inventoried (60–150 fixtures), the content pages are honest, and launch happens.

**Contract (A-6xx):**
- A-601 `/about` explains what the site is and is not. It's not affiliated with the town or GMLD. To report outages, call **GMLD at 978-372-1671**; that's the number from v1 and the research, so check it's current at launch (H5).
- A-602 `/privacy` lists what is stored (handle, reports, rounded locations after 30 days, IP HMAC, photos without metadata), retention periods, and how to request deletion.
- A-603 `/admin/export.csv` exports all lights and the latest status, for handing to Highway, GMLD or the Parks committee.
- A-604 Printable poster `/poster` (A4/Letter, QR code to `/`) passes print-media screenshot review.
- A-605 Seed coverage (`verified: "human"`): every park in `docs/seed-parks.md` has at least one light **[the park list is yours to provide: v1 named them, but this plan doesn't re-verify them]**. Verified by Anthony in the field; the judge records it from the A-603 export, not from an automated test (E5).

| Task | Role | Owns | Deps | Claims |
|---|---|---|---|---|
| T-600 | test-author | `acceptance/**` | — | A-601..605 |
| T-601 ‖ | ui-worker | `web/templates/about.html`, `web/templates/privacy.html` | T-600 | A-601, A-602 |
| T-602 ‖ | worker | `internal/web/export*.go` | T-600 | A-603 |
| T-603 ‖ | ui-worker | `internal/web/poster*.go`, `web/templates/poster.html` | T-600 | A-604 |

The QR code library needs `allow_deps`, or use a small vendored QR encoder; the planner decides.

**Human-heavy phase:** the inventory walks themselves, final copy approval, and announcement channels.

**DoD:**

```
make check && make holdout && make validate MILESTONE=M6
make smoke BASE_URL=https://<domain>
```

Plus the human checklist in §13.

### Phase 7: Street lights (gated on GMLD; was v1 Phase 6)

**Gate:** start only if GMLD (Kevin Snow per the research) agrees to share pole data or to accept reports. Otherwise the scope stays parks and buildings.

**Contract sketch (A-7xx):** import a pole CSV or GeoJSON with `gml_pole_no`; scale clustering to 713+ points; add a "Report to GMLD" deep link or email template. The planner writes the details after the gate opens.

**Validation:**
- performance check of `lights.geojson` with 1,000 features (size under 150 KB gzipped, generated in under 50 ms on shared-cpu-1x) **[targets are decisions, not measurements]**;
- clustering screenshot review.

---

## 8. Testing and typing practices

### 8.1 Testing pyramid (what runs where)

| Layer | Tooling | Count target | Runs in | Speed budget |
|---|---|---|---|---|
| Unit (pure functions: status, geo, img, config parsing, ratelimit) | `go test -race -shuffle=on`, table tests | Most tests | check-fast (every worker loop) | < 20 s |
| Store / integration | `go test` against a temp-file SQLite DB with migrations applied (never mocks) | Every query | check-fast | < 20 s |
| HTTP handler | `httptest` + real router + temp DB; asserts status codes, headers, HTMX partial vs full page | Every route × (anon, user, admin, HTMX) | check-fast | < 15 s |
| Template | Parse all, render each with fixtures, html-validate | Every template | check | < 10 s |
| Acceptance (Go, black-box) | Built binary on a random port; HTTP only | Every contract assertion that has no UI | check, gate, validator | < 60 s |
| E2E (Playwright) | Built binary; 4 projects; geolocation, upload, map hooks | Every contract assertion with UI | check, gate, validator | < 6 min with workers=4 |
| a11y / visual | axe; `toHaveScreenshot` | Every public page | check | included above |
| Holdout | Same tools, hidden repo | 1–2 variants per assertion | gate, validator | < 5 min |
| Smoke | `@smoke` Playwright against a deployed URL | 5–8 tests | after every deploy | < 60 s |

### 8.2 Typing in Go

- Typed IDs, `Status` and `Kind` types with `Parse*` constructors, and the `exhaustive` linter (§4.2).
- **No `any` in domain code.** `map[string]any` only at the JSON edge.
- Configuration is parsed once into a typed `Config` struct. Startup fails fast and lists every missing variable at once.
- SQL through `sqlc` generated code, so a schema change breaks compilation rather than production.
- Errors: wrap with `%w` (enforced by `errorlint`). Sentinel errors (`ErrNotFound`, `ErrRateLimited`, `ErrDuplicate`) map to HTTP codes in one place.
- `context.Context` is the first parameter on every store and network function (`noctx` linter).

### 8.3 Table and property tests

- **Table tests** for every handler and pure function. Case names read like assertions: `"second report same IP does not confirm"`.
- **Property tests:** stdlib `testing/quick`, or hand-rolled loops over seeded random inputs; **no new dependency [decision]**. Properties:
  - status invariants (§4.4);
  - Haversine symmetry and triangle inequality;
  - image output bounds (any decodable input → long edge ≤ 1600 and no APP1);
  - points recomputation equals the stored value.
- **Fuzz tests** (`go test -fuzz`, run for 30 s in nightly CI only) for `ParseReport`, the EXIF orientation reader and `ParseBBox`.

### 8.4 Templates

- A test parses `web/templates/**` with the production FuncMap and renders every page and partial with a fixture model. That catches missing fields at test time, because `html/template` otherwise fails at runtime.
- Rendered output is written to `testdata/rendered/` and checked with html-validate.
- **No `template.HTML` outside one audited helper** (golangci `forbidigo` rule), so all user text is escaped.

### 8.5 Migrations

- `TestMigrationsApplyCleanly`: an empty DB plus all migrations produces a schema equal to `testdata/schema.golden.sql`. The golden file is protected and changes only in the migration task that owns it.
- `TestMigrationsUpgradeWithData`: seed at version N−1 using that version's fixtures, migrate to N, then assert `PRAGMA foreign_key_check` is empty, `PRAGMA integrity_check` returns `ok`, and row counts are preserved.
- `TestMigrationChecksumGuard`: an edited applied migration stops the boot (A-104).
- Rule in AGENTS.md: **migrations are append-only.** A new file per change; never edit a merged one.

### 8.6 JavaScript

The client JavaScript is small: `walk.js`, `map.js`, `sheet.js` and `report.js`, each under 200 lines **[target]**.
- Plain ES modules with JSDoc types, `// @ts-check` at the top, and checked by `tsc --noEmit --checkJs --strict`.
- No bundler.
- Playwright specs are TypeScript, checked by the same `tsc`.
- No inline scripts or `on*=` attributes (the CSP forbids them anyway).

---

## 9. Validation

Validation is separate from "the tests pass". Workers make the tests pass. Validation asks whether the **product** does what the contract says, judged by someone who didn't build it, with evidence.

### 9.1 Layers

1. **Contract first.** `acceptance/contract.json` lists every assertion as `WHEN … THE SYSTEM SHALL …`. You skim it once per phase. Each assertion maps to at least one acceptance test (checked by `harness`) and at least one task (checked by `harness`).
2. **Protected tests.** Workers can't edit `acceptance/**`. That's enforced three times: the permission rules, the guard and checksum, and CODEOWNERS for external PRs.
3. **Clean-context review** by two model families (GLM 5.3 and DeepSeek), with reviewer-alt focused on test integrity and reward hacking.
4. **Black-box validator** on the merged candidate: built binary, HTTP and browser only, all projects, artifacts kept.
5. **Vision review** of artifacts by GLM 5.3 Flash or DeepSeek.
6. **Holdout tests** the builders never saw.
7. **Judge** sign-off, defaulting to NOT DONE.
8. **Deployed smoke** after every deploy, plus the human field test per phase.

### 9.2 Hidden held-out tests

- **Where:** a private repo `amscotti/groveland-lights-holdout` (a human step creates it), cloned next to the main repo. Its **absolute** path is resolved by the scheduler into `harness/policy.json` (`holdout_dir`, E1) — a relative `../holdout` would resolve wrong from any worktree. It stays outside every worktree.
- **Who writes them:** `test-author`, in the same `acceptance` task as the visible tests, with an `external_directory` allow scoped to that absolute path. Only `test-author` has it.
- **Who runs them:** `make holdout`, called by the gate and the validator. Workers and reviewers get pass/fail counts and assertion IDs, **never the test source or the failing values**, so they can't fit to them.
- **What they contain:** the same assertions with different data (coordinates, asset numbers, image orientations, timestamps at the boundaries), so code that hard-codes the visible fixtures fails.
- **Rotation:** after each phase the judge may move one holdout variant per assertion into the visible suite and have `test-author` write a fresh one.

### 9.3 Playwright artifacts and vision review

- **Config:** `trace: 'retain-on-failure'`, `screenshot: 'only-on-failure'`, `video: 'retain-on-failure'`.
- **Explicit `page.screenshot` calls** at the key states named in each contract item (for example "walk mode with accuracy shown", "sheet open over marker", "report form at 320 px") are always kept. They're saved under `harness/runs/<ts>/validate/<project>/<A-###>-<state>.png`.
- **The visual-reviewer** gets these PNGs through `opencode run --file`, together with each assertion's *visual notes* (written by test-author in `contract.json`, for example "three status buttons visible without scrolling at 320×640"). It answers PASS/BLOCK per image, citing the file name.
- **The validator** opens the screenshot and trace for every failed assertion and describes what's on screen, not just what the assertion message says.
- **Only GLM 5.3 Flash or DeepSeek** see images. GLM 5.3 (judge, planner, reviewer) gets only their text verdicts.

### 9.4 HTMX partial contract checks

A Go test drives every route registered as an HTMX endpoint (`Register` adds `hx: true` metadata) twice:
- **with `HX-Request: true`:** the response is a fragment (no `<html>`, `<head>` or `<body>`), contains no `id="map"`, and its root element's id matches the `hx-target` used in the templates that call it;
- **without it:** a full page, or a 303 to the canonical full page, so deep links and no-JS clients work.

It also checks:
- auth failures on HTMX requests return 401 plus `HX-Redirect`, not a login page swapped into a panel;
- form validation errors return 422 with the form fragment, and the templates set `htmx.config.responseHandling` so 422 swaps **[verify the exact htmx 2 config key at scaffold time]**.

A Playwright spec counts map initialisations across swaps (§3.3).

### 9.5 Accessibility

- `@axe-core/playwright`: `new AxeBuilder({ page }).withTags(['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa']).analyze()` on every public and admin page, in default and opened-sheet states. **Zero violations**; no `disableRules` without an ADR.
- **Keyboard-only flows:** list → light → report.
- **Status is never colour-only:** checked by the vision review and a DOM assertion that each marker legend item has text.
- **Map alternative:** `/list` (A-306).

### 9.6 Visual snapshots

- `toHaveScreenshot` baselines are generated **only in CI's Linux environment**, or locally in the official Playwright Docker image **[pick the image tag matching 1.65.0 at scaffold time]**, so font rendering matches.
- The map canvas is masked; map content is checked through `queryRenderedFeatures` and the vision review.
- Baselines live under `acceptance/e2e/__screenshots__/` and are protected. Updating them is an `acceptance` task: test-author runs `--update-snapshots`, and the visual-reviewer approves the new images.
- **Headless WebGL in CI** (MapLibre needs it) is **[unverified on GitHub runners]**. If the map fails to initialise, run the map projects with Chromium's software GL flags, or mark map-canvas specs `chromium-only` and rely on the human phone check for WebKit.

### 9.7 Backup and restore drill

- **Local, every CI run from Phase 5:** `make drill-local`:
  1. seed a fixed dataset;
  2. run the backup job against the S3 fake;
  3. delete the DB and photos;
  4. `restore.sh`;
  5. compare row counts per table, the sha256 of every photo, and `PRAGMA integrity_check`;
  6. boot the server on the restored data and run `@smoke`.
- **Production, quarterly (human, runbook):**
  - **Drill A, R2 restore:** download last night's backup to the laptop and verify counts.
  - **Drill B, Fly snapshot restore:** the steps in Phase 5.
  - **Drill C, full disaster recovery:**
    1. create a new volume;
    2. `fly ssh console`, then download from R2 and decompress into `/data` (`restore.sh` supports running in the machine);
    3. restart;
    4. `make smoke`.

  Record the timings. The success criterion is under 15 minutes (§1).

### 9.8 Deployed smoke tests

`make smoke BASE_URL=…` runs Playwright `@smoke` specs with no webServer, no test hooks (`APP_ENV=prod` hides them, so the specs only use visible UI and `/lights.geojson`), and no writes. The one exception is the probe counter in Phase 1.

The specs check:
- home loads, the map canvas appears, attribution is visible, and there are no console or CSP errors;
- `/lights.geojson` returns 200 with a valid ETag;
- `/l/<a known id>` returns 200;
- `/healthz` and `/healthz/backup` return 200;
- the TLS certificate is valid for 14+ days.

They run after every `fly deploy` (CI deploy job or manual) and nightly from CI against production.

---

## 10. Decisions log (v2)

| # | Decision | Why | Revisit if |
|---|---|---|---|
| 1 | Fly.io, one machine, one volume, ewr, auto-stop off | CLI-scriptable, ~$4/month and likely waived, the in-process ticker needs an always-on machine | Fly pricing or waiver changes; need for zero-downtime deploys |
| 2 | htmx 2.0.11, not 4.x | Stable `latest`; 4 is npm `next` | htmx 4 becomes `latest` (2027) |
| 3 | Alpine CSP build + strict CSP | XSS defence in depth for a public form site | — |
| 4 | Nightly `VACUUM INTO` to R2 + Fly snapshots; no Litestream | Simple, testable, RPO 24 h is fine for this data | You need RPO < 1 h |
| 5 | Makefile as the single gate entry point; mise for tool pins | Same commands for you, agents and CI | — |
| 6 | Harness scheduler is a Go program, not an agent | Deterministic, testable, can't be talked out of the rules | — |
| 7 | Holdout tests in a separate private repo | Workers can't read what isn't on their filesystem path; external_directory is denied | — |
| 8 | Hand-written migration runner and EXIF orientation reader | Small and tested; the EXIF libraries are stale (goexif last released 2019, go-exif v3 last tagged 2022) | Requirements grow |
| 9 | Accuracy threshold 30 m for walk mode | Phone GPS outdoors is usually well under that; still allows "save anyway" | Field test shows otherwise |
| 10 | Location rounding to 3 decimal places after 30 days | Privacy margin; reports remain useful per light (the light ID carries the precise location) | — |
| 11 | Turnstile on **every** report/comment POST, managed (invisible) mode **[v2.1]** | Invisible mode adds no tap for most users, and it deletes the "count reports, then require a token" state machine from server and tests (E15) | Managed mode starts showing visible challenges to legitimate users |
| 12 | Lighthouse is a recorded target, not a gate | MapLibre size; the gates are axe, layout and no-JS | — |
| 13 | AWS SDK for Go v2 for R2 **[v2.1]** | Backups are the one path where SigV4 edge cases fail silently; a restore drill is the wrong place to find out. Costs several MB of binary | Image size ever becomes a hard constraint |
| 14 | Image size target 40 MB, not 25 MB **[v2.1]** | modernc sqlite + the AWS SDK make 25 unrealistic; Fly pricing doesn't care | — |
| 15 | Gate retries a failed Playwright target once, then fails **[v2.1]** | A serial gate must not block all merges on one flake; two consecutive failures still fail the task | Flakes start masking real failures → switch to best-of-3 |

---

## 11. Human-only steps

Agents can't do these, and you shouldn't let them. Each step names the phase that needs it.

| # | Step | Needed by | Notes |
|---|---|---|---|
| H1 | Create `amscotti/groveland-lights` (public or private) and the **private** `amscotti/groveland-lights-holdout`. Clone them side by side. Turn on branch protection for `main`: require CI, and require CODEOWNERS review for PRs. | Phase 0 | The local gate pushes as you. CODEOWNERS protects against external PRs. |
| H2 | Install **OpenCode v2** with `curl -fsSL https://opencode.ai/v2/install \| bash`, `npm install -g @opencode/cli`, or `brew install anomalyco/tap/opencode-v2`. **The npm package `opencode-ai` is still the 1.x line (1.19.5)**, so don't use it. Sign in to the **Z.ai Coding Plan** and **DeepSeek API** providers. Confirm `opencode models` lists GLM 5.3, GLM 5.3 Flash and DeepSeek. | Phase 0 | Verified v2.2.7 (2026-11-10). |
| H3 | Install `mise` and Docker. Run `mise install` once the bootstrap creates `mise.toml`. | Phase 0 | — |
| H4 | **Fly.io:** create an account and add a payment method; `fly auth login`; create the app and volume (`fly launch --no-deploy --ha=false`, `fly volumes create data --region ewr --size 1 --snapshot-retention 14`). Generate secrets with `openssl rand -base64 32` ×3 and run `fly secrets set SESSION_KEY=… IP_HASH_KEY=… ADMIN_TOKEN=…`. Create a deploy token for CI and store it as the GitHub secret `FLY_API_TOKEN` (`fly tokens create deploy` **[verify the subcommand in flyctl v0.5.8 help]**). | Phase 1 | Secrets never go in the repo or in agent prompts. `.env.example` lists names only. |
| H5 | **Domain:** buy it, point DNS at Fly, `fly certs add <domain>`. Re-check the GMLD outage number shown on `/about`. | Phase 6 (can wait) | `<app>.fly.dev` works until then. |
| H6 | **Cloudflare:** create a Turnstile widget (site key + secret → Fly secrets), an R2 bucket, and an API token scoped to that bucket only (→ Fly secrets `R2_*`). | Phases 4 and 5 | CI uses Turnstile test keys and an S3 fake, so no real secrets are in CI except `FLY_API_TOKEN`. |
| H7 | External uptime monitor on `/healthz` and `/healthz/backup`. | Phase 5 | Any service; alerts to you. |
| H8 | Talk to the Highway Department (Arthur Markos) about a weekly digest, and to GMLD (Kevin Snow) about pole data. | Before Phase 6 / gate for Phase 7 | The research's top product risk. |
| H9 | Per phase: skim `contract.json` (H9a); do the phone field test (H9b); approve any change to protected files (H9c). | Every phase | Your three highest-leverage minutes. |
| H10 | Inventory walks; final copy approval; launch announcement. | Phase 6 | — |

---

## 12. Risks and open questions

| # | Risk | Mitigation |
|---|---|---|
| R1 | Nobody acts on reports (research's top risk) | H8 before launch; CSV export (A-603); weekly digest later |
| R2 | Agents game the tests: weakening them, special-casing fixtures, mocking the behaviour away | Protected acceptance tests + checksum + ratchet + reviewer-alt + holdout + judge (§9) |
| R3 | **Agent permissions are bypassable**: shown on v2.2.7 via shell, subagent delegation, and `git diff --output`; and a linked-worktree session may treat the **main checkout** as internal, letting a worker edit files the scheduler itself executes (E4) | Deny-all baseline + trailing denies + exact allowlists. **Real enforcement is the guard and gate**, plus the scheduler's clean-main assertion before every claim and gate run, and the S6 selftest probe that edits `<main>/cmd/harness/sched.go` from a worktree. `verify-agents` in the Phase 0 DoD. |
| R4 | OpenFreeMap has no SLA | Fallback: self-host a Massachusetts PMTiles extract (research). Map failure must never break `/list` or reporting. |
| R5 | Z.ai Coding Plan and DeepSeek rate limits or concurrency are unknown to this plan **[unverified]** | `max_parallel: 2` by default; the scheduler backs off on HTTP 429 in the transcript and retries later; raise the limit after observing |
| R6 | iOS photo uploads arrive as HEIC, which Go can't decode | Real-iPhone test in Phase 2 (C18). If HEIC arrives, options are client-side canvas re-encode to JPEG before upload (adds JS) or a cgo decoder (breaks pure-Go builds). Decide after the test. |
| R7 | WebGL unavailable in headless CI | §9.6 fallback |
| R8 | Single machine: each deploy has a brief outage; a host failure means downtime until restore | Accepted (§10 #1); runbook Drill C under 15 minutes |
| R9 | GPS accuracy under tree cover in parks | 30 m threshold + pin drag + photo; field test |
| R10 | Fly waiver or pricing changes | Costs recorded in §2 C7; re-check yearly |
| R11 | Model names change in `opencode models` | Bootstrap and `verify-agents` resolve names to IDs and fail loudly |

**Open questions for you** (Q2, Q3, Q5 and Q7 were decided in v2.1 — §10 #11, #13, D10, §5.1):
- Q1 Domain name?
- Q4 HEIC: decide after the iPhone test.
- Q6 The authoritative park and building list for seeding (`docs/seed-parks.md`).

---

## 13. Final checklist

**Harness (Phase 0)**
- [ ] `make doctor` all OK at the pinned versions
- [ ] 10 agents load. `verify-agents`: deny-all baseline, trailing denies, vision roles only on GLM 5.3 Flash or DeepSeek.
- [ ] `harness selftest` 6/6 deterministic (incl. S9 dirty worktree); `--live` S1, S6 (incl. the main-checkout probe) and S8 pass
- [ ] `make check` green locally and in CI; a red branch is shown red with the Playwright report artifact
- [ ] lefthook blocks a secret and bad formatting
- [ ] Holdout repo cloned next to the main repo, its absolute path in `harness/policy.json`; only test-author can reach it

**Per phase**
- [ ] `contract.json` delta skimmed (H9a)
- [ ] Acceptance task merged **before** any feature task started (`progress.log` order)
- [ ] All tasks `done`; no `blocked`
- [ ] `make check && make holdout` green on `main`
- [ ] Validator report: every assertion PASS with evidence paths
- [ ] Visual-reviewer PASS on all key-state screenshots
- [ ] Judge: DONE
- [ ] Human field test (H9b)

**Launch**
- [ ] 60–150 fixtures with photos; every listed park covered
- [ ] `/about` (GMLD number re-checked) and `/privacy` reviewed by you
- [ ] Backups: last success under 26 h; local drill green; R2 restore drill done; Fly snapshot drill done and timed
- [ ] Uptime monitor alerting to you
- [ ] `make smoke BASE_URL=https://<domain>` green
- [ ] Highway digest agreed (H8)
- [ ] `fly status`: 1 machine; `fly volumes list`: 1 volume, 14-day retention

---

## 14. Recommendation

Build it this way. The product is small enough that one person could hand-code it, and that's exactly why the **harness** is worth doing properly in Phase 0. It costs **two to three supervised days** of bootstrap (the paste block has two hard review checkpoints, after steps 5 and 12), and Phase 1 will shake out scheduler bugs the selftest didn't predict — budget a week for P0+P1 together (E12). After that, every phase is `make plan`, a two-minute contract skim, `make run`, and a phone test.

What makes it safe to leave GLM 5.3 Flash and DeepSeek workers running unattended isn't the agent prompts. It's the parts that don't depend on any model behaving:
- the deterministic scheduler;
- protected and checksummed acceptance tests;
- the hidden holdout suite;
- the serial `make check` gate;
- vision-based review of real screenshots.

My testing on OpenCode v2.2.7 showed permission rules alone can be bypassed, so don't skip the guard. Spend your own attention in three places: the contract, the field test, and talking to the Highway Department, because a perfect map that nobody acts on fails the real goal.

**Start now:** do H1–H3, save this file as `docs/PLAN.md`, and paste §6.2.

---

## 15. References (checked 2026-11-12)

- **Versions:**
  - Go: go.dev/dl (JSON), go.dev/doc/devel/release
  - htmx: npm dist-tags, github.com/bigskysoftware/htmx/releases, htmx.org
  - Alpine and `@alpinejs/csp`: npm
  - Pico: npm and GitHub
  - MapLibre GL JS: github.com/maplibre/maplibre-gl-js/releases and maplibre.org docs (`setWorkerUrl`)
  - Playwright: npm and playwright.dev docs (emulation and geolocation, `setInputFiles`, `toHaveScreenshot`, trace viewer, device list in 1.65 types)
  - @axe-core/playwright, axe-core, Lighthouse, @lhci/cli, html-validate, TypeScript: npm
  - golangci-lint: GitHub releases and golangci-lint.run configuration docs; staticcheck releases
  - Go modules: proxy.golang.org for govulncheck (x/vuln), modernc.org/sqlite, x/image, sqlc, goose
  - gitleaks, osv-scanner, lefthook, just, mise: GitHub releases
  - GitHub Actions majors: each action's releases page
- **OpenCode:** github.com/anomalyco/opencode releases (v2.2.7, 2026-11-10); opencode.ai/docs/agents and /permissions (fetched 2026-11-12: frontmatter fields, `mode: all`, last-match-wins, action names, default allow-all base policy); `opencode --version`, `opencode run --help`, `opencode debug agents` (machine-readable JSON) and `opencode models` (provider-scoped IDs) re-run **in the execution environment** on 2026-11-12; earlier empirical permission tests (§6.1.2); npm dist-tags for `htmx.org`, `maplibre-gl`, `@playwright/test`; go.dev/dl JSON — all fetched 2026-11-12.
- **Models:** Z.ai docs for GLM 5.3 (text input) and GLM 5.3 Flash (image, video, text, file input); DeepSeek API pricing and models page (Flash model has vision; V4 Pro doesn't).
- **Fly.io:** fly.io/docs/about/pricing (machines, volumes, snapshots, waiver, trial); fly.io/docs/volumes/snapshots; fly.io/docs/reference/configuration; flyctl v0.5.8 command help (`volumes create`, `volumes snapshots list`, `deploy --ha`, `secrets set --stage`, `ssh console -C`).
- **Cloudflare:** developers.cloudflare.com/turnstile (plans, test keys, server-side validation); developers.cloudflare.com/r2/pricing.
- **OpenFreeMap:** openfreemap.org and its ToS.
- **Litestream:** github.com/benbjohnson/litestream releases (v0.5.18).
- **Inputs:** Groveland Lights plan v1; the Groveland research brief (contacts, counts, product rules, risks); long-running-agent methodology (oracle-first, task graph, clean-context review, holdout tests, serial merge gate).

---

## 16. Unverified items and assumptions

These are flagged in the text. None of them blocks starting Phase 0.

1. Whether iOS Safari sends HEIC or JPEG for `<input type=file accept="image/*">` (C18, R6). Needs a real iPhone test.
2. Exact size added by `apk add sqlite` (C12). Moot, since this plan uses `server admin stats`.
3. MapLibre GL JS 6.18 bundle size and its effect on Lighthouse (D9).
4. Fly's under-$5 waiver: current per Fly's pricing page, but theirs to change (C7).
5. Concurrency and rate limits of the Z.ai Coding Plan and DeepSeek API for parallel agents (R5).
6. ~~Exact model **IDs**~~ Resolved and verified in the execution environment on 2026-11-12 (`zai-coding-plan/glm-5.3`, `zai-coding-plan/glm-5.3-flash`, `deepseek/deepseek-flash`); bootstrap re-checks via `opencode models` anyway (§6.1.1 rule 3), because provider catalogues drift.
7. Headless WebGL for MapLibre on GitHub-hosted runners (§9.6).
8. `TOWN_BBOX` accuracy against the real town boundary (§5.1).
9. My OpenCode permission tests used OpenCode's default free model, not your providers. Enforcement is client-side, so the results should carry over; the schema, flags, action names and last-match-wins mechanics were re-confirmed against the V2 docs and a live v2.2.7 install **in the execution environment** on 2026-11-12. The *bypass* experiments themselves were not re-run — `verify-agents` and selftest S6 (now including the main-checkout probe) remain the check.
10. Exact CSP hosts for OpenFreeMap styles, glyphs and sprites, and for Turnstile (§3.4); whether MapLibre 6.18's ESM dist serves a worker usable with `worker-src 'self'` (fallback pre-decided: `worker-src 'self' blob:` — §3.4, E11). Proven by the CSP-violation test, not assumed.
11. htmx 2 config key for swapping 422 responses (§9.4), the Playwright Docker image tag for 1.65.0 (§9.6), and `fly tokens create deploy` syntax (H4). Check each at scaffold time.
12. The GMLD outage number and the contacts (Arthur Markos, Kevin Snow) come from v1 and the research and weren't re-verified here.
13. The 713-light count is from a GMLD return headed 2019 (§1).
14. Targets marked **[target]** or **[decision]** (accuracy 30 m, rounding 3 dp, GeoJSON size and latency at 1,000 features, JS file sizes) are choices, not measurements.
