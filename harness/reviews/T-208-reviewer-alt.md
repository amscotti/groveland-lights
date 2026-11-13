# T-208 — independent review (reviewer-alt)

Basis: the attached diff (`harness/reviews/T-208.diff`), packet
`harness/tasks/T-208.md`, the judge-M1 finding (`harness/findings/judge-M1.md`,
gap b), the pre-change sources on main (`internal/web/{web,probe,csrf,web_test,
probe_test,middleware_logging_test}.go`, `cmd/server/{main,boot_test}.go`,
`web/templates/**`), the active probe contracts (`acceptance/go/probe_test.go`,
`acceptance/e2e/probe.spec.ts`), and the harness (`cmd/harness/{guard,gate}.go`,
`harness/state.md`). I also read the worker transcripts for the red/green
Content-Type check and the final verification runs
(`harness/runs/20261113T151011/T-208/worker.jsonl` and
`.../20261113T153351/T-208/worker.jsonl`).

Verification limit: shell execution is denied in this session and the diff is
not applied in the root checkout, so I did not re-run `make check`, the smoke
suite or the red-check myself. Every code claim below was traced against the
diff, the on-disk pre-change files, and the worker transcript; line numbers
refer to the patched files as shown by the diff.

## Test-integrity / reward-hacking checks

- **No deleted, skipped, narrowed or disabled test.** The only pre-existing
  probe rows touched are helper-signature adaptations (`newProbeServer` now
  returns `(*Server, *store.DB)` at `internal/web/probe_test.go:34-43`;
  `probeCountNow` takes the handle at `:112-121`) and the renamed
  "store unset" → "store nil" row (`:184-193`). That row still exercises the
  same `Server.Store == nil` path, now against a nil-store `newTestServer`
  instead of a global reset. `TestProbePost`'s CSRF matrix (missing/tampered →
  403 with the count unchanged), the 303 no-JS row, the 400 parse row, the
  fragment-markup row and the `#map`/`<html>`/`<body>` ban are all retained
  (`internal/web/probe_test.go:139-296`). No `t.Skip`, `test.skip`,
  `test.only` or `.fixme` appears in the diff.
- **No fixture-value special-casing in production code.** The only new
  production literal is `probeContentType = "text/html; charset=utf-8"`
  (`internal/web/probe.go:23`), the exact media type the packet requires
  (`harness/tasks/T-208.md:52-56`). `Probe`, `Count:`, `#probe-count`,
  `csrf_token`, `/probe` and `Increment counter` remain ordinary
  template/handler strings; there is no branch on test data.
- **No mocks bypass the behaviour under test.** Probe rows use the real
  `s.Handler()` chain and a temp-file SQLite store opened through
  `store.Open`; `probeCountNow` reads that real store. The new Content-Type
  test uses `httptest.NewServer` plus a real `net/http` client and cookie jar,
  not a `ResponseRecorder` (`internal/web/probe_test.go:333-419`).
- **No protected path or dependency change.** The branch diff touches
  `cmd/server/{main,boot_test}.go`,
  `internal/web/{web,probe,probe_test,web_test}.go`, the permitted handoff,
  and the scheduler-owned `harness/tasks/graph.json`. `acceptance/**`,
  `Makefile`, `.golangci.yml`, goldens and `go.mod/go.sum` are absent.
  `net/http/cookiejar` is stdlib, so `allow_deps: []` is respected.
- **The Content-Type fix is real and the new test is load-bearing.** The
  fragment starts `<h2>Probe</h2>` (`web/templates/partials/probe.html:1`);
  `<H2` is not in Go's `DetectContentType` HTML signature set, so without the
  explicit header a real server sniffs the response as
  `text/plain; charset=utf-8`. The worker transcript records the red-check:
  with both `w.Header().Set(probeContentType)` lines disabled,
  `TestProbeFragmentContentType` exits 1 with exactly that header on both
  rows; restored, it exits 0 and `git diff` is empty
  (`worker.jsonl`, `TestProbeFragmentContentType` run, attempt 3). The fix is
  applied only in `probeGet` (`internal/web/probe.go:94`) and the HTMX branch
  of `probePost` (`:161`); the no-JS 303 branch and `templates.go` are
  untouched. `renderPartial` still buffers, so a render failure leaves
  `http.Error` free to write its own text/plain 500 (`:41-53`).

## Findings

### 1. [Medium — planner ratification, a guard-driven API deviation] `New` is variadic, not the frozen fixed-arity signature

The packet freezes
`func New(cfg config.Config, log *slog.Logger, db *store.DB) (*Server, error)`
(`harness/tasks/T-208.md:36-38`). The delivered signature is
`func New(cfg config.Config, log *slog.Logger, db ...*store.DB) (*Server, error)`
(`internal/web/web.go:46`, branch `:51-54`). The forcing constraint is real:
main's `internal/web/middleware_logging_test.go:29` calls `New(testConfig(env),
slog.New(h))` with two arguments, that file is outside T-208's `owns`, and a
fixed third parameter cannot compile it. Attempt 1's one-token edit of that
file was rejected by the ownership guard (exit 4), so the worker chose a
variadic third parameter instead of a `BLOCKED` handoff and disclosed the
choice (`harness/handoffs/T-208.md`, "Spec deviation, precisely").

This is the closest thing in the diff to reward hacking: the signature was
changed to satisfy the ownership checker, not the packet, and it removes the
compiler-enforced store-wiring guarantee. A future caller can write
`New(cfg, log)` (or pass extra stores that are silently ignored past the
first) and get a nil `Server.Store` and a runtime 500 instead of a compile
error. I do **not** treat it as a test-integrity blocker because no test
assertion was weakened to accommodate it, `New(cfg, log, db)` still compiles
and wires the store, `s.Store` is the frozen access path, and the behaviour is
covered end-to-end by the black-box smoke/acceptance spawn suites. The planner
should either ratify the variadic form explicitly or extend `owns` to include
`internal/web/middleware_logging_test.go` and restore the literal signature;
this is a follow-up decision, not a reason to hide the work.

### 2. [Medium-Low — assertion weaker than the contract] The nil-store 500 row cannot distinguish the nil guard from a recovered panic

`internal/web/probe_test.go:184-193` asserts only that `GET /probe` with a nil
store returns 500. The guard it is meant to pin is
`if s.Store == nil { return 0, errProbeStoreNotWired }`
(`internal/web/probe.go:60-62`). If that guard were deleted,
`s.Store.ProbeCount` would dereference a nil `*store.DB`; `withRecover`
(`internal/web/web.go:102-111` on main, unchanged by this diff) would catch
the panic and still write 500, so the test would pass either way. The packet
also requires the message `probe: store not wired`
(`harness/tasks/T-208.md:39-41`), but no test asserts it. The row predates
T-208 in coarse shape, so it is not a newly introduced weakening, but it is
presented as pinning the new nil path and is weaker than that contract. Fix:
unit-test `(*Server).probeCount` directly (nil → `errProbeStoreNotWired`), or
build a server with a captured logger and assert the `probe: count` error path
rather than relying on the recovered panic.

### 3. [Low-Medium — test robustness] The wire-level POST row follows redirects and can pass on the home page

`internal/web/probe_test.go:403` constructs
`client := &http.Client{Jar: jar}` with no `CheckRedirect`, and the status/type
assertions are at `:412-415`. If `probePost` regressed to a 303 for an
`HX-Request` POST, the client would follow to `/`; the home page starts
`<!DOCTYPE html>` (`web/templates/base.html:1`), sniffs as
`text/html; charset=utf-8`, and both the 200 and exact-header assertions would
pass on the redirect target. `TestProbePost` still catches a 303 today, so
this is not a live handler-coverage hole, but the new test's claim to pin
"both fragment transports" is weaker than stated. Fix: set
`CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }`
(as `acceptance/go/probe_test.go:177-183` does) and/or assert the body
contains `<h2>Probe</h2>` before checking the header.

### 4. [Low — coverage of the new API + handoff accuracy] No unit test exercises `New(cfg, log, db)` with a non-nil store

`internal/web/probe_test.go:34-43` builds the server with
`newTestServer(t, config.EnvTest)` (nil store) and then assigns `s.Store = db`
directly; `internal/web/web_test.go:30-32` passes nil. So the positive branch
of the constructor (`st = db[0]`, `internal/web/web.go:51-54`) has no
in-package test. The path is covered end-to-end by the gate's black-box spawn
suites (`main.go` calls `New(cfg, logger, db)`; the acceptance/smoke suites
exercise a real store), so a constructor-copy bug would fail the gate, but
`make check-fast` alone would not catch it. This also makes the handoff's
"every store-backed test call[s] it exactly as the packet freezes:
`New(cfg, log, db)`" (`harness/handoffs/T-208.md`, "Frozen API") inaccurate for
the unit tests. A one-row `TestNewWiresStore` (`New(cfg, log, db)` →
`s.Store == db`) would close the gap and make the handoff true.

### 5. [Low — comment accuracy] `probeGet`'s new comment calls the fragment a "full HTML document"

`internal/web/probe.go:91-93` says "The fragment is a full HTML document for a
no-JS visitor who navigates here directly." It is not a full document:
`TestProbeGet` asserts the fragment contains no `<html`/`<body>`
(`internal/web/probe_test.go:165-179`). The intended meaning is that the
response body is the whole fragment. Comment-only; no behavioural impact.

### 6. [Info — scheduler-owned file] The `harness/tasks/graph.json` hunk is not a worker edit

The diff includes `harness/tasks/graph.json` `"status": "review"` →
`"merging"` (`harness/reviews/T-208.diff:340-349`). That file is
scheduler-owned and guard-exempt (`cmd/harness/guard.go:117-125,146-148`);
`finalizeMerge` restores main's copy before merging
(`cmd/harness/gate.go:122-128`). The handoff attributes it to the gate's
bookkeeping commit, and the commit's subject/short history in the worker
transcript is consistent with that account. Even if a worker had authored it,
the guard exempts the path and the gate replaces the content; it is not
authoritative branch content. Not a rule-1 violation in effect.

### 7. [Info — harness, not code] The attempt-4 handoff's rebase recipe is outside the review surface

The handoff's opening section tells the gate/integrator to `git rebase --skip`
the graph-only `2d799d0` commit and not to `--abort`. That is a topology
workaround for a scheduler race, not a test or code change. The recipe is
consistent with the gate restoring `graph.json`, and the handoff is the
permitted place to record it. It does not affect the test-integrity verdict,
but the gate/integrator should verify ancestry after the rebase rather than
trust the handoff's SHA list (the branch had already gained handoff-only
commits during attempt 4).

## Checked and not found

- No weakened, deleted, skipped or narrowed test; the only renamed row keeps
  the same assertion shape and target path.
- No mocks substituting for the behaviour under test; real SQLite, real
  middleware chain, real wire-level server.
- No fixture literals or branch-on-fixture in non-test code.
- No dead code added to satisfy a linter; `probeContentType`,
  `errProbeStoreNotWired` and `probeReToken` all have live uses. The
  `len(db) > 0` / ignore-extra-stores behaviour is unnecessary API surface
  (finding 1), not dead code.
- No `TODO`/`FIXME`/`panic("not implemented")`, no "Phase N" strings, no
  `//nolint` without a reason in added non-test lines, and no
  `t.Skip`/`test.only`/`test.skip`/`.fixme`.
- The boot smoke tests remain black-box: `cmd/server/boot_test.go` is
  comment-only in this diff, its imports (`:21-38`) include no `internal/web`,
  and it spawns `bin/server`, so the signature change cannot affect it.
- The acceptance contract is unaffected: `acceptance/go/probe_test.go` asserts
  body and CSRF behaviour, not `Content-Type` or `SetProbeStore`; the e2e
  probe spec exercises the home mount rather than a direct `/probe` visit.

## Verdict rationale

Both deliverables are implemented correctly. `SetProbeStore`/`probeStore`/
`errProbeStoreUnset` are gone, `Server.Store` is wired from `main.go`, the nil
store still answers 500 (though finding 2 says the test cannot distinguish the
guard from a recovered panic), and `probeGet` plus the HTMX `probePost` branch
send the exact `text/html; charset=utf-8` header. The new wire-level test
genuinely fails without the header and passes with it. No protected or
pre-existing test was weakened, deleted, skipped or narrowed, and I found no
fixture special-casing or mock substitution.

The findings are a disclosed API-shape deviation/potential guard workaround
for the planner to ratify (finding 1), two test-strength gaps (finding 2 and
finding 3), and a missing positive constructor unit test plus handoff
overstatement (finding 4). None hides the defect the task was cut to fix or
removes a contract assertion in a way that lets the gate pass falsely. I would
block only if the plan treats the fixed-arity `New` as immutable; as written
the packet is contradictory and the worker disclosed the compromise, so the
correct disposition is merge-with-follow-up rather than another gate cycle.

VERDICT: PASS
