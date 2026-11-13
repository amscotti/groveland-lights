# T-200 review (attempt 5) — M2 acceptance drafts, fixtures, schema oracle, frozen UI contract

Review basis: the attached diff (`harness/reviews/T-200.diff`, identical to the
diff supplied with this review request) against `harness/tasks/T-200.md`
(including the appended attempt-1/2 reviews), the downstream packets
(T-201, T-202, T-203, T-204, T-205, T-206, T-207), `acceptance/contract.json`,
`docs/spec/architecture.md`, and the repo's actual test topology
(`playwright.config.ts`, `Makefile`, `scripts/acceptance-go.sh`, `.gitignore`,
`.env.test`, `tsconfig.json`, `.golangci.yml`, `migrations/0001_init.sql`,
`cmd/harness/guard.go`, `harness/policy.json`, `harness/tasks/graph.json`, and
the existing `acceptance/go/*` suite). Prior rounds: attempts 1–2 BLOCK,
attempts 3–4 PASS (both reviewers).

Verification limits: the diff is **not applied** in this checkout (no
`acceptance/drafts/`, no `testdata/photos/*`, no `testdata/schema.expected.sql`
on disk — main has no `/admin` routes, `migrations/` holds only 0001, verified
by grep), and shell execution is restricted to a small allowlist (`ls`, `git
status/log`, `grep` on files), so sha256 recomputation of the manifest entries,
dynamic re-runs, and holdout inspection (`harness/policy.json:52`, external
repo, commit `2dd3354` per handoff) were not possible — the same constraints as
all four prior rounds. Every finding below is a static check of the diffed
bytes against the packets and repo facts I could read; the handoff's executed
red/green exit codes are taken on trust where marked. The merge gate re-verifies
the manifest (`VerifyManifest`) and the fixture generator self-verifies its
bytes on every run.

## What this attempt is

Attempt 5 is a pure hardening pass on the PASS'd attempt-4 bytes: it closes
every residual Low/Info the two attempt-4 reviews recommended fixing "while
the author still owns the oracle" (T-207 activates by pure rename and may not
edit). I verified each claimed fix is present in the bytes and — the part that
matters for a frozen oracle — that each strengthened assertion remains
satisfiable against the worker packets it governs. Nothing was weakened,
deleted, skipped or special-cased.

## Attempt-4 residual findings — verified closed in the bytes

| Attempt-4 finding | Fix verified (location in `harness/reviews/T-200.diff`) |
|---|---|
| alt 1 (Low): `path=/` substring also accepts `Path=/admin`; Max-Age never asserted | `admin_auth_go_test.go.txt` diff:1052-1076 — Set-Cookie parsed into an attribute map with EXACT comparisons: `attrs["path"] != "/"` fails `Path=/admin`; `attrs["max-age"] != "86400"` now enforces the 24 h MaxAge (T-203.md:62-64); `samesite == lax`; `httponly` presence. `Secure` deliberately unasserted (frozen prod-only, server runs APP_ENV=test) — stated in code and README. Correctly closes the finding: `Path=/admin` splits to `path=/admin` ≠ `/` |
| alt 2 (Low): blank-token 200 is status-only | diff:1025-1033 — the four frozen markup regexes (heading/label/input/button) re-run on the blank-token body. Matches T-203.md:53-56 ("no/blank token → 200 full page") |
| alt 2b (Low): `#walk-pos` asserted loose | diff:439-441 (`openWalkForm`) and diff:475 (test 1) — `\d+\.\d{6}, -?\d+\.\d{6}`. T-205.md:89 freezes `#walk-pos` as `"lat, lon" 6 dp`, so the whole-text regex (Playwright `toHaveText` requires a full match) is satisfiable by exactly that rendering; `#pin-pos`/`#light-pos` stay anchored 6 dp |
| alt 3 (Low): preview could be a static landscape placeholder | diff:760-791 — before/after `src` capture with `hadPreview` guard; picking the file must change (or first produce) the rendered source, and the decoded-frame poll (`naturalWidth > 0`) precedes the intrinsic-dims comparison. Any compliant mechanism qualifies (object URL / data URL / processed thumbnail — CSP `img-src ... data: blob:` per architecture.md:38 legalises both); only a static placeholder or a no-src img fails. Sensible choice over freezing `naturalWidth === 800`: the README explicitly allows a processed preview of any size, and the exact dims stay asserted on the SERVED bytes (Go, exactly 800×600 = T-202.md:83's own freeze, no-upscale T-202.md:79) |
| alt 4 (Low, flake): Delete then immediate `goto` | diff:821-824 — `waitForURL(/\/admin$/)` (the frozen 303 target, T-205.md:71) before leaving the page |
| alt 5 (Low): "unspecified fields keep values" proved only asset+position | `TestA208EditSoftDelete` now creates WITH `landscape.jpg` and asserts the SAME `src="/photos/..."` survives the field-omitting edit (substring match, so even a cache-busting suffix would still pass — the check fails only if the photo genuinely changes/clears). Satisfiable: T-205's edit touches label/asset_no only |
| alt 6 (Low): merge proved one row, not the merge | Go `TestA205Duplicates`: survivor's alt must be exactly `alt="GP-017 Ball field lamp twin"` and `alt="GP-017 Ball field lamp"` (closing-quote anchored, so the twin string cannot satisfy the original check) must be ABSENT — grounded in T-205.md:67-68 ("apply photo/label onto ExistingID") + the frozen alt format. e2e twin: diff:677-678 (`img[alt^=asset]` count 1 AND `img[alt="<asset> Dup panel lamp twin"]` count 1) |
| alt 7 (Low): low-acc never attempts a save while gated | diff:518-519 — `toHaveAttribute('disabled')` in addition to `toBeDisabled()`. The platform attribute is what T-205.md:91-92 freezes and it is a reflected IDL attribute (both plain DOM and Alpine `:disabled` produce it), so the assertion is satisfiable; an aria-disabled/CSS-only gate now fails. The real-save-at-80 m half was already present (diff:523-530) |
| reviewer #9 (Info): HTTP table lacks `GET /admin?page=N` | README table row added ("the same list page for page N (full page; the cleanup sweeps rely on it)") — documents what T-205.md:49-50 already implements; no new behaviour |

All twelve attempt-2 closures and the attempt-1 blockers were re-verified still
in place (per-test `uniqAsset` tags, jittered unnumbered coordinates, href
dedupe in `lightHrefsByLabel` (Set) and `alDetailURL` (`seen` map), replay from
the cookie-less `bare` client (diff:1083-1086), `maxRedirects: 0` on every
`page.request.post`, self-contained @A-206, `hasAPP1 === false` in e2e,
keyboard pin nudged+saved, `#rows` fragment root asserted, reconciliations 1-3
aligned with T-205.md:58/65-69, token economy via memo + gitignored
`test-run.db.session.json` (`.gitignore:20` `test-run.db*` verified) +
validation GET).

## Correctness spot-checks that passed

- **Satisfiability of the tightened cookie parse**: Go's `http.Cookie` renders
  `Path=/; Max-Age=86400; HttpOnly; SameSite=Lax` for T-203.md:62-64's frozen
  attributes; the split-on-`;`/`[1:]` correctly skips the cookie value (the
  base64url+hex value cannot contain `;`), lowercasing normalises Go's
  canonical attribute names, and flag-only attributes (`HttpOnly`) land as
  presence-with-empty-value.
- **e2e geometry re-walked**: jitter band lat [42.72, 42.7499], lon
  [-71.0801, -71.0501] — inside TOWN_BBOX; adjacent grid cells ≥ ~8.2 m (lon)
  / 11.1 m (lat) apart, above `UnnumberedRadiusM = 8.0` (T-204.md:38);
  pagination lanes -71.0480..-71.0495 sit ~49 m clear of the band edge and 41 m
  apart, seeds ~69 m apart along the column; @A-205's unnumbered pair ~4.4 m
  apart (inside 8 m) with the numbered pair ~7 m (inside the 25 m candidate
  window, T-204.md:37); Go pagination seeds 42.7605..42.7875 /
  -71.0445..-71.0175 in-bbox and >8 m apart. The 55-seeds-newest argument
  holds because `created_at DESC, id DESC` is frozen with the id tie-break.
- **`uniqAsset` budget**: `GP-017-<run8>-<proj2-4>-<tag≤5>` ≤ 26 chars, within
  the frozen `^[A-Za-z0-9-]{1,32}$`.
- **Substring hazards checked**: "Ball field lamp" ⊂ "Ball field lamp twin"
  only in the safe direction (the twin string is queried for absence);
  zero-padded "Pagination lamp 0N" labels are not substrings of each other;
  `img[alt*="Dup panel lamp twin"]` cannot match the original's alt.
- **Drafts are genuinely inert**: `.go.txt` carries `//go:build acceptance`
  and lives outside `./acceptance/go/...` (scripts/acceptance-go.sh:36), so
  `go vet ./...`, `golangci-lint run ./...` (tag not set; also gosec/errcheck
  excluded for `_test.go` anyway), `go test -race ./...` never compile it;
  `.spec.ts.txt` sits outside `testDir: 'acceptance/e2e'`; the ACTIVE utils
  match no spec pattern (default testMatch) and are type-checked by
  `make js-check` (tsconfig.json:15). Ratchet `go=51 pw=32` unchanged —
  correct for inert drafts. At activation, the acceptance tag keeps the
  drafts out of gate-time lint/vet, and `gofmt -l .` is the only gate check
  that will see them (T-207's sanctioned gofmt pass covers it).
- **Pure-rename activation stays collision-free**: `aa*`/`al*` identifiers
  are disjoint from `probe*`, `mig*`, `m1*`, `envOr`, `syncBuf`
  (grep-verified); each draft file is self-contained as required; spawn env
  byte-identical to the proven `probeSpawnDir` (probe_test.go:289-301);
  test names `TestA20x*` disjoint from `TestA1xx*`.
- **Schema oracle**: `testdata/schema.expected.sql` compared line-by-line to
  the packet's frozen block (T-200.md:86-169) — all 11 statements identical;
  probe statement verbatim from migrations/0001_init.sql:2-5 (comment/INSERT
  correctly excluded).
- **Fixtures**: generator stdlib-only, deterministic, self-verifying on every
  run; APP1/TIFF layout consistent across the three independent parsers
  (`parseExifAPP1`, `alHasExifGps`/`alTiffHasGps`, images.ts `tiffHasGpsIfd`);
  CRC-correct 45-byte bomb whose `DecodeConfig` sees >40 MP; 16-byte
  ftyp/heic box matching T-206.md:71's sniff; 15 MB commit cap enforced;
  no >15 MB file committed (oversize bodies synthesised at runtime, per
  packet Deliverable 1).
- **Red-by-design is factual**: main has no `/admin` route anywhere under
  `internal/` (grep) and no 0002 migration; the handoff's executed red runs
  (7/7 Go at the guard redirect/login, 11/11 e2e at login/`toHaveURL`) fail at
  exactly the first frozen assertions such a main would fail at.
- **`@axe-core/playwright` 4.14.0 and `@playwright/test` are existing
  devDependencies (package.json:9-10), already imported by home.spec.ts /
  probe.spec.ts with the same tags — `allow_deps: []` respected (no new deps;
  Go side stdlib-only per T-200.md:47-48, verified: no sqlite driver, spawn +
  HTTP + byte/file inspection only).

## Residual findings (none blocking)

1. **[Info] `harness/tasks/graph.json` status flip (`review`→`merging`) rides
   in the diff** (diff:2933-2945). Outside the packet's `owns` but permitted
   by `acceptanceAllowed` (guard.go:243-246 includes `harness/tasks/**`);
   scheduler-attributed; on disk the scheduler has since moved T-200 to
   attempts:4/review again, so the hunk is bookkeeping, not content. Same
   note as attempt-4 finding 5. No test-integrity impact.
2. **[Info] Reconciliation 4 (client-side bbox alert on each position update,
   without submitting) remains an addition beyond T-205.md:84-101's walk.js
   behaviour list.** Satisfiable (watchPosition fires → bbox check →
   role=alert with the frozen server message substring), recorded in the
   README's reconciliation item 4, and T-205 is directed to the drafts as the
   primary oracle (T-205.md:21-24) — but planner ratification is still
   pending (handoff note 1, sixth item). It stays the most likely
   activation-time surprise; unchanged from the PASS'd attempt 4.
3. **[Low] README "Contract reconciliations" intro says "froze three details
   differently" yet lists four numbered items** — item 4 reconciles
   packet-vs-packet rather than draft-vs-packet. Wording only; the handoff's
   "Six items total" for ratification counts correctly.
4. **[Low] Residual probabilistic collisions, disclosed and bounded**: the
   distinct-key jitter birthday window (~16 keys over 90,000 cells, order
   0.1% per run; README/handoff say ~1/300 per project for the seed case) and
   @A-203's dragged+nudged pin offset landing within 8 m of an occupied
   neighbouring cell (~1e-4). Both fail loudly (409 instead of `Saved`) and
   are rerun-safe by construction (pre-clean sweeps). Accepted unchanged by
   both attempt-4 PASS reviews.
5. **[Low] @A-204's `page.getByRole('alert')` (diff:624) is a strict-mode
   single-element locator**: a compliant walk page that renders a second,
   simultaneous `role=alert` region would trip a strict-mode violation rather
   than the intended message check. The contract table freezes exactly one
   alert for the bbox rejection and the same shape passed two review rounds;
   noted so T-207 recognises the class if it ever fires.
6. **[Low] The e2e draft implicitly freezes the detail-page edit/delete as
   plain form POSTs** (`toHaveURL` after `Save changes`, diff:811-812;
   `waitForURL(/\/admin$/)` after `Delete light`, diff:821-824) — an hx-post
   implementation would not navigate and would fail these waits. T-205.md:70-71
   freezes the 303s without naming the transport, and the drafts are the
   primary oracle, so this is satisfiable; flagged for T-207 awareness only.
   Pre-existing shape since attempt 3.
7. **[Info] A-207 has no e2e coverage** (Go-only: `TestA207NoStrayFiles` plus
   the 413/415 matrix inside `TestA206PhotoPipeline`). Packet-conformant —
   Deliverable 4's test list (T-200.md:231-257) contains no @A-207 e2e entry;
   recorded for the judge's coverage map.
8. **[Info] Unverifiable in this session** (same constraint as all prior
   rounds): the six manifest sha256s (additions-only, hash-sorted, all nine
   pre-existing entries untouched — verified in the hunk, diff:2917-2932; the
   gate's `VerifyManifest` re-checks at merge), the binary fixture bytes
   (generator self-verifies; README records per-file sha256), the holdout
   drafts (external repo, `2dd3354`), and the handoff's exit codes. The
   handoff's per-attempt manifest accounting ("exactly those 4 re-hashed"
   this attempt vs the 6 cumulative task entries) is internally consistent.

## Security / architecture / test-quality pass

- **Security**: no app code is touched — no `template.HTML` (also a
  forbidigo-pattern, .golangci.yml:30), no SQL, no request handling. Upload
  abuse is frozen exhaustively: 413 over 15 MB with a runtime-built body
  (valid JPEG + junk after EOI, so the size check — not sniffing — fires),
  415 for HEIC (frozen wording ⊂ T-206.md:68-70's message), text-as-jpg and
  the >40 MP bomb (sniffing, never filenames), no-stray-files on every reject
  path, whole-APP1 strip (stronger than GPS-only, asserted at byte level in
  BOTH suites), long edge ≤ 1600, no upscale (exact 800×600 for the
  Orientation=6 fixture). `csrf_token` is required on every admin POST in
  both suites; the session cookie is frozen HttpOnly/SameSite=Lax/Path=/
  Max-Age=86400 with exact parsing. The only literals are the dummy
  `.env.test` secrets already public in `playwright.config.ts:58-61`,
  `scripts/acceptance-go.sh:20-23` and `probe_test.go:295-297`. The session
  cache holds only a localhost test cookie, written 0600 under the gitignored
  `test-run.db*` pattern, with the write-failure consequence surfaced on
  stderr (admin.ts `writeCache`). A `/photos/` traversal probe is absent from
  the drafts but is explicitly T-206's unit-test territory (T-206.md:76-78,
  94) — packet-conformant, not blocking.
- **Architecture**: ownership respected — only `acceptance/**`,
  `harness/protected.sha256`, `testdata/photos/**`,
  `testdata/schema.expected.sql`, the handoff, plus the acceptance-allowed
  `harness/tasks/**` flip (finding 1). HTMX never swaps `#map` or ancestors:
  the drafts freeze `#rows` as the pagination swap root and `#dup-panel`/form
  targets for the walk flow; the walk page uses a pin pad, not the map
  instance; @A-209 additionally proves no full navigation occurs. Parse-
  don't-validate and typed IDs are N/A (no app code). No "Phase N" strings
  anywhere in the diff (grep: zero matches). The drafts enforce the
  architecture's own rules rather than bypassing them.
- **Tests actually exercise the behaviour**: every claim A-201..A-209 has
  black-box coverage against real spawned `bin/server` binaries (fresh
  `t.TempDir()` DB per Go test, `ErrUseLastResponse` clients, byte-level
  JPEG parsing) and a real browser on all four projects, including the
  negative halves (numbered panel has no `Save anyway`; nothing saved while
  the panel is up in both suites; blank token renders the full frozen form;
  replay refused from a second, cookie-less client; `Path=/admin` cannot
  pass). The attempt-5 additions are all stricters — attribute-exact cookie
  checks, 6 dp position text, platform `disabled` attribute, merge-applies-
  label proof, photo-preserving edit, preview-must-track-the-pick — none of
  which a compliant build per T-202/T-203/T-205/T-206's own freezes can fail.

## Verdict rationale

Attempt 5 does exactly what its handoff claims and nothing it shouldn't: every
residual Low/Info from the two attempt-4 PASS reviews is closed in the bytes
with strictly stronger assertions, each re-verified satisfiable against the
worker packet that owns the behaviour (T-203.md:53-64 for the auth flow,
T-205.md:55/58/65-69/89/91-92 for the UI/HTTP contract, T-202.md:79-83 and
T-206.md:67-71 for the photo pipeline, T-204.md:36-39 for the duplicate
radii); the prior blockers stay closed; the inertness mechanism, ownership,
manifest discipline and rerun-safety design are unchanged and sound; and no
existing test is weakened, skipped, deleted or special-cased. The remaining
findings are documentation-precision items, disclosed low-probability
collisions, and ratification bookkeeping — none can produce a false green or
an unsatisfiable oracle against a contract-compliant M2 build.

VERDICT: PASS
