param(
  [int[]]$Partidas = @(1, 2, 4, 6, 8, 10),
  [int]$Repeticiones = 1,
  [int]$TimeoutSegundos = 600
)

$ErrorActionPreference = "Continue"
$repo = (Resolve-Path "$PSScriptRoot\..\..").Path
$script = Join-Path $PSScriptRoot "partida.js"
$resultados = Join-Path $PSScriptRoot "resultados"
New-Item -ItemType Directory -Force -Path $resultados | Out-Null
$csv = Join-Path $resultados "partidas.csv"
$env:SSL_CERT_FILE = Join-Path $repo "api-gateway\ca\bololo-dev-ca.crt"

foreach ($n in $Partidas) {
  for ($rep = 1; $rep -le $Repeticiones; $rep++) {
    $pgUser = (docker compose exec -T postgres sh -lc 'printf %s "$POSTGRES_USER"').Trim()
    $pgDb = (docker compose exec -T postgres sh -lc 'printf %s "$POSTGRES_DB"').Trim()
    docker compose exec -T postgres psql -U $pgUser -d $pgDb `
      -c "TRUNCATE TABLE game_results, lobby_players, lobbies, players CASCADE;" 2>$null
    docker compose exec -T redis sh -lc 'redis-cli FLUSHDB >/dev/null' 2>$null

    $base = Join-Path $resultados "partidas${n}_r${rep}"
    $stats = "$base.stats.txt"
    $health = "$base.salud.txt"
    $started = Get-Date

    $statsJob = Start-Job -ScriptBlock {
      param($destination)
      while ($true) {
        Add-Content -Path $destination -Value "--- $(Get-Date -Format o)"
        docker stats --no-stream --format '{{.Name}} cpu={{.CPUPerc}} mem={{.MemUsage}}' |
          Add-Content -Path $destination
        Start-Sleep -Seconds 5
      }
    } -ArgumentList $stats

    $healthJob = Start-Job -ScriptBlock {
      param($destination)
      $names = @("auth","auth-postgres","lobby-manager","postgres","game-logic","bot-service","redis","rabbitmq","ratings-service","ratings-postgres")
      while ($true) {
        $parts = foreach ($name in $names) {
          $id = docker compose ps -q $name
          if ($id) {
            $state = docker inspect -f '{{.State.Health.Status}}' $id
            "$name=$state"
          }
        }
        Add-Content -Path $destination -Value "$(Get-Date -Format o) $($parts -join ' ')"
        Start-Sleep -Seconds 2
      }
    } -ArgumentList $health

    & k6.exe run --no-color -q `
      -e PARTIDAS=$n `
      -e TIMEOUT_MS=$($TimeoutSegundos * 1000) `
      -e BASE_URL=https://localhost `
      -e WS_URL=wss://localhost `
      --summary-export "$base.json" $script *> "$base.k6.txt"

    Stop-Job $statsJob, $healthJob
    Remove-Job $statsJob, $healthJob -Force

    docker compose logs --no-log-prefix --since $started.ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ") `
      game-logic lobby-manager bot-service ratings-service rabbitmq redis > "$base.servicios.txt"

    Write-Host "Corrida partidas=$n rep=$rep finalizada"
  }
}
