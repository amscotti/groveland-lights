# T-208 — independent review (reviewer-alt)

Basis: the attached diff (`harness/reviews/T-208.diff`), packet
`harness/tasks/T-208.md`, the judge-M1 finding (`harness/findings/judge-M1.md`,
gap b), the pre-change sources on main (`internal/web/{web,probe,csrf,web_test,
probe_test,middleware_logging_test}.go`, `cmd/server/{main,boot_test}.go`,
`web/templates/**`), the active probe contracts (`acceptance/go/probe_test.go`,
`acceptance/e2e/probe.spec.ts`), and the harness (`cmd/harness/{guard,gate}.go`,
`harness/state.md`). I also cross-checked the worker's recorded run
(`harness/runs/20261113T151011/T-208/worker.jsonl`) for the red/green
Content-Type reproduction and the final `make check` / `git diff --name-only`
output.

Verification limit: shell execution is denied in this session, so I did not
re-run `make check`, the smoke suite or the red-check myself; the recorded
outputs are quoted where they matter, and every code claim was traced against
the diff and the on-disk pre-change files. Line numbers below refer to the
patched tree (the diff is not applied in this checkout).

## Findings

### 1. [Medium — planner ratification, not a test weakening] `New` is variadic, not the frozen fixed-arity signature

The packet freezes
`func New(cfg config.Config, log *slog.Logger, db *store.DB) (*Server, error)`
(`harness/tasks/T-208.md:36-38`); the delivered signature is
`db ...*store.DB` at `internal/web/web.go:45` (copy at `:46-49`). The forcing
constraint is real and I verified it: main's
`internal/web/middleware_logging_test.go:29` calls `New(testConfig(env),
slog.New(h))` (two args), that file is outside T-208's `owns`, and a fixed
third parameter cannot compile it. Attempt 1's one-token edit of that file was
rejected by the ownership guard (exit 4), so the worker chose a variadic third
parameter rather than a `BLOCKED` handoff, and disclosed the choice
(`harness/handoffs/T-208.md`, "Spec deviation, precisely").

Impact: `New(cfg, log, db)` still compiles and wires the store, and `s.Store`
is the frozen access path, so T-203/T-205/T-206 are unaffected if they use the
documented form. The cost is loss of the compiler-enforced wiring guarantee: a
future caller can write `New(cfg, log)` (or pass extra stores that are silently
ignored past the first) and get a nil `Server.Store` and a runtime 500 instead
of a compile error. No test assertion was weakened to accommodate this, and the
handoff documents the trade-off, but the planner should either ratify the
variadic form or extend `owns` and restore the literal signature; I do not
block on it.

### 2. [Medium-Low — coverage theatre] The nil-store 500 subtest cannot detect removal of the nil guard

`internal/web/probe_test.go:184-194` asserts only that `GET /probe` with a
nil store returns 500. The guard it is meant to pin is
`if s.Store == nil { return 0, errProbeStoreNotWired }` at
`internal/web/probe.go:60-62`. If that guard is deleted, `s.Store.ProbeCount`
dereferences a nil `*store.DB`, `withRecover` in `internal/web/web.go:102-111`
catches the panic and still writes 500, so the test passes either way. The
packet also requires the message `probe: store not wired`
(`harness/tasks/T-208.md:39-41`), but no test asserts it. This row predates
T-208 in coarse shape, so it is not a newly introduced weakening, but it is
presented as pinning the new nil path and is weaker than that contract. Fix:
unit-test `(*Server).probeCount` directly (nil → `errProbeStoreNotWired`), or
build the server with a captured logger and assert the `probe: count` error
path rather than relying on the recovered panic.

### 3. [Low-Medium — test robustness] The wire-level POST row follows redirects and can pass on the home page

`internal/web/probe_test.go:403` constructs
`client := &http.Client{Jar: jar}` with no `CheckRedirect`, and the status/type
assertions are at `:412-415`. If `probePost` regressed to a 303 for an
`HX-Request` POST, the client would follow to `/`; the home page starts
`<!DOCTYPE html>` (`web/templates/base.html:1`), sniffs as
`text/html; charset=utf-8`, and both the 200 and exact-header assertions would
pass on the redirect target. `TestProbePost` still catches a 303 today, so this
is not a live handler-coverage hole, but the new test's claim to pin "both
fragment transports" is weaker than stated. Fix: set
`CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }`
(as `acceptance/go/probe_test.go:177-183` does) and/or assert the body contains
`<h2>Probe</h2>` before checking the header.

### 4. [Low — coverage of the new API + handoff accuracy] No unit test exercises `New(cfg, log, db)` with a non-nil store

`internal/web/probe_test.go:34-43` builds the server with
`newTestServer(t, config.EnvTest)` (nil store) and then assigns
`s.Store = db` directly; `internal/web/web_test.go:30-32` passes nil. So the
positive branch of the constructor (`st = db[0]`, `internal/web/web.go:46-49`)
has no in-package test. The path is still covered end-to-end by the gate's
black-box spawn suites (`make check` includes `test-acceptance-go`, Makefile:136;
`acceptance/go/probe_test.go` starts the binary, and `main.go` calls
`New(cfg, logger, db)`), so a constructor-copy bug would fail the gate, but
`make check-fast` alone would not catch it. This also makes the handoff's
"every store-backed test call[s] it exactly as the packet freezes:
`New(cfg, log, db)`" (`harness/handoffs/T-208.md`, "Frozen API") inaccurate for
the unit tests. A one-row `TestNewWiresStore` (`New(cfg, log, db)` → `s.Store
== db`) would close the gap and make the handoff true.

### 5. [Info — verified] The Content-Type fix is real and the new test is load-bearing

The fragment begins `<h2>Probe</h2>` (`web/templates/partials/probe.html:1`);
`<H2` is not in Go's `DetectContentType` HTML signature set, so without the
explicit header a real server sniffs the response as text/plain. The recorded
red-check confirms it: with the two `w.Header().Set(probeContentType)` lines
disabled, `TestProbeFragmentContentType` exits 1 with
`fragment Content-Type = "text/plain; charset=utf-8"` on both rows
(`harness/runs/20261113T151011/T-208/worker.jsonl`, the `go test ... -run
TestProbeFragmentContentType` tool result); with them restored it exits 0 and
`git diff` on `probe.go` is empty. The new test uses
`httptest.NewServer` + a real `net/http` client, not a `ResponseRecorder`
(which never sniffs), so it genuinely exercises the defect. The fix is applied
only in `probeGet` and the HTMX branch of `probePost`; the no-JS 303 branch and
`templates.go` (not owned) are untouched, and `http.Error` on a render failure
overwrites the header with its own text/plain 500.

## Checked and not found

- **No weakened, deleted, skipped or narrowed test.** The only pre-existing
  probe rows touched are helper-signature adaptations and the renamed
  "store unset" → "store nil" row, which still exercises the same
  `Server.Store == nil` path against a real `store.Open` temp SQLite DB where a
  counter is needed. `TestProbePost`'s CSRF matrix (missing/tampered → 403,
  count unchanged), the 400 parse row, the 303 no-JS row, the fragment markup
  row and the `#map`/`<html>`/`<body>` ban are all retained. No
  `t.Skip`/`test.skip`/`test.only`/`.fixme` appears in the diff.
- **No mocks bypass the behaviour under test.** Probe rows run through the real
  `s.Handler()` middleware chain against a real SQLite store; the new
  Content-Type row uses a real server and cookie jar. `probeCountNow` reads the
  real store to prove the increment, which is a legitimate direct oracle, not a
  mock.
- **No fixture-value special-casing in production code.** The only new
  production literal is `probeContentType = "text/html; charset=utf-8"`
  (`internal/web/probe.go:23`), the exact media type the packet requires.
  `Probe`, `Count:`, `#probe-count`, `csrf_token`, `/probe` and
  `Increment counter` remain ordinary template/handler strings; there is no
  branch on test/fixture values.
- **No dead code to satisfy linters.** `probeContentType` is used twice
  (`internal/web/probe.go:94` GET, `:159` POST); `errProbeStoreNotWired` is used
  by both `probeCount` and `probeInc`; `probeReToken` is used by the new test;
  the `len(db) > 0` branch is exercised by both `New(cfg, log, nil)` and the
  production `New(cfg, log, db)` call. No new `//nolint` without a reason, no
  `TODO`/`FIXME`, no "Phase N" strings in code.
- **No protected-path or dependency change.** The branch diff touches
  `cmd/server/{main,boot_test}.go`, `internal/web/{web,probe,probe_test,
  web_test}.go`, and the permitted handoff; `acceptance/**`, the harness code,
  `Makefile`, `.golangci.yml`, goldens and `go.mod/go.sum` are absent.
  `harness/tasks/graph.json`'s `review` → `merging` line is the
  scheduler/gate artifact, explicitly exempt in the guard
  (`cmd/harness/guard.go:117-123,146`) and restored by the gate
  (`cmd/harness/gate.go:122-126`), not a worker edit.
- **The boot smoke tests remain black-box.** `cmd/server/boot_test.go` is
  comment-only in this diff (the old comment named `SetProbeStore`); its bodies
  spawn `bin/server` and are unaffected by the signature change. `make check`
  still runs `test-acceptance-go`, which exercises `main.go`'s
  `New(cfg, logger, db)` wiring end-to-end.

## Verdict rationale

Both deliverables are implemented correctly. `SetProbeStore`/`probeStore`/
`errProbeStoreUnset` are gone, `Server.Store` is wired from `main.go`, the
nil-store path still answers 500, and `probeGet` plus the HTMX `probePost`
branch send the exact `text/html; charset=utf-8` header. The new wire-level
test genuinely fails without the header and passes with it. No protected or
pre-existing test was weakened, deleted, skipped or narrowed, and I found no
fixture special-casing or mock substitution.

The findings are a disclosed API-shape deviation (planner should ratify or
extend `owns`), two test-strength gaps (the nil-store row cannot distinguish
the intended error from a recovered panic; the POST row follows redirects), and
a missing positive unit test for the constructor. None of these changes the
correctness of the implementation or hides the defect the task was cut to fix,
so they are follow-up hardening items rather than merge blockers.

VERDICT: PASS
