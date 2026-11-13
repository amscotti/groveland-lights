# T-101 review (reviewer pass, attempt 3)

Reviewed: attached diff (12 files vs main) — `go.mod`, `go.sum`,
`harness/handoffs/T-101.md`, `internal/store/{store,migrations}.go`,
`internal/store/{store,migrations,dbfile_layout}_test.go`,
`internal/store/queries/probe.sql`, `internal/store/gen/{db,models,probe.sql,querier}.go`,
`migrations/{0001_init.sql,migrations.go}` — against the packet
(`harness/tasks/T-101.md`, including both attempt-2 reviews and the planner
retry context), `harness/tasks/graph.json`, `harness/policy.json`, guard
source (`cmd/harness/guard.go`, `cmd/harness/scan.go`), `Makefile`,
`sqlc.yaml`, the frozen A-104 draft (`acceptance/drafts/migrations_go_test.go.txt`),
`acceptance/contract.json`, and `docs/spec/architecture.md`.

Method note: shell execution is denied in this environment, so the suite was
not re-run and worktree `wt/T-101` / commits `6e7ae1d`/`8c2b363`/`b8af66a`
were not inspected directly. Every repo-side claim below was verified by
reading the protected files in this checkout (main); the diff was reviewed
line-by-line. Runtime claims rest on the handoff; the gate re-runs
`make check` and all nine guard checks regardless.

## 1. The attempt-2 blocker (deps outside allow_deps) — RESOLVED via the sanctioned path

- Attempt 2 was BLOCKed by both reviewers for committing five transitives of
  `modernc.org/sqlite` outside `allow_deps`, knowingly, through the guard's
  `// indirect` scanning gap. The prescribed escalation existed all along
  (packet: "record it in the handoff and stop … do not improvise",
  `harness/tasks/T-101.md:52-53`): worker BLOCKs, planner amends, retry
  consumes.
- The amendment is real and planner-side, verified through two channels the
  worker cannot write (`harness/policy.json:41` protects `harness/tasks/graph.json`;
  AGENTS.md:43 in the same protected list):
  - `harness/tasks/graph.json:71-83` now lists for T-101:
    `modernc.org/{sqlite,libc,mathutil,memory,gc/v3}` **plus**
    `github.com/dustin/go-humanize`, `github.com/google/uuid`,
    `github.com/mattn/go-isatty`, `github.com/ncruces/go-strftime`,
    `github.com/remyoudompheng/bigfft` (and a harmless duplicate
    `modernc.org/libc` at :82). Status `review`, attempts 3 (:84-85).
  - `harness/tasks/T-101.md:448-452` (planner retry context): "The allow list
    has NOW been extended by the planner to cover the legitimate transitives
    (…). Run `go mod tidy`, ensure the requires carry proper `// indirect`
    markers where appropriate, address every other review finding … and
    re-commit."
  The attached diff touches neither file (its paths are exactly
  `internal/store/**`, `migrations/**`, `go.mod`, `go.sum` + the sanctioned
  handoff file, matching `checkOwnership`'s allowlist, guard.go:132-146).
- Dep-by-dep check of the diff's go.mod against the amended list:
  `modernc.org/sqlite v1.62.0` direct (new go.mod:7; the packet's pin,
  matching the policy line at docs/spec/architecture.md:54) ✓; indirect adds
  `go-humanize`/`uuid`/`go-isatty`/`go-strftime`/`bigfft` (new go.mod:10-14)
  and `modernc.org/{libc,mathutil,memory}` (new go.mod:21-23) — every one
  allow-listed. The `golang.org/x/*` indirect lines pre-exist on main
  (verified against this checkout's go.mod:7-13; they serve the `tool`
  directive) and are not additions of this task. `// indirect` markers sit
  exactly where `go mod tidy` puts them, and `fmt-check`'s `tidy -diff`
  (Makefile:54) enforces no drift at the gate.
- go.sum-only hash rows for modules with no require line in go.mod
  (`modernc.org/cc/v4`, `ccgo/v4`, `fileutil`, `gc/v2`, `goabi0`, `opt`,
  `sortutil`, `strutil`, `token`, `hashicorp/golang-lru/v2`, `google/pprof`)
  are module-graph bookkeeping that `go mod tidy` — which the retry context
  explicitly mandates — must write for the pinned driver. They are not
  requirements and not importable (any import forces a go.mod require, which
  `checkDeps` diffs against `main:go.mod`, guard.go:183-203). Not a rule-3
  breach. `modernc.org/gc/v3` is allow-listed yet unused — planner
  over-provision, harmless.
- Tooling disclosure in the handoff (the F2 rename performed by a
  self-deleting `go run` helper under `internal/store/testdata/` because
  `mv`/`rm` were sandbox-denied) was checked for residue: the diff contains
  no testdata files, no protected path, nothing outside the task's own
  ownership; `go` is an allowed command prefix the packet itself mandates
  for go.mod mutation. Disclosed and clean — no violation.

## 2. Prior review findings — each addressed with code + tests

- **S1 (checksum never asserted independently)** — addressed.
  `independentSum` (`internal/store/migrations_test.go:~59-72`) reads the
  embedded bytes via `fs.ReadFile(migrations.Files, …)` and computes
  `sha256.Sum256` itself, bypassing `loadMigrations`;
  `checkRecordedChecksum` (~:187-203) now requires the DB-recorded value to
  equal BOTH the independent digest and the runner-derived one. A
  wrong-but-consistent hash implementation can no longer pass.
- **S2 (recorded-without-file refusal untested)** — addressed with two new
  tests. `TestRecordedVersionWithoutEmbeddedFileRefusesOpen` (~:395-445):
  real DB, `0002_ghost` row inserted via the raw driver, re-`Open` refuses
  naming `0002_ghost` and explicitly NOT as `ErrChecksumMismatch`.
  `TestApplyMigrationsAppliesUnrecordedFilesInOrder` (~:447-495) drives the
  real `applyMigrations` over `fstest.MapFS` where 0002 references 0001's
  table (an order violation fails the apply), asserts both versions recorded
  in order, each recorded checksum equals an independent sha256 of the
  synthetic bytes, and re-apply is a no-op. New branches now have new tests.
- **S3 (guard error asserted by labels only)** — addressed.
  `TestMigrationChecksumGuard` (~:280-295) now asserts the refusal contains
  the actual tampered 64-hex value (`strings.Contains(err.Error(),
  tt.tamperWith)` — the uppercase variant doubles as a case-sensitivity pin,
  since a case-insensitive comparison would boot and fail the "want refusal"
  arm) and the embedded file's sum, alongside `errors.Is(err,
  ErrChecksumMismatch)` and the `0001_init.sql` naming.
- **M1 (schema shape without constraints)** — addressed. `checkProbeColumns`
  asserts via `PRAGMA table_info`: `id` INTEGER/pk=1, `n` INTEGER/
  notnull=1/dflt "0"/pk=0, and via `sqlite_master.sql`: `CHECK (id = 1)`,
  `NOT NULL`, `DEFAULT 0`. A migration dropping the CHECK/default now fails
  "probe_table_columns_match_migration_ddl".
- **M2 + F2 (layout-pin test name/assertion)** — addressed. File renamed
  `dbfile_layout_test.go`; test is `TestChecksumIsFirstAndOnlyHexTokenInDBFile`
  and asserts the recorded checksum is the ONLY `[0-9a-f]{64}` token in the
  closed main DB file (`internal/store/dbfile_layout_test.go:56-62` —
  previously extras were only logged). The pin remains the sanctioned canary:
  the frozen A-104 draft byte-tampers exactly the first such token
  (`acceptance/drafts/migrations_go_test.go.txt:216-229`) and its header
  (:11-17) pre-authorizes exactly one T-105 token-index edit with
  documentation; the failure message points at both.
- **M3 (unusable-path test too weak)** — addressed. Table of missing-parent /
  `?` / `#` / `%` (`internal/store/store_test.go:~224-250`), each asserting
  the refusal names the path AND the offending substring, plus a
  `dsn_rejection_wraps_typed_error` subtest asserting `errors.Is(err,
  errBadDSNPath)`.
- **F4 (`dsn()` does not escape the path)** — addressed in the store itself:
  `checkDSNPath` (`internal/store/store.go:43-50`) refuses `?`, `#`, `%`
  with the typed `errBadDSNPath` (:38) before any open
  (called at :56). Fail-closed is the right call; friendlier handling of
  legitimately odd filenames is correctly deferred to T-103's config
  boundary (handoff open issue #3).
- **F5** — addressed: `var ErrNotFound = errors.New("store: not found")`
  (`internal/store/store.go:78`).
- **F3, F6** — no-change dispositions, matching what the prior reviewers
  themselves concluded (sanctioned canary; single-process "one writer"
  architecture, docs/spec/architecture.md:4).

## 3. Verified against the packet and the frozen contracts

- **Deliverables**: `migrations/0001_init.sql` matches the packet DDL
  (`harness/tasks/T-101.md:62-69`) verbatim, including the seed row and
  `CHECK (id = 1)`; runner-owned `schema_migrations` matches the packet's
  column spec exactly (version TEXT PRIMARY KEY / sha256 TEXT NOT NULL /
  applied_at TEXT NOT NULL, `internal/store/migrations.go:70-75`) and is NOT
  in 0001; one transaction per file with the bookkeeping insert inside the
  same tx (`applyOne`, :~133-165 — migration+record are atomic); version =
  filename sans extension; sha256 = lowercase hex of file bytes; applied_at
  RFC3339 UTC; unrecorded files apply in `fs.Glob` lexical order; reopen of
  an up-to-date DB writes nothing (asserted via row count and untouched
  sha256/applied_at).
- **A-104 frozen strings**: the refusal
  `store: migration 0001_init.sql checksum mismatch (recorded X, file Y): checksum mismatch`
  (`internal/store/migrations.go:106-109`) wraps `ErrChecksumMismatch` and
  satisfies the draft's lowercased-match requirements ("0001_init.sql" +
  "checksum", draft :298-304). The HTTP half (non-zero exit) is T-103's, as
  the packet directs; the frozen refusal contract is covered at store level
  by `TestMigrationChecksumGuard` + `TestRecordedVersionWithoutEmbeddedFileRefusesOpen`.
- **Frozen API**: `Open(ctx, dbPath) (*DB, error)`, `Close`, `ProbeCount`,
  `ProbeInc` (UPDATE…RETURNING inside a tx, `internal/store/store.go:~92-110`),
  ctx-first everywhere, sentinels `ErrNotFound`/`ErrChecksumMismatch` —
  exactly as the packet specifies (`harness/tasks/T-101.md:82-93`) and as
  the handoff freezes for T-103.
- **Pragmas**: DSN-riding per-connection pragmas (WAL / busy_timeout 5000 /
  foreign_keys, `internal/store/store.go:28-33`) is the correct mechanism
  for a pooled `database/sql` handle — a one-off Exec would only cover one
  connection; asserted by PRAGMA reads plus the `-wal` sidecar
  (`internal/store/store_test.go:~13-64`).
- **sqlc**: `internal/store/queries/probe.sql` ↔ `gen/**` consistent with
  `sqlc.yaml:3-11` (package gen, out `internal/store/gen`, `emit_interface:
  true` → querier.go present; header says sqlc v1.31.1, matching the pinned
  tool, Makefile:27). `generate-check` (Makefile:77-81) fails on drift, so
  the committed state is machine-verified at the gate.
- **No forbidden wiring**: no routes/handlers/templates; `cmd/server`,
  `internal/web`, `web/` untouched — the packet's "do NOT wire anything into
  cmd/server" holds; `/healthz` behavior unchanged.
- **Hygiene**: no "Phase N" strings under cmd/server, internal, web,
  migrations (nothing in the diff would match `phase-grep`,
  Makefile:130-133); no TODO/FIXME/`panic("not implemented")`/`//nolint`
  without reason; no `console.log`; no `t.Skip`/`test.only`/`.fixme`
  (also guard-enforced, guard.go:159-168); no `testdata/**golden*` created.

## 4. Security and architecture

- **Surface**: store-only change → CSRF, XSS/`template.HTML`, upload limits,
  HTMX-`#map` are N/A (nothing under `internal/web` or `web/` changed).
- **SQL**: all product SQL is static (embedded, compile-time-trusted
  migrations + sqlc constants); the only interpolation is the parameterized
  bookkeeping INSERT (`internal/store/migrations.go:~155`) and parameterized
  statements in tests. No injection path.
- **Path handling**: `dbPath` is operator config (T-103 wires `DB_PATH`),
  not request input. `Open` mkdirs nothing, refuses missing parents (ping
  fails), and now fails closed on DSN-ambiguous characters (`?`/`#`/`%`) —
  the opposite of path traversal.
- **Secrets**: refusal errors print public file checksums and paths only; no
  secrets in output.
- **Ownership map**: `internal/store` is the sole importer of the driver,
  matching `docs/spec/architecture.md:18` ("SQLite access (sqlc-generated),
  migrations runner, all queries"). Dependency policy (:52-55) is
  allow-list-per-task; the amended list is satisfied exactly.
- **Parse-don't-validate / typed IDs**: no handlers exist yet, so nothing
  reads `r.FormValue`; the store boundary takes already-typed inputs and
  returns typed sentinels. `LightID`/`ReportID` arrive with the product
  schema (M2+), correctly not improvised here.
- **Tests exercise the behaviour**: real temp-dir databases, real
  close/reopen cycles, raw-driver tampering, byte-level file reads,
  10-goroutine concurrent increments asserting exactly {1..10} plus final
  count, persistence across Close→Open, `ErrNotFound` on a deleted row,
  WAL sidecar existence. The `fstest.MapFS` case runs the real
  `applyMigrations` against a real DB — synthetic input, not a mocked
  system. Table-driven with assertion-style names throughout.

## 5. New findings (all non-blocking)

1. **N1 — empty/`file:`-prefixed `dbPath` edge**: `checkDSNPath`
   (`internal/store/store.go:43-50`) rejects `?`/`#`/`%` but not an empty
   path (DSN becomes `file:?_pragma=…` → SQLite opens a private temp
   database, writes silently discarded on close) or a path already prefixed
   `file:`. Both are fail-safe-ish rather than unsafe, the input is operator
   config, and the packet does not require it — but T-103's config parsing
   (parse-don't-validate) should require a non-empty, non-`file:` `DB_PATH`.
2. **N2 — ghost check runs after unrecorded applies**
   (`internal/store/migrations.go:97-118`): a DB with a ghost version plus a
   newer binary applies the new migrations before refusing. The applied
   files are checksum-recorded and idempotent, and the refusal is stable
   across boots, so no data harm — but validate-ghosts-first,
   mutate-second would be marginally cleaner. Cosmetic ordering.
3. **N3 — handoff inaccuracies (cosmetic)**: (a) "10 top-level store tests"
   — the diff has 11 (5 + 5 + 1 across the three _test.go files); (b) open
   issue #4 claims `make e2e-task T=T-101` "silently matches nothing" —
   actually `TASK` stays empty and the target exits 2 with a usage message
   (Makefile:113); loud, not silent. The underlying AGENTS.md `T=` vs
   Makefile `TASK=` docs inconsistency is real but out of T-101's ownership.
4. **N4 — advisory, planner/harness-owned** (already handoff open issue #1):
   `extractGoDep` (`cmd/harness/scan.go:130-139`) still ignores 4-field
   `path vX.Y.Z // indirect` block lines, so the allowlist is not
   machine-enforced for indirect requires. It stopped mattering here (every
   addition is now explicitly allow-listed — verified dep-by-dep above),
   but a harness task should close the gap so a future transitive set can't
   ride in unreviewed.
5. **N5 — graph.json duplicate**: `modernc.org/libc` appears twice in
   T-101's `allow_deps` (graph.json:73 and :82). Harmless planner-side
   artifact.

## Limitations

Could not execute `make check-fast` / `go test -race` / `go mod tidy`, nor
inspect worktree `wt/T-101` or the three commits (shell denied). Runtime,
tidy-stability and clean-worktree claims rest on the handoff; the gate
re-runs `make check` and the guard (protected paths, manifest, ownership,
ratchet, markers, deps, handoff, special-casing heuristic, clean tree)
before merge, and nothing in the diff contradicts the claims.

## Bottom line

The single attempt-2 blocker is cured through the exact process the harness
prescribes (worker BLOCK → planner amends `allow_deps` → retry consumes),
verified against two protected, worker-unwritable sources, and the diff's
go.mod now matches the amended allowlist dep-by-dep (go.sum-only rows are
mandated `go mod tidy` bookkeeping, not requirements). Every substantive
finding from both prior reviews (S1-S3, M1-M3, F2, F4, F5) is verifiably
addressed in code with tests that exercise the behaviour; deliverables match
the packet and the frozen A-104 contract strings; the ownership map,
security posture and hygiene rules are respected. Remaining items are nits
and correctly-routed advisory notes.

VERDICT: PASS
