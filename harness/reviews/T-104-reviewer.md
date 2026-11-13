# T-104 — reviewer, attempt 4 (correctness / security / architecture)

Scope: the attached diff (byte-consistent with the stored
`harness/reviews/T-104.diff` at every hunk I cross-checked, including the
trap/`CDPATH`/`sed -n`/`grep -qi` lines), reviewed against
`harness/tasks/T-104.md`, the frozen probe contract (`harness/tasks/T-103.md:39-62`,
`acceptance/drafts/probe.spec.ts.txt`, `acceptance/drafts/probe_go_test.go.txt:17-28`),
`internal/config/config.go`, `internal/web/web.go`, `Makefile`,
`harness/tasks/graph.json` (`claims: []` at :165), `harness/state.md`,
`harness/policy.json:37-51`, and the main-worktree pre-change state of
`Dockerfile`, `fly.toml`, `.dockerignore` (context lines match byte-for-byte).
Shell execution is permission-denied in this session (git reads and file reads
only), so the shell logic was re-traced by hand — the same method that caught
attempt 2's two blockers. smoke.sh line numbers refer to the new file per the
hunk header (`@@ -0,0 +1,224 @@`).

## Delta vs attempt 3, verified

Exactly two behaviour-neutral shellcheck fixes in `scripts/smoke.sh` plus the
handoff rewrite; the other four deliverables are the shapes the attempt-3
reviews verified:

- `scripts/smoke.sh:66` — `trap 'rm -rf "$WORKDIR"' EXIT` (SC2064): expansion
  deferred to EXIT-time; `WORKDIR` is assigned at :64-65 and never reassigned,
  so behaviour is identical and correct under `set -u`.
- `scripts/smoke.sh:214` — `ROOT=$(CDPATH='' cd "$(dirname "$0")/.." && pwd)`
  (SC1007): same empty-value guard, quoted. The handoff's line numbers for
  both (66, 214) match the file exactly.

## Attempt-2 blockers — still fixed, independently re-derived

1. **CSRF extraction** (`scripts/smoke.sh:162`):
   `sed -n "s/^.*value=\"\([^\"]*\)\".*$/\1/p"` — with `-n`, sed's
   end-of-cycle auto-print is suppressed and the `p` flag writes the captured
   token exactly once (POSIX; GNU and BSD identical). BRE quoting passes the
   intended pattern; command substitution strips the trailing newline; the
   token arrives byte-exact. The no-match path prints nothing (rc 0) and dies
   at the `[ -n "$csrf" ]` gate (:169); the no-input path dies at the
   `|| return 1` (:156). The whitespace gate (:172-174) converts any future
   extraction-mangling regression into a one-read diagnosis instead of an
   inexplicable 403. The extraction pattern mirrors the frozen Go draft's own
   regexes (`probe_go_test.go.txt:52-54`) — grep for the input tag, sed for
   `value="…"`, both order-tolerant via `[^>]*`.
2. **CSP grep** (`scripts/smoke.sh:121`): `grep -qi "^content-security-policy:"`
   matches both the HTTP/1.1 canonical and the h2 lowercase wire form
   (RFC 9113 §8.1) that curl `-D` dumps behind the Fly edge; no false positive
   on `content-type:`. Grounded: `withSecurityHeaders` sets CSP unconditionally
   on every response (`internal/web/web.go:65, 85-99`).

## Deliverable conformance (all five, packet-exact)

- **Dockerfile:11-12,14-15,18,21-22** — `su-exec` in the apk line, `COPY
  scripts/docker-entrypoint.sh` + `RUN chmod 0755`, `USER root`,
  `ENTRYPOINT ["/docker-entrypoint.sh"]` + `CMD ["/app/server"]`; build stage
  byte-untouched (context only); alpine stays pinned at 3.22; su-exec ~50 KB
  keeps the image ≈29.3 MB (< 40 MB). Exactly `T-104.md:62-74`.
- **scripts/docker-entrypoint.sh:1-15** (mode 100755) — POSIX sh, `set -eu`,
  `[ -d /data ]`-guarded best-effort `chown -R 10001:10001 /data || true`
  (:8-10), `exec su-exec appuser "$@"` (:14) so the server stays PID 1 for
  signals. Verbatim packet pattern; the root window is the minimum the pattern
  requires, on a literal path with no untrusted input.
- **fly.toml:8-19** — `[env]` with exactly the seven packet-verbatim values;
  names verified against `config.Load` (PORT/BASE_URL/DB_PATH/PHOTO_DIR
  required, config.go:76-80; APP_ENV="prod" valid :81-89; TOWN_BBOX parses
  with min<max on both axes :44-62; TZ :108-111). Secrets appear only as
  names in a comment (:9-10); snapshot-retention correctly absent (volume
  flag, H4). Nothing else in the file changed.
- **.dockerignore:9-11,16** — adds `test-run.db`, `test-run-photos/`,
  `selftest*`; `harness/runs` → `harness`; all 13 prior entries preserved;
  `scripts/` stays in the context so the runtime-stage COPY resolves.
- **scripts/smoke.sh** (mode 100755) — `set -euo pipefail` (:15); all five
  packet steps with the `smoke: <check> OK` / `smoke: FAIL:` output contract;
  run header with `date -u` + git sha (:73-74) so two runs bracket a deploy
  (`harness/state.md:65`); argument hygiene (second positional :43-47, unknown
  flag :37-42, no-args :52, `--help` :36); bash 3.2-safe throughout (no
  associative arrays, no `${var,,}`, no mapfile; `10#` base forcing at
  :175; BSD/GNU-safe `mktemp` template at :64).

## Contract grounding of step 4

`GET /healthz` → 200 `ok` (`internal/web/web.go:49-52`); probe fragment
`<span id="probe-count">N</span>` + hidden `csrf_token` input
(`T-103.md:46-54`); valid `HX-Request` POST → 200 / plain POST → 303, bogus
token → 403 with unchanged count (`T-103.md:55-59`,
`probe_go_test.go.txt:22-26`) — the negative control at :200-210 asserts
exactly that. Cookie jar shared across GET/POST (:136, :178, :201) matches the
double-submit flow (`state.md:23`). T-103/T-102 are indeed unmerged
(`internal/web/` holds only web.go, context.go, templates.go, web_test.go), so
building to the frozen contract was the only option (`T-104.md:55-58`); the
script's probe steps fail fast with a clear `smoke: FAIL:` until then, as the
handoff discloses (open issue 2).

## Full re-trace of the shell logic (no new blockers found)

Arg parsing, URL normalization, mktemp/trap cleanup, all four curl flows,
both extractors' rc semantics under `pipefail` (grep no-match → rc 1 → `|| die`),
the numeric case gates before `$(( ))`, the +1 assertion with its documented
race (:191-195), and the 403 negative control were each traced against
bash 3.2/POSIX semantics and hold. Every failure path fails red with a
`smoke: FAIL:` line; I found no path that can false-green.

## Advisory findings (non-blocking; carry-forward + new)

1. **[Medium, disclosed]** `--smoke-suite` prints OK when zero specs matched —
   `make smoke` uses `--pass-with-no-tests` (`Makefile:194-197`); until T-105
   activates the drafts the check cannot fail. Disclosed (handoff open issue
   5); the curl-level probe evidence does not depend on it.
2. **[Medium, packet-sanctioned]** The `HX-Request: true` POST accepts
   `200|303` (`scripts/smoke.sh:181-184`); T-103 pins 200 for HTMX and 303 for
   the no-JS flow. The packet itself says "accept either" (`T-104.md:103-104`)
   and the observed status is printed (:196), so a regression stays visible —
   but a stricter split (assert 200 with HX, 303 without) would be stronger
   evidence.
3. **[Medium, process]** The attempt-4 "machine execution" table
   (handoff :126-144) ran through a scratch makefile/verifier that was deleted;
   nothing committed re-runs `bash -n`/shellcheck/the extractor cases, so the
   claims are coherent but not reproducible by a reviewer or CI. A committed
   `scripts/smoke-selftest.sh` (the `.dockerignore` `selftest*` entry suggests
   such artifacts were anticipated) would convert the assertion into evidence.
   The two-line delta itself is trivially correct by reading, so this does not
   block.
4. **[Medium, handoff doc]** The human DoD docker-run recipe (handoff :160-165)
   omits `-e DB_PATH=… -e PHOTO_DIR=…`, both required by `config.Load`
   (config.go:76-80, 112-114) and not baked into the image (Dockerfile sets
   only PORT). Followed verbatim, the local entrypoint/volume proof false-fails
   at config load; and the handoff's "without a mount the server must still
   boot" is imprecise for the same reason. The Fly path is correct —
   `fly.toml:12-19` supplies both.
5. **[Low]** `https://host/?x` normalizes to `https://host/` (trailing-slash
   strip runs before the query strip, :53-57) → `//healthz` → Go mux 301, no
   `-L`, false red. Unrealistic input; fails red.
6. **[Low]** `grep -o … | head -n 1` under `pipefail` could in principle die
   via SIGPIPE (rc 141) with many matches; practically impossible for the
   contract fragment; fails red.
7. **[Low]** The TLS step has no hard connect timeout on `openssl s_client`
   (`-connect` can hang in pathological network states); `</dev/null` covers
   the normal case.
8. **[Low]** `extract_count`'s anchor requires `id="probe-count"` immediately
   followed by `>` (:144); the frozen contract pins that markup, but any extra
   span attribute added later by T-103+ would false-red here before the
   acceptance spec notices. Contract change would be the feature task's
   problem; noting for the future.

## Security

No secrets anywhere in the diff: fly.toml `[env]` holds public runtime config
only, secret names appear in a comment, and the handoff's docker-run example
uses `<32+ bytes>` placeholders — nothing gitleaks-shaped. The entrypoint's
root window is the packet-prescribed minimum (`[ -d /data ]`, chown -R on a
literal path, immediate `exec su-exec` before CMD). smoke.sh has no injection
surface: BASE_URL is scheme-gated (:59-62) and quoted at every use; counts are
validated numeric before arithmetic; scratch files live in a `mktemp -d` dir
removed by an EXIT trap (:64-66); the CSRF token is never echoed except in the
whitespace-gate failure diagnostic on the local operator's stderr. No
`template.HTML`, SQL, uploads, or path traversal implicated — the diff touches
no Go/templates.

## Architecture

The diff touches exactly the five owned paths (`graph.json:158-164`) plus the
auto-allowed handoff; nothing in `harness/policy.json:37-51`'s protected list.
No Go/TS/template surface, so the ownership map, HTMX/`#map` rule,
parse-don't-validate and typed-ID rules are unimplicated. No "Phase N"
strings, no TODO/FIXME/`console.log`/`panic` (grepped). No new Go/npm
dependency — `su-exec` is the apk package the packet itself prescribes
(`T-104.md:69-70`).

## Tests

`claims: []`, `ui: false`; the packet defines verification as CI
`docker build` (`make ci` → `docker-build`, Makefile:139-143) plus the
deployed smoke run (`T-104.md:23-25`) — no handler or UI change, so AGENTS
rules 6/7 don't apply, and no test was weakened, skipped or special-cased
(no test file is touched; the inert drafts are intact). The handoff lists
every machine-unverified path with exact runnable commands (docker build/run,
flyctl volume/secrets/deploy, the two smoke runs around a deploy), and
`make e2e-task TASK=T-104` → "no claims" exit 0 is by construction
(`Makefile:112-116`) and disclosed as expected.

## Position on the verdict

Attempt 2's two blockers (doubled CSRF token; h2-blind CSP grep) are fixed
correctly and I re-derived both fixes independently against POSIX sed/grep
semantics; attempt 4's delta is two behaviour-neutral shellcheck fixes whose
line numbers and semantics check out; the other four deliverables are
byte-stable and packet-exact; every remaining weakness either fails red
(never false-green) or is disclosed in the handoff. Nothing blocks merge.

VERDICT: PASS
