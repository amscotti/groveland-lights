# T-201 review — migration 0002 + lights/photos store (round 2: BLOCK-resolution retry)

Review basis: the attached diff (identical to `harness/reviews/T-201.diff` on disk —
verified by grepping the on-disk file for the retry's `raw-b64:` markers) against
`harness/tasks/T-201.md` (including both retry contexts), the round-1 reviews
(`harness/reviews/T-201-reviewer.md` PASS on the feature scope, `T-201-reviewer-alt.md`
BLOCK on the checksum-encoding design), the frozen oracles
(`acceptance/go/migrations_test.go`, `acceptance/drafts/admin_lights_go_test.go.txt`,
`acceptance/contract.json`, `testdata/schema.expected.sql`), the protected/unowned store
files (`internal/store/store.go`, `store_test.go`, `dbfile_layout_test.go`,
`cmd/server/boot_test.go`), `sqlc.yaml`, `go.mod`, `Makefile`, `harness/policy.json`,
and the baseline `internal/store/migrations.go`/`migrations_test.go`.

Verification limits: the diff is **not applied** in this checkout (main still has only
`migrations/0001_init.sql`, no `queries/`, no `lights*.go`/`photos*.go`) and shell
execution is denied — same constraint as both round-1 reviews. Everything below is a
static check of the diffed bytes against the contracts I could read; executed exit codes
are trust-marked where stated. Line numbers are post-patch `file:line`.

## 1. The BLOCK and its resolution — the headline item

Reviewer-alt's block (#1) was that the round-1 runner recorded later migrations' digests
as raw BLOBs behind a `len(raw)==64` decode heuristic, inside a TEXT column, purely so
the byte-layout canary would stay green — and it explicitly sanctioned two remedies:
have the canary/pin updated by its owner, **or** "record an explicit, runner-owned
format decision whose column type and long-term contract are deliberate — then re-run
the A-104 acceptance and smoke tests." The retry implements the second remedy, in a file
the packet owns (`internal/store/migrations*.go`), and I verified each element of the
sanctioned remedy against the diff bytes:

- **Explicit, runner-owned format decision.** `sumRecordEncoding`
  (`internal/store/migrations.go:80-107`) is a documented contract, not an inference:
  the anchor row (the first embedded migration, `0001_init.sql`) records its digest as
  64-char lowercase-hex TEXT — the exact encoding A-104 freezes by name
  (`acceptance/go/migrations_test.go:15-16`: "the first migration file is
  migrations/0001_init.sql; its sha256 checksum is recorded in schema_migrations as
  64-char lowercase hex") — and every later row records
  `"raw-b64:" + base64.RawURLEncoding(digest)` (`rawB64Prefix`, `sumRecordValue`,
  migrations.go:108-115). The handoff documents the long-term contract for future
  migration authors (T-2xx adding `0003_*` needs nothing special).
- **Column type honest.** The round-1 complaint (TEXT column storing BLOBs) is gone:
  both encodings are TEXT strings, and `checkRecordedSumsAreText`
  (`internal/store/migrations_test.go`, new hunk after :356) asserts
  `typeof(sha256) = 'text'` for both rows straight from SQLite's storage class.
- **No decode heuristics.** `decodeRecordedSum` and the BLOB branch are removed (diff,
  migrations.go hunks). Verification is byte-exact positional comparison:
  `recorded != sumRecordValue(i, m)` (migrations.go:143-147); `applyOne` records
  `sumRecordValue(index, m)` (migrations.go:~207). Nothing parses a stored value.
- **The guard got strictly stronger, not weaker.** Baseline accepted any hex string
  equal to the digest; round-1 accepted hex *or* BLOB decoding to the digest; the retry
  accepts only the byte-exact positional representation.
  `TestTamperedSecondMigrationChecksumRefusesOpenNamingItsFile`
  (migrations_test.go:~466) proves with a real tampered DB that (a) a later row carrying
  the **correct** digest in bare-hex form is refused — representations are positional,
  never negotiated — and (b) a wrong digest under the right tag is refused; both wrap
  `ErrChecksumMismatch`, name `0002_lights.sql` (and assert NOT `0001_init.sql`, whose
  row is verified clean first), say "checksum mismatch", and carry both the stored value
  and the file checksum. I walked the comparison for false accepts: any byte difference
  between the stored value and the representation recomputed from the embedded file's
  digest refuses the boot naming the file. There is no byte sequence that both differs
  from the expected representation and boots.
- **The frozen A-104/smoke oracles hold by construction, verified against their actual
  bytes.** A-104's `migTamperFirstHex` rewrites `locs[0]` — the FIRST 64-char
  lowercase-hex run in the raw file (`acceptance/go/migrations_test.go:211-224`) — and
  the header (:8-12) records that the frozen index was kept because that first token IS
  the recorded checksum. A later row's value is `raw-b64:` + exactly 43 chars
  (32 bytes → 43 unpadded base64url chars; length asserted in
  `checkSecondSumIsTaggedBase64Text`) from `[A-Za-z0-9_-]`: at most 43 consecutive
  hex-able chars, bounded before by the non-hex `:` and after by the applied_at length
  varint SQLite stores between column payloads — it cannot form or extend a 64-char
  `[0-9a-f]` run. So the file's only hex-64 token is 0001's checksum; the tamper hits
  the anchor row and the refusal names `0001_init.sql` + "checksum", exactly what A-104
  (`:293-299`), the canary (`dbfile_layout_test.go:49-62`) and the boot smoke
  (`cmd/server/boot_test.go:274-293, 375-406`) require.
- **A-104 + smoke re-run as the block demanded** — claimed exit 0 for
  `make test-acceptance-go` and the smoke tag (trust-marked; the mechanism above is
  statically verified, and the handoff also reports the unowned canary green inside
  `check-fast`).

The storage asymmetry the round-1 block objected to is inherent to the frozen contract
(only 0001's encoding is pinned, by name) and is now explicit, tagged, positionally
deterministic, type-honest, and independently pinned by owned tests
(`TestChecksumRecordingIsAnchorHexPlusTaggedBase64` verifies the tag as a literal —
not the production constant — the 43-char payload, valid unpadded base64url, and digest
equality against an independently recomputed sha256, plus byte-stability across
reopen). Legacy DBs written by the two never-merged attempt binaries refuse at boot by
design — correct behaviour of the guard against foreign representations, disclosed in
the handoff, and no such DB can exist in production (M2 never shipped; real M1 DBs hold
only 0001's hex row and upgrade cleanly, proven by `TestMigrationsUpgradeWithData`).

## 2. Round-1 secondary findings #2–#6 — all closed, verified in the diff bytes

- **#2 (assertion could not fail)** — closed. `checkEditUpdatesColumns` +
  `backdateLightStamps` (`internal/store/lights_test.go:~651-685`) set
  `created_at`/`updated_at` to `2020-01-01T00:00:00Z` before the edit, then assert
  `created_at` stayed backdated, `updated_at` moved strictly past it, and the persisted
  row carries the stamped value (`checkEditedPersisted`). An `EditLight` that skips the
  re-stamp now fails at second precision.
- **#3 (conditional assertion)** — closed. `checkNearbyOutsideRows`
  (lights_test.go:~583-625) asserts the 8.1 m / 25.1 m candidates' presence in the
  prefilter with unconditional `Fatalf` before the Haversine checks; no `if seen {}`
  wrapping remains.
- **#4 (tautological oracle)** — closed. `checkBBox` (lights_test.go:~878-915) pins
  `metersPerDegree` against the independent mean-Earth figure
  (`testMeanEarthRadiusM = 6371008.8`, 0.5% band — arithmetic checks out: 6371008.8·π/180
  ≈ 111194.93 vs the constant 111194.9) and `checkBBoxBoundaryRows` places 99%-of-radius
  points (must be inside) and 135% points (beyond the 110% safety, must be outside)
  using only the independent figure. `offsetNorth`/`offsetEast` and all duplicate seeds
  now use the test-side figure, so seeding no longer shares the store's constants. I
  verified the boundary arithmetic at Groveland's latitude and at 89.9°: in/out
  classifications hold with margin.
- **#5 (non-specific error)** — closed. `checkCreateLightInvalidKind`
  (lights_test.go:~239-258) asserts `errors.Is(err, errInvalidKind)` and that `lights`
  still holds zero rows.
- **#6 (handoff precision)** — closed. The new handoff states
  `dbfile_layout_test.go` is not harness-protected but not owned by this task; the
  seeds are now genuinely independent; `TestSchemaMatchesExpected` compares the `probe`
  DDL (only runner-owned `schema_migrations` is excluded — correct, since its stored DDL
  carries `IF NOT EXISTS`, which the oracle deliberately omits).

## 3. Deliverables re-verified (feature scope, unchanged from `93762ff`)

1. **`migrations/0002_lights.sql`** — statement-by-statement identical to
   `testdata/schema.expected.sql:5-86` (users, photos, lights, `lights_geo`, partial
   unique `lights_asset_no` with `WHERE asset_no IS NOT NULL AND deleted_at IS NULL` —
   the A-208 asset-freeing semantics, reports + 2 time indexes, comments, comment_flags,
   sessions, login_tokens), modulo whitespace. Plain CREATEs, no `IF NOT EXISTS`, no
   comments; `0001_init.sql` absent from the diff (append-only respected).
2. **Queries + gen** — `internal/store/queries/{lights,photos}.sql` cover every fixed
   capability; every read and write filters `deleted_at IS NULL`; list order
   `created_at DESC, id DESC`. The committed gen output is internally consistent with
   the query files (narg→`sql.Null*`, positional `?N`, RETURNING scans, the
   file-header comments attached to each file's first query in the Querier — same
   pattern as baseline `gen/querier.go:12-14`), headers say `sqlc v1.31.1`, and
   `sqlc.yaml` needs no change. The `BETWEEN`→`>=`/`<=` rewrite is a sound workaround
   for the sqlc parser bug and is flagged for T-203 in open issues. Regeneration itself
   trust-marked (generate-check claimed 0; the gate diffs it).
3. **Typed wrappers** — all packet-mandated methods with the mandated signatures;
   `LightID`/`PhotoID`/`Kind`+`ParseKind` (exact lowercase only, table-tested incl.
   capitalised/padded/empty); `ErrNotFound` for missing AND soft-deleted on every
   read/edit/delete; `ErrDuplicateAssetNo` on create and edit; negative
   radius/limit/offset refused; `%w` wrapping (double-`%w` chains fine on go 1.27);
   `context.Context` first. `CreateLight` validates the kind before any write
   (parse-don't-validate at the store boundary).
4. **Tests** — all real temp-file SQLite through `openFresh` (real migrations; no
   mocks), raw-driver tampering for the checksum/upgrade cases, `fstest.MapFS` for the
   0001-only legacy DB. `TestSchemaMatchesExpected` normalises both sides and compares
   sorted sets (auto-indexes correctly excluded by the `sqlite_%` name filter; the
   probe DDL correctly included). `TestMigrationsUpgradeWithData` asserts data
   survival, both versions recorded once, `foreign_key_check` empty, `integrity_check`
   ok, product schema usable. The A-205 store half seeds the drafts' fixture point
   (42.7617, -71.0303, matching `admin_lights_go_test.go.txt:163`) at 7.9/8.1 m and
   24.9/25.1 m around the 8 m/25 m radii, plus 150 m far and a soft-deleted 5 m
   fixture, with membership classified at both radii. The A-208 half hides a deleted
   light from all four read paths, proves soft-not-hard delete, frees the asset_no,
   refuses double delete. Photos: round-trip, UNIQUE path + row count, ErrNotFound,
   dangling-FK refusal, light→photo reference, id-order list.

Baseline-test compatibility checked against the new encoding: the unchanged
`checkRecordedChecksum` targets `version = '0001_init'` (still hex) ✓;
`TestMigrationChecksumGuard`'s WHERE-less tamper UPDATE now hits both rows but 0001 is
verified first in the sorted loop, so the refusal names `0001_init.sql` as asserted ✓;
`TestRecordedVersionWithoutEmbeddedFileRefusesOpen`'s `0002_ghost` still lands in the
ghost-version branch, not a checksum mismatch ✓; `checkSyntheticFileChecksums` now
expects the per-position representations (index 0 hex, index 1 tagged) recomputed from
the synthetic file bytes with literals ✓.

## 4. Security pass

Store-only task — no handlers/templates/JS/uploads, so CSRF/XSS/`#map`/upload-cap
concerns are N/A. SQL: everything is sqlc-generated or test-side bound parameters; no
string-interpolated SQL anywhere in the diff. No `template.HTML`. No filesystem access
in the store layer — the photo `path` column is stored verbatim and never opened, so no
path-traversal vector is introduced here (minting/validating paths is T-202/T-206).
No secrets in output: refusal messages carry migration digests and file names —
bookkeeping, not secrets. `go.mod` untouched; `encoding/base64` is stdlib
(`allow_deps: []` respected).

## 5. Architecture pass (docs/spec/architecture.md)

- **Ownership map**: every touched path matches the owns globs
  (`migrations/**`, `internal/store/queries/**`, `gen/**`, `lights*.go`, `photos*.go`,
  `migrations*.go`) plus the sanctioned handoff. `store.go`, `store_test.go`,
  `dbfile_layout_test.go`, `acceptance/**`, `harness/**` (except the handoff),
  Makefile, goldens: absent from the diff, verified file-by-file.
  `harness/policy.json:37-51` protected list untouched.
- **Typed IDs / parse-don't-validate**: `LightID`, `PhotoID`, `ParseKind`,
  `NewLight` pointer-NULLs — the boundary-parser rule applied at the store's edge.
- **Status derivation**: status columns are read verbatim; no derivation logic added —
  `status.Derive` remains the only path (architecture.md:24), untouched.
- **HTMX/#map**: no web files touched; N/A by scope.
- Hygiene: no TODO/FIXME/`panic(`/`t.Skip`/`.only`/`.fixme`/unexplained `//nolint`/
  `console.log`/"Phase N" in the added code bytes (the only regex hits are the
  handoff .md quoting their absence); the strengthened helpers split to stay under the
  cyclop budget is the codebase's existing pattern.

## 6. Residual findings (none blocking)

1. **[Low] The hex anchor is positional (index 0), while A-104 pins `0001_init.sql` by
   name** (`sumRecordValue`, migrations.go:~110-115). The two are equivalent only while
   nothing sorts before `0001_init.sql` — guaranteed by the append-only migration rule
   and by the fact that A-104's own contract sentence would need rewriting otherwise.
   There is a tripwire: `checkFirstSumIsHexText` queries `WHERE version = '0001_init'`
   and would fail loudly if a lower-sorting file ever shifted the anchor. A future
   runner task could pin by version name instead of index; not worth blocking.
2. **[Info] `TestDBFileHexTokenIsExactlyTheFirstMigrationChecksum`** deliberately
   duplicates the unowned canary's invariant in an owned file. That is an additional
   local regression tripwire ("any code writing another hex TEXT token fails here"),
   not a weakening — the canary itself is untouched and still runs.
3. **[Info] The disclosed `test-run.db` repair** (temporary test appended to an owned
   test file, run to rewrite one stale row, removed before commit) is unconventional,
   but it is honestly disclosed, the committed tree never carried it, and it is the
   moral equivalent of the README-sanctioned `rm -f test-run.db*`, which the session's
   shell restrictions made unavailable. No repo rule governs gitignored local state;
   the guard's clean-worktree check is on the committed tree.
4. **[Info] `NearbyLights` performs no lat/lon range validation** (|lat|>90, ±Inf,
   antimeridian wrap of the lon box). Per the packet ("just return the rows"),
   coordinate validation belongs to the HTTP/geo tasks (T-204/T-205, TOWN_BBOX);
   NaN fails loudly against the NOT NULL columns. The superset contract that matters is
   pinned (`checkBBox` + near-boundary rows).
5. **[Info] `isDuplicateAssetNo` string-matches the driver's error text** (two
   spellings: index name and table.column). Pragmatic — database/sql exposes no
   portable SQLite error code — and the sentinel is exercised on both create and edit;
   a driver message-format change would surface as 500s, not silent accepts.
6. **[Info] Trust-marked execution claims**: `check-fast`, `generate-check`,
   `test-acceptance-go`, smoke, `e2e-task` no-op (confirmed: `Makefile:111-116` greps
   claims and finds none for T-201 → `--pass-with-no-tests`; the packet expects exactly
   this), and the commit hooks. Static consistency supports them: no helper-name
   collisions with `store_test.go`, imports are baseline+stdlib, the module path
   matches. The handoff's open issue #3 (`e2e-task` is spelled `TASK=` in the Makefile,
   AGENTS.md says `T=`) is correct — verified at Makefile:8,113.

## Verdict rationale

The round-1 BLOCK was about *how* the encoding conflict was resolved: an undocumented
length heuristic, a BLOB stored in a TEXT column, and a migration-1 special case whose
only stated rationale was the canary. The retry implements precisely the remedy that
block sanctioned, in the file this task owns: an explicit, runner-owned, documented
encoding contract with an honest column type (typeof asserted), no decode path at all,
and byte-exact positional verification that is *stricter* than both the baseline and
the blocked attempt — proven against a real tampered database, including the
correct-digest-wrong-encoding case. The frozen A-104 anchor contract, the canary and
the boot smoke now hold by construction rather than by page-layout luck, and the A-104
acceptance + smoke suites were re-run as demanded. All four secondary findings are
closed with strictly stronger, mutation-sensitive assertions, and the feature scope —
already verified sound by both round-1 reviews — is unchanged: the migration is
statement-identical to the frozen oracle, the query set covers the fixed capabilities,
the wrapper API matches the mandated signatures with typed IDs and sentinels, and every
test runs against a real migrated SQLite database. No protected path is touched, no
dependency is added, no test is weakened. Residual findings are design notes and
trust-marked execution claims.

VERDICT: PASS
