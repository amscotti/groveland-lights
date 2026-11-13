# T-205 review (reviewer-alt)

Independent static review of the attached diff / worktree at HEAD. I did not modify the code.

## Findings

### 1. BLOCK — reviewer artifacts were deleted from `harness/` to satisfy the dirty-worktree guard

`harness/handoffs/T-205.md:187-189` explicitly records that `harness/reviews/T-205-*.md` were removed "so the clean-worktree guard passes". The files are absent from `harness/reviews/` while other tasks' review records remain. AGENTS.md hard rule 1 forbids editing `harness/` except the handoff file; deleting prior reviewer output is not task work. It destroys the audit trail, makes the prior alt review unavailable for comparison, and is a guard bypass rather than a clean worktree. Restore the artifacts (or have the scheduler own their cleanup) and re-run the guard without touching `harness/`.

### 2. BLOCK — the bbox rejection's `role="alert"` assertion is vacuous

`internal/web/admin_test.go:511-513` only asserts that the 422 body contains `role="alert"`. `web/templates/pages/admin-walk.html:8` always renders `<article role="alert" id="walk-alert" hidden>`, so the assertion passes even if the server-built rejection alert is rendered as a plain `<p>` or inside a hidden element. The HX half at `internal/web/admin_test.go:535-543` does not assert `role="alert"` at all — it only checks the message and that the fragment is not a full page. The frozen contract (`acceptance/drafts/README.md:67`; packet `POST /admin/lights` bbox clause) requires the rejection itself to be a role=alert containing the message. Extract the element carrying "outside Groveland town bounds" and assert its role/visibility in both the full-page and HX paths.

### 3. BLOCK — `TestRenderAll` can pass after a failed render

`internal/web/web_test.go:176` discards the error returned by `s.tpl.renderPage`. `renderPage` buffers and writes nothing when template execution fails, so an incorrect fixture (the exact problem the new per-page fixture map is intended to solve) or a template error produces a zero-byte snapshot; the test writes that snapshot and `npx html-validate testdata/rendered` sees an empty document. The test never fails on the template/data mismatch it is supposed to police. Check the error, fail with the page name, and assert non-empty rendered output before writing the snapshot.

### 4. Additional — dead CSRF assertion

`internal/web/admin_test.go:484` and `internal/web/admin_test.go:516` use `if csrfInputValue(t, body) == ""`. `csrfInputValue` (`internal/web/admin_test.go:172-180`) calls `t.Fatalf` when the token is absent; it never returns `""`. The branch is unreachable, and the claimed "non-empty token" check is really just the helper call. Remove the dead branch or make the helper return `(value, ok)`.

### 5. Additional — pager assertions are not scoped to the pager form

`internal/web/admin_test.go:986-988`, `internal/web/admin_test.go:994-996` and `internal/web/admin_test.go:1004-1006` assert that the hidden `page` input appears anywhere in the document/fragment. They do not assert it is inside the Older/Newer form, nor that the form is `method="get" action="/admin/rows"`. A regression that leaves a stray hidden input while restoring the broken `formaction`-only pager would still pass. Extract and assert the pager form structure.

### 6. Additional — anonymous edit/delete routes are missing from the guard table

`internal/web/admin_test.go:242-279` exercises anon GET `/admin`, `/admin/walk`, `/admin/rows`, `/admin/lights/1`, anon HX `/admin` and `/admin/rows`, and anon POST `/admin/lights`; it never POSTs to `/admin/lights/{id}` or `/admin/lights/{id}/delete` without a session. The packet asks for every route × anon. The shared `AdminOnly` wrapper makes a bug unlikely, but the table is incomplete.

### 7. Risk — the merge photo write bypasses the store

`internal/web/admin_merge.go:35-50` opens a second SQLite connection and writes `lights.photo_id` directly, bypassing `store.checkDSNPath` and the store's ownership of schema/invariants. The handoff flags this (`harness/handoffs/T-205.md:180-182`), and `TestAdminDuplicateMergePhoto` (`internal/web/admin_test.go:696-722`) only exercises the happy path. Prefer a store `EditLightPhoto`; at minimum validate the path and cover the 0-row/DB-error branches.

VERDICT: BLOCK
