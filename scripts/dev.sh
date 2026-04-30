#!/usr/bin/env bash
# One-shot dev launcher: build the SPA, bring up infra+Go services+nginx, wait for health.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB="$ROOT/golive-web"
COMPOSE_DIR="$ROOT/golive-backend/deploy"

echo "[dev] building golive-web (production bundle)..."
cd "$WEB"
if [ ! -d node_modules ]; then
  pnpm install
fi
pnpm build

echo "[dev] docker compose up -d..."
cd "$COMPOSE_DIR"
docker compose up -d

wait_for() {
  local name="$1" url="$2" tries=60
  echo -n "[dev] waiting for $name "
  while ((tries--)); do
    if curl -fsS -o /dev/null "$url"; then echo " ok"; return 0; fi
    echo -n "."; sleep 2
  done
  echo " FAILED ($url)"; return 1
}

wait_for "nginx"  "http://localhost/"                     || true
wait_for "api-gw" "http://localhost:8080/healthz"          || true
wait_for "im-gw"  "http://localhost:8081/healthz"          || true
wait_for "srs"    "http://localhost:8082/api/v1/versions"  || true

cat <<EOF

----------------------------------------
  Web   : http://localhost
  API   : http://localhost:8080/api
  WS    : ws://localhost:8081/ws
  SRS   : http://localhost:8082
  Graf. : http://localhost:3000  (admin/admin)
  Jaeger: http://localhost:16686
----------------------------------------
EOF
