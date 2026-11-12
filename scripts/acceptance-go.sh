#!/usr/bin/env bash
# Boot the built binary on a free port with a temp DB and run the Go acceptance
# suite against it. Used by make test-acceptance-go (plan §6.1.8).
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
TMP="$(mktemp -d)"
PORT_FILE="$TMP/port"

# Pick a free port.
while :; do
  CANDIDATE=$(( (RANDOM % 2000) + 18000 ))
  if ! lsof -iTCP:"$CANDIDATE" -sTCP:LISTEN >/dev/null 2>&1; then PORT="$CANDIDATE"; break; fi
done
echo "$PORT" > "$PORT_FILE"

BASE_URL="http://localhost:$PORT" \
PORT="$PORT" APP_ENV=test \
DB_PATH="$TMP/acc.db" PHOTO_DIR="$TMP/photos" \
SESSION_KEY="dGVzdC1zZXNzaW9uLWtleS0zMi1ieXRlcy1hYQ==" \
IP_HASH_KEY="dGVzdC1pcGhhc2gta2V5LTMyLWJ5dGVzLWJi" \
ADMIN_TOKEN="dGVzdC1hZG1pbi10b2tlbi0zMi1ieXRl" \
TOWN_BBOX="42.70,-71.09,42.82,-70.96" TZ="America/New_York" \
"$ROOT/bin/server" >"$TMP/server.log" 2>&1 &
SERVER_PID=$!

cleanup() { kill "$SERVER_PID" 2>/dev/null || true; }
trap cleanup EXIT

# Wait for readiness.
for _ in $(seq 1 50); do
  if curl -sf "http://localhost:$PORT/healthz" >/dev/null 2>&1; then break; fi
  sleep 0.2
done

BASE_URL="http://localhost:$PORT" go test -tags acceptance -count=1 ./acceptance/go/...
