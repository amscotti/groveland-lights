# T-103 review (reviewer: GLM-5.3, 2026-11-13)

Reviewed the attached diff against `harness/tasks/T-103.md`, the inert drafts
(`acceptance/drafts/probe_go_test.go.txt`, `probe.spec.ts.txt`,
`migrations_go_test.go.txt`, `README.md`), `harness/tasks/T-100.md`,
`harness/tasks/T-105.md`, `docs/spec/architecture.md`, and the merged T-100/
T-101/T-102 code (`internal/web/{web,templates,csrf,context}.go`,
`internal/store/*`, `internal/config/config.go`, `Makefile`, `.gitignore`,
`cmd/harness/ratchet.go`). The diff is not applied to this checkout and shell
execution was denied in this session, so this is a static review; every
behavioural claim below was verified by tracing the touched code paths and
their callees, and the two draft-contract questions are decidable by
inspection (shown below). Line numbers for new files refer to the diff.

## Packet compliance — verified

1. **Boot wiring** (`cmd/server/main.go:16` import, `:36-51` new): order is
   exactly `config.Load` → logger → `store.Open(context.Background(),
   cfg.DBPath)` → on error `return err` (main logs `fatal` and `os.Exit(1)`)
   → `defer func() { _ = db.Close() }()` → `web.SetProbeStore(db)` →
   `web.New` → serve, graceful shutdown untouched. The deferred close runs
   when `run()` returns, i.e. after `httpSrv.Shutdown` has drained requests —
   the WAL checkpoint happens post-drain, which is what the smoke
   persistence test relies on. The refusal wording needs no new code: the
   error from `internal/store/migrations.go:107` is
   `store: migration 0001_init.sql checksum mismatch (recorded …, file …)` —
   it names `0001_init.sql` and `checksum`, satisfying drafts README:53 and
   the A-104 runtime half.
2. **Fragment byte-exact** (`web/templates/partials/probe.html:1-6`):
   `<h2>Probe</h2>`, `<p>Count: <span id="probe-count">{{.Count}}</span></p>`,
   form with plain `action="/probe" method="post"` AND
   `hx-post="/probe" hx-target="#probe" hx-swap="innerHTML"`, hidden
   `csrf_token`, `<button type="submit">Increment counter</button>`. Matches
   the packet's frozen strings; no `<html>`/`<body>`, no `id="map"`, no
   inline handlers or styles (CSP clean).
3. **Home mount** (`web/templates/pages/index.html:10-12`): empty shell with
   the four hx attributes plus `<a href="/probe">Probe</a>`, inside
   `{{define "content"}}` after the intro copy — precisely the packet's
   deliverable text (packet lines 98-104). Pure static markup; renders under
   the frozen `homeData` fixture and the funcless parser (verified against
   `templates.go:24-66` — `TestRenderAll`/html-validate input unchanged in
   shape).
4. **Handler semantics** (`internal/web/probe.go:89-106` GET, `:130-170`
   POST): GET → count (unset/error → 500 + log line) → `IssueCSRFToken`
   (its "" return means it already answered 500) → buffered partial render.
   POST → `parseProbePost` (ParseForm error → 400) → `VerifyCSRF` → uniform
   403 with **no increment on the reject paths** → `ProbeInc` (error → 500)
   → HTMX: 200 fragment at N+1; no `HX-Request`: 303 `Location: /` after
   incrementing. All five POST clauses of the packet hold; the increment
   strictly follows verification.
5. **Scaffold as prescribed**: package-level `probeStore` +
   `SetProbeStore` (`probe.go:27-34`) called from main before `web.New`;
   `init()` registers both routes via the real `register`/`serverFrom`
   pattern (`probe.go:39-42`, matching `web.go:14-24,154-156` — no duplicate
   pattern conflict with `GET /{$}`); `renderPartial` (`probe.go:55-66`)
   mirrors `renderPage` over the existing `partials` map, buffers before
   writing, and looks up a constant name — no traversal surface. The SCAFFOLD
   doc comment marks the refactor point without any `Phase [0-9]` string
   (passes `Makefile:130-133` phase-grep). `web.go`, `templates.go`,
   `csrf.go`, `context.go`, `internal/store/**`, `migrations/**` are
   untouched by the diff — owns respected (`cmd/server/**`,
   `internal/web/probe*.go`, `web/templates/**`, plus the sanctioned handoff
   file). No new dependencies (`allow_deps: []`; imports are stdlib +
   in-module only).
6. **Rule 6 (parse-don't-validate)**: `parseProbePost` (`probe.go:115-125`)
   is the feature's sole form reader; it parses once into
   `probeSubmission{CSRFToken, HTMX}` and handlers consult only the typed
   fields. No `r.FormValue` anywhere in the new code (grepped). It reads
   `r.PostForm`, not `r.Form` — a query-string token can never satisfy
   verification, which is the stricter and correct choice for CSRF. Typed
   IDs: N/A (no entities in this feature).

## Security — clean

- **CSRF**: verification precedes any state change; `VerifyCSRF`
  (`csrf.go:69-78`) is constant-time and uniformly `ErrCSRF`, so the plain
  403 leaks no oracle; rejected paths leave the counter untouched, asserted
  against the **store** (not just the render) in both `probe_test.go` rows
  and the boot smoke.
- **XSS**: `html/template` auto-escaping only; no `template.HTML` (grepped);
  `Count` is `int64` and `CSRFToken` is server-minted base64url — no user
  input is reflected anywhere in the feature.
- **SQL**: none added; all access goes through the frozen sqlc-backed store
  API.
- **Path traversal / uploads**: no filesystem or upload surface added;
  `renderPartial` resolves a constant against the embedded-FS map. `ParseForm`
  bounds urlencoded bodies at Go's 10MB default.
- **Open redirect**: the 303 targets the fixed `"/"`. **Secrets in output**:
  none; the boot-test env values are the repo's established test dummies,
  byte-identical to the frozen drafts' own env blocks
  (`migrations_go_test.go.txt:144-146`) and `playwright.config.ts`.
- The `HX-Request` header only selects response shape (fragment vs
  redirect), never authorization — safe to trust.

## Architecture — conforms

Both HTMX targets are `#probe` (fragment form and home shell); the partial
contains no `id="map"` and is test-banned from it — the architecture.md
`#map` rule holds by construction. Feature registration follows the
init()/register pattern the codebase actually uses (architecture.md's
`Register(mux)` wording predates the skeleton's real shape; probe.go matches
the real code). The package-global store is explicitly packet-sanctioned
scaffold (packet lines 76-81), set once at boot before serving, so the
unsynchronized global is race-free in production; tests never run rows in
parallel. Status derivation, migrations append-only, etc. are untouched.

## Tests — they exercise the real behaviour

- `internal/web/probe_test.go`: all packet-required table rows present under
  the required names (`valid token increments`, `missing token 403 leaves
  count unchanged`, `tampered token 403`, `non-HTMX post redirects 303 to /`,
  `fragment has no html/body and no id="map"`, home-mount test) plus
  unparseable-body-400, store-unset-500 and closed-store-500 rows. Rows go
  through `s.Handler()` (the full middleware chain) against a real
  `store.Open` temp-file DB wired via the same `SetProbeStore` main uses;
  increment rows assert the store value via `probeCountNow`, and the
  contract-fragment row asserts the hidden input carries exactly the minted
  cookie value — a genuine double-submit check. Fresh server+store per row
  → shuffle-safe under `test-unit`'s `-shuffle=on`; cleanup order
  (`SetProbeStore(nil)` LIFO before `db.Close`) keeps rows independent; the
  closed-store row's later double `Close` is safe (`sql.DB.Close` is
  idempotent). The `(*Server).probeGet` test helper and package-level
  `probeGet` handler occupy different namespaces — compiles.
- `cmd/server/boot_test.go` (`//go:build smoke`, excluded from
  `go test ./...` exactly like the repo's `-tags acceptance` convention):
  spawns the real binary on a free port with a temp DB and drives, over real
  HTTP, the home mount, missing/tampered → 403 with counter provably
  unchanged, valid HTMX POST → 200 N+1, **SIGTERM + respawn on the same
  DB_PATH → count persisted** (the A-103 restart path, including the
  graceful-shutdown-then-close ordering it depends on), no-JS POST → 303
  `Location: /`, and the **tampered-checksum boot refusal**: non-zero exit
  with output naming `0001_init.sql` + `checksum`. Missing binary is a
  `t.Fatal`, never a skip; no `t.Skip`/`test.only` anywhere in the diff.

## The two handoff BLOCKED gaps — independently verified genuine

**(a) No-JS home clause is unsatisfiable within owns — confirmed.**
`probe.spec.ts.txt:76-94` and drafts `README.md:42` require `GET /` with JS
off to show a server-rendered heading, live `Count: N` in `#probe-count`,
and a submittable form. The home render is `home()` (`web.go:158-164`)
passing `homeData{Title,TestEnv,EnvLabel}`; `templates.go` parses with no
`Funcs`, so nothing in the owned `web/templates/**` tree can inject live
count/token data, and registering a second `GET /{$}` from probe.go would
panic the mux. The T-103 packet contradicts the draft it points at (packet
lines 43-44/58-59 vs the explicit deliverable at lines 99-104: "static form
is NOT needed … shell plus link" — and T-100.md:116-118 sides with the
draft). The worker implemented the packet's deliverable, led the handoff
with BLOCKED, and specified the exact ~15-line web.go fix. This is the
correct rule-1 escalation; it must be scheduled before T-105 activates the
TS no-JS test.

**(b) The Go draft's `probeReCount` cannot match the packet-frozen markup —
confirmed by inspection.** `probe_go_test.go.txt:52` pins
`Count:\s*(\d+)` over the raw HTML, but the packet freezes
`<p>Count: <span id="probe-count">N</span></p>`: after `Count:` the source
is ` <span…`, and `\s*` can only consume whitespace before the mandatory
digit — `<` is unreachable under any backtracking, so the regex can never
match. No markup satisfies both the frozen string and the regex (the TS
draft independently requires the digits-only span). Disposition check: the
drafts README's "only sanctioned edit" list (README:29-32) does not cover
it, **but T-105's own packet does** — `harness/tasks/T-105.md:40-45`
explicitly permits minimal edits for "a selector that must be scoped" with
handoff justification. So the worker's "rule 3b" routing is legitimate;
planner should still pre-ratify it (or re-freeze the string) so T-105
doesn't burn budget discovering it.

## Findings (non-blocking)

1. **`test-run.db*` worktree dirt is real and systemic.** With boot now
   opening `DB_PATH`, every Playwright `webServer` run
   (`playwright.config.ts:56`, `DB_PATH=./test-run.db`) creates
   `test-run.db`/`-shm`/`-wal` at the repo root; `.gitignore` covers only
   `dev.db*`, and `cmd/harness/ratchet.go:127-129`'s `DirtyLines` allowlist
   does not include them — so any locally-run `make check` leaves files the
   guard treats as dirty. Handoff open issue #3 is accurate; route a
   `.gitignore` owner task before the next e2e-running task.
2. **Smoke tamper test fails by hanging, not by assertion.** In
   `TestBootSmokeRefusedTamperedChecksum`, `_ = cmd.Run()`
   (`cmd/server/boot_test.go:~380`) is unbounded: if the boot refusal ever
   regressed, the server would listen and the test would sit in `Run()` until
   the go-test global timeout instead of the draft's explicit 15s bound
   (`migrations_go_test.go.txt:286-295`). Consider the goroutine+timeout
   shape on the next touch.
3. **`bootSpawn` inherits `os.Environ()`** (`boot_test.go:129`) while the
   frozen drafts use closed env blocks; with duplicated keys the runtime's
   getenv is first-match-wins, so a stray `DB_PATH`/`APP_ENV` in a dev shell
   could shadow the appended values. Practically nil; the drafts' closed env
   is marginally more robust.
4. **Smoke tests are a manual ritual.** The `smoke` tag keeps them out of
   `make check` (correctly — they spawn a listening binary), so nothing in
   the gate re-runs them; they are covered at T-105 by the activated drafts
   exercising the same paths. Noted so nobody assumes CI proves the
   SIGTERM-persistence claim.
5. **Scratch makefile outside the sanctioned exception.**
   `harness/handoffs/T-103-scratch.mk` (disclosed, untracked, artifact
   cleanup only) is a write into `harness/` beyond AGENTS.md rule 1's "except
   your handoff file". Harmless and transparent, but the orchestrator should
   delete it and give artifact cleanup a sanctioned home.
6. **Handoff nits**: first line is `BLOCKED (partial):` rather than the
   literal `BLOCKED:` prefix AGENTS.md specifies (substance intact); and the
   sandbox note's "writes to bin/ denied" sits awkwardly next to the
   reported `make build` runs — environment drift mid-task, exit codes
   otherwise consistent with the repo's targets.

## Verdict rationale

Everything the packet scopes to T-103 is implemented correctly, securely,
inside owns, byte-exactly to the frozen fragment strings, with table tests
that drive the real middleware chain and a real store, plus boot-level tests
that drive the real binary through the persistence and checksum-refusal
paths. The two draft assertions that cannot pass are inherited
packet-vs-draft contradictions the worker provably could not resolve without
breaking hard rules (frozen `web.go`, protected drafts), and both are
escalated with precise, verified dispositions rather than gamed. Blocking
the worker for a planner-owned contract contradiction would be the wrong
signal; the BLOCKED markers are in the handoff for the gate and planner.

VERDICT: PASS
