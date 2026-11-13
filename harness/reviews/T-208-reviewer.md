# T-208 reviewer — store onto Server + /probe fragment Content-Type

Basis: the attached diff (`harness/reviews/T-208.diff`), packet
`harness/tasks/T-208.md`, judge finding `harness/findings/judge-M1.md` (Gap b,
lines 80-87 and path-to-done item 4 at 113-114), the active frozen oracles
(`acceptance/go/probe_test.go`, `acceptance/e2e/probe.spec.ts`), the pre-change
main sources, and the handoff added by the diff.

Method note: shell execution was denied in this session, so runtime claims
(test exit codes, `make check`) are cross-checked statically. Every
load-bearing claim with a pointable referent was verified by reading the tree:
the diff's "before" side matches main exactly on all six code files
(`internal/web/web.go:28-44`, `internal/web/probe.go:1-189`,
`internal/web/probe_test.go:1-39+`, `internal/web/web_test.go:28-35`,
`cmd/server/main.go:46-53`, `cmd/server/boot_test.go:1-9`).

## Deliverable 1 — store onto Server: complete and correct

- `Server.Store *store.DB` exported field with nil-semantics doc (diff,
  internal/web/web.go); `SetProbeStore`/`probeStore`/`errProbeStoreUnset`
  fully deleted — the only call sites on main were `cmd/server/main.go:49`,
  `internal/web/probe.go` itself and `internal/web/probe_test.go:36-37,181`,
  all rewritten by the diff; no remaining reference anywhere in `cmd/` or
  `internal/`.
- nil store → 500 with the exact packet-mandated message
  `probe: store not wired` (diff, internal/web/probe.go
  `errProbeStoreNotWired`, `probeCount`, `probeInc`).
- `cmd/server/main.go`: `web.SetProbeStore(db)` dropped, `web.New(cfg,
  logger, db)` in its place; boot order otherwise unchanged. `boot_test.go`
  is comment-only — correct, because those tests spawn `bin/server`
  black-box (imports at cmd/server/boot_test.go:21-38 include no
  `internal/web`; no signature in use).
- `web_test.go` `newTestServer` passes nil; probe tests open a real temp
  store via `store.Open(ctx, filepath.Join(t.TempDir(), "probe.db"))` per
  packet:44-46. The global-mutation hazard of the old
  `t.Cleanup(SetProbeStore(nil))` is gone — each test now gets a genuinely
  separate server+store (net test-isolation improvement).

## Deviation: `New` is variadic — real, forced, disclosed; planner item

Packet:36-38 freezes `New(cfg, log, db *store.DB)`; delivered is
`db ...*store.DB` (diff, internal/web/web.go). I verified each leg of the
forcing constraint independently:

- `internal/web/middleware_logging_test.go:29` calls
  `New(testConfig(env), slog.New(h))` — 2 args, the only other `New(` call
  site in the package besides `web_test.go:30` and `main.go:51`.
- That file is not covered by `owns`: `internal/web/probe*.go` covers
  `probe.go`/`probe_test.go` only (`*` does not cross `/`,
  cmd/harness/globmatch.go:10-12); `middleware_logging_test.go` shares no
  prefix with `probe*`.
- A fixed-arity 3-param `New` therefore cannot compile the branch without
  editing an unowned file, which the guard mechanically rejects
  (cmd/harness/guard.go:153-155, exit 4 — the packet's own retry context
  records exactly this failure from attempt 1). The packet is
  self-contradictory as written.
- The frozen downstream contract survives: `harness/state.md:20-22` freezes
  the call form "`New(cfg,log,db)` + `s.Store`", which the variadic satisfies
  for T-203/T-205/T-206; `SetProbeStore` is gone; misuse fails loudly
  (nil store → logged 500, tested) rather than silently.

This is a planner-ratification item, not a worker defect: the honest
alternative was a `BLOCKED` handoff, and the chosen compromise is fully
disclosed with a one-line remediation (extend owns next time the signature
moves). Residual cost: the compiler no longer enforces the store argument —
worth recording, not worth blocking correct, fully-tested work.

## Deliverable 2 — Content-Type fix: correct, at the right layer, tested at the wire

- Header set exactly where the packet demands (probeGet; HTMX branch of
  probePost only — packet:52-53 forbids touching `renderPartial`/
  templates.go, respected); const `probeContentType`; no-JS 303 branch
  untouched; `probeFragment` (home mount) correctly untouched — the page
  starts `<!doctype html>` and sniffs text/html.
- Ordering is safe Go semantics: `renderPartial` buffers (probe.go:56-67 on
  main, unchanged), so on render failure nothing is written and `http.Error`
  overrides the header with its own `text/plain` 500 — the handoff's "error
  responses unchanged" claim is accurate.
- The defect mechanism is real: the partial starts `<h2>Probe</h2>`
  (web/templates/partials/probe.html:1); Go's `http.DetectContentType` HTML
  signatures include `<H1` but not `<H2`, so pre-fix the wire response was
  sniffed `text/plain; charset=utf-8` under the global nosniff
  (internal/web/web.go:95) — exactly judge-M1 Gap (b). Matches the recorded
  red/green reproduction.
- New test `TestProbeFragmentContentType`: table of two rows (GET /probe;
  HTMX POST /probe with a token minted through the real CSRF flow — cookie
  jar + `probeReToken`, whose regex matches the template's hidden input at
  web/templates/partials/probe.html:4), over `httptest.NewServer` +
  `s.Handler()` so the real middleware chain (incl. nosniff) and real
  sniffing are in play, asserting the header **exactly**
  `text/html; charset=utf-8` per packet:54-56. The recorder-based rows
  elsewhere could never see this defect; the wire-level design is the right
  one and the test genuinely fails pre-fix. Compile-checked statically:
  `strings` (probe_test.go:12) and `store` (:16) already imported; diff adds
  only `net/http/cookiejar`.

## Frozen oracles: no conflict

- `acceptance/go/probe_test.go` spawns the binary and asserts body regexes
  (probeReCount/probeReInput), never Content-Type and never
  `SetProbeStore`/`web.New` — unaffected by the refactor.
- `acceptance/e2e/probe.spec.ts` exercises the home page (HTMX swap +
  server-rendered no-JS section) and never navigates directly to `/probe`,
  so making the direct-visit fragment render as HTML cannot break it.
- A-103/A-104/A-105 behaviour preserved; the HTMX/#map contract test in
  acceptance/go/m1_guards_test.go is untouched and the probe partial still
  targets `#probe` (never `#map`).

## Protected paths, ownership, rules

- Code paths in the diff are exactly `cmd/server/{main,boot_test}.go`,
  `internal/web/{web,web_test,probe,probe_test}.go` + the handoff — all
  inside owns ∪ the allowed handoff (guard.go:139).
- The `harness/tasks/graph.json` hunk in the diff is NOT a worker violation:
  it is scheduler-owned and explicitly exempt from both protected-path and
  ownership checks (cmd/harness/guard.go:121-125 and 146-148), and
  `finalizeMerge` restores main's copy before merging (cmd/harness/gate.go:
  122-126) — branch content is never authoritative. Its diff base
  (`attempts: 1`) no longer matches main (`attempts: 2`, status review),
  which corroborates the handoff's scheduler-race narrative; the handoff's
  predicted rebase conflict and its resolution (take main's version) match
  the gate's actual behavior.
- No dependency change (cookiejar is stdlib; `allow_deps: []` respected);
  no TODO/FIXME/`panic("not implemented")`/nolint-without-reason in added
  lines (scan.go:24-31 exempts tests/handoff anyway; the non-test additions
  in probe.go/web.go/main.go are clean); no "Phase N" strings in code; no
  skip/only markers; ratchet only grows (`TestProbeFragmentContentType`
  added, no test deleted — "store unset answers 500" is renamed, same
  assertion shape).

## Architecture & security

- Ownership map (docs/spec/architecture.md:15-25) respected: store access
  stays `internal/web` → `internal/store`; the change *removes* hidden
  package-global mutable state. Parse-don't-validate intact
  (`parseProbePost` unchanged, no `r.FormValue` added). The single audited
  `template.HTML` door (`probeFragment`) and its nolint reasons untouched.
- CSRF matrix retained in full (missing/tampered → 403 with count unchanged,
  both still asserted); no new `template.HTML` conversions; no SQL, path,
  upload or secret surface touched; the header fix makes the declared type
  match the actual escaped, server-rendered content under nosniff — the
  correct security direction.

## Findings (non-blocking)

1. **Variadic `New` (see above)** — planner must ratify or schedule the
   fixed-arity change with `internal/web/middleware_logging_test.go` (or
   `internal/web/**`) in owns. Disclosed by the handoff with remediation.
2. **Handoff overstatement (minor):** "Production and every store-backed
   test call it exactly as the packet freezes: `New(cfg, log, db)`" — no
   unit test passes a live store through the constructor; `newProbeServer`
   assigns `s.Store = db` after a nil-store `New`. The positive
   constructor-with-store branch is unit-untested; it is covered end-to-end
   by the smoke suite and the acceptance spawn suites (a wiring regression
   500s there), but a one-row `New(cfg, log, db)` → `s.Store == db` unit
   test would close the gap cheaply.
3. **Nil-store 500 row is panic-equivalent:** the subtest asserts only
   `rec.Code == 500`; if the `s.Store == nil` guard were deleted,
   `s.Store.ProbeCount` would nil-deref (`store.DB` fields are pointers,
   internal/store/store.go:20-22), `withRecover` would answer the same 500,
   and the row would still pass. Pre-existing assertion weakness carried
   over from main's "store unset" row (not a weakening); a direct unit
   assertion on `errProbeStoreNotWired` or the log line would pin the
   packet's error message.
4. **POST-row redirect robustness:** the wire-level POST client follows
   redirects; a regression to 303 on the HTMX path would follow to `/`
   (which sniffs text/html) and the row would pass. Mitigated today by
   `TestProbePost`'s recorder rows pinning the fragment response;
   `CheckRedirect: http.ErrUseLastResponse` (the pattern
   acceptance/go/probe_test.go:177-183 already uses) would harden it.
5. **Comment nit:** probeGet's new comment calls the fragment "a full HTML
   document" — it is not (TestProbeGet asserts no html/body); the intended
   meaning is that the response body is nothing but the fragment.
6. **Handoff open issues verified true:** `make e2e-task T=` exits 2 because
   the Makefile variable is `TASK` (Makefile:8,113); `node_modules` symlink
   whitelist is real (cmd/harness/ratchet.go:128); `renderPage`'s missing
   Content-Type is correctly left alone (templates.go unowned; page sniffs
   text/html from the doctype); `max_attempts: 3` (harness/policy.json:31)
   matches "attempt 3 of 3"; the ff-only refusal message in the packet's
   retry context matches gate.go:130-131 verbatim and `commitHarnessState`
   exists as described (cmd/harness/sched.go:106,158-161).

## Verdict rationale

Both deliverables are semantically complete and correct against the packet;
the header fix is implemented at exactly the layer the packet specifies and
tested at the wire level where the defect actually lived, with a credible
red/green mechanism; no frozen oracle is weakened, narrowed or in conflict;
owns, protected paths, deps and marker rules are all respected (the graph.json
hunk is a guard-exempt scheduler artifact); and every checkable factual claim
in the handoff verified true, including its own deviation disclosure. The
findings above are test-strength gaps and a disclosed, mechanically forced
API-shape compromise for the planner to ratify — none is a correctness,
security or test-integrity defect.

VERDICT: PASS
