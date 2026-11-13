# T-200 — reviewer-alt (test integrity / reward hacking), attempt 2

Review basis: the attached diff (`harness/reviews/T-200.diff`), the operative
packet `harness/tasks/T-200.md`, the downstream packets/contracts (T-201,
T-203, T-204, T-205, T-207, `acceptance/contract.json`), the real test topology
(`playwright.config.ts`, `Makefile`, `scripts/holdout-run.sh`, `.gitignore`),
and the checked-in existing acceptance suite.

Verification limits: the diff is **not applied** in this checkout (there is no
`acceptance/drafts/`, and `testdata/photos/*` is absent), shell execution is
denied in this session, and the holdout repo is outside the sandbox. I could
therefore not execute the handoff's commands; all findings below are static
checks of the drafted bytes and the contracts they freeze. The blocker is a
static certainty (an interaction is simply missing), not an environmental
guess. The handoff's exit codes and the holdout results are taken on trust.

## Findings

### 1. [Blocker] `@A-205` never submits its first create — the test can never pass

`acceptance/drafts/admin-walk.spec.ts.txt:311` calls
`addLightFromForm(page, { assetNo: asset, label: 'Dup panel lamp' })`, then
`:312` immediately expects `getByRole('status')` to contain `Saved`.

`addLightFromForm` (`:158-164`) only opens `/admin/walk`, clicks
`Add light here`, and fills the fields; its own comment (`:156-157`) says
“the caller clicks `Confirm pin`”. Every other caller does so
(`@A-206` at `:400`, `@A-208` at `:436`, `@A-203` at `:273`). `@A-205` does
not: no POST `/admin/lights` ever happens, no `Saved` status appears, and the
subsequent `#dup-panel` assertion at `:321` cannot be reached. On every
project, against a fully compliant M2 build, this test fails at its first
`Saved` assertion.

This is not a flaky wait or an extractor issue; the interaction that produces
the number under test is absent. The handoff's claim
(`harness/handoffs/T-200.md:18`) that `@A-205` “creates its own
uniquely-numbered light, … clicks `Merge`” is false, and the handoff's
would-fail analysis never walks this flow against a compliant build (the red
run fails earlier, at login). This is the same class of unsatisfiable-oracle
failure that blocked attempt 1.

### 2. [High] Detail-link discovery assumes exactly one link per row; the frozen contract does not

`acceptance/drafts/admin_lights_go_test.go.txt:97-115`: `alReDetail` matches
every `<a href="/admin/lights/N">…</a>` whose inner HTML contains the label,
and `alDetailURL` fatals unless exactly one href is found (`:111`).
`acceptance/drafts/admin-walk.spec.ts.txt:112-140`: `lightHrefsByLabel`
collects every matching link, and `deleteAllByLabel` (`:142-164`) deletes each
href.

The README only freezes “the label text links to `/admin/lights/{id}`
(accessible name contains the label)” (`acceptance/drafts/README.md:69`). A
compliant row with a thumbnail link (alt = label) and a separate label link
produces the same href twice. `alDetailURL` then fatals with two matches, and
`deleteAllByLabel` posts the same delete twice — the second `POST …/delete`
answers 404, so the cleanup `expect(res.status()).toBe(303)`
(`admin-walk.spec.ts.txt:133-137`) fails. Either dedupe hrefs in both helpers
or freeze “exactly one detail link per row”. As written, a plausible
compliant rendering fails the oracle.

### 3. [Medium] One-time-token replay is only tested from the same client

`acceptance/drafts/admin_auth_go_test.go.txt:122-126` replays the token with
`browser`, the same cookie-jar client that performed the exchange. The README
freezes “one-time, GLOBAL: replay from any client → 403”
(`acceptance/drafts/README.md:79`), and the handoff says the holdout tests a
fresh jar. The visible Go test would pass an implementation that scopes
consumption to a client (e.g. a burned-token cookie); only the hidden holdout
catches it. The already-created `bare` client (`:70`) is never used for the
replay, so the visible oracle is weaker than the contract it claims to freeze.

### 4. [Medium] Asset numbers are unique per run+project, not per test — failures poison later tests

`uniqAsset` (`admin-walk.spec.ts.txt:78`) yields
`GP-017-<run>-<projectCode>`. `@A-202` (`:194`), `@A-205` (`:300`) and
`@A-206` (`:388`) all use that same value. Pre-clean deletes by **label**
(`:142-164`), not by asset. If `@A-202` creates its light and then fails
before cleanup (`:201`), `@A-205`'s first create (once Finding 1 is fixed)
sees a live numbered duplicate instead of saving; if `@A-205` leaves its
light, `@A-206` inherits the collision. The handoff's “Each test is
self-contained” (`harness/handoffs/T-200.md:18`) is therefore false. A
per-test suffix (`…-<test>`) or cleanup by asset would make the tests
independent.

### 5. [Medium] `@A-202` low-accuracy test proves only button state, not the save gate

`acceptance/drafts/admin-walk.spec.ts.txt:217-219` asserts `Add light here` is
disabled, clicks `Save anyway (low accuracy)`, and asserts the button becomes
enabled. It never submits at accuracy 80 and never saves after choosing
“anyway”. A UI that disables the button but still accepts a form submit, or
shows the button without wiring it, passes. The contract
(`acceptance/contract.json:48`) is “save gated at <=30m accuracy or explicit
save-anyway”; the visible suite (the Go tests do not touch the UI gate) does
not prove it. The test should attempt a save at 80 m and assert it is blocked,
then choose save-anyway and assert a real save.

### 6. [Medium] `@A-206` preview assertion can pass with a CSS-stretched portrait

`acceptance/drafts/admin-walk.spec.ts.txt:423-424` compares the rendered
`boundingBox` width/height of `#photo-preview`. A portrait image displayed
with `width: 100%` in a wide container is wider than tall **without** any EXIF
correction, so the test can pass for the wrong reason. The README freezes
that the preview renders EXIF orientation applied
(`acceptance/drafts/README.md:66`). Assert the preview image's
`naturalWidth > naturalHeight` (and/or its intrinsic dimensions) instead of
only the CSS box.

### 7. [Low] `@A-205` numbered panel does not assert the negative half of the button contract

`acceptance/drafts/admin-walk.spec.ts.txt:321-323` asserts `Merge` is visible
but never asserts `Save anyway` is absent, nor that the cancel link exists.
The README freezes the numbered panel as `Merge` + a cancel link, with
`Save anyway` unnumbered-only (`acceptance/drafts/README.md:54`, `:58`). An
implementation that also shows `Save anyway` on numbered is not caught by the
e2e; the Go test catches the server's 422 but not the UI. Add
`expect(panel.getByRole('button', { name: 'Save anyway' })).toHaveCount(0)`.

### 8. [Low] E2E photo purity checks only GPS, not any APP1/EXIF

`acceptance/e2e/util/images.ts:22-55` returns `false` for an APP1 segment that
has no GPS IFD. `acceptance/drafts/admin-walk.spec.ts.txt:422` only asserts
`hasExifGps(bytes) === false`, so a served photo that retains EXIF orientation
or other metadata (without GPS) passes the e2e. The README freezes “NO APP1
segment (so no EXIF, no GPS)” (`acceptance/drafts/README.md:86`), and the Go
test catches any APP1 (`admin_lights_go_test.go.txt:270`). The e2e
assertion is weaker than the frozen contract; add an APP1-segment check or
state that the Go test is the only proof.

### 9. [Low] `@A-203` does not exercise the frozen keyboard-movable pin

`acceptance/drafts/admin-walk.spec.ts.txt:236-284` only drags with the mouse.
The UI contract freezes `#pin` as “keyboard-movable”
(`acceptance/drafts/README.md:52`). The holdout reportedly has a
keyboard-moved pin, but the visible oracle does not; a worker could omit arrow
key support and still pass the visible suite. Add an arrow-key nudge assertion.

### 10. [Low] Handoff overstates what was proven; the red run cannot catch Finding 1

`harness/handoffs/T-200.md:18` claims every test is self-contained and
describes `@A-205` as creating and merging lights; neither is true. The
handoff's red-against-main run fails all admin tests at login, so it cannot
distinguish a satisfiable draft from one that never submits its create. The
holdout is unreadable from this sandbox, so the `holdout: PASS` claim is
unverified. This is bookkeeping, but handoffs are evidence.

### 11. [Low] `admin.ts` cache-write fallback is not actually safe

`acceptance/e2e/util/admin.ts:170-181` swallows `writeCache` failures and the
comment says an unwritable worktree “simply forces the (one-time) exchange per
invocation”. The token is one-time **global**; the second Playwright
invocation (`a11y`) would try to exchange the already-consumed token and fail.
The fallback should surface the error or the comment should state that the
`make check` e2e→a11y sequence requires a writable cache path.

### 12. [Low] `TestA209` does not assert the frozen `#rows` fragment root

`acceptance/drafts/admin_lights_go_test.go.txt:448` onward checks the page-2
fragment's contents but never asserts the root element id `rows`, although the
README freezes “fragment root element id `rows`”
(`acceptance/drafts/README.md:85`). The e2e may catch a bad root indirectly via
HTMX, but the Go oracle is weaker than the contract.

## What is sound (checked)

- `testdata/schema.expected.sql` matches the packet's frozen block line for
  line; the probe statement is copied verbatim from
  `migrations/0001_init.sql:2-5`; no comments.
- `testdata/photos/gen/main.go` is stdlib-only, deterministic, and
  self-verifying; the APP1/TIFF layout, orientation fixtures, CRC-correct
  pixel bomb, HEIC sniff box, 15 MB cap, and tiny fixtures check out
  statically. The README's sha256 table is the documented protection.
- The Go drafts spawn their own `bin/server` with `t.TempDir()` DBs, free
  ports, `ErrUseLastResponse` clients, and context-bounded health waits; they
  have no cross-test state. The e2e drafts now use per-run/per-project asset
  numbers, jittered coordinates for unnumbered creates, pre/post cleanup,
  `maxRedirects: 0` on every `page.request.post`, a self-contained `@A-206`,
  `expect.poll` after the drag, and the link-or-button `Older` selector.
- `acceptance/e2e/util/images.ts` now has the truncated-SOF guard the Go twin
  has; the utils typecheck under the repo tsconfig and have no tests of their
  own (per Deliverable 5).
- The manifest adds the new acceptance files only; the pre-existing hashes are
  unchanged; the ratchet is unchanged (drafts are inert). No
  `t.Skip`/`test.skip`/`.only`/`.fixme`/TODO/FIXME is added; ownership is
  respected; no app code is touched.

## Required before merge

1. Fix Finding 1: add the missing `Confirm pin` click in `@A-205`'s first
   create and re-walk the whole numbered → Merge → unnumbered → Save-anyway
   flow against a compliant build.
2. Resolve Finding 2: dedupe detail hrefs in both helpers or freeze exactly one
   link per row in the README and the worker packet.
3. Strengthen the visible oracles where noted (Findings 3–9), especially the
   global replay and the `@A-202` save gate.
4. Correct the handoff claims (Finding 10) and regenerate
   `harness/protected.sha256` after any draft edit.

VERDICT: BLOCK
