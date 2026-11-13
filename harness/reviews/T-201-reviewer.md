# T-201 review — migration 0002 (product schema) + lights/photos store (retry)

Review basis: the attached diff (`harness/reviews/T-201.diff`, identical to the diff
supplied with this request) against `harness/tasks/T-201.md` (including the retry
context), `docs/spec/architecture.md`, `docs/PLAN.md` contract sentences, the frozen
oracles (`acceptance/go/migrations_test.go`, `acceptance/drafts/admin_lights_go_test.go.txt`,
`testdata/schema.expected.sql`), the protected store files
(`internal/store/store.go`, `store_test.go`, `dbfile_layout_test.go`,
`cmd/server/boot_test.go`), `sqlc.yaml`, `go.mod`, the baseline
`internal/store/migrations.go` / `migrations_test.go`, and the Makefile targets the
handoff claims to have run.

Verification limits: the diff is **not applied** in this checkout (main has only
`0001_init.sql`, no `queries/`, no `lights*.go`/`photos*.go`) and shell execution is
restricted, so `make check-fast` / `generate-check` exit codes and sqlc regeneration
are taken on trust where marked — the same constraint as every prior round (see
T-200-reviewer.md:14-25). Everything below is a static check of the diffed bytes
against the packets and repo facts I could read; the reasoning about the one
non-trivial design change (checksum recording encoding) is verified against the
actual frozen test bytes, not the handoff's summary of them.

## The retry fix — checksum recording encoding (the headline item)

**The blocker was real.** Three independent protected oracles all pin "the FIRST (and
only) 64-char lowercase-hex token in the raw DB file is 0001_init's recorded
checksum":

- `acceptance/go/migrations_test.go:211-224` (`migTamperFirstHex` rewrites `locs[0]`)
  + `:293-295` (refusal output must name `0001_init.sql`); frozen contract text at
  `:14-18`.
- `internal/store/dbfile_layout_test.go:49-62` — requires `len(locs) == 1`, exactly
  one hex-64 token in the file.
- `cmd/server/boot_test.go:42,276-282,405` — smoke tamper of the first hex token,
  refusal must name `0001_init.sql`.

Adding ANY second migration recorded as hex TEXT produces a second hex-64 token → the
layout canary fails unconditionally (`len(locs) != 1`), and because SQLite allocates
leaf-cell content downward from the page end, the later-inserted row (0002)
typically sits at the LOWER byte offset, so the byte-tamper hits 0002's checksum and
the refusal names `0002_lights.sql` — failing two more frozen oracles. No fix inside
T-201's owns globs other than changing the recording encoding exists: the three
oracle files are outside ownership (`dbfile_layout_test.go` is named NOT-ours at
T-201.md:87-88; acceptance/** is hash-pinned in `harness/protected.sha256:5`).

**The fix is scoped to exactly what the frozen contract pins.** The A-104 contract
sentence (acceptance/go/migrations_test.go:15-16) freezes hex-TEXT recording for
*0001_init.sql only*: "the first migration file is migrations/0001_init.sql; its
sha256 checksum is recorded in schema_migrations as 64-char lowercase hex". The diff
keeps that row byte-identical in behavior (`checksumRecordValue(first=true)` → hex
TEXT, internal/store/migrations.go, new hunk @@ -74,6 +76,39 @@) and records every
later migration's digest as the raw 32-byte BLOB — 32 binary bytes cannot contain a
64-char ASCII-hex run, so the one-token invariant now holds **by construction** for
any future migration count, which the layout test's own header
(dbfile_layout_test.go:19-22) invites the migration's task to make conscious.

**The guard did not weaken — checked exhaustively for false accepts.**
`decodeRecordedSum` maps both encodings to the hex digest string that
`applyMigrations` compares against the file's digest: intact row (either encoding) →
equal → boot; 32-byte tamper → different hex → refuse; 64-byte value → taken as the
hex string → ≠ digest unless it literally IS the digest (i.e. the correct value in
the old encoding); any other length → hex ≠ digest → refuse. There is no byte
sequence a tamperer can write that both differs from the true digest and boots.
Corroboration that the BLOB row is still guarded is *new* test
`TestTamperedSecondMigrationChecksumRefusesOpenNamingItsFile` (corrupts 0002's stored
bytes → refusal wraps `ErrChecksumMismatch`, names `0002_lights.sql`, carries both
digests), and the unchanged `TestMigrationChecksumGuard` still proves the TEXT row's
guard, including the uppercase and replaced-hex cases whose tampered values must
appear verbatim in the refusal message. The ghost-version branch
(`TestRecordedVersionWithoutEmbeddedFileRefusesOpen`) is untouched by the encoding
change and still behaves (the ghost row is reported as no-embedded-file, not a
checksum mismatch).

**No test was weakened, skipped, deleted or special-cased.** Every modified owned
assertion got strictly stronger: `checkTablesAreExactlyTheMigratedSchema` demands the
exact 10-table set (was 2); `checkRecordedVersions` requires both versions in order
(was one); the reopen count pins exactly 2 (was 1); `checkSyntheticFileChecksums`
now decodes both encodings but still requires equality with sha256 of the file
bytes. `TestMigrationsApplyCleanly`'s `recorded_checksum_is_64_char_lowercase_hex_of_file`
subtest is unchanged and still pins 0001's row as hex TEXT. No `t.Skip`/`.only`/
`//nolint` anywhere in the diff; new behaviour all has new tests.

## Deliverable-by-deliverable verification

1. **`migrations/0002_lights.sql`** — compared statement-by-statement against
   `testdata/schema.expected.sql:5-86` (product part): users, photos, lights,
   `lights_geo`, partial-unique `lights_asset_no` (`WHERE asset_no IS NOT NULL AND
   deleted_at IS NULL` — the A-208 asset-freeing semantics), reports + 2 time
   indexes, comments, comment_flags, sessions, login_tokens — identical modulo
   whitespace. Plain CREATEs, no `IF NOT EXISTS`, no comments, `0001_init.sql`
   untouched (not in the diff). Append-only respected.
2. **Queries + gen** — `internal/store/queries/{lights,photos}.sql` carry every
   capability the packet fixes (CreateLight with NULL-able asset_no/photo_id/
   created_by/accuracy_m; GetLightByAssetNo/ListRecentLights/NearbyLights/Update/
   SoftDelete all filter `deleted_at IS NULL`; `ORDER BY created_at DESC, id DESC`
   matches the frozen A-209 tie-break). The committed gen files are internally
   consistent with the query files (narg→sql.Null* params, positional ?N, RETURNING
   scans, alphabetised Querier with the file-header comments attached to each file's
   first query — same pattern as baseline `gen/querier.go:12-14`), headers say
   `sqlc v1.31.1` matching all four baseline gen files, and `sqlc.yaml` (schema
   `migrations`, queries `internal/store/queries`, out `internal/store/gen`,
   `emit_interface: true`) needs no change. Regeneration itself is trust-marked
   (generate-check exit 0 claimed; the gate diffs it).
3. **Typed wrappers** — every packet-mandated method present with the packet's
   signatures: `CreateLight/GetLight/LightByAssetNo/RecentLights(limit,offset)/
   EditLight/SoftDeleteLight/NearbyLights(lat,lon,radiusM)/CreatePhoto/PhotoByID`;
   `LightID`/`PhotoID`/`Kind` + `ParseKind` (exact lowercase match only — tested
   against capitalised/padded/empty/unknown); `ErrNotFound` mapping for
   missing AND soft-deleted ids on every read/edit/delete; `ErrDuplicateAssetNo` on
   create and edit. `bboxDegrees` uses a 1.10 over-cover factor — the bbox is a
   strict superset of the radius disk (pinned by `TestBBoxDegrees` and by the
   8.1 m/25.1 m rows appearing in results while their Haversine distance is above
   the radius), which is exactly the packet's "prefilter, exact distance is
   internal/geo's" division of labour. Negative radius/limit/offset refused.
   `%w` wrapping (incl. double-`%w` sentinel chains, fine on go 1.27), `context`
   first everywhere.
4. **Tests** — all real temp-file SQLite through `openFresh` (real migrations
   applied; no mocks anywhere). `TestSchemaMatchesExpected` normalises both sides
   (comment strip, whitespace collapse, sorted set) and compares against the frozen
   oracle; auto-indexes (`sqlite_autoindex_%`) are correctly excluded by the
   `NOT LIKE 'sqlite_%'` name filter and NULL `sql`. `TestMigrationsUpgradeWithData`
   applies 0001 alone via a filtered MapFS, seeds probe data, full-opens, asserts
   survival + both versions + `foreign_key_check` empty + `integrity_check` ok +
   product tables usable. The A-205 store half seeds Haversine-derived offsets at
   7.9/8.1 m and 24.9/25.1 m around the drafts' fixture point (42.7617,-71.0303,
   matching acceptance/drafts/admin_lights_go_test.go.txt:163), plus 150 m far and a
   soft-deleted 5 m fixture, and classifies membership at both radii — exactly the
   packet's table (T-201.md:76-78). The A-208 half hides a deleted light from all
   four read paths, proves soft-not-hard delete by reading `deleted_at` back
   non-NULL, frees the asset_no for reuse (partial unique index semantics), and
   refuses a second delete. Photos: round-trip, UNIQUE path, FK dangling refusal,
   id-order list.

## Security pass

Store-only task — no handlers, templates, JS or uploads, so CSRF/XSS/`#map` swap/
upload caps are N/A here. SQL: 100% sqlc-generated bound parameters; the few raw
`db.sql` statements in tests use bound params. No `template.HTML`. No secrets, no
log output of sensitive values (error messages carry labels/asset numbers/ids and
the *migration* digests — bookkeeping, not secrets). No filesystem access at all in
the store layer, so no path-traversal vector is introduced (the photo `path` column
is stored verbatim; minting/validating/serving paths is T-202/T-206 territory, and
the store never opens them). No new module deps (`allow_deps: []` respected — diff
touches no go.mod/go.sum).

## Architecture pass

Ownership: every touched path matches the owns globs (`migrations/**`,
`internal/store/queries/**`, `internal/store/gen/**`, `internal/store/lights*.go`,
`photos*.go`, `migrations*.go`) plus the sanctioned handoff.
`store.go`/`store_test.go`/`dbfile_layout_test.go`/`session*`/`user*`/`acceptance/**`/
Makefile/goldens: absent from the diff (verified file-by-file). Typed IDs
(`LightID`, `PhotoID`) with boundary parsing (`ParseKind`, `NewLight` pointer-NULLs)
— the parse-don't-validate rule applied at the store's boundary. Status columns are
read verbatim only; derivation stays behind `status.Derive` (untouched, per the
ownership map at docs/spec/architecture.md:24). No "Phase N" strings, no
TODO/FIXME/panic, no `console.log` (no JS). The migration runner remains the single
`schema_migrations` owner (docs/spec/architecture.md:18) — the encoding change is
inside that ownership.

## Residual findings (none blocking)

1. **[Low] `schema_migrations.sha256` is declared `TEXT NOT NULL`
   (internal/store/migrations.go:70-75) but now stores raw BLOBs for every
   migration after the first.** SQLite's dynamic typing keeps this functional
   (TEXT affinity does not convert BLOB bindings) and both encodings decode
   correctly, but the column declaration no longer describes its contents, and
   tooling that assumes TEXT (ORMs, `sqlite3` CLI rendering) will see blobs on the
   later rows. Deliberate, loudly documented in code comments and the handoff, and
   the only ownership-respecting way to keep the three frozen first-token oracles
   true with ≥2 migrations; a future task that owns a migration could consider
   amending the runner-owned DDL comment/column docs. Not a guard-weakening (see
   analysis above).
2. **[Low] `checkNearbyOutsideRows` (internal/store/lights_test.go, diff) only
   asserts distance when the row was seen** (`if seen8 { ... }`). Soft, but it is
   supplementary: membership of the 8.1 m/25.1 m rows is already hard-asserted by
   the two `checkNearby` subtests' `wantIn` lists, so no false green is possible.
3. **[Info] `NearbyLights` does not range-check lat/lon** (|lat|>90, Inf). NaN REALs
   become NULL in SQLite and fail the NOT NULL constraint loudly; the bbox is an
   internal prefilter whose callers (T-204/T-205) own coordinate validation per the
   packet ("just return the rows"). Acceptable.
4. **[Info] `EditLight`'s `assetNo == nil` means "clear"** — overloading documented
   and tested (`checkEditClearsAssetNo`); the HTTP layer implements A-208's
   "unspecified fields keep values" via read-modify-write (T-205's concern).
5. **[Info] `isDuplicateAssetNo` matches driver error-message text** (two spellings:
   index name and table.column). Pragmatic — database/sql exposes no portable
   SQLite error code — and both spellings are tested; a driver upgrade changing the
   format would surface as 500s not silent accepts.
6. **[Info] Unverifiable in this session:** executed exit codes (`make check-fast`,
   `generate-check`, `test-acceptance-go`, smoke, `e2e-task` no-op) and the sqlc
   regeneration identity. Static consistency checks pass (Makefile targets exist at
   Makefile:68-136; the claimed red→green mechanism for
   `TestChecksumIsFirstAndOnlyHexTokenInDBFile`, A-104
   `TestA104RefusesTamperedChecksum` and `TestBootSmokeRefusesTamperedChecksum`
   follows from the encoding analysis above).

## Verdict rationale

The retry does what its handoff claims and nothing it shouldn't. The prior blocker
was real and the fix is the minimal within-ownership resolution: it preserves every
frozen contract byte (0001's row stays 64-char lowercase-hex TEXT exactly as A-104
worded it), makes the first-and-only-hex-token invariant hold by construction for
all future migrations instead of by page-layout luck, keeps the boot guard
equally strong for both encodings (proven by new tamper tests plus the unchanged
guard tests), and strengthens every owned assertion it touched. All four
deliverables match the packet precisely — the migration is byte-equivalent to the
frozen oracle, the query set covers the fixed capabilities, the wrapper API matches
the mandated signatures with typed IDs and sentinels, and the tests exercise real
databases at Haversine-derived offsets exactly as the packet dictates. No protected
path is touched, no dependency added, no test weakened. Residual findings are
design-notes and trust-marked execution claims.

VERDICT: PASS
