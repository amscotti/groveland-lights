# T-102 — reviewer (correctness / security / architecture) — attempt 5

Scope: the attached diff (4 files: `internal/web/csrf.go`, `internal/web/csrf_test.go`,
`internal/web/middleware_logging_test.go`, `harness/handoffs/T-102.md`) reviewed against
`harness/tasks/T-102.md`, `acceptance/contract.json` (A-102/A-105), the protected
acceptance specs, the current `internal/web` source (`web.go`, `context.go`,
`web_test.go`, `templates.go` — the diff adds files only, so these ARE the production
code under test), `cmd/server/main.go`, and the harness (`cmd/harness/{sched,guard}.go`,
`harness/policy.json`). Shell execution is permission-denied in this environment, so I
did not re-run check-fast/e2e myself; note, however, that the scheduler generates this
diff from `git diff main...HEAD` in the worktree only AFTER guard and `make check-fast`
pass there (`cmd/harness/sched.go:244-265`), so the diff under review is by construction
the committed state and its suite is green on it. Every code-level claim below was
verified by direct source trace, including a fresh independent trace of the disputed
logging chain.

## The blocker: re-confirmed real, pre-existing, outside owns, escalated per packet

- `internal/web/web.go:62-63` composes `h = s.withRequestID(h)` then
  `h = s.withLogging(h)` — id-assigner INNER, logger OUTER.
  `withRequestID` reassigns only its local request (`web.go:117`
  `r = r.WithContext(...)`), so `withLogging` still holds the pre-enrichment request
  when it logs `"req_id", requestIDOf(r.Context())` (`web.go:141`), and
  `internal/web/context.go:28-33` returns the `"-"` no-id sentinel. Every served
  request therefore logs `req_id:"-"` while the response `X-Request-Id` header
  carries the real 16-hex id (`context.go:18-22`: 8 rand bytes → 16 hex chars).
  `cmd/server/main.go:43` installs `srv.Handler()` unchanged, so production does the
  same. A-105 (`acceptance/contract.json:34-37`) requires "… and request id", and
  `"-"` is the explicit no-id sentinel; the protected acceptance test still passes
  because `acceptance/go/m1_guards_test.go:275-277` checks only non-empty.
- `web.go` is T-100's, and the packet forbids editing it
  (`harness/tasks/T-102.md:29-32`), prescribing exactly the `BLOCKED` escalation the
  handoff's first line delivers, with root cause, line-accurate references (all four
  verified correct), the one-line owner fix (swap `web.go:62-63`), and the
  post-fix migration of the unit assertion onto `s.Handler()`.

## Response to the prior BLOCK (reviewer-alt, round 2) — verified row by row

Reviewer-alt's verdict sanctioned two acceptable end states: a failing assertion
through `Handler()`, "or by removing/renaming the weak assertions so they are not
represented as coverage". The delivered state is the second, and every disposition
claim in the handoff is accurate against the final diff:

- **The served-chain `req_id` assertions are gone, not weakened further.**
  `TestWithLoggingRequestFields` asserts method, `pattern=` key presence
  (`strings.Contains`) + value, logged status cross-checked against the actual
  `rec.Code` and against the case's expected code, `dur_ms >= 0`, and a real 16-hex
  response `X-Request-Id` — the id-assigner half, which is genuinely coverable and
  covered (the header is set at `web.go:116` before `next`, on the same header map
  the recorder sees). It makes NO assertion on the logged `req_id`, with a comment
  saying exactly that and why. `jsonRequestLine` likewise checks no `req_id` (neither
  value nor key presence), with a pointing comment. No `t.Logf` probe remains
  anywhere in the file.
- **The composition test is renamed and honestly scoped.**
  `TestWithLoggingLogsContextRequestID` (ex-`...RealRequestID`) is documented in its
  doc comment as a unit test of `withLogging`'s OWN contract ("NOT production
  coverage"), instructing the `web.go` owner to move the assertion onto
  `s.Handler()` after the swap. This framing is legitimate: the defect is in
  `Handler()`'s composition (`web.go:62-63`), not in either middleware unit, and the
  test both pins `withLogging`'s unit behaviour and pre-verifies the handoff's
  proposed fix. The handoff no longer calls it "the real guard"; its coverage
  statement now leads with "**No test in `internal/web` asserts the served chain's
  logged `req_id`.**"
- Secondary round-2 findings: "full field set" doc reworded to "every field
  coverable through the served chain" with the exclusion named; `TestCSRFIssue` is
  now a genuine table (one `run func` row per flow); the idempotency row closes
  `rec2.Result()`'s body before counting cookies; the `-v`-only `t.Logf` is deleted.
  All confirmed in the diff.
- The "why no red test" argument is structurally accurate: the scheduler hard-fails
  on check-fast before reviews (`cmd/harness/sched.go:254-259`), so a deliberately
  red test could never reach a review cycle, let alone the merge gate. With
  `max_attempts: 3` (`harness/policy.json:31`) exhausted, a red suite would terminally
  stall T-102 → T-103/T-105 without producing the one thing that fixes the contract:
  the `web.go` owner edit.

The information asymmetry reviewer-alt objected to is gone at every level: no test,
comment, or handoff sentence in this diff represents the served chain's `req_id` as
covered.

## Packet compliance — verified

- **Frozen API exact** (T-102.md:42-55): `IssueCSRFToken(w, r) string`,
  `var ErrCSRF = errors.New("csrf: token mismatch")`, `VerifyCSRF(r, submitted) error`,
  `CSRFTokenName = "csrf_token"`. 32 `crypto/rand` bytes as unpadded base64url
  (43 chars); cookie `Path=/, HttpOnly, SameSite=Lax, Secure` iff
  `serverFrom(r.Context()).Cfg.IsProd()`; idempotent reuse of a well-formed request
  cookie with no redundant Set-Cookie; stdlib only — `allow_deps: []` respected
  (imports of both new files verified: crypto/{rand,subtle}, encoding/base64,
  errors, net/http and test-side stdlib + `internal/config`).
- **All packet-required test cases present** (T-102.md:57-61) with the required
  names: missing-cookie / empty-submission / tampered-value / valid-round-trip /
  idempotent-per-cookie / cookie-flags-prod-vs-test, plus extra rows (spaces,
  non-base64url, padded, over-long, under-length, malformed-cookie-even-when-matched)
  and the frozen-literal pin `TestCSRFTokenNameFrozen` (round-1 finding 3).
- **Middleware guard tests** (T-102.md:63-77): `bufferedServer` constructs the logger
  exactly as `cmd/server/main.go:70-75` does (JSONHandler iff prod, TextHandler
  otherwise — verified line by line); served-chain tests run the real
  `s.Handler().ServeHTTP`; no-IP assertions cover both `RemoteAddr` and an injected
  `X-Forwarded-For: 203.0.113.77` in text AND JSON modes, matching A-105's
  "never the IP".
- **e2e claim shape verified**: only `@A-102` has a Playwright spec
  (`acceptance/e2e/home_m1.spec.ts:18`); `make e2e-task` greps the task's claims with
  `--pass-with-no-tests` (`Makefile:111-116`), so A-105 contributing no spec is
  expected, as the packet states ("do not expect a red→green acceptance flip"). The
  diff adds library/test files only — no route, template, or static change exists
  that could regress the already-green A-102/A-105 specs.
- **Ownership clean**: the diff is scheduler-generated from `main...HEAD`
  (`sched.go:265`), so its four files are the complete change set:
  `csrf.go`/`csrf_test.go` under owns `internal/web/csrf*.go`,
  `middleware_logging_test.go` under owns `internal/web/middleware*.go`, plus the
  auto-allowed handoff (`guard.go:133`). `web.go`, `context.go`, `templates.go`,
  `web_test.go` untouched; no protected path (`harness/policy.json:37-51`) near the
  diff.

## Security

- **CSRF correct for the mandated pattern.** Strict `RawURLEncoding`
  well-formedness on BOTH cookie and submission (rejects padding, non-alphabet
  garbage, invalid lengths), ≥32-byte entropy floor on both sides (the
  "malformed cookie rejected even when matched" row exercises the cookie side),
  256-char DoS bound on hostile fields/cookies, `subtle.ConstantTimeCompare`,
  uniform `ErrCSRF` (no cause oracle for the 403 path), entropy failure → 500 +
  `""` rather than rendering a form with an empty token. Cookie flags pinned per
  env by `TestCSRFCookieFlags` (`Secure=false` outside prod is load-bearing for the
  frozen http-based test flows). `IssueCSRFToken`'s documented requirement for the
  server in context matches `serverFrom`'s panic (`context.go:42-47`), and
  `withServer` is outermost in `Handler()` (`web.go:66`), so every real request
  satisfies it.
- **Inherited, packet-mandated limitation (unchanged from prior reviews):** the
  token is unsigned and session-unbound, so cookie fixation by anything that can
  set an origin `csrf_token` cookie passes verification. Inherent to the
  packet-mandated "Double-submit cookie pattern, stdlib only" (T-102.md:44);
  acceptable for the M1 probe counter, must be revisited when sessions land before
  anything security-relevant relies on it.
- No XSS surface (no `template.HTML`; tokens are base64url, escaped by
  `html/template` downstream), no SQL, no path traversal, no uploads, no secrets in
  output (the "k" test keys are pre-existing `web_test.go:21`, not this diff), no
  client-IP leakage (asserted absent in both log modes). The single
  `//nolint:gosec` (SetCookie) carries a same-line reason (AGENTS rule 9).

## Architecture

- Ownership map respected: `internal/web` owns CSRF and middleware
  (`docs/spec/architecture.md:19`). No routes, handlers, templates or static files;
  the `#map`/HTMX rule is untouched by construction. Parse-don't-validate / typed
  IDs not implicated (library task); the handoff explicitly instructs T-103 to
  parse the form into a typed struct at the boundary before `VerifyCSRF`
  (AGENTS rule 6). No migrations, `status.Derive` untouched, no "Phase N" strings
  in code, no `console.log`, no TODO/FIXME.

## Tests exercise real behaviour

- CSRF tests drive real `Server`s via `httptest` with the server injected at the
  same context key the chain uses (`ctxServer`), covering mint, reuse-without-
  Set-Cookie, reissue-on-malformed-cookie, both Secure polarities,
  shape/length-preserving tamper, entropy floor, uniqueness, and the frozen
  `csrf_token` literal. Referenced helpers exist with the used signatures
  (`testConfig`/`newTestServer`, `web_test.go:17-35`; `ctxServer`,
  `context.go:15`).
- Middleware tests run the real chain (`s.Handler().ServeHTTP`) against a buffered
  slog handler and parse actual text/JSON output; logged status is cross-checked
  against the response recorder in both modes; `pattern=` key presence is
  distinguished from empty value; recover/panic→500+logged, header==context id and
  per-request id uniqueness pin the real units from `web.go`.
- `TestWithLoggingLogsContextRequestID` runs the corrected composition over a real
  mux; its `pattern` assertion relies on Go 1.27's ServeMux setting
  `Request.Pattern` on the request object `withLogging` holds (the same behaviour
  `web.go:130` reads as its primary source). I could not execute the suite here
  (shell denied), but this exact composition and assertion passed guard/check-fast
  in attempt 4 and is unchanged in substance, and the current attempt reached
  review, which the scheduler only allows after `check_fast_ok` (`sched.go:254-260`).

## Findings

1. **Pre-existing, disclosed, escalated — must survive the merge record.** Every
   served request logs `req_id:"-"` while the response header carries the real id
   (trace above). Not introduced by this diff; unfixable within owns; escalated via
   the packet's own `BLOCKED` channel with a verified one-line fix. **Required
   follow-up for the `web.go` owner:** swap `web.go:62-63` (or have `withLogging`
   read `rec.Header().Get("X-Request-Id")` after `next`), then move
   `TestWithLoggingLogsContextRequestID`'s assertion onto `s.Handler()` as the
   handoff prescribes; the acceptance owner should consider tightening
   `acceptance/go/m1_guards_test.go:275-277` from non-empty to the real-id shape.
   A PASS here certifies the diff, not the served chain's request-id clause.
2. **Minor — scope of the composition test.** It pins the two middleware units in
   the corrected arrangement, not the production `Handler()`; recorded so the gate
   does not over-read it. Its own doc comment and the handoff say the same.
3. **Minor, inherited — unsigned/session-unbound double-submit token** (see
   Security); revisit with the sessions work before security-relevant use.
4. **Verification limits:** shell denied — check-fast, coverage %, e2e and guard
   exits are from the handoff; structurally corroborated by the scheduler ordering
   (guard + check-fast precede diff generation and reviews, `sched.go:244-265`), and
   the merge gate re-runs everything.

## Checked and not found

No edits to protected paths (`acceptance/`, `cmd/harness/`, Makefile, CI,
`.golangci.yml`, goldens, `harness/policy.json`); no scratch/probe files committed;
no `t.Skip`/`test.only`/`.fixme` markers; no `t.Logf` residue; no deleted or
narrowed pre-existing tests (the diff is pure additions; the ratchet counts only
increase); no new dependencies; no `template.HTML`; no `console.log`; no IP/secret
leakage; no HTMX/`#map` surface; no handlers added; migrations untouched; no
fixture special-casing in non-test code.

## Position on the verdict

The diff introduces no defect, weakens nothing, claims nothing false, and delivers
every packet deliverable with tests that exercise the real code paths. The one
unsatisfied contract clause is a verified pre-existing defect in a file the packet
explicitly forbids this task to edit, escalated through the packet's own `BLOCKED`
channel — and, decisive this round, the previous BLOCK's test-integrity objections
(assertions that rubber-stamped `"-"`; a composition test misrepresented as the
production guard) are resolved exactly as reviewer-alt's round-2 verdict sanctioned:
the weak assertions are removed outright, and what remains is honestly labelled
non-coverage at every level. Blocking again would demand either an owns violation or
a permanently red suite that the scheduler structurally cannot merge, while the
contract fix requires the `web.go` owner either way. The CSRF library that T-103
compiles against is correct, secure for the mandated pattern, and well tested.

VERDICT: PASS
