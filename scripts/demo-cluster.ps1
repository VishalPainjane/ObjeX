# ObjeX — 3-node cluster demo
Set-Location $PSScriptRoot\..

Write-Host "Starting ObjeX cluster (3 nodes)..." -ForegroundColor Cyan
docker compose -f docker-compose.cluster.yml up -d --build

Write-Host ""
Write-Host "Cluster ready:" -ForegroundColor Green
Write-Host "  Node 1:  http://localhost:9001  (/cluster)"
Write-Host "  Node 2:  http://localhost:9002"
Write-Host "  Node 3:  http://localhost:9003"
Write-Host ""
Write-Host "Tests: go test ./..."
Write-Host "Stop:  docker compose -f docker-compose.cluster.yml down"
