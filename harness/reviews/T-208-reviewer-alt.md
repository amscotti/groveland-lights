# T-208 — independent review (reviewer-alt)

Basis: the attached diff, packet `harness/tasks/T-208.md`, judge-M1 gap (b)
(`harness/findings/judge-M1.md:80-87,113-114`), the pre-change sources on disk
(`internal/web/{web,probe,csrf,web_test,probe_test,middleware_logging_test}.go`,
`cmd/server/{main,boot_test}.go`, `web/templates/**`), and the active probe
contracts (`acceptance/go/probe_test.go`, `acceptance/e2e/probe.spec.ts`).

Verification limit: the root checkout is still at the pre-change code
(`internal/web/probe.go` on disk still declares `probeStore`/`SetProbeStore`),
so the diff is the review surface. I verified that the diff's pre-image matches
the on-disk files for all six pre-existing code paths; line numbers below refer
to the post-patch side of the diff. Shell execution is denied in this session,
so I did not personally re-run `make check`, the smoke suite, or the red-check.
Those claims are evaluated mechanically against the code and the templates.

## Test-integrity / reward-hacking checks

- **No deleted, skipped, narrowed or disabled test.** Comparing the diff with
  the pre-change `internal/web/probe_test.go` on disk, every row is retained:
  the fragment-markup row, the `<html>/<body>/id="map"` ban, the closed-store
  500, valid-token increment (fragment count and DB count), missing/tampered
  token 403 with the count unchanged, the no-JS 303 + `Location: /`, the
  unparseable-body 400, and the home-mount markup row. Only helper signatures
  (`newProbeServer`, `probeCountNow`) and the "store unset" → "store nil" name
  changed. No `t.Skip`/`test.skip`/`test.only`/`.fixme` appears in the diff.
- **No fixture-value special-casing in production.** The only new production
  literal is `probeContentType = "text/html; charset=utf-8"`
  (`internal/web/probe.go:24`), the packet-mandated header; there is no branch
  on test data. No acceptance-fixture string is promoted into non-test code.
- **No mocks bypassing the behaviour under test.** Probe rows use the real
  `s.Handler()` chain and a real temp-file SQLite store opened through
  `store.Open`; `probeCountNow` reads that real store. The new Content-Type
  test uses `httptest.NewServer` with a real `net/http` client and cookie jar,
  not a `ResponseRecorder` (`internal/web/probe_test.go:334-419`). That matters:
  sniffing happens only on a real server write, so a recorder assertion could
  not have caught this defect.
- **The new test is load-bearing.** The fragment starts `<h2>Probe</h2>`
  (`web/templates/partials/probe.html:1`); `<H2` is not in Go's HTML sniff
  signature set, so without the explicit header the wire response is
  `text/plain; charset=utf-8`. Removing either
  `w.Header().Set(probeContentType)` line therefore makes the exact assertion
  at `internal/web/probe_test.go:414-415` fail. (This is the mechanical reason
  the worker's red-check succeeds; I did not execute it here.)
- **No dead code to satisfy linters.** `probeContentType`,
  `errProbeStoreNotWired` and `probeReToken` all have live uses; the
  `len(db) > 0` branch is exercised by every caller.
- **No protected path or dependency change.** The diff touches
  `cmd/server/{main,boot_test}.go`, `internal/web/{web,probe,probe_test,web_test}.go`
  and the handoff. `acceptance/**`, `Makefile`, CI, `.golangci.yml`, goldens and
  `go.mod/go.sum` are absent. `net/http/cookiejar` is stdlib and
  `internal/store` is intra-module, so `allow_deps: []` is respected.
- **The header placement is safe.** Both sets happen immediately before
  `renderPartial`, which buffers (`internal/web/probe.go:52-67`); a render
  failure writes nothing first, so `http.Error` can overwrite the type with its
  own plain-text 500. The no-JS 303 branch is untouched, as the packet allows.

## Findings

### 1. [Medium — disclosed API deviation, planner must ratify] `New` is variadic, not the frozen fixed-arity signature

The packet freezes
`func New(cfg config.Config, log *slog.Logger, db *store.DB) (*Server, error)`
(`harness/tasks/T-208.md:36-38`). Delivered:
`func New(cfg config.Config, log *slog.Logger, db ...*store.DB) (*Server, error)`
(`internal/web/web.go:46`), with `len(db) > 0` selecting `db[0]`
(`internal/web/web.go:51-55`). The forcing constraint is real:
`internal/web/middleware_logging_test.go:29` calls 2-arg `New`, that file is
outside the packet's `owns` (`harness/tasks/T-208.md:11`), and a fixed third
parameter cannot compile it. With Go's lack of default arguments, the packet's
"literal 3-param signature" and "diff inside owns" requirements are mutually
unsatisfiable; the variadic is the only shape that keeps main's untouched test
compiling and preserves the frozen call form `New(cfg, log, db)`.

That said, this is the closest thing in the diff to gaming a constraint: the
signature was shaped to satisfy the ownership guard, not the packet, and it
removes the compiler-enforced store-wiring guarantee. A future production
caller can write `New(cfg, log)` and get a nil `Server.Store` with runtime 500s
instead of a compile error; `New(cfg, log, db1, db2)` silently ignores `db2`.
I do not treat this as a test-integrity blocker because no test assertion was
weakened, production still passes `db` (`cmd/server/main.go`), `s.Store` remains
the frozen access path, and the black-box smoke/acceptance suites exercise the
real wiring. The planner should either ratify the variadic form explicitly or
extend `owns` to `internal/web/middleware_logging_test.go` and restore the
literal signature.

### 2. [Medium-Low — the new constructor path has no unit test] `New(cfg, log, db)` is never called with a non-nil store by any test in the diff

`newProbeServer` builds the server with `newTestServer` (nil store) and then
assigns `s.Store = db` directly (`internal/web/probe_test.go:34-43`);
`newTestServer` passes explicit `nil` (`internal/web/web_test.go:30-32`). No
test exercises the positive branch `st = db[0]` at `internal/web/web.go:51-55`.
Consequently a constructor bug (e.g. dropping the assignment and leaving
`Store` nil) survives `make check-fast` and the packet's own
`go test ./internal/web/... ./cmd/server/... -count=1`; only the smoke-tagged
boot suite and the acceptance spawns (which call `New(cfg, logger, db)`) would
catch it. This is a real gap in the central deliverable ("store onto Server"),
and it makes the handoff's "Frozen API" sentence — "Production and every
store-backed test call it exactly as the packet freezes: `New(cfg, log, db)`" —
inaccurate for the unit tests. A one-row `TestNewWiresStore` (`New(cfg, log,
db)` → `s.Store == db`) would close it and make the handoff true.

### 3. [Low-Medium — assertion weaker than the packet's contract] The nil-store 500 row cannot distinguish the nil guard from a recovered panic, and the required message is untested

`internal/web/probe_test.go:184-193` asserts only that `GET /probe` with a nil
store returns 500. The guard it is presented as pinning is
`if s.Store == nil { return 0, errProbeStoreNotWired }`
(`internal/web/probe.go:59-64`). If that guard were deleted,
`s.Store.ProbeCount` would panic (or error) and `withRecover`
(`internal/web/web.go`, unchanged) would still write 500, so the row would
still pass. The packet additionally requires the message
`probe: store not wired` (`harness/tasks/T-208.md:39-41`); no test asserts it.
The row predates T-208 in coarse shape, so this is not a newly introduced
weakening, but it is weaker than the contract it is cited against. A direct
unit test of `(*Server).probeCount` with a nil store, or a captured-logger
assertion, would pin the real guard and message.

### 4. [Low — test robustness] The wire-level POST row follows redirects and can pass on the home page

`internal/web/probe_test.go:403` constructs `client := &http.Client{Jar: jar}`
with no `CheckRedirect`; the status/type assertions are at
`internal/web/probe_test.go:411-415`. If `probePost` regressed to a 303 for an
`HX-Request` POST, the client would follow to `/`, which starts
`<!DOCTYPE html>` (`web/templates/base.html:1`) and sniffs as
`text/html; charset=utf-8` — both the 200 and exact-header assertions would
pass on the redirect target. `TestProbePost` still catches a 303 today, so this
is not a live gate hole, but the new test's claim to pin both fragment
transports is weaker than stated. Mirror
`acceptance/go/probe_test.go:177-183` (`CheckRedirect` returning
`http.ErrUseLastResponse`) and/or assert the body contains `<h2>Probe</h2>`
before checking the header.

### 5. [Low — comment accuracy] The new `probeGet` comment calls the fragment a "full HTML document"

`internal/web/probe.go:91-93` says "The fragment is a full HTML document for a
no-JS visitor who navigates here directly." It is not: `TestProbeGet` asserts
the fragment contains no `<html`/`<body` (post-patch
`internal/web/probe_test.go` fragment-ban row). The intended meaning is that
the response body is the whole fragment. Comment-only; no behavioural impact.

### 6. [Info — handoff overstatement and minor hygiene]

- `internal/web/probe_test.go:34-43` also claims `s.Store` is set "exactly as
  main's `web.New` call does"; it bypasses the constructor (finding 2). The
  frozen-API claim in the handoff should be corrected to say unit helpers
  assign `Store` directly.
- `New(cfg, log, db1, db2)` silently ignores `db2` (`internal/web/web.go:51-55`);
  no caller does this. If the variadic form is ratified, either document the
  first-argument rule or reject `len(db) > 1`.
- The new test's client has no timeout (`internal/web/probe_test.go:403`); a
  hung server would wait for the Go test timeout rather than fail promptly.
- The handoff's attempt/topology narrative is unusually long and is not part of
  the code review surface. Its claims about `sched.go`/`gate.go` are plausible
  but were not re-verified here.

## Checked and not found

- No weakened, deleted, skipped or narrowed test; the only renamed subtest
  keeps the same 500 assertion on the same `Server.Store == nil` path.
- No fixture literals or branch-on-fixture in non-test code.
- No mocks substituting for the behaviour under test: real SQLite, real
  middleware chain, real wire-level server.
- No dead code added to satisfy a linter, and no `TODO`/`FIXME`/`panic("not
  implemented")`, "Phase N" string, or unreasoned `//nolint` in added lines.
- `cmd/server/boot_test.go` is comment-only in this diff and remains black-box
  (its imports pull in no `internal/web`), so the signature change cannot
  affect it; the packet's "update boot_test.go" is satisfied vacuously.
- The acceptance contract is unaffected: it asserts body/CSRF behaviour, not
  the fragment `Content-Type` or `SetProbeStore`, so the header addition cannot
  regress A-103/A-104/A-105.
- `errProbeStoreNotWired` is exactly `errors.New("probe: store not wired")`
  (`internal/web/probe.go:27`), matching the packet's message.

## Verdict rationale

Both deliverables are implemented and the Content-Type test genuinely exercises
the wire-level defect at the right level. `SetProbeStore`/`probeStore`/
`errProbeStoreUnset` are gone, `Server.Store` is wired from `cmd/server/main.go`,
nil still answers 500, and `probeGet` plus the HTMX `probePost` branch send the
exact `text/html; charset=utf-8` header. No pre-existing contract assertion was
deleted, skipped or weakened, and I found no fixture special-casing, mock
substitution or coverage theatre that hides the defect.

The findings are a disclosed, guard-driven API-shape deviation for the planner
to ratify (finding 1), one missing positive constructor unit test and a
resulting handoff overstatement (findings 2 and 6), two test-strength gaps
(findings 3 and 4), and a comment inaccuracy (finding 5). None lets the gate
pass a broken deliverable, and the packet is internally contradictory on the
signature. The correct disposition is merge-with-follow-up rather than another
gate cycle.

VERDICT: PASS
