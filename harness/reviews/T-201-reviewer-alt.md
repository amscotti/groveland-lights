# T-201 — independent review (reviewer-alt): migration 0002 + lights/photos store

Basis: the attached diff (`harness/reviews/T-201.diff`), packet
`harness/tasks/T-201.md`, `AGENTS.md`, `acceptance/contract.json` (A-104/A-205/
A-208), the activated A-104 Go spec (`acceptance/go/migrations_test.go`), the
inert A-204..A-209 Go draft (`acceptance/drafts/admin_lights_go_test.go.txt`),
`testdata/schema.expected.sql`, `internal/store/dbfile_layout_test.go`,
`harness/policy.json`, and the existing runner/store tests in the checkout.

Verification limit: the diff is **not applied** in this checkout (there is no
`migrations/0002_lights.sql`, no `internal/store/lights.go`/`photos.go`, and no
`harness/handoffs/T-201.md` on disk), and shell execution is denied in this
session. This is therefore a static review of the diffed bytes against the
contracts I can read; the handoff's executed results (`check-fast`,
`test-acceptance-go`, `generate-check`) are taken on trust. Line numbers below
are post-patch `file:line` unless stated otherwise.

## Findings

### 1. [High — reward hacking / test-shaped production special-case] The runner changes its on-disk checksum encoding to keep a byte-layout test green

`internal/store/migrations.go:32,92-110,136` introduces an asymmetry that exists
to satisfy a test's regex, not a product requirement:

- `checksumRecordValue` (`migrations.go:92-97`) records the **first** embedded
  migration's checksum as 64-char lowercase-hex TEXT (`m.sum`) and **every later**
  migration's checksum as the raw 32-byte digest BLOB (`m.sumRaw`).
- `decodeRecordedSum` (`migrations.go:105-110`) infers the encoding from
  `len(raw) == 64`; there is no explicit tag.
- The production comment (`migrations.go:79-91`) states the purpose verbatim: keep
  the single-hex-token property that `internal/store/dbfile_layout_test.go` pins,
  so the A-104 byte-tamper oracle's "first 64-char token is 0001's checksum"
  assumption stays true.
- The handoff (`harness/handoffs/T-201.md:10-51`) admits the frozen canary's own
  header anticipated a first-token change and offered the sanctioned remedies
  (update its pin, or T-105's index pin); it calls both "outside T-201's
  ownership" and chooses the encoding workaround.

That is exactly the pattern the canary was designed to prevent. The header of
`internal/store/dbfile_layout_test.go:19-22` says a legitimate later hex token
makes the first-token property change and that "This test failing is the canary
for exactly that." The retry silences the canary by changing production so the
invariant is true by construction; the new
`TestDBFileHexTokenIsExactlyTheFirstMigrationChecksum`
(`migrations_test.go:525`) then pins the workaround. The guard's **verification**
path is not weakened (both encodings are decoded and compared, and the new
`TestTamperedSecond...` at `migrations_test.go:464` exercises it), but the
*storage asymmetry* is test-shaped, and the canary no longer independently
detects the layout assumption it exists to protect.

There are also real costs to shipping it:
- `schema_migrations.sha256` is declared `TEXT NOT NULL`
  (`migrations.go:70-75`), but every migration after the first is now stored as a
  BLOB. Any future audit/tooling that reads `sha256` as text gets binary for all
  later rows.
- `decodeRecordedSum`'s length heuristic is a latent format contract: a future
  64-byte digest (e.g. SHA-512, if the digest algorithm ever changes) would be
  misinterpreted as hex TEXT and would refuse valid boots.
- The handoff's claim that reads accept "TEXT hex unconditionally" is true only
  for 64-byte values; that is an undocumented implicit encoding rule.

The correct resolution when a non-owned/frozen test conflicts with the new
migration is the one the rules describe: stop with `BLOCKED` so the canary/pin
is updated by its owner (the packet itself says `dbfile_layout_test.go` is "NOT
yours"), or record an explicit runner-owned format decision and change the
column type/contract in a task that owns it. Production code should not carry a
migration-1 special case whose comments cite `dbfile_layout_test.go` and the
A-104 draft as the rationale.

### 2. [Medium — assertion cannot fail as written] `EditLight`'s `updated_at` stamping is not actually verified

`internal/store/lights_test.go:651-681` names the row
`edit_updates_label_and_asset_no_and_stamps_updated_at`, but the only timestamp
assertion is:

```go
if got.UpdatedAt.Before(got.CreatedAt) { ... }
```

`CreateLight` and `EditLight` both stamp `time.Now().UTC().Format(time.RFC3339)`
at **second** precision (`internal/store/lights.go:170-172` and `:256-260`), and
the test creates then edits immediately. `got.UpdatedAt == got.CreatedAt` in the
normal case, and `Before` is false — so the assertion passes even if `EditLight`
never sets `updated_at` (the column would still hold the create-time value). The
test does not cover its stated contract. Backdate `created_at/updated_at` with a
raw UPDATE before the edit (or move the clock past a second boundary) and assert
the value advanced.

### 3. [Low — conditional assertion / doc overclaim] `checkNearbyOutsideRows` passes when the near-threshold candidates are missing

`internal/store/lights_test.go:583-615` collects `seen8`/`seen25` and then wraps
the distance checks in `if seen8 { ... }` / `if seen25 { ... }`. Nothing asserts
that `seen8`/`seen25` are true, despite the doc at `:585-588` saying the 8.1 m and
25.1 m candidates "appear in the prefilter". If `NearbyLights`/`bboxDegrees`
clipped those rows, the test would still pass. The membership rows in
`TestNearbyLights` (`:486-505`) do assert their presence at the same radii, so
this is a redundant/misleading assertion rather than a gate hole; make it
unconditional and delete the now-redundant second subtest, or assert the
presence here.

### 4. [Low — tautological oracle] `TestBBoxDegrees` re-derives the implementation constants

`internal/store/lights_test.go:878-902` computes
`spanM := latDelta * metersPerDegree` where `latDelta` itself is
`nearbySafety*rM/metersPerDegree` from the production constants
(`internal/store/lights.go:328-346`). The checks only prove `nearbySafety >= 1`
and that `lonDelta` receives the same constant; a wrong `metersPerDegree` (or any
formula using that same wrong value) passes. The real bbox coverage is the
seeded `TestNearbyLights`, which would catch a tight/loose box for the current
fixture points. If the absolute metre conversion is part of the contract, add a
row with an independently computed latitude/longitude delta (using the Earth
radius in `haversineM`, not `metersPerDegree`) and assert a near-boundary point
is inside/outside.

### 5. [Low — non-specific error assertion] Invalid-kind test only checks `err != nil`

`internal/store/lights_test.go:239-248` accepts any error from
`CreateLight(Kind: "shop")`; an unrelated failure (closed DB, constraint) would
pass. Assert `errors.Is(err, errInvalidKind)` and that no row was written, so the
test names the boundary parser it claims to cover.

### 6. [Info — handoff precision]

- `harness/handoffs/T-201.md` calls `internal/store/dbfile_layout_test.go`
  "frozen"/protected; it is not in `harness/policy.json:37-51`'s protected list.
  The packet does say it is "NOT yours", which is the stronger reason not to edit
  it, but the distinction matters for the conflict-resolution path.
- The handoff describes the duplicate seeds as "Haversine-derived"
  (`harness/handoffs/T-201.md:47-50`; helpers at `lights_test.go:1207-1229`), but
  `offsetNorth`/`offsetEast` use the production `metersPerDegree`; only
  `haversineM` is independent. For the current constant the 8.1 m/25.1 m points
  agree with Haversine, but the test cannot detect a wrong production constant
  (see #4).
- `harness/handoffs/T-201.md` says `TestSchemaMatchesExpected` compares "probe
  and bookkeeping excluded", but the test excludes only `schema_migrations` and
  correctly compares the `probe` DDL (`migrations_test.go:809-835`). The
  comparison itself matches `testdata/schema.expected.sql`; this is only a
  wording error.
- `harness/handoffs/T-201.md:144-146` says the store-side halves of A-205/A-208
  are covered by unit tests; that is accurate for what exists, but the
  `dbfile_layout_test.go` workaround is not a store-side claim and shouldn't be
  folded into the A-104 story as if it were the only option.

## Checked and found sound

- **No existing test was weakened, deleted, skipped or narrowed.** The adapted
  assertions are exact-set assertions, not relaxations: the table set is pinned
  to all ten expected tables (`migrations_test.go:112-133`), recorded versions
  to `0001_init,0002_lights` (`:222-253`), and the idempotent-reopen row count to
  two (`:600-611`). `checkSyntheticFileChecksums`
  (`migrations_test.go:773-789`) now decodes the value but still compares the
  resulting digest exactly to `sha256` of the synthetic file bytes; it is not an
  assertion that anything decoded is acceptable.
- **No fixture-value special-casing in production code.** `GP-017`,
  `42.7617`, `-71.0303`, "Path light", "Ball field lamp" and the
  `al/` acceptance helpers appear only in `internal/store/*_test.go` and the
  handoff. `internal/store/lights.go` carries only the bbox constants
  (`metersPerDegree`, `nearbySafety`); `internal/store/migrations.go` carries no
  acceptance filenames or fixture values (the test-file citations in its comments
  are the #1 finding, not a value special-case).
- **No mocks bypassing the behaviour under test.** The tests use a real
  temp-file SQLite database opened through `Open`/`openFresh`, real migrations,
  and real `database/sql` tampering for the upgrade/checksum cases. The upgrade
  test genuinely builds a 0001-only database with `fstest.MapFS` and reopens it
  with the full embedded FS (`migrations_test.go:883-931`).
- **The schema comparison is exact.** `TestSchemaMatchesExpected`
  (`migrations_test.go:809-858`) reads `sqlite_master`, normalizes only line
  comments and whitespace, and compares sorted statement sets to
  `testdata/schema.expected.sql`; the product DDL in `migrations/0002_lights.sql`
  matches the expected file statement for statement (tables and both indexes).
- **The duplicate/soft-delete/photos tests are mutation-sensitive.**
  `checkNearby` (`lights_test.go:548-582`) would fail if the bbox clipped the
  7.9 m point or included the 24.9 m point at radius 8; `checkSoftDeleteHidesEverywhere`
  (`:760-795`) would fail if deletes were hard or reads forgot a `deleted_at`
  filter; `TestEditLight` duplicate and `TestCreatePhoto` UNIQUE-path rows check
  both the error and the row count.
- **Scope and ownership.** Only `migrations/**`, `internal/store/queries/**`,
  `internal/store/gen/**`, `internal/store/lights*.go`, `photos*.go`,
  `migrations*.go` and the handoff are touched. No `store.go`, `store_test.go`,
  `dbfile_layout_test.go`, `acceptance/**`, `harness/**` (except the handoff),
  `Makefile`, golden or `testdata/schema.expected.sql` change. No new
  dependencies; no `t.Skip`/`.only`/`.fixme`; no `TODO`/`FIXME`/`panic("not
  implemented")`/unexplained `//nolint`/`Phase N` strings in the added bytes.

## Verdict rationale

The feature half of the packet is well implemented and genuinely tested: the
product schema matches the frozen oracle, the upgrade path preserves data and
passes FK/integrity checks, and the duplicate/soft-delete/photo tests use real
rows and mutation-sensitive assertions. The secondary findings are assertion
strength/coverage nits that a follow-up can close.

The blocking issue is #1: the migration runner changes the on-disk encoding of
`schema_migrations.sha256` for every migration after the first, with a
length-based decoder, **because a byte-layout test scans the DB file for a single
64-char hex token and the A-104 oracle tampers the first one**. The production
comments say so; the handoff says so; the canary's own header says its failure
was the intended prompt for a conscious update. Making the canary pass by
reshaping production storage is the test-shaped workaround the review process is
supposed to reject. Resolve it by having the canary/pin updated by its owner
(the packet's "not yours" rule / T-105 index pin) or by an explicit,
runner-owned format change whose column type and long-term contract are
deliberate—then re-run the A-104 acceptance and smoke tests.

VERDICT: BLOCK
