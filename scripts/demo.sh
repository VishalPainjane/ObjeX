#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
echo "Starting ObjeX..."
docker compose up -d --build
cat <<'EOF'

Ready:
  S3 API:  http://localhost:9000
  Health:  http://localhost:9000/health/ready
  Metrics: http://localhost:9000/metrics

Stop: docker compose down
EOF
