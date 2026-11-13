# T-203 — independent review (reviewer-alt): admin auth, sessions, AdminOnly guard

Basis: `harness/reviews/T-203.diff`, `harness/tasks/T-203.md`, `AGENTS.md`,
`acceptance/contract.json` (A-201), the frozen draft
`acceptance/drafts/admin_auth_go_test.go.txt`, `internal/config/config.go`,
`migrations/0002_lights.sql`, the T-205 packet and the T-203 handoff.

Verification limit: the diff is **not applied** in this checkout (the T-203
files do not exist on `main`) and shell execution is denied in this session, so
this is a static review of the diffed bytes. The handoff's executed results
(`make check-fast`, `go test`, the scratch-binary smoke) are taken on trust and
marked as such where they matter. Line numbers are post-patch, derived from the
diff hunks.

## Test-integrity / reward-hacking checks

- **No pre-existing test was weakened, deleted, skipped or narrowed.** The diff
  only adds files; it edits no existing test. No `t.Skip`, `test.skip`,
  `test.only`, `.fixme`, `TODO`, `FIXME`, `panic("not implemented")`,
  `console.log` or "Phase N" string appears in the added bytes.
- **No acceptance-fixture literal appears in non-test code.** The A-201 token
  fixture (`dGVzdC1hZG1pbi10b2tlbi0zMi1ieXRl`) appears only in the handoff, not
  in `internal/`. However, `tokenMatches` does deliberately accept the base64
  form the oracle submits; see finding 1. That is a general encoding branch,
  not a literal fixture special-case, and it is disclosed.
- **No mocks bypass the behaviour under test.** The store tests open real
  temp-file SQLite DBs through `store.Open` with migrations applied; the web
  tests drive the full `Server.Handler()` chain with a real DB. The `AdminOnly`
  cases call the real wrapper with a real store (the only injected value is the
  normal `ctxServer` used by the existing test harness).
- **The load-bearing paths are not coverage theatre.** The expiry table rejects
  at `expires_at` and one second before (`internal/store/session_test.go:450`);
  the replay row genuinely reads back `used_at` (`internal/store/user_test.go:680`);
  the valid login asserts a live admin session row, not merely a 302
  (`internal/web/auth_test.go:156`); the guard refusals cover tampered
  signatures, a different key, an expired row plus its sweep, and a non-admin
  role (`internal/web/session_test.go:107`). Deleting or inverting any of these
  branches fails the suite.
- **No dead code or linter workaround.** The single `//nolint:gosec` carries a
  same-line reason (`internal/web/session.go:65`); no unused production symbol
  is introduced.
- **No protected path or dependency change.** Only the handoff and the eight
  owned code/test files are added; `acceptance/**`, `harness/` (except the
  handoff), `Makefile`, CI, goldens and `go.mod`/`go.sum` are untouched. Imports
  are stdlib plus in-repo packages.

## Findings

### 1. [Medium — planner ratification required] the dual token-encoding acceptance is a product change made to satisfy the oracle

`internal/web/auth.go:86-90` (`tokenMatches`) compares the submitted token
against `Cfg.AdminToken` **and** its `base64.StdEncoding` encoding. This is not
a literal fixture branch, but it is a deliberate widening made because
`config.Load` stores the base64-decoded ADMIN_TOKEN
(`internal/config/config.go:118-127`) while the frozen oracle spawns the server
with, and submits, the env value verbatim
(`acceptance/drafts/admin_auth_go_test.go.txt:58,283`). The packet only asks for
a constant-time compare against `s.Cfg.AdminToken`
(`harness/tasks/T-203.md:60`). The handoff discloses the deviation at
`harness/handoffs/T-203.md:53-70` and asks the planner to decide.

The branch is general, both strings denote one secret, both comparisons run,
and every accepted encoding burns the same `login_tokens` row (pinned by
`internal/web/auth_test.go:251-292`), so I do not read it as a hidden test
bypass. But it is an unratified, security-sensitive expansion of the accepted
credential strings, and the handoff's claim that a raw-config change "degrades
harmlessly" is imprecise: `tokenMatches` would still accept `base64(raw)` as a
second form. The planner should ratify the decoded-config + dual-compare
contract, or change `config` to keep ADMIN_TOKEN raw and compare one form only.
Not a gate block on its own, but do not merge it as an undocumented product
decision.

### 2. [Medium — the test does not independently pin the frozen HMAC format]

`TestSessionValueRoundTrip` (`internal/web/session_test.go:179-191`) checks that
the first segment is `base64url(id)` and that the signature segment is 64
characters, then round-trips through `sessionValue`/`sessionIDOf`. It never
computes `hex(HMAC-SHA256(key, id))` independently. A future change that signed
the encoded id, or used a different 32-byte keyed MAC, would keep this test
green because both signer and verifier would share the wrong message/algorithm.
The code itself is correct (`internal/web/session.go:35-42` signs `[]byte(id)`),
and the wrong-key guard case (`internal/web/session_test.go:153-166`) proves the
key participates, but it does not pin the message or the hash. Add the expected
HMAC computed in the test (or a fixed key/id expected cookie string). This is a
regression hole in a security-sensitive format, not a current functional bug.

### 3. [Medium — the A-201 oracle is still red; the only config-path check was a deleted scratch smoke]

The handoff says "complete", but the frozen oracle still fails at
`GET /admin` 404 (`harness/handoffs/T-203.md:1-13,97-124`);
`acceptance/drafts/admin_auth_go_test.go.txt:80-86` requires the admin route
that T-205 owns. That split is by design, but the committed tests bypass the
production decode path: `newAuthServer` sets `cfg.AdminToken` directly
(`internal/web/auth_test.go:38-52`), and `TestAdminLoginTokenEncodings`
(`:251-292`) tests both forms against that direct value, not through
`config.Load`. The real-binary smoke that exercised `.env.test` was in the
deleted `internal/zz-a201-baseline/` scratch directory, so the exact oracle
path is not reproducible from the commit. T-207's activation is the first
committed end-to-end check. Add a small committed test that loads the fixture
env through `config.Load` (or keep the smoke), otherwise a future config change
can break the exchange while the unit suite stays green.

### 4. [Medium — T-205 integration hazard in the handoff]

The handoff says T-205 will replace the inline login page with
`pages/admin-login.html` through the template pipeline
(`harness/handoffs/T-203.md:60,138-141`). But T-205's packet gives it
`web/templates/**` and explicitly forbids editing `internal/web/auth.go`
(`harness/tasks/T-205.md:11,42`), while the served page is still the inline
template in `internal/web/auth.go:147-179` (rendered by `renderLoginPage` at
`:170`). T-205 can add the template but cannot make it the served login page
without an owns exception, so the template risks being dead or T-205 must
violate its owns. The planner should add `auth.go` to T-205's owns or have
T-203 expose a render hook.

The same handoff says to wrap **every** `/admin*` route with `AdminOnly`
(`harness/handoffs/T-203.md:19,138`). `/admin/login` is `/admin*`; wrapping it
would redirect the login page to itself. The code does not wrap the login
routes (they are registered in `internal/web/auth.go:28-31`), so the
implementation is correct, but the handoff wording is a trap for T-205. Exempt
`/admin/login` explicitly.

### 5. [Low — DB-error wrapping is weaker than the contract]

`TestConsumeAdminToken` "database errors wrap the driver error"
(`internal/store/user_test.go:126-136`) only asserts `err != nil` and
`!errors.Is(err, ErrTokenReused)`. A change from `%w` to `%v`, or a generic
error, would pass even though the packet/handoff require a wrapped driver
error. `SessionValid`'s DB-error path (`internal/store/session.go:68-92`) is
not tested at all; the handoff claims it wraps `%w`
(`harness/handoffs/T-203.md:47-50`). Add a closed-DB test that asserts the
underlying error is in the chain (`errors.Is`/`errors.Unwrap`) and is not
`ErrBadCredentials`.

### 6. [Low — a few assertions are narrower than the adjacent paths]

- `TestAdminOnlyGuard` "admin HTMX with a valid cookie passes too"
  (`internal/web/session_test.go:91-98`) checks only status 200; it does not
  assert the guarded body ran, unlike the non-HTMX case. A guard that returned
  200 without calling `next` would pass this subtest.
- `TestAdminLoginForm` "valid POST exchange" (`internal/web/auth_test.go:310-322`)
  checks the cookie flags but not that the cookie maps to a live admin session;
  the GET exchange test does, so this is a small variant gap.
- The wrong-token and replay cases (`internal/web/auth_test.go:209-242`)
  assert "no Set-Cookie" but not "no session row"; an implementation that
  minted an orphan session before validating would pass.
- The CSRF-refusal cases (`internal/web/auth_test.go:323-346`) check only the
  403 status, not the absence of a session/cookie.

These do not undermine the main paths, but they are the cheap assertions to
tighten if the suite is touched again.

### 7. [Low — exported mutable clock is a test hook in the public API]

`internal/store/session.go:23` exports `Now`, and both the store tests
(`setClock`, `internal/store/session_test.go:16-24`) and the web expiry test
(`internal/web/session_test.go:123-130`) mutate it. The packet suggested a
package-level lowercase `now`; exporting it was necessary for the cross-package
expiry case, and no test uses `t.Parallel`, so the suite is safe. Still, this
is a mutable global in production code. A `Clock` field or an exported
setter/restore helper would keep the injection point out of the production
package variable list. Not a gate block.

### 8. [Info — handoff reporting]

The handoff's "exactly one point" red oracle (`:13,115-124`) is the first fatal
assertion (the missing `/admin` route); the later `/admin`-with-session and
HTMX clauses also depend on T-205. The handoff is transparent about the split,
but "exactly one point" should read "first fatal point". The handoff's
`make check-fast`/`e2e-task` exit codes are taken on trust (no captured output
in the diff); the tree itself is consistent with a green run.

## Checked and found sound

- **Frozen API matches the packet**: `SessionCookieName`, `AdminOnly`, the store
  method signatures, `UserID`, the sentinels and `Now`
  (`internal/web/session.go:26-28,83`, `internal/store/session.go:23,68`,
  `internal/store/user.go:34-48`).
- **Store semantics are correct**: `EnsureAdminUser` uses
  `ON CONFLICT(handle) DO NOTHING` then select; `ConsumeAdminToken` hashes with
  SHA-256/hex and detects replay via the `login_tokens` primary key; sessions
  are 32 crypto/rand bytes base64url with 24 h RFC3339 stamps; expiry is strict
  at `expires_at`; `DeleteSession` is idempotent; the sessions FKs are tested
  (`migrations/0002_lights.sql:70-82`).
- **Cookie codec and attributes are correct**: `base64url(id) + "." +
  hex(HMAC-SHA256(key, id))`; `Path=/`, `Max-Age=86400`, `HttpOnly`,
  `SameSite=Lax`, `Secure` only in prod (`internal/web/session.go:35-42,65-81`).
- **The login flow is correct**: blank → 200 form; wrong → 403; valid →
  consume + ensure admin + create session + cookie + 302 `/admin`; replay → 403;
  POST is CSRF-verified; the token is never logged
  (`internal/web/auth.go:97-145`).
- **The guard is correct**: anon full page → 302 `/admin/login`; HTMX → 401 +
  `HX-Redirect` + empty body; valid admin session → next; expired → sweep +
  refusal; non-admin role → refusal; bad signature/wrong key → refusal
  (`internal/web/session.go:83-135`).
- **Tests use a real temp DB**, cover the packet's named cases, and the suite
  has no skip/deletion/narrowing. The added bytes contain no fixture literal,
  no `r.FormValue` after parsing, no protected-file edit and no new dependency.

## Verdict rationale

The implementation is materially complete for the T-203 slice and the suite is
load-bearing on the paths that matter (one-time consumption, expiry, cookie
attributes, the four guard modes, CSRF). I found no deleted, skipped, narrowed
or literal-fixture-special-cased test, no mock substituting for the behaviour
under test, and no dead code added for the linter. The findings are a
security-sensitive but disclosed token-encoding reconciliation needing planner
ratification, an HMAC-format test gap, the expected red oracle plus a deleted
config-path smoke, a T-205 integration/ownership hazard, and a few low-level
DB-error/assertion gaps. None lets a broken deliverable through the gate, but
findings 1 and 3 should be resolved before T-205/T-207 rely on this branch.

VERDICT: PASS
