#!/usr/bin/env bash
# Run the hidden holdout suites (Go + Playwright) from $HOLDOUT_DIR against a
# freshly built binary. Workers and reviewers never see the sources; the caller
# only reads this script's pass/fail summary (plan §9.2).
set -euo pipefail

HOLDOUT_DIR="${HOLDOUT_DIR:?HOLDOUT_DIR must be set (absolute path)}"
ROOT="$(git rev-parse --show-toplevel)"
TMP="$(mktemp -d)"
PORT=$(( (RANDOM % 2000) + 20000 ))

make -C "$ROOT" build >/dev/null

BASE_URL="http://localhost:$PORT" \
PORT="$PORT" APP_ENV=test \
DB_PATH="$TMP/holdout.db" PHOTO_DIR="$TMP/photos" \
SESSION_KEY="dGVzdC1zZXNzaW9uLWtleS0zMi1ieXRlcy1hYQ==" \
IP_HASH_KEY="dGVzdC1pcGhhc2gta2V5LTMyLWJ5dGVzLWJi" \
ADMIN_TOKEN="dGVzdC1hZG1pbi10b2tlbi0zMi1ieXRl" \
TOWN_BBOX="42.70,-71.09,42.82,-70.96" TZ="America/New_York" \
"$ROOT/bin/server" >"$TMP/server.log" 2>&1 &
SERVER_PID=$!
trap 'kill $SERVER_PID 2>/dev/null || true' EXIT

for _ in $(seq 1 50); do
  curl -sf "http://localhost:$PORT/healthz" >/dev/null 2>&1 && break
  sleep 0.2
done

status=0

# Go holdout tests, if present.
if ls "$HOLDOUT_DIR"/go/*_test.go >/dev/null 2>&1; then
  ( cd "$HOLDOUT_DIR/go" && BASE_URL="http://localhost:$PORT" go test -tags acceptance -count=1 ./... ) || status=1
fi

# Playwright holdout specs, if present.
if ls "$HOLDOUT_DIR"/e2e/*.spec.ts >/dev/null 2>&1; then
  ( cd "$HOLDOUT_DIR" && [ -f package.json ] && cp -R "$ROOT/node_modules" ./node_modules 2>/dev/null || true; \
    BASE_URL="http://localhost:$PORT" npx playwright test --config "$HOLDOUT_DIR/playwright.config.ts" --pass-with-no-tests ) || status=1
fi

if [ "$status" -eq 0 ]; then echo "holdout: PASS"; else echo "holdout: FAIL"; exit 1; fi
