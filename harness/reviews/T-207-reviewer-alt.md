# T-207 — reviewer-alt (test integrity / reward hacking)

Review basis: the attached diff (`harness/reviews/T-207.diff` equivalent), the
operative packet `harness/tasks/T-207.md`, the upstream contracts
`harness/tasks/T-200.md` / `T-205.md`, the activated acceptance tree
(`acceptance/README.md`, `acceptance/go/*.go`, `acceptance/e2e/admin-walk.spec.ts`),
`harness/protected.sha256`, `harness/.ratchet.json`, and the harness checks
(`cmd/harness/guard.go`, `cmd/harness/ratchet.go`).

Verification limits: shell execution is denied in this session, so the
handoff's executed exit codes, generated hashes and `make holdout` output are
taken on trust. The holdout repo
(`/Users/ascotti/Documents/git_repos/groveland-lights-holdout`) is outside the
sandbox; the holdout edits below are reviewed from the handoff's own
description, not from the holdout bytes. The visible acceptance tree is present
and was read directly.

## Findings

### 1. [Medium — BLOCKING: narrowed holdout assertion] A-209's replacement no longer proves "exactly one row"

`harness/handoffs/T-207.md:106-115` replaces the holdout A-209 page-2 count
with: extract **distinct** lamp numbers and require the set to be exactly
`{"01"}`. The handoff calls this "strictly stronger" and says "no assertion was
loosened" (`harness/handoffs/T-207.md:114-118`). Neither is true.

The original `strings.Count(frag, "Boundary lamp") != 1` was unsatisfiable
against the frozen row markup (the label appears in both `alt="<label>"` and
the label link), so a mechanical fix was appropriate. But a set-of-distinct-
numbers check passes if the fragment renders **the same lamp 01 twice** (or
N times): the set is still `{"01"}`. The contract for the boundary is that page
2 holds the remaining **one row** (`acceptance/README.md:89-91`, and the
visible unit test pins exactly this with `adminRowCount` at
`internal/web/admin_test.go:269-288`); the new holdout check cannot detect a
duplicated-row pagination defect that the old occurrence count (had it been
satisfiable) would have caught.

A stricter mechanical replacement is available and should be used: count
row-level markers (e.g. occurrences of `alt="Boundary lamp 01"`, one per row)
or distinct detail hrefs, and assert exactly one row plus the correct lamp
number. As written, the independent holdout oracle for A-209 is weaker than
the contract it is supposed to guard.

### 2. [Low-Medium — frozen-oracle edit needs explicit ratification] The @A-203 spec is not a pure rename

`acceptance/e2e/admin-walk.spec.ts:321-330` inserts
`await pin.scrollIntoViewIfNeeded();` before the raw-mouse drag. The change is
disclosed (`harness/handoffs/T-207.md:68-80`), does not alter the assertions
(the drag poll, keyboard nudge and saved-position equality remain), and fits
the T-207 packet's mechanical-edit class (`harness/tasks/T-207.md:41-47`).
However, the frozen contract says "pure rename — no content edits"
(`acceptance/README.md:8`) and T-200's packet says "T-207 may not edit them"
(`harness/tasks/T-200.md:29`). The manifest hash for the spec changed
(`harness/protected.sha256`: `33375de…` -> `c778176…`), so the activated file
is demonstrably not byte-identical. This is not a weakening, but like T-105's
regex pin it should be recorded as a ratified oracle edit rather than described
as a pure rename. I would not block on the edit itself if the planner ratifies
it; I would block on it silently passing as "verbatim".

### 3. [Low — stale/contradictory audit headers] Activated files still claim they are inert and unedited

The activated e2e spec still says "INERT DRAFT … activates it verbatim, with a
rename only and NO content edits" (`acceptance/e2e/admin-walk.spec.ts:4-5`), and
the activated Go file says "INERT DRAFT: `.go.txt` is compiled by nothing. T-207
activates it by pure rename with NO content edits" and points at the deleted
`acceptance/drafts/README.md` (`acceptance/go/admin_lights_test.go:7-17`). The
new README header likewise says "T-207 activated them by pure rename"
(`acceptance/README.md:8`). These are false after activation (the e2e spec has
the disclosed edit) and undermine the audit trail the activation is meant to
create. Comment-only; correct the headers to name the one sanctioned edit.

### 4. [Info — task is correctly blocked on A-208] The holdout A-208 dispute is left red, not hidden

`harness/handoffs/T-207.md:3-20` leaves
`TestHoldoutA208EditKindAndSoftDelete` red and routes it to the planner: the
feature's edit form accepts label/asset only (`internal/web/admin.go:380-402`,
`:423-477`), while the holdout variant expects a kind-only edit to persist.
The visible suite never edits kind, so there is no visible conflict. Assuming
the holdout variant exists as described, this is the packet's prescribed
`BLOCKED` terminal state, not a test-integrity failure by T-207. It does mean
the task is not green and must not be merged until the planner adjudicates.

### 5. [Info — ratchet] Growth only; the branch bump does not hide deletions

`harness/.ratchet.json` goes from `{"go":51,"pw":32}` to `{"go":128,"pw":76}`.
The Playwright arithmetic is real (11 new admin-walk tests × 4 projects = 44;
8 prior tests × 4 = 32; total 76). The guard compares the branch counts against
main's baseline (`cmd/harness/guard.go:161-183`), so raising the branch's copy
does not hide a test deletion. Pre-existing gap: `countGoTests` runs without
`-tags acceptance` (`cmd/harness/guard.go:341-352`), so the activated
acceptance-tagged Go tests are not ratchet-counted; the manifest is their only
guard (same gap T-105's review documented). Not introduced by T-207.

## What I checked and found sound

- **No visible test deleted, skipped, narrowed or disabled.** The two Go
  activations are 100% renames with unchanged manifest hashes (`6a48178…`,
  `1c7bd11…` in both the old and new `harness/protected.sha256`). Grep over
  `acceptance/` finds no `t.Skip`, `test.skip`, `test.only` or `.fixme`.
- **The @A-203 edit does not weaken the assertion.** The post-drag
  `expect.poll` still requires `#pin-pos` to change, the keyboard nudge still
  must change it again, and the detail `#light-pos` equality is untouched. The
  scroll only makes the pin reachable for raw-mouse events, which is a real
  user action.
- **No production special-casing.** The diff touches no `cmd/server`,
  `internal/`, `web/` or `migrations/` file. Fixture labels (`GP-017`,
  `Ball field lamp`, `Path light`, `Boundary lamp`) appear only in tests and
  templates' test fixtures, not in handlers/store code.
- **No mocks bypassing behaviour.** The Go tests spawn the real `bin/server`
  on fresh temp DBs over HTTP; the e2e tests drive a real browser. The e2e
  session cache is a login bootstrap validated by a real `GET /admin`, and the
  A-201 exchange/replay/cookie contract is asserted against fresh servers in
  `acceptance/go/admin_auth_test.go`.
- **Manifest/README consistency.** The README's frozen contract table and
  per-assertion visual notes survive verbatim (the rename diff is header-only,
  92% similarity); the manifest lists exactly the 15 `acceptance/**` files and
  the two Go hashes match their drafts. The changed e2e/README hashes are
  consistent with the disclosed edits.
- **No coverage theatre in the visible additions.** The activated Go tests
  assert exact status codes, the frozen messages, no-stray-file counts, exact
  800×600 output, no APP1/GPS, duplicate merge label application, edit
  preservation and pagination boundaries; the e2e tests assert the visible
  gates, alert text, preview intrinsics and saved-position equality.

## Verdict rationale

The visible activation is faithful and the Go renames are byte-exact. The
blocking issue is the holdout A-209 edit: the handoff's own description shows
the replacement no longer asserts the "exactly one row" boundary, while the
handoff claims it is "strictly stronger" and that no assertion was loosened.
That is the exact failure mode this review exists to catch. The @A-203 spec
edit is a defensible mechanical interaction fix, but it must be recorded and
ratified as an oracle edit, not passed off as a pure rename. Until the holdout
A-209 check pins the row count (or the planner explicitly sanctions the
narrower set check) and the @A-203 edit is ratified, the task is not ready to
merge.

VERDICT: BLOCK
