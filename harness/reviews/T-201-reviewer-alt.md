# T-201 — independent review (reviewer-alt, retry): migration 0002 + lights/photos store

Basis: `harness/reviews/T-201.diff` (the retry diff supplied with this review),
`harness/tasks/T-201.md` (packet + retry context + the prior reviewer-alt review),
`AGENTS.md`, `acceptance/contract.json` (A-104/A-205/A-208), the activated
A-104 Go spec (`acceptance/go/migrations_test.go`), the inert A-204..A-209 draft
(`acceptance/drafts/admin_lights_go_test.go.txt`), `testdata/schema.expected.sql`,
`internal/store/dbfile_layout_test.go`, `cmd/server/boot_test.go`,
`harness/policy.json`, and the baseline runner/store tests on disk.

Verification limit: the diff is **not applied** in this checkout (`migrations/`
holds only `0001_init.sql`; there is no `internal/store/lights.go`/`photos.go`/
`queries/`), and shell execution is denied in this session, so this is a static
review of the diffed bytes. The handoff's executed results (`check-fast`,
`generate-check`, A-104, smoke, e2e no-op) are taken on trust and marked as such
below. Line numbers are **post-patch, derived from the diff**, since the files
are not on disk in post-patch form.

## Findings

### 1. [Low — residual test-shaped asymmetry; accepted under the prior review's sanctioned remedy]

`internal/store/migrations.go:80-116` (`sumRecordEncoding` comment, `rawB64Prefix`,
`sumRecordValue`) keeps a deliberate split: the migration at index 0 records its
digest as 64-char lowercase hex, and every later migration records
`"raw-b64:" + base64url(raw digest)`. That split has no product need; it exists so
the protected byte-scanning oracles keep working — `acceptance/go/migrations_test.go:211-224`
(tampers the first hex token), `internal/store/dbfile_layout_test.go:49-62`
(requires exactly one hex-64 token), and `cmd/server/boot_test.go:274-296`
(tampers the first token). This is the same structural concern the previous
reviewer-alt raised, and it remains visible in the production encoding.

What changed in the retry is material, and it satisfies the prior review's
alternative remedy (b):
- the stored values are **TEXT**, matching the declared `TEXT NOT NULL` column
  (`migrations.go:70-75`; pinned by `typeof(sha256) = 'text'` in
  `migrations_test.go:369`, `checkRecordedSumsAreText`);
- there is **no decoder**: `applyMigrations` compares the recorded bytes against
  `sumRecordValue(i, m)` (`migrations.go:136-150`); the old length/heuristic
  acceptance path is gone;
- the comparison is **stricter** than the old one: a later row carrying the correct
  digest in bare hex is now refused as a foreign encoding
  (`migrations_test.go:518`, `TestTamperedSecondMigrationChecksumRefusesOpenNamingItsFile`),
  and a wrong digest under the right tag is refused;
- the anchor's hex encoding is still the frozen A-104 contract
  (`acceptance/go/migrations_test.go:14-18`), and `TestMigrationsApplyCleanly`'s
  `recorded_checksum_is_64_char_lowercase_hex_of_file` is unchanged.

Residual risk to record, not a test-integrity failure: any future tooling that
assumes `schema_migrations.sha256` is a bare hex digest will see tagged base64 for
rows ≥ 2, and old attempt-era DBs (bare hex or BLOB for `0002_lights`) refuse at
boot by design (disclosed in the handoff). This is now an explicit runner-owned
contract, not an implicit length heuristic.

### 2. [Low — the runner-owned bookkeeping DDL is not pinned by the schema oracle]

`internal/store/migrations_test.go:895` (`TestSchemaMatchesExpected`) compares
`sqlite_master.sql` to `testdata/schema.expected.sql` but adds
`AND name <> 'schema_migrations'`, and reads only `sql` (not `type` as the packet
suggests). The oracle has no `schema_migrations` statement, so the exclusion is
necessary to match the frozen file — but it means the table whose `sha256 TEXT
NOT NULL` declaration the new encoding contract depends on is never compared.
`checkRecordedSumsAreText` pins the *storage class* of values, not the declared
DDL. A future edit that dropped `NOT NULL`/changed the column would not be caught
by the schema test. Recommend a small explicit DDL/column assertion for the
runner-owned table (or a follow-up task that owns the oracle).

### 3. [Low — the new DB-file hex-token test inherits the canary's substring blind spot]

`internal/store/migrations_test.go:599`
(`TestDBFileHexTokenIsExactlyTheFirstMigrationChecksum`) asserts
`len(locs) == 1` over `regexp.MustCompile("[0-9a-f]{64}")`. `FindAllIndex` returns
non-overlapping 64-char matches: a hypothetical 65+-char lowercase-hex run would
still yield one 64-char match equal to the recorded checksum and pass, so the
comment's "exactly ONE 64-char lowercase-hex token" overstates what is proven.
The protected canary (`dbfile_layout_test.go:59-62`) has the same limitation, and
the new base64 encoding makes a longer run unreachable in practice (43-char
payload, non-hex tag), so this is a robustness nit — but the new test claims an
invariant slightly stronger than it checks. It also duplicates the unowned
canary inside the owned suite, so an owner-side canary update must be mirrored
here.

### 4. [Low — prefilter distance check uses recomputed seed coordinates, not the persisted rows]

`internal/store/lights_test.go:603-635` (`checkNearbyOutsideRows`) verifies that
the 8.1 m / 25.1 m candidate IDs appear in the prefilter (good, unconditional now),
then computes the Haversine distance from `offsetNorth`/`offsetEast` outputs
rather than reading each seeded row's stored `Lat`/`Lon`. The membership check by
ID pins the prefilter, and the seeds are built from the same helpers, so this is
low risk; reading the rows back would make the distance assertion independent of
the seeding helper as well.

### 5. [Low — two refusal tests assert only "an error occurred"]

`internal/store/lights_test.go:267` (`checkCreateLightPhotoReferenceFK`) accepts
any error for the dangling `photo_id` (no FK sentinel is defined, so this is
defensible), and `internal/store/lights_test.go:460`
(`checkRecentRejectsNegative`) plus the `negative_radius_is_refused` subtest only
assert `err != nil`. These match contract-level "must refuse" wording, but they
cannot distinguish the intended validation error from an unrelated failure. The
invalid-kind case was correctly tightened to `errors.Is(err, errInvalidKind)` plus
a zero-row check (`lights_test.go:250`), so the pattern is already available.

### 6. [Info — verification limits and handoff precision]

- The diff is not applied in this checkout and shell is denied, so the exit codes
  in the handoff's verify table and the sqlc regeneration identity are
  trust-marked. Static consistency checks (schema vs oracle, queries vs generated
  code, test references vs helpers) pass.
- The handoff discloses that `test-run.db` was repaired by a temporary test that
  rewrote the `0002_lights` row to the tagged-base64 value and was then removed
  (`git status` shows no scratch test in the committed tree). That is a
  disclosed environment repair, not a committed test hack; in a clean environment
  the sanctioned `rm -f test-run.db*` is still the right action.
- The handoff's `make e2e-task TASK=T-201` spelling differs from AGENTS.md's
  `T=` shorthand; the Makefile target's variable is `TASK`, so this is a doc
  mismatch, not a code defect.
- `dbfile_layout_test.go` is not in `harness/policy.json`'s protected list
  (policy lines 37-51) but is outside T-201's `owns` globs — the handoff now
  states this correctly.

## Checked and found sound

- **No test was weakened, deleted, skipped, or narrowed.** Every modified owned
  assertion is at least as strong: the table set is pinned to all ten tables
  (was two), recorded versions to `0001_init,0002_lights` (was one), the reopen
  row count to two (was one), and `checkSyntheticFileChecksums`
  (`migrations_test.go:840`) still compares each recorded value to a
  representation independently recomputed from the synthetic file bytes (hex for
  index 0, literal `raw-b64:`+base64url for index 1). No `t.Skip`/`.only`/`//nolint`
  in the added bytes.
- **The prior review's secondary findings are genuinely closed.** `EditLight`'s
  test backdates `created_at`/`updated_at` to 2020 and asserts `created_at` stays
  put, `updated_at` advances, and the persisted row carries the new stamp
  (`lights_test.go:673` and helpers); `checkNearbyOutsideRows` now asserts the
  8.1 m/25.1 m candidates' presence with `t.Fatalf` (no `if seen {}`); `checkBBox`
  pins `metersPerDegree` against an independent mean-Earth figure and places
  near-boundary rows with the test-side figure (`lights_test.go:946,992`);
  invalid kind asserts `errors.Is(err, errInvalidKind)` and zero rows
  (`lights_test.go:250`).
- **The checksum guard is stronger, not weaker.** `TestMigrationChecksumGuard`
  (unchanged) still proves a corrupted anchor refuses and names `0001_init.sql`;
  `TestTamperedSecondMigrationChecksumRefusesOpenNamingItsFile`
  (`migrations_test.go:518`) proves a later row is guarded too, including
  rejecting the correct digest in the foreign hex encoding and a wrong digest
  under the right tag, while naming `0002_lights.sql` (not `0001_init.sql`);
  `TestRecordedVersionWithoutEmbeddedFileRefusesOpen` still catches ghost
  versions; `TestChecksumRecordingIsAnchorHexPlusTaggedBase64`
  (`migrations_test.go:369`) independently recomputes both representations, pins
  TEXT storage, and proves byte-stability across close/reopen.
- **Schema fidelity.** `migrations/0002_lights.sql:1-82` is statement-identical
  (modulo whitespace) to the product part of `testdata/schema.expected.sql:5-86`
  — users, photos, lights, `lights_geo`, the partial-unique `lights_asset_no`,
  reports plus both time indexes, comments, comment_flags, sessions, login_tokens.
  Plain `CREATE`s, no `IF NOT EXISTS`; `0001_init.sql` untouched; append-only.
- **Queries/generated code.** `internal/store/queries/{lights,photos}.sql` cover
  every packet capability; every read/edit/delete filters `deleted_at IS NULL`;
  list order is `created_at DESC, id DESC`; the bbox uses `>=`/`<=` pairs; the
  committed `gen/` files match the queries and the `sqlc v1.31.1` baseline shape.
- **Typed wrappers.** Signatures, typed `LightID`/`PhotoID`, `Kind`/`ParseKind`
  (exact lowercase only), `NewLight` pointer NULLs, `ErrNotFound` on missing and
  soft-deleted rows, `ErrDuplicateAssetNo` on create/edit, RFC3339 stamping, and
  the 1.10 bbox over-cover are present and tested. Soft delete stamps
  `deleted_at`, hides the row from all four read paths, and frees `asset_no` via
  the partial unique index.
- **Tests are real, not mocks.** All store tests use a temp-file SQLite DB opened
  through `Open`/`openFresh` with real migrations; tamper and upgrade tests use
  the raw `database/sql` driver and a `fstest.MapFS` filtered to 0001 for the
  legacy upgrade path — exactly the packet's prescribed approach, not a
  behaviour-bypassing mock.
- **No fixture literals in production.** `GP-017`, `42.7617`, `-71.0303`,
  "Ball field lamp", "Path light", etc. appear only in `*_test.go`/the handoff.
  `internal/store/lights.go` has only the physical bbox constants;
  `migrations.go` has only the encoding tag.
- **Scope, deps, hygiene.** Touched paths are all inside the `owns` globs plus the
  sanctioned handoff; no `acceptance/**`, Makefile, policy, golden, or
  `testdata/schema.expected.sql` change; no new module dependency; no
  TODO/FIXME/panic/"Phase N".

## Verdict rationale

The feature half is well built and genuinely tested: the schema matches the
frozen oracle statement-for-statement, the upgrade path preserves data and passes
FK/integrity checks, and the duplicate/soft-delete/photo tests use real rows and
mutation-sensitive assertions. The retry does what it claims for the prior BLOCK:
it replaces the earlier BLOB/length-heuristic encoding with an explicit,
runner-owned TEXT contract, removes all decode heuristics, verifies by
byte-exact comparison (rejecting a later row's correct digest in a foreign
encoding), and adds tests that pin the contract and the guard. No test was
weakened or skipped, and every changed assertion is stronger. The remaining
findings are coverage/robustness notes (runner DDL not in the oracle;
`FindAllIndex` substring blind spot; distance recomputation; non-specific refusal
errors) and trust-marked execution claims, none of which undermines the
contract. The residual test-shaped origin of the encoding is now an explicit,
documented, verified contract — the resolution the prior review sanctioned.

VERDICT: PASS
