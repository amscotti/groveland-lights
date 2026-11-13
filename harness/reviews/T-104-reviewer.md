# T-104 — reviewer, attempt 2 (correctness / security / architecture)

Scope: the attached diff (byte-identical to the stored `harness/reviews/T-104.diff`;
cross-checked at the `-checkend` and `extract_csrf` hunks), reviewed against
`harness/tasks/T-104.md`, the frozen probe contract (`harness/tasks/T-103.md:46-59`,
`acceptance/drafts/probe.spec.ts.txt`, `harness/handoffs/T-100.md:73-74`),
`internal/config/config.go`, `internal/web/web.go`, `Makefile`,
`harness/tasks/graph.json`, `harness/state.md`, and the current `Dockerfile`,
`fly.toml`, `.dockerignore`. Shell execution is permission-denied in this session
as well (git reads pass), and the task worktree `../wt/T-104` is outside this
session's readable scope — every claim below was verified by direct read of the
diff and the main-worktree reference files. smoke.sh line numbers refer to the
new file per the hunk header (`@@ -0,0 +1,206 @@`).

## The previous BLOCK is substantively addressed

All five owned paths are now in the diff with the right shapes: `Dockerfile`
(su-exec, `COPY scripts/docker-entrypoint.sh`, `RUN chmod 0755`, `USER root`,
ENTRYPOINT/CMD split; build stage untouched — hunk context only), `fly.toml`
(`[env]` block, fly.toml:8-20), `.dockerignore` (the packet's four changes),
`scripts/docker-entrypoint.sh` (new 100755), `scripts/smoke.sh` (new 100755 —
exec bits are in the diff metadata, resolving the first attempt's mode concern).
Ownership is respected: the diff touches exactly the five owned files plus the
handoff `harness/handoffs/T-104.md`; nothing protected. The handoff is complete
(commands + exit codes, machine-unverified list with exact human/CI commands,
open issues, disclosed write workaround that the prior review sanctioned).

## Verified-true claims (spot checks)

- **fly.toml env block** — the seven values are byte-exact against
  `T-104.md:78-83`; names and semantics check out against
  `internal/config/config.go`: `PORT/BASE_URL/DB_PATH/PHOTO_DIR` required
  (config.go:76-79), `APP_ENV="prod"` valid (config.go:81-89), `TZ` optional
  with matching default (config.go:108-111), `TOWN_BBOX="42.70,-71.09,42.82,-70.96"`
  parses with min<max on both axes (config.go:44-62). Secrets correctly absent —
  only their NAMES appear in a comment. Nothing else in fly.toml changed.
- **.dockerignore** — exactly `test-run.db`, `test-run-photos/`, `selftest*`,
  and `harness/runs` → `harness`; the 13 prior entries are preserved;
  `scripts/` stays in the build context so the runtime-stage COPY works.
- **docker-entrypoint.sh** — POSIX sh, `set -eu`, guarded best-effort
  `chown -R 10001:10001 /data || true`, `exec su-exec appuser "$@"`
  (docker-entrypoint.sh:7-15): the packet-verbatim pattern; the server remains
  the exec'd process for signal handling.
- **healthz / CSP middleware** — 200 `ok` at `internal/web/web.go:49-52`;
  `withSecurityHeaders` wraps the whole chain and sets CSP unconditionally
  (web.go:65, 85-99), so the smoke script's expectations are grounded.
- **Verification routing** — `make smoke BASE_URL=` exists (Makefile:194-197,
  with the `--pass-with-no-tests` weakness honestly disclosed as handoff open
  issue 5); `make ci` includes `docker build` (Makefile:139-143);
  `make e2e-task TASK=T-104` → "no claims", exit 0 by construction
  (graph.json:165 `claims: []`, Makefile:113-116).
- **T-102/T-103 genuinely unmerged** — `internal/web` holds only web.go,
  templates.go, web_test.go, context.go (no csrf*.go, no probe*.go), so
  writing smoke.sh against the frozen contract was the only option
  (`T-104.md:55-58`).
- **Prior advisories folded in** — the `%Z` date parse is gone (assertion now
  uses `openssl x509 -checkend 1209600`, exit-code only, no date parsing);
  every curl carries `|| die` or `-f`; the N₁=N₀+1 race is documented at the
  assertion (smoke.sh:173-176); argument hygiene errors on a second positional
  and unknown flags; `--help` exits 0.

## BLOCKER 1 — `extract_csrf` doubles the token; step 4 can never pass

`scripts/smoke.sh:149` (diff @ `harness/reviews/T-104.diff:403`):

```sh
printf "%s" "$tag" | sed "s/^.*value=\"\([^\"]*\)\".*$/\1/p"
```

The `sed` is missing `-n`. Without it, sed auto-prints the pattern space at the
end of the cycle AND the `p` flag prints it again after a successful
substitution — universal POSIX sed behavior (GNU and BSD alike). On the match
path the output is therefore the token twice, e.g. `T\nT`; command substitution
strips only trailing newlines, so `csrf` holds `T\nT` (smoke.sh:156). The
`[ -n "$csrf" ]` guard passes; `--data-urlencode "csrf_token=$csrf"`
(smoke.sh:161-163) URL-encodes the embedded newline, the server receives
`T%0AT`, `VerifyCSRF` fails, and the run dies at the status gate:

- `scripts/smoke.sh:159-166` → `die "POST /probe: expected 200 (HTMX) or 303
  (plain), got 403"` — against a **correctly deployed** T-103.

The non-match path is no better (the whole `<input …>` tag is sent → 403).
Step 4 is the M1 judge input (`harness/state.md:65`: "judge with both deploys'
`scripts/smoke.sh` outputs"; the packet's title deliverable), and it can never
succeed as written. Contrast `extract_count`'s sed (smoke.sh:137), which
correctly omits the `p` flag — the asymmetry is the missing `-n`.
Fix: `sed -n "s/^.*value=\"\([^\"]*\)\".*$/\1/p"` (then the token arrives once;
optionally also assert the extracted value contains no whitespace).

This is exactly the machine-unverifiable path the packet told reviewers to
scrutinize (`T-104.md:27-36`). The handoff's hardening claim ("extraction
helpers return non-zero on no-match") is true but incomplete — the MATCH path
is broken, and "read-back byte-exact against the intended content" cannot see
a semantic bug; this is why it survived two careful passes.

## BLOCKER 2 — case-sensitive CSP grep false-fails over HTTP/2

`scripts/smoke.sh:113-114`:

```sh
grep -q "^Content-Security-Policy:" "$HDR"
```

The handoff justifies case-sensitivity because "Go canonicalizes header names
on send" — true only for the origin's HTTP/1.1 response. The smoke client
talks to the Fly edge, not to the Go server: with any HTTP/2-capable curl
(default Linux distro builds, Homebrew curl) ALPN negotiates h2, h2 field
names are lowercase on the wire (RFC 9113 §8.1), and `curl -D` dumps them
lowercase — so the grep misses `content-security-policy:` and a healthy site
fails with "Content-Security-Policy header missing". macOS *system* curl
(LibreSSL, no HTTP/2) happens to work, which makes the bug environment-
dependent and thus worse: works on the author's machine, red on the CI/Linux
box. The previous review's "grep -qi … needlessly case-insensitive (harmless)"
had it backwards — case-insensitivity is load-bearing behind an h2-terminating
proxy. Fix: `grep -qi "^content-security-policy:"` or add `--http1.1` to the
curl invocation.

Both blockers fail in the safe direction (false RED, no assertion weakened),
but the deliverable's sole purpose — collecting deployed persistence evidence
after H4 — fails at first use in the common case, and per blocker 1 can never
pass at all.

## Minor

- `smoke.sh:87-89` — a BASE_URL with a query string (`https://host?x`) leaks
  `?x` into `hostport`/`host`; openssl then fails with a die naming the
  mangled host. Acceptable error path; trimming at `?` would be tidier.
- Accepting `303` on the `HX-Request: true` POST follows the packet verbatim
  (`T-104.md:103-104`); the observed status is printed in the OK line
  (smoke.sh:178), so a regression stays visible. Noted, not blocking —
  reviewer-alt's earlier objection is against the packet, not this diff.
- No unit tests: packet-consistent (`claims: []`; `T-104.md:23-25` defines
  verification as CI `docker build` + the deployed smoke run; no handler, no
  UI, so AGENTS rules 6/7 don't apply). The consequence — zero machine
  verification of smoke.sh's logic before deploy — is precisely what blockers
  1 and 2 exploit.

## Security

- No secrets anywhere in the diff: `fly.toml [env]` holds only non-secret
  runtime config; secret names appear in a comment only; the handoff's
  docker-run example uses `<32+ bytes>` placeholders. Nothing gitleaks-shaped.
- The entrypoint's root window is the packet-prescribed minimum: `sh` +
  `[ -d /data ]` + `chown -R` on a literal path (no untrusted input) +
  `exec su-exec` before the server starts. Standard Fly volume pattern.
- smoke.sh has no injection surface found: BASE_URL is quoted at every use,
  counts are validated numeric before `$(( ))`, scratch files live in a
  `mktemp -d` dir removed by an EXIT trap, tokens are never echoed.

## Architecture

No Go/TS/templates touched, so the ownership map, HTMX/#map rule,
parse-don't-validate and typed-ID rules are unimplicated. The diff contains no
"Phase N" strings, no TODO/FIXME/console.log, and no dependency beyond the
packet-authorized `su-exec` apk package.

## Position on the verdict

Attempt 2 delivers everything the first BLOCK demanded, and four of the five
files are packet-exact on read. But `scripts/smoke.sh` — the only executable
verification this task produces and the M1 close-out evidence source — carries
two defects that make the deployed smoke run fail: one guaranteed
(doubled CSRF token → valid POST always 403), one environment-dependent
(case-sensitive CSP grep → false miss over h2). The packet's own bar for this
review ("write the files defensively… reviewers will scrutinize this diff
precisely because it cannot be executed here", `T-104.md:30-36`) makes
correctness of exactly these lines the review's responsibility, and they are
wrong. Both fixes are one-liners; land them (plus, ideally, a
`bash -n`/shellcheck pass and one local execution of the two extraction
functions against a synthetic fragment once any shell is available) and this
should pass on re-review without further redesign.

VERDICT: BLOCK
