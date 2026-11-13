# T-104 — reviewer (correctness / security / architecture)

Scope: the attached diff — a single new file, `harness/handoffs/T-104.md`
(357 lines) — reviewed against `harness/tasks/T-104.md`, the frozen probe
contract (`harness/tasks/T-103.md`, `acceptance/drafts/probe.spec.ts.txt`),
`internal/config/config.go`, `internal/web/web.go`, `Makefile`,
`harness/policy.json`, `harness/state.md`, and the current `Dockerfile`,
`fly.toml`, `.dockerignore`, `scripts/`. Shell execution is almost entirely
permission-denied in this session (git read commands pass; `make check-fast`,
`date`, writes outside the repo, even writes to the system temp dir are
denied), so no command from the handoff could be re-run; every file-level
claim below was verified by direct read.

## The blocker: zero of five deliverables exist

The diff's only hunk is the handoff markdown. All five owned paths are
untouched, verified against the tree:

- `Dockerfile:11-17` — runtime stage unchanged: no `su-exec`, no
  `COPY scripts/docker-entrypoint.sh`, still `USER appuser` (:14) and
  `ENTRYPOINT ["/app/server"]` (:17), no ENTRYPOINT/CMD split.
- `fly.toml:1-27` — no `[env]` block at all.
- `.dockerignore:13` — still `harness/runs`; `test-run.db`, `test-run-photos/`,
  `selftest*` absent.
- `scripts/` — contains only `acceptance-go.sh`, `holdout-run.sh`,
  `vendor.sh`, `worktree-new.sh`, `worktree-rm.sh`; no `smoke.sh`, no
  `docker-entrypoint.sh`.

Why this cannot PASS, concretely:

1. **A Fly deploy of the merged state fails at boot.** `config.Load` requires
   non-empty `PORT/BASE_URL/DB_PATH/PHOTO_DIR`
   (`internal/config/config.go:68-80,112-114`) and `APP_ENV` (:81-89); with no
   `[env]` in fly.toml the server exits listing the missing vars.
2. **Even with env supplied another way, the volume path is broken.** The
   current image runs uid 10001 (`Dockerfile:14`) against Fly's root-owned
   `/data` mount → `store.Open` cannot create `/data/lights.db` → crash loop.
   The entrypoint + `su-exec` chown pattern is the deliverable that fixes
   exactly this (`harness/tasks/T-104.md:62-74`).
3. **M1 close-out loses its evidence path.** The judge consumes both deploys'
   `scripts/smoke.sh` outputs as persistence evidence
   (`harness/state.md:62-66`); no script exists.

The handoff is honest about this ("Nothing on disk outside this handoff",
`harness/handoffs/T-104.md:16-18`), but honesty is not delivery. AGENTS.md
rule 10 ("commit everything your task needs") cannot be satisfied by a
handoff that describes files; the gate has nothing to gate.

## The BLOCKED claim (sandbox)

- Plausible in kind: this reviewer session also hits selective permission
  denials (shell rejected for `make`/`date`; writes rejected outside the
  repo), so a restrictive per-session layer demonstrably exists.
- Not verifiable, and contradicted as a general condition: T-101/T-102
  worker diffs in this same harness contained real source files
  (`harness/reviews/T-102.diff` includes `internal/web/csrf.go` etc.), i.e.
  workers have written their owned paths in prior sessions. If T-104's
  session really was handoff-write-only, that is an environment fault to
  escalate to the operator; a review cannot bless an empty task because of it.
- Worker discipline is correct regardless: `BLOCKED:` first line
  (`harness/handoffs/T-104.md:3`), owned files untouched, no workaround
  attempted against the harness, accurate open-issues list. The verdict below
  is about task delivery, not worker conduct.

## Verified-true claims inside the handoff (the design work is real)

- **fly env names exact**: `PORT/BASE_URL/DB_PATH/PHOTO_DIR`
  (`config.go:76-80`), `APP_ENV` (:81-89), `TOWN_BBOX` (:103),
  `TZ` (:108-111); secrets consumed via `secret()` (:118-127) and correctly
  kept OUT of fly.toml. `TOWN_BBOX="42.70,-71.09,42.82,-70.96"` parses via
  `ParseBBox` with min<max holding on both axes (`config.go:44-62`).
- **healthz**: 200 `ok` at `internal/web/web.go:49-52`. **CSP on every
  response**: `web.go:65` wraps the whole chain in `withSecurityHeaders`
  (`web.go:85-99` sets `Content-Security-Policy` unconditionally), so the
  smoke script's header grep will succeed.
- **`make e2e-task TASK=T-104` → "no claims", exit 0**: `Makefile:111-116`
  with `claims: []` (`harness/tasks/graph.json:165`, `T-104.md:12`).
  Consistent with the Makefile logic; not re-runnable here.
- **T-103 (and T-102/T-101) genuinely not merged**: `internal/web` holds only
  `web.go`, `context.go`, `templates.go`, `web_test.go` (no `csrf*.go`, no
  `probe*.go`); `internal/store` absent. Writing smoke.sh against the frozen
  contract (`T-103.md:39-59`; `acceptance/drafts/probe.spec.ts.txt:30-61`
  pins `#probe-count`, the Count regex, the `csrf_token` form field, and the
  deploy-persistence annotations) is exactly what the packet ordered
  (`T-104.md:55-58`).
- **Verification routing is real**: `make smoke BASE_URL=` exists
  (`Makefile:194-197`); `make ci` includes `docker build`
  (`Makefile:139-143`).

## Advisory findings on the proposed blocks (fix when they actually land)

The "ready to apply verbatim" claim should not be taken at face value:

1. **smoke.sh `epoch_of()` BSD fallback is a likely always-fail on macOS.**
   `date -ju -f '%b %d %H:%M:%S %Y %Z'` relies on BSD strptime parsing `%Z`,
   which is unreliable; the working idiom hardcodes the literal
   (`'%b %d %H:%M:%S %Y GMT'` — openssl `notAfter=` always prints GMT). If
   `%Z` fails, every macOS run dies at `tls: cannot parse notAfter`. The
   GNU-first branch only saves Linux. Unverifiable by execution here (shell
   denied) — precisely the machine-unverified class the packet tells
   reviewers to scrutinize (`T-104.md:27-36`).
2. **`http_get()` aborts silently on curl transport failure.** Steps 1 and 4
   GETs have no `|| die`; under `set -euo pipefail` the script exits with no
   `smoke: FAIL:` line (step 3's inline curl does it correctly —
   inconsistent with the packet's "each step prints evidence" intent,
   `T-104.md:93-95`).
3. **`N₁ = N₀ + 1` is racy** against any concurrent `/probe` hit between the
   two GETs. Acceptable on a single-admin deployment; deserves a comment or
   one retry so a smoke window cannot produce a false red.
4. Minor: the `for arg in "$@"` loop lets a second positional silently
   overwrite `BASE_URL`; `extract_csrf` assumes no embedded `"` in the token
   (true for T-102's frozen unpadded-base64url tokens — worth a pinning
   comment); `grep -qi` on the CSP header is needlessly case-insensitive
   (harmless).
5. The Dockerfile / fly.toml / .dockerignore / entrypoint blocks match the
   packet to the letter (`T-104.md:62-113`): `su-exec` + `RUN chmod 0755` +
   `USER root` + ENTRYPOINT/CMD split; the seven `[env]` values verbatim;
   `.dockerignore` = current file plus `test-run.db`, `test-run-photos/`,
   `selftest*`, `harness/runs` → `harness`. Root-only-for-chown with
   immediate privilege drop is the packet-prescribed pattern, not a smell.

## Security / architecture

- The diff adds a markdown file only: no code surface, no `template.HTML`, no
  SQL, no path traversal, no upload limits implicated, no secrets (only env
  var NAMES and `…` placeholders in the docker-run example; nothing
  gitleaks-relevant).
- Architecture map (`docs/spec/architecture.md`) not implicated (no
  Go/TS/templates); ownership respected — the only touched path is the
  auto-allowed handoff; nothing protected (`harness/policy.json:37-51`)
  anywhere near the diff.

## Tests

- None exist and the packet requires none for this task (`claims: []` —
  verified by CI's `docker build` and the deployed smoke run,
  `T-104.md:23-25`). But that also means there is zero machine verification
  of any kind for T-104 today; the handoff's "How verified" table is
  unverifiable from this session (`make` denied) and only structurally
  corroborated (a markdown-only diff trivially passes check-fast).

## Position on the verdict

The worker followed the blocked protocol correctly, and the embedded design
is accurate against every frozen contract I could check by reading. But
review certifies a diff, and this diff delivers none of the five owned
deliverables. Merging it would mark T-104 done while the Fly deploy remains
broken for want of exactly these files: boot fails at `config.Load` without
the `[env]` block, the appuser cannot write the root-owned volume without the
entrypoint, and M1's judge has no smoke-evidence collector. Whether or not
the sandbox story holds, the remedy is the same: restore write permission
(or hand-apply the blocks), re-run T-104, and re-review a diff containing the
actual files — folding in the advisory fixes above, especially the macOS
`%Z` date parse, which I expect to fail as written.

VERDICT: BLOCK
