#!/usr/bin/env bash
# smoke.sh — deployed evidence collector (T-104).
#
# usage: scripts/smoke.sh [--smoke-suite] <BASE_URL>
#
# Every step prints `smoke: <check> OK` on success; the first failure prints
# `smoke: FAIL: <reason>` and exits non-zero. Step 4 (probe persistence) is
# the M1 evidence: run this before and after a deploy and the printed
# counters must continue across the deploy boundary.
#
#   --smoke-suite  also run `make smoke BASE_URL=<url>` (the @smoke Playwright
#                  specs; skipped with a note when node_modules is absent).
#
# Written for macOS bash 3.2 and GNU/Linux: no associative arrays, no
# ${var,,}, no mapfile. Requires curl, openssl, sed, grep, tr, head, make.
set -euo pipefail

usage() {
    printf "usage: scripts/smoke.sh [--smoke-suite] <BASE_URL>\n" >&2
}

die() {
    printf "smoke: FAIL: %s\n" "$*" >&2
    exit 1
}

ok() {
    printf "smoke: %s OK\n" "$1"
}

BASE_URL=""
RUN_SUITE=0
for arg in "$@"; do
    case "$arg" in
        --smoke-suite) RUN_SUITE=1 ;;
        -h|--help) usage; exit 0 ;;
        --*)
            printf "smoke: FAIL: unknown flag: %s\n" "$arg" >&2
            usage
            exit 2
            ;;
        *)
            if [ -n "$BASE_URL" ]; then
                printf "smoke: FAIL: unexpected extra argument: %s\n" "$arg" >&2
                usage
                exit 2
            fi
            BASE_URL="$arg"
            ;;
    esac
done
[ -n "$BASE_URL" ] || { usage; exit 2; }
BASE_URL="${BASE_URL%/}"

case "$BASE_URL" in
    https://*|http://*) ;;
    *) die "BASE_URL must be an http(s) URL, got: $BASE_URL" ;;
esac

WORKDIR=$(mktemp -d "${TMPDIR:-/tmp}/gl-smoke.XXXXXXXX") \
    || die "cannot create temp dir"
trap "rm -rf \"$WORKDIR\"" EXIT
BODY="$WORKDIR/body.out"
HDR="$WORKDIR/headers.txt"
JAR="$WORKDIR/cookies.txt"
CERT="$WORKDIR/cert.pem"

GIT_SHA=$(git rev-parse --short HEAD 2>/dev/null || printf "unknown")
printf "smoke: run %s UTC, git %s, target %s\n" \
    "$(date -u "+%Y-%m-%dT%H:%M:%SZ")" "$GIT_SHA" "$BASE_URL"

# 1. GET /healthz -> 200 with body `ok` (fly checks the same path).
status=$(curl -sS --max-time 5 -o "$BODY" -w "%{http_code}" "$BASE_URL/healthz") \
    || die "GET /healthz: curl transport failure"
[ "$status" = "200" ] || die "GET /healthz: expected 200, got $status"
body=$(tr -d "[:space:]" < "$BODY")
[ "$body" = "ok" ] || die "GET /healthz: expected body ok, got [$body]"
ok "healthz -> 200 ok"

# 2. TLS: the certificate must expire >= 14 days out. `openssl x509
# -checkend` avoids per-platform date parsing entirely (BSD strptime chokes
# on %Z); 1209600 seconds = 14 days.
case "$BASE_URL" in
    https://*)
        hostport=${BASE_URL#https://}
        hostport=${hostport%%/*}
        host=${hostport%%:*}
        port=443
        case "$hostport" in
            *:*) port=${hostport##*:} ;;
        esac
        openssl s_client -servername "$host" -connect "$host:$port" </dev/null 2>/dev/null \
            | openssl x509 > "$CERT" \
            || die "tls: could not retrieve certificate for $host:$port"
        enddate=$(openssl x509 -in "$CERT" -enddate -noout) \
            || die "tls: could not read certificate dates"
        notafter=${enddate#notAfter=}
        [ -n "$notafter" ] || die "tls: empty notAfter"
        openssl x509 -in "$CERT" -checkend 1209600 -noout \
            || die "tls: certificate expires within 14 days ($notafter)"
        ok "tls cert expires >= 14 days out ($notafter)"
        ;;
    *)
        printf "smoke: tls SKIPPED (BASE_URL is plain http)\n"
        ;;
esac

# 3. GET / -> 200 and CSP header present. The server sets the header on every
# response and Go canonicalizes names, so a case-sensitive match is correct.
status=$(curl -sS --max-time 10 -o "$BODY" -D "$HDR" -w "%{http_code}" "$BASE_URL/") \
    || die "GET /: curl transport failure"
[ "$status" = "200" ] || die "GET /: expected 200, got $status"
grep -q "^Content-Security-Policy:" "$HDR" \
    || die "GET /: Content-Security-Policy header missing"
ok "index -> 200 + Content-Security-Policy"

# 4. Probe persistence evidence (the M1 judge input). Frozen contract
# (harness/tasks/T-103.md): GET /probe returns an HTML fragment carrying
# <p>Count: <span id="probe-count">N</span></p> and a form with a hidden
# csrf_token input; POST /probe with a valid token increments the counter.
# With `HX-Request: true` the contract response is 200 (fragment swap); the
# no-JS plain-form fallback answers 303 -> /. The task packet says accept
# either, so both are tolerated here; the count assertions below are the
# real proof.
probe_get() {
    # Body of GET /probe with the shared cookie jar; -f turns HTTP errors
    # into a non-zero exit so `|| die` at the call site always reports.
    curl -fsS --max-time 10 -b "$JAR" -c "$JAR" "$BASE_URL/probe"
}

extract_count() {
    # N from the frozen probe-count span; tr flattens lines so the pattern
    # survives wrapped markup.
    flat=$(printf "%s" "$1" | tr "\n" " ")
    printf "%s" "$flat" \
        | grep -o "id=\"probe-count\">[0-9][0-9]*" \
        | sed "s/^id=\"probe-count\">//" \
        | head -n 1
}

extract_csrf() {
    # Hidden input named csrf_token; attribute order is not pinned by the
    # contract so [^>]* bridges either order. Tokens are unpadded base64url
    # (T-102), i.e. they never contain a double quote.
    flat=$(printf "%s" "$1" | tr "\n" " ")
    tag=$(printf "%s\n" "$flat" \
        | grep -o "<input[^>]*name=\"csrf_token\"[^>]*>" \
        | head -n 1) || return 1
    printf "%s" "$tag" | sed "s/^.*value=\"\([^\"]*\)\".*$/\1/p"
}

frag=$(probe_get) || die "GET /probe: transport or HTTP failure"
n0=$(extract_count "$frag") || die "GET /probe: could not parse probe count"
csrf=$(extract_csrf "$frag") || die "GET /probe: could not parse csrf_token"
case "$n0" in ""|*[!0-9]*) die "GET /probe: count is not a number: [$n0]" ;; esac
[ -n "$csrf" ] || die "GET /probe: empty csrf_token"
n0=$((10#$n0))

pstatus=$(curl -sS --max-time 10 -o /dev/null -w "%{http_code}" \
    -b "$JAR" -c "$JAR" -H "HX-Request: true" \
    --data-urlencode "csrf_token=$csrf" "$BASE_URL/probe") \
    || die "POST /probe: curl transport failure"
case "$pstatus" in
    200|303) ;;
    *) die "POST /probe: expected 200 (HTMX) or 303 (plain), got $pstatus" ;;
esac

frag1=$(probe_get) || die "re-GET /probe: transport or HTTP failure"
n1=$(extract_count "$frag1") || die "re-GET /probe: could not parse probe count"
case "$n1" in ""|*[!0-9]*) die "re-GET /probe: count is not a number: [$n1]" ;; esac
n1=$((10#$n1))
printf "smoke: probe count %s -> %s\n" "$n0" "$n1"
# Single-admin deployment assumption: a concurrent /probe hit inside this
# window would legitimately break the +1 assertion — re-run the script
# before treating a mismatch as a persistence failure.
[ "$n1" -eq "$((n0 + 1))" ] \
    || die "probe count did not increment by exactly 1 ($n0 -> $n1)"
ok "probe persistence (POST accepted as $pstatus, count incremented)"

# Negative control: a tampered token must be rejected with 403 and must NOT
# change the counter.
bstatus=$(curl -sS --max-time 10 -o /dev/null -w "%{http_code}" \
    -b "$JAR" -c "$JAR" -H "HX-Request: true" \
    --data "csrf_token=bogus-token-negative-test" "$BASE_URL/probe") \
    || die "POST /probe (bogus token): curl transport failure"
[ "$bstatus" = "403" ] \
    || die "POST /probe with bogus token: expected 403, got $bstatus"
frag2=$(probe_get) || die "re-GET /probe after bogus POST: transport or HTTP failure"
n2=$(extract_count "$frag2") || die "re-GET /probe: could not parse probe count"
case "$n2" in ""|*[!0-9]*) die "re-GET /probe: count is not a number: [$n2]" ;; esac
[ "$n2" -eq "$n1" ] || die "bogus-token POST changed the counter ($n1 -> $n2)"
ok "bogus csrf rejected 403, count unchanged ($n2)"

# 5. Optional @smoke Playwright suite via the repo `make smoke` target.
if [ "$RUN_SUITE" -eq 1 ]; then
    ROOT=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
    if [ ! -d "$ROOT/node_modules" ]; then
        printf "smoke: suite SKIPPED (node_modules absent; run npm ci first)\n"
    else
        (cd "$ROOT" && make smoke BASE_URL="$BASE_URL") \
            || die "make smoke (@smoke suite) failed"
        ok "suite (@smoke specs via make smoke)"
    fi
fi

printf "smoke: all checks passed\n"
