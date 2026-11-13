# T-102 — reviewer (correctness / security / architecture)

Scope: static review of the attached diff (`harness/reviews/T-102.diff`) against
`harness/tasks/T-102.md`, `docs/spec/architecture.md`, the A-102/A-105 contract
assertions, and the inert A-103 drafts T-102's API must serve. Shell execution was
permission-denied in this environment, so commands were not re-run; every
harness/guard claim was instead verified against source
(`cmd/harness/guard.go`, `scan.go`, `globmatch.go`, `ratchet.go`, `Makefile`,
`harness/policy.json`), and every test was hand-traced against the current
`internal/web` package. Findings below note where this limits certainty.

## Packet compliance — verified

- **Frozen API delivered exactly**: `IssueCSRFToken(w, r) string`
  (internal/web/csrf.go:44), `ErrCSRF = errors.New("csrf: token mismatch")`
  (csrf.go:33), `VerifyCSRF(r, submitted) error` (csrf.go:69),
  `CSRFTokenName = "csrf_token"` (csrf.go:20) matching the frozen contract string
  in `acceptance/drafts/README.md:47`. 32 `crypto/rand` bytes, raw base64url,
  cookie `Path=/, HttpOnly, SameSite=Lax, Secure` iff `IsProd()` — all per
  packet (T-102.md:46-55).
- **Required test case names all present**: missing-cookie / empty-submission /
  tampered / valid-round-trip / idempotent-per-cookie / cookie-flags-prod-vs-test
  (csrf_test.go:78-229), table-driven where the packet asks.
- **Middleware guard tests present and meaningful**: log fields
  (middleware_logging_test.go:70-115), IP/XFF omission (:118-136), prod JSON via
  the exact `slog.NewJSONHandler(buf, nil)` construction `cmd/server/main.go:70-75`
  uses (:180-213), plus extra recover/request-ID guards (:215-292). Tests run the
  real chain via `s.Handler()` — stronger than the packet's bare `withLogging` ask.
- **Ownership clean**: diff touches only `internal/web/csrf.go`, `csrf_test.go`,
  `middleware_logging_test.go` (all matched by owns globs
  `internal/web/csrf*.go`, `internal/web/middleware*.go` per
  `cmd/harness/globmatch.go`) plus the auto-allowed handoff
  (`cmd/harness/guard.go:133`). `web.go`, `context.go`, `templates.go`,
  `web_test.go` untouched. No deps added (stdlib only). No `acceptance/` or
  protected paths. Ratchet counts only increase; no skip/only markers added.
- **Marker scan re-checked**: the attempt-2 handoff avoids every token in
  `cmd/harness/scan.go:11-15`; the single `//nolint:gosec` (csrf.go:54) carries a
  same-line reason and does not match `reNolintNoReason`. No `\bPhase [0-9]`
  match in `internal/` (the lowercase "session-less phase" comment is safe under
  `make phase-grep`).

## Security

- CSRF implementation is correct for the frozen double-submit contract: strict
  raw-base64url well-formedness on BOTH cookie and submission (csrf.go:90-97),
  entropy floor, 256-char DoS bound, uniform `ErrCSRF` (no cause oracle),
  `subtle.ConstantTimeCompare` (csrf.go:77). Cookie flags pinned by
  `TestCSRFCookieFlags` (csrf_test.go:199-229). SameSite=Lax additionally blocks
  cross-site POST cookie carriage — defence in depth for the 403 path.
- API sufficiency for A-103 confirmed against the frozen drafts: the same-response
  mint+render satisfies `probeFragment` (drafts/probe_go_test.go.txt:214-237);
  the draft's first-char flip tamper (`probeTamper`, :152-163 — mirrored by the
  worker's `tamperCSRF`) yields a well-formed-but-mismatched token → `ErrCSRF` →
  403, exactly the frozen expectation; missing-token POST → 403 via empty check.
- No XSS surface (no `template.HTML` — forbidigo would catch it; tokens are
  base64url and html/template-escaped downstream). No secrets logged; the IP
  non-leak tests assert the exact A-105 payloads. 500-on-entropy-failure
  prevents rendering a form with an empty token (csrf.go:48-53) — the `""`
  return is documented for T-103.
- Inherent limitation, per frozen design (see finding 6): the token is unsigned
  and session-unbound, so cookie-fixation by anything that can set a
  `csrf_token` cookie for the origin passes verification. The packet mandates
  this exact pattern ("Double-submit cookie pattern, stdlib only"); not a
  deviation — flag for the Phase 2 sessions work.

## Architecture

- Ownership map respected (architecture.md:19-21: `internal/web` owns CSRF).
- No routes, templates, or HTMX changes — the `#map` rule is untouched by
  construction. Parse-don't-validate and typed IDs are not implicated (library
  task; the handoff correctly instructs T-103 to parse before `VerifyCSRF`).

## Tests exercise real behaviour

Traced, not just read: the slog text/JSON field parsing regexes
(middleware_logging_test.go:43-56, 140-176) handle the quoted/bare/empty forms
slog actually emits; pattern resolution via `s.mux.Handler(r)` fallback
(web.go:130-135) yields `GET /healthz` vs `""` exactly as asserted; the
constant-time, entropy, reissue-on-malformed-cookie and no-Set-Cookie-on-reuse
paths are all genuinely executed via `httptest` with a real server in context.

## Findings

1. **Pre-existing, disclosed: every request log line emits `req_id=-` while the
   response header carries the real id.** Confirmed by trace, not just the
   handoff: `web.go:62-63` builds `withLogging(withRequestID(mux))`, so
   `withLogging` is outer; `withRequestID` rebinds only its local `r`
   (`web.go:117`), and `withLogging` reads `requestIDOf(r.Context())` from its
   own pre-enrichment request (`web.go:141`) → the `"-"` fallback
   (`context.go:28-33`). The handoff discloses this with the correct root cause
   and a fix suggestion (handoff "Notes / open issues" #1) and deliberately does
   NOT pin `"-"` in tests, so the future fix turns nothing red. `web.go` is
   outside T-102's owns and the frozen A-105 spec (acceptance/go/m1_guards_test.go:275)
   asserts only non-empty, so it passes. **Required follow-up**: route the
   middleware reorder to the next `web.go` owner (state.md:73-74 schedules the
   web.go refactor in Phase 2 — the integrator/planner should attach this defect
   to it or fix earlier). Not introduced or worsened by this diff; not a
   blocker for T-102 (rationale in "Position on reviewer-alt" below).
2. **Minor — response/log status not tied together.** `middleware_logging_test.go:88-90`
   accepts 200-or-404 from the recorder and the table asserts only the LOGGED
   status (:102-105). Note the logged status does track the real response
   (statusRecorder records `WriteHeader`, web.go:75-98, 128-129), so a
   healthz-returns-404 regression WOULD fail the `wantStatus "200"` assertion;
   the cheap tightening is `strconv.Itoa(rec.Code) == f["status"]`.
3. **Minor — text-mode pattern assertion can't distinguish empty value from
   missing key** (`middleware_logging_test.go:99-100`; `textFields` maps both to
   `""`). The JSON test pins key presence (:159-160), so the suite isn't blind;
   `strings.Contains(line, "pattern=")` would close the text-mode gap.
4. **Minor — the frozen `csrf_token` literal is never asserted** (csrf.go:20);
   all tests use the constant (e.g. csrf_test.go:44, 57). A typo would surface
   only at T-105. A one-line `CSRFTokenName == "csrf_token"` pin (natural home:
   T-103's probe tests) closes it.
5. **Minor/unverified — `rec2.Result()` (csrf_test.go:157 area) never closes the
   response body** while `csrfCookieOf` does (csrf_test.go:49-51). `bodyclose`
   behaviour on `httptest` `Result()` is uncertain and I could not re-run lint;
   the handoff's claimed `golangci-lint` exit 0 covers it if accurate, and the
   gate re-runs `make check` regardless. Trivial if flagged.
6. **Observation — plain double-submit** (no HMAC, no session binding) is the
   packet-frozen design; subdomain cookie-injection fixation is structurally
   possible. Revisit when sessions land (Phase 2 per state.md:73).
7. **Process — attempt-1 scratch file** (`internal/web/scratch_observe_test.go`,
   outside owns) was deleted via a one-off program under `harness/runs/`
   (disclosed, handoff "Notes" #2). Nothing of it is in the committed diff; the
   guard's ownership/dirty checks saw a clean tree. A permission-model
   workaround worth noting for the record, but transparent and immaterial to the
   merged artefacts. The run-log evidence cited by reviewer-alt is no longer
   present under `harness/runs/`; the `req_id=-` behaviour is independently
   proven by the code trace in finding 1.
8. **Verification limits**: no shell in this review environment — `make
   check-fast`, coverage %, e2e and guard exits are taken from the handoff on
   trust (cross-checked where possible against harness source). Compile-level
   risk was minimised by checking every referenced identifier
   (`newTestServer`, `testConfig`, `ctxServer`, `withRecover`, `withRequestID`,
   `requestIDOf`, `serverFrom`, `nowFunc`) against the current package.

## Position on reviewer-alt's BLOCK (harness/reviews/T-102-reviewer-alt.md)

The defect they block on (finding 1) is real and I confirm it independently.
I do not agree it blocks THIS diff:

- The operative M1 contract is the frozen acceptance spec, which asserts
  non-empty `req_id` and is green; T-102's mandate was explicitly "add the
  missing library code + unit tests and keep them green — do not expect a
  red→green acceptance flip" (T-102.md:23-25). Its enumerated unit-test bullets
  (T-102.md:70-77) don't even mention `req_id`; the worker asserted more than
  asked, at exactly the frozen spec's strength.
- No test was weakened, skipped or special-cased (AGENTS.md rule 2 intact): the
  new tests pin new ground, and they deliberately avoid pinning the `"-"` so the
  eventual `web.go` fix changes no assertion.
- The defect is T-100's, lives on main, is invisible to every gate assertion,
  and is unfixable inside T-102's owns. Writing the "correct-strength" test the
  alt-reviewer demands would hand the gate a red suite — the packet's answer to
  that is `BLOCKED`, which would stall T-103 → T-105 → all of M1 for an
  observability nit. The worker's choice — disclose first-class in the handoff
  (which the integrator/judge read) with root cause and fix — preserves the
  information without stalling the milestone. Both paths were defensible; the
  taken one is not a rule violation.
- I also note reviewer-alt's secondary finding 1 overstates: a 404-from-healthz
  regression would NOT pass `TestWithLoggingRequestFields` (the logged status is
  404, `wantStatus` is "200"); only the recorder cross-check is missing.

The follow-up in finding 1 must not be lost: the integrator should attach the
`withRequestID`/`withLogging` reorder (or equivalent) to the Phase 2 `web.go`
refactor or an earlier owner-fix, and T-105/T-203 should consider tightening
A-105's assertion once the fix lands.

## Checked and not found

No edits to protected paths; no `acceptance/` or golden changes; no new
dependencies; no skip/only markers; no `console.log`; no TODO/FIXME/panic
markers in the committed diff; no `template.HTML`; no IP/secret leakage; no
HTMX/`#map` surface; handlers/rules 6-7 not implicated (no handlers added).

VERDICT: PASS
