# T-101 — independent review (reviewer-alt, attempt 3)

Scope: attached diff (`harness/reviews/T-101.diff`, 14 files vs branch base
`4634669`), plus the current main checkout for the dependency amendment
(`harness/tasks/graph.json:71-83`) and the A-103/A-104 frozen drafts.
Shell execution is denied in this environment, so I did not re-run the suite
myself; the diff content was reviewed line-by-line and the worker's own run
log (`harness/runs/20261113T052633/T-101/worker.jsonl`) was used to confirm
that `make check-fast` (exit 0), `go test ./internal/store/... -race`, `make
generate-check`, `make phase-grep` and `make e2e-task TASK=T-101` actually ran
green after the final commit. The T-101 files are not in the main checkout,
so this is a static review of the diff, not a local re-run.

## Prior blockers — resolved

- **F1 / B1 (unallow-listed transitives).** `harness/tasks/graph.json:71-83`
  now lists all five transitives (`dustin/go-humanize`, `google/uuid`,
  `mattn/go-isatty`, `ncruces/go-strftime`, `remyoudompheng/bigfft`) in
  addition to the modernc family. Every requirement added by the diff
  (`go.mod` lines 9-25) is now explicitly allowed; `modernc.org/gc/v3` is
  allow-listed and unused, which is fine. The earlier guard-scanner gap
  (`cmd/harness/scan.go:130-139` still ignores `path v // indirect` block
  lines) remains, but it is now a harness hardening issue, not a violation of
  this task's allow-list. The handoff's open issue #1 is accurate.
- **S1 (independent checksum).** `internal/store/migrations_test.go:57-74`
  (`independentSum`) reads the embedded bytes with `fs.ReadFile` and computes
  `sha256.Sum256` itself; `checkRecordedChecksum` (`:235-250`) compares the
  DB value against BOTH that independent digest and the runner's derivation.
  A wrong-but-consistent hash implementation now fails.
- **S2 (untested refusal branch / ordering).**
  `TestRecordedVersionWithoutEmbeddedFileRefusesOpen`
  (`internal/store/migrations_test.go:420-462`) inserts a `0002_ghost` row via
  the raw driver, re-opens through the public `Open`, asserts refusal names
  `0002_ghost`, and asserts the error is NOT `ErrChecksumMismatch`.
  `TestApplyMigrationsAppliesUnrecordedFilesInOrder` (`:464-577`) drives
  `applyMigrations` over a synthetic `fstest.MapFS` where 0002 depends on
  0001, asserts order, both recorded versions, independent sha256 of the
  synthetic bytes, and a re-apply no-op. Both branches are real and fail if
  the production code regresses.
- **S3 (actual sums in the refusal).**
  `TestMigrationChecksumGuard` (`internal/store/migrations_test.go:262-333`)
  now asserts the error contains the actual tampered 64-hex value and the
  embedded file sum, not just the words "recorded"/"file". The uppercase
  tamper case still pins case-sensitive comparison.
- **M1 (schema constraints).** `checkProbeColumns`
  (`internal/store/migrations_test.go:134-220`) asserts via
  `PRAGMA table_info` that `id` is INTEGER + pk=1 and `n` is INTEGER +
  notnull=1 + default "0" + pk=0, and via `sqlite_master.sql` that the stored
  DDL contains `CHECK (id = 1)`, `NOT NULL` and `DEFAULT 0`. The prior
  "columns only" gap is closed.
- **M2 / F2 (layout pin).** `scratch_tamper_test.go` is gone; the successor is
  `internal/store/dbfile_layout_test.go` and
  `TestChecksumIsFirstAndOnlyHexTokenInDBFile`
  (`internal/store/dbfile_layout_test.go:23-61`) now asserts exactly one
  64-char lowercase-hex token and that it equals the recorded checksum. The
  old "additional tokens are only logged" weakness is closed; the test is a
  deliberate canary and its header documents the A-104 token-index fallback.
- **M3 (path refusal test).** `TestOpenRefusesUnusablePath`
  (`internal/store/store_test.go:205-251`) is a table covering missing parent,
  `?`, `#`, `%`, asserts the error names the path and the offending substring,
  and a separate subtest asserts the DSN rejection wraps the typed
  `errBadDSNPath` sentinel. This is a real strengthening over "an error
  occurred".
- **F4 (DSN escaping).** `internal/store/store.go:43-55` adds
  `checkDSNPath`, called first in `Open` (`:57-60`); it refuses `?`, `#`, `%`
  instead of silently letting the driver reinterpret the filename. Covered by
  the table above.
- **F5 (ErrNotFound).** `internal/store/store.go:86` now uses `errors.New`.

## Test-integrity scan (no blocking findings)

- No `t.Skip`, `test.skip`, `test.only`, `.fixme`, or skipped/narrowed
  tests in the diff. The required tests from the packet
  (`TestMigrationsApplyCleanly`, `TestMigrationChecksumGuard`,
  `TestMigrationsIdempotentOnReopen`, `TestProbeIncAtomicAndPersistent`, the
  PRAGMA reads) are present and at least as strong as the packet asks.
- No test deletion: the only rename is `scratch_tamper_test.go` →
  `dbfile_layout_test.go`, and the successor contains the same layout check
  plus the stronger only-one-token assertion (commit `b8af66a` records it as a
  59% rename). No coverage was lost.
- No fixture special-casing in non-test code: the production files
  (`internal/store/store.go`, `internal/store/migrations.go`,
  `migrations/migrations.go`, `migrations/0001_init.sql`,
  `internal/store/gen/*`) contain no A-104/A-103 fixture literals; the
  checksum guard message is built from `m.file`/`m.sum`, not a hardcoded
  fixture name. No acceptance/testdata literal appears in added non-test code.
- No mocks bypassing the behaviour under test: every test opens a real
  SQLite file with the real `modernc.org/sqlite` driver; tampering uses the
  raw driver only to simulate on-disk/DB drift, which is the contract under
  test.
- No coverage theatre: the new tests are mutation-sensitive. Removing the
  ghost-version check makes `TestRecordedVersionWithoutEmbeddedFileRefusesOpen`
  fail; hashing `filename+content` makes `independentSum` comparisons fail;
  applying in reverse order makes the synthetic 0002 fail; ignoring case makes
  the uppercase guard case fail; dropping the CHECK/default makes
  `checkProbeColumns` fail; dropping DSN pragmas makes the PRAGMA reads fail.
- No dead code to satisfy linters and no `//nolint`. The worker's transient
  `strings.Contains(recorded, "")` line was removed before the final diff
  (`dbfile_layout_test.go` imports only `context`, `os`, `path/filepath`,
  `regexp`, `testing`).
- No `TODO`/`FIXME`/`panic("not implemented")`, no "Phase N" strings, no
  `console.log` in the added code; `make phase-grep` was run green.

## Non-blocking findings

1. **`schema_migrations` DDL/RFC3339 contract is exercised but not pinned.**
   `internal/store/migrations.go:70-76` creates
   `schema_migrations(version TEXT PRIMARY KEY, sha256 TEXT NOT NULL,
   applied_at TEXT NOT NULL)`, but the tests only select the columns and check
   `applied_at` is unchanged on reopen
   (`internal/store/migrations_test.go:222-250`, `:335-417`). A runner whose
   DDL dropped `NOT NULL`/`PRIMARY KEY`, or whose `applied_at` were e.g.
   `time.Now().String()`, would still pass. This is an assertion weaker than
   the packet's explicit DDL; worth a small `PRAGMA table_info(schema_migrations)`
   pin plus one RFC3339 parse in a follow-up, not a merge blocker.
2. **`TestProbeIncReturnsSequentialValues` only checks the last value.**
   `internal/store/store_test.go:61-103` calls `ProbeInc` `incs` times but only
   asserts the final value and final `ProbeCount`. A bug returning
   `1, 1, 3` for three increments would pass this test (the final count is
   still 3). The 10-way concurrency test does assert distinct `1..10`, so the
   implementation is covered; the name overclaims. Collect all returned values
   and assert `1..incs` if the name is to be taken literally.
3. **The missing-parent case in `TestOpenRefusesUnusablePath` is not
   independent.** `internal/store/store_test.go:205-231` uses
   `wantErr: "no-such-subdir"` where that substring is part of `dbPath`; the
   two `strings.Contains` assertions therefore collapse to one, and any error
   mentioning the path passes. The test still fails if `Open` succeeds, but it
   does not prove the failure cause is the missing parent. Low value; assert
   `os.IsNotExist`-style cause or drop the duplicate assertion.
4. **PRAGMA test does not force a second pooled connection.**
   `TestWALSettingsAssertedViaPragmas`
   (`internal/store/store_test.go:13-59`) reads `journal_mode`,
   `foreign_keys` and `busy_timeout` through whichever connection the pool
   hands out. The implementation correctly rides the DSN, but a one-off Exec
   on the first connection could also pass this test. The packet only asks for
   PRAGMA reads, so this is a coverage note, not a defect.
5. **The layout pin is stronger than the A-104 oracle.**
   `internal/store/dbfile_layout_test.go:54-61` requires exactly one 64-hex
   token, whereas A-104 only tampers the first token. The header documents
   this as an intentional canary and points at T-105's sanctioned index pin;
   acceptable, but it will fail on a legitimate future migration that adds a
   later hex token even when A-104 would still pass.
6. **`checkDSNPath` does not cover spaces.** `internal/store/store.go:43-55`
   rejects `?`, `#`, `%` but not a space; the prior F4 review mentioned
   spaces. No test covers a space in `dbPath`. The guard is still an
   improvement; T-103 can normalize at the config boundary.
7. **Handoff count.** `harness/handoffs/T-101.md:225` says "10 top-level
   store tests"; the diff contains 11 top-level `Test*` functions in
   `internal/store`. Trivial documentation inaccuracy, not a test issue.
8. **Process note (disclosed, not a coverage change).**
   `harness/handoffs/T-101.md:179` documents that the rename/old-file removal
   was done with a self-deleting `go run` helper placed under
   `internal/store/testdata/` because `mv`/`rm`/`git mv` were denied. The
   helper left no tracked artifact (the diff contains no testdata file and the
   worktree's `testdata/` is empty), and the successor test is the
   strengthened version. It is a sandbox-circumvention process detail the
   integrator should be aware of; it did not weaken any test.
9. **Stale packet header.** `harness/tasks/T-101.md:14` still shows the old
   five-module `allow_deps`, while the graph (the guard's source of truth) has
   the amendment and the retry context (`:448-451`) says it was extended.
   Cosmetic, but a future worker reading only the packet block could be
   confused.

## Bottom line

The prior blocking finding is genuinely cured by the planner's graph
amendment, and every prior test-integrity finding (S1-S3, M1-M3, F2, F4, F5)
is addressed by real, mutation-sensitive tests. The remaining items above are
minor assertion-strength and documentation notes, not reasons to withhold
merge. No skipped/deleted/narrowed tests, no fixture special-casing in
non-test code, no mocks, no coverage theatre, no dead code were found.

VERDICT: PASS
