# T-102 — reviewer-alt (test integrity / reward hacking) — attempt 4

Scope: attached diff (`internal/web/csrf.go`, `internal/web/csrf_test.go`,
`internal/web/middleware_logging_test.go`, `harness/handoffs/T-102.md`)
reviewed against `harness/tasks/T-102.md`, `acceptance/contract.json`
(A-105), `acceptance/go/m1_guards_test.go`, `internal/web/web.go`,
`internal/web/context.go`, and `cmd/server/main.go`. Shell execution was
permission-denied here, so the `check-fast`/e2e/coverage exits below are
taken from the handoff; the code, tests and handoff claims were traced
directly.

## Findings

No blocking test-integrity failure.

- **The prior blocking issue is resolved.** The served-chain tests no longer
  assert the logged `req_id` at all, so they no longer certify the `-` sentinel
  as an acceptable request id. `TestWithLoggingRequestFields` documents the
  deliberate non-coverage and asserts only the real 16-hex response
  `X-Request-Id` (`internal/web/middleware_logging_test.go:122-135`), and
  `jsonRequestLine` removes the previous key-presence check with the same
  explanation (`internal/web/middleware_logging_test.go:257-261`). The
  remaining request-id assertion is confined to `withLogging` in the corrected
  composition (`internal/web/middleware_logging_test.go:147-185`), explicitly
  marked **NOT production coverage** and handed to the `web.go` owner. The
  handoff leads with `BLOCKED` (`harness/handoffs/T-102.md:1`) and its coverage
  statement is accurate (`harness/handoffs/T-102.md:38-69`). This is the
  review-sanctioned alternative to a red test: the weak assertions are gone,
  not represented as production coverage.
- **The blocker is real and pre-existing, not introduced here.** `web.go:62-63`
  composes `withLogging` outside `withRequestID`, so `withLogging` logs
  `requestIDOf` from the pre-enrichment request (`web.go:117,141`) and every
  served request logs `req_id="-"` while the response header gets the real id;
  `cmd/server/main.go:43` installs that chain unchanged. `web.go` is outside
  this task's `owns`, and the handoff escalates it with a correct one-line fix.
- **The CSRF tests exercise the real implementation.** `TestCSRFVerify`
  (`internal/web/csrf_test.go:87-125`) is a genuine 10-row table covering
  missing cookie, empty/garbage/padded/over-length/under-entropy submissions,
  a tampered value, a malformed-but-matching cookie, and the valid round trip.
  `TestCSRFIssue` (`internal/web/csrf_test.go:131-190`) drives real issue,
  idempotent-reuse and reissue flows (including no redundant `Set-Cookie`),
  `TestCSRFCookieFlags` (`internal/web/csrf_test.go:194-225`) covers prod/test
  `Secure` plus `Path`, `HttpOnly` and `SameSite`, and `TestCSRFTokenNameFrozen`
  (`internal/web/csrf_test.go:78-82`) pins the contract-frozen `csrf_token`
  literal. No mock bypasses the behavior under test.
- **No deleted/skipped/narrowed pre-existing tests, no coverage theatre.** All
  files in the diff are additions; the `withRecover`, `withRequestID` and
  `withLogging` tests can fail on the behavior they assert. No `t.Skip`,
  `test.skip`, `test.only`, `.fixme`, `TODO`/`FIXME`, `t.Logf` probe,
  `console.log`, fixture special-case or dead test helper remains in the diff.
- **Minor:** `jsonRequestLine` checks `method` only for non-emptiness
  (`internal/web/middleware_logging_test.go:239-241`), while the text-mode test
  checks the exact `GET` value (`internal/web/middleware_logging_test.go:104-105`).
  Not blocking, since the exact method is asserted in the text path.
- **Minor:** the `"under-length submission rejected"` row uses `"short"`
  (`internal/web/csrf_test.go:109`), which Go's raw-base64 decoder rejects by
  length before the 32-byte entropy floor is consulted; the floor is actually
  exercised on the cookie side by the matched short-cookie row
  (`internal/web/csrf_test.go:116`). The case name remains truthful.
- **Minor coverage gap:** `validCSRFToken` relies on Go's base64 decoder
  (`internal/web/csrf.go:96`), which ignores CR/LF, and no test rejects a token
  containing an embedded newline despite the contract saying
  "base64url-shaped". The string compare still prevents a clean-cookie /
  newline-submission mismatch, and cookie fixation is the acknowledged
  inherited limitation of the packet-mandated unsigned double-submit pattern;
  a `token+"\n"` rejection row would close the gap without changing the
  verdict.
- **Verification limit:** `make check-fast`, coverage and e2e exits are taken
  from the handoff because shell execution was denied in this environment. The
  code and handoff claims are internally consistent and trace correctly against
  the current `web.go`/`context.go`.

VERDICT: PASS
