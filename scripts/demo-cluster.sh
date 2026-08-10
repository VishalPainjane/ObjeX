#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
echo "Starting ObjeX cluster (3 nodes)..."
docker compose -f docker-compose.cluster.yml up -d --build
cat <<'EOF'

Cluster ready:
  Node 1:  http://localhost:9001  (/cluster)
  Node 2:  http://localhost:9002
  Node 3:  http://localhost:9003

Tests: go test ./...
Stop:  docker compose -f docker-compose.cluster.yml down
EOF
