# T-103 — reviewer-alt (test integrity / reward hacking)

Scope: the attached diff (`cmd/server/boot_test.go`, `cmd/server/main.go`,
`internal/web/probe.go`, `internal/web/probe_test.go`, the two templates and
`harness/handoffs/T-103.md`) read against `harness/tasks/T-103.md`, the frozen
drafts (`acceptance/drafts/probe_go_test.go.txt`,
`acceptance/drafts/probe.spec.ts.txt`), `Makefile`,
`internal/web/web.go`, `internal/web/templates.go`, `internal/web/csrf.go`,
`internal/store/{store,migrations}.go`, `acceptance/e2e/*.spec.ts` and
`harness/policy.json`. Shell execution is permission-denied in this session,
so the `make check`/`go test` exit codes are taken from the handoff; the code,
tests and gate definitions were traced directly.

## Findings

### 1. [Medium] The two boot tests are skipped by every automated gate

`cmd/server/boot_test.go:1` carries `//go:build smoke`. No target in the
Makefile runs `go test -tags smoke`:
`test-unit` is `go test -race -count=1 -shuffle=on ./...` (`Makefile:64-66`),
`test-acceptance-go` is `-tags acceptance` (`Makefile:103-105`), and the
`smoke` target is the Playwright `--grep @smoke` suite (`Makefile:194-197`).
`make check` (`Makefile:135-136`) therefore never compiles
`cmd/server/boot_test.go`. The two tests that carry the only runtime evidence
for the A-103 restart path and the A-104 boot refusal
(`cmd/server/boot_test.go:302` and `:377`) are effectively skipped by default.
The handoff is transparent about the manual invocation
(`harness/handoffs/T-103.md:1,133-143`), and the Makefile is protected, but
the open issues never ask the Makefile/harness owner to wire a `go test -tags
smoke` target or move the test into the acceptance suite. Until that happens,
the tests can rot and a regression in `cmd/server/main.go`'s boot refusal
will not fail the gate. This is the one real "skipped test" in the diff.

### 2. [Medium] The new count regex deliberately differs from the frozen draft; A-103 #2/#7 still fail

The frozen Go draft pins `probeReCount = regexp.MustCompile("Count:\\s*(\\d+)")`
over the raw HTML (`acceptance/drafts/probe_go_test.go.txt:52`). The new
tests instead parse the span (`internal/web/probe_test.go:23`,
`cmd/server/boot_test.go:41`), because the packet-frozen markup is
`<p>Count: <span id="probe-count">N</span></p>`
(`harness/tasks/T-103.md:49`). That is a truthful response to an inconsistent
packet/draft pair, and the handoff documents it (`harness/handoffs/T-103.md:103-116`);
but it means the green unit/smoke tests cannot detect the draft mismatch. The
only test that would is the inert draft, which is protected. This is a
narrowed test relative to the contract (the new test passes while the frozen
assertion fails), not a hidden weakening, but it must be resolved by the
planner before T-105.

### 3. [Medium] The no-JS home section (A-103 #12) is not implemented

`internal/web/probe_test.go:297-315` pins only the empty shell and the
`<a href="/probe">Probe</a>` link. The frozen TS draft requires `GET /` to
server-render the heading, live `Count: N` in `#probe-count`, and a live-form
CSRF token (`acceptance/drafts/probe.spec.ts.txt:73-94`). The handoff admits
the gap (`harness/handoffs/T-103.md:66,70-101`) and explains why the owned files
cannot inject live data through `homeData`/`index.html`; `web.go` is out of
scope. This is a contract gap, not a test hack, but it is real: the task's
A-103 claim cannot be validated as delivered.

### 4. [Medium] The POST store-error 500 path is untested

The packet requires POST store errors to map to 500
(`harness/tasks/T-103.md:87-88`). `internal/web/probe.go:147-151` has that
branch, but `TestProbePost` (`internal/web/probe_test.go:202-294`) only tests
success, CSRF rejection, the no-JS 303 and the parse 400. There is no test for
a closed/unset store on POST, so a regression that returns 200/403 on
`probeInc` failure would pass. The handoff's "store unset/error → 500" row
(`harness/handoffs/T-103.md:41`) covers the GET handler only.

### 5. [Low] The "500 with a log line" contract is not asserted; the handoff overclaims

`internal/web/probe.go:92-95` and `:149-150` log the store failure, but
`internal/web/probe_test.go:179-198` asserts only the 500 status; no logger
output is captured. A regression deleting the `s.Log.Error` calls would pass.
The handoff says "both log lines asserted via 500"
(`harness/handoffs/T-103.md:41`), which is not true. The packet requires the
log line (`harness/tasks/T-103.md:82-83`), so this is an assertion weaker than
the contract.

### 6. [Low] The hx-attribute assertions are unscoped substring checks

`internal/web/probe_test.go:135-176` and `:297-315`, and
`cmd/server/boot_test.go:306-314`, check `hx-get`, `hx-trigger`,
`hx-target` and `hx-swap` with `strings.Contains` anywhere in the body. They do
not prove the attributes are on `<section id="probe">` (or that the form's
`hx-post`/`hx-target`/`hx-swap` are on the `<form>`). A regression moving an
attribute to a sibling element would still pass. The template is correct; the
test is weaker than the packet's "section containing the four hx attributes"
clause (`harness/tasks/T-103.md:41-43`).

### 7. [Low] POST success only re-parses the count, not "the same fragment"

`internal/web/probe_test.go:203-222` and `cmd/server/boot_test.go:340-351`
assert status 200 and the incremented `#probe-count`, but not that the POST
response is the fragment (heading, form, hidden token). The contract says
"200 with the same fragment" (`harness/tasks/T-103.md:56`). The GET test covers
the fragment; a POST-specific rendering regression that returned only the
count would pass. The frozen TS draft would catch it on activation, but the
unit/smoke tests do not.

### 8. [Low] The tampered-checksum test can hang; the tamper is heuristic

`cmd/server/boot_test.go:399-400` runs `cmd.Run()` with no deadline. If the
byte flip does not trigger the checksum refusal, the child starts listening and
the test hangs instead of failing. The rest of the file is careful with bounded
waits (`bootWaitHealthy`/`bootStop`). In addition, `bootTamper`
(`cmd/server/boot_test.go:276-296`) assumes the first 64-hex token in the DB is
`schema_migrations.sha256`; a more targeted `SELECT sha256 ... WHERE
version='0001_init'` would be stronger. Neither is a false-green (a wrong
tamper fails the refusal assertion), but it is a fragile test.

### 9. [Low] Handoff accuracy: "11 of 12" and the active-suite a11y claim are overstated

The handoff counts the two regex-dependent draft assertions (#2 and #7) as one
"sanctioned mechanical" issue and calls #10/#11 green
(`harness/handoffs/T-103.md:1,56-66`), but #2 and #7 are two distinct
assertions and #10 is not executable until T-105. The claim that
`@A-000 @a11y` passes "with the HTMX-filled section present"
(`harness/handoffs/T-103.md:64,143`) is also not supported by the active test:
`acceptance/e2e/home.spec.ts:13-19` does not wait for the probe heading, so the
axe run can happen before `hx-trigger="load"` inserts the fragment.
`acceptance/e2e/home_m1.spec.ts:35-52` likewise checks console/CSP at `load`,
not after the XHR. The active 20/20 green is real, but it does not prove the
populated fragment is clean; the @A-103 draft is the only test that does that.

### 10. [Low] Process: the handoff is `BLOCKED (partial)` rather than `BLOCKED: ...`

The hard rule says to write `BLOCKED: <reason>` and stop. The first line is
`BLOCKED (partial)` (`harness/handoffs/T-103.md:1`) and the worker continued to
deliver. The escalation is honest and the two gaps are documented, so this is
process hygiene rather than a hidden hack, but the exact handoff contract was
not followed.

## Checked and not found

- No `t.Skip`, `test.skip`, `test.only`, `.fixme`, TODO/FIXME or `console.log`
  in the diff. The only skip is the `smoke` build tag (finding 1).
- No protected file touched: `acceptance/**`, `Makefile`, `.golangci.yml`,
  `cmd/harness/**` and golden files are untouched; `harness/handoffs/T-103.md`
  is the permitted handoff.
- No existing test was deleted or weakened. All changes are additions except
  the `main.go` boot wiring and the static `index.html` mount.
- No mock bypasses the behaviour under test: `probe_test.go` uses a real
  `store.Open` temp-file DB, the full `s.Handler()` middleware chain, and the
  real CSRF/store code; `boot_test.go` spawns the real `bin/server`.
- No fixture-value special-casing in production logic: `Probe`, `Count:`,
  `#probe-count`, `Increment counter`, `csrf_token` and `/probe` are the
  feature's frozen markup/route strings, not branches in `probe.go`/`main.go`.
- No dead code to satisfy linters: `errProbeStoreUnset`, `probeData`,
  `renderPartial`, `bootReHex`, `bootServer.dbPath/out` are all used.
- No new dependency; `go.mod`/`go.sum` are absent from the diff.
- The core assertions can fail: the counter must be exactly `n+1`, rejected
  tokens must be 403 with an unchanged count, the no-JS POST must be 303 to
  `/`, the restart must preserve `n+1`, and the tampered DB must exit non-zero
  with `0001_init.sql`/`checksum` in the output.

## Verdict

The work is honest about the two frozen-contract gaps and does not weaken or
edit a protected test. The defects above are coverage/skip/scoping gaps and
handoff overclaims, not a hidden special-case or a mock that bypasses the
behaviour. The handoff's `BLOCKED` escalation is the correct mechanism for
findings 2 and 3; this verdict is a test-integrity verdict, not an A-103
acceptance verdict, and the planner must resolve the regex/no-JS gaps before
T-105.

VERDICT: PASS
