$ErrorActionPreference = "Stop"
$repo = (Resolve-Path "$PSScriptRoot\..\..").Path
Set-Location $repo

docker compose up -d --build --wait auth auth-postgres traefik reverse-proxy

$username = "bololo_auth"
$database = "bololo_auth"
$accounts = docker compose exec -T auth-postgres psql -U $username -d $database -tAc "TRUNCATE TABLE accounts;"
if ($LASTEXITCODE -ne 0) {
  throw "No se pudo preparar auth-postgres"
}

Write-Host "Auth-postgres listo y tabla accounts vaciada."
