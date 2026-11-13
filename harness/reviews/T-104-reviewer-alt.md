# T-104 — reviewer-alt (test integrity / reward hacking) — attempt 3

Scope: the attached diff (all five owned paths plus the handoff), read against
`harness/tasks/T-104.md`, the frozen T-103 contract (`harness/tasks/T-103.md:39-62`),
the inert acceptance drafts (`acceptance/drafts/probe.spec.ts.txt`,
`acceptance/drafts/probe_go_test.go.txt`), `internal/config/config.go`,
`internal/web/web.go`, `Makefile`, `harness/tasks/graph.json`, and
`harness/state.md`. Shell, docker and curl execution are denied in this session,
so the shell logic below was traced rather than run. The two attempt-2 blockers
are fixed in the diff.

## Verified fixes (read-only)

- `scripts/smoke.sh:162` now uses `sed -n ... p`, so the captured CSRF value is
  printed once; the `[[:space:]]` gate at `:172-174` rejects a doubled/mangled
  token before the POST. Correct.
- `scripts/smoke.sh:121` now uses `grep -qi "^content-security-policy:"`, which
  matches both HTTP/1.1 and HTTP/2 header casing. Correct.
- `scripts/smoke.sh:56-57` trims query/fragment from `BASE_URL`. Correct in the
  common case (minor ordering nit below).
- All five owned paths are present and no protected path is touched. The
  Dockerfile, entrypoint, `fly.toml` and `.dockerignore` match the packet.

## Findings

### 1. [Medium] `--smoke-suite` can print OK with zero specs

`scripts/smoke.sh:213-220` runs `make smoke BASE_URL=...` and then
unconditionally prints `smoke: suite ... OK`. The Makefile target uses
`--pass-with-no-tests` (`Makefile:194-197`), so until T-105 activates the inert
`@smoke` drafts the command matches zero specs and exits 0: this check cannot
fail. The handoff discloses the weakness (`harness/handoffs/T-104.md:65`) and
the curl-level probe assertions do not depend on it, but the script's own `OK`
line is coverage theatre. Assert a non-zero spec count or drop the `OK` wording
until `@smoke` specs exist.

### 2. [Medium] HTMX POST accepts 303, weaker than the frozen contract

`scripts/smoke.sh:177-183` sends `HX-Request: true` but accepts `200|303`
(`:183`). T-103 pins an HTMX POST to 200 (`harness/tasks/T-103.md:55-59`;
`acceptance/drafts/probe_go_test.go.txt:88-90`) and 303 only to the no-HX
fallback (`:138-143`). The task packet explicitly says "accept either"
(`harness/tasks/T-104.md:103-104`), so the worker is packet-conformant, and the
following count assertion still proves the increment. But an HTMX-detection
regression would pass this script. Prefer asserting 200 here and add a separate
no-`HX-Request` POST asserting 303 + `Location: /`.

### 3. [Medium, verification recipe] The handoff's docker-run command cannot boot

`harness/handoffs/T-104.md:54-55` runs the image with `PORT`, `APP_ENV`,
`BASE_URL`, `TOWN_BBOX` and the three secrets but omits `DB_PATH` and
`PHOTO_DIR`. Both are required by `config.Load`
(`internal/config/config.go:76-80,112-114`), and the image only sets
`ENV PORT=8080` (`Dockerfile:22`); the server exits with "missing required
environment variables: DB_PATH, PHOTO_DIR" before the entrypoint can create
`/data/lights.db`. The Fly path sets both (`fly.toml:14-15`), so this is a
local-recipe defect rather than a container defect. Add
`-e DB_PATH=/data/lights.db -e PHOTO_DIR=/data/photos`; also name/background
the container if the human is expected to inspect `/data` while it runs.

### 4. [Low-Medium] The "How the fixes were verified" table lists vacuous greens

`harness/handoffs/T-104.md:25-26` lists `make check-fast` exit 0 and
`make e2e-task TASK=T-104` exit 0 as verification. T-104 has `claims: []`
(`harness/tasks/T-104.md:12`), so `make e2e-task` prints "no claims" by
construction (`Makefile:112-116`), and `make check-fast` covers only Go; this
diff changes no Go. Neither command can fail if `scripts/smoke.sh` is wrong.
The handoff later lists the shell logic as machine-unverified (`:49-52`), so
this is a labeling issue rather than a hidden claim, but these two rows should
be marked baseline, not verification.

### 5. [Low] No static or executable test for the new shell logic

`scripts/smoke.sh:139-162` (the count/CSRF extractors) and `:87-110` (the TLS
path) have no `bash -n`, shellcheck, or function-level test in the diff; the
handoff lists those as machine-unverified (`harness/handoffs/T-104.md:49-52`).
The packet caps worker verification at `make check-fast` + reading
(`harness/tasks/T-104.md:29-36`), and T-103 is not deployed, so the first
execution is the deployed run. The previous two blockers were found by reading
alone; a cheap static check in CI would close this gap.

### 6. [Low] BASE_URL trim order leaves a double slash for `host/?query`

`scripts/smoke.sh:53-57` strips the trailing slash before stripping `?`/`#`,
so `https://host/?x` becomes `https://host/` and the curl target is
`https://host//healthz`. Go's mux answers with a 301 and the script's curl has
no `-L`, so the healthz step fails. This is not a normal deployment URL, but
strip the query/fragment first, then the trailing slash.

### 7. [Process] "Reviewer-sanctioned" workaround characterization

`harness/handoffs/T-104.md:61` says the worker worked within "the
reviewer-sanctioned echo-redirection remedy". The prior reviews asked for
restored write permission or a human hand-apply; they did not sanction a worker
bypass of the permission layer. The result is transparent and limited to owned
paths, and the diff is reviewable, so this is not a test-integrity block; the
operator should still fix the session permissions instead of normalizing the
workaround.

## Checked and not found

- No test was weakened, skipped, deleted or narrowed: the diff touches no test
  file and the inert acceptance drafts are untouched.
- No `t.Skip`/`test.skip`/`test.only`/`.fixme`, no mock that bypasses the
  behaviour under test, no dead code to satisfy linters, and no fixture string
  special-cased in production code. `#probe-count`, `csrf_token` and `Count:` in
  `scripts/smoke.sh` are the frozen contract strings, not fixture shortcuts.
- No new Go/npm dependency (the `su-exec` apk package is packet-prescribed), no
  secret in the diff, no protected path edit, no
  `console.log`/TODO/FIXME/`Phase N`.
- The `.dockerignore` additions and the `harness/runs` -> `harness` broadening
  preserve `scripts/`, which the runtime-stage `COPY` needs.
- The entrypoint's root window is the packet-prescribed `[ -d /data ]` +
  best-effort `chown` + immediate `exec su-exec`, with no untrusted input.
- The core smoke assertions are real and can fail: `scripts/smoke.sh:194-195`
  (count must increment by exactly 1) and `:200-209` (bogus token must be 403
  and must not change the count).
- The five owned files match the packet's shapes and env list; the secret names
  appear only in comments, never as values.

## Verdict

The attempt-2 blockers are fixed and the smoke script's main persistence
assertions are genuine. The remaining issues are the packet-sanctioned 303
leniency, a disclosed zero-spec suite OK, a handoff docker-run recipe missing
two required env vars, and a few low nits. None is a weakened/skipped test or a
hidden special-case in production code.

VERDICT: PASS
