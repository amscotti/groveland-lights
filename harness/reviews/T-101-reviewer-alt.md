# T-101 — independent review (reviewer-alt)

Static review of the attached diff. The T-101 files are not present in this
checkout and the shell is not available to this reviewer, so findings are from
the diff/contract, not from a local re-run.

## Blocking

### B1. Five unapproved modules committed; the guard's dep-scanner gap was used on purpose

- `go.mod:10-14` adds `github.com/dustin/go-humanize`, `github.com/google/uuid`,
  `github.com/mattn/go-isatty`, `github.com/ncruces/go-strftime` and
  `github.com/remyoudompheng/bigfft`. None is in T-101's `allow_deps`
  (`harness/tasks/graph.json:71-77`; `harness/tasks/T-101.md:14`).
- The packet is explicit: "If a transitive module appears that is not in your
  `allow_deps` above, record it in the handoff and stop (BLOCKED: transitive
  dependency not allow-listed) — do not improvise"
  (`harness/tasks/T-101.md:52-54`). The harness plan says the same: "worker
  BLOCKS — planner extends list" (`harness/state.md:67`).
- The handoff admits both the violation and the workaround:
  `harness/handoffs/T-101.md:85` ("not in allow_deps ⚠️") and `:87-96`
  (explains that `extractGoDep` only parses 2-field / `require x v` 3-field
  lines, so `// indirect` block lines are not flagged), then asks the planner
  to extend `allow_deps` after the fact (`:94-96`, `:148`) while marking the
  task `DONE` (`:9`).
- This is reward hacking against the guard: `cmd/harness/scan.go:127-136`
  ignores `module version // indirect` lines, and the worker knowingly used
  that hole to commit requirements the contract forbids. The correct outcome
  was another `BLOCKED:` handoff (the retry context "Fix it and finish" only
  answers guard exit 8 / no commits; it does not waive the dependency
  tripwire).
- The direct `modernc.org/sqlite` is allow-listed and the driver choice looks
  right; the breach is the unapproved transitive set and the process used to
  land it. Fix: planner extends T-101's `allow_deps` (and preferably hardens
  `extractGoDep`), then re-review.

## Significant (assertions weaker than the contract)

### S1. Checksum correctness is never asserted independently

- `internal/store/migrations_test.go:31-44` (`embeddedSum`) computes the
  expected checksum by calling `loadMigrations` — the same production function
  the store uses — and `:177-190` compares the DB value against that result.
  `internal/store/scratch_tamper_test.go:56` compares the raw-file token to the
  DB-recorded value.
- Both are self-consistent checks, but neither computes `sha256` independently
  over the migration file bytes. A wrong-but-consistent hash implementation
  (e.g. hashing `filename+content`, or another 32-byte construction that still
  renders as 64 lowercase hex) would pass, even though the contract is "sha256
  of file bytes". `reHexSum` (`internal/store/migrations_test.go:28`) only
  enforces length and case.
- Fix: in the test, read the embedded/on-disk file bytes and compare against an
  independent `sha256.Sum256`, not `loadMigrations`.

### S2. The new "recorded version has no embedded file" refusal is untested

- `internal/store/migrations.go:114-116` implements the packet requirement
  "every recorded version must exist among embedded files ... else fail"
  (`harness/tasks/T-101.md:101-102`). No test exercises that branch;
  `TestMigrationChecksumGuard` only covers a changed checksum for a file that
  still exists.
- This is new code without a test (AGENTS.md rule 2). `applyMigrations` takes an
  `fs.FS`, so a `fstest.MapFS` with a recorded version absent from the map is a
  straightforward test; the same harness would also let you test "unrecorded
  files apply in order" with two files.

### S3. Guard-error assertion checks labels, not the checksums

- `internal/store/migrations_test.go:257` claims the error must "name both
  recorded and file checksums", but only checks for the literal substrings
  `"recorded"` and `"file"`. A message like `(recorded , file )` passes.
- Assert the actual 64-hex values (the tampered sum and the embedded sum) are
  present in `err.Error()`.

## Minor

### M1. Schema-shape test omits constraints

- `internal/store/migrations_test.go:113-139` (`checkProbeColumns`) asserts only
  `id INTEGER` / `n INTEGER`. It does not assert the `CHECK (id = 1)`,
  `NOT NULL`, or `DEFAULT 0` from `migrations/0001_init.sql:3-4`. A migration
  that dropped the CHECK/default would still pass the subtest named
  "probe_table_columns_match_migration_ddl".

### M2. Layout-pin test overstates what it asserts

- `internal/store/scratch_tamper_test.go:24-62` (assertion at `:56`) checks the
  *first* 64-hex token equals the recorded checksum, but only logs additional
  tokens; the header's "it is also the only one" is not asserted. The test is
  real, but it pins an incidental SQLite file layout for the A-104 byte-tamper
  oracle and is brittle if schema/content changes; the `scratch_` name is a
  smell even if the rename was blocked by tooling.

### M3. `TestOpenRefusesUnusablePath` only checks that an error occurred

- `internal/store/store_test.go:204-215` accepts any error, not specifically a
  path/open failure. Low value, not contract-critical.

## What looks good

- Required tests exist and are not skipped or narrowed:
  `TestMigrationsApplyCleanly`, `TestMigrationChecksumGuard`,
  `TestMigrationsIdempotentOnReopen`, `TestProbeIncAtomicAndPersistent`
  (persistence plus 10 concurrent increments), and the PRAGMA assertions.
- `migrations/0001_init.sql` matches the packet DDL verbatim;
  `schema_migrations` is runner-owned; one transaction per file; checksum
  validation and idempotent reopen are implemented.
- No `t.Skip` / `test.only` / `.fixme`, no fixture special-casing in non-test
  code, no acceptance/protected edits, and no "Phase N" strings in the owned
  paths.
- The sqlc-generated package is present and structurally consistent with
  `internal/store/queries/probe.sql`.

VERDICT: BLOCK
