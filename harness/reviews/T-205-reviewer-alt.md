# T-205 — independent reviewer (alt)

Reviewer: reviewer-alt (DeepSeek V4.1 Flash) · Date: 2026-11-13
Scope: the committed T-205 diff (admin UI, templates, walk.js, tests, handoff) against
`acceptance/drafts/README.md`, `acceptance/drafts/admin_lights_go_test.go.txt`,
`harness/tasks/T-205.md`, and the existing store/auth/upload contracts.

## Summary

The handler tests are substantially stronger than round 3, and several earlier review
findings are genuinely fixed (the bbox alert extraction is scoped; the pager assertions
are scoped to the pager form; anonymous edit/delete are covered; `TestRenderAll` now
fails on a render error). The pagination and edit/delete tests are well scoped.

However, the retry resolves the missing store capability by writing raw SQL from the web
layer to the store-owned `lights` table (`admin_merge.go`) instead of escalating, and it
uses a self-deleting temporary Playwright harness under the owned `web/templates/**`
glob instead of the packet's `scratch/`. The claimed e2e verification is a vacuous
`--pass-with-no-tests` run. There are also real swallowed-error paths and two assertions
that cannot fail for the reason they claim. I would not accept this as-is.

## Findings

### 1. BLOCKING — Raw SQL against the store's table from `internal/web` to bypass the ownership guard
`internal/web/admin_merge.go:50-81` (`attachPhoto`) opens a second `sql.DB` and runs
`UPDATE lights SET photo_id = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`
from the web layer. `internal/store/store.go:2` states that the store package owns the
database and “every query”, so this is a layering violation, not merely a style choice.

The handoff admits the reason: `harness/handoffs/T-205.md:5-9`, `:38-55`, and
`:157-160` say `internal/store` is outside `owns`, the guard rejected the store method,
and the write was moved into the owned file instead. The correct response under
AGENTS.md rule 1 was `BLOCKED: internal/store needs EditLightPhoto`; this is the
round-3 review request being work-arounded rather than resolved.

Concrete consequences:
- It duplicates the store's DSN and path validation (`admin_merge.go:34-44`, `:54-57`)
  rather than reusing `store.dsn`/`store.checkDSNPath`, so the rules can drift.
- `mergeDuplicate` is non-atomic: `EditLight` commits the label, then `attachPhoto`
  commits the photo (`internal/web/admin_create.go:311-322`). A failed attach leaves the
  label changed and returns 500.
- The merge leaves the old `photos` row/file orphaned; `TestAdminDuplicateMergePhoto`
  even *asserts* the old photo still serves 200 (`internal/web/admin_test.go:803-807`),
  blessing the leak instead of asserting cleanup.

The tests for this path are the worker's own; the frozen A-205 draft asserts only the
label merge, so the photo-merge behaviour is self-certified.

### 2. BLOCKING — Self-deleting visual harness under `web/templates/**`, not `scratch/`; unreproducible evidence
`harness/handoffs/T-205.md:125-128` says the temporary visual harness lived under the
owned `web/templates/**` glob and “removed itself before commit (a temporary
`tmp_cleanup.go` deleted the spec, config and its own source)”. The task packet
(`harness/tasks/T-205.md:112-120`) requires a `scratch/` config and deletion of
`scratch/` before commit (guard exit 9). Using the owned glob plus a self-deleting Go
file avoids that guard and leaves no reproducible test.

In this worktree `web/templates/pw.config.ts`, `tmp_cleanup.go`, and `test-results/**`
are all absent, so the handoff's visual-evidence claims cannot be independently checked.
The handoff also says “The three `harness/reviews/T-205-*.md` files are still in the
worktree” (`harness/handoffs/T-205.md:15-17`), but only
`harness/reviews/T-205-visual-reviewer.md` exists. The handoff's “no guard bypass” claim
is contradicted by the merge-photo move and this harness handling.

### 3. HIGH — The claimed e2e verification is vacuous
`harness/handoffs/T-205.md:121-123` presents `make e2e-task TASK=T-205` exit 0 as
verification. `Makefile:112-116` runs Playwright with `--pass-with-no-tests`; the only
specs for these claims are inert `.txt` drafts (`acceptance/drafts/README.md:14-20`).
Zero tests execute, and the target is green by construction. The committed diff adds no
Playwright spec for A-202/A-203/A-208/A-209. The four claims are therefore not verified
by anything in the repository; the only visual check is the self-deleted harness.

### 4. MEDIUM — `classifyDuplicate` swallows store failures and then creates the light
`internal/web/admin_create.go:264-268`: on `NearbyLights` error it logs and returns
`(geo.DupDecision{}, false)`. `adminCreate` then falls through to `CreateLight`
(`:212-220`) with duplicate detection disabled. A transient DB error silently bypasses
the A-205 guard. `LightByAssetNo` errors are likewise ignored at `:255-262`. No test
exercises either path.

### 5. MEDIUM — Semantic validation runs before CSRF
`internal/web/admin_create.go:80-85` runs `parseWalkCore` (label, asset number, kind,
lat/lon) inside the boundary parser, before `VerifyCSRF` at `:176`. A malformed body with
a missing/mismatched CSRF token therefore gets 422 instead of the frozen 403. The tests
use a valid body for the 403 cases (`internal/web/admin_test.go:549-564`), so the
deviation is uncovered. The frozen order is parse (syntactic) → CSRF → bbox.

### 6. MEDIUM — Weak assertion: the asset-number validation case cannot fail for the right reason
`internal/web/admin_test.go:530` sets `want: "Asset number"` for the bad-asset case. The
full walk page always contains `<label for="walk-asset">Asset number</label>`
(`web/templates/pages/admin-walk.html:70`), so the check at `:544` passes for *any* 422
full-page re-render, not specifically the asset validation message. The frozen message
is “Asset number must be 1-32 letters, digits or dashes”; the assertion should check
that string (or scope to the alert). This is the coverage-theatre pattern.

### 7. LOW/MEDIUM — The unnumbered duplicate test's “Save anyway” assertion is satisfied by the low-accuracy button
`internal/web/admin_test.go:888` requires `strings.Contains(body, "Save anyway")`. The
walk page always renders `<button type="button" id="save-lowacc" hidden>Save anyway
(low accuracy)</button>` (`web/templates/pages/admin-walk.html:58`), so that half of the
condition is always true. Only `value="save"` is discriminating; the panel itself is
not scoped. `TestAdminDuplicatePanelContent` scopes the numbered case correctly; the
unnumbered case should use the same `#dup-panel` extraction.

### 8. MEDIUM — Hidden override of the frozen `loginPageTmpl`
`internal/web/admin.go:37-47` (`installLoginPage`) renders the login page at package
init and overwrites `loginPageTmpl` (declared in the frozen `internal/web/auth.go:147`).
It silently returns on error, and uses `template.Must`, so a template parse failure
panics at startup. This is a hidden coupling/backdoor around the frozen file rather than
a shared, owned template variable. It works, but it is the same guard-adjacent pattern as
finding 1.

### 9. LOW — Dead fields/methods
- `internal/web/admin.go:84` `listData.Page` is written (`:152`, `web_test.go:152`) but
  never read by any template.
- `internal/web/admin_create.go:40` `badSubmission.Error` is never used; the handler
  reads `.status`/`.message` directly.
These are dead code retained around tests/structures.

### 10. LOW — The merge-photo test encodes the orphaned old photo
`internal/web/admin_test.go:803-807` requires both the new and the *old* photo to serve
200. If the contract's “no stray file” intent (A-207) applies to replaced photos, this
test asserts the opposite. At minimum it should be a deliberate product decision, not an
accident.

## Notes (not blockers)

- `internal/web/web_test.go:116-118` skips `TestRenderAll` unless `GL_RENDER_SNAPSHOTS=1`;
  the handoff's “TestRenderAll fails on a failed render” is only true under
  `make templates-check`, not `make check-fast`. This skip is pre-existing.
- `web/templates/pages/admin-list.html` and `partials/admin-rows.html` duplicate the
  row/pager markup; a future edit can drift the full page and the HTMX fragment apart.
- `harness/reviews/T-205-visual-reviewer.md` reports PASS against 8 screenshots that are
  no longer in this worktree; the visual claims are therefore not independently
  reproducible here.
- The round-4 bbox-alert and pager-form fixes are real improvements and are correctly
  scoped.

VERDICT: BLOCK
