# T-105 — reviewer-alt (test integrity / reward hacking)

Scope: the attached diff (`harness/reviews/T-105.diff`, byte-consistent with
the attachment), read against `harness/tasks/T-105.md`, the frozen drafts
(`acceptance/drafts/README.md`, `acceptance/drafts/probe_go_test.go.txt`,
`acceptance/drafts/migrations_go_test.go.txt`,
`acceptance/drafts/probe.spec.ts.txt`), `harness/tasks/T-103.md`, the current
`web/templates/**`, `internal/web/templates.go`, `internal/web/probe_test.go`,
`internal/store/dbfile_layout_test.go`, the Makefile and
`cmd/harness/{guard,ratchet,gate,status}.go`.

Verification limit: the diff is not applied to this checkout (the worktree is
still at the pre-activation tree: `acceptance/drafts/` exists and
`acceptance/go/probe_test.go` / `acceptance/e2e/probe.spec.ts` do not), and
shell execution is denied in this review session (`git`/`go`/`ls` all
permission-denied). The handoff's exit codes are therefore taken on trust;
every claim about the test content, the markup and the harness logic was
traced statically against the sources.

## Findings

### 1. [Medium] A frozen acceptance test was edited beyond the drafts README's "only sanctioned edit"

`acceptance/go/probe_test.go:52-57` (the activation hunk; the new regex is
line 57) replaces `Count:\s*(\d+)` with
`Count:\s*<span id="probe-count">(\d+)</span>`. The now-deleted
`acceptance/drafts/README.md:29-32` explicitly said the A-104 token index is
"the only sanctioned edit", and the draft header
(`acceptance/drafts/probe_go_test.go.txt:7-10`) says "NO content edits".
The handoff relies instead on T-105 packet rule 3
(`harness/handoffs/T-105.md:51-60`) and T-103's gap (b), and the change is
genuinely **stricter**, not looser: it still captures the same digits and now
also requires the `#probe-count` span (the original regex could never match
`<p>Count: <span id="probe-count">N</span></p>`, so it was a broken extractor,
not a working assertion). This is not a weakening and not reward hacking, but
the handoff should quote the README's contradictory "only sanctioned edit"
line and ask the planner to ratify/re-freeze the string. The exact
`<span id="probe-count">` spelling is also implementation-shaped; the README
contract only pins "#probe-count text is the digits", so a slightly less
attribute-order-sensitive pattern (or an explicit re-freeze) would be safer.

### 2. [Medium] The holdout oracle was edited, but the edit is not in the reviewed diff

`harness/handoffs/T-105.md:62-63` says the holdout `hsReCount` got "the same
one-line edit", and `harness/handoffs/T-105.md:35-41` says the two holdout
drafts were renamed there. The holdout is the independent oracle and is not
covered by `harness/protected.sha256`; its diff/hashes are not attached here,
so this review cannot verify that only the regex changed (and not an
assertion). The handoff should include the exact old/new holdout line and the
before/after hashes, or the scheduler should attach the holdout diff.

### 3. [Medium] The Go ratchet still does not count acceptance-tagged tests

`.ratchet.json` goes `go: 28 -> 51`, but `countGoTests`
(`cmd/harness/guard.go:337-349`) runs `go test -list .* ./...` **without**
`-tags acceptance`. The 51 is the untagged unit count (I count 51 test
functions outside `acceptance/go/*` and the `smoke`-tagged
`cmd/server/boot_test.go`), so the four activated tests
(`TestA103ProbeRequiresCSRF`, `TestA103ProbeIncrementsAndPersists`,
`TestA104AppliesMigrations`, `TestA104RefusesTamperedChecksum`) are **not**
ratcheted. `cmd/harness/guard.go:127-129,239-242` lets an acceptance task
edit/delete `acceptance/**` and regenerate the manifest, so the manifest is
not an independent guard against a later acceptance task deleting these tests.
This is pre-existing (A-101/A-105 have the same hole) and `cmd/harness/**` is
outside T-105's owns, but the handoff's "count only grew, as required"
(`harness/handoffs/T-105.md:115`) must not be read as the new Go tests being
ratchet-protected. Planner/harness follow-up recommended.

### 4. [Medium] The BLOCKED gap is real, but the suggested fix cannot work as written

The no-JS e2e test (`acceptance/e2e/probe.spec.ts:73-94`) requires the home
page to server-render a populated `section#probe`; the current
`web/templates/pages/index.html:10-12` is only the empty shell plus
`<a href="/probe">Probe</a>`, and the frozen README requires the populated
form (`acceptance/drafts/README.md:42,61-63`). The worker correctly did **not**
edit the test, and the red result is a genuine feature gap, not a flaky test.
However, the fix sketch at `harness/handoffs/T-105.md:123-131` proposes
`{{template "partials/probe.html" .}}` in `index.html`; that will fail at
execution because `internal/web/templates.go:44-60` parses partials into a
separate `t.partials` map and pages are parsed with only `base.html` + the
page file (`:49-53`), so the page template set does not contain the partial.
The planner's fix task must also change `templates.go` (or inline the
partial markup), otherwise it still cannot satisfy the clause.

### 5. [Low] Stale "no edits" headers in the activated tests

`acceptance/go/probe_test.go:7-10` still claims the file is activated "with a
rename only and NO content edits", while it now contains the regex edit at
`:52-57`. `acceptance/go/migrations_test.go:11-17` still says "NO content
edits ... exactly ONE sanctioned exception" while the file was gofmt'd
(comment indent only). Both edits are disclosed in the handoff, so this is an
audit-trail inconsistency, not a hidden change; the headers should note the
activation deltas.

### 6. [Low] "Everything else is activated, green, and committed" conflicts with the handoff's own red `make check`

`harness/handoffs/T-105.md:22` says this, but the command table records
`make check | 2` (`:116`) and the A-103 row is explicitly **SPLIT / RED**
(`:86`). The branch is intentionally red pending gap (a); the summary should
say so at the top so a skimming reader does not infer a green suite.

### 7. [Low] `make check` does not run the holdout suite; the gate does

The task packet calls the full suite "`make check` ... + holdout"
(`harness/tasks/T-105.md:61-64`), but `Makefile:135-136` has no holdout
prerequisite; `cmd/harness/gate.go:99-118` runs `make holdout` separately.
The handoff's `make holdout | 0` row (`harness/handoffs/T-105.md:113`) is
fine; the packet/prose should not imply `make check` covers the activated
holdout variants.

### 8. [Info] A-104's first-token assumption is actually pinned

The handoff's claim (`harness/handoffs/T-105.md:70-75`) that the first
64-char hex token is the recorded checksum is supported by
`internal/store/dbfile_layout_test.go:11-63` (first == recorded and exactly
one token), so the absence of an A-104 index pin is justified.

### 9. [Info] The e2e spec is a true 100% rename

`harness/reviews/T-105.diff:70-73` shows `probe.spec.ts` at similarity 100%,
and `harness/protected.sha256:4` carries the old draft hash
`4ec1052d...` unchanged to the new path. The no-JS test was not edited,
skipped or narrowed.

## Checked and not found

- **No skipped or deleted test.** No `t.Skip`, `test.skip`, `test.only` or
  `.fixme` in the diff; the only test-content change is the stricter regex
  above. The no-JS test is left red rather than removed.
- **No weakened assertion.** The regex change is a tightening; the `(\d+)`
  capture, the `#probe-count` digits requirement and every other assertion in
  `probe_test.go` / `migrations_test.go` are unchanged. The e2e spec is
  byte-identical.
- **No fixture special-casing in non-test code.** The diff touches no
  `cmd/server`, `internal/` or `web/` file; the only non-acceptance writes are
  `harness/protected.sha256`, `harness/.ratchet.json` and the handoff.
- **No mock bypass.** The activated Go tests spawn the real `bin/server` on a
  free port with a real temp-file SQLite DB and drive real HTTP
  (`acceptance/drafts/probe_go_test.go.txt:167-237,270-306`); the e2e tests
  drive the real browser.
- **No coverage theatre.** The activated tests can fail (four do), and the
  assertions are concrete counts, statuses and `Location: /`.
- **No dead code / new dependency / protected-path escape.** `allow_deps: []`;
  `go.mod`/`go.sum` are absent from the diff; the changes stay within
  `acceptance/**`, `harness/protected.sha256`, `harness/.ratchet.json` and
  `harness/handoffs/T-105.md`.

## Verdict rationale

The activation itself is honest: the e2e spec is unchanged, the A-104 tests
are unchanged, and the one Go test change fixes an extraction regex that could
never match the frozen markup while making the assertion stricter rather than
looser. The red no-JS test is a real feature gap the worker escalated with
`BLOCKED:` instead of editing the test to fit the current shell+link behavior.
The remaining issues are auditability (the README's contradicting "only
sanctioned edit", the unattached holdout diff), a pre-existing Go-ratchet
blind spot for acceptance-tagged tests, an incomplete fix sketch that omits
`templates.go`, and documentation nits. None is a weakened/skipped test or a
hidden special-case in production code. This is a PASS on test integrity;
the task remains blocked on the gap-(a) app fix, and the branch cannot merge
with `make check` red.

VERDICT: PASS
