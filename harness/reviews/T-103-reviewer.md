# T-103 review (reviewer: GLM-5.3, 2026-11-13)

Reviewed the attached diff (`harness/reviews/T-103.diff`) against the task
packet (`harness/tasks/T-103.md`), the frozen drafts
(`acceptance/drafts/{probe_go_test.go.txt,probe.spec.ts.txt,
migrations_go_test.go.txt,README.md}`), the activation rules
(`harness/tasks/T-105.md`), `docs/spec/architecture.md`, and the merged code
the diff builds on (`internal/web/{web,templates,csrf,context}.go`,
`internal/store/{store,migrations}.go`, `internal/config/config.go`,
`migrations/0001_init.sql`, `Makefile`, `.gitignore`, `playwright.config.ts`,
`scripts/acceptance-go.sh`, `acceptance/go/*`, `cmd/harness/{guard,ratchet}.go`).
The diff is not applied to this checkout and shell execution was denied in
this session, so this is a static review: every behavioural claim was
verified by tracing code paths and callees; the two contract questions
(gaps a and b) are decidable by inspection. I could not re-run
`make check`/tests; the handoff's green claims are consistent with
everything statically verifiable.

## Packet compliance — verified

1. **Boot wiring** (`cmd/server/main.go`, diff hunks at the import and after
   the logger): order is exactly `config.Load` → logger →
   `store.Open(context.Background(), cfg.DBPath)` → on error `return err`
   (main logs `fatal` and `os.Exit(1)` — non-zero) →
   `defer func() { _ = db.Close() }()` → `web.SetProbeStore(db)` →
   `web.New` → serve; graceful shutdown untouched. The deferred close runs
   when `run()` returns, i.e. strictly after `httpSrv.Shutdown` drained
   requests — the post-drain WAL checkpoint the persistence smoke relies on.
   The refusal needs no new message code: `internal/store/migrations.go:107`
   emits `store: migration 0001_init.sql checksum mismatch (recorded …, file
   …)`, naming `0001_init.sql` and `checksum` per drafts README:53.
2. **Fragment byte-exact** (`web/templates/partials/probe.html:1-6`):
   `<h2>Probe</h2>`, `<p>Count: <span id="probe-count">{{.Count}}</span></p>`,
   form carrying BOTH the plain `action="/probe" method="post"` and
   `hx-post="/probe" hx-target="#probe" hx-swap="innerHTML"`, hidden
   `csrf_token`, `<button type="submit">Increment counter</button>`. No
   `<html>`/`<body>`, no `id="map"`, no inline handlers/styles.
3. **Home mount** (`web/templates/pages/index.html:10-12`): empty shell with
   the four hx attributes plus `<a href="/probe">Probe</a>` inside
   `{{define "content"}}` after the intro copy — precisely the packet's
   deliverable text (packet lines 98-104, which explicitly say a static
   noscript form is NOT needed). Pure static markup; renders under the frozen
   `homeData` fixture through the funcless parser (`templates.go:24-66`);
   h1 (intro) → h2 (fragment) heading order holds for html-validate/axe.
4. **Handler semantics** (`internal/web/probe.go`, probeGet/probePost): GET →
   count (unset/error → 500 + log) → `IssueCSRFToken` (its `""` return means
   it already answered 500) → buffered partial render. POST → parse error →
   400 → `VerifyCSRF` → uniform 403 with **no increment on reject paths** →
   `ProbeInc` error → 500 → HTMX: 200 fragment at N+1; no `HX-Request`:
   303 `Location: /` after incrementing. All five POST clauses hold;
   verification strictly precedes the state change.
5. **Scaffold as prescribed**: package-level `probeStore` + `SetProbeStore`
   (`probe.go:27-34`) wired from main before `web.New`; `init()` registers
   both routes through the real `register` accumulators (`web.go:14-24`,
   `154-156`; no pattern conflict with `GET /{$}`); `renderPartial`
   (`probe.go:55-66`) mirrors `renderPage` over the existing `partials` map,
   buffers before writing, resolves a constant name (no lookup surface).
   The SCAFFOLD doc comment marks the refactor point with no `Phase N`
   string. Diff touches only owned paths (`cmd/server/**`,
   `internal/web/probe*.go`, `web/templates/**`) plus the sanctioned handoff
   file — `web.go`/`templates.go`/`csrf.go`/`store`/`migrations` untouched.
   Imports are stdlib + in-module only (`allow_deps: []` honoured).
6. **Rule 6**: `parseProbePost` (`probe.go:115-125`) is the feature's sole
   form reader; parses once into `probeSubmission{CSRFToken, HTMX}`; no
   `r.FormValue` anywhere in the diff. It reads `r.PostForm` (not `r.Form`),
   so a query-string token can never satisfy verification — the stricter,
   correct choice. Typed IDs: N/A (no entities in this feature).

## Security — clean

- **CSRF**: `VerifyCSRF` (`csrf.go:69-78`) is constant-time and uniformly
  `ErrCSRF`, so the plain 403 leaks no oracle; rejected paths leave the
  counter untouched — asserted against the **store** (`probeCountNow`), not
  just the render, in both unit rows and the boot smoke.
- **XSS**: `html/template` auto-escaping only; no `template.HTML` in the
  diff (grepped); `Count` is `int64`, `CSRFToken` is server-constrained
  base64url (a reused cookie value must decode ≥32 bytes of base64url ≤256
  chars, so no quote-bearing reflection is even possible); no user input is
  reflected.
- **SQL**: none added; all access via the frozen sqlc-backed store API.
- **Uploads/path traversal**: no filesystem surface; `ParseForm` bounds
  urlencoded bodies at Go's 10MB default; the 303 targets the fixed `"/"`.
- **Secrets in output**: none — the boot-test env values are the repo's
  established dummies, byte-identical to the frozen drafts' own env blocks
  and `playwright.config.ts`. `HX-Request` only selects response shape,
  never authorization.
- **No harness regressions from boot-time DB open**: every existing spawn
  point already provides `DB_PATH` — `acceptance/go/m1_guards_test.go:119`,
  `scripts/acceptance-go.sh:19`, `playwright.config.ts:56` — so the active
  acceptance/e2e suites keep booting.

## Architecture — conforms

Both HTMX targets are `#probe` (home shell and fragment form); the partial
contains no `id="map"` and is test-banned from it — the architecture.md map
rule holds by construction (there is no map on the home page yet). Feature
registration follows the init()/register pattern the code actually uses.
The package-global store is explicitly packet-sanctioned scaffold (packet
lines 76-81), set once at boot before serving (race-free in production;
tests never run rows in parallel — safe under `-shuffle=on`, which only
permutes sequential order). Ownership map respected; status derivation and
migrations untouched.

## Tests — they exercise the real behaviour

- `internal/web/probe_test.go`: every packet-required row present under the
  required names (`valid token increments`, `missing token 403 leaves count
  unchanged`, `tampered token 403`, `non-HTMX post redirects 303 to /`,
  `fragment has no html/body and no id="map"`, home-mount) plus
  unparseable-body-400, store-unset-500, closed-store-500, and a
  contract-fragment row asserting the hidden input carries exactly the
  minted cookie value (a genuine double-submit check). Rows drive
  `s.Handler()` — the full middleware chain — against a real `store.Open`
  temp-file DB wired via the same `SetProbeStore` main uses. Fresh
  server+store per row; LIFO cleanup (`SetProbeStore(nil)` before
  `db.Close()`) keeps rows independent; the closed-store row's later double
  `Close` is safe (idempotent). The `probeTamper` shape-preserving flip
  exercises the constant-time mismatch branch, not just the missing/invalid
  branch.
- `cmd/server/boot_test.go` (`//go:build smoke`, excluded from
  `go test ./...` like the repo's `-tags acceptance` convention): spawns the
  real binary on a free port over real HTTP — home mount markup,
  missing/tampered → 403 with counter provably unchanged, valid HTMX POST →
  200 N+1, **SIGTERM + respawn on the same DB_PATH → count persisted** (the
  A-103 restart path, including the drain-then-close ordering it depends
  on), no-JS POST → 303 `Location: /`, and the **tampered-checksum boot
  refusal**: non-zero exit naming `0001_init.sql` + `checksum` (A-104's
  runtime half). Missing binary is `t.Fatal`, never a skip; no
  `t.Skip`/`test.only`/`.fixme` anywhere in the diff (ratchet-safe).

## The two handoff BLOCKED gaps — independently re-derived; both genuine

**(a) No-JS home clause unsatisfiable within owns — confirmed.**
`probe.spec.ts.txt:76-94` (JS off: server-rendered heading, live
`#probe-count`, submit button on `/`) and drafts `README.md:42` require the
home render to carry live data. `home()` (`web.go:158-164`) renders
`homeData{Title,TestEnv,EnvLabel}` only; `templates.go` parses with no
`Funcs`, so nothing in the owned `web/templates/**` tree can inject a live
count, and a live CSRF token *must* be minted by a handler
(`IssueCSRFToken(w, r)`) — no template-only construction exists. Registering
a competing home pattern from probe.go is not an option (`GET /{$}` is
taken; `GET /` would never win the match). The packet is internally
inconsistent: its UI-contract bullet says the section "must also work
without JS" while its deliverable text explicitly prescribes the empty shell
+ `<a href="/probe">` link. The worker implemented the packet's explicit
deliverable and escalated per AGENTS rule 1 with a precise ~15-line web.go
fix sketch (correct: add Count/CSRFToken to homeData, mint in `home()`,
`{{template "partials/probe.html"}}` in the section). This must be
scheduled before T-105 activates the TS no-JS test.

**(b) Go draft regex cannot match the packet-frozen markup — confirmed.**
`probe_go_test.go.txt:52` pins `Count:\s*(\d+)` against the raw HTML, but
the packet freezes `<p>Count: <span id="probe-count">N</span></p>`: after
`Count:` the source is ` <span…`; `\s*` consumes only whitespace and `(\d+)`
requires a digit, so the regex can never match — under any backtracking,
`<` is unreachable. No markup satisfies both the frozen string and the
regex without corrupting the visual contract (the TS draft independently
requires digits-only inside `#probe-count` AND `^Count:\s*\d+$` text —
duplicating digits outside the span to feed the Go regex would render
"Count: NN" and violate README:55-60's visual contract). Three of the four
contract sources (packet string, TS draft, README visual notes) agree with
the implemented span markup; only the raw-HTML regex dissents. Disposition:
T-105's own rules (`harness/tasks/T-105.md:40-45`) sanction minimal edits
for "a selector that must be scoped", which covers this; the drafts README's
narrower "only sanctioned edit" wording (README:29-32) creates a
planner-level tension the handoff honestly surfaced. The planner should
pre-ratify the regex fix (or re-freeze the string) so T-105 doesn't burn
activation budget discovering it.

## Findings (non-blocking)

1. **`test-run.db*` worktree dirt is real and systemic.** Boot now opens
   `DB_PATH`, and `playwright.config.ts:56` uses `DB_PATH=./test-run.db` at
   the repo root; `.gitignore` covers only `dev.db*` (lines 4-5), and the
   guard's `DirtyLines` allowlist (`cmd/harness/ratchet.go:127-129`) exempts
   only `harness/handoffs/**`, `harness/runs/**`, `node_modules`,
   `.env.test`, `.merge.lock`. Every local `make check` therefore leaves
   files the guard flags. Handoff open issue #3 is accurate — route a
   `.gitignore` owner task before the next e2e-running task.
2. **Tamper smoke test fails by hanging, not by assertion.** In
   `TestBootSmokeRefusesTamperedChecksum`, `_ = cmd.Run()`
   (`cmd/server/boot_test.go:381`) is unbounded: if the boot refusal ever
   regressed, the process would listen forever and the test would sit in
   `Run()` until the package timeout, unlike the frozen A-104 draft's
   explicit 15s bound + kill (`migrations_go_test.go.txt:293-294`). The
   activated draft covers the path properly; adopt the goroutine+timeout
   shape on the next touch of this file.
3. **`bootSpawn` inherits `os.Environ()`** with overrides appended; on Unix
   duplicate env keys resolve first-match-wins, so a stray `DB_PATH`/
   `APP_ENV` in a developer shell could shadow the intended values. The
   frozen drafts use closed env blocks (`m1Spawn`, `probeSpawnDir`).
   Practically nil; closed env is marginally more robust.
4. **Smoke tests are outside the gate** (`smoke` tag not in `make check`):
   correct for spawn-based tests, but nothing in CI re-proves
   SIGTERM-persistence or boot refusal until T-105 activates the drafts,
   which exercise the same paths.
5. **Scratch makefile beyond the sanctioned exception.**
   `harness/handoffs/T-103-scratch.mk` (disclosed, untracked, artifact
   cleanup only) exploits the guard's `harness/handoffs/**` dirt exemption —
   technically a write into `harness/` beyond AGENTS rule 1's "except your
   handoff file". Transparent and harmless, but the orchestrator should
   delete it once the `.gitignore` fix lands.
6. **Handoff nits**: first line is `BLOCKED (partial):` rather than the
   literal `BLOCKED:` prefix; the "11 of 12 assertions satisfied" headline
   counts the regex-hit assertions (#2/#7) as one draft-side defect — the
   detail table immediately below marks them ❌ honestly, so no
   misrepresentation. Sandbox forensics (git commands denied then permitted)
   are environment drift, not fabrication — the topology analysis matches
   the retry contexts appended to this task's packet.

## Verdict rationale

Everything the packet scopes to T-103 is implemented correctly, securely,
inside owns, byte-exactly to the frozen fragment strings, with table tests
that drive the real middleware chain and a real SQLite store, plus boot
smoke tests that drive the real binary through the persistence and
checksum-refusal paths. The two draft assertions that cannot pass are
packet-vs-draft contradictions the worker provably could not resolve without
breaking hard rules (frozen `web.go`/`web_test.go`, protected drafts), and
both are escalated with precise, independently verifiable dispositions
rather than gamed — no test was weakened, no protected file touched, no
spec hacked to fit. Blocking the worker would not fix either gap; the fixes
belong to a web.go owner task and T-105's sanctioned selector repair, both
of which the handoff routes to the planner. PASS the owned work; the BLOCKED
markers in the handoff stand for the planner and gate.

VERDICT: PASS
