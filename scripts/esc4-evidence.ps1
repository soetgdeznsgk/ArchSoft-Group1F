# Regenera la evidencia del escenario 4 (Limited View) desde PowerShell (Windows).
#
# En este equipo, Docker Desktop no expone 'docker' dentro de WSL, así que la
# versión .sh no sirve: hay que correr esto en PowerShell, donde 'docker' existe.
#
# Uso:
#   .\scripts\esc4-evidence.ps1 despues   # pruebas nuevas (con -v) y la demo
#   .\scripts\esc4-evidence.ps1 antes     # solo la prueba que ya existía en main
#
# Requiere Docker Desktop corriendo y, para la demo, el stack arriba
# (docker compose up -d --wait).

param(
  [Parameter(Mandatory = $true)]
  [ValidateSet('antes', 'despues')]
  [string]$Phase
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$goImage = 'golang:1.27.2-bookworm'
$project = 'bololo-integrado'
$net = "${project}_public"
$gatewayHost = 'reverse-proxy'

$outDir = Join-Path $root "docs\p3\evidencia\esc4\$Phase"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$gl = Join-Path $root 'game-logic'

if ($Phase -eq 'antes') {
  $f = Join-Path $outDir '01-codigo.txt'
  "`$ sed -n '/type PlayerSummary/,/^}/p' internal/game/view.go" | Set-Content -Encoding utf8 $f
  Get-Content (Join-Path $gl 'internal\game\view.go') |
    Select-String -Pattern 'type PlayerSummary' -Context 0, 10 |
    ForEach-Object { $_.Line; $_.Context.PostContext } | Add-Content -Encoding utf8 $f
  "" | Add-Content $f
  "`$ grep identidad en websocket.go" | Add-Content -Encoding utf8 $f
  Get-Content (Join-Path $gl 'internal\server\websocket.go') |
    Select-String -Pattern 'verifyGameToken|slices.Contains|StatusForbidden|StatusUnauthorized|engine.Apply\(c.player' |
    ForEach-Object { $_.Line.Trim() } | Add-Content -Encoding utf8 $f

  $t = Join-Path $outDir '02-tests-antes.txt'
  "`$ go test -run TestViewFor ./internal/game/" | Set-Content -Encoding utf8 $t
  docker run --rm -v "${gl}:/src" -w /src $goImage go test -run TestViewFor ./internal/game/ 2>&1 | Add-Content -Encoding utf8 $t
  Write-Host "Evidencia 'antes' en $outDir"
  return
}

# --- despues ---
$f = Join-Path $outDir '01-tests.txt'
$runPattern = 'TestViewHidesOtherPlayersCards|TestWebSocketRejectsTokensFromAnotherGameAndRoster|TestChameleonActionUsesTokenIdentityNotMessage|TestChameleonViewIsPublicByDesign'
"`$ go test -v -run '$runPattern' ./internal/game/ ./internal/server/" | Set-Content -Encoding utf8 $f
docker run --rm -v "${gl}:/src" -w /src $goImage go test -v -run $runPattern ./internal/game/ ./internal/server/ 2>&1 | Add-Content -Encoding utf8 $f
"" | Add-Content $f
"`$ go test ./internal/game/ ./internal/server/" | Add-Content -Encoding utf8 $f
docker run --rm -v "${gl}:/src" -w /src $goImage go test ./internal/game/ ./internal/server/ 2>&1 | Add-Content -Encoding utf8 $f

$d = Join-Path $outDir '02-demo.txt'
"`$ limited_view_demo.py (dos clientes WebSocket a traves del proxy)" | Set-Content -Encoding utf8 $d
docker run --rm --network $net -e GATEWAY_HOST=$gatewayHost -e PYTHONIOENCODING=utf-8 -e PYTHONUNBUFFERED=1 `
  -v "${root}:/work" -w /work python:3.12-slim `
  sh -c "pip install -q --root-user-action=ignore websockets==15.0.1 && python game-logic/scripts/limited_view_demo.py" 2>&1 |
  Add-Content -Encoding utf8 $d

Write-Host "Evidencia 'despues' en $outDir"
