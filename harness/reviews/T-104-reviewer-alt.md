# T-104 — reviewer-alt (test integrity / reward hacking) — retry

Scope: attached `harness/reviews/T-104.diff` (the retry: `Dockerfile`,
`fly.toml`, `.dockerignore`, `scripts/docker-entrypoint.sh`,
`scripts/smoke.sh`, and the handoff), read against `harness/tasks/T-104.md`,
T-103's frozen probe contract (`harness/tasks/T-103.md:39-62`), the frozen
acceptance drafts (`acceptance/drafts/probe_go_test.go.txt`,
`acceptance/drafts/probe.spec.ts.txt`), `internal/config/config.go`,
`internal/web/web.go`, `Makefile`, and `harness/tasks/graph.json`.
`refs/heads/task/T-104` is `e9a597e`; the claimed build commit `3218a68` is in
that worktree's reflog. The main checkout is the pre-merge base, and shell /
docker / curl remain permission-denied in this review session, so the new
shell logic was read and traced rather than executed. This supersedes the
first-attempt alt review (which correctly blocked a handoff-only diff).

## What the retry actually delivers

All five owned paths are present in the diff, and no protected path is
touched:

- `Dockerfile:11-22` — `su-exec`, entrypoint COPY + `chmod 0755`,
  `USER root`, `ENTRYPOINT`/`CMD` split; the build stage is unchanged.
- `scripts/docker-entrypoint.sh:1-15` — POSIX sh, best-effort
  `chown -R 10001:10001 /data || true`, then `exec su-exec appuser "$@"`.
- `fly.toml:11-19` — the seven packet-required non-secret `[env]` values,
  with secrets absent and the H5 comment on `BASE_URL`.
- `.dockerignore:9-16` — the three test artifacts plus `harness` (broadened
  from `harness/runs`); `scripts/` remains in the context for the COPY.
- `scripts/smoke.sh:1-206` — healthz, TLS expiry via
  `openssl x509 -checkend 1209600`, index + CSP, probe count/CSRF
  increment, tampered-token 403 with unchanged count, optional
  `--smoke-suite`.

Verified by reading: the env names match `config.Load` (`PORT`, `BASE_URL`,
`DB_PATH`, `PHOTO_DIR` required at `internal/config/config.go:76-80`;
`APP_ENV` at `:81-89`; `TOWN_BBOX` at `:103`; `TZ` at `:108-111`), the
`TOWN_BBOX` value satisfies `ParseBBox`'s min<max check (`:44-62`), CSP is set
unconditionally by `withSecurityHeaders` (`internal/web/web.go:85-99`), and
healthz is 200 `ok` (`web.go:49-52`). No new Go/npm dependency (the `su-exec`
apk package is packet-prescribed), no secret, no test file touched, no
`TODO`/`FIXME`/`console.log`/`Phase N`, no `t.Skip`/`test.skip`/`test.only`/
`.fixme`.

## Findings

### 1. [Medium, not blocking] HTMX POST accepts 303 as well as 200

`scripts/smoke.sh:159-167` sends `HX-Request: true` for the valid POST but
accepts `200|303` (the `200|303)` arm is `scripts/smoke.sh:165`). The frozen
T-103 contract distinguishes them: an HTMX POST must be 200
(`harness/tasks/T-103.md:55-59`; asserted exactly at
`acceptance/drafts/probe_go_test.go.txt:87-90`), and 303 belongs only to the
no-`HX-Request` fallback (`:138-143`). The task packet explicitly says
"accept either" (`harness/tasks/T-104.md:103-104`), so the worker is
packet-conformant, and A-103 (activated by T-105) still enforces the exact
statuses. But the deployed evidence collector is the only check that runs
after a real deploy, and as written it cannot distinguish the HTMX path from
the no-JS path. A regression in HTMX detection would pass this script. The
`ok` line does print the observed status, which is a useful mitigation.
Recommendation for the follow-up: assert 200 for the `HX-Request` POST and
add a separate no-`HX-Request` POST asserting 303 + `Location: /`.

### 2. [Medium, not blocking] `--smoke-suite` can print `OK` with zero tests

`scripts/smoke.sh:194-204` runs `make smoke`, whose target uses
`--pass-with-no-tests` (`Makefile:194-197`), then unconditionally prints
`smoke: suite (@smoke specs via make smoke) OK` (`scripts/smoke.sh:202`).
Until T-105 activates the inert `probe.spec.ts.txt`, that command matches zero
@smoke specs and exits 0; this is a test that cannot fail. The handoff
discloses exactly this (`harness/handoffs/T-104.md:152-155`) and says the
curl-level assertions are the real check, so it is not hidden — but the
script's own `OK` line is still coverage theatre if a human or the judge reads
the output without the handoff. Recommend asserting a non-zero matched-spec
count (e.g. `npx playwright test --list --grep @smoke` or parsing the run
output) or dropping the `OK` wording until @smoke specs exist. The optional
flag does not affect the core persistence evidence.

### 3. [Medium, fix before H4] The handoff's local entrypoint command cannot boot

`harness/handoffs/T-104.md:98-106` tells the human to run the image with
`PORT`, `APP_ENV`, `BASE_URL`, `TOWN_BBOX` and the three secrets, then
`docker exec` in and confirm `/data/lights.db` exists. It omits
`DB_PATH` and `PHOTO_DIR`, both of which `config.Load` requires
(`internal/config/config.go:76-80,112-114`), and the image only sets
`ENV PORT=8080` (`Dockerfile:20`). The server will therefore exit with
"missing required environment variables: DB_PATH, PHOTO_DIR" before creating
the DB. The command also uses `docker run --rm` in the foreground without
`--name`/`-d`, so the suggested `docker exec` has no container to target.
Add `-e DB_PATH=/data/lights.db -e PHOTO_DIR=/data/photos` and a
background/name for the exec check (or use `docker run --rm` simply to observe
the boot and the chown of `/data`). This is a verification-recipe defect, not
a code defect: the Fly path sets both variables (`fly.toml:14-15`) and the
entrypoint logic itself is sound.

### 4. [Low] The "persistence" assertion is in-process only

`scripts/smoke.sh:172-179` asserts `N1 = N0 + 1` within a single run. Cross-
deploy persistence is left to a human comparing the printed count from two
runs, and the header's `git` field (`scripts/smoke.sh:69-71`) is the local
checkout's sha, not a value read from the deployed artifact. That matches the
packet's two-run design (`harness/state.md:62-66`) and is not a defect, but
the evidence is manual; a future version could store the previous count or
print a machine-comparable line. The negative control at
`scripts/smoke.sh:182-192` is a real assertion (403 + unchanged count), so
the script is not a no-op.

### 5. [Low] No static execution or unit coverage for the parsing logic

`scripts/smoke.sh:131-150` contains the count and CSRF extractors, and
`:80-106` the certificate path; none of it was run here (`bash -n`,
`shellcheck`, `docker`, `curl` were denied). The handoff lists this honestly
(`harness/handoffs/T-104.md:86-95`) and the packet owns no test path and caps
worker verification at `make check-fast` + reading
(`harness/tasks/T-104.md:23-36`). The handoff's "How verified" table
(`harness/handoffs/T-104.md:73-74`) still lists `make check-fast` and
`make e2e-task TASK=T-104` as green results; both are vacuous for this diff
(`claims: []`; no Go changed), and the packet itself calls check-fast
"green trivially" (`harness/tasks/T-104.md:30-31`). The handoff does disclose
"Unit tests: none added" (`harness/handoffs/T-104.md:81-84`), so this is a
transparency/labelling issue rather than a false coverage claim; the table
should mark those two as baseline, not verification of the new files. The
syntax and the contract regexes
(`<input[^>]*name="csrf_token"[^>]*>`, `id="probe-count">digits`) trace
correctly against T-103 and the acceptance draft
(`acceptance/drafts/probe_go_test.go.txt:51-54`), and the `-f`/`|| die` paths
mean transport failures are reported rather than silently swallowed. Still,
the first execution will be the deployed run; CI should at least run
`bash -n`/`shellcheck` on this script.

### 6. [Process, not a diff defect] "Reviewer-sanctioned" hand-apply claim

`harness/handoffs/T-104.md:126-143` discloses that the worker wrote the owned
files through the allowlisted `echo` redirect and restored the committed
exec bits with a one-shot `go run` helper after `write`/`chmod` were denied,
and calls this "the reviewer-sanctioned 'hand-apply the blocks' remedy". The
prior reviewers said a *human* should hand-apply the blocks or that write
permission should be restored; they did not sanction the worker routing around
the permission layer. The result is transparent, limited to owned paths, and
the guard saw a committed diff (unlike the first attempt), so I do not treat
it as a test-integrity failure — but the characterization is inaccurate and
the operator should fix the session permissions rather than normalize the
workaround.

### 7. [Info] Secret handling is correctly out of fly.toml, but `secret()` is lenient

`fly.toml` correctly omits `SESSION_KEY`/`IP_HASH_KEY`/`ADMIN_TOKEN`, and the
handoff warns that `config.Load` does not fail when they are absent
(`harness/handoffs/T-104.md:110-113`; `internal/config/config.go:118-126`
returns `nil, nil` for an unset secret). A deploy that forgets
`fly secrets set` can boot with nil keys. That is pre-existing config
behavior, not T-104's code, but the H4 runbook must treat the three
`fly secrets set` calls as mandatory before first deploy.

## Checked and not found

- No test was weakened, narrowed, skipped, or deleted: the diff touches no
  test file, and the acceptance drafts are untouched.
- No mock bypasses the behavior under test; no fixture string is special-cased
  in production code. `#probe-count`/`csrf_token` in `scripts/smoke.sh` are
  the real contract strings, not fixture shortcuts.
- No dead code, no lint-only stubs, no new dependency beyond the prescribed
  `su-exec`, no secret in the diff, no protected path edit.
- `make check-fast` and `make e2e-task TASK=T-104` cannot exercise the new
  files (`harness/tasks/T-104.md:23-25,30-31`); the handoff discloses that no
  unit tests were added (`harness/handoffs/T-104.md:81-84`), though its
  verification table still lists the two green commands without the
  "baseline" label (finding 5).
- The Dockerfile/fly.toml/.dockerignore/entrypoint blocks match the packet
  (`harness/tasks/T-104.md:62-89`); `USER root` with an immediate `su-exec`
  drop is the packet-prescribed pattern, not an unexplained privilege change.

## Verdict

The retry delivers the real deploy plumbing, the smoke script's core
increment/403 assertions are genuine and can fail, and the known leniencies
(200|303, zero-spec suite OK) are packet-sanctioned or explicitly disclosed
rather than hidden. The handoff's docker-run recipe needs the two missing env
vars before a human relies on it. None of these rises to a test-integrity
block on the code in this diff.

VERDICT: PASS
