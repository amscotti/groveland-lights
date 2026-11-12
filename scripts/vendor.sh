#!/usr/bin/env bash
# Copy pinned frontend libraries into web/static/vendor with versioned names.
# Run after changing package.json versions (plan §3.1: vendored, versions in names).
set -euo pipefail
ROOT="$(git rev-parse --show-toplevel)"
V="$ROOT/web/static/vendor"
mkdir -p "$V"

cp "$ROOT/node_modules/htmx.org/dist/htmx.min.js"        "$V/htmx-2.0.11.min.js"
cp "$ROOT/node_modules/@alpinejs/csp/dist/cdn.min.js"    "$V/alpine-csp-3.17.5.min.js"
cp "$ROOT/node_modules/@picocss/pico/css/pico.min.css"   "$V/pico-2.1.1.min.css"
if [ -f "$ROOT/node_modules/maplibre-gl/dist/maplibre-gl.min.js" ]; then
  cp "$ROOT/node_modules/maplibre-gl/dist/maplibre-gl.min.js" "$V/maplibre-6.18.0.min.js"
  cp "$ROOT/node_modules/maplibre-gl/dist/maplibre-gl.min.css" "$V/maplibre-6.18.0.min.css" 2>/dev/null || true
fi
ls -la "$V"
