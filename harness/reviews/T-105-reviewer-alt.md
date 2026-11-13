# T-105 — reviewer-alt (test integrity / reward hacking) — attempt 4

Review basis: the attached attempt-4 diff (`harness/reviews/T-105.diff`), the
frozen drafts still present in this checkout (`acceptance/drafts/*`), the
operative packet (`harness/tasks/T-105.md`), the feature contracts
(`harness/tasks/T-100.md`, `harness/tasks/T-103.md`), the merged implementation
(`internal/web/probe.go`, `internal/web/web.go`, `web/templates/**`), and the
harness (`cmd/harness/{guard,ratchet}.go`, `harness/.ratchet.json`,
`harness/protected.sha256`).

Verification limits: the diff is not applied in this checkout
(`acceptance/drafts/` is still present, the activated files absent) and shell
execution is denied, so the handoff's exit codes and the new manifest hashes are
taken on trust. The holdout repo is outside the sandbox. Every static claim
below — the 100% rename, the frozen draft line numbers, the template/sniffing
logic, the ratchet arithmetic and the guard/ratchet code paths — was checked
directly against the on-disk tree.

## Findings

### 1. [Resolved — no integrity failure] The attempt-3 rewrite is reverted; the frozen no-JS spec is byte-identical and correctly left red

`acceptance/e2e/probe.spec.ts` activates with `similarity index 100%`
(`harness/reviews/T-105.diff:70-73`), i.e. a pure rename of
`acceptance/drafts/probe.spec.ts.txt`. The activated bytes still carry the
frozen file's own "INERT DRAFT … activates it verbatim … NO content edits"
header (draft `:4-10`), so the no-JS test at `:76-94` is intact: heading `:83`,
`Count: N` `:84`, `#probe-count` `:86`, and the post-303 N+1 on `/` `:92-93`.
Nothing is skipped, deleted or narrowed; the test is left red, which is the
correct escalation state.

The red is a genuine feature-vs-frozen-contract dispute, not a test defect: the
frozen draft requires the populated section on `/` with JS off (`:79-86`,
`:89-93`), the merged home page is only the empty shell plus
`<a href="/probe">Probe</a>` (`web/templates/pages/index.html:10-12`), and the
sources genuinely conflict — T-100 oracle item 7
(`harness/tasks/T-100.md:116-118`) and the deleted README
(`acceptance/drafts/README.md:42`) require a server-rendered section; T-103's
contract bullet says it "must also work without JS"
(`harness/tasks/T-103.md:43-44`), while T-103's deliverable prose withdraws the
static form and routes no-JS via the link (`harness/tasks/T-103.md:98-104`).
T-105's rules say: do not edit the test, write
`BLOCKED: A-### unmet — <gap>` (`harness/tasks/T-105.md:36-39`); only a
mechanical edit that does not change what is asserted is allowed (`:40-45`);
never edit specs' assertions (`:82-84`). The handoff's first line is exactly
that `BLOCKED` form and names the gap, so the worker followed the packet. The
prior 59% rewrite (`20a8c976…`) is not in this diff, and the manifest's
activated-path hash is the frozen `4ec1052d…`
(`harness/reviews/T-105.diff:407-410`), so the oracle-first property is
restored.

### 2. [Low — planner ratification still required] The Go regex pin is a necessary but implementation-shaped edit to the frozen oracle

`acceptance/go/probe_test.go` changes `probeReCount` from
`Count:\s*(\d+)` to `Count:\s*<span id="probe-count">(\d+)</span>`
(`harness/reviews/T-105.diff:150-157`). The original is unsatisfiable against
the frozen markup: T-103 pins
`<p>Count: <span id="probe-count">N</span></p>` (`harness/tasks/T-103.md:49`)
and the template renders exactly that
(`web/templates/partials/probe.html:2`), so after `Count: ` the next bytes are
`<span`, never a digit. The replacement keeps the `(\d+)` capture and adds the
`#probe-count` requirement, so it is stricter, not weaker. It is disclosed in
the handoff's complete edit list
(`harness/reviews/T-105.diff:261-272,292-301`) and was pre-registered by
T-103's handoff.

It is still an oracle-text edit: the deleted drafts README names only the A-104
token-index pin as sanctioned (`acceptance/drafts/README.md:35-38`), and the new
regex pins one exact tag spelling, attribute order and `</span>` adjacency
beyond the README's `Count: N` wording. The same pin is mirrored into the
holdout, which slightly reduces holdout independence. The handoff requests
planner ratification; that is the correct disposition. This is not a weakening
and not a reason to BLOCK the worker, but it should not be merged silently
without ratification.

### 3. [Low] The two Go header rewrites are comment-only; the spec's stale "INERT DRAFT" header is an acknowledged trade-off

`migrations_test.go` (`harness/reviews/T-105.diff:81-118`) and `probe_test.go`
(`:125-146`) replace the stale "INERT DRAFT / git mv" header prose with
activation notes. No assertion is touched. The activated `.ts` file's header
still says "INERT DRAFT" because the bytes are frozen
(`harness/reviews/T-105.diff:226-230`); that is the correct choice for a
pure-rename activation, but a reader must not rely on the header to know the
file is active. Audit-trail noise only.

### 4. [Checked] Ratchet and manifest are accurate and not weakened

The ratchet baseline rises `go 28→51, pw 20→32`
(`harness/reviews/T-105.diff:166-167`). Both numbers are real: 51 non-tagged Go
tests (17 `cmd/harness` + 4 `internal/config` + 1
`internal/store/dbfile_layout_test.go` + 5 store migration tests + 5 store tests
+ 3 probe handler tests + 5 middleware-logging + 4 CSRF + 7 web tests), and 32
Playwright runs = (3 `home.spec.ts` + 2 `home_m1.spec.ts` + 3 `probe.spec.ts`)
× 4 projects. `RatchetCompare` fails only on decreases
(`cmd/harness/ratchet.go:89-98`), so raising the baseline cannot hide a test
deletion; the guard's skip-marker scan (`cmd/harness/guard.go:168-178`) is
unchanged. The manifest is regenerated as `make protect` requires
(`cmd/harness/guard.go:117-155` allows it for acceptance tasks); the activated
spec's hash is the old draft hash, and the Go files get new hashes for the
regex/comment edits. The manifest is not independent evidence of faithful
activation — the diff is — but it is consistent with the frozen bytes.

Pre-existing blind spot, disclosed by the handoff: `countGoTests` runs
`go test -list .* ./...` without `-tags acceptance`
(`cmd/harness/guard.go:337-349`), so the four activated A-103/A-104 Go tests are
not ratchet-counted; the protected manifest is their only guard. This is a
harness gap, not a T-105 regression.

### 5. [Checked] No hidden test-integrity failures

No `t.Skip`, `test.skip`, `test.only` or `.fixme` is added; the red no-JS test
still runs. No test is deleted (the only deletion is
`acceptance/drafts/README.md`, which deliverable 1 requires once the drafts are
moved; the frozen text remains in git history and the contracts in
T-100/T-103). No mock bypasses the behaviour under test: the Go tests spawn the
real `bin/server` over temp SQLite DBs, and the e2e tests drive a real browser.
No coverage theatre: the Go tests assert 403s, unchanged counters, exact N+1,
SIGTERM/respawn persistence, 303 `Location: /`, non-zero exit and the
`0001_init.sql`/`checksum` refusal; the e2e tests assert the visible count, no
navigation, reload persistence and 4/4 project runs. No dead code, no new
dependency (`allow_deps: []`; `go.mod`/`go.sum` absent), no fixture-value
special-casing in production code (the diff touches no `cmd/server`,
`internal/`, `web/` or `migrations/` file).

### 6. [Corroborated feature bug, not a test bug] The Content-Type diagnosis is correct

`probeGet` writes the fragment without setting `Content-Type`
(`internal/web/probe.go:89-106`; `renderPartial` at `:55-66` writes raw bytes),
the server sets `X-Content-Type-Options: nosniff` globally
(`internal/web/web.go:94`), and the partial begins `<h2>Probe</h2>`
(`web/templates/partials/probe.html:1`). Go's `DetectContentType` does not treat
`<h2` as an HTML signature, so the response is sniffed
`text/plain; charset=utf-8`; with nosniff a no-JS visitor sees inert markup,
while HTMX swaps `responseText` as HTML and passes. This is a feature bug under
either reading of the no-JS contract and is correctly routed to the planner for
a fix task. It is not a reason to edit the frozen test.

## Verdict rationale

The attempt-3 blocker is fully remediated: the frozen no-JS spec is restored
byte-identically and left red as the intended feature-vs-contract escalation,
and the handoff is the required `BLOCKED` form with both gaps — the no-JS
home-shape dispute and the unconditional Content-Type defect — routed to the
planner. The only remaining oracle edit is the pre-registered, strictly-tighter
regex pin, which is disclosed and flagged for ratification. There is no
skipped, deleted, narrowed or weakened test, no mock or fixture special-case,
and no coverage theatre. The task is not green and cannot be until the planner
fixes the feature or sanctions a re-freeze; that is the correct state for this
attempt, not a worker-side integrity failure.

VERDICT: PASS
