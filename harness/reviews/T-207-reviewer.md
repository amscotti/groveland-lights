# T-207 review — Activate the M2 acceptance suite (A-201..A-209)

Reviewer: reviewer (clean context). Scope: the attached diff (renames
`acceptance/drafts/*` → activated locations, README header rewrite, one e2e
edit, manifest + ratchet, handoff) against `harness/tasks/T-207.md`, the
frozen drafts (recovered byte-for-byte from `harness/reviews/T-200.diff`),
AGENTS.md and the guard/gate code. Shell was denied in this session; all
verification was done by reading the trees and the frozen-draft record.

## What was verified, and how

1. **Renames + drafts removal.** `acceptance/` now contains only
   `README.md`, `contract.json`, `e2e/`, `go/`; `acceptance/drafts/` is gone.
   The three activated files exist at the packet's exact target paths.
2. **Frozen contract preserved (packet rule 5).** I compared every section
   of `acceptance/README.md:26-237` ("## Contract reconciliations" through
   "## Holdout mirror") against the frozen drafts README recovered from
   `harness/reviews/T-200.diff:32-243`: byte-identical, including the frozen
   UI/HTTP tables, rerun-safety rules, fixture sha256 table and all
   per-assertion visual notes. Only the header prose (README.md:1-24) is new
   — sanctioned, since the drafts header was future-tense activation
   instructions now stale.
3. **Go specs: comment-only deltas.** The attached diff shows exactly one
   hunk per Go file, both rewriting the "INERT DRAFT" header comment to
   "ACTIVATED"; no code or assertion lines change. I read both activated
   files in full (`acceptance/go/admin_auth_test.go`,
   `acceptance/go/admin_lights_test.go`): dense black-box assertions (exact
   cookie-attribute parsing at admin_auth_test.go:133-157, one-time-token
   replay from a second cookie-less client at :159-167, APP1 byte-scan +
   exact 800×600 Orientation=6 dims at admin_lights_test.go:289-314,
   no-stray-files PHOTO_DIR counting at :362-415, pagination overlap check
   at :539-546). No `t.Skip`, no helper shortcuts. The files spawn their own
   servers per test, so `make test-acceptance-go`'s shared server does not
   mask per-test isolation.
4. **The @A-203 e2e edit is exactly what is disclosed.** Against the frozen
   draft (`harness/reviews/T-200.diff:549-615`), the activated
   `acceptance/e2e/admin-walk.spec.ts:302-378` differs by exactly one
   inserted statement (`await pin.scrollIntoViewIfNeeded();` at :332) plus
   its comment. The drag poll (`.not.toBe(before)`), the keyboard-nudge
   poll, and the saved-position equality (`#light-pos` text === `final`)
   are byte-identical — the edit cannot satisfy the test for a build that
   doesn't actually move the pin. This is squarely packet rule 3's
   "mechanical defect that cannot change what it asserts" class
   (interaction reachability, same family as the sanctioned `expect.poll`
   fix), it is listed with a one-line justification as required, and it is
   additionally labelled an ORACLE EDIT up for planner ratification
   (README.md:15-19, spec header :4-12, handoff "Content edits" item 2 with
   the hash lineage). The stricter "pure rename" wording in T-200's drafts
   README would have demanded BLOCKED instead; the packet governs and
   permits this, and the planner keeps the final call. Not a violation.
5. **A-208 holdout BLOCKED is the packet-sanctioned terminal state.**
   Handoff first line (harness/handoffs/T-207.md:3) is the required
   `BLOCKED: A-208 unmet — <precise gap>`; the holdout test was not edited.
   I verified the diagnosis is accurate, not a misdiagnosis:
   `internal/web/admin.go:423-477` (`adminEdit`) applies label+asset_no
   only, `web/templates/pages/admin-detail.html:7` renders Kind as static
   text with no form control, and T-205's packet freezes
   "`POST /admin/lights/{id}` (edit: label, asset_no)"
   (harness/tasks/T-205.md:70) — so the feature conforms to its own packet
   while the T-200 holdout oracle reads the drafts README's generic
   "changed fields" edit row (README.md:93) as including kind. Genuine
   contract ambiguity, correctly routed with both planner options. The
   visible suite never edits kind (Go edits label at
   admin_lights_test.go:444-446; e2e edits label at
   admin-walk.spec.ts:573), so no visible spec was softened to dodge it.
   `make check` (the task gate) excludes holdout; the red surfaces at M2
   validation where the planner must resolve it.
6. **Test integrity.** Grep over `acceptance/` for
   `t.Skip|test.skip|test.only|.fixme|fixme(`: zero matches. The guard also
   allows acceptance paths past the skip-marker check (guard.go:172-175),
   so I checked it directly. No assertions were loosened, moved or deleted
   in the visible suite (see 3 and 4 — deltas are comments + one
   reachability insertion).
7. **Ownership, manifest, ratchet.** Diff touches only `acceptance/**`,
   `harness/protected.sha256`, `harness/.ratchet.json`,
   `harness/handoffs/T-207.md`. The ratchet change is explicitly
   guard-sanctioned for `type: acceptance` tasks (cmd/harness/guard.go:
   140-144; packet deliverable 2). Manifest is complete (15 entries = every
   file under acceptance/, sorted as `GenerateManifest` emits) and drops
   the four deleted draft entries. Ratchet is growth-only: pw 32→76 is
   exactly 11 new admin-walk tests × 4 projects (I counted 11 tests in the
   spec; 8 M1 tests × 4 = the old 32); go 51→128 growth-only.
8. **Security.** Nothing app-facing is touched. The tokens in the test
   files mirror the already-committed `.env.test` dummy values
   (scripts/acceptance-go.sh:20-22) — not secrets. The suite *asserts*
   security-relevant behaviour: upload cap 413, HEIC/pixel-bomb 415, no
   APP1/EXIF-GPS in served photos, PHOTO_DIR untouched on rejects,
   CSRF on every POST, cookie flags parsed exactly. No template.HTML, no
   SQL, no path handling, no new deps (`allow_deps: []` — go.mod/package.json
   untouched).

## Findings

1. **[Low] README provenance claim is now false: "the two Go specs are
   byte-identical to their drafts" (acceptance/README.md:15-16).** Attempt
   3's header corrections rewrote both Go files' headers (comment-only —
   the attached diff shows those hunks), so the current manifest hashes
   (09506a3c…, 535d4ad6…) no longer equal the drafts'
   (6a481784…, 1c7bd11f…). The handoff discloses this precisely ("NOTE
   (attempt 3): … the current deltas against the drafts are header comments
   only"), but the README — the provenance record the packet points workers
   and reviewers at — contradicts the actual bytes. Same minor tension
   inside the Go files' own headers ("zero content edits … the manifest
   carried the draft's sha256 across the move",
   admin_auth_test.go:6-10, admin_lights_test.go:7-11), which was true only
   at the activation commit `e8ee540`. Comment-accuracy only; no assertion
   impact; a one-line README fix would resolve it. Not blocking.
2. **[Low/Info] Holdout-repo edits (handoff items 3–6) are unverifiable
   from this review context** — the holdout checkout
   (`harness/policy.json:52`) is outside this session's read scope
   (external_directory denied). The handoff discloses them exhaustively
   (including the rejected attempt-2 A-209 set-check and its restoration to
   an exactly-one-row pin), and the attempt-3 reviewer-alt verified the
   A-209 fix in that tree, but this review takes them on report.
   `make holdout` (validator scope) is the enforcement point.
3. **[Info] The frozen "Holdout mirror" section is now stale**
   (acceptance/README.md:228-237 says `<holdout_dir>/drafts/` "holds inert
   variants" that "activate by the same rename procedure"). Inherited from
   the packet-ordered verbatim preservation of the frozen body; the header
   above it discloses that activation happened. No action required.
4. **[Info] Prior-attempt review files cited by the handoff
   (harness/reviews/T-207-reviewer.md PASS at attempt 1;
   T-207-reviewer-alt.md five findings at attempt 3) are not in this
   worktree** — reviews are scheduler-copied, dirty-ignored artifacts
   (cmd/harness/guard.go DirtyLines, ratchet.go:115-134), so they live on
   main. The retry records' responses to the five findings are all
   traceable in the final tree (A-209 row-marker pin, oracle-edit
   labelling with hash lineage, corrected headers). No objection stands
   unanswered.
5. **[Info] Process disclosure:** attempt 1's renames went through a
   gitignored scratch helper because `mv`/`git mv` were permission-denied.
   Fully disclosed; the guard checks the final tree and commits, which is
   what this review examined. The one self-inflicted incident (session
   cache poisoned by a scratch server on port 8931) was recovered via the
   documented `rm -f test-run.db*` and re-run green.

## Verdict rationale

The packet's three risk points — don't edit the oracle to fit the feature,
disclose every mechanical edit, preserve the frozen tables — are all
satisfied: the one behaviour-bearing edit is minimal, assertion-preserving
(verified against the frozen bytes), and flagged for planner ratification;
the one failing holdout oracle was left red and routed as BLOCKED with an
accurate diagnosis; the frozen contract tables survive byte-identical. The
ratchet grows only; ownership and protected paths are respected; no test
integrity markers exist anywhere in the suite. Remaining findings are
documentation-accuracy nits (finding 1) and scope limitations of this
review (finding 2), neither of which violates the packet or weakens the
contract. The A-203 ratification and the A-208 dispute are correctly the
planner's to decide, with everything green otherwise.

VERDICT: PASS
