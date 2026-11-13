# T-203 review — Admin auth: one-time token exchange, DB sessions, admin guard (A-201)

Review basis: the attached diff (`harness/reviews/T-203.diff` = `harness/handoffs/T-203.md`
+ 8 new files) against the packet `harness/tasks/T-203.md`, the frozen oracle
`acceptance/drafts/admin_auth_go_test.go.txt`, the frozen contract table
`acceptance/drafts/README.md:55-80`, the merged sources the diff builds on
(`internal/web/{web,csrf,context,web_test}.go`, `internal/store/{store,migrations_test}.go`,
`internal/config/config.go`, `migrations/0002_lights.sql`), `.env.test`,
`acceptance/e2e/util/admin.ts`, `docs/PLAN.md:326,861`, `harness/tasks/T-205.md`,
and the `Makefile`.

Verification limits: the diff is not applied in this checkout (task worktree) and shell
execution is denied — same constraint as the T-201/T-208 reviews. Everything below is a
static check of the diffed bytes against the contracts I could read; the handoff's executed
exit codes are trust-marked but were cross-checked for internal consistency. Line numbers
are post-patch `file:line` into the new files as positioned by the diff hunks (±2).

## 1. Correctness vs the packet — both deliverables met

Store slice (packet Deliverable 1):

- `EnsureAdminUser` is the packet's exact recipe: `INSERT … ON CONFLICT(handle) DO
  NOTHING` then `SELECT` (`internal/store/user.go:46-60`); idempotency, same-id and
  exactly-one-row proven (`user_test.go` "second call is idempotent and returns the same
  id", count==1). Schema fit verified against `migrations/0002_lights.sql:1-8`
  (handle UNIQUE, role CHECK satisfied by 'admin', email NOT NULL).
- `ConsumeAdminToken`: `token_hash = hex(sha256(token))` (`user.go:70-71`); first use
  INSERTs email `admin`, `expires_at` now+24h, `used_at` now (`user.go:73-78`); a second
  call re-SELECTs `used_at` and wraps `ErrTokenReused` (`user.go:82-90`); every other
  failure wraps the driver error `%w` (`user.go:101-105`). The closed-DB test proves a
  real DB error is never misreported as a replay (and vice versa via the ErrNoRows
  branch). Consumption is a global PK row — permanent, client-independent — which is
  exactly the draft's "one-time and GLOBAL … a fresh jar must not help"
  (draft :30-34); pinned by "replayed token 403 from a fresh cookie-less client" (web)
  and "second use wraps ErrTokenReused" (store).
- `CreateSession`: 32 crypto/rand bytes → base64url id, RFC3339 `created_at` now /
  `expires_at` now+24h (`internal/store/session.go:44-60`); entropy, uniqueness and both
  stamps pinned (the timestamps test asserts the stored strings against the injected
  clock).
- `SessionValid` triage (`session.go:68-94`): empty/missing id → `(0,"",false,nil)`;
  expired → `(0,"",false,err)` wrapping `ErrBadCredentials`; DB failure → wrapped `%w`.
  Expiry is compared in Go on a parsed time — no lexicographic SQL string comparison
  trap — and the boundary is pinned at exactly-24h (rejected), −1s (valid) and +1h.
- `DeleteSession` idempotent (`session.go:96-103`), pinned.
- Injectable clock implemented (see deviation 2 below).

Web slice (packet Deliverable 2):

- Routes registered in `init()` (`internal/web/auth.go:29-32`); `web.go` untouched —
  verified against the on-disk merged file.
- GET no/blank token → 200 with the four frozen markup elements: the inline template
  (`auth.go` `loginPageTmpl`, ≈:149-167) satisfies the draft's exact regexes
  (`<h1>Admin login</h1>`, `<label for="token">Token</label>`,
  `<input … name="token" …>`, `<button …>Sign in</button>`); the web tests re-use the
  draft's regexes verbatim.
- Wrong token → 403 (`auth.go:113-117`); valid → `ConsumeAdminToken` →
  `EnsureAdminUser` → `CreateSession(…, "admin")` → `setSessionCookie` → 302 `/admin`
  (`auth.go:118-142`).
- Cookie `gl_session`: `Path=/`, `HttpOnly`, `SameSite=Lax`, `MaxAge 86400`, `Secure`
  exactly `IsProd()` (`internal/web/session.go` `setSessionCookie`, ≈:66-76) — asserted
  field-exact in `TestAdminLoginExchange`/`assertFrozenCookieFlags` (incl. a prod-Secure
  case), and the handoff's real-binary smoke replayed the draft's raw `Set-Cookie`
  parsing against the spawned server.
- `AdminOnly`: valid admin session → `next`; full page → 302 `/admin/login`;
  `HX-Request` → 401 + `HX-Redirect: /admin/login` + empty body (`session.go` ≈:85-99) —
  matches draft :17 and :36-37 exactly; all three branches table-tested including the
  empty-body assertion and a "guarded handler ran" canary.
- Logging via `s.Log`; the token, cookie value and session id never appear in any log
  line or error message (grep over the diff; only `user_id` is logged, `auth.go:141`).

## 2. Security

- **CSRF**: `POST /admin/login` is verified via `VerifyCSRF` before any state change
  (`auth.go:71-74`); missing/mismatch → 403, both tested. The state-changing GET
  exchange is the frozen contract (draft :25-29); it is not cross-site exploitable
  without already knowing the token, and `Referrer-Policy: same-origin`
  (`web.go:109`) bounds URL leakage from the redirect.
- **Token compare**: two unconditional constant-time compares (`auth.go` `tokenMatches`
  ≈:87-92); consumption always uses the canonical `Cfg.AdminToken` bytes, so both
  accepted encodings burn the same `login_tokens` row (`auth.go:118`) — pinned by
  "either accepted encoding burns the same one-time row".
- **Session integrity**: cookie = `base64url(id) + "." + hex(HMAC-SHA256(SessionKey,
  id))` with constant-time signature compare; validity requires the live server-side row
  (expiry + role). Tampered signature, wrong key, user-role, structurally-garbage and
  expired cookies are each refused and tested. The guard fails closed on nil store and
  on DB lookup errors (`session.go` ≈:116-119, ≈:132-133).
- **XSS**: the login page is a static template with no data interpolated, rendered into
  a buffer before any write, `charset=utf-8` set (`auth.go` `renderLoginPage`); no
  `template.HTML` anywhere in the diff; `http.Redirect` bodies contain fixed paths only.
- **SQL**: every statement is parameterized (`?` placeholders throughout
  `user.go`/`session.go`); no string interpolation of user input.
- **Secrets in output**: refusal paths log no credential values; wrapped errors never
  embed the token or session id.
- **Path traversal / upload limits**: N/A to this slice (no filesystem paths, no
  uploads — T-206 owns `LimitBody`).
- **Dependencies**: none added (stdlib + internal imports only) — matches
  `allow_deps: []`.

## 3. Architecture

- **Ownership**: all 8 files land inside the owns globs
  `internal/web/{auth,session}*.go`, `internal/store/{user,session}*.go` (test files
  match the globs); no shared or protected file touched — `web.go`, `csrf.go`,
  `templates.go`, `internal/config`, `migrations/` are byte-identical to merged main.
- **Parse-don't-validate**: `loginForm` is lifted at the boundary
  (`auth.go:36-56`); neither handler reads `r.FormValue`/`PostForm`/query after
  parsing (grep-verified).
- **Typed IDs**: `store.UserID int64` (`user.go:20`); the session id stays a string
  exactly as the packet freezes its signature.
- **HTMX/#map rule**: no `hx-target`/swap content introduced — only the `HX-Redirect`
  response header; the map is untouched.
- **Hygiene**: no "Phase N" strings, no TODO/FIXME/`panic(`/`console.log` in the new
  files; the single `//nolint:gosec` carries its reason on the same line
  (`session.go` ≈:67), matching the `csrf.go:54` precedent.
- Migrations append-only and untouched; status derivation N/A.

## 4. Tests actually exercise the behaviour

- Full-chain `httptest` through `s.Handler()` (all middleware) against a **real
  temp-file SQLite with migrations applied** (`newAuthServer` → `store.Open`), not
  mocks: exchange 302 + Location + exact cookie attributes + the minted cookie value
  verifying to a live admin row in the DB; wrong-token 403 with **zero** cookies set;
  replay 403 from a second cookie-less request; GET/POST blank → 200 + frozen markup;
  the POST CSRF matrix; the guard table (anon 302 / anon HTMX 401+header+empty body /
  admin pass / admin HTMX pass / tampered / expired-via-injected-clock + row swept /
  user-role / wrong key / garbage); cookie codec round-trip + six malformed inputs;
  store-side consume-once, row contents pinned at the injected clock, FK refusal,
  closed-DB error wrapping, 24 h boundary at −1 s / exact / +1 h.
- Regression value: removing the HMAC check, the expiry check, the role check, the
  one-time consume or any fail-closed path each fails a named case. Test names read as
  assertions per house style; no skips/special-casing anywhere in the diff.
- Oracle honesty: pre-implementation red demonstrated (draft fails first at
  `GET /admin` 404); post-implementation red is confined to the `/admin` route
  registration that T-205's packet (`harness/tasks/T-205.md:31-32,46-49`) owns; every
  T-203-owned clause was additionally verified against the real spawned binary
  (handoff smoke). `make e2e-task` no-op matches the packet's "inert drafts —
  expected no-op"; the handoff's `TASK`-vs-`T` note is accurate (`Makefile:8,113`).

## 5. Deviations (all disclosed in the handoff) — assessed

1. **Dual token encoding** (`auth.go` `tokenMatches` ≈:87-92). The premise is verified
   true: `config.secret()` base64-decodes values of ≥16 bytes
   (`internal/config/config.go:118-127`), so `Cfg.AdminToken` holds *decoded* bytes,
   while the oracle submits the env value verbatim (`admin_auth_go_test.go.txt:58,117`,
   `.env.test:8`), as do `admin.ts:34,128` and `docs/PLAN.md:326` ("32 random bytes,
   base64"). A literal single compare 403s the frozen valid-token step. The fix accepts
   exactly {raw secret, base64(raw secret)}: base64 is deterministic and bijective, so
   knowledge of either form **is** knowledge of the secret — the accepted credential
   set does not widen. Consumption stays on canonical bytes so both encodings burn one
   row, and both facts are pinned by tests. This is a sound reconciliation *within the
   task's owns* (config is not T-203's to edit), prominently documented and routed to
   the planner with a clean fallback. Not a block — but the planner should resolve it
   (raw-in-config or an operator-contract note) so the second compare doesn't look like
   cruft to a future reader.
2. **Exported `store.Now`** (`session.go:23`) vs the packet's lowercase `now`: the
   packet itself requires the web-level "expired session rejected (injected clock)"
   case, which is unreachable through a package-private store var. Consistent with the
   repo's existing `nowFunc` pattern (`internal/web/context.go:50`); production never
   mutates it; every test swap restores via `t.Cleanup`.
3. **Double-encoded id in the cookie value** (`session.go` `sessionValue` ≈:36-39):
   the literal reading of the packet's "base64url(id)" where id is already a base64url
   string; the draft pins only the `gl_session=` prefix; the round-trip test pins the
   implemented reading. Harmless redundancy, HMAC covers the decoded id on both sides.
4. **POST blank token → 200 form** (`auth.go:100-102`, reached only after the CSRF
   check): uniform with the frozen GET blank case ("nothing entered"); the draft
   freezes only GET paths; pinned by tests for T-205's form work. Reasonable.
5. **Makefile variable spelling**: accurate and useful.

## 6. Notes (non-blocking)

- **Operability**: after the first successful exchange, the static `ADMIN_TOKEN` is
  consumed forever (the PK row never leaves the table, by design — draft :30-34 and
  T-200's own admin.ts review embraced the one-time-GLOBAL semantics), so re-login
  after the 24 h session TTL requires token rotation or `login_tokens` cleanup. A
  runbook item, not a code defect.
- The handoff's "Wrap EVERY `/admin*` route" phrasing could be misread by T-205 to
  include `/admin/login` (redirect loop); in context the parenthetical route list and
  the "already registered — do NOT re-register" note exclude it, and T-205's packet
  independently enumerates the routes. A one-line "…except the login routes
  themselves" would remove the ambiguity.
- The inline GET login form carries no `csrf_token` field/cookie, so a browser-driven
  POST login isn't reachable from the minimal page (the frozen flow is the GET
  exchange; the POST route exists for completeness and is tested with minted tokens).
  T-205's template can add the hidden field without touching the frozen strings.
- No rate limiting on token attempts: brute force is infeasible at the provisioned
  entropy (PLAN.md: "32 random bytes"), and rate-limit middleware is a separate slice
  in architecture.md's request flow — flagging for the planner, not this task.
- Handoff open issue 3 (untracked `node_modules` symlink) is moot for the dirty-tree
  guard: `node_modules/` is gitignored (`.gitignore:2`). Honest disclosure of a
  non-issue.

VERDICT: PASS
