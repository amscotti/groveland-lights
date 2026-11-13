# T-102 — reviewer (correctness / security / architecture) — attempt 4

Scope: the attached diff (4 files: `internal/web/csrf.go`,
`internal/web/csrf_test.go`, `internal/web/middleware_logging_test.go`,
`harness/handoffs/T-102.md`) reviewed against `harness/tasks/T-102.md`,
`acceptance/contract.json` (A-102/A-105), the protected acceptance specs, the
current `internal/web` source, `cmd/server/main.go`, and the harness
(`cmd/harness/{sched,gate,guard,globmatch}.go`, `harness/policy.json`,
`harness/state.md`). Shell execution is permission-denied here, so
check-fast/coverage/e2e exits are taken from the handoff; every code-level
claim below was verified directly against source, including a fresh
independent trace of the disputed logging behaviour.

## The blocker: independently confirmed real, pre-existing, and outside owns

- `internal/web/web.go:62-63` composes `h = s.withRequestID(h)` then
  `h = s.withLogging(h)` — the id-assigner is INNER, the logger OUTER.
  `withRequestID` enriches only its local request (`web.go:117`
  `r = r.WithContext(...)`), so `withLogging` logs
  `requestIDOf(r.Context())` (`web.go:141`) from the pre-enrichment request
  and `internal/web/context.go:28-33` returns the `"-"` no-id sentinel. Every
  served request therefore logs `req_id: "-"` while the response
  `X-Request-Id` header carries the real 16-hex id.
  `cmd/server/main.go:43` installs `srv.Handler()` unchanged, so production
  violates the intent of A-105's "request id" clause
  (`acceptance/contract.json:33-38`), even though the frozen, protected
  acceptance spec passes — `acceptance/go/m1_guards_test.go:275-277` asserts
  only non-empty, and `"-"` is non-empty.
- The defect originates in T-100's `web.go`, which this task is forbidden to
  edit (`harness/tasks/T-102.md:29-32`: "`web.go` is NOT in your owns — do
  not edit it. If you believe it needs a change, write `BLOCKED: <reason>`
  instead"). I verified it is genuinely unfixable within owns
  (`internal/web/csrf*.go`, `internal/web/middleware*.go`): `Handler()` in
  `web.go` is the only middleware-composition point, Go permits no method
  override from another file, and `init()` registers routes, not middleware.
- The worker used exactly the packet's prescribed escape hatch: the handoff's
  first line is `BLOCKED: web.go request-ID chain order needs an owner fix`,
  with root cause, the precise one-line fix (swap the two compositions in
  `Handler()` so the id exists before the logger runs), and the alternative
  fix (logger reads `rec.Header().Get("X-Request-Id")` after `next`).
- The proposed fix is correct and pre-verified:
  `TestWithLoggingLogsRealRequestID` composes exactly
  `s.withRequestID(s.withLogging(mux))` — the swapped order — and asserts the
  logged `req_id` equals the response `X-Request-Id` and matches
  `^[0-9a-f]{16}$`. After the owner's swap this is the production
  arrangement, so the fix cannot silently fail.

## Response to the prior BLOCK (reviewer-alt, attempt 3)

- **Real-id guard added.** `TestWithLoggingLogsRealRequestID`
  (middleware_logging_test.go) pins what reviewer-alt demanded: logged id ==
  response header, real 16-hex shape, never the `-` sentinel — on the two
  middleware units composed the way a conforming chain must compose them. If
  the logger stops logging the context id, or the assigner stops setting
  header/context, it goes red.
- **Explicit non-coverage note delivered** — the alternative reviewer-alt's
  own verdict sanctioned ("with a failing test or an explicit non-coverage
  note"). Both served-chain assertions (text and JSON) now carry comments
  marking them as knowingly non-covering for `req_id`, plus a `t.Logf` probe
  printing the observed sentinel under `-v`, plus the handoff disclosure.
- **Why not a red test:** structurally infeasible to merge. The scheduler
  hard-fails on `make check-fast` before reviews run
  (`cmd/harness/sched.go:254-259`), and the merge gate runs `make check`
  (`cmd/harness/gate.go:98-105`). A permanently red suite can never pass a
  review cycle. With `max_attempts: 3` (`harness/policy.json:31`) and this
  being attempt 4, another BLOCK terminally blocks T-102 → T-103 → T-105
  (graph deps) for a one-line fix in a file this task is forbidden to touch.
  Declining to pin `-` explicitly was also correct — that would bless the
  sentinel as contract.
- The served-chain tests do not present themselves as covering the request-id
  clause; the information asymmetry reviewer-alt objected to (gate told
  "guarded" while it is not) is gone — it is first-line-of-handoff loud.

## Packet compliance — verified

- **Frozen API exact** (T-102.md:42-55): `IssueCSRFToken(w, r) string`,
  `var ErrCSRF = errors.New("csrf: token mismatch")`,
  `VerifyCSRF(r, submitted) error`, `CSRFTokenName = "csrf_token"`
  (csrf.go:20,44,34,69 per the diff). 32 `crypto/rand` bytes → unpadded
  base64url (43 chars); cookie `Path=/, HttpOnly, SameSite=Lax, Secure` iff
  `IsProd()`; idempotent reuse of a well-formed request cookie; stdlib only —
  `allow_deps: []` respected (verified imports).
- **All required test cases present** (T-102.md:57-61): missing-cookie /
  empty-submission / tampered / valid-round-trip / idempotent-per-cookie /
  cookie-flags-prod-vs-test — `TestCSRFVerify` is a genuine 10-row table with
  case names as assertions; `TestCSRFIssue` stays as named subtests with a
  documented reason (multi-step flows), which satisfies the packet's intent.
- **Middleware guard tests** (T-102.md:63-77): run the real `s.Handler()`
  chain, parse actual slog text/JSON output, cross-check logged status
  against `rec.Code` AND the case's expected code (both modes), assert
  `pattern=` key presence even when the value is empty, no-IP on the exact
  A-105 payloads (`192.0.2.1` RemoteAddr, `203.0.113.77` XFF), prod JSON via
  the exact `slog.NewJSONHandler(buf, nil)` construction
  `cmd/server/main.go:70-75` uses.
- **Ownership clean**: changed files are `csrf.go`, `csrf_test.go` (owns
  `internal/web/csrf*.go`), `middleware_logging_test.go` (owns
  `internal/web/middleware*.go`), and the auto-allowed handoff
  (`cmd/harness/guard.go:133`); `*` is `[^/]*` (`cmd/harness/globmatch.go:42`)
  so the test files are covered. `web.go`, `context.go`, `templates.go`,
  `web_test.go` untouched. No protected path near the diff
  (`harness/policy.json:37-51`). Test count only increases; no
  skip/only/fixme markers added (`guard.go:159-169` would catch them).
- **Markers/lint**: no TODO/FIXME/panic markers; the single `//nolint:gosec`
  (csrf.go SetCookie) carries a same-line reason (AGENTS rule 9); no
  `console.log`, no `template.HTML`, no `Phase N` strings, no new deps.

## Security

- **CSRF correct for the mandated pattern.** Strict `RawURLEncoding`
  well-formedness on BOTH cookie and submission (rejects padding, non-alphabet
  garbage, invalid lengths), ≥32-byte entropy floor on both sides (the
  "malformed cookie rejected even when matched" row exercises the floor on
  the cookie side too), 256-char DoS bound, `subtle.ConstantTimeCompare`,
  uniform `ErrCSRF` (no cause oracle for the 403 path), entropy failure → 500
  + `""` rather than rendering a form with an empty token. Cookie flags pinned
  per env by `TestCSRFCookieFlags` — `Secure=false` outside prod is
  load-bearing for the frozen A-103 drafts (http + cookie jar in test env).
- **Inherited, packet-mandated limitation (carried from the prior review,
  unchanged):** the token is unsigned and session-unbound, so cookie fixation
  by anything that can set an origin `csrf_token` cookie (subdomain
  injection, non-TLS hop) passes verification. This is inherent to the
  packet-mandated "Double-submit cookie pattern, stdlib only"
  (T-102.md:44) and acceptable for the M1 probe counter; it must be
  revisited with the Phase 2 sessions work (state.md:73-75) before anything
  security-relevant relies on it.
- No XSS surface (base64url tokens, escaped by `html/template` downstream; no
  `template.HTML`), no SQL, no path traversal, no uploads, no secrets in
  output; IP non-leak assertions match A-105 byte-for-byte.
- `IssueCSRFToken` requires the server in context (`serverFrom` panics
  otherwise, `context.go:42-47`) — documented in its doc comment and in the
  handoff for T-103; `withServer` is outermost in `Handler()` so every real
  request satisfies it.

## Architecture

- Ownership map respected (`docs/spec/architecture.md:19`: `internal/web`
  owns CSRF). No routes, handlers, templates or static files — the `#map`
  HTMX rule is untouched by construction. Parse-don't-validate / typed IDs
  not implicated (library task); the handoff explicitly instructs T-103 to
  parse the form into a typed struct at the boundary before `VerifyCSRF`
  (AGENTS rule 6). No migrations. `status.Derive` untouched.

## Tests exercise real behaviour

- CSRF tests drive real `Server`s via `httptest` with the server injected at
  the same context key the chain uses (`ctxServer`), exercising mint,
  reuse-without-Set-Cookie, reissue-on-malformed-cookie, both Secure
  polarities, tamper (shape/length-preserving flip mirroring the frozen
  A-103 draft's `probeTamper`), entropy floor, and uniqueness.
- Middleware tests run the real chain (`s.Handler().ServeHTTP`) and parse
  actual slog output; `TestWithLoggingLogsRealRequestID` runs the corrected
  composition over a real mux (verified: `s.mux` is nil there, so pattern
  resolution comes from `r.Pattern`, which Go's ServeMux sets on the request
  object `withLogging` itself holds in that composition — Go 1.27, the
  pattern behaviour is standard). Recover/panic, header-vs-context id, and
  per-request id uniqueness all test the real units from `web.go`.
- Referenced helpers exist: `testConfig` / `newTestServer`
  (`internal/web/web_test.go:17-35`), `ctxServer` (`context.go:15`),
  `withRecover`/`withRequestID`/`requestIDOf` (`web.go:101-120`,
  `context.go:28`).

## Findings

1. **Pre-existing, disclosed, escalated — requires owner action, must not be
   lost by this PASS:** every served request logs `req_id: "-"` while the
   response header carries the real id (trace above). Not introduced by this
   diff; unfixable within owns; the handoff's `BLOCKED` first line is the
   packet-prescribed escalation and includes the verified one-line fix.
   **Required follow-up for the integrator/web.go owner:** apply the
   `web.go:62-63` swap (or the header-read alternative), and have the
   acceptance owner consider tightening
   `acceptance/go/m1_guards_test.go:275-277` from key-presence to the real-id
   shape (`^[0-9a-f]{16}$`, or equality with the response `X-Request-Id`).
   The `state.md:73-75` Phase 2 `web.go` refactor is the natural vehicle if
   no earlier owner-fix lands. A PASS here certifies the diff, not the
   served chain's request-id clause.
2. **Minor — scope of the new real-id guard.** `TestWithLoggingLogsRealRequestID`
   pins the two units in the corrected composition, not the production
   `Handler()` arrangement (which is the defect). That is the strongest
   green-suite guard possible; recorded here so the gate does not over-read
   it as end-to-end coverage.
3. **Minor, inherited — `rec2.Result()` body not closed** in the idempotency
   subtest (`csrf_test.go`, reuse path, `len(rec2.Result().Cookies())`).
   Recorder `Result()` bodies are in-memory; the handoff reports lint exit 0
   and the gate re-runs `make check`.
4. **Minor — handoff line-reference drift:** the check-fast hard-fail is at
   `cmd/harness/sched.go:254-259` (handoff says 250-253); A-105 sits at
   `acceptance/contract.json:33-38` (handoff says 34-37). Substance of both
   claims verified correct.
5. **Verification limits:** no shell here — `make check-fast`, coverage %,
   e2e and guard exits are from the handoff (cross-checked against harness
   source: `max_attempts`, coverage floor 70, `node_modules` allowlist
   behaviour, `--pass-with-no-tests` for the Playwright-only A-105 grep).
   The gate re-runs everything on merge.

## Position on the verdict

The diff introduces no defect, weakens no pre-existing test (all tests are
net additions; nothing on main was narrowed), adds no dependency, respects
every ownership and marker rule, and delivers each packet deliverable with
tests that exercise the real code paths. The one unsatisfied contract clause
is a verified pre-existing defect in a file the packet explicitly forbids
this task to edit, escalated through the packet's own `BLOCKED` channel with
root cause, exact fix, and a unit test that pre-verifies the fixed
composition. Reviewer-alt's sanctioned alternative to the (structurally
unmergeable) failing test — the explicit non-coverage note — is delivered at
every level. Blocking again would demand an impossible action and terminally
stall T-103/T-105 to no corrective effect: the fix requires the `web.go`
owner either way, and this review + the handoff keep that action on the
record. The CSRF library itself — which T-103 compiles against — is correct,
secure for the mandated pattern, and well tested.

## Checked and not found

No edits to protected paths (`acceptance/`, `cmd/harness/`, Makefile, CI,
`.golangci.yml`, goldens, `harness/policy.json`); no scratch or probe files
committed; no `t.Skip`/`test.only`/`.fixme`; no `console.log`; no
`template.HTML`; no IP/secret leakage; no HTMX/`#map` surface; no handlers
added; migrations untouched; no fixture special-casing in non-test code.

VERDICT: PASS
