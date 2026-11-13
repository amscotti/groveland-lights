# T-208 — independent review (reviewer-alt)

Basis: the attached diff (`harness/reviews/T-208.diff`), packet
`harness/tasks/T-208.md`, `harness/findings/judge-M1.md` (gap b), the active
probe contracts (`acceptance/go/probe_test.go`, `acceptance/e2e/probe.spec.ts`),
the pre-change sources on main (`internal/web/{web,probe,csrf}.go`,
`web_test.go`, `probe_test.go`, `middleware_logging_test.go`,
`cmd/server/{main,boot_test}.go`), and the handoff added by the diff.

Verification limit: the worktree is at the pre-change `main` (the diff is not
applied here) and shell execution is denied in this session, so I did not re-run
`make check-fast`, `make check` or the red/green command. The exit codes below
are from the handoff; every claim about the code was traced statically against
the diff and the current sources.

## Findings

### 1. [Medium, API/spec — planner decision, not a test-integrity failure] `New` is variadic, not the frozen fixed-arity signature

`internal/web/web.go:46` delivers
`func New(cfg config.Config, log *slog.Logger, db ...*store.DB) (*Server, error)`,
with the first element copied at `internal/web/web.go:51-54`. The packet
(`harness/tasks/T-208.md:36-38`) explicitly freezes
`New(cfg config.Config, log *slog.Logger, db *store.DB)` and the handoff itself
says downstream tasks compile against `New(cfg, log, db)`
(`harness/handoffs/T-208.md` "Frozen API").

The reason is real and disclosed: main's `internal/web/middleware_logging_test.go:29`
calls `New` with two arguments and that file is outside `owns`, so a literal
3-arg signature cannot compile the branch without a guard failure. The worker
chose the variadic third parameter instead of editing the unowned test and
documented the contradiction (`harness/handoffs/T-208.md:53-69, 90-115`).

Test-integrity impact: no test assertion was weakened to accommodate it, and
`New(cfg, log, db)` still compiles and wires the store. `web_test.go:30-32` adds
the explicit `nil` argument, which is a test-file edit but not a narrowing.
The cost is that the compiler no longer enforces the packet's frozen arity: a
future caller can omit the store and get a silent nil-`Server.Store` (every
store-backed route 500s) instead of a compile error. The planner should either
accept the variadic form or extend `owns` and restore the fixed arity; I do not
block on this because the handoff discloses it and the requested call form
works.

### 2. [Low, unit coverage of the new API] No unit test calls `New(cfg, log, db)` with a non-nil store

`internal/web/probe_test.go:34-43` builds the probe server with
`newTestServer(t, config.EnvTest)` and then assigns `s.Store = db` directly at
line 42. `internal/web/web_test.go:30-32` passes `nil`. So no test in
`internal/web` exercises the production call form's positive branch
(`internal/web/web.go:51-55`).

The branch is still covered end-to-end by the gate's spawned-binary suites:
the acceptance Go probe test (`acceptance/go/probe_test.go`, which `make check`
runs as `acceptance-go`) and the Playwright probe spec start `bin/server`, whose
`main.go` calls `web.New(cfg, logger, db)`; if `New` stopped copying `db` into
`Server.Store`, those suites would 500 and fail. The additional smoke-tagged
`cmd/server/boot_test.go` covers it too but is not in the gate
(`harness/handoffs/T-208.md` lists it as a separate command). So this is not a
missing coverage hole so much as a missing unit-level assertion: a
`check-fast` run alone would not catch a `New` wiring regression. Adding
`TestNewWiresStore` (or constructing `newProbeServer` through
`New(testConfig(config.EnvTest), logger, db)`) would close the gap cheaply.

### 3. [Medium–Low, coverage theatre] The nil-store 500 row cannot fail if the nil guard is deleted

`internal/web/probe_test.go:184-194` asserts only `rec.Code == 500`. The guard
it is meant to protect is `internal/web/probe.go:60-62`
(`if s.Store == nil { return 0, errProbeStoreNotWired }`). If that guard were
deleted, `s.Store.ProbeCount(ctx)` would dereference a nil `*store.DB`, and
`withRecover` in `internal/web/web.go` would catch the panic and answer 500, so
the test would still pass. The packet (`harness/tasks/T-208.md:39-41`) requires
the message `probe: store not wired`; no test captures the logger or asserts
`errProbeStoreNotWired`. The test is therefore weaker than the contract it is
presented as pinning (this is the same issue the T-103 alt review raised for the
"store unset" row).

Fix: unit-test `(*Server).probeCount` directly (nil store returns
`errProbeStoreNotWired`) or capture `s.Log` output and assert the
`probe: count` log path rather than relying on the recovered panic.

### 4. [Low, test robustness] The wire-level POST row follows redirects; it can pass on the home page instead of the fragment

`internal/web/probe_test.go:385` sends the HTMX POST and
`internal/web/probe_test.go:403` creates the client as
`&http.Client{Jar: jar}` with no `CheckRedirect`. The status/header assertions
are at `internal/web/probe_test.go:412-414`. If `probePost` regressed to a 303
for the HTMX request, the client would follow to `/`; the home page starts with
`<!DOCTYPE html>` and sniffs as `text/html; charset=utf-8`, so both the 200 and
the exact Content-Type assertion would pass on the redirect target — the test
would not notice that the POST response was not the fragment.

`TestProbePost`'s existing rows would catch the 303 today, so this is not a live
coverage hole for the handler; it is a robustness flaw in the new test's claim
to pin both fragment transports. Set
`CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }`
(as `acceptance/go/probe_test.go:177-183` already does) and/or assert the body
contains `<h2>Probe</h2>` before the header check.

### 5. [Info] The handoff's `make e2e-task T=` note is accurate but unrelated to the diff

`harness/handoffs/T-208.md` records that `make e2e-task T=T-208` exits 2
because the Makefile variable is `TASK`. The Makefile is protected and the
packet has `claims: []`, so this is documentation for the planner, not a
test-integrity issue.

## Checked and not found

- **No weakened, deleted, skipped or narrowed test.** The only pre-existing test
  rows touched are `TestProbeGet`'s "store unset" row and the helper signatures;
  the "store unset" row is semantically preserved as "store nil" (both exercise
  the same `Server.Store == nil` path), and all probe rows still run against a
  real `store.Open` temp SQLite DB. No `t.Skip`, `test.skip`, `test.only` or
  `.fixme` is added (`harness/reviews/T-208.diff` scan).
- **No mock bypasses the behaviour under test.** The probe tests use
  `s.Handler()` (the real middleware chain) and a real SQLite store; the new
  Content-Type test uses `httptest.NewServer` and a real `net/http` client, not
  a `ResponseRecorder` (which would not sniff, as the handoff correctly notes).
- **No fixture-value special-casing in production code.** The only new
  production literal is `probeContentType = "text/html; charset=utf-8"`
  (`internal/web/probe.go`), which is the HTTP media type required by the
  packet, not an acceptance fixture string. `Probe`, `Count:`, `#probe-count`,
  `csrf_token`, `/probe` and `Increment counter` are unchanged feature
  strings, not branches.
- **No dead code to satisfy a linter.** `probeContentType` is used twice;
  `errProbeStoreNotWired` is used by both `probeCount` and `probeInc`; the
  `len(db) > 0` branch is exercised by the production `New(cfg, log, db)` call
  (and by `New(cfg, log, nil)` in `web_test.go`). `probeReToken` is used by the
  new test. No `//nolint` without a reason was added.
- **The header fix is correct and the new test can fail.** The fragment starts
  with `<h2>`; Go's `http.DetectContentType` HTML signatures include `<H1` but
  not `<H2`, so without the explicit header a real server sniffs the fragment as
  `text/plain; charset=utf-8` and the new exact-header assertion fails. With the
  two `w.Header().Set(probeContentType)` lines (`internal/web/probe.go:92` for
  GET, `internal/web/probe.go:161` for the HTMX POST) it passes. The handoff's
  red/green claim matches this mechanism.
- **No protected path or dependency change.** The diff touches `cmd/server/{main,boot_test}.go`,
  `internal/web/{web,web_test,probe,probe_test}.go`, and the permitted handoff;
  `acceptance/**`, `harness/` orchestration code, `Makefile`, `.golangci.yml`
  and goldens are absent. No `go.mod`/`go.sum` change (`allow_deps: []`).

## Verdict rationale

Both deliverables are implemented correctly: the store moved onto `Server`
without the package global, `SetProbeStore` is gone, and `probeGet` plus the
HTMX `probePost` branch now send the exact `text/html; charset=utf-8` header.
No protected or pre-existing test was weakened, deleted, skipped or narrowed,
and the new wire-level test is a genuine regression guard (it fails on the
unfixed wire response). The findings above are a disclosed API-shape deviation
and three test-strength/coverage gaps, not hidden special-cases or fabricated
verification. The planner should resolve the variadic-vs-fixed `New` signature,
and the worker (or a follow-up) should add the positive `New` wiring test and
strengthen the nil-guard and POST-redirect assertions; none of these justify
blocking the merge of the correct implementation.

VERDICT: PASS
