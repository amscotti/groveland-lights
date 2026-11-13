# T-105 — reviewer-alt (test integrity / reward hacking)

Review basis: the attached diff (`harness/reviews/T-105.diff`), the pre-activation
drafts still present in this checkout (`acceptance/drafts/*`), the task packets
(`harness/tasks/T-105.md`, `T-100.md`, `T-103.md`), the merged implementation
(`internal/web/probe.go`, `internal/web/web.go`, `web/templates/**`), and the
harness (`cmd/harness/{guard,ratchet}.go`). The diff is not applied in this
checkout and shell execution is denied, so the handoff's exit codes are taken on
trust; the findings below are traced statically from the diff and the current
tree. The holdout repo is outside the sandbox, so its activation is not
independently verified.

## Findings

### 1. [Blocker] The frozen no-JS acceptance test was rewritten to fit the implementation, removing assertions

`acceptance/e2e/probe.spec.ts` is not the required pure `git mv`: the diff header
says **similarity index 59%** (`harness/reviews/T-105.diff:70-74`), and the no-JS
hunk (`harness/reviews/T-105.diff:105-156`) replaces the frozen flow with a
different one.

The frozen draft required a no-JS visitor to see the populated probe section on
`/` — heading `Probe`, `Count: N`, `#probe-count` — and, after submitting the
form and taking the 303, to see N+1 on `/` (`acceptance/drafts/probe.spec.ts.txt:83-84,92`).
The activated test deletes those `/`-page assertions and substitutes only a link
check:

- `acceptance/e2e/probe.spec.ts:100-102` — on `/`, the test now checks only that
  a `Probe` link exists with `href="/probe"`.
- `acceptance/e2e/probe.spec.ts:106-111` — the heading/count/`#probe-count`
  assertions are moved to `/probe` after a click, not on `/`.
- `acceptance/e2e/probe.spec.ts:116-122` — immediately after the 303 the test
  asserts only `section#probe` is visible; the N+1 assertion is deferred until
  after a **second** navigation.

That is a narrowed and partly weakened acceptance contract, not a mechanical
selector fix. It converts the frozen requirement "home page works without JS"
into "home page contains a link to a fragment page", which is precisely the
implementation's current shell+link shape. The handoff admits the rewrite is a
re-freeze and requests ratification after the fact
(`harness/reviews/T-105.diff:326-343,357-360`), but T-105's rules require the
opposite order:

- Activate by rename, not rewrite: `harness/tasks/T-105.md:22-26` ("by rename,
  not rewrite — so the oracle-first property is auditable").
- If the feature violates the frozen contract, **do not edit the test**; write
  `BLOCKED` and let the planner decide: `harness/tasks/T-105.md:36-39`.
- Only a mechanical defect that "does not change what it asserts" may be edited;
  weakening an assertion is forbidden: `harness/tasks/T-105.md:40-45`.
- "Never … specs' assertions": `harness/tasks/T-105.md:82-84`.
- The deleted drafts README said pure `git mv` with no content edits, the A-104
  token index being the only exception: `acceptance/drafts/README.md:27-38`.

The underlying contract conflict is real — T-103's prose says the no-JS flow
reaches the form via a plain link (`harness/tasks/T-103.md:98-104`), while T-103
also says the drafts "must pass against your work with zero edits"
(`harness/tasks/T-103.md:26`) and T-100's item 7 requires the section on `/`
(`harness/tasks/T-100.md:116-118`). That conflict is for the planner, not for the
activator to resolve by editing the oracle. Revert `acceptance/e2e/probe.spec.ts`
to the frozen draft (leave it red) and BLOCK.

The edit also changes the failure surface: the unedited test fails at the `/`
heading assertion (draft `probe.spec.ts.txt:83`), whereas the rewritten test
fails later at `probe.spec.ts:107` for the Content-Type gap. The branch is still
red, but the original A-103 gap has been narrowed out of the suite.

### 2. [High] The protected manifest was regenerated on the same branch to bless the rewritten test

`harness/protected.sha256:2-3,7` replaces the draft hashes with new hashes,
including `20a8c976…` for the rewritten `acceptance/e2e/probe.spec.ts`, and
removes the old `4ec1052d…` draft hash (`harness/reviews/T-105.diff:451-467`).
The manifest is the only guard that detects modification of acceptance tests, and
`cmd/harness/guard.go:127-129,239-242` explicitly lets an acceptance task edit
`acceptance/**` and regenerate it. T-105 owns the manifest and `make protect` is
required, so this is not an ownership violation by itself — but it is exactly the
mechanism that would hide a weakened test, and here it hides the no-JS rewrite
from any post-hoc protected-path check. The handoff should not present the
manifest update as independent evidence of faithful activation.

Compounding blind spot: `countGoTests` runs without `-tags acceptance`
(`cmd/harness/guard.go:337-349`), so the four activated A-103/A-104 Go tests are
not ratchet-counted. The go baseline `28→51` (`harness/.ratchet.json`) is
therefore unrelated to this task's new acceptance tests; it is a refresh of
feature-task unit-test growth. The ratchet itself is not weakened —
`RatchetCompare` fails only on decreases (`cmd/harness/ratchet.go:89-98`).

### 3. [Medium] The Go oracle regex was re-frozen unilaterally and mirrored into the holdout

`acceptance/go/probe_test.go:52-56` changes `probeReCount` from
``Count:\s*(\d+)`` to ``Count:\s*<span id="probe-count">(\d+)</span>``. The new
regex is genuinely **stricter**, not weaker: the old expression could never match
the frozen `Count: <span …>N</span>` markup (`web/templates/partials/probe.html:2`),
so this is likely a real extractor defect, and the capture still parses the
count. The handoff lists it and requests ratification
(`harness/reviews/T-105.diff:313-322`).

It is still an unauthorized re-freeze of oracle text: the deleted README said the
A-104 token index was the only sanctioned edit
(`acceptance/drafts/README.md:35-38`), and pinning one exact tag spelling,
attribute order and `</span>` adjacency makes the test fail on contract-compliant
markup variations. The same edit in the holdout (`hsReCount`) also reduces the
holdout's independence. This should be ratified or reverted by the planner; it is
not the primary reason for BLOCK, but it should not be merged silently with the
no-JS rewrite.

### 4. [Medium] The handoff overstates what the rewritten test preserves and cites stale approvals

The handoff says the edit "KEEPS every capability assertion, byte-equivalent
where the locators allow" and "MOVES exactly one observation point"
(`harness/reviews/T-105.diff:332-340`). That is not accurate: the `/`-page
heading/count assertions and the immediate post-303 N+1 assertion are gone
(`harness/reviews/T-105.diff:128-129,147-149`), and the test now performs an
extra navigation before reading N+1. The handoff's own edit list concedes the
observation point moved (`harness/reviews/T-105.diff:339-343`), so the
"byte-equivalent" claim should not be relied on.

The handoff also cites "two reviewer PASS verdicts … on test integrity"
(`harness/reviews/T-105.diff:410-414`), but those reviews explicitly reviewed a
pure-rename `probe.spec.ts` at 100% similarity and did not see the 59%,
contract-changing rewrite in this diff. They are stale approvals for a different
artifact; using them as support for the current diff is misleading.

### 5. [Low] The new header self-certifies the edit and the in-tree contract table is deleted

`acceptance/e2e/probe.spec.ts:4-17` states that activation "made exactly ONE
content edit" and explains the T-103 justification. That is transparent, but it
is the worker's own certification of an edit the task did not authorize. The
deleted `acceptance/drafts/README.md` also removed the frozen contract table and
visual notes (`harness/reviews/T-105.diff:1-69`). Removing `acceptance/drafts/`
once empty is allowed by deliverable 1, and git history retains the text, so this
is an auditability cost rather than a separate violation.

## Checked and not found

- **No skip/delete/only/fixme:** no `t.Skip`, `test.skip`, `test.only` or
  `.fixme` in the diff; the no-JS test is left red rather than skipped or
  removed.
- **No fixture special-casing in non-test code:** the diff touches no
  `cmd/server`, `internal/`, `web/` or `migrations/` file; no acceptance string
  literal appears in production code.
- **No mocks or coverage theatre:** the activated Go tests spawn the real binary
  over a temp SQLite DB and drive real HTTP; the e2e tests drive a real browser.
  Four e2e runs fail, so the suite is not vacuous.
- **No dead code / new dependency:** `allow_deps: []`; `go.mod`/`go.sum` are
  absent from the diff; the only writes are inside `acceptance/**`,
  `harness/protected.sha256`, `harness/.ratchet.json` and the handoff.
- **The Content-Type gap is real, but it is a feature bug, not a test bug:**
  `probeGet` writes the fragment without setting a Content-Type
  (`internal/web/probe.go:89-106`), and the global
  `X-Content-Type-Options: nosniff` (`internal/web/web.go:94`) makes a
  `text/plain`-sniffed fragment inert. The correct disposition was `BLOCKED`
  without editing the test, not rewriting the no-JS contract to avoid the
  home-page failure.

## Verdict rationale

T-105's core requirement is activation by rename, not rewrite. The 59% rewrite
of `acceptance/e2e/probe.spec.ts` changes the frozen no-JS contract, removes the
`/`-page assertions and the immediate post-303 count assertion, and narrows the
acceptance surface to the implementation's shell+link shape. The protected
manifest was then regenerated to bless the change, and the handoff cites stale
PASS reviews from a different diff. The after-the-fact ratification request does
not restore the oracle-first property. The branch is also red, so deliverable 1
("activated, green specs", `harness/tasks/T-105.md:68-69`) is unmet regardless.
This is a test-integrity failure and warrants BLOCK, not PASS.

VERDICT: BLOCK
