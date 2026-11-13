# T-105 review — Activate the M1 acceptance suite (A-103/A-104)

Reviewed: the attached diff (`T-105.diff`), read against the operative packet
(`harness/tasks/T-105.md`), the frozen oracles it cites (`acceptance/drafts/*`
still present at HEAD in this checkout, `harness/tasks/T-100.md:93-146`,
`harness/tasks/T-103.md:39-62`), the merged implementation
(`internal/web/probe.go`, `internal/web/templates.go`,
`web/templates/pages/index.html`, `web/templates/partials/probe.html`,
`internal/store/dbfile_layout_test.go`), and the harness itself
(`cmd/harness/guard.go`, `cmd/harness/ratchet.go`).

Verification limits: the diff is not applied to this checkout (still
pre-activation; confirmed `acceptance/drafts/` present, activated files
absent), and shell beyond read-only git is denied (`go`, `npx`, `shasum`,
`openssl` all permission-rejected), so the handoff's exit codes are taken on
trust; every content, hash, markup and harness-logic claim was traced
statically. The holdout repo (`harness/policy.json:52`) is outside the sandbox
(`external_directory` denied) — see finding 3.

## The one scrutinised test edit — sanctioned, faithful, strictly tighter

`acceptance/go/probe_test.go` (diff hunk at draft line 52):
`probeReCount` changed from ``Count:\s*(\d+)`` to
``Count:\s*<span id="probe-count">(\d+)</span>``. This is the packet's
explicitly sanctioned mechanical class, not a weakening:

- T-105 packet rule 3, second bullet (`harness/tasks/T-105.md:40-45`) permits
  a minimal edit for "a mechanical defect that does not change what it asserts
  (e.g. … **a selector that must be scoped** …)", requires every such edit be
  listed with justification — done (handoff "Exact edits", edit #1).
- The old regex was unsatisfiable against the very markup frozen alongside it:
  T-103's packet pins the fragment byte-exactly as
  `<p>Count: <span id="probe-count">N</span></p>`
  (`harness/tasks/T-103.md:49`); the merged partial renders exactly that
  (`web/templates/partials/probe.html:2`); the frozen e2e spec — activated
  verbatim, zero edits — independently requires `#probe-count` to hold
  digits-only text (`acceptance/drafts/probe.spec.ts.txt:33`,
  `toHaveText(/^\d+$/)`). After `Count: ` the raw HTML is `<span`, which
  `Count:\s*(\d+)` can never match. The Go regex runs over raw HTML
  (`probeFragment`, draft lines 214-237), unlike the e2e `getByText` — this is
  exactly the draft-vs-runtime selector mismatch class.
- The replacement keeps the `(\d+)` capture and every other assertion
  byte-identical (diff touches no other hunk in the test bodies; git
  similarity 96%) and ADDS the requirement that N live inside `#probe-count` —
  it fails on strictly more markup, including any contract-compliant
  regression to bare `Count: N`. Looser-match/weaker-check direction: none.
- The disposition was pre-registered by T-103's handoff gap (b)
  (`harness/handoffs/T-103.md:121-141`) before T-105 ran — not post-hoc
  test-fitting.

## Remaining activation moves — verified faithful

- `acceptance/e2e/probe.spec.ts`: pure rename. Git similarity 100% (diff
  header) and the regenerated manifest carries the draft's hash unchanged
  (`4ec1052d…` on both the removed `drafts/probe.spec.ts.txt` line and the new
  `acceptance/e2e/probe.spec.ts` line; old manifest verified at
  `HEAD:harness/protected.sha256`). Byte-identical, no edits.
- `acceptance/go/migrations_test.go`: one gofmt comment re-indent only
  (similarity 99%, single hunk is the header `git mv` line). No A-104 token
  index pin was needed and none was made: `migTamperFirstHex` still uses the
  first token, and `internal/store/dbfile_layout_test.go:13-60` pins that the
  recorded checksum is the FIRST and ONLY 64-char lowercase-hex token after a
  healthy boot+close — the frozen test is sound as written.
- `gofmt` on the two `.go` files: comment-indent normalisation only (the two
  header hunks); required by the fmt gate; zero semantic change.
- No `t.Skip` / `test.skip` / `test.only` / `.fixme` anywhere in the diff; the
  failing no-JS test is left red rather than removed or narrowed.
- Ownership: diff touches only `acceptance/**`, `harness/protected.sha256`,
  `harness/.ratchet.json`, and the handoff — all within the packet's owns plus
  the ratchet baseline, which `cmd/harness/guard.go:140-144` explicitly allows
  an acceptance task to regenerate. No app code, no `go.mod`, no Makefile.
  Counts only grew (`{"go":28→51,"pw":20→32}`; `RatchetCompare`,
  `cmd/harness/ratchet.go:90-98`, fails only on decrease; pw +12 = 3 new e2e
  tests × 4 projects, as the packet's deliverable 2 predicts).
- Manifest regeneration is internally consistent: unchanged files carry
  identical hashes to `HEAD:harness/protected.sha256`; drafts lines removed
  with the directory (sanctioned by packet deliverable 1, "the README too").

## The BLOCKED no-JS clause — the oracle working, correctly routed

The frozen no-JS e2e test (`acceptance/drafts/probe.spec.ts.txt:76-94`,
activated verbatim) requires `GET /` with JS off to server-render a populated
`section#probe` (heading `Probe`, `Count: N` in `#probe-count`, increment
form). The merged home is the empty shell + `<a href="/probe">Probe</a>`
(`web/templates/pages/index.html:10-12`) — exactly what T-103's deliverable
prose ordered (`harness/tasks/T-103.md:99-104`) while T-103's own contract
line (`harness/tasks/T-103.md:43-44`), T-100's packet
(`harness/tasks/T-100.md:116-118`) and the drafts README (contract table row
`GET /` HTML; visual notes) all demand the populated server-side section. The
packet contradiction is in T-103's deliverable text, not in the test. With JS
off nothing fills the shell, so the heading lookup at `probe.spec.ts:83`
fails deterministically on all four projects — matching the handoff's 4/32
red. Per packet rule 3 first bullet the worker did NOT edit the test and
wrote `BLOCKED:` as the handoff's first line — the documented flow
(`harness/state.md:33-34` "feature-vs-test disputes → BLOCKED to planner").
The fix sketch's technical caveat is correct: `internal/web/templates.go:44-60`
parses pages with only `base.html` + the page file, so
`{{template "partials/probe.html" .}}` in `index.html` will not resolve
without extending `parseTemplates` or inlining the markup.

## Findings

1. **[Medium, procedural] Unilateral re-freeze of one oracle string.** The
   deleted drafts README ("that is the only sanctioned edit") was narrower
   than the operative T-105 packet's mechanical-defect class; the worker
   applied the packet rule (which governs) and disclosed + requested
   ratification in the handoff. The new regex also pins one exact tag
   spelling, so a future contract-compliant markup variation would fail —
   brittleness in the stricter direction, acceptable for a byte-exact
   contract, but the planner should ratify the frozen string when cutting the
   fix task. Not a weakened assertion.
2. **[Low] Manifest regenerated on the branch that edited the protected
   tests** — the documented tamper vector (`cmd/harness/guard.go` allows an
   acceptance task to regenerate). Mitigated here by hash continuity for the
   pure rename and 96%/99% similarity for the two edited files, both
   reviewed line-by-line. Pre-existing harness holes compound it:
   `countGoTests` runs without `-tags acceptance`
   (`cmd/harness/guard.go:337-349`) so the four activated Go tests are not
   ratchet-counted, and the new-skip-marker scan skips `acceptance/`
   (`cmd/harness/guard.go:169-170`). Planner follow-up, correctly flagged in
   the handoff (note 5); not a T-105 violation.
3. **[Low] Holdout activation not independently verifiable here** (outside
   sandbox; diff not attached). The handoff quotes the identical one-line
   `hsReCount` edit with rationale; `make holdout` at the gate is the
   compensating control. Main+holdout now share the implementation-shaped
   regex — an inherent cost of the sanctioned class; worth noting for the
   holdout's independence.
4. **[Low] Stale "INERT DRAFT / NO content edits" headers** remain in the
   activated files (e.g. new `probe_test.go:6-10`). Disclosed in the handoff;
   reconcile at ratification. Audit-trail noise only.
5. **[Info] Handoff pre-asserts both reviewer PASS verdicts** — the files
   exist, so not fabricated, but a handoff citing its own reviews is process
   smell; this review was derived independently of them.
6. **[Info] Security/arch spot-checks clean:** diff touches no app code
   (CSRF/XSS/SQL/upload/path-traversal surfaces unchanged — for the record,
   the merged `probePost` maps `ErrCSRF`→403 without incrementing and the
   no-JS flow 303s to `/`, `internal/web/probe.go:130-170`; tests use only
   `.env.test` dummy secrets, `t.TempDir()` paths, real spawns — no mocks, no
   coverage theatre). HTMX targets `#probe`, never `#map`. Parse-once at the
   boundary into a typed struct (`parseProbePost`, `probe.go:108-125`). No
   "Phase N" strings, no console.log, no new deps.

## Verdict rationale

T-105's own deliverable — faithful, verifiable activation with no weakened,
skipped or special-cased assertion — is met; the residual red is a frozen
oracle correctly catching a feature-side gap that T-103 flagged, that no task
currently owns the fix for, and that T-105 is forbidden from fixing
(packet "Never": app code). The packet's designed terminal state for exactly
this situation is `BLOCKED` in the handoff (written, first line) while the
tests stand unedited. The merge gate independently holds the branch red until
the planner's fix task lands; a reviewer BLOCK here would add nothing except
pressure toward the one forbidden move (editing the test). PASS is on test
integrity and rule compliance, not merge readiness.

VERDICT: PASS
