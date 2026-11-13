# T-105 review (attempt 3) — Activate the M1 acceptance suite (A-103/A-104)

Review basis: the attached diff (`harness/reviews/T-105.diff`, identical to the diff
in the task prompt), read against the operative packet (`harness/tasks/T-105.md`),
the frozen oracles it activates (`acceptance/drafts/*` — still present in this
checkout, so every draft byte was compared directly), the feature contract
(`harness/tasks/T-100.md`, `harness/tasks/T-103.md`), T-103's handoff with its
pre-registered gaps (`harness/handoffs/T-103.md`), the merged implementation
(`internal/web/probe.go`, `internal/web/web.go`, `web/templates/**`,
`internal/store/dbfile_layout_test.go`), and the harness state
(`harness/state.md`, `harness/.ratchet.json`, `harness/protected.sha256`).

Verification limits: the diff is not applied to this checkout (confirmed:
`acceptance/drafts/` present, activated files absent) and shell execution is
denied (`go`, `npx`, `shasum`, git log all rejected), so the handoff's exit codes
and the new manifest hashes are taken on trust; every content, markup,
sniffing-behaviour and harness-logic claim was traced statically. The holdout
repo is outside the sandbox — its activation is unverifiable here.

## What is verified sound (and worth keeping)

1. **The BLOCKED Content-Type diagnosis is technically correct and is a real
   feature bug.** `probeGet` writes the fragment without ever setting
   `Content-Type` (`internal/web/probe.go:89-106`; `renderPartial` at
   `probe.go:55-66` writes raw bytes), the global
   `X-Content-Type-Options: nosniff` is set on every response
   (`internal/web/web.go:94`), and the partial leads with `<h2>Probe</h2>`
   (`web/templates/partials/probe.html:1`). Go's `DetectContentType` HTML
   signature set recognises only `<!DOCTYPE HTML`, `<HTML`, `<HEAD`, `<SCRIPT`,
   `<IFRAME`, `<H1`, `<DIV`, `<FONT`, `<TABLE`, `<A`, `<STYLE`, `<TITLE`, `<B`,
   `<BODY`, `<BR`, `<P`, `<!--` — `<H2` is **not** among them — so the response
   is sniffed `text/plain; charset=utf-8` and nosniff makes every browser render
   the fragment as source text. HTMX swaps `responseText` as HTML regardless of
   Content-Type, which is exactly why all JS-on runs pass and only the no-JS
   runs are red — internally consistent with the handoff's 28/32. The demanded
   fix (set `Content-Type: text/html; charset=utf-8` in `probeGet` and the
   HTMX branch of `probePost`, or once in `renderPartial`) is correct,
   one-line-class, and contradicts no packet. A fix task owning
   `internal/web/probe.go` should be cut **regardless of the verdict below** —
   under either reading of the no-JS contract, T-103's own words ("the no-JS
   flow reaches the same form via a plain link … so non-JS visitors can reach
   the fragment page", `harness/tasks/T-103.md:100-104`) are violated by an
   unrenderable document.
2. **Edit #1 (the Go regex pin) is the sanctioned mechanical class, pre-registered
   and strictly tighter.** `probeReCount` `Count:\s*(\d+)` →
   `Count:\s*<span id="probe-count">(\d+)</span>` (diff hunk `@@ -49,7 +49,12 @@`,
   activated `acceptance/go/probe_test.go:52-57`). The frozen markup pins the
   digits inside the span (`harness/tasks/T-103.md:49`;
   `web/templates/partials/probe.html:2` renders exactly that), so after
   `Count: ` the raw HTML is `<span` and the old regex could never match any
   contract-compliant fragment — an unsatisfiable extractor, not an assertion
   choice. The replacement keeps the `(\d+)` capture and adds the requirement
   that N live in `#probe-count`. This defect class was pre-registered by
   T-103's handoff gap (b) (`harness/handoffs/T-103.md:121-141`) before T-105
   ran, and fits `harness/tasks/T-105.md:40-45` (mechanical defect, minimal
   edit, listed with justification — done). Keep, subject to planner
   ratification of the exact string.
3. **`migrations_test.go` is a faithful activation.** Single hunk
   (`@@ -6,7 +6,7 @@`) is gofmt comment re-indentation only; no A-104 token-index
   pin was needed or made — sound, because
   `internal/store/dbfile_layout_test.go:23-62` pins that the recorded checksum
   is the first and only 64-char hex token in the DB file after a clean boot.
4. **No weakening-to-green, no skips.** Grep across `acceptance/` finds no
   `t.Skip`/`test.skip`/`test.only`/`.fixme`; the two JS-on e2e tests and both
   A-104 Go tests are pure renames; the suite is left red rather than narrowed
   to pass. Ratchet only grows: pw 20→32 is exactly (3 `home.spec.ts` + 2
   `home_m1.spec.ts` + 3 `probe.spec.ts`) × 4 projects (verified by grep);
   go 28→51 is a refresh of the stale T-100-era baseline with T-101..T-104 unit
   growth (59 `func Test*` repo-wide; acceptance-tagged and smoke-tagged files
   excluded from the count — the pre-existing `countGoTests` gap the handoff
   discloses as open issue 5c).
5. **Security/arch spot-checks clean.** The diff touches no app code: no
   template.HTML, no SQL, no path handling, no uploads, no secret material (the
   spawn tests hardcode only the sanctioned `.env.test` dummy base64 values,
   draft `probe_go_test.go.txt:291-293`). HTMX targets `#probe`, never `#map`.
   Parse-once-at-the-boundary into a typed struct is untouched
   (`parseProbePost`, `internal/web/probe.go:108-125`). No new dependencies
   (`allow_deps: []` respected). Ownership: writes are confined to
   `acceptance/**`, `harness/protected.sha256`, `harness/.ratchet.json`, and the
   handoff — all within the packet's owns.

## Findings

1. **[Blocker] The frozen no-JS oracle was rewritten to fit the implementation —
   a contract re-specification, not the sanctioned mechanical class.**
   `acceptance/e2e/probe.spec.ts` activates at git similarity **59%** (diff
   header). The frozen draft (`acceptance/drafts/probe.spec.ts.txt:76-94`)
   requires, with JS off, the *populated* section on `/` (heading at `:83`,
   `Count: N` at `:84`, `#probe-count` at `:86`) and N+1 visible on `/`
   immediately after the 303 (`:92`). The rewrite (diff hunk
   `@@ -73,23 +80,45 @@`, activated lines ≈84-124): on `/` it checks only that
   a `Probe` link with `href="/probe"` exists (≈:100-102), moves the
   heading/count assertions onto the `/probe` page behind a click (≈:106-111),
   and after the 303 asserts only `section#probe` visible (≈:116-117) before
   reading N+1 via a second navigation (≈:122-124). That deletes real
   assertions ("home page itself works without JS"; "the incremented count is
   visible where the user lands") and converts the frozen requirement into the
   implementation's shell+link shape. The packet draws exactly the line this
   crosses:
   - activate "by rename, not rewrite" (`harness/tasks/T-105.md:22-26`);
   - if the FEATURE violates the frozen contract, "do NOT edit the test.
     Write `BLOCKED`" (`:36-39`);
   - only "a mechanical defect that does not change what it asserts" may be
     minimally edited; "anything that weakens an assertion (looser match,
     removed check …) is forbidden" (`:40-45`);
   - "Never … specs' assertions … If the contract itself seems wrong,
     `BLOCKED: <reason>` and stop" (`:82-84`);
   - "feature-vs-test disputes → BLOCKED to planner" (`harness/state.md:33-34`).

   The draft's regex defect (edit #1) is mechanical because it is unsatisfiable
   against *any* contract-compliant markup. The no-JS clause is different in
   kind: the draft fails only because the merged feature chose a different
   shape — which is precisely a feature-vs-frozen-contract *dispute*, and the
   sources genuinely conflict (T-100 oracle item 7, `harness/tasks/T-100.md:
   116-118`; the deleted drafts README's "server-rendered (with a plain form)
   when JS is off"; even T-103's own contract bullet "The section must also
   work without JS" at `harness/tasks/T-103.md:43-44` — versus T-103's
   deliverable prose at `:98-104` withdrawing the static form). Resolving that
   conflict is the planner's, not the activator's. T-103's handoff already
   prescribed the correct disposition: "T-105's activation of test #12 will
   fail as its rules anticipate (BLOCKED → fix task)"
   (`harness/handoffs/T-103.md:118-119`), and attempts 1-2 followed it. The
   worker's transparency (full edit list, ratification request, branch left
   red) mitigates intent, and the rewritten test is arguably *more* faithful to
   T-103's deliverable text — but the oracle-first mechanism this whole task
   exists to protect ("activated verbatim … were not edited after",
   `harness/state.md:36-47`) cannot survive an activator unilaterally
   re-freezing a disputed clause, however well argued. If this is PASSable,
   any frozen assertion the implementer believes superseded is editable.

2. **[High] The protected manifest was regenerated to bless the rewritten test
   before adjudication.** `harness/protected.sha256` (diff) replaces
   `4ec1052d… acceptance/drafts/probe.spec.ts.txt` with
   `20a8c976… acceptance/e2e/probe.spec.ts` — the rewritten bytes are now in
   the protected set. `make protect` is a required deliverable and acceptance
   tasks own the manifest, so this is procedural, not an ownership breach — but
   the manifest is the only guard over acceptance-test bytes, and here it
   launders a contract-changing rewrite into "protected" state. Compounding
   (pre-existing, disclosed as open issue 5c): `countGoTests` skips
   acceptance-tagged Go tests, so the four activated Go tests are not
   ratchet-counted either. If finding 1 is remediated by revert, the manifest
   naturally reverts with it.

3. **[Medium] The handoff overstates what the rewrite preserves and cites stale
   approvals as support.** "KEEPS every capability assertion, byte-equivalent
   where the locators allow" is not accurate: the `/`-page heading/count
   assertions and the immediate post-303 N+1-on-`/` check are gone (finding 1);
   the next bullet concedes the observation point moved. And the handoff cites
   "two reviewer PASS verdicts … on test integrity" for the BLOCKED routing —
   those verdicts reviewed the *attempt-1/2 pure-rename* artifact and their PASS
   rationale explicitly rested on the test standing **unedited** (on-disk
   `harness/reviews/T-105-reviewer.md:145-154`: "the tests stand unedited").
   They are not support for this diff; the same-round alt review of this exact
   diff independently reaches BLOCK.

4. **[Low] Stale "INERT DRAFT / no content edits" headers** remain in the two
   activated Go files (the spec's header was reconciled only because it was
   edited anyway). Disclosed as open issue 5d; audit-trail noise. The holdout
   regex mirror (`hsReCount`) repeats edit #1's implementation-shaped string,
   reducing holdout independence — unverifiable here; gate `make holdout` is
   the compensating control.

5. **[Info] Process observation for the planner:** attempts 1-2 ended BLOCKED
   with the tests unedited — the documented terminal state — and the retry loop
   bounced the task back with only "gate: make check failed … Fix it and
   finish" (`harness/tasks/T-105.md:87-95`), never routing the dispute to the
   planner. That structural pressure is real and not the worker's fault, but it
   does not authorise the oracle rewrite; the fix is a harness/planner-side
   adjudication step, noted here so the next attempt is not squeezed the same
   way.

## Required remediation (converts this to PASS)

1. Revert `acceptance/e2e/probe.spec.ts` to the frozen draft text (pure rename,
   similarity 100%); it stays red at `:83` — that red is the dispute, correctly
   routed. Re-run `make protect` so the manifest again carries the draft hash
   under the activated path (as attempt 1 did).
2. Keep edits #1/#2 (regex pin + gofmt) and the handoff's BLOCKED first line,
   but present **both** gaps for adjudication: (a) the no-JS home-shape dispute
   (T-100 item 7 / README / T-103:43-44 vs T-103:98-104) and (b) the
   Content-Type defect, which is unconditional. Drop the stale-verdict
   citation; correct the "byte-equivalent" claim.
3. Planner actions: cut the fix task owning `internal/web/probe.go`
   (Content-Type line, plus a fragment Content-Type assertion in its table
   tests); then either ratify the shell+link no-JS shape (at which point the
   test edit may be re-applied as a planner-sanctioned re-freeze — the
   attempt-3 text is a good draft for it) or restore the server-rendered
   populated section per T-103's handoff sketch (`harness/handoffs/
   T-103.md:110-117`), noting the `templates.go` parse caveat. Keep the
   `<a href="/probe">Probe</a>` link either way.

## Verdict rationale

Everything except one file is faithful, disclosed, and in one case (the
Content-Type diagnosis) genuinely excellent work the milestone needs. But the
core deliverable of T-105 is activation of a frozen oracle "verbatim", with a
narrow, enumerated escape hatch; attempt 3 used that hatch to re-specify a
disputed acceptance clause to the merged implementation's shape and then
regenerated the protected manifest over it. The branch being red elsewhere does
not undo that: the oracle for assertion #12 as frozen no longer exists in the
suite, and whether it should is the planner's call. BLOCK with the cheap,
precise remediation above.

VERDICT: BLOCK
