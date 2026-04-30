# One-shot dev launcher (Windows PowerShell).
# Frontend is built locally; infra, Go services, nginx, and observability run in Docker Compose.
$ErrorActionPreference = 'Stop'

$Root       = Split-Path -Parent $PSScriptRoot
$Web        = Join-Path $Root 'golive-web'
$ComposeDir = Join-Path $Root 'golive-backend\deploy'

Write-Host "[dev] building golive-web (production bundle)..." -ForegroundColor Cyan
Push-Location $Web
try {
  if (-not (Test-Path 'node_modules')) { pnpm install }
  pnpm build
} finally { Pop-Location }

Write-Host "[dev] docker compose up -d..." -ForegroundColor Cyan
Push-Location $ComposeDir
try { docker compose up -d } finally { Pop-Location }

function Wait-For($name, $url, $tries = 60) {
  Write-Host -NoNewline "[dev] waiting for $name "
  for ($i = 0; $i -lt $tries; $i++) {
    try {
      $r = Invoke-WebRequest -UseBasicParsing -Uri $url -TimeoutSec 2 -ErrorAction Stop
      if ($r.StatusCode -lt 500) { Write-Host " ok"; return $true }
    } catch { }
    Write-Host -NoNewline "."
    Start-Sleep -Seconds 2
  }
  Write-Host " FAILED ($url)"
  return $false
}

Wait-For 'nginx'  'http://localhost/'                       | Out-Null
Wait-For 'api-gw' 'http://localhost:8080/healthz'            | Out-Null
Wait-For 'im-gw'  'http://localhost:8081/healthz'            | Out-Null
Wait-For 'srs'    'http://localhost:8082/api/v1/versions'    | Out-Null

@"

----------------------------------------
  Web   : http://localhost
  API   : http://localhost:8080/api
  WS    : ws://localhost:8081/ws
  SRS   : http://localhost:8082
  Graf. : http://localhost:3000  (admin/admin)
  Jaeger: http://localhost:16686
----------------------------------------
"@ | Write-Host
