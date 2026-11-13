# T-208 reviewer — store onto Server + /probe fragment Content-Type

Artifact: `harness/reviews/T-208.diff` (branch diff vs main) + handoff
`harness/handoffs/T-208.md`. Packet: `harness/tasks/T-208.md`.

Method note: reviewer shell access was unavailable in this session, so runtime
claims (test exits, `make check`) are cross-checked statically — every
load-bearing factual claim in the handoff that has a pointable referent in the
tree was verified by reading it (details below). The worktree `../wt/T-208` is
outside this session's permitted root; main-tree files match the diff's
"before" state exactly (spot-verified `internal/web/web.go`, `probe.go`,
`probe_test.go`, `web_test.go`, `cmd/server/main.go`, `boot_test.go`).

## Packet compliance

**Deliverable 1 (store onto Server)** — delivered:
- `Server.Store *store.DB` exported field with nil-semantics doc comment
  (diff, internal/web/web.go:30-40); `SetProbeStore`/`probeStore`/
  `errProbeStoreUnset` fully deleted — grep of the diff confirms removal and
  no remaining caller anywhere in `cmd/`, `internal/` (only historical
  reviews/handoffs mention the name).
- nil store → 500 with the exact packet-mandated message
  `probe: store not wired` (diff, internal/web/probe.go:26-27,57-71).
- `cmd/server/main.go`: `web.SetProbeStore(db)` dropped, `web.New(cfg,
  logger, db)` in its place; boot order unchanged otherwise. boot_test.go is
  comment-only (correct — those tests spawn the binary black-box; no
  signature in use).
- `web_test.go` `newTestServer` passes nil; probe tests open a real temp
  store via `store.Open(ctx, t.TempDir()/probe.db)` per packet:44-46.
- Owns compliance: branch touches only `cmd/server/**`, `web.go`,
  `web_test.go`, `probe.go`, `probe_test.go` + the handoff file (explicitly
  allowed, cmd/harness/guard.go:139). Guard mechanics verified:
  `exitOwnership = 4` (guard.go:17), check over the branch diff
  (guard.go:92,138-159), glob semantics per globmatch.go:13-19 —
  `internal/web/probe*.go` does not cover `middleware_logging_test.go`.

**Deliverable 2 (Content-Type)** — delivered exactly as specified:
- Header set in `probeGet` and the HTMX branch of `probePost` only (packet
  explicitly forbids touching `renderPartial`/templates.go — respected);
  const `probeContentType`; no-JS 303 branch untouched.
- Ordering is safe: `renderPartial` buffers (internal/web/probe.go:56-67 on
  main, unchanged), so on render failure nothing has been written and
  `http.Error` overwrites the header with its own `text/plain` 500 — the
  handoff's "error responses unchanged" claim is correct Go semantics.
- Table test `TestProbeFragmentContentType` (probe_test.go, new): two rows
  (GET /probe; HTMX POST /probe with minted token via cookie jar), asserting
  the header **exactly** `text/html; charset=utf-8` as packet:54-56 demands.
- The wire-level design is the right one and the handoff's reasoning is
  technically sound: `httptest.ResponseRecorder` never sniffs (sniffing
  lives in net/http's chunkWriter), so only `httptest.NewServer` reproduces
  the defect; and the partial starts `<h2>Probe</h2>`
  (web/templates/partials/probe.html:1) — `<H2` is indeed absent from Go's
  `htmlSig` sniff list (which has `<H1`, not `<H2`), so the pre-fix
  text/plain sniff claim is credible, as is the recorded red/green
  reproduction.
- No frozen contract conflicts: grep of `acceptance/` shows no assertion on
  /probe's Content-Type and no reference to `SetProbeStore`/`web.New`
  (acceptance spawns the binary) — A-103/104/105 staying green is plausible.

## Architecture

- Ownership map (docs/spec/architecture.md:15-25) respected: store access
  stays `internal/web` → `internal/store`; the change *removes* a
  package-global mutable pointer, which is a net improvement (kills hidden
  process state and the old tests' `t.Cleanup(SetProbeStore(nil))` mutation
  of shared globals — the new nil-store subtest builds a genuinely separate
  server).
- HTMX/#map rule untouched; the `fragment has no html/body and no id="map"`
  subtest is retained.
- Parse-don't-validate preserved: `parseProbePost` unchanged; no
  `r.FormValue` reads added. The single audited `template.HTML` door
  (`probeFragment`) and its nolint reasons are untouched.
- No new deps (stdlib `net/http/cookiejar` only), no TODO/FIXME, no
  "Phase N" strings in code, no skip markers, no `console.log`.

## Security

CSRF matrix retained in full (missing/tampered → 403 with count unchanged);
no new `template.HTML` conversions; no SQL, path, or upload surface touched;
error paths log the error and emit generic text (no secret leakage); the
header fix makes the declared type match actual content under global
`nosniff` — the correct direction. Nothing adversarial introduced.

## Findings

1. **Spec deviation, disclosed: `New` is variadic, not fixed-arity.**
   Packet:36-38 says the signature "becomes `New(cfg, log, db *store.DB)`";
   delivered is `db ...*store.DB` (diff, internal/web/web.go:41-52). The
   forcing constraint is real — I verified each leg:
   internal/web/middleware_logging_test.go:29 calls `New(testConfig(env),
   slog.New(h))` (2 args, fixed); that file is not covered by T-208's owns
   (globmatch.go:13-19); and a fixed 3-param `New` cannot compile that call.
   Attempt 1's edit of the file was correctly rejected by the guard
   (guard.go:153-155, exit 4 — the packet's own retry context confirms).
   The delivered signature preserves every downstream-frozen element:
   `New(cfg, log, db)` compiles and is what main.go uses; `s.Store` is the
   access path; harness/state.md:20-22 records the freeze as the *call form*
   "`New(cfg,log,db)` + `s.Store`", which the variadic satisfies for
   T-203/T-205/T-206 (state.md:26-28). Residual cost: a caller that omits
   the store (or passes extras, silently ignored past the first) gets no
   compile error — the packet's compile-time guarantee becomes a loud,
   tested runtime 500. This is a planner-ratification item, not a worker
   defect: the packet is unsatisfiable as literally written, the honest
   alternative was a `BLOCKED` handoff, and the chosen compromise is fully
   documented with a one-line remediation (extend owns next time the
   signature moves). Accepted here; the planner should either ratify the
   variadic or schedule the fixed-arity change with `middleware_logging_test.go`
   in owns.

2. **Handoff overstatement (minor):** "every store-backed test call[s] it
   exactly as the packet freezes: `New(cfg, log, db)`" — no unit test passes
   a live store through the constructor; probe tests assign `s.Store = db`
   after a nil-store `New` (diff, probe_test.go `newProbeServer`). The
   constructor-with-store path is exercised only by the smoke suite
   (black-box spawn). Behaviourally equivalent (`New` just copies `db[0]`),
   but if this file is touched again a one-row unit test
   (`New(cfg, log, db)` → `s.Store == db`) would close the gap.

3. **Nits:** `newTestServer` passes an explicit `nil` third arg that the
   variadic makes unnecessary (harmless; documents intent). "store failure
   answers 500" closes the DB that cleanup will close again (pre-existing
   pattern, error ignored). Neither matters.

4. **Handoff open issues verified:** `make e2e-task T=` usage-branch exit 2
   is consistent with a `TASK`-variable Makefile; the `node_modules` symlink
   whitelist is real (cmd/harness/ratchet.go:128); `renderPage`'s missing
   Content-Type is correctly left alone (templates.go not owned; page starts
   `<!doctype html>`, which sniffs as text/html).

## Verdict rationale

Both deliverables are semantically complete and correct against the packet;
the test additions genuinely exercise the fixed behaviour at the level where
the defect lived; owns, protected paths, deps, and marker rules are all
respected; every checkable factual claim in the handoff verified true. The
single deviation (variadic `New`) is real but was mechanically forced by the
protected guard against a self-contradictory packet, preserves every frozen
downstream contract, fails loudly on misuse, and is disclosed with a
remediation path — flag for planner ratification, not grounds to block.

VERDICT: PASS
