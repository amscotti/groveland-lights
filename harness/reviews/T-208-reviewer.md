# T-208 reviewer — store onto Server + /probe fragment Content-Type

Basis: the attached diff (attempt 5), packet `harness/tasks/T-208.md`, judge-M1
Gap (b) (`harness/findings/judge-M1.md:80-87`, path-to-done item 4 at
:113-114), the pre-change sources on disk (`internal/web/{web,probe,csrf,
context,web_test,probe_test,middleware_logging_test}.go`,
`cmd/server/{main,boot_test}.go`, `web/templates/**`, `web/templates/base.html`),
the active contract `acceptance/go/probe_test.go`, and the handoff added by
the diff.

Method note: shell execution was denied this session, so exit-code claims were
not re-run. Every load-bearing claim was verified statically: the diff's
pre-image matches the on-disk files exactly on all five pre-existing code
paths; grep confirms no reference to `SetProbeStore`/`probeStore` survives
outside the rewritten files; the sniffing failure mode was re-derived from
first principles (below).

## Deliverable 1 — store onto Server: complete, one forced and disclosed deviation

- `Server.Store *store.DB` exported field with accurate nil-semantics doc
  (internal/web/web.go, diff hunk at :27-40). Globals `probeStore`,
  `SetProbeStore`, `errProbeStoreUnset` and the SCAFFOLD note fully deleted
  from internal/web/probe.go. Repo-wide grep: the only call sites were
  `cmd/server/main.go:49,51`, probe.go itself and probe_test.go:36-37,181 —
  all rewritten; no stragglers.
- Nil store → 500 with the packet's exact message `probe: store not wired`
  (packet :39-41 → `errProbeStoreNotWired = errors.New("probe: store not
  wired")`); same log+`http.Error` 500 path as before.
- `cmd/server/main.go`: `SetProbeStore(db)` dropped, `web.New(cfg, logger,
  db)` in its place; boot order otherwise unchanged (store.Open → checksum
  refusal → `defer db.Close()` → New → serve).
- `boot_test.go` comment-only is correct: those tests spawn `bin/server`
  black-box (imports at cmd/server/boot_test.go:21-38 pull in no
  `internal/web`), so the packet's "update boot_test.go" (:43) is satisfied
  vacuously; the new comment describes the real wiring.
- `web_test.go` `newTestServer` passes nil; probe tests open a real temp
  store via `store.Open(ctx, filepath.Join(t.TempDir(), "probe.db"))`
  (signature verified at internal/store/store.go:57). Dropping the package
  global also removes the old `t.Cleanup(SetProbeStore(nil))` cross-test
  mutation hazard — a real isolation improvement.

**Deviation — `New` is variadic** (`db ...*store.DB`) vs the packet's frozen
fixed-arity third parameter (packet :36-38). Verified FORCED, not chosen:
`internal/web/middleware_logging_test.go:29` calls `New(testConfig(env),
slog.New(h))` — 2 args — and that file is outside T-208's owns (packet :11,
:60-62); attempt 1's recorded guard exit 4 is exactly this. Go has no default
arguments, so "literal 3-param signature" and "diff stays inside owns" are
mutually unsatisfiable; the variadic is the unique shape keeping main's
untouched tests compiling while the frozen call form `New(cfg, log, db)`
(packet :65) and `s.Store` survive verbatim for T-203/T-205/T-206. The diff
leaves middleware_logging_test.go untouched, as claimed. Disclosed prominently
in the handoff with planner options. Residual cost: a future production
caller writing `New(cfg, log)` now compiles and gets runtime nil-Store 500s
instead of a compile error — the compiler-is-the-reviewer guarantee (AGENTS.md
rule 6) is weakened at exactly one seam. Mitigated: `cmd/server/main.go` is
the only production caller and passes `db`; the black-box A-103 acceptance and
smoke boot suites prove the wiring on the spawned binary end-to-end. Planner
should ratify the variadic or extend owns next time the signature moves. Not a
blocker.

## Deliverable 2 — Content-Type fix: correct, and the diagnosis is right

- `probeGet` and the HTMX branch of `probePost` set
  `Content-Type: text/html; charset=utf-8` (const `probeContentType`)
  immediately before `renderPartial` — in the handlers, not templates.go,
  exactly as the packet demands (:52).
- Sniffing diagnosis independently confirmed: the fragment starts
  `<h2>Probe</h2>` (web/templates/partials/probe.html:1); Go's HTML sniff
  signature list includes `<H1` but NOT `<H2`, so the untyped response was
  sniffed `text/plain; charset=utf-8`, and the global `X-Content-Type-Options:
  nosniff` (internal/web/web.go:95) made no-JS visitors see escaped source —
  judge-M1 Gap (b). The home page starts `<!doctype html>`
  (web/templates/base.html:1), which DOES match a sniff signature, so the
  handoff's open issue 4 (home harmless today; explicitness belongs to T-205)
  is also correct.
- Error paths unchanged: `renderPartial` buffers before writing
  (internal/web/probe.go:56-66 pre-patch, unchanged), so a render failure
  writes nothing first and `http.Error` resets Content-Type to its own
  plain-text 500. The no-JS 303 branch is untouched, per the packet's
  behaviour-neutral rule (:64).

## Tests — exercise the actual behaviour at the right level

- `TestProbeFragmentContentType` is table-driven over exactly the two
  transports the packet names (GET /probe; HTMX POST /probe) and asserts the
  header **exactly** over a real `httptest.NewServer` connection with a cookie
  jar. This is the correct level: sniffing happens only on a real server
  write; a `ResponseRecorder` never sniffs, so the pre-existing
  recorder-based rows could not have caught the defect. The token-extraction
  regex matches web/templates/partials/probe.html:4 byte-for-byte, and the
  token is unpadded base64url (internal/web/csrf.go:80-87) — no html/template
  escaping can break the match. The 200s are earned through a real temp SQLite
  store, not stubs.
- The red-check claim (headers disabled → both rows fail with
  `text/plain; charset=utf-8`) follows mechanically from the sniff analysis; I
  re-derived it independently.
- Test integrity: comparing the diff against the on-disk probe_test.go, every
  pre-existing row is retained (fragment markup, no-`<html>`/`id="map"` ban,
  closed-store 500, valid-token increment vs the DB, missing/tampered 403 with
  count unchanged, no-JS 303 + `Location: /`, unparseable-body 400, home
  mount). Only helper signatures and the "store unset"→"store nil" rename
  changed, accurately. No `t.Skip`/`test.only`/`.fixme` anywhere in the diff.

## Security

CSRF: mint/verify untouched (internal/web/csrf.go; uniform 403), re-tested on
both recorder and wire. XSS: the `template.HTML` door (probeFragment, §8.4
audited, unchanged) is the sole conversion point; declaring `text/html` on
html/template-escaped output is correct and restores the intended rendering —
no new injection surface. SQL: store calls unchanged. No path traversal,
upload, or secret-output surface in scope; tokens are never logged.

## Architecture / rules

- Ownership: every diff path is inside owns ∪ handoff — `cmd/server/{main,
  boot_test}.go`, `internal/web/{web,probe,probe_test,web_test}.go`,
  `harness/handoffs/T-208.md`. `middleware_logging_test.go` is absent from
  the diff (and must be, for it to compile — see the deviation).
- HTMX/map rule: fragment has no `id="map"` (asserted), `hx-target="#probe"`;
  nothing swapped into or out of `#map` or ancestors.
- Parse-don't-validate: `parseProbePost` remains the single form-parse point
  into typed `probeSubmission`; handlers never read `r.FormValue` after.
  Typed IDs: N/A for a counter.
- No new dependencies (`allow_deps: []`: cookiejar is stdlib; `internal/store`
  is intra-module). No "Phase N", TODO/FIXME, `console.log`, or new
  `//nolint` in added code. Migrations untouched; `status.Derive` N/A.
- Acceptance safety: `acceptance/go/probe_test.go` is fully black-box (spawns
  the binary; no reference to `SetProbeStore`, `New`, or a fragment
  Content-Type assertion), and the rewire preserves every probe flow through
  the real binary — A-103/A-104/A-105 cannot regress from this diff. The
  attempt-5 diff also no longer carries any `harness/tasks/graph.json` hunk
  (the attempt-4 scheduler artifact is gone; nothing protected is touched).

## Handoff accuracy

Frozen-API section records what packet :65-66 requires; commands with exit
codes are listed; open issues (e2e-task `T=` vs `TASK=` spelling, the owns
gap for signature-moving tasks, home-page Content-Type for T-205, the
gate/scheduler race) are recorded rather than worked around — none is test
weakening. One overstatement noted below (finding 2).

## Findings

1. [Medium — disclosed, planner to ratify] Variadic `New` deviates from the
   packet's frozen fixed-arity signature; forced by
   internal/web/middleware_logging_test.go:29 being out of owns; frozen call
   form and `s.Store` preserved; compile-time store-wiring guarantee weakened
   at one seam. (internal/web/web.go New; handoff "Spec deviation".)
2. [Low-Medium] No in-process test exercises `New` with a non-nil store:
   `newProbeServer` builds via nil-`New` then assigns `s.Store = db` directly
   (internal/web/probe_test.go newProbeServer), so the constructor's
   `st = db[0]` branch is covered only by the smoke/acceptance spawns — and
   the handoff's "every store-backed test call it exactly as the packet
   freezes: `New(cfg, log, db)`" overstates the unit-test side. A one-row
   `TestNewWiresStore` would close it.
3. [Low] The wire-level POST row's client has no `CheckRedirect`/timeout
   (internal/web/probe_test.go TestProbeFragmentContentType): a regression to
   303 on an HX-Request POST would follow to `/`, which sniffs `text/html`
   (base.html:1), and both assertions would pass on the redirect target.
   `TestProbePost`'s recorder row catches that regression today, so not a
   gate hole; mirroring `http.ErrUseLastResponse` (as
   acceptance/go/probe_test.go:177-183 does) would make the new test
   self-contained.
4. [Low] New comment "The fragment is a full HTML document for a no-JS
   visitor" (internal/web/probe.go probeGet) is inaccurate — the contract test
   bans `<html>`/`<body>` in the fragment; the response body is the bare
   fragment. Comment-only.
5. [Info] The nil-store 500 row cannot distinguish the nil guard from a
   recovered panic (withRecover also answers 500), and the required message
   `probe: store not wired` is untested at unit level; pre-existing coarse
   shape, not newly weakened.
6. [Info] `New(cfg, log, db1, db2)` silently ignores `db2`; documented in the
   handoff; no such caller exists.

## Verdict rationale

Both deliverables match the packet (with the one forced, disclosed signature
deviation whose alternative — editing an out-of-owns file — the guard
correctly rejects); the fix's technical diagnosis is independently confirmed;
the new test genuinely exercises the wire-level behaviour it claims at the
only level where it manifests; no pre-existing test was deleted, skipped or
weakened; no security or architecture rule is broken; the frozen acceptance
contracts cannot regress. The findings above are follow-up material, not
gate blockers.

VERDICT: PASS
