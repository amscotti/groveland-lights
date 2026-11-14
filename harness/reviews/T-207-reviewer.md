# T-207 review — Activate the M2 acceptance suite (A-201..A-209)

Reviewer: GLM-5.3 (zai-coding-plan). Scope: commit `e8ee540` vs `2877b66`
(the attached diff), judged against `harness/tasks/T-207.md` (the packet),
`AGENTS.md`, the frozen contract in `acceptance/README.md`, and the T-105
activation precedent the packet names as required reading.

## What the diff is

Pure `git mv` activation of the three frozen M2 drafts, the drafts-README →
`acceptance/README.md` move with a header rewrite, ONE content edit to the
activated e2e spec, manifest + ratchet regeneration, and the handoff.

## Verified (statically, byte-level via git; execution blocked by this
session's permission layer — see Limitations)

1. **Pure renames confirmed.** `git diff -M --stat 2877b66 e8ee540` shows
   `admin_auth_test.go` and `admin_lights_test.go` at 0 content changes;
   `harness/protected.sha256` carries the drafts' sha256 forward verbatim
   (`6a48178…`, `1c7bd11…`). The two Go oracles are byte-identical to what
   T-200 froze. Oracle-first property preserved.
2. **The single visible-spec edit is minimal, insert-only, and cannot change
   what the test asserts.** `acceptance/e2e/admin-walk.spec.ts:321-330` adds
   a comment + `await pin.scrollIntoViewIfNeeded();` before the raw-mouse
   drag. The assertion chain is untouched: the drag itself must move
   `#pin-pos` (`expect.poll … .not.toBe(before)`, lines 339-344), the arrow-
   key nudge must move it again (350-360), and the saved `#light-pos` must
   equal the final 6-dp string (373). If the app ignored drags, hid the pin,
   or failed to persist, the test still fails — scrolling cannot flip a red
   contract green; it only lets the frozen interaction reach the element on
   the 1280×720 desktop project (playwright.config.ts:26-48) against a pad
   that scales with its container. This is squarely the packet's rule-3
   mechanical class, it is disclosed as "the only one in this file" (true —
   single hunk), and it mirrors T-105's sanctioned regex pin
   (`harness/reviews/T-105-reviewer.md:75,182`).
3. **README rewrite is sanctioned and the frozen contract is intact.** The
   packet's deliverable 3 / rule 5 asks for a *new permanent*
   `acceptance/README.md` carrying the tables + visual notes. The diff has
   exactly one hunk (header lines 1-27 → 1-21); everything from
   "## Contract reconciliations" down — reconciliations, the frozen UI and
   HTTP contract tables, rerun-safety rules, fixture table, per-assertion
   visual notes, holdout mirror — is byte-identical (git single-hunk diff).
   `acceptance/drafts/` is gone (`acceptance/` = README.md, contract.json,
   e2e/, go/).
4. **Ratchet and manifest are the sanctioned mechanism, growth-only.**
   `RatchetCompare` fails on shrink only (`cmd/harness/ratchet.go:90-98`);
   go 51→128, pw 32→76. The guard explicitly allows `harness/.ratchet.json`
   when `make protect` regenerates it (`cmd/harness/guard.go:141-143`). The
   pw count is independently verified: 19 tests in `acceptance/e2e/*.spec.ts`
   × 4 projects = 76.
5. **Ownership respected.** Commit `e8ee540` touches only `acceptance/**`,
   `harness/protected.sha256`, `harness/.ratchet.json`, and the handoff —
   the packet's owns + sanctioned extras, nothing else. Worktree clean apart
   from the tolerated `node_modules` symlink (explicitly whitelisted in
   `DirtyLines`, `cmd/harness/ratchet.go:128`). No app code, go.mod,
   package.json, Makefile, or golden files touched. No `t.Skip` /
   `test.skip` / `test.only` / `.fixme` anywhere under `acceptance/`.
6. **The A-208 BLOCKED is the correct terminal state, not a dodge.** The
   feature matches its own governing packet: T-205 froze
   `POST /admin/lights/{id}` as "edit: label, asset_no"
   (`harness/tasks/T-205.md:70`) and `parseLightEdit` implements exactly that
   (`internal/web/admin.go:380-401` — no `kind` field). The holdout draft
   asserts kind editability under the README's generic "multipart
   csrf_token + changed fields" row (`acceptance/README.md:87`) — a genuine
   frozen-oracle-vs-feature-packet conflict only the planner can cut. T-207
   did NOT edit that test, left it red, and made `BLOCKED:` the handoff's
   first line: packet rule 3, bullet 1, followed to the letter. The visible
   twin (`TestA208EditSoftDelete`, label-only edit preserving
   asset/position/photo, `acceptance/go/admin_lights_test.go:420-483`) is
   untouched and green. `make check` (the gate) does not include holdout
   (`Makefile:136`), so nothing is bypassed.
7. **Security posture: no new surface, and the activated oracles assert the
   security contract for real.** No app/template/SQL/upload code changed.
   The drafts exercise a spawned binary end-to-end: CSRF token on every
   POST, session cookie flags parsed exactly — HttpOnly, SameSite=Lax,
   Max-Age=86400 (`acceptance/go/admin_auth_test.go:135-166`), one-time
   token replay → 403, wrong token → 403, blank → 200; bbox 422; >15 MB →
   413; HEIC/>40 MP/wrong type → 415; served photos byte-scanned for APP1 /
   EXIF-GPS absence and exact post-orientation dimensions. No secrets in
   the diff (the `ADMIN_TOKEN` in playwright.config.ts is the pre-existing
   dummy test value; the handoff names env vars, never values).
8. **Tests exercise behaviour, not mocks.** Go acceptance spawns the real
   binary per test (`aaSpawnDir`/`alSpawnDir`), drives real HTTP + files on
   disk (stray-file checks count PHOTO_DIR contents), and the e2e suite
   runs on all four projects including mobile. Helper prefixes (`aa*`,
   `al*`) are self-contained by design, so the rename couldn't collide with
   the existing package (`acceptance/go/admin_lights_test.go:549-550`).

## Holdout edits (#3–#6 in the handoff) — outside this diff, taken on
disclosure (the holdout checkout is not readable from this review sandbox)

I could not independently diff the holdout repo. Their justifications are
corroborated by frozen text in *this* repo, and none is asserted to weaken
anything:

- **A-209 distinct-set rewrite**: the frozen row markup repeats the label
  (thumbnail `alt` + label link — `acceptance/README.md:63,75`), so a raw
  `strings.Count == 1` is unsatisfiable against ANY compliant fragment; the
  visible twin deliberately uses per-lamp `strings.Contains` and an overlap
  check instead (`acceptance/go/admin_lights_test.go:539-546`) and passes —
  corroborating that the holdout's occurrence-count extractor was the
  anomaly. The disclosed replacement (distinct lamp-number set == `{"01"}`)
  is stronger, not weaker. Sanctioned rule-3 class ("unsatisfiable
  extractor against ANY contract-compliant markup").
- **Session file cache**: the frozen holdout-mirror section itself mandates
  the intent — "exchanges the token exactly once per run and reuses the
  session" (`acceptance/README.md:229-230`); a module-level memo cannot
  deliver that across four fresh worker processes. Bootstrap only.
- **findBin delegation**: matches the frozen M1 `holdoutFindBin` layering;
  binary discovery only.

Process nit for the operator (flagged by T-207 itself): the holdout
activation is uncommitted working-tree state and should be committed so the
frozen→activated audit trail exists there too.

## Minor notes

- The drafts README's "pure rename — no content edits; gofmt only" line is
  stricter than the packet's rule 3. The packet is the later, planner-issued
  authority and codifies the T-105 precedent; T-207 followed the packet and
  disclosed every deviation exhaustively (six edits, each with a one-line
  justification, as rule 3 requires). Noting for the record; not a
  violation.
- Handoff gap #3 (declining T-205's ask to touch `internal/web/web_test.go`)
  is correct — the packet's Never-list forbids anything outside
  `acceptance/**`.
- The disclosed port-8931 session-cache incident was runtime-state only,
  recovered per the README's own runbook note (`acceptance/README.md:143-145`).

## Limitations

This review sandbox's permission layer denied `make`, `go`, and shell
execution generally; verification is static — byte-level git diffs, manifest
cross-checks, count arithmetic, and code reading. The gate re-runs
`make check` (suites included) regardless, and the guard re-verifies the
manifest and ratchet.

## Verdict

The activation is faithful: both Go oracles byte-identical, one disclosed
and sanctioned mechanical spec edit that cannot mask non-compliance, frozen
contract tables preserved verbatim, ratchet growth-only via the sanctioned
mechanism, ownership respected, and the one genuine contract conflict
routed as BLOCKED exactly as the packet prescribes.

VERDICT: PASS
