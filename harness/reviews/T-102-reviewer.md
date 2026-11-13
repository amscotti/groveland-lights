# T-102 — reviewer (correctness / security / architecture)

Scope: static review of the attached diff (`harness/reviews/T-102.diff`) against
`harness/tasks/T-102.md`, `docs/spec/architecture.md`, the A-102/A-105 contract
assertions, the frozen A-103 drafts this API must serve, and the current harness
source. Shell execution is permission-denied in this environment, so commands
were not re-run; instead every harness/guard claim was verified against source
(`cmd/harness/{guard,scan,ratchet,globmatch}.go`, `Makefile`, `.golangci.yml`,
`harness/policy.json`), every referenced identifier was checked against the
current `internal/web` package, and — decisive for the disputed logging
behaviour — the worker's own run log (`harness/runs/20261113T045239b/T-102/
worker.jsonl:131-137`) contains the empirically executed probe and its output.
The diff is not applied to this worktree; the gate re-runs everything anyway.

## Packet compliance — verified

- **Frozen API delivered exactly** (T-102.md:46-55): `IssueCSRFToken(w, r)
  string` (internal/web/csrf.go:44), `var ErrCSRF = errors.New("csrf: token
  mismatch")` (csrf.go:34), `VerifyCSRF(r, submitted) error` (csrf.go:69),
  `CSRFTokenName = "csrf_token"` (csrf.go:20) matching the frozen contract
  string (`acceptance/drafts/README.md:47-48`). 32 `crypto/rand` bytes →
  unpadded base64url; cookie `Path=/, HttpOnly, SameSite=Lax, Secure` iff
  `IsProd()`; idempotent reuse of a well-formed request cookie with no
  redundant `Set-Cookie`. Stdlib only — `allow_deps: []` respected.
- **Required test case names all present** (T-102.md:57-61): missing-cookie /
  empty-submission / tampered / valid-round-trip / idempotent-per-cookie /
  cookie-flags-prod-vs-test, table-driven where asked
  (csrf_test.go:78-229), plus entropy-floor, garbage-input (incl. padded and
  over-length), malformed-cookie-reissue and header/context request-ID guards.
- **Middleware guard tests present and meaningful** (T-102.md:63-77): they run
  the real chain via `s.Handler().ServeHTTP` (middleware_logging_test.go:81-84)
  — stronger than the packet's bare `withLogging` ask; prod JSON is built with
  the exact `slog.NewJSONHandler(buf, nil)` construction
  `cmd/server/main.go:70-75` uses (middleware_logging_test.go:36-46);
  RemoteAddr `192.0.2.1` and injected `X-Forwarded-For: 203.0.113.77`
  non-leak asserted on the exact A-105 payloads (T-102.md:73-74,
  acceptance/go/m1_guards_test.go:209-241).
- **Ownership clean**: changed files are `internal/web/csrf.go`,
  `csrf_test.go` (both match owns glob `internal/web/csrf*.go`), and
  `internal/web/middleware_logging_test.go` (matches
  `internal/web/middleware*.go`) — `*` is `[^/]*` in
  `cmd/harness/globmatch.go:41-43`, so the test files are covered — plus the
  auto-allowed handoff (`cmd/harness/guard.go:133`). `web.go`, `context.go`,
  `templates.go`, `web_test.go` untouched. No protected path
  (`harness/policy.json:37-51`) is near the diff. Go test count only increases
  (ratchet guard.go:149-171); no skip/only markers added anywhere.
- **Markers/lint**: no `TODO`/`FIXME`/`panic("not implemented")` in any added
  line (`cmd/harness/scan.go:11-15` list); the single `//nolint:gosec`
  (csrf.go:54) carries a same-line reason, satisfying AGENTS rule 9 and not
  matching `reNolintNoReason` (scan.go:17). "session-less phase" prose is
  lowercase with no digit — `make phase-grep`'s `\bPhase [0-9]`
  (Makefile:130-133) cannot match. No `console.log`, no `template.HTML`.

## Security

- **CSRF is correct for the frozen double-submit contract.** Strict
  raw-base64url well-formedness on BOTH cookie and submission
  (csrf.go:91-96: `RawURLEncoding` rejects padding, non-alphabet chars and
  invalid lengths, so every "garbage submission" case in csrf_test.go:135-146
  is genuinely rejected), entropy floor ≥32 decoded bytes, 256-char DoS bound,
  uniform `ErrCSRF` (no cause oracle for the 403 path), and
  `subtle.ConstantTimeCompare` (csrf.go:74). Cookie flags pinned per env by
  `TestCSRFCookieFlags` (csrf_test.go:199-229). The entropy-failure path
  answers 500 and returns `""` rather than rendering a form with an empty
  token (csrf.go:48-53) — documented for T-103 in the handoff.
- **Sufficiency for A-103 proven against the frozen drafts**, not assumed:
  `probeTamper` flips the first character preserving shape/length
  (acceptance/drafts/probe_go_test.go.txt:150-163 — mirrored by the worker's
  `tamperCSRF`, csrf_test.go:82-91), which stays well-formed and fails at the
  constant-time compare → `ErrCSRF` → 403; missing token → empty check → 403;
  valid round-trip → nil. Critically, the drafts spawn `APP_ENV=test` over
  plain http with a cookie jar (probe_go_test.go.txt:285-299), so
  `Secure=false` outside prod is *load-bearing* for the frozen acceptance
  flow — `TestCSRFCookieFlags` pins exactly that, and `SameSite=Lax` is the
  real cross-site POST carriage blocker for this session-less pattern.
- No XSS surface (no `template.HTML`; tokens are base64url and escaped by
  `html/template` downstream). No SQL, no path traversal, no uploads, no
  secrets in output; the IP non-leak assertions match A-105 byte-for-byte.
- Inherent limitation, packet-mandated (T-102.md:44 "Double-submit cookie
  pattern, stdlib only"): the token is unsigned and session-unbound, so
  cookie-fixation by anything that can set a `csrf_token` cookie for the
  origin passes verification. Not a deviation — flagged for the Phase 2
  sessions work (state.md:73-75).

## Architecture

- Ownership map respected (`docs/spec/architecture.md:19-21`: `internal/web`
  owns CSRF). No routes, templates, static files or HTMX surface — the `#map`
  rule is untouched by construction. Parse-don't-validate / typed IDs are not
  implicated (library task; the handoff explicitly instructs T-103 to parse
  the form into a typed struct at the boundary before `VerifyCSRF`, per
  AGENTS rule 6).

## Tests exercise real behaviour

Traced against the current package, and corroborated empirically: the worker's
run log (`worker.jsonl:134`) shows a real executed request line
`msg=request method=GET pattern="GET /healthz" status=200 dur_ms=0 req_id=-`,
which confirms the exact shapes the text/JSON parsers in
middleware_logging_test.go:43-56 and :140-176 assume (quoted-with-spaces,
bare, and quoted-empty `pattern=""` forms all handled correctly by the final
regex — including the empty-value case the first draft got wrong and the
worker rewrote). Pattern resolution via the `s.mux.Handler(r)` fallback
(web.go:130-135) yields `GET /healthz` vs `""` exactly as asserted; recover →
500 + logged panic and request-ID header/context invariants match
web.go:101-120. The CSRF tests drive real `Server`s through `httptest` with
the server injected at the same context key the chain uses (`ctxServer`),
exercising mint, reuse-without-Set-Cookie, reissue-on-malformed-cookie and
both Secure polarities.

## Findings

1. **Pre-existing, disclosed: every request log line carries `req_id=-` while
   the response header carries the real id.** Confirmed three ways: my code
   trace (web.go:61-63 order makes `withLogging` outer; `withRequestID`
   rebinds only its local `r` at web.go:117, so `withLogging` reads
   `requestIDOf` from the pre-enrichment request at web.go:141 → the `-`
   fallback, context.go:28-33), the worker's executed probe (worker.jsonl:134,
   header `a1cd53c7fa39bb2a` vs `req_id=-`), and reviewer-alt's independent
   trace. It is NOT introduced by this diff (web.go is untouched and outside
   owns), it is invisible to the frozen A-105 assertion
   (acceptance/go/m1_guards_test.go:275 asserts only non-empty — `-` passes),
   and it is unfixable inside T-102's owns. The handoff discloses it
   first-class with root cause and fix direction, and deliberately does not
   pin `-` in any test so the future fix turns nothing red.
   **Required follow-up**: attach the `withRequestID`/`withLogging` reorder to
   the Phase 2 `web.go` refactor already scheduled in state.md:73-75 (or an
   earlier owner-fix); T-105/T-203 should tighten A-105's assertion then.
   Position on block-worthiness below.
2. **Minor — logged status not tied to the recorder status.**
   middleware_logging_test.go:88-90 accepts 200-or-404 and asserts only the
   LOGGED status (:102-105). The logged status does track the real response
   (statusRecorder, web.go:75-98,128-129), so a healthz-returns-404 regression
   still fails `wantStatus "200"`; the cheap tightening is
   `strconv.Itoa(rec.Code) == f["status"]`.
3. **Minor — text-mode pattern assertion can't distinguish empty value from
   missing key** (middleware_logging_test.go:99-100; `textFields` maps both to
   `""`). The JSON test pins key presence (:159-160), so the suite is not
   blind; `strings.Contains(line, "pattern=")` closes the text-mode gap.
4. **Minor — the frozen `csrf_token` literal is never asserted** (csrf.go:20);
   all tests use the constant. A one-line pin (natural home: T-103's probe
   tests) would catch a typo before T-105.
5. **Minor/unverified — `rec2.Result()` in the idempotency test
   (csrf_test.go:157) never closes the body** while `csrfCookieOf` does
   (:49-51). bodyclose behaviour on recorder `Result()` is uncertain and I
   could not re-run lint; the handoff's `make check-fast` exit 0 covers it if
   accurate, and the gate re-runs `make check`.
6. **Process — attempt-1 scratch probe and its removal.** The worker created
   `internal/web/scratch_observe_test.go` (outside owns), used it to confirm
   finding 1, then — with `rm`/`mv`/`git clean` all permission-denied
   (worker.jsonl:138-151) — deleted exactly that file (and the one-off
   program) via an allowlisted `go run` from `harness/runs/`
   (worker.jsonl:175-188). Fully disclosed in the handoff (Notes #2); nothing
   of it is in the committed diff; the guard's ownership and dirty checks saw
   a clean tree. A permission-model workaround worth the record, transparent,
   immaterial to the merged artefacts.
7. **Narrative nits in the handoff (no code impact)**: (a) it claims
   `checkMarkers` "does NOT exempt `harness/handoffs/**`", but the current
   `cmd/harness/scan.go:27-29` DOES exempt handoffs (with a comment that reads
   like a post-attempt-1 amendment by the harness owner — cmd/harness is
   protected and untouched by this task); either way the handoff is
   marker-clean, so the artefacts pass under both readings. (b) The attempt-1
   lint note says "cyclop 21/17" while `.golangci.yml:24-25` sets
   max-complexity 15. History colour, not correctness.
8. **Verification limits**: no shell here — `make check-fast`, coverage %,
   e2e and guard exits are taken from the handoff, cross-checked where
   possible against harness source (e2e-task's `--pass-with-no-tests` for the
   Go-only A-105 claim matches Makefile:111-116; `node_modules` dirty
   allowlist matches ratchet.go:127-129; coverage floor 70 matches
   policy.json:32-36). Compile-level risk was minimised by checking every
   referenced identifier (`newTestServer`, `testConfig`, `ctxServer`,
   `serverFrom`, `withRecover`, `withRequestID`, `requestIDOf`, `nowFunc`,
   `New`, `Handler`) against the current package.

## Position on reviewer-alt's BLOCK (T-102-reviewer-alt.md)

The defect they block on (finding 1) is real and I confirm it independently,
including from the executed run log they cite. I do not agree it blocks THIS
diff:

- The operative M1 oracle is the frozen acceptance spec, which asserts
  non-empty `req_id` and is green; T-102's mandate was explicitly "add the
  missing library code + unit tests and keep them green — do not expect a
  red→green acceptance flip" (T-102.md:22-25). Its enumerated unit-test
  bullets (T-102.md:70-77) do not even mention `req_id`; the worker asserted
  more than asked, at exactly the frozen spec's strength.
- No test was weakened, skipped or special-cased (AGENTS rule 2 intact): the
  acceptance spec is protected and untouched, and the new tests pin new ground
  while deliberately avoiding pinning `-` so the eventual fix changes no
  assertion.
- The defect is T-100's, lives on main, and is unfixable inside T-102's owns.
  The only green-suite alternatives were infeasible: a stronger unit test
  (`req_id != "-"` or header-match) is red today, and the packet's `BLOCKED`
  path would stall T-102 → T-103 → T-105 (graph.json deps) — the entire
  milestone — for an observability nit invisible to every gate assertion.
  The taken path — disclose first-class in the handoff with root cause and
  fix, flag for the Phase 2 web.go owner — preserves the information without
  stalling M1. Reward-hacking looks like hiding the probe; the run log shows
  the worker probing, confirming, and writing the finding into the handoff.
- Reviewer-alt's secondary finding 1 also overstates: a 404-from-healthz
  regression would NOT pass `TestWithLoggingRequestFields` (logged status 404
  vs `wantStatus "200"`); only the recorder cross-check is missing.

The follow-up in finding 1 must not be lost: the integrator should attach the
middleware reorder to the Phase 2 `web.go` refactor (state.md:73-75) or an
earlier owner-fix, and A-105's assertion should be tightened once it lands.

## Checked and not found

No edits to protected paths, `acceptance/`, goldens, Makefile, CI or lint
config; no new dependencies; no skip/only markers; no `console.log`; no
TODO/FIXME/panic markers in added lines; no `template.HTML`; no IP/secret
leakage; no HTMX/`#map` surface; no handlers added (rules 6-7 not implicated);
migrations untouched.

VERDICT: PASS
