# T-103 — reviewer-alt (test integrity / reward hacking)

Scope: the attached diff (`harness/reviews/T-103.diff`) read against
`harness/tasks/T-103.md`, the protected drafts (`acceptance/drafts/probe_go_test.go.txt`,
`acceptance/drafts/probe.spec.ts.txt`, `acceptance/drafts/README.md`,
`acceptance/drafts/migrations_go_test.go.txt`), `harness/tasks/T-105.md`,
`Makefile`, `scripts/acceptance-go.sh`, `internal/web/{web,templates,csrf}.go`,
`internal/store/{store,migrations}.go`, `cmd/server/main.go`, the active e2e
specs and `harness/handoffs/T-103.md`.

Verification limit: the diff is not applied to this checkout (the worktree is
at the pre-change `main`), and shell execution is restricted in this session
(`git`/`go`/`ls` are denied; only shell builtins ran). The `make check-fast` /
`make check` / e2e exit codes below are therefore taken from the handoff; every
claim about the code was traced statically against the current sources.

## Findings

### 1. [Medium] The only boot-level tests are excluded from every gate

`cmd/server/boot_test.go:1` carries `//go:build smoke`. No target runs that
tag: `Makefile:64-66` (`test-unit`) is `go test ... ./...` with no tags;
`Makefile:135-136` (`check`) has no `-tags smoke` step; `Makefile:194-197`
(`smoke`) is the Playwright `--grep @smoke` suite; the Go acceptance target is
`-tags acceptance` (`scripts/acceptance-go.sh:36`). The handoff is transparent
about the manual invocation (`harness/handoffs/T-103.md:36`) and the full-gate
row (`:152`), but the tests that carry the only runtime evidence for the A-103
restart path (`cmd/server/boot_test.go:353-360`) and the A-104 boot refusal
(`:377-408`) are invisible to `check-fast`, `make check` and CI. The open
issues never ask for a `go test -tags smoke` target. Until one exists this is
an additive suite that cannot fail in the gate.

### 2. [Medium] The 500 rows cannot prove the nil guard and no log line is asserted

`internal/web/probe_test.go:179-187` and `:189-199` assert only the status
code. If the nil-store guard in `internal/web/probe.go:70-75` were deleted,
`probeCount` would dereference a nil `*store.DB`; `withRecover`
(`internal/web/web.go:101-111`) would catch that panic and answer 500 as well,
so the test would still pass. The packet requires "store unset or error → 500
with a log line" (`harness/tasks/T-103.md:82-83`); no test captures the logger
or checks the `s.Log.Error` calls at `internal/web/probe.go:93`/`:149`.
Deleting either log statement would pass. The handoff's "both log lines
asserted via 500" (`harness/handoffs/T-103.md:55`) overclaims the assertion
strength.

### 3. [Medium] The POST store-error 500 branch is untested

`internal/web/probe.go:147-151` maps a `ProbeInc` failure to 500. `TestProbePost`
(`internal/web/probe_test.go:202-292`) covers success, missing/tampered CSRF,
the no-JS 303 and the parse 400, but never closes the store before a valid
POST. A regression returning 200 or 403 on `ProbeInc` failure would pass. The
packet requires store errors → 500 (`harness/tasks/T-103.md:87-88`); this is
an explicit handler clause left unguarded.

### 4. [Medium] The no-JS home clause is not implemented; the new tests pin the weaker shell

`web/templates/pages/index.html:10-12` emits only the empty shell and the plain
link. The frozen TS draft requires `GET /` with JS off to server-render the
heading, a live `Count: N` in `#probe-count`, and a submittable form with a
live CSRF token (`acceptance/drafts/probe.spec.ts.txt:76-94`,
`acceptance/drafts/README.md:42`); the packet also says the home page shows
the new count after the 303 (`harness/tasks/T-103.md:58-59`).
`TestProbeHomeMount` (`internal/web/probe_test.go:297-317`) and the boot mount
assertions (`cmd/server/boot_test.go:310-314`) assert only the shell/link, and
the no-JS POST row (`internal/web/probe_test.go:254-271`) asserts the 303 and
the store count but not that `/` renders N+1. The handoff's gap (a)
(`harness/handoffs/T-103.md:84-115`) is accurate; this is a packet/draft
contradiction the worker cannot resolve within owns, not a hidden test hack,
but the new tests must not be read as covering the frozen no-JS contract.

### 5. [Medium] The tampered-checksum boot test can hang instead of fail

`cmd/server/boot_test.go:400` runs `_ = cmd.Run()` with no deadline. If the
byte flip at `:276-302` does not produce a mismatch (the first 64-hex token
is not the checksum, a future migration adds an earlier token, WAL state), the
child boots and `Run` blocks until the go-test global timeout; no cleanup
kills it. The frozen draft turns "keeps running" into a bounded failure with
a goroutine plus 15s (`acceptance/drafts/migrations_go_test.go.txt:281-295`).
The first-index heuristic is also weaker than tampering the recorded
`schema_migrations` row. The test fails (not passes) on a miss, but it can
wedge the suite.

### 6. [Medium] The proposed gap (a) fix cannot work as described

`harness/handoffs/T-103.md:106-115` proposes changing `index.html` to
`{{template "partials/probe.html" .}}` as a one-owner (`web.go`) fix.
`internal/web/templates.go:44-60` parses each page with only `base.html` and
the page file; `partials/probe.html` is parsed into a separate `partials` map
and is not associated with the page template. The include would fail at
execution ("no such template") unless `templates.go` is also changed or the
partial markup is inlined with the new `homeData` fields. The escalation
should name `templates.go` (or an inline duplicate) as part of the fix; as
written the planner may schedule an owner that still cannot satisfy the
clause.

### 7. [Low] The hx-* checks are unscoped substrings

`internal/web/probe_test.go:135-161`, `:297-317` and
`cmd/server/boot_test.go:310-314` use `strings.Contains` for `hx-get`,
`hx-trigger`, `hx-target` and `hx-swap` anywhere in the body. They do not
prove the attributes are on `<section id="probe">` (or the form's `hx-post` /
`hx-target` / `hx-swap` on the `<form>`). Moving an attribute to a sibling
still passes, so the assertion is weaker than the packet's "section
containing the four hx attributes" (`harness/tasks/T-103.md:41-43`).

### 8. [Low] POST success only re-parses the count, not "the same fragment"

`internal/web/probe_test.go:203-223` and `cmd/server/boot_test.go:339-351`
assert 200 and `#probe-count` N+1, but not that the POST response is the
fragment (heading, form, hidden token). The contract says "200 with the same
fragment" (`harness/tasks/T-103.md:56`). A POST-specific render regression
returning only the count would pass here; the TS draft would catch it at
activation.

### 9. [Low] Count-regex disposition overreaches; handoff overclaims

- `probeReCount = Count:\s*(\d+)` over raw HTML
  (`acceptance/drafts/probe_go_test.go.txt:52`) cannot match the
  packet-frozen `<p>Count: <span id="probe-count">N</span></p>`
  (`harness/tasks/T-103.md:49`). The worker's span regex
  (`internal/web/probe_test.go:23`) matches the packet and the TS draft, so
  the mismatch is a real draft defect, not a hidden weakening. But the handoff
  calls the T-105 regex edit "sanctioned"
  (`harness/handoffs/T-103.md:128-135`), while
  `acceptance/drafts/README.md:29-32` says the A-104 token index is "the only
  sanctioned edit". The planner must explicitly authorize the regex change
  (or re-freeze the string); A-103 Go #2/#7 fail at activation as written.
- The handoff says "11 of the 12" (`:1`) while its own table marks #2, #7 and
  #12 ❌ (`:70`, `:75`, `:80`) — nine ✅.
- The a11y claim (`:79`) says the active @A-000 @a11y test passes "with the
  HTMX-filled section present"; `acceptance/e2e/home.spec.ts:13-19` never
  waits for the probe heading, so axe may run before the `hx-trigger="load"`
  XHR inserts the fragment. The inert @A-103 a11y test does wait
  (`acceptance/drafts/probe.spec.ts.txt:64-71`); the green does not prove the
  populated fragment is clean.
- The scratch makefile (`harness/handoffs/T-103.md:205`) is an extra write
  under `harness/` beyond the permitted handoff file; disclosed, untracked and
  guard-exempt, but not part of AGENTS.md rule 1's exception. The
  `test-run.db*` cleanup it performs is a real `.gitignore` follow-up
  (`playwright.config.ts:56`), not something this task's owns can fix.

### 10. [Low] Handoff `BLOCKED (partial)` is not the AGENTS.md form

`harness/handoffs/T-103.md:1` starts `BLOCKED (partial):`, not
`BLOCKED: <reason>`. The escalation is honest and the substance is present;
the literal prefix is not.

## Checked and not found

- **No deleted or weakened pre-existing test.** The diff only adds files; no
  `acceptance/` or existing `*_test.go` file is modified.
- **No `t.Skip`, `test.skip`, `test.only`, `.fixme`, TODO/FIXME or
  `console.log`.** The only skip-like mechanism is the additive `smoke` build
  tag (finding 1).
- **No mock bypasses the behaviour under test.** `probe_test.go` drives
  `s.Handler()` (the real middleware chain) against a real `store.Open`
  temp-file SQLite DB wired through the same `SetProbeStore` main uses;
  `boot_test.go` spawns the real `bin/server` on a real port.
- **No fixture-value special-casing in production code.** `Probe`, `Count:`,
  `#probe-count`, `Increment counter`, `csrf_token` and `/probe` are the
  feature's frozen markup/route strings, not branches in `probe.go` or
  `main.go`.
- **No dead code to satisfy linters.** `errProbeStoreUnset`, `probeData`,
  `renderPartial`, `bootReHex`, `bootServer.dbPath/out`, `syncBuf` and the
  test helpers are all used.
- **No protected path touched.** `acceptance/**`, `Makefile`,
  `.golangci.yml`, `cmd/harness/**` and golden files are absent from the
  diff; `harness/handoffs/T-103.md` is the permitted handoff.
- **No new dependency.** `go.mod`/`go.sum` are absent from the diff;
  `allow_deps: []`.
- **The core assertions can fail.** The counter must be exactly `n+1`;
  rejected tokens must be 403 with an unchanged count; the no-HX POST must be
  303 to `/`; the restart must preserve `n+1`; the tampered DB must exit
  non-zero with `0001_init.sql` and `checksum` in the output.

## Verdict rationale

The implementation is correct on the packet's explicit code contract, and the
tests drive the real middleware/store/binary rather than a mock. No protected
test was weakened, deleted, narrowed or edited; the two frozen-contract gaps
are packet/draft contradictions the worker escalated instead of papering over.
The medium findings are gate-visibility, negative-path assertion strength and
an incomplete fix sketch, not hidden special-cases; findings 1-6 should be
fixed or explicitly tracked, and the planner must resolve the regex and no-JS
dispositions before T-105. A BLOCK of this worker would force it to fix a
planner-owned contract, so the code is acceptable to merge with the above
conditions recorded for T-105/the planner.

VERDICT: PASS
