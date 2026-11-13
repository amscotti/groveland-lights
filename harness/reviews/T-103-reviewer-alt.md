# T-103 — reviewer-alt (test integrity / reward hacking)

Scope: the attached diff (`cmd/server/boot_test.go`, `cmd/server/main.go`,
`internal/web/probe.go`, `internal/web/probe_test.go`,
`web/templates/pages/index.html`, `web/templates/partials/probe.html`,
`harness/handoffs/T-103.md`) read against `harness/tasks/T-103.md`, the
protected drafts (`acceptance/drafts/probe_go_test.go.txt`,
`acceptance/drafts/probe.spec.ts.txt`, `acceptance/drafts/README.md`),
`Makefile`, `internal/web/{web,templates,csrf,context}.go`,
`internal/store/{store,migrations}.go`, `acceptance/e2e/*.spec.ts` and
`cmd/harness/{guard,ratchet}.go`. Shell execution is permission-denied in this
session, so the `make check`/`go test` exit codes are taken from the handoff;
the code, tests, gate definitions and test helpers were traced directly.

## Findings

### 1. [Medium] The only boot-level tests are excluded from every gate

`cmd/server/boot_test.go:1` carries `//go:build smoke`. No target executes
that tag: `test-unit` is `go test -race -count=1 -shuffle=on ./...`
(`Makefile:64-66`), the Go acceptance target is `-tags acceptance`
(`Makefile:103-105`), the `smoke` target is the Playwright `--grep @smoke`
suite (`Makefile:194-197`), and `make check` (`Makefile:135-136`) has no
`-tags smoke` step. As a result, the tests that carry the only runtime
evidence for the A-103 restart path (`cmd/server/boot_test.go:302`) and the
A-104 boot refusal (`cmd/server/boot_test.go:377`) are invisible to
`check-fast`, `make check` and CI. The handoff is transparent about the manual
invocation (`harness/handoffs/T-103.md:145,161`), and this is additive
evidence rather than a weakened pre-existing test, but the open issues never
ask a Makefile/harness owner to wire a `go test -tags smoke` target or move
the cases into the acceptance suite. Until then this is the one real
"skipped test" in the diff: a regression in `cmd/server/main.go`'s store
wiring or boot refusal can merge green.

### 2. [Medium] The tampered-checksum boot test can hang instead of failing

`cmd/server/boot_test.go:400` runs `_ = cmd.Run()` with no deadline. If the
byte flip at `cmd/server/boot_test.go:276-296` does not produce a checksum
mismatch (wrong token index, a future migration adds a prior 64-hex run,
SQLite rewrites the page), the child boots successfully, `Run` waits forever,
and the test hangs instead of failing. The frozen A-104 draft explicitly
turns "keeps running" into a bounded failure with a goroutine plus a 15 s
timeout (`acceptance/drafts/migrations_go_test.go.txt:281-295`); this
reimplementation drops that protection. If finding 1 is fixed by wiring the
smoke tag into a gate, a guard regression would time out the whole job rather
than report the refusal regression. The first-index heuristic is also weaker
than the store-side `TestMigrationChecksumGuard`, which tampers the recorded
row directly; a targeted `UPDATE schema_migrations` (or the draft's
sanctioned index pin) would be stronger.

### 3. [Medium] The POST store-error 500 branch is untested and the handoff overclaims

The packet requires `POST /probe` store errors to map to 500
(`harness/tasks/T-103.md:87-88`). `internal/web/probe.go:148-153` has that
branch, but `TestProbePost` (`internal/web/probe_test.go:202-294`) only covers
success, CSRF rejection, the no-JS 303 and the parse 400. A regression that
returned 200/403 on a `ProbeInc` failure would pass. The handoff's
"Store unset/error → 500 with log line ✅ TestProbeGet unset/failure rows
(both log lines asserted via 500)" (`harness/handoffs/T-103.md:48`) is false
for POST and false about log lines (see finding 4). This is a coverage gap,
not a hidden bypass, but it leaves one of the packet's explicit handler
clauses unguarded.

### 4. [Medium] The "store unset answers 500" test cannot prove the nil guard or the log line

`internal/web/probe_test.go:179-198` asserts only the status code. If the
nil-store check in `internal/web/probe.go:70-76` were removed, `probeCount`
would panic on the nil `*store.DB`; `withRecover` (`internal/web/web.go:101-111`)
would catch that panic and answer 500 as well, so the test would still pass.
The packet requires "store unset or error → 500 with a log line"
(`harness/tasks/T-103.md:82-83`), and no test captures the logger or checks
the `s.Log.Error` calls at `internal/web/probe.go:93,149`. Deleting either
log statement would pass. The handoff's claim that both log lines are
"asserted via 500" (`harness/handoffs/T-103.md:48`) is an overclaim about
assertion strength.

### 5. [Medium] The no-JS home path is not implemented; the new tests pin the weaker shell

`web/templates/pages/index.html:10-12` emits only the empty shell and a plain
link. The frozen TS draft requires `GET /` to server-render the heading, a
live `Count: N` in `#probe-count`, and a working form with a live CSRF token
when JS is off (`acceptance/drafts/probe.spec.ts.txt:76-94`, and
`acceptance/drafts/README.md:42`). `TestProbeHomeMount`
(`internal/web/probe_test.go:297-315`) and the boot-level mount assertions
(`cmd/server/boot_test.go:310-317`) assert the shell/link, so they pass while
A-103 assertion #12 cannot. The no-JS POST test
(`internal/web/probe_test.go:254-272`) asserts only the 303 and the store
count; it does not show that `/` renders the new count, which is the outcome
the packet itself describes (`harness/tasks/T-103.md:58-59`). The handoff
documents this as gap (a) (`harness/handoffs/T-103.md:77-108`) and correctly
identifies `internal/web/web.go` as the blocker; this is a genuine
packet/draft contradiction, not a hidden test hack, but the new unit tests
must not be read as covering the frozen no-JS contract.

### 6. [Low] The hx-* assertions are unscoped substring checks

`internal/web/probe_test.go:135-176` and `:297-315`, and
`cmd/server/boot_test.go:310-317`, use `strings.Contains` for `hx-get`,
`hx-trigger`, `hx-target` and `hx-swap` anywhere in the body. They do not
prove the attributes are on `<section id="probe">` (or that the form's
`hx-post`/`hx-target`/`hx-swap` are on the `<form>`). Moving an attribute to
a sibling element would still pass, so the test is weaker than the packet's
"section containing the four hx attributes" clause
(`harness/tasks/T-103.md:41-43`). The template is correct; the assertions
could be scoped to the opening tag without changing implementation.

### 7. [Low] POST success only re-parses the count, not "the same fragment"

`internal/web/probe_test.go:202-222` and `cmd/server/boot_test.go:345-356`
assert status 200 and the incremented `#probe-count`, but not that the POST
response is the fragment (heading, form, hidden token). The contract says
"200 with the same fragment" (`harness/tasks/T-103.md:56`). A POST-specific
rendering regression that returned only the count would pass; the frozen TS
draft would catch it at activation, but the unit and smoke tests do not.

### 8. [Low] The count-regex mismatch is a real draft defect, but the handoff's "sanctioned" disposition is not sanctioned

The frozen Go draft pins `probeReCount = Count:\s*(\d+)` over raw HTML
(`acceptance/drafts/probe_go_test.go.txt:52`); the packet freezes
`<p>Count: <span id="probe-count">N</span></p>`
(`harness/tasks/T-103.md:49`), so the draft regex cannot match the packet
string. The new tests correctly read the span
(`internal/web/probe_test.go:23`, `cmd/server/boot_test.go:41`), and this is
not a hidden weakening. However, the handoff calls the fix a "sanctioned
mechanical-selector class" edit (`harness/handoffs/T-103.md:120-125`), while
`acceptance/drafts/README.md:29-32` says the A-104 token-index pin is "the
only sanctioned edit". The planner must explicitly authorize the regex fix
(or re-freeze the contract) before T-105; the worker cannot treat it as
pre-approved. A-103 Go assertions #2 and #7 will fail at activation as
written.

### 9. [Low] Handoff accuracy and process

- The first line is `BLOCKED (partial)` (`harness/handoffs/T-103.md:1`), not
  the AGENTS.md `BLOCKED: <reason>` form; T-102's precedent used the exact
  form. The escalation is honest, but the contract was not followed.
- The intro claims "11 of the 12 frozen A-103 draft assertions are satisfied
  in substance" (`harness/handoffs/T-103.md:1`), while its own table marks
  #2, #7 and #12 as ❌ (`:63,68,73`) — nine ✅, not eleven — and #2/#7 are
  two distinct assertions, not one "sanctioned" issue.
- The a11y claim "active @A-000 @a11y passes 4/4 projects with the
  HTMX-filled section present" (`harness/handoffs/T-103.md:72`) is not
  supported by the active test: `acceptance/e2e/home.spec.ts:13-19` does not
  wait for the probe heading, so axe may run before the `hx-trigger="load"`
  XHR inserts the fragment; `acceptance/e2e/home_m1.spec.ts:35-52` likewise
  checks load-time console/CSP only. The green 20/20 is real but does not
  prove the populated fragment is clean; the inert @A-103 draft does.
- The scratch makefile at `harness/handoffs/T-103-scratch.mk`
  (`harness/handoffs/T-103.md:174-176`) is an extra write under `harness/`
  beyond the permitted handoff file. It is disclosed, untracked and
  guard-exempt (`cmd/harness/ratchet.go:127`), but should not become a
  pattern; the `test-run.db*` artifacts it cleans are a real `.gitignore`
  follow-up (`playwright.config.ts:56`).

## Checked and not found

- **No deleted or weakened pre-existing test.** The diff only adds test code;
  no acceptance/ or existing `*_test.go` file is modified.
- **No `t.Skip`, `test.skip`, `test.only`, `.fixme`, TODO/FIXME or
  `console.log`.** The only skip is the additive `smoke` build tag (finding 1).
- **No mock bypasses the behaviour under test.** `probe_test.go` uses a real
  `store.Open` temp-file SQLite DB and the full `s.Handler()` middleware
  chain; `boot_test.go` spawns the real `bin/server`.
- **No fixture-value special-casing in production code.** `Probe`, `Count:`,
  `#probe-count`, `Increment counter`, `csrf_token` and `/probe` are the
  feature's frozen markup/route strings, not branches in `probe.go` or
  `main.go`.
- **No dead code to satisfy linters.** `errProbeStoreUnset`, `probeData`,
  `renderPartial`, `bootReHex`, `bootServer.dbPath/out`, `syncBuf` and the
  test helpers are all used.
- **No protected path touched:** `acceptance/**`, `Makefile`,
  `.golangci.yml`, `cmd/harness/**` and golden files are absent from the
  diff; `harness/handoffs/T-103.md` is the permitted handoff.
- **No new dependency:** `go.mod`/`go.sum` are absent from the diff;
  `allow_deps: []`.
- **The core assertions can fail.** The counter must be exactly `n+1`;
  rejected tokens must be 403 with an unchanged count; the no-HX POST must
  be 303 to `/`; the restart must preserve `n+1`; the tampered DB must exit
  non-zero with `0001_init.sql` and `checksum` in the output.
- **Verification limit:** `make check-fast`, `make check`, coverage and e2e
  exits are taken from the handoff because shell execution was denied here;
  the code and handoff claims are internally consistent with the current
  `web.go`, `templates.go`, `csrf.go` and store sources.

## Verdict rationale

The work is honest about the two frozen-contract gaps and does not weaken,
delete or edit a protected test. The medium findings are a gate-invisible
smoke suite, a hang-prone boot test, missing POST store-error/log
assertions, and a no-JS contract the task's owns cannot satisfy. None is a
hidden special-case, mock bypass or false-green assertion, but findings 1–4
should be fixed or explicitly tracked before the smoke suite is cited as
merge evidence, and the planner must resolve the regex/no-JS gaps before
T-105.

VERDICT: PASS
