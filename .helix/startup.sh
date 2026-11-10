#!/bin/bash
# How this module boots on Helix. Helix runs the copy of this file on the
# project's helix-specs branch, with CWD = the code checkout.
#
#  • Web service (HELIX_WEB_SERVICE_PORT set): build and run the production
#    image in the foreground, data on the durable HELIX_WEB_SERVICE_DATA_DIR.
#  • Agent dev environment (no port): build, then start ./dev in the background:
#    the module on :8080 (hot-reloading UI; what HelixOS Dev preview frames) and
#    the local dev host on :8090. Log: /tmp/module-dev.log.
set -euo pipefail
echo "🚀 Module startup in $(pwd)"

for _ in $(seq 1 60); do docker info >/dev/null 2>&1 && break; sleep 2; done

if [ -n "${HELIX_WEB_SERVICE_PORT:-}" ]; then
  export GIT_SHA="$(git rev-parse --short HEAD 2>/dev/null || echo dev)"
  mkdir -p "${HELIX_WEB_SERVICE_DATA_DIR:-/data}"
  echo "🐳 web service: port=$HELIX_WEB_SERVICE_PORT data=${HELIX_WEB_SERVICE_DATA_DIR:-/data} sha=$GIT_SHA"
  exec docker compose -f docker-compose.prod.yml up --build --remove-orphans
fi

echo "🛠  agent dev mode: see AGENTS.md (run ./selftest to check everything)"
if command -v go >/dev/null 2>&1; then go build ./... || true; fi
if command -v npm >/dev/null 2>&1; then (cd web && npm ci --no-audit --no-fund) || true; fi
if command -v go >/dev/null 2>&1 && command -v npm >/dev/null 2>&1; then
  nohup ./dev > /tmp/module-dev.log 2>&1 &
  echo "▶  ./dev started: module http://localhost:8080, dev host http://localhost:8090 (log /tmp/module-dev.log)"
fi
