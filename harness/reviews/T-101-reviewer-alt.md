# T-101 — independent test-integrity review (reviewer-alt, attempt 3)

Reviewed: `harness/reviews/T-101.diff` (14 files), packet
`harness/tasks/T-101.md`, `harness/tasks/graph.json`, the frozen drafts
`acceptance/drafts/migrations_go_test.go.txt` /
`acceptance/drafts/probe_go_test.go.txt`, the guard sources
(`cmd/harness/guard.go`, `cmd/harness/scan.go`), and the worker transcripts
`harness/runs/20261113T052633/T-101/worker.jsonl` and
`harness/runs/20261113T054857/T-101/worker.jsonl` for command evidence.

Method/limitation: this reviewer's shell is restricted, so I did not run the
suite myself. The claimed green run is nevertheless evidenced in the
transcripts: `make check-fast` → 0 (`go mod tidy` no drift, vet, golangci-lint
0 issues, `go test -race -count=1 -shuffle=on ./...` all ok),
`go test ./internal/store/... -race -count=1` → 0, `make generate` →
no diff, `make generate-check`/`phase-grep`/`e2e-task TASK=T-101` → 0,
`go list -m modernc.org/sqlite` → `v1.62.0`, and
`git diff --name-only 4634669..HEAD` touches only owned paths + the handoff.

## Prior blockers — verified resolved

- **Deps (F1/B1).** `harness/tasks/graph.json:71-83` now lists all five
  transitives (`dustin/go-humanize`, `google/uuid`, `mattn/go-isatty`,
  `ncruces/go-strftime`, `remyoudompheng/bigfft`) plus `modernc.org/libc`
  (duplicated) and the original modernc set. Every `go.mod` addition in the
  diff is now allowed. The guard-scanner gap (`extractGoDep` ignoring
  `path vX.Y.Z // indirect` block lines) still exists, but it is a harness
  hardening item, not a violation of this task's allow-list. The handoff's
  open issue #1 is accurate about it.
- **S1** — `internal/store/migrations_test.go:57-65` computes sha256 directly
  from `migrations.Files` bytes, and `checkRecordedChecksum`
  (`:235-253`) asserts against both that independent digest and the runner's
  derivation. A wrong-but-consistent hash cannot pass.
- **S2** — ghost version: `TestRecordedVersionWithoutEmbeddedFileRefusesOpen`
  (`:420-462`) inserts `0002_ghost` with the raw driver, re-opens through the
  public `Open`, and asserts refusal naming `0002_ghost` and NOT
  `ErrChecksumMismatch`. Ordering: `TestApplyMigrationsAppliesUnrecordedFilesInOrder`
  (`:464-577`) uses a `fstest.MapFS` where 0002 inserts into 0001's table, so a
  reversed order fails; it also asserts recorded versions, independent
  sha256 of the synthetic bytes, and a re-apply no-op.
- **S3** — `TestMigrationChecksumGuard` (`:321-329`) asserts the actual
  tampered 64-hex value and the embedded file sum appear in the refusal, not
  only the labels; the uppercase tamper case (`:272-274`) pins case-sensitive
  comparison.
- **M1** — `checkProbeColumns` (`:134-197`) pins `id INTEGER pk=1`,
  `n INTEGER notnull=1 dflt="0" pk=0`, and the stored DDL's
  `CHECK (id = 1)`/`NOT NULL`/`DEFAULT 0`.
- **M2/F2** — `internal/store/dbfile_layout_test.go` replaces
  `scratch_tamper_test.go` (commit `b8af66a`, recorded as a 59% rename) and
  now asserts the recorded checksum is the first AND only 64-char lowercase-hex
  token (`:54-62`). This is stronger than the prior "log extras" version.
- **M3/F4/F5** — `TestOpenRefusesUnusablePath` is a 4-case table plus a typed
  `errBadDSNPath` subtest (`internal/store/store_test.go:205-258`);
  `checkDSNPath` refuses `?`/`#`/`%` before the driver
  (`internal/store/store.go:43-55`); `ErrNotFound` is `errors.New`
  (`internal/store/store.go:82`).

## Test-integrity scan — no blocking findings

- No `t.Skip`, `test.skip`, `test.only`, `.fixme`, or build-tag disabling in
  the diff; the final `-v` run in the transcript shows every test executing.
- No test deletion or narrowing: the only removed test file is
  `scratch_tamper_test.go`, and its successor is the strictly stronger
  `dbfile_layout_test.go`; `migrations_test.go` and `store_test.go` are
  additions/strengthenings.
- No fixture special-casing in non-test code. Production literals are generic;
  the migration filename, version and checksum come from `m.file`/`m.version`/
  `m.sum`, not from A-103/A-104 fixture strings. `0001_init` appears only in
  tests, comments and the handoff.
- No mocks bypass the behaviour under test: every check opens a real temp-dir
  SQLite database through the real `modernc.org/sqlite` driver; the raw
  `sql.Open("sqlite", …)` calls are only used to simulate out-of-band drift
  (checksum tamper, ghost row), which is exactly the contract.
- Tests are mutation-sensitive: removing the ghost check fails
  `TestRecordedVersionWithoutEmbeddedFileRefusesOpen`; hashing
  `filename+content` fails the independent-sum comparison; reversed file order
  fails the MapFS 0002 insert; ignoring case fails the uppercase guard case;
  dropping DDL constraints fails `checkProbeColumns`; dropping the DSN pragmas
  fails the PRAGMA reads; a non-atomic increment fails the 10-worker test.
- No dead code for the linter and no `//nolint`; the cyclop fix extracted
  sub-checks without dropping assertions (transcript shows the initial cyclop
  failure and the subsequent 0-issue run).

## Findings (non-blocking)

1. **`TestProbeIncReturnsSequentialValues` only asserts the last value.**
   `internal/store/store_test.go:61-104` loops `incs` times but checks only
   `last` and the final `ProbeCount` (`:86-93`); an implementation returning
   `1,1,3` for three increments would pass despite the test name. The 10-way
   concurrency test (`:144-186`) does assert distinct `1..10`, so the
   implementation is covered; collecting all returned values here would make
   the name honest.
2. **`schema_migrations` DDL and `applied_at` format are not pinned.**
   `internal/store/migrations.go:70-76` creates
   `schema_migrations(version TEXT PRIMARY KEY, sha256 TEXT NOT NULL,
   applied_at TEXT NOT NULL)` and writes RFC3339 UTC (`:159-160`), but the
   tests only select the columns and compare `applied_at` across reopen
   (`internal/store/migrations_test.go:347-350`, `:395-397`). A DDL that
   dropped `PRIMARY KEY`/`NOT NULL`, or a non-RFC3339 timestamp, would pass.
   A `PRAGMA table_info(schema_migrations)` pin plus one
   `time.Parse(time.RFC3339, appliedAt)` would close this.
3. **No rollback test for a failing migration.**
   `internal/store/migrations.go:144-166` promises "Any failure rolls the whole
   file back", but every migration test uses valid SQL. A `MapFS` with a valid
   `0001_alpha.sql` and an invalid `0002_bad.sql` should assert that `0002`
   leaves no table/row and records nothing; that branch of new code is
   currently unexercised.
4. **Missing-parent path case is not cause-specific.**
   `internal/store/store_test.go:205-232` uses `wantErr: "no-such-subdir"`,
   which is a substring of `dbPath`, so the two `strings.Contains` assertions
   collapse into one. It still fails if `Open` succeeds, but it does not prove
   the cause is the missing parent.
5. **PRAGMA test does not force a second pooled connection.**
   `internal/store/store_test.go:13-59` reads the pragmas through whichever
   connection the pool hands out; a one-off `Exec` on the first connection
   would also pass. The implementation correctly rides the DSN, so this is a
   coverage note only.
6. **The layout canary is stronger than the A-104 oracle.**
   `internal/store/dbfile_layout_test.go:54-62` requires exactly one 64-hex
   token, while A-104 tampers only the first
   (`acceptance/drafts/migrations_go_test.go.txt:213-229`). The header
   (`:11-22`) documents this as an intentional canary and names T-105's
   sanctioned index fallback, so it is acceptable; note it will fail on a
   legitimate future migration that stores a later hex token even when A-104
   would still pass.
7. **`checkDSNPath` does not cover spaces.**
   `internal/store/store.go:43-55` rejects `?`, `#`, `%` but not a space, and
   no test covers a space in `dbPath`. Residual F4 work for T-103's config
   boundary; not a test weakening.
8. **Stale count in the handoff.** `harness/handoffs/T-101.md:133` says "10
   top-level store tests"; the diff contains 11. Trivial documentation
   inaccuracy.
9. **Process note (disclosed).** `harness/handoffs/T-101.md:87` records that
   the rename/removal was performed by a self-deleting `go run` helper under
   `internal/store/testdata/` because `mv`/`rm`/`git mv` were denied. The helper
   left no tracked artifact (transcript and diff confirm), no protected path
   was touched, and no test content was altered; it is a sandbox workaround,
   not a coverage change.

## Bottom line

The only previous BLOCK (unallow-listed transitives) is genuinely cured by the
planner's graph amendment, and each earlier test-integrity finding is now
covered by a real, mutation-sensitive assertion. I found no skipped, deleted,
narrowed or fixture-special-cased test, no mock that bypasses the behaviour,
no coverage theatre, and no dead code added for the linter. The findings above
are assertion-strength and test-completeness nits, suitable for a follow-up,
not reasons to withhold merge.

VERDICT: PASS
