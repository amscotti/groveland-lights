# T-208 reviewer — store onto Server + /probe fragment Content-Type

Basis: attached diff, packet `harness/tasks/T-208.md`, judge finding
`harness/findings/judge-M1.md` (Gap b, lines 80–87; path-to-done item 4, lines
113–114), frozen oracles `acceptance/go/probe_test.go` +
`acceptance/e2e/probe.spec.ts`, current main sources, and the handoff added by
the diff.

Method note: shell execution was denied this session, so runtime claims (exit
codes, `make check`) are cross-checked statically. Every load-bearing claim
with a pointable referent was verified against the tree; the diff's "before"
side matches main exactly on all five code files.

## Deliverable 1 — store onto Server: complete, one disclosed deviation

- `Server.Store *store.DB` exported field with nil-semantics doc; globals
  `probeStore`/`SetProbeStore`/`errProbeStoreUnset` fully deleted. Grep of
  main confirms the only call sites were `cmd/server/main.go:49,51`,
  `internal/web/probe.go` itself and `internal/web/probe_test.go:36-37,181` —
  all rewritten by the diff; no stragglers.
- Nil store → 500 with the exact packet-mandated message `probe: store not
  wired` (`errProbeStoreNotWired`; `probeCount`/`probeInc` nil checks).
- `cmd/server/main.go`: `SetProbeStore(db)` dropped, `web.New(cfg, logger, db)`
  in its place; boot order (config → logger → `store.Open` → checksum refusal →
  `defer db.Close()` → `web.New`) otherwise unchanged.
- `boot_test.go` comment-only is correct: those tests spawn `bin/server`
  black-box (imports at cmd/server/boot_test.go:21-38 pull in no
  `internal/web`), so no signature was in use to update. Packet line 43's
  "update boot_test.go" is satisfied vacuously; the comment now describes the
  real wiring.
- `web_test.go` `newTestServer` passes nil (packet:44); probe tests open a real
  temp store via `store.Open(ctx, filepath.Join(t.TempDir(), "probe.db"))`
  (packet:44-46). Dropping the package global removes the old
  `t.Cleanup(SetProbeStore(nil))` cross-test mutation hazard — a genuine
  isolation improvement, not a regression.
- Retained behaviour coverage is real, not just structural: nil-store 500,
  closed-store 500, valid-token increment verified against the DB handle,
  missing/tampered token 403 with count unchanged, no-JS 303, unparseable body
  400, home mount markup.

**Deviation — `New` is variadic** (`db ...*store.DB`, web.go) vs the packet's
frozen fixed-arity `db *store.DB` (packet:36-38). Verified FORCED, not chosen:
`internal/web/middleware_logging_test.go:29` calls `New(testConfig(env),
slog.New(h))` — 2 args — and that file is outside T-208's owns
(packet:11,60-62); guard `checkOwnership` (cmd/harness/guard.go:138-159) has no
exemption for it, and attempt 1's recorded failure ("guard exit 4", packet
retry context) is exactly this. Go has no default arguments, so the packet's
two requirements — literal 3-param signature AND diff inside owns — are
mutually unsatisfiable; the variadic is the unique signature satisfying both
"main's tests compile untouched" and "owns clean" while the frozen *call form*
`New(cfg, log, db)` (packet:65) still compiles verbatim for T-203/T-205/T-206
and `s.Store` remains the access path. Disclosed prominently in the handoff
with the planner-fix option. Residual cost, worth recording: a future
production caller that forgets the store (`New(cfg, log)`) now compiles and
gets nil-Store 500s at runtime instead of a compile error — the
compiler-is-the-reviewer goal (AGENTS.md rule 6) is weakened at exactly one
seam. Mitigated today: `cmd/server/main.go` is the only production caller and
passes `db`; the smoke-tagged boot tests and A-103 acceptance prove the wiring
end-to-end on the spawned binary. Planner should bless the variadic or extend
owns for whichever task next moves the signature. Not a blocker.

## Deliverable 2 — Content-Type fix: correct, and the diagnosis is right

- `probeGet` and the HTMX branch of `probePost` set
  `Content-Type: text/html; charset=utf-8` (const `probeContentType`)
  immediately before `renderPartial` — set in the handlers, not templates.go,
  exactly as the packet demands (packet:52).
- The sniffing diagnosis is technically accurate: Go's `htmlSig` list
  (net/http sniff) includes `<H1` but NOT `<H2`; the fragment starts
  `<h2>Probe</h2>` (web/templates/partials/probe.html:1), so untyped responses
  were sniffed `text/plain; charset=utf-8` and the global nosniff
  (web.go `withSecurityHeaders`) made no-JS visitors see escaped source —
  judge-M1 Gap (b) confirmed. The home page starts `<!DOCTYPE html>`
  (web/templates/base.html:1), which DOES match a sniff signature, so the
  handoff's open issue 4 (home currently harmless; explicitness belongs to
  T-205) is also correct.
- Error paths unchanged: `renderPartial` buffers before writing (probe.go
  doc comment), so a render failure writes nothing first, and `http.Error`
  forcibly resets Content-Type to text/plain — the handoff's claim holds.
- No-JS 303 branch untouched (packet: behaviour-neutral except the header).

## Tests — exercise the actual behaviour at the right level

- `TestProbeFragmentContentType` is table-driven over exactly the two
  transports the packet names (GET /probe; HTMX POST /probe) and asserts the
  header **exactly** over a real `httptest.NewServer` connection with a
  cookie jar. This is the correct level: the defect was wire-level (Go sniffs
  only on real writes; `ResponseRecorder` never sniffs), so a recorder-based
  assertion could not have caught it. The token-extraction regex
  `<input type="hidden" name="csrf_token" value="([^"]+)">` matches
  partials/probe.html:4 byte-for-byte, and the mint-then-POST flow matches the
  double-submit contract in internal/web/csrf.go (cookie `csrf_token` frozen
  at csrf.go:20 vs form value). Real store behind it via `newProbeServer`, so
  the 200s are earned through the DB, not stubbed.
- The red-check claim (headers disabled → both rows fail with
  `text/plain; charset=utf-8`) is consistent with the sniff mechanics; I
  re-derived it independently above.
- Minor coverage note: no in-process unit test exercises the variadic `New`
  with a non-nil db (probe tests assign `s.Store` directly after a nil-`New`);
  the New→Store wiring path is covered only by the smoke-tagged spawn tests
  and acceptance suite. Acceptable for a chore; noting for completeness.

## Security

- CSRF: nothing changed in mint/verify; 403 paths re-tested over both recorder
  and wire. XSS: the `template.HTML` door (probe.go `probeFragment`, §8.4
  audited) is untouched, and declaring `text/html` on the fragment is correct
  because the bytes are html/template-escaped output — no new injection
  surface. SQL: store calls unchanged (`ProbeCount`/`ProbeInc` via
  internal/store). No path traversal, upload, or secret-output surface in
  scope; tokens are never logged.

## Architecture / rules

- Ownership: every diff path is inside owns ∪ handoff — `cmd/server/**`,
  `internal/web/{web,probe,probe_test,web_test}.go`,
  `harness/handoffs/T-208.md`. `middleware_logging_test.go` correctly absent.
- HTMX/map rule: partial has no `id="map"`, `hx-target="#probe"` — untouched;
  the no-partial-contains-map contract is unaffected.
- Parse-don't-validate: `parseProbePost` remains the single form-parse point;
  handlers read the typed `probeSubmission`. Typed IDs: N/A for a counter.
- No new dependencies (go.mod/go.sum untouched; `internal/store` import is
  intra-module; cookiejar is stdlib). No "Phase N" strings, TODO/FIXME,
  `console.log`, or new `//nolint` in added code.
- Acceptance safety: grep of `acceptance/` shows no response Content-Type
  assertion on the fragment (only request Content-Type at
  acceptance/go/probe_test.go:199 and the unchanged nosniff check at
  acceptance/go/healthz_test.go:35-36) — the header addition cannot break the
  frozen A-103/A-104/A-105 specs, consistent with the handoff's green claims.

## The `harness/tasks/graph.json` hunk — gate artifact, not a rule-1 breach

The diff carries one graph.json change (`status: review → merging`). This is
scheduler-owned bookkeeping, not worker tampering: the guard exempts the file
in BOTH `checkProtectedPaths` and `checkOwnership` with the rationale that
branch content is never authoritative (cmd/harness/guard.go:117-125,145-148),
and `finalizeMerge` itself creates exactly this artifact on task branches —
`git checkout main -- harness/tasks/graph.json` + commit
`chore(harness): keep scheduler graph state on merge` (cmd/harness/gate.go:
121-129) — which is the commit the handoff tells the integrator to `rebase
--skip`. Git history is unverifiable in this denied-shell session, so
provenance rests on those mechanisms plus the hunk being a bare status flip
with zero task content; I find that sufficient. Documented here so the gate
sees it was examined.

## Handoff accuracy spot-checks (it makes strong harness claims)

- `runIntegrator` abort-loophole at sched.go:561-569: real — "no rebase in
  progress" + clean tree is treated as success, so a `--abort` masquerades as
  a finished rebase. The handoff's DO-NOT-ABORT instruction and the
  `merge-base --is-ancestor` suggestion are sound.
- gate.go:127 as the source of the graph.json artifact: verified (above).
- The topology analysis (ff-only refusal from scheduler commits diverging
  main) matches the packet's own retry context verbatim.
- Open issue 1 (`make e2e-task T=` vs `TASK=`): consistent with a protected
  Makefile; correctly recorded rather than worked around.

## Findings summary

1. Variadic `New` deviates from the packet's frozen signature — forced by the
   owns list (middleware_logging_test.go:29 is out of owns and calls 2-arg
   `New`), disclosed, frozen call form preserved; planner should bless or
   extend owns later. (web.go, handoff "Spec deviation".)
2. graph.json hunk in the diff is gate-generated scheduler state, exempted by
   guard and restored by finalizeMerge; provenance unverifiable in this
   session — flagging for the record, no task content at stake.
3. Minor: 3-arg `New` with non-nil store has no in-process unit coverage
   (covered only via smoke/acceptance spawns); variadic invites silently
   store-less production callers — the one seam where rule 6's guarantee
   weakened.
4. Minor: `New(cfg, log, db1, db2)` silently ignores `db2`; documented, no
   such caller exists.
5. Non-code: handoff is attempt-heavy but its harness claims (sched.go:561-569,
   gate.go:121-129) verify against the tree — unusual and welcome.

Both deliverables match the packet (with the one forced, disclosed signature
deviation), the fix's technical diagnosis is independently confirmed, the new
test genuinely exercises the wire-level behaviour it claims, frozen
acceptance contracts cannot regress, and no security or architecture rule is
broken.

VERDICT: PASS
