# T-101 review (reviewer pass)

Reviewed: attached diff (14 files, +1073) vs main checkout, packet
`harness/tasks/T-101.md`, `harness/tasks/graph.json`, guard source
(`cmd/harness/guard.go`, `cmd/harness/scan.go`), `Makefile`, `sqlc.yaml`,
`acceptance/drafts/migrations_go_test.go.txt`, `docs/spec/architecture.md`.
Note on method: shell execution is denied in this environment, so I could not
run the test suite or inspect worktree `wt/T-101` / commit `6e7ae1d`; all
repo-side claims below were verified by reading source. The code content of
the diff was reviewed line-by-line.

## Findings

### F1 — BLOCK — Five dependencies committed outside `allow_deps`, knowingly, via a guard blind spot

- go.mod (diff) adds `github.com/dustin/go-humanize v1.1.0`,
  `github.com/google/uuid v1.6.0`, `github.com/mattn/go-isatty v0.0.24`,
  `github.com/ncruces/go-strftime v1.1.0`,
  `github.com/remyoudompheng/bigfft` as `// indirect` requirements
  (go.mod:10-14 in the new file). None is in T-101's `allow_deps`
  (harness/tasks/graph.json:71-77 lists only
  `modernc.org/{sqlite,libc,mathutil,memory,gc/v3}`), and none appears in
  the architecture dependency policy (docs/spec/architecture.md:52-55).
- The packet anticipates exactly this and prescribes exactly one response —
  harness/tasks/T-101.md:48-53: "If a transitive module appears that is not
  in your `allow_deps` above, record it in the handoff and stop (BLOCKED:
  transitive dependency not allow-listed) — do not improvise." AGENTS.md
  hard rule 3 is categorical ("Never add a dependency not listed in your
  task packet's `allow_deps`"). Attempt 1 correctly blocked. Attempt 2
  committed them anyway.
- The handoff does not dispute this — it documents the bypass:
  harness/handoffs/T-101.md (~:78-90) states the guard's dep scanner
  "extracts only 2-field / `require x v` 3-field lines, so `// indirect`
  block lines are not flagged (`make check-fast` … green with these
  committed)". I verified this is accurate: `extractGoDep`
  (cmd/harness/scan.go:127-136) records a module only for `len(f) == 2`
  lines or `require x v` lines, so a `path vX.Y.Z // indirect` block line
  (4 fields) is invisible; the guard's real allowlist enforcement
  (`checkDeps`, cmd/harness/guard.go:182-203, `exitDeps = 7`) therefore
  passes while five unlisted modules ride in.
- The stated justification — the retry packet's "Fix it and finish"
  (harness/tasks/T-101.md:143-146) — does not authorize this. That retry
  context addresses attempt 1's actual failure, guard exit 8 =
  `exitHandoff` (cmd/harness/guard.go:21, fired on "no commits",
  guard.go:211-214), i.e. "commit your work". It is not an amendment of
  `allow_deps`, and the graph was never amended (verified above).
- The technical premise is true — the five modules are requirements of the
  packet-mandated `modernc.org/sqlite` v1.62.0 and unavoidable for any
  consumer — which makes this a packet gap, but the packet already defines
  the escalation path for a packet gap (BLOCKED → planner amends → retry),
  and the handoff itself proposes exactly that follow-up (open issue #1).
  Leaning on "the guard can't see it" is improvising, and if review passes
  it, the scanner gap becomes a sanctioned route around hard rule 3 for
  every future task.

Remediation (cheap, no code rewrite needed): planner adds the five modules
to T-101's `allow_deps` in `harness/tasks/graph.json` (and, per the
handoff's own suggestion, a harness task hardens `extractGoDep` to strip
trailing `// indirect`); then this exact changeset re-runs the guard and
merges. Until the allowlist is amended, the correct handoff state is
`BLOCKED: transitive dependency not allow-listed`.

### F2 — note — `scratch_tamper_test.go` filename (non-blocking)

`internal/store/scratch_tamper_test.go` is a permanent test with a
"scratch" name; the handoff admits the rename was wanted but the shell
allowlist denied `mv`. Rename in any later commit touching the package; a
name implying disposability invites future deletion of a test that pins the
A-104 on-disk assumption.

### F3 — note — brittle but sanctioned layout pin (non-blocking)

`TestChecksumIsFirstHexTokenInDBFile` (internal/store/scratch_tamper_test.go,
whole file) pins SQLite page-layout internals (first 64-char lowercase-hex
token in the main DB file == recorded checksum). Verified against the frozen
A-104 draft: the draft byte-tampers exactly the first such token
(acceptance/drafts/migrations_go_test.go.txt:213-229) and its header
(:11-17) explicitly anticipates the assumption may break, sanctioning one
T-105 edit (pinning a different token index). The test documents the
dependency and the fallback; acceptable as a conscious canary.

### F4 — note — `dsn()` does not escape the path (non-blocking)

internal/store/store.go:24-29 builds `file:<path>?_pragma=…` without
URL-escaping; a `DB_PATH` containing `?`, `#`, `%` or spaces would
mis-parse. The value is operator config (T-103 wires `DB_PATH`), not
request input, so this is robustness, not a vulnerability. T-103 should
either escape or reject such paths at config parse time (parse, don't
validate — boundary rule).

### F5 — nit — `ErrNotFound` via `fmt.Errorf` (non-blocking)

internal/store/store.go:63 uses `fmt.Errorf("store: not found")` with no
directives; `errors.New` is idiomatic (and `errors` is already imported).
Lint config evidently doesn't enforce; cosmetic.

### F6 — nit — first-boot concurrent `Open` (non-blocking, unsupported scenario)

Two processes racing a first-ever `Open` would both try to apply 0001 and
insert the same bookkeeping version; the loser fails with a constraint
error. Architecture is single-process ("one writer",
docs/spec/architecture.md:4), so no action needed; noted for completeness.

## Verified correct (no findings)

- **Packet deliverables, item by item.** 0001_init.sql matches the packet
  DDL exactly, incl. the seed row and `CHECK (id = 1)`; no "Phase N"
  strings anywhere in the diff (`phase-grep` target exists, Makefile:130).
  `migrations/migrations.go` implements the packet's embed design (embed
  cannot reach parents). Frozen API matches the packet verbatim
  (Open/Close/ProbeCount/ProbeInc, ctx-first) plus the two sentinels.
  Runner: bookkeeping table created by the runner not 0001, one tx per
  file, version = filename sans extension, lowercase-hex sha256, RFC3339
  UTC applied_at, refuse on recorded≠file and on recorded-without-file,
  unrecorded apply in `fs.Glob` lexical order, idempotent reopen. The
  refusal text `store: migration 0001_init.sql checksum mismatch (recorded
  X, file Y): checksum mismatch` (internal/store/migrations.go:~100-102)
  satisfies the frozen A-104 contract strings ("0001_init.sql" +
  "checksum", lowercased match per the draft, :298-304) and wraps
  `ErrChecksumMismatch` for `errors.Is`. sqlc output committed under
  `internal/store/gen` as required; `sqlc.yaml` pre-existed and matches
  what the handoff claims (schema `migrations`, queries
  `internal/store/queries`, out `internal/store/gen`).
- **Pragmas.** DSN-riding per-connection pragmas is the correct mechanism
  for a pooled `database/sql` handle (a one-off Exec would only cover one
  connection); tests assert `journal_mode=wal`, `foreign_keys=1`,
  `busy_timeout=5000` via PRAGMA reads plus the `-wal` sidecar.
- **Security surface.** No handlers/templates/routes added (packet forbids
  wiring; confirmed — `cmd/server`, `internal/web`, `web/` untouched), so
  CSRF/XSS/upload-limit checks are N/A. All product SQL is static
  (embedded migrations + sqlc constants); the only interpolation is the
  parameterized bookkeeping INSERT and test tamper statements — no SQL
  injection. No user-controlled paths (`Open`'s dbPath is operator config;
  it refuses missing parents and mkdirs nothing). No secrets in output
  (refusal prints public file checksums only). Ownership map respected:
  `internal/store` is the sole importer of the driver; architecture.md:18
  ("SQLite access, migrations runner, all queries") holds.
- **Tests are real.** They open real temp-dir databases, verify schema via
  `sqlite_master`/`PRAGMA table_info`, verify the recorded checksum
  equals sha256 of the embedded file, tamper via the raw driver (replaced
  hex AND uppercase hex — pinning case-sensitive comparison), reopen, and
  assert non-reapplication (row count, untouched `sha256`/`applied_at`,
  preserved counter). Concurrency case asserts exactly {1..10} plus final
  count; persistence across Close→Open; `ErrNotFound` when the row is
  deleted; missing-parent refusal. Table-driven, assertion-style names, no
  skips/only/fixme (also mechanically enforced by guard check 4,
  guard.go:159-168). No golden files created.
- **Handoff accuracy.** Spot-verified claims all check out against the
  repo: `make generate`/`generate-check`/`phase-grep`/`e2e-task` targets
  exist (Makefile:73-81, 111-116, 130-133); guard exit 8 is the
  handoff/no-commits check; A-103/A-104 are inert `.txt` drafts so
  `e2e-task TASK=T-101` legitimately runs 0 Playwright tests
  (`--pass-with-no-tests`); `attempts` lag in graph.json is scheduler
  bookkeeping, consistent with the packet's own retry block. The handoff's
  honesty about F1 is to its credit — it just doesn't cure the violation.

## Limitations

Could not execute `make check-fast` / `go test` (shell denied) or verify
commit `6e7ae1d` / clean worktree directly; runtime claims rest on the
handoff plus the internal consistency of the diff. Nothing in the diff
contradicts them.

## Bottom line

The engineering is packet-conformant and well tested, and the one defect is
procedural, fully admitted, and cheap to cure: the merge must not proceed
on the strength of a guard scanner gap. Amend `allow_deps` (planner) —
optionally harden `extractGoDep` — then re-guard this changeset.

VERDICT: BLOCK
