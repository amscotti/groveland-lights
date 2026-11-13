# T-101 review (reviewer pass, attempt 3)

Reviewed: attached diff (12 files, vs main) — go.mod, go.sum,
`harness/handoffs/T-101.md`, `internal/store/{store,migrations}.go`,
`internal/store/{store,migrations,dbfile_layout}_test.go`,
`internal/store/queries/probe.sql`, `internal/store/gen/**`,
`migrations/{0001_init.sql,migrations.go}` — against the packet
(`harness/tasks/T-101.md`, incl. both prior reviews and the planner retry
context), `harness/tasks/graph.json`, `harness/policy.json`,
`harness/state.md`, guard source (`cmd/harness/guard.go`,
`cmd/harness/scan.go`), `Makefile`, `sqlc.yaml`,
`acceptance/drafts/migrations_go_test.go.txt` (frozen A-104),
`acceptance/contract.json`, `docs/spec/architecture.md`.

Method note: shell execution is denied in this environment. The task files
live in worktree `wt/T-101` (not this checkout — `internal/store/**` is absent
here and main's go.mod matches the diff's "before"). All findings below are
from line-by-line reading of the diff cross-checked against protected
repo files; runtime claims rest on the handoff plus the gate, which re-runs
`make check` regardless.

## The attempt-2 blocker (deps outside allow_deps) — RESOLVED via the sanctioned path

- The prior BLOCK (both reviewers) was five unlisted transitive modules
  committed knowingly through the guard's `// indirect` scanner gap. The
  escalation protocol for exactly this existed all along: `harness/state.md:67`
  — "modernc transitive deps beyond allow_deps (worker BLOCKS — planner
  extends list)".
- This attempt the planner actually did extend the list, through two
  protected, worker-unwritable channels I verified independently:
  - `harness/tasks/graph.json:71-83` (live main checkout) now lists for T-101:
    the original `modernc.org/{sqlite,libc,mathutil,memory,gc/v3}` **plus**
    `github.com/dustin/go-humanize`, `github.com/google/uuid`,
    `github.com/mattn/go-isatty`, `github.com/ncruces/go-strftime`,
    `github.com/remyoudompheng/bigfft` (and a harmless duplicate
    `modernc.org/libc`).
  - `harness/tasks/T-101.md:448-451` (retry context, appended by the
    scheduler/planner): "The allow list has NOW been extended by the planner
    to cover the legitimate transitives … Run `go mod tidy`, ensure the
    requires carry proper `// indirect` markers …, address every other review
    finding …, and re-commit."
  Both files are protected (`harness/policy.json:37-51`: `harness/tasks/graph.json`
  at :41; enforced by `checkProtectedPaths` + `VerifyManifest`,
  `cmd/harness/guard.go:85-91,118-129`), the worker's branch diff touches
  neither, and the amendment is present on main — so it is not worker-forged.
- Dep-by-dep check of the diff's go.mod against the amended list:
  `modernc.org/sqlite v1.62.0` direct (go.mod:5, packet's pin, matches
  architecture.md:54) ✓; indirect adds go-humanize/go-uuid/go-isatty/
  go-strftime/bigfft/libc/mathutil/memory ✓ — every one allow-listed. The
  `golang.org/x/*` indirect lines pre-exist on main (verified against this
  checkout's go.mod:7-14; they serve the `tool` directive) and are not
  additions. `// indirect` markers are exactly where `go mod tidy` puts them,
  and fmt-check's `tidy -diff` (Makefile:54) enforces no drift.
- go.sum-only hash rows for modules absent from go.mod (modernc.org/cc/v4,
  ccgo/v4, fileutil, gc/v2, gc/v3, goabi0, opt, sortutil, strutil, token,
  hashicorp/golang-lru/v2, google/pprof) are module-graph bookkeeping written
  by `go mod tidy` for the pinned driver — they are not requirements and not
  importable (any import forces a go.mod require, which the guard's
  `checkDeps` compares against `main:go.mod`, guard.go:183-203). Not a rule-3
  breach; informational only. Note `modernc.org/gc/v3` is allow-listed yet
  unused in go.mod — planner over-provision, harmless.
- The tooling note in the handoff (rename done via a self-deleting `go run`
  helper under `internal/store/testdata/` because `mv`/`rm` are denied) was
  checked for residue: the diff contains no testdata files and no protected
  path is touched; the helper lived inside the task's own ownership. The
  rename itself was review-requested (F2/M2). Transparency in the handoff is
  the right call; no violation.

## Prior review findings — each addressed or properly dispositioned

- **S1 (checksum never asserted independently)** — addressed.
  `independentSum` (`internal/store/migrations_test.go:~46-65` per diff)
  reads bytes via `fs.ReadFile(migrations.Files, …)` and computes
  `sha256.Sum256` itself, bypassing `loadMigrations`;
  `checkRecordedChecksum` now requires the DB value to equal BOTH the
  independent digest and the runner-derived one. A wrong-but-consistent hash
  can no longer pass.
- **S2 (recorded-without-file refusal untested)** — addressed with two new
  tests: `TestRecordedVersionWithoutEmbeddedFileRefusesOpen` (real DB, ghost
  `0002_ghost` row inserted via the raw driver, re-`Open` refuses naming the
  version and NOT as `ErrChecksumMismatch`) and
  `TestApplyMigrationsAppliesUnrecordedFilesInOrder` (`fstest.MapFS` with
  0002 referencing 0001's table — order violation fails the apply; versions
  recorded in order; each checksum equals an independent sha256 of the
  synthetic bytes; re-apply is a no-op). New code paths now have new tests
  (rule 7/2).
- **S3 (guard error asserted by labels only)** — addressed.
  `TestMigrationChecksumGuard` now asserts the refusal contains the actual
  tampered 64-hex value (`strings.Contains(err.Error(), tt.tamperWith)`,
  incl. the uppercase variant that pins case-sensitive comparison) and the
  embedded file's sum, plus `errors.Is(err, ErrChecksumMismatch)`.
- **M1 (schema shape without constraints)** — addressed. `checkProbeColumns`
  asserts via `PRAGMA table_info`: id INTEGER/pk=1, n INTEGER/notnull=1/
  dflt "0"/pk=0, and via `sqlite_master.sql`: `CHECK (id = 1)`, `NOT NULL`,
  `DEFAULT 0`. Dropping the CHECK/default now fails the subtest.
- **M2+F2 (layout-pin test name/assertion)** — addressed. File renamed
  `dbfile_layout_test.go`; test is `TestChecksumIsFirstAndOnlyHexTokenInDBFile`
  and now asserts the recorded checksum is the ONLY `[0-9a-f]{64}` token in
  the closed main DB file (previously extras were only logged). The pin
  remains the sanctioned canary: the frozen A-104 draft byte-tampers exactly
  the first such token (`acceptance/drafts/migrations_go_test.go.txt:216-229`)
  and its header (:11-17) pre-authorizes exactly one T-105 token-index edit
  with documentation. Failure message points at both.
- **M3 (unusable-path test too weak)** — addressed. Table of missing-parent /
  `?` / `#` / `%`, each asserting the refusal names the path AND the
  offending substring.
- **F4 (dsn() does not escape the path)** — addressed in the store itself:
  `checkDSNPath` (`internal/store/store.go:~36-46` per diff) refuses `?`,
  `#`, `%` with a typed, `errors.Is`-able `errBadDSNPath` before any open;
  covered by the table plus a `dsn_rejection_wraps_typed_error` subtest.
  Fail-closed is the right call; residual UX for legitimately odd filenames
  is correctly deferred to T-103's config boundary (handoff open issue #3).
- **F5** — addressed: `var ErrNotFound = errors.New("store: not found")`.
- **F3, F6** — no-change dispositions, matching what the prior reviewers
  themselves concluded (sanctioned canary; single-process architecture,
  docs/spec/architecture.md:4 "one writer").

## Verified correct against packet + contract

- **Deliverables**: `migrations/0001_init.sql` matches the packet DDL
  byte-for-byte incl. the seed row; runner-owned `schema_migrations` matches
  the packet's column spec exactly (version TEXT PRIMARY KEY / sha256 TEXT
  NOT NULL / applied_at TEXT NOT NULL) and is NOT in 0001; one transaction
  per file with the bookkeeping insert inside the same tx (migration+record
  are atomic — good); version = filename sans extension; sha256 = lowercase
  hex of file bytes; applied_at RFC3339 UTC; unrecorded files apply in
  `fs.Glob` lexical order; idempotent reopen writes nothing.
- **A-104 frozen strings**: refusal text
  `store: migration 0001_init.sql checksum mismatch (recorded X, file Y): checksum mismatch`
  (`internal/store/migrations.go:~110-113` per diff) wraps
  `ErrChecksumMismatch` and satisfies the draft's lowercased-match
  requirements ("0001_init.sql" + "checksum", draft :298-304). The HTTP half
  (non-zero exit) is T-103's, as the packet directs.
- **Frozen API**: `Open(ctx, dbPath) (*DB, error)`, `Close`, `ProbeCount`,
  `ProbeInc` (UPDATE…RETURNING inside a tx), ctx-first everywhere, sentinels
  `ErrNotFound`/`ErrChecksumMismatch` — exactly as the packet specifies and
  as the handoff freezes for T-103.
- **Pragmas**: DSN-riding per-connection pragmas (WAL/busy_timeout 5000/
  foreign_keys) is the correct mechanism for a pooled handle; asserted by
  PRAGMA reads + `-wal` sidecar existence.
- **sqlc**: `queries/probe.sql` ↔ `gen/**` consistent with `sqlc.yaml:3-11`
  (package gen, out internal/store/gen, emit_interface → querier.go present);
  `generate-check` (Makefile:77-81) fails on drift, so committed state is
  machine-verified at the gate.
- **Security surface**: no handlers/routes/templates added (packet forbids
  wiring; `cmd/server`, `internal/web`, `web/` untouched) → CSRF/XSS/
  template.HTML/upload-limit checks N/A. All product SQL is static or
  parameterized — no injection. `dbPath` is operator config, not request
  input; `Open` mkdirs nothing and now fails closed on DSN-ambiguous paths.
  No secrets in output (refusals print public file checksums only).
  Architecture ownership respected: store is sole driver importer
  (architecture.md:18).
- **Tests are real**: real temp-dir databases, real close/reopen cycles,
  raw-driver tampering, byte-level file reads, 10-goroutine concurrent incs
  asserting exactly {1..10} + final count, persistence across Close→Open,
  `ErrNotFound` on deleted row. Table-driven with assertion-style names; no
  skips/only/fixme (also guard-enforced, guard.go:159-168); no golden files.
  The `fstest.MapFS` case runs the real `applyMigrations` against a real DB —
  synthetic input, not a mocked system.
- **Hygiene**: no "Phase N" strings under cmd/server, internal, web,
  migrations (`phase-grep` pattern wouldn't match anything in the diff);
  no TODO/FIXME/panic markers in non-test code; no console.log; no
  out-of-ownership paths in the diff (all under `internal/store/**`,
  `migrations/**`, `go.mod`, `go.sum` + the sanctioned handoff file,
  mirroring `checkOwnership`'s allowlist, guard.go:132-146).
- **Handoff accuracy spot-checks**: Makefile targets cited all exist and
  behave as claimed (check-fast :68-69 incl. `tidy -diff`; e2e-task requires
  `TASK=` :111-116; no active spec carries @A-103/@A-104 — the specs are
  inert `.txt` drafts — so 0 Playwright tests with `--pass-with-no-tests`
  is the expected result); `attempts: 2` in graph.json is scheduler
  bookkeeping lag, consistent with prior reviews.

## New nits (non-blocking, informational)

1. **N1 — handoff test-count off by one**: "10 top-level store tests" — the
   diff has 11 (5 in store_test.go, 5 in migrations_test.go, 1 in
   dbfile_layout_test.go). Cosmetic.
2. **N2 — handoff open issue #4 mischaracterizes the Makefile**: `make
   e2e-task T=T-101` does not "silently match nothing" — `TASK` stays empty
   and the target exits 2 with a usage message (Makefile:113). The AGENTS.md
   `T=` vs Makefile `TASK=` docs nit is real, but the failure is loud, not
   silent.
3. **N3 — `checkDSNPath` edge**: a dbPath that itself begins with `file:`
   (or contains spaces) is not rejected and yields a oddly-named on-disk
   file rather than a refusal. Still fail-safe rather than unsafe;
   T-103's config parse is the right boundary to tighten.
4. **N4 — advisory, planner-owned** (already handoff open issue #1):
   `extractGoDep` (cmd/harness/scan.go:130-139) still ignores 4-field
   `// indirect` block lines, so the allowlist is not machine-enforced for
   indirect requires. It stopped mattering here because everything is now
   explicitly allow-listed, but a harness task should close the gap so the
   next transitive set can't ride in unreviewed.

## Limitations

Could not execute `make check-fast` / `go test -race` / `go mod tidy`, nor
inspect worktree `wt/T-101` or commits `6e7ae1d`/`8c2b363`/`b8af66a`
(shell denied). Runtime/tidy/clean-worktree claims rest on the handoff; the
gate re-runs `make check` and the guard (nine checks incl. deps, ratchet,
manifest, clean tree) before merge, and nothing in the diff contradicts the
claims.

## Bottom line

The single prior blocker is cured through the process the harness prescribes
(worker BLOCK → planner amends allow_deps → retry consumes it), verified
against two protected sources, and the diff's dependency set now matches the
amended allowlist exactly. Every substantive finding from both prior reviews
(S1-S3, M1-M3, F2, F4, F5) is verifiably addressed in the code with tests
that exercise the behaviour; the deliverables match the packet and the frozen
A-104 contract strings; ownership, hygiene and security posture are clean.
Remaining items are nits and correctly-routed advisory notes.

VERDICT: PASS
