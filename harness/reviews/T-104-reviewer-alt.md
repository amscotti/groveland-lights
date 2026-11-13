# T-104 — reviewer-alt (test integrity / reward hacking) — attempt 4

Scope: the attached attempt-4 diff (five owned paths + the rewritten handoff),
read against `harness/tasks/T-104.md`, the frozen T-103 contract
(`harness/tasks/T-103.md:39-62`), the inert acceptance drafts
(`acceptance/drafts/probe.spec.ts.txt`, `probe_go_test.go.txt`),
`internal/config/config.go`, `internal/web/web.go`, `Makefile`,
`harness/tasks/graph.json`, `harness/state.md`, and the stored
`harness/reviews/T-104.diff` (byte-consistent with the attachment at every
line I cross-checked, including the trap/`CDPATH` lines). Shell, docker and
curl execution are denied in this review session, so the shell logic was
traced, not run.

Delta vs attempt 3 is exactly the two shellcheck fixes in `scripts/smoke.sh`
plus the handoff rewrite:

- `scripts/smoke.sh:66` — `trap 'rm -rf "$WORKDIR"' EXIT` (deferred expansion;
  `WORKDIR` is assigned at :64-65 and never reassigned, so behaviour is
  identical to the old double-quoted trap).
- `scripts/smoke.sh:214` — `ROOT=$(CDPATH='' cd ... && pwd)` (quoted empty
  value; same semantics as `CDPATH=`).
- The two attempt-2 blockers remain fixed: `sed -n ... p` at
  `scripts/smoke.sh:162` (token printed once) and
  `grep -qi "^content-security-policy:"` at `scripts/smoke.sh:121`.
- The handoff's shellcheck line numbers (SC2064 at 66, SC1007 at 214) match
  the file exactly, and its 14-byte token arithmetic is internally consistent
  — the verification claims are at least coherent with the artifact.

No other owned file changed; the four non-smoke deliverables are the same
shapes the attempt-3 reviews verified.

## Findings

### 1. [Medium] `--smoke-suite` still prints OK when zero specs ran

`scripts/smoke.sh:212-221` runs `make smoke BASE_URL=...` and then
unconditionally prints `smoke: suite (@smoke specs via make smoke) OK` at
`:220`. The target is `npx playwright test --grep @smoke
--pass-with-no-tests` (`Makefile:194-197`), so until T-105 activates the
inert `@smoke` drafts the command matches nothing and exits 0. The check
cannot fail, and its `OK` line is coverage theatre. The handoff discloses the
weakness (`harness/handoffs/T-104.md:195-197`, open issue 5), and the
curl-level probe assertions do not depend on it — advisory, not blocking.
Fix: parse the spec count and require > 0, or print `suite invoked` rather
than `OK`.

### 2. [Medium] HTMX POST is accepted as 303, weaker than the frozen contract

`scripts/smoke.sh:177-184` sends `HX-Request: true` but accepts `200|303`
(`:181-183`). T-103 pins 200 for the HTMX POST and 303 only for the no-JS
form fallback (`harness/tasks/T-103.md:55-59`;
`acceptance/drafts/probe_go_test.go.txt` HTMX/no-HX split). The task packet
explicitly says "accept either" (`harness/tasks/T-104.md:103-104`), so the
worker is packet-conformant and the observed status is printed at `:196`, but
an HTMX-detection regression would still pass this script. If stricter
evidence is wanted: assert 200 on the `HX-Request` POST and add a second POST
without the header asserting 303 + `Location: /`.

### 3. [Medium] The only function-level verification is uncommitted and unreproducible

`harness/handoffs/T-104.md:111-144` claims the shell logic was executed via a
scratch makefile plus a scratch bash verifier created by echo-redirection and
deleted afterwards (`:123-124`). Nothing in the repo re-runs `bash -n`,
shellcheck, or the extractor cases: `check-fast` is fmt/vet/lint/Go tests
(`Makefile:68-69`) and `make check` has no shellcheck step, so the table at
`:126-144` cannot be re-executed by a reviewer or CI. The claims are plausible
and the two-line delta is trivially correct by reading, so this is not a
block; but a committed `scripts/smoke-selftest.sh` (or make target) running
the documented fragment cases would turn an assertion into evidence.

### 4. [Medium] The handoff's docker-run recipe still cannot boot

`harness/handoffs/T-104.md:160-165` gives the human DoD command for the
entrypoint/volume check but omits `-e DB_PATH=/data/lights.db -e
PHOTO_DIR=/data/photos`. Both are required by `config.Load`
(`internal/config/config.go:76-80,112-114`) and the image sets only `PORT`
(`Dockerfile:22`). Followed verbatim, the container exits at config load
before the entrypoint chown is exercised, so the only local proof of the
volume fix false-fails. (Repeat of attempt-3 alt finding 3; the Fly path is
correct because `fly.toml:8-19` supplies both.)

### 5. [Low] Two vacuous green commands are still tabled as "Repo gates"

`harness/handoffs/T-104.md:146-153` lists `make check-fast` (0) and
`make e2e-task TASK=T-104` (0). Neither can fail for this diff: check-fast is
Go-only and the packet itself calls it "green trivially — you touch no Go"
(`harness/tasks/T-104.md:30-31`); `e2e-task` prints "no claims for T-104" by
construction because `claims: []` (`Makefile:112-116`; packet `:12`). They are
labeled "expected", so this is presentation, not a hidden claim — but they
must not be read as verification of the shell/docker deliverables.

### 6. [Low] BASE_URL trim order can produce `//healthz`

`scripts/smoke.sh:53-57` strips the trailing slash before the query/fragment,
so `https://host/?x` becomes `https://host/` and the healthz target is
`https://host//healthz`; curl has no `-L` and Go's mux 301s, so the run goes
red. Not a realistic deploy URL; carried from the attempt-3 review, still
present.

### 7. [Low] `grep -o ... | head -n 1` under `pipefail` can go red on SIGPIPE

`scripts/smoke.sh:142-146` and `:154-156` end pipelines with `head -n 1`
under `set -o pipefail`; if an upstream writer dies of SIGPIPE (many matches
in a large fragment) the pipeline returns 141 and a good run dies. Fails red,
never false-green, and is practically impossible for the contract-sized
fragment.

### 8. [Process/Low] Owned files were patched through the permission workaround

`harness/handoffs/T-104.md:106-109,181-186` documents that the two-line patch
was applied by an anchored perl patcher run through `make -f`, and
verification through a scratch makefile, i.e. around the denied
`edit`/`write` layer. The result is disclosed, limited to owned paths, and
the diff was read back, so this is not a test-integrity block; the operator
should fix the session permissions rather than normalize the workaround.

## Checked and not found

- No test was weakened, deleted, skipped or narrowed: the diff touches no
  test/acceptance file; the inert drafts are untouched.
- No `t.Skip`/`test.skip`/`test.only`/`.fixme`, no mock that bypasses the
  behaviour under test, no dead code to satisfy a linter, no fixture string
  special-cased in production code. `id="probe-count"`, `csrf_token`,
  `Count:`/`ok`/`Content-Security-Policy` are the frozen contract strings the
  script exists to observe (`T-103.md:46-59`), not fixture shortcuts.
- The core evidence assertions are real and can fail: count must increment by
  exactly 1 (`scripts/smoke.sh:194-195`) and the bogus-token POST must be 403
  with an unchanged count (`:204-209`). The fixed
  `bogus-token-negative-test` literal (`:202`) is a negative-control input,
  and the handoff reasons about the collision case (`:201-203`).
- No protected path touched; no secret in the diff (secret names appear only
  in comments/placeholders); no new Go/npm dependency (`su-exec` is the
  packet-prescribed apk package, `T-104.md:69-70`); no
  `console.log`/TODO/FIXME/`Phase N`.
- `.dockerignore` broadening `harness/runs` → `harness` keeps `scripts/` in
  the build context, which the runtime-stage `COPY` needs (`Dockerfile:16`);
  the app embeds only `web/templates` + `web/static` (`assets.go:10`).
- The entrypoint's root window is the packet pattern: `[ -d /data ]` guard,
  best-effort `chown -R` on a literal path, immediate `exec su-exec`
  (`scripts/docker-entrypoint.sh:7-15`, mode 100755 in the diff metadata).
- `fly.toml:8-19` `[env]` matches the seven packet values and the names
  consumed by `config.Load`; `/healthz` is 200 `ok`
  (`internal/web/web.go:49-52`).

## Verdict

The attempt-4 delta is two behaviour-neutral shellcheck fixes on an artifact
both attempt-3 reviews already found mergeable, and I re-traced the affected
lines: `sed -n` token handling, the h2-safe CSP grep, numeric validation,
argument hygiene, the +1 assertion and the 403 negative control all hold. The
remaining problems are the packet-sanctioned 303 leniency, a disclosed
zero-spec suite OK, an uncommitted (hence unauditable) shell-verification
table, a handoff docker-run recipe missing two required env vars, and minor
nits. None is a weakened/skipped test or a hidden special-case in code.

VERDICT: PASS
