# T-104 — reviewer, attempt 3 (correctness / security / architecture)

Scope: the attached diff, reviewed against `harness/tasks/T-104.md`, the frozen
probe contract (`harness/tasks/T-103.md:39-59`,
`acceptance/drafts/probe.spec.ts.txt`), `internal/config/config.go`,
`internal/web/web.go`, `Makefile`, `harness/state.md`, `harness/policy.json`,
and the main-worktree pre-change state of `Dockerfile`, `fly.toml`,
`.dockerignore`. smoke.sh line numbers refer to the new file per the hunk
header (`@@ -0,0 +1,224 @@`). Shell execution is permission-denied in this
review session as in all prior ones (`bash`, `make`, `sed` invocation all
rejected; git reads pass), so the two blocker fixes were verified by
specification-level analysis of the exact commands — the same scrutiny the
packet demands (`T-104.md:27-36`) and the same method that caught attempt 2's
defects.

## Diff authenticity

The diff corresponds exactly to the committed chain: `3218a68` (all five
deliverables), `cae2d49` (smoke.sh fixes only — `git diff 3218a68 cae2d49`
shows +22/−4 in `scripts/smoke.sh` and nothing else in code), `05629d0`
(final 67-line handoff). The smoke.sh blob in the attached diff
(`index 0000000..42ddbe9`) matches the blob at `cae2d49`, and `05629d0`'s stat
touches only the handoff. The Dockerfile/fly.toml/.dockerignore context lines
match the main worktree byte-for-byte. Both scripts carry
`new file mode 100755` in the diff metadata (the attempt-1 mode concern stays
resolved).

## BLOCKER 1 (attempt 2) — fixed and correct

`scripts/smoke.sh:162`:

```sh
printf "%s" "$tag" | sed -n "s/^.*value=\"\([^\"]*\)\".*$/\1/p"
```

With `-n`, sed's end-of-cycle auto-print is suppressed (POSIX: "Do not write
to the standard output … unless … the p flag of the s command is used"), and
the `p` flag writes the pattern space exactly once on successful substitution.
GNU and BSD sed implement this identically; it is the canonical extraction
idiom. The shell passes the intended BRE (`\"` → literal quote for the
pattern, `\(`/`\1` preserved inside double quotes). The token now arrives
once; the no-match path prints nothing and dies at the `[ -n "$csrf" ]` gate
(smoke.sh:171). The asymmetry with `extract_count` (smoke.sh:135-141, bare
`s///` with no `p` → auto-print once) is now self-consistent: each helper
prints exactly once. Defense in depth added at smoke.sh:172-176: a
`*[[:space:]]*` case gate (POSIX bracket class, bash 3.2-valid) refuses any
whitespace-laden extraction with a specific `smoke: FAIL:` before the POST —
a future regression of this class is diagnosed in one read instead of
surfacing as an inexplicable 403. Fails red, never false-green.

## BLOCKER 2 (attempt 2) — fixed and correct

`scripts/smoke.sh:121`: `grep -qi "^content-security-policy:" "$HDR"`.
Case-insensitive match covers both the canonical `Content-Security-Policy:`
(Go/HTTP-1.1 form) and the lowercase h2 wire form curl `-D` dumps behind the
Fly edge (RFC 9113 §8.1), with the reasoning preserved in the comment
(smoke.sh:112-117). Absent header → grep exit 1 → `|| die` fires
(smoke.sh:122). Grounded: `withSecurityHeaders` sets CSP unconditionally on
every response (`internal/web/web.go:65, 85-99`).

## Advisory fixes folded in

- BASE_URL query/fragment trim (smoke.sh:54-57): `${BASE_URL%%\?*}` (escaped
  `?` is a literal in the glob pattern) then `${BASE_URL%%#*}` (`#` is not a
  pattern metacharacter); `%%` strips from the first occurrence, so both
  `?…#…` and `#…?…` orderings reduce correctly. Bash 3.2-safe.
- TLS expiry via `openssl x509 -checkend 1209600` (exit-code only, no date
  parsing — the macOS `%Z` hazard from attempt 1 stays gone), with
  `-enddate` retained for the printed evidence (smoke.sh:83-105).
- Every curl carries `|| die` or `-f` (`probe_get`, smoke.sh:124-128); counts
  validated numeric before `$(( ))` with base-10 forcing (`10#`, smoke.sh:177);
  the N₁=N₀+1 race is documented at the assertion (smoke.sh:190-193).

## Deliverable conformance (all five, packet-exact)

- **Dockerfile** — `su-exec` added to the apk line (:11-12), `COPY
  scripts/docker-entrypoint.sh` + `RUN chmod 0755` (:14-15), `USER root`
  (:18), `ENTRYPOINT ["/docker-entrypoint.sh"]` + `CMD ["/app/server"]`
  (:21-22); build stage byte-untouched (context lines only); alpine stays
  pinned 3.22. Exactly `T-104.md:62-74`.
- **scripts/docker-entrypoint.sh:1-15** — POSIX sh, `set -eu`,
  `[ -d /data ]`-guarded best-effort `chown -R 10001:10001 /data || true`,
  `exec su-exec appuser "$@"` (server remains PID 1 for signals). Verbatim
  packet pattern; root window is the minimum the pattern requires, on a
  literal path with no untrusted input.
- **fly.toml:8-19** — `[env]` with exactly the seven values from
  `T-104.md:78-83`; names verified against `internal/config/config.go`
  (PORT/BASE_URL/DB_PATH/PHOTO_DIR required :76-79, APP_ENV :81-89, TOWN_BBOX
  :103 with min<max holding on both axes for `42.70,-71.09,42.82,-70.96`
  via ParseBBox :44-62, TZ :108-111). Secrets appear only as names in a
  comment; snapshot retention correctly absent (volume-creation flag, H4).
  Nothing else in the file changed.
- **.dockerignore:9-11,16** — adds `test-run.db`, `test-run-photos/`,
  `selftest*`; `harness/runs` → `harness`; all 13 prior entries preserved;
  `scripts/` stays in the context so the runtime-stage COPY works.
- **scripts/smoke.sh** — all five packet steps present with the specified
  output contract (`smoke: <check> OK`, first failure `smoke: FAIL:` +
  non-zero), `set -euo pipefail`, `--smoke-suite` → `make smoke BASE_URL=…`
  with node_modules-absent skip note (smoke.sh:209-219), run header with
  `date -u` + git sha (smoke.sh:72-74) so two runs bracket a deploy. No
  associative arrays, no `${var,,}`, no mapfile — bash 3.2-safe throughout.

## Contract grounding

Probe fragment parsing matches the frozen T-103 contract exactly:
`<span id="probe-count">N</span>` (`harness/tasks/T-103.md:49`, draft spec
:32-33) → `extract_count`; hidden `csrf_token` input (:50-54) →
`extract_csrf` (order-independent via `[^>]*` bridging); tampered token → 403
with unchanged count (:57) → negative control (smoke.sh:196-208); valid POST
200 HTMX / 303 plain (:56-59) → accepted 200|303 per the packet's own "accept
either" (`T-104.md:103-104`), with the observed status printed in the OK line
(smoke.sh:195) so a regression stays visible. `/healthz` → 200 `ok`
(`internal/web/web.go:49-52`). `make smoke BASE_URL=` exists
(`Makefile:194-197`), `make e2e-task TASK=T-104` → "no claims" exit 0 by
construction (`Makefile:113-116`, `claims: []` in the packet), `make ci`
includes `docker build` (`Makefile:139-143`), and `harness/state.md:65` names
both deploys' `scripts/smoke.sh` outputs as the M1 judge evidence — the
deliverable's purpose is intact. T-103 is indeed unmerged (`internal/web/`
holds only web.go, context.go, templates.go, web_test.go), so building to the
frozen contract was the only option (`T-104.md:55-58`).

## Security

No secrets anywhere: fly.toml `[env]` holds public runtime config only;
secret names appear in comments; the handoff's docker-run example uses
`<32+ bytes>` placeholders. smoke.sh has no injection surface: BASE_URL is
scheme-gated (smoke.sh:59-62) and quoted at every use; counts are validated
numeric before arithmetic; scratch files live in a `mktemp -d` dir removed by
an EXIT trap (smoke.sh:64-66); the token is never echoed except in the
failure diagnostic of the whitespace gate (local operator stderr; not a
secret). No template.HTML, SQL, uploads, or path traversal implicated — the
diff touches no Go/templates.

## Architecture

Diff touches exactly the five owned paths plus the auto-allowed handoff;
nothing in `harness/policy.json:37-51`'s protected list. No Go/TS/template
surface, so the ownership map, HTMX/#map rule, parse-don't-validate and
typed-ID rules are unimplicated. No "Phase N"/TODO/FIXME/console.log/panic
strings (grepped). No new Go/npm dependency — `su-exec` is an apk package the
packet itself prescribes (`T-104.md:69-70`).

## Tests

`claims: []`, `ui: false`; the packet defines verification as CI
`docker build` (`make ci`) plus the deployed smoke run (`T-104.md:23-25`).
No handler or UI change, so AGENTS rules 6/7 do not apply. The handoff
discloses every machine-unverified path (`bash -n`, shellcheck, docker,
flyctl, the live smoke run) with the exact commands for human/CI, plus
commands run with exit codes — the disclosure the packet demanded. The
`--pass-with-no-tests` weakness of `make smoke` and the +1 race are
consciously documented as open issues rather than hidden.

## Minor (non-blocking)

- `grep -o | head -n 1` under pipefail could in principle die via SIGPIPE
  (grep exit 141) if a fragment contained many matches — fails red, never
  false-green; practically impossible with the contract fragment.
- Handoff's "lines 170–174" for the whitespace gate is off by two (actual
  smoke.sh:172-176); the other line references (121, 162, 54-57) are exact.
  Prose imprecision only.
- The TLS host split would mishandle an IPv6-literal BASE_URL — irrelevant
  for the deployment target.
- Accepting 303 on an `HX-Request: true` POST is the packet's own leniency;
  the status is printed, so a T-103 HTMX-detection regression remains visible
  in the evidence output.

## Position on the verdict

Attempt 2 delivered all five files but left the sole executable deliverable
unable to pass: the CSRF extraction doubled the token (valid POST always 403)
and the CSP grep false-failed over h2. Attempt 3 fixes both exactly as
prescribed — `sed -n … p` prints the captured token exactly once
(smoke.sh:162) and `grep -qi` matches both header casings (smoke.sh:121) —
folds in the advisory BASE_URL trim, adds a whitespace gate that converts any
future extraction-mangling regression into a one-read diagnosis, and touches
nothing else: the four other files are byte-identical to the reviewer-verified
`3218a68` state. The fixes are the two one-liners the prior review specified,
independently re-derived here against POSIX sed/grep semantics and found
correct; the surrounding logic (arg hygiene, numeric validation, TLS checkend,
cookie-jar probe flow, negative control, cleanup trap) was re-traced and holds.
The handoff is complete, honest about the sandbox, and lists every
machine-unverified path with runnable commands. Nothing blocks merge.

VERDICT: PASS
