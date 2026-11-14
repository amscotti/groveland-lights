# T-207 review — Activate the M2 acceptance suite (A-201..A-209)

Review basis: the attached diff (commits `e8ee540` + `c2e9492` on `task/T-207`), the
operative packet `harness/tasks/T-207.md`, the frozen contract `acceptance/README.md`
(and its pre-activation bytes `acceptance/drafts/README.md` via `git show e8ee540^`),
the T-200 packet/reviews (the drafts' history), the T-105 precedent review
(`harness/reviews/T-105-reviewer.md`, required reading per the packet), the merged
feature (`internal/web/admin.go`, `web/templates/pages/admin-walk.html`), and the
harness mechanics (`cmd/harness/ratchet.go`, `cmd/harness/guard.go:141-143,341-366`,
`scripts/holdout-run.sh`, `playwright.config.ts`).

Verification limits: this session's shell is read-only (go/playwright/make/shasum
denied), so suites were not re-executed; the holdout repo is correctly inaccessible
(external_directory — the same limit T-200's reviewers recorded). Everything below is
static verification via git plumbing, which turned out to be decisive for the claims
that matter.

## What is correct (verified)

- **Pure renames are byte-exact.** `git diff e8ee540^ e8ee540 -M` records
  `acceptance/drafts/admin_auth_go_test.go.txt → acceptance/go/admin_auth_test.go`
  and `admin_lights_go_test.go.txt → acceptance/go/admin_lights_test.go` at
  `similarity index 100%` with zero content hunks, and the regenerated
  `harness/protected.sha256` carries the SAME sha256 for both files
  (`6a48178…`, `1c7bd11…`) as the pre-activation manifest carried for the drafts.
  Two independent signals agree on byte-identity — the oracle-first property
  (tests predate implementations) survives activation auditable in git history.
- **The frozen contract table is preserved.** `acceptance/README.md` differs from
  the drafts README only in the header prose (diff hunk lines 1–27); everything
  from `## Contract reconciliations` down — the UI contract table, HTTP contract
  table (incl. the disputed edit row at `acceptance/README.md:87`), rerun-safety
  section, fixture hashes and the per-assertion visual notes (`:172`+) — is
  unchanged. Packet Deliverable 3 satisfied; `acceptance/drafts/` is gone.
- **The single visible-suite test edit is the packet's sanctioned mechanical
  class, and nothing was weakened.** The one hunk in
  `acceptance/e2e/admin-walk.spec.ts:321-330` inserts
  `await pin.scrollIntoIfNeeded();` (plus a justification comment) before the
  raw-mouse drag. Verified against the geometry: `#pinpad` is a fluid square SVG
  (`web/templates/pages/admin-walk.html:12`, `width="100%"`, viewBox 200×200) in
  the pico `container` (`web/templates/base.html:28`), so on the 1280×720
  desktop-chromium project the pin (viewBox centre, `admin-walk.html:16`) renders
  below the fold and the draft's raw `page.mouse` events — which bypass
  Playwright's auto-scrolling actions — could never hit it: an
  interaction-reachability defect against ANY contract-compliant build that scales
  the pad, not an assertion choice. The `expect.poll` still requires the drag
  itself to move `#pin-pos` (`admin-walk.spec.ts:339-344`); the keyboard nudge
  (`:350-360`) and the saved-position equality against `#light-pos` (`:373`) are
  untouched. This is exactly the T-105 "regex pin" precedent
  (`harness/reviews/T-105-reviewer.md:75-88`: unsatisfiable interaction/extractor,
  kept + disclosed + planner ratification requested), sanctioned by packet rule 3
  (`harness/tasks/T-207.md:41-47`).
- **The A-208 holdout BLOCKED is the sanctioned terminal state, not a gate
  dodge.** The feature matches its own governing packet — T-205 freezes
  "POST /admin/lights/{id} (edit: label, asset_no)" (`harness/tasks/T-205.md:70`)
  and `lightEdit`/`parseLightEdit`/`adminEdit` implement exactly that with no Kind
  field (`internal/web/admin.go:380-402, 424-477`) — while the frozen README's
  generic edit row ("multipart `csrf_token` + changed fields; unspecified fields
  keep values", `acceptance/README.md:87`) reads as permitting a kind edit. That
  is a genuine T-200↔T-205 contract conflict: the oracle was NOT edited, the app
  was NOT touched, the holdout test is left red, and the handoff's first line is
  the required `BLOCKED: A-208 (holdout variant) unmet — …` with a precise gap and
  both planner options. Identical in shape to the accepted T-105 disposition
  (frozen no-JS oracle left red, `T-105-reviewer.md:43-59`). The visible suite
  genuinely conflicts with nothing here: both the Go
  (`acceptance/go/admin_lights_test.go:444-446`) and e2e
  (`acceptance/e2e/admin-walk.spec.ts:571`) A-208 tests edit label only.
- **Ratchet is growth-only and arithmetically exact.** `harness/.ratchet.json`
  go 51→128 (repo-wide `go test -list ./...`, `cmd/harness/guard.go:341-353`),
  pw 32→76 = 19 tests × 4 projects — I counted exactly 19 `test(` declarations in
  `acceptance/e2e/` (11 of them in the activated `admin-walk.spec.ts`), i.e. the
  pw growth is precisely the 11 activated tests. The ratchet edit is itself
  sanctioned (packet Deliverable 2; `cmd/harness/guard.go:141-143`).
- **Ownership and hygiene.** Both commits touch only `acceptance/**`,
  `harness/protected.sha256`, `harness/.ratchet.json` and the handoff (verified
  via `git show --stat`); worktree clean apart from the tolerated `node_modules`
  symlink. No `t.Skip`/`test.skip`/`test.only`/`.fixme`, no `console.log`, no
  "Phase N" strings in the activated files (grep clean). Every content edit —
  six, counting the holdout — is listed in the handoff with justification; the
  handoff claims "ideally none" and honestly reports otherwise.
- **Disclosure quality is exemplary.** The handoff discloses the shell-allowlist
  workaround (byte-preserving `os.Rename`/`os.Remove` via a gitignored scratch
  helper — outcome independently confirmed by the 100% rename detection), the
  recovered session-cache incident, the holdout-repo commit `4b64fbc`, per-claim
  proof tables A-201..A-209 with test names, and correctly routes out-of-owns
  requests back to the planner (T-205 handoff issue 7,
  `harness/handoffs/T-205.md:196-200` → `internal/web`, which the packet's Never
  clause forbids T-207 from touching). The pending human field test is listed as
  pending-human evidence, as the packet requires.

## Security / architecture / test-quality pass

- Security: no app code, template, SQL, upload, or auth code changed in this diff,
  so CSRF/XSS/SQL/traversal/upload-limit exposure is unchanged; the activated
  specs themselves pin the security posture (csrf_token on every admin POST,
  HttpOnly/SameSite=Lax cookie flags, one-time GLOBAL token replay → 403, 413/415
  upload gates, APP1/GPS strip, bbox 422). Only dummy `.env.test` literals appear
  (already public in `playwright.config.ts:60` / `scripts/holdout-run.sh:19`); the
  handoff names `ADMIN_TOKEN` without leaking any real secret.
- Architecture: nothing introduces an HTMX target near `#map` (admin pagination
  is frozen to the `#rows` fragment, `acceptance/README.md:91`; the walk page uses
  the pin-pad SVG, not the map). Parse-at-boundary/typed-ID rules are untouched
  (test-only diff). The activated tests are black-box (spawn + HTTP + byte scans
  + real browsers) and DO exercise the behaviour: the low-accuracy gate is proven
  by a real 80 m save after the explicit choice (`admin-walk.spec.ts:274-279`),
  the drag test proves save-equality at 6 dp (`:373`), and the photo pipeline is
  byte-scanned for APP1 absence.
- Both reviewers were directed to look for weakened assertions (packet rule 3):
  none found. The one edit in the visible suite strengthens reachability only;
  the handout's holdout A-209 edit (distinct-lamp set instead of raw substring
  count) is strictly stronger given the frozen alt-format
  (`acceptance/README.md:63,75` — a compliant row legitimately mentions the label
  twice).

## Findings (non-blocking)

1. **Contract-text tension worth explicit ratification.** The frozen drafts
   README header said "pure rename — no content edits … a gofmt pass is the only
   sanctioned mechanical step", while the operative T-207 packet (rule 3,
   `harness/tasks/T-207.md:41-47`) sanctions minimal mechanical edits. The packet
   governs and the edit fits its carve-out, but the handoff's "Gaps" list asks
   the planner to ratify the six T-200 reconciliations without explicitly adding
   the scroll edit (`admin-walk.spec.ts:321-330`) to that ratification request.
   Recommend the planner ratify it in the same breath.
2. **Holdout claims are verified on trust.** The three holdout renames, four
   holdout edits and commit `4b64fbc` are outside this sandbox (by design). They
   are internally consistent with `scripts/holdout-run.sh` (one server, one
   one-time global ADMIN_TOKEN, per-project workers plausibly defeating a
   module-level memo) and with the frozen README:84, and the decision to leave
   the holdout A-208 red rather than bend it is itself strong anti-oracle-editing
   evidence — but no reviewer of the main repo has seen those bytes.
3. **Audit-trail gap:** the retry record cites an attempt-1 primary PASS at
   `harness/reviews/T-207-reviewer.md`, but no such file exists in the tree
   (scheduler-side bookkeeping; review files are not the worker's to manage).
   Noted so the discrepancy is on record.
4. **Execution not re-proven by this review.** Shell was read-only, so the
   76/76 ×4, a11y 12/12, acceptance-go 7/7 and `make check` exit codes are taken
   from the handoff. Structural cross-checks (ratchet arithmetic exact at 19×4=76,
   byte-identical renames, deterministic desktop-fold geometry) all corroborate
   the reported results, and `make check`/guard re-run at the gate independently.

## Verdict rationale

The activation did what the packet froze: byte-exact renames (proven by rename
detection + identical manifest hashes), the contract table and visual notes
preserved verbatim, exactly one minimal disclosed mechanical edit in the visible
suite that changes no assertion (proven by the diff hunk and the pin-pad
geometry), growth-only ratchet (arithmetically exact), no skip/only/weakening
patterns, ownership respected — and the one genuine feature-vs-oracle conflict
(A-208 holdout kind edit) handled the only honest way: test untouched, app
untouched, `BLOCKED` first line, dispute routed to the planner with both
resolution options. This mirrors the adjudicated T-105 precedent precisely.

VERDICT: PASS
