# T-102 — reviewer-alt (test integrity / reward hacking)

Scope: attached `harness/reviews/T-102.diff` (four files: `internal/web/csrf.go`,
`internal/web/csrf_test.go`, `internal/web/middleware_logging_test.go`,
`harness/handoffs/T-102.md`) reviewed against `harness/tasks/T-102.md`,
`acceptance/contract.json` (A-105), `internal/web/web.go`/`context.go`, and
`cmd/server/main.go`. Shell execution is permission-denied in this environment,
so I did not re-run `make check-fast`/e2e; the code, handoff and the current
worktree were traced directly.

## Blocking: the served-chain request-id assertions still pass on `-`

- `TestWithLoggingRequestFields` runs the real `s.Handler()` chain
  (`internal/web/middleware_logging_test.go:87`) but its only request-id
  assertion is `f["req_id"] != ""`
  (`internal/web/middleware_logging_test.go:129-131`). Because `web.go:61-67`
  composes `withLogging` outside `withRequestID`, the logger reads the
  pre-enrichment request (`web.go:117,141`) and logs the `-` sentinel
  (`context.go:28-33`). The test is therefore green on exactly the defective
  value. The `t.Logf` at
  `internal/web/middleware_logging_test.go:128` is not an assertion, and it is
  suppressed unless tests run with `-v`.
- `jsonRequestLine` makes the same non-empty-only check
  (`internal/web/middleware_logging_test.go:248-250`), so
  `TestProdLoggingIsJSON` — which does run the real `s.Handler()` chain
  (`internal/web/middleware_logging_test.go:262`) — certifies a request line
  with `req_id="-"` as acceptable. A-105 requires a request id
  (`acceptance/contract.json:34-37`); `-` is the explicit no-id sentinel, not
  one.
- The handoff now leads with `BLOCKED` and openly documents this non-coverage
  (`harness/handoffs/T-102.md:1,47-51`). That is transparent, but it does not
  change the artifact: the owned unit tests are still green while the served
  chain violates the contract, so a PASS would report coverage that does not
  exist. The explicit note is an escalation, not a fix.

## Blocking: the new "real-id guard" tests a different chain, not production

- `TestWithLoggingLogsRealRequestID` composes
  `s.withRequestID(s.withLogging(mux))`
  (`internal/web/middleware_logging_test.go:158`) — the fixed order — and then
  asserts the logged id equals the response header
  (`internal/web/middleware_logging_test.go:173-175`). It never calls
  `s.Handler()`, so it cannot fail for the actual production composition. It
  is a reasonable unit test of `withRequestID` and `withLogging` in isolation,
  but it is not a regression guard for the behavior under test (the real
  chain), and it is precisely the "compose around the bug" pattern the prior
  review blocked.
- The handoff calls this test "the real guard the review demanded"
  (`harness/handoffs/T-102.md:52-59`), and the test comments claim the full
  contract is pinned by it
  (`internal/web/middleware_logging_test.go:125-127,246-247`). That overstates
  what the test proves: it shows the two middleware units can satisfy the
  contract when arranged correctly, not that the application does. The
  handoff itself concedes "only the production arrangement in web.go does not"
  (`harness/handoffs/T-102.md:57-59`).

## Consequence

The suite still has no test that fails for the known A-105 defect.
`TestProdLoggingIsJSON` runs the production-shaped chain and passes with
`req_id="-"`; the protected acceptance test also only checks key presence
(`acceptance/go/m1_guards_test.go:275-276`). The handoff's first line is
`BLOCKED` (correct), but the green tests and their comments present the
request-id clause as guarded. Under the test-integrity rules this is a BLOCK:
the block should be represented by a real assertion that fails through
`Handler()` (the task is allowed to stay blocked), or by removing/renaming the
weak assertions so they are not represented as coverage — not by adding a
passing test that changes the composition.

## Secondary findings

1. `TestProdLoggingIsJSON`'s doc says request lines carry "the full field set"
   (`internal/web/middleware_logging_test.go:254-257`), but `req_id` is only
   key-present via `jsonRequestLine`
   (`internal/web/middleware_logging_test.go:248-250`). The comment overstates
   the assertion.
2. `TestCSRFIssue` remains named subtests rather than table-driven
   (`internal/web/csrf_test.go:132+`); the packet asks for table-driven tests.
   Case names are still assertions, so this is a minor packet deviation, as
   the handoff notes.
3. `rec2.Result().Cookies()` in the idempotency subtest
   (`internal/web/csrf_test.go:149`) does not close the response body, unlike
   `csrfCookieOf` (`internal/web/csrf_test.go:49-51`). Lint/cleanup nit, not
   blocking; the handoff claims `make check-fast` is green.
4. The handoff's claim that the `-v` line keeps the defect visible is weaker
   than it sounds: `t.Logf` is only emitted in verbose runs, so a normal
   `make check-fast` prints nothing.

## Checked and not found

- No new `t.Skip`/`test.skip`/`test.only`/`.fixme` markers, no deleted or
  narrowed pre-existing tests, no protected-path edits, no new dependencies,
  no fixture special-casing, no `console.log`, no TODO/FIXME in added lines;
  the single `//nolint:gosec` in `internal/web/csrf.go:54` carries a same-line
  reason.
- The CSRF implementation and tests are genuine and not mocked: missing,
  empty, tampered, garbage, padded, over-long and under-length submissions,
  malformed-cookie reissue, the 32-byte entropy floor, prod/test cookie flags,
  constant-time comparison, and uniform `ErrCSRF` are all exercised. The only
  weaker-than-contract assertions are the request-id ones above.
- The blocker itself is real and pre-existing: `web.go` is outside this task's
  owns, and the one-line reorder in the handoff
  (`harness/handoffs/T-102.md:67-83`) is the right owner fix. The problem is
  not the escalation; it is that the tests added in response do not guard the
  production path and still pass on `-`.

VERDICT: BLOCK
