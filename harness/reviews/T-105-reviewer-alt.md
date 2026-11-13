# T-105 — reviewer-alt (test integrity / reward hacking)

Scope: the attached diff (`harness/reviews/T-105.diff`), read against the frozen
drafts still present at the pre-activation tree (`acceptance/drafts/README.md`,
`acceptance/drafts/probe_go_test.go.txt`,
`acceptance/drafts/migrations_go_test.go.txt`,
`acceptance/drafts/probe.spec.ts.txt`), the task packets
(`harness/tasks/T-105.md`, `T-100.md`, `T-103.md`), the merged implementation
(`internal/web/probe.go`, `internal/web/templates.go`,
`web/templates/pages/index.html`, `web/templates/partials/probe.html`), and the
harness itself (`cmd/harness/{guard,ratchet,gate}.go`, `Makefile`,
`scripts/*`).

Verification limits: the diff is not applied to this checkout (the worktree is
still pre-activation: `acceptance/drafts/` exists and
`acceptance/go/probe_test.go` / `acceptance/e2e/probe.spec.ts` do not), and
shell execution is denied in this review session (`git`/`go`/`ls` all
permission-denied). The handoff's exit codes are therefore taken on trust;
every claim about test content, markup, hashes and harness logic was traced
statically. The holdout repo
(`/Users/ascotti/Documents/git_repos/groveland-lights-holdout`) is outside the
sandbox (`external_directory` denied), so its activation is unverifiable here.

## Findings

### 1. [Medium] A frozen Go oracle was edited beyond the drafts README's "only sanctioned edit"

`acceptance/drafts/probe_go_test.go.txt:52` pinned
`probeReCount = regexp.MustCompile(`Count:\s*(\d+)`)`; the activated
`acceptance/go/probe_test.go:57` now pins
`Count:\s*<span id="probe-count">(\d+)</span>` (plus a five-line rationale
comment at `:52-56`). The now-deleted `acceptance/drafts/README.md:35-38`
said the A-104 token index "is the only sanctioned edit", and the draft header
still says "NO content edits" (`acceptance/go/probe_test.go:6-10`). The worker
relies instead on T-105 packet rule 3 (`harness/tasks/T-105.md:36-45`), whose
second bullet permits a minimal mechanical edit ("a selector that must be
scoped"). The edit is genuinely **stricter**, not weaker: the capture is the
same digits and it additionally proves N is held by `#probe-count`; the old
regex could never match the frozen `Count: <span …>N</span>` markup
(`web/templates/partials/probe.html:2`), so it was a broken extractor rather
than a working assertion.

That said, this is a unilateral re-freeze of the oracle. The README contract
only requires `#probe-count` to hold the digits (`acceptance/drafts/README.md:50-51`);
the new regex pins one exact tag spelling, attribute order and
`</span>` adjacency, so a contract-compliant alternative markup would now
fail. The handoff does disclose the edit and asks for ratification
(`harness/handoffs/T-105.md:82-97`), which is the right paper trail, but the
planner — not the activator — should own the re-freeze. This is a procedural
finding, not a weakened assertion.

### 2. [Medium] The protected manifest was regenerated on the branch that edited the protected tests

`harness/protected.sha256:2` and `:7` carry brand-new hashes for the edited
`acceptance/go/migrations_test.go` and `acceptance/go/probe_test.go`; the old
draft hashes are removed (`harness/reviews/T-105.diff:350-361`). The manifest
is the only guard that detects modification of acceptance tests, and
`cmd/harness/guard.go:127-129,239-242` lets an `acceptance` task edit
`acceptance/**` and regenerate it. Here the worker disclosed the edit and the
change is stricter, so this is not a hidden tamper — but it is exactly the
mechanism a bad-faith worker would use to bless a weakened test. The handoff
should not present the manifest as an independent check.

This is compounded by two pre-existing harness holes: `countGoTests`
(`cmd/harness/guard.go:337-349`) runs without `-tags acceptance`, so the four
activated A-103/A-104 Go tests are not ratchet-counted (the `go:51` baseline
is the untagged count), and the new-skip-marker scan skips `acceptance/`
(`cmd/harness/guard.go:169-170`). A later acceptance task could therefore
delete or weaken these tests, regenerate the manifest, and the ratchet would
not see it. The handoff flags the ratchet half at
`harness/handoffs/T-105.md:199-203`; planner/harness follow-up is warranted.
Not a T-105-code violation, but it is why the manifest regeneration matters.

### 3. [Medium] The independent holdout oracle was edited identically, but the holdout diff is not attached

The handoff says the holdout `drafts/probe_sessions_go_test.go.txt` was
activated with "the same one sanctioned regex edit + gofmt"
(`harness/handoffs/T-105.md:65-72`) and quotes the new
`hsReCount = regexp.MustCompile(`Count:\s*<span id="probe-count">(\d+)</span>`)`
with the old expression and rationale in a comment (`:98-104`). The holdout is
the independent oracle and is not covered by `harness/protected.sha256`; its
diff, hashes and the line range are not in the reviewed diff, so I cannot
verify that only the regex changed and no assertion was dropped or narrowed.
Editing both the main and holdout oracles with the same implementation-shaped
pattern also reduces the holdout's independence. Attach the holdout diff or
before/after hashes before relying on `make holdout` as independent evidence.

### 4. [Low] Activated tests retain stale "INERT DRAFT / no content edits" headers

`acceptance/e2e/probe.spec.ts:4-10` still says "T-105 activates it verbatim,
with a rename only and NO content edits"; `acceptance/go/probe_test.go:6-10`
says the same while the file now contains the regex edit, the five-line
comment and a gofmt line; `acceptance/go/migrations_test.go:6-17` still says
"NO content edits … exactly ONE sanctioned exception" after its gofmt change.
The handoff acknowledges the cosmetic debt (`harness/handoffs/T-105.md:118-121`),
so this is an audit-trail inconsistency, not a hidden change. A future reader
of the test file alone would be misled; the planner should reconcile the
headers when the new regex is ratified (or the fix task touches the file).

### 5. [Low] The handoff pre-asserts reviewer PASS and cites findings as already adjudicated

`harness/handoffs/T-105.md:28-36` says "both reviewer reports are **VERDICT:
PASS**", and `:51-53` says both reviews "verify the gap is real… Verdicts:
PASS". Those report files do exist (`harness/reviews/T-105-reviewer.md`,
`harness/reviews/T-105-reviewer-alt.md`), so this is not fabricated, but a
handoff asserting its own review outcome is not evidence and should not
influence a reviewer. This review was re-derived independently; the claim on
its own is a process smell, not a test-integrity failure.

### 6. [Info] The BLOCKED gap is real and correctly routed; the branch is red and not mergeable

The no-JS test (`acceptance/e2e/probe.spec.ts:76-94`, activated verbatim from
`acceptance/drafts/probe.spec.ts.txt:76-94`) requires `GET /` to server-render
a populated `section#probe` (heading `Probe`, `Count: N` in `#probe-count`, a
plain form with a live CSRF token). The merged home is the empty shell plus
`<a href="/probe">Probe</a>` (`web/templates/pages/index.html:10-12`), so the
test fails exactly at `probe.spec.ts:83`. T-100's packet required this clause
(`harness/tasks/T-100.md:116-118`), the drafts README repeated it
(`acceptance/drafts/README.md:48,61-69`), and T-103's own contract line agrees
(`harness/tasks/T-103.md:41-45`), while T-103's deliverable prose ordered the
shell + link (`harness/tasks/T-103.md:98-104`). The worker correctly did not
edit the test (T-105 packet rule 3 first bullet, `harness/tasks/T-105.md:36-39`)
and wrote `BLOCKED:`.

The fix sketch at `harness/handoffs/T-105.md:171-186` is honest about the
template-engine catch: `{{template "partials/probe.html" .}}` will not work
as-is because `internal/web/templates.go:44-60` parses partials into a
separate map while pages are parsed with only `base.html` + the page file.
Once the owner task lands, the four red runs should go green with no further
test edits. The gate will fail `make check` (`cmd/harness/gate.go:99-118`)
until then; that is the sanctioned BLOCKED → fix-task flow, but it means T-105
deliverable 1 ("activated, green specs", `harness/tasks/T-105.md:66-69`) is
not met and the branch must not merge. This review is a PASS only on test
integrity, not on merge readiness.

### 7. [Info] `make check` does not run the holdout suite; the gate does

`Makefile:135-136` has no holdout prerequisite; `cmd/harness/gate.go:114`
runs `make holdout` separately. The handoff's `make holdout | 0` row
(`harness/handoffs/T-105.md:157`) is fine, but the packet prose should not
imply `make check` covers the activated holdout variants.

## Checked and not found

- **No skipped, deleted or removed test.** No `t.Skip`, `test.skip`,
  `test.only` or `.fixme` in the diff; the no-JS test is left red rather than
  removed. The only test-content change is the stricter Go regex above.
- **No weakened assertion.** The e2e spec is a true 100% rename — git
  similarity 100% (`harness/reviews/T-105.diff:70-73`) and the manifest hash
  `4ec1052d…` is carried unchanged from the draft path to the new path
  (`harness/protected.sha256:4`). The `migrations_test.go` change is one
  gofmt comment re-indent (99% similarity, `T-105.diff:74-89`); no assertion
  changed. The A-104 first-token assumption is pinned by
  `internal/store/dbfile_layout_test.go:23-63` (first token == recorded
  checksum, exactly one token), so no index pin was needed.
- **No fixture special-casing in non-test code.** The diff touches no
  `cmd/server`, `internal/`, `web/` or `migrations/` file; the activated Go
  tests use real spawns and real HTTP, not mocks.
- **No coverage theatre.** The activated Go tests spawn the real binary on a
  temp SQLite file and drive real HTTP; the e2e tests drive a real browser.
  Four e2e runs fail, so the suite is not vacuous.
- **No dead code, new dependency or protected-path escape.** `allow_deps: []`;
  `go.mod`/`go.sum` are absent from the diff; the only writes are within
  `acceptance/**`, `harness/protected.sha256`, `harness/.ratchet.json` and the
  handoff. The ratchet numbers are plausible: `go:28→51` equals the 51
  untagged test functions (57 total − 4 acceptance-tagged
  `acceptance/go/{m1_guards,healthz}_test.go` − 2 smoke-tagged
  `cmd/server/boot_test.go`), and `pw:20→32` equals +12 project-runs of
  probe.spec.ts's three tests (`acceptance/e2e/probe.spec.ts:21,64,76`).

## Verdict rationale

The activation itself is honest: the e2e spec is byte-identical, the A-104
tests are unchanged, and the one Go test change fixes an extraction regex
that could never match the frozen markup while making the assertion stricter
rather than looser. The red no-JS test is a real feature gap that the worker
escalated with `BLOCKED:` instead of editing the test to fit the current
shell + link behaviour. The remaining issues are auditability and harness
mechanics — the README's contradicting "only sanctioned edit", the
implementation-shaped re-freeze of the regex, the manifest self-blessing and
the acceptance-tagged ratchet blind spot, the unattached holdout diff, and the
stale "no edits" headers. None is a weakened, skipped or deleted test, a
fixture special-case in production code, or a mock bypass. The branch is still
blocked on the gap-(a) feature fix and cannot merge red.

VERDICT: PASS
