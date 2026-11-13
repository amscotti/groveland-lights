# T-105 review (attempt 4) — Activate the M1 acceptance suite (A-103/A-104)

Review basis: the attached diff, read against the operative packet
(`harness/tasks/T-105.md`), the frozen drafts — still present, unactivated, in
this checkout (`acceptance/drafts/*`, so every activated byte was compared to
its oracle directly), the feature contracts (`harness/tasks/T-100.md`,
`harness/tasks/T-103.md`), T-103's pre-registered gaps
(`harness/handoffs/T-103.md`), the merged implementation (`internal/web/probe.go`,
`internal/web/web.go`, `web/templates/**`, `internal/store/dbfile_layout_test.go`),
the harness (`harness/protected.sha256`, `harness/.ratchet.json`,
`cmd/harness/guard.go`), the attempt-3 reviews (both BLOCK, with prescribed
remediation, reproduced in `harness/tasks/T-105.md:99-475`), and
`docs/spec/architecture.md` (ownership map, HTMX/#map rule).

Verification limits: the diff is not applied to this checkout and shell
execution is denied, so exit codes, `make protect`/`make holdout` runs and the
holdout repo are taken on trust; every claim that can be checked against the
tree was checked, and the internally verifiable ones (byte-identity, ratchet
arithmetic, manifest consistency, the two gap diagnoses) all hold.

## The attempt-3 remediation was implemented exactly

Both attempt-3 reviews BLOCKed on one thing: the no-JS oracle had been
rewritten to the implementation's shape. Their prescribed remediation was
(i) revert `acceptance/e2e/probe.spec.ts` to the frozen bytes and leave it red
at `:83` as the routed dispute, (ii) keep the regex pin + gofmt, keep the
BLOCKED first line, present both gaps, drop the stale-verdict citation and the
"byte-equivalent" overstatement, (iii) route planner actions. Attempt 4 does
precisely this:

1. **The frozen e2e oracle is restored byte-exact.** The diff activates it as a
   pure rename (`similarity index 100%`, zero content hunks), and the
   regenerated manifest carries sha256 `4ec1052d…` for
   `acceptance/e2e/probe.spec.ts` — the identical hash the pre-diff manifest
   (`harness/protected.sha256:3`) carried for
   `acceptance/drafts/probe.spec.ts.txt`. Two independent signals inside the
   diff agree on byte-identity; the attempt-3 rewrite (`20a8c976…`) is gone.
   The on-disk draft confirms the activated text: no-JS test at
   `acceptance/drafts/probe.spec.ts.txt:76-94`, with the disputed assertions at
   `:83` (heading inside `section#probe` on `/`), `:84` (`Count: N`), `:86`
   (`#probe-count`), `:92` (N+1 visible after the 303). Nothing was weakened,
   moved or deleted.
2. **The red at `:83` is the dispute, not a defect of this task.** Verified
   against the merged feature: `web/templates/pages/index.html:10-12` renders
   `section#probe` as the empty HTMX shell containing only
   `<a href="/probe">Probe</a>` — with JS off there is no heading, no count, no
   form, so the frozen `:83` times out exactly as the handoff reports. The
   sources genuinely conflict: T-100 oracle item 7 (`harness/tasks/T-100.md:
   116-118`) and T-103's own contract bullet (`harness/tasks/T-103.md:43-44`)
   require the section to work without JS on `/`, while T-103's deliverable
   prose (`harness/tasks/T-103.md:98-104`) withdraws the static form in favour
   of shell+link — and T-103's handoff predicted this exact failure and its
   disposition before T-105 ever ran ("T-105's activation of test #12 will
   fail as its rules anticipate (BLOCKED → fix task)",
   `harness/handoffs/T-103.md:118-119`). Per the packet's own rule 3 first
   bullet (`harness/tasks/T-105.md:36-39`), the activator must NOT edit the
   test and must write BLOCKED — which is what this diff does. The suite being
   red at e2e (28/32) is the sanctioned terminal state here, correctly
   documented as the handoff's first line, not a gate dodge.
3. **Gap (b), the Content-Type defect, is real and correctly diagnosed.**
   `probeGet` (`internal/web/probe.go:89-106`) and the HTMX branch of
   `probePost` (`internal/web/probe.go:161-169`) never set `Content-Type`;
   `renderPartial` (`internal/web/probe.go:55-66`) writes raw buffered bytes;
   the partial leads with `<h2>Probe</h2>` (`web/templates/partials/probe.html:
   1`); `<H2` is not among Go's `DetectContentType` HTML signatures, so the
   fragment is sniffed `text/plain; charset=utf-8`, and the global
   `X-Content-Type-Options: nosniff` (`internal/web/web.go:94`) makes browsers
   render it as source text. The Go acceptance tests read raw bytes over
   HTTP/1.1 (no sniffing) and pass; browser no-JS flows cannot. This violates
   T-103's own words ("non-JS visitors can reach the fragment page",
   `harness/tasks/T-103.md:100-104`) under EITHER reading of gap (a), so the
   demanded one-line fix in `internal/web/probe.go` (plus a Content-Type
   assertion in its table tests) is correctly routed to the planner as a fix
   task. No task-owned file was touched to make it — correct restraint.
4. **The regex pin (edit #1) is the sanctioned mechanical class, kept as the
   remediation instructed.** The frozen markup is pinned byte-exact by
   `harness/tasks/T-103.md:49` (`<p>Count: <span id="probe-count">N</span></p>`)
   and shipped verbatim (`web/templates/partials/probe.html:2`); after
   `Count: ` the source is `<span`, so the draft's `Count:\s*(\d+)` could never
   match any contract-compliant fragment — an unsatisfiable extractor, i.e.
   exactly "a selector that must be scoped" (`harness/tasks/T-105.md:40-45`),
   not an assertion choice. T-103's handoff pre-registered this defect and this
   disposition before T-105 ran (`harness/handoffs/T-103.md:121-141`), and both
   attempt-3 reviewers ruled "keep" (subject to planner ratification of the
   string, which the handoff requests). The replacement keeps the `(\d+)`
   capture and adds the requirement that N live in `#probe-count` — stricter,
   not weaker; the handoff lists it with justification and mirrors it in the
   holdout.
5. **`migrations_test.go` is a faithful activation.** Both hunks verified
   comment-only against the on-disk draft (`acceptance/drafts/
   migrations_go_test.go.txt:3-30` — INERT-DRAFT prose replaced with
   activation notes); `migTamperFirstHex` keeps the frozen first-token index,
   sound because `TestChecksumIsFirstAndOnlyHexTokenInDBFile`
   (`internal/store/dbfile_layout_test.go:23-62`) pins that the first and only
   64-char hex token in the DB file after a clean boot IS the recorded
   `0001_init` checksum. No A-104 edit was needed or made.
6. **Ratchet only grows, and the arithmetic is exact.** pw 20→32 =
   (3 `home.spec.ts` + 2 `home_m1.spec.ts` + 3 `probe.spec.ts`) × 4 projects
   (`playwright.config.ts:26-45`, including the 320px mobile project — rule 7's
   mobile requirement is met). go 28→51 = 61 `func Test` post-activation − 8
   acceptance-tagged − 2 smoke-tagged, matching `countGoTests`'s tagless
   `go test -list .* ./...` (`cmd/harness/guard.go:337-349`) — the pre-existing
   counting gap the handoff discloses as open issue 5c; the manifest remains
   the guard for the activated Go tests, and it now lists them.
7. **Manifest hygiene.** The pre-diff on-disk manifest matches the diff's
   minus lines exactly; the post-diff manifest lists exactly the eight expected
   acceptance files plus `contract.json`, with the drafts entries removed
   alongside the sanctioned `drafts/` deletion (deliverable 1 explicitly
   includes removing the README; the frozen bytes and contract table remain in
   git history). Unlike attempt 3, the manifest now blesses the *frozen* bytes
   under the activated path — nothing rewritten is being laundered into
   "protected" state.
8. **Handoff accuracy (remediation item 2).** The attempt-2 PASS verdicts are
   no longer cited as support — they are explicitly disclaimed ("NOT offered
   as approval of any later state"); the "byte-equivalent" overstatement is
   gone (the file now genuinely is byte-identical, with the proof stated); both
   gaps are presented for adjudication with precise file:line citations; the
   per-assertion A-101…A-105 table names the proving tests; deliverable 3's
   `test.info()` annotations survive verbatim in the pure-renamed spec
   (draft `:39`, `:61`).

## Security / architecture spot-checks (all clean)

- No app code, `go.mod`, Makefile or CI touched; writes confined to
  `acceptance/**`, `harness/protected.sha256`, `harness/.ratchet.json` (its
  regeneration is deliverable 2) and the handoff — within the packet's owns
  (`harness/tasks/T-105.md:11`) and consistent with the ownership map in
  `docs/spec/architecture.md:15-25`.
- No `template.HTML`, no SQL, no path handling, no uploads in the diff. The
  drafts' spawned-server env hardcodes exactly the sanctioned `.env.test`
  dummy values (verified byte-equal against `.env.test:6-8`) — no secret
  material.
- HTMX targets `#probe` (`web/templates/partials/probe.html:3`,
  `web/templates/pages/index.html:10`), never `#map` or ancestors
  (`docs/spec/architecture.md:27-33` rule respected; the frozen fragment
  contains no `id="map"`).
- Parse-don't-validate untouched (`parseProbePost`,
  `internal/web/probe.go:117-125`, single parse into a typed struct); typed IDs
  not applicable to the probe slice; `allow_deps: []` respected.
- No `t.Skip`/`test.skip`/`test.only`/`.fixme` anywhere in the diff; the
  disputed test is left red rather than skipped, narrowed or deleted — the
  suite is not vacuous (4 failing runs prove it bites).
- The activated tests exercise real behaviour: self-spawned real binary over
  temp SQLite (SIGTERM + respawn persistence, 403 on missing/tampered token
  with counter-unchanged, 303 `Location: /`), byte-level DB tamper for A-104,
  real browsers on 4 projects for the e2e side.

## Findings (none blocking)

1. **[Low] Trivial prose inaccuracies in the handoff.** The table says the
   migrations activation was "99% similarity"; the diff header says 91% (the
   two comment rewrites). The new `probe_test.go` header says "Exactly one
   content edit" while gofmt whitespace (edit #2) was also applied — the
   handoff itself discloses both edits, so the audit trail is complete.
2. **[Low] The ephemeral `acceptance/tmpclean` helper** (self-deleting
   `go run` workaround for a shell allowlist without `rm`) briefly created a
   file inside the protected tree. Disclosed, self-removed, never committed;
   audit noise only.
3. **[Info] Unverifiable in this sandbox:** command exit codes, `make protect`
   hash recomputation, and the holdout repo's state (claimed clean, both
   variants activated, `make holdout` PASS). Internal consistency checks all
   pass; same trust basis as the attempt-3 rounds.
4. **[Info] Deliverable 1's "green specs" is unmet (28/32)** — deliberately:
   this is the BLOCKED terminal state the packet's rule 3 prescribes for a
   feature-vs-frozen-contract dispute, with the BLOCKED reason as the
   handoff's first line. Three planner items remain open by design: (a)
   adjudicate the no-JS home shape (or sanction a re-freeze — attempt 3's
   text, preserved in history, is a starting draft), (b) cut the
   `internal/web/probe.go` Content-Type fix task (unconditional), and
   (c) ratify the regex pin's exact string. Until (a)+(b) land the frozen
   no-JS test stays red; that is correct, not a flaw of this activation.

## Verdict rationale

Attempt 3's single blocking failure — an activator unilaterally re-freezing a
disputed oracle clause and regenerating the protected manifest over it — is
fully remediated: the frozen spec is back byte-exact (proven by the pure-rename
diff and the matching manifest hash), it is left red at exactly the disputed
assertion, and the dispute plus the unconditional Content-Type bug are routed
to the planner with a BLOCKED-first-line handoff, exactly as the packet, the
attempt-3 reviews and T-103's pre-registered prediction require. The one
content edit that remains (the regex pin) is the packet's sanctioned mechanical
class, pre-registered by the feature task's own handoff before T-105 ran,
strictly tighter than the frozen text, disclosed, and held open for planner
ratification. No test was weakened, skipped or deleted; the ratchet only
grows; ownership and security bounds are respected. This is a faithful
activation with its residual red correctly converted into planner actions.

VERDICT: PASS
