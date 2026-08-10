# ObjeX — single-node demo
Set-Location $PSScriptRoot\..

Write-Host "Starting ObjeX..." -ForegroundColor Cyan
docker compose up -d --build

Write-Host ""
Write-Host "Ready:" -ForegroundColor Green
Write-Host "  S3 API:  http://localhost:9000"
Write-Host "  Health:  http://localhost:9000/health/ready"
Write-Host "  Metrics: http://localhost:9000/metrics"
Write-Host ""
Write-Host "Stop: docker compose down"
