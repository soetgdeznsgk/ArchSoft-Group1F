param(
  [Parameter(Mandatory = $true)]
  [ValidateSet("guest", "register", "login", "me")]
  [string]$Operacion,
  [int[]]$Niveles = @(1, 50, 100, 200, 400, 800, 1200, 2000),
  [int]$Repeticiones = 1,
  [string]$ModoLogin = "valid"
)

$ErrorActionPreference = "Continue"
$repo = (Resolve-Path "$PSScriptRoot\..\..").Path
$script = Join-Path $PSScriptRoot "$Operacion.js"
$resultados = Join-Path $PSScriptRoot "resultados"
$out = Join-Path $resultados $Operacion
$csv = Join-Path $resultados "$Operacion.csv"
$baseUrl = "https://localhost"
$rampa = "15s"
$sostenida = "60s"
$env:SSL_CERT_FILE = Join-Path $repo "api-gateway\ca\bololo-dev-ca.crt"
$env:DOCKER_CONTEXT = "desktop-linux"

New-Item -ItemType Directory -Force -Path $out | Out-Null
$servicios = @("auth", "auth-postgres", "traefik", "reverse-proxy")

foreach ($vus in $Niveles) {
  for ($rep = 1; $rep -le $Repeticiones; $rep++) {
    $healthDeadline = (Get-Date).AddSeconds(300)
    while ((Get-Date) -lt $healthDeadline) {
      $states = foreach ($name in $servicios) {
        $id = docker compose ps -q $name
        if ($id) { docker inspect -f '{{.State.Health.Status}}' $id }
      }
      if (($states | Where-Object { $_ -ne "healthy" }).Count -eq 0) { break }
      Start-Sleep -Seconds 5
    }
    if ($Operacion -eq "register") {
      docker compose exec -T auth-postgres psql -U bololo_auth -d bololo_auth -c "TRUNCATE TABLE accounts;" | Out-Null
    }
    $base = Join-Path $out "vus${vus}_r${rep}"
    $stats = "$base.stats.txt"
    $health = "$base.salud.txt"
    $k6Text = "$base.k6.txt"
    $json = "$base.json"
    $nginx = "$base.nginx.txt"
    $time = "$base.time.txt"

    $statsJob = Start-Job -ScriptBlock {
      param($destination)
      while ($true) {
        $stamp = Get-Date -Format o
        Add-Content -Path $destination -Value "--- $stamp"
        docker stats --no-stream --format '{{.Name}} cpu={{.CPUPerc}} mem={{.MemUsage}}' |
          Add-Content -Path $destination
        Start-Sleep -Seconds 5
      }
    } -ArgumentList $stats

    $healthJob = Start-Job -ScriptBlock {
      param($destination, $names)
      while ($true) {
        $stamp = Get-Date -Format o
        $parts = foreach ($name in $names) {
          $id = docker compose ps -q $name
          if ($id) {
            $state = docker inspect -f '{{.State.Health.Status}}' $id
            "$name=$state"
          }
        }
        Add-Content -Path $destination -Value "$stamp $($parts -join ' ')"
        Start-Sleep -Seconds 2
      }
    } -ArgumentList $health, $servicios

    $started = Get-Date
    $arguments = @(
      "run", "--no-color", "-q",
      "-e", "VUS=$vus",
      "-e", "RAMPA=$rampa",
      "-e", "SOSTENIDA=$sostenida",
      "-e", "BASE_URL=$baseUrl",
      "-e", "MODE=$ModoLogin",
      "-e", "RUN_ID=r${vus}x${rep}",
      "--summary-export", $json,
      $script
    )
    & k6.exe @arguments *> $k6Text
    $exitCode = $LASTEXITCODE
    $elapsed = (Get-Date) - $started
    Set-Content -Path $time -Value "ElapsedSeconds=$([int]$elapsed.TotalSeconds)`nK6ExitCode=$exitCode"

    Stop-Job $statsJob, $healthJob
    Remove-Job $statsJob, $healthJob -Force

    $nginxText = docker compose logs --no-log-prefix --since $started.ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ") reverse-proxy 2>$null
    Set-Content -Path $nginx -Value ($nginxText -join "`n")

    $reinicios = 0
    foreach ($id in docker compose ps -q) {
      if ($id) {
        $reinicios += [int](docker inspect -f '{{.RestartCount}}' $id)
      }
    }
    $env:REINICIOS = [string]$reinicios
    python3 -I (Join-Path $repo "perf\lib\resumir.py") $csv $Operacion $vus $rep 60 $json $stats $nginx $time
  }
}
