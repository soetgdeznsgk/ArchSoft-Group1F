#!/usr/bin/env bash
# Regenera la evidencia del escenario 2 (Reverse Proxy) en docs/p3/evidencia/esc2/<fase>/.
# Cada archivo muestra el comando exacto ($ ...) seguido de su salida.
#
# Uso: ./scripts/esc2-evidence.sh antes|despues
#   antes    stack levantado desde main @ 878cec4 (Traefik es el borde)
#   despues  stack levantado desde feature/p3-esc2 (Nginx es el borde)
#
# El contenedor "expuesto" es el que publica puertos en el host; el script lo detecta
# solo, así que funciona igual en las dos fases.
# Requiere docker y curl, el stack arriba y el certificado de scripts/gen-certs.sh.

set -uo pipefail
export MSYS_NO_PATHCONV=1
ROOT="${ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$ROOT" || exit 1

PHASE="${1:-}"
if [[ "$PHASE" != "antes" && "$PHASE" != "despues" ]]; then
  echo "uso: $0 antes|despues" >&2
  exit 2
fi
D="docs/p3/evidencia/esc2/$PHASE"
mkdir -p "$D"
export BASE=https://localhost
export TLS="--cacert api-gateway/ca/bololo-dev-ca.crt --ssl-revoke-best-effort"
export PROJECT=bololo-integrado

run() {
  printf '$ %s\n' "$1"
  bash -c "$1" 2>&1 </dev/null | sed "s#$ROOT#.#g"
  printf '\n'
}

EXPOSED=$(docker ps --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Label "com.docker.compose.service"}} {{.Ports}}' |
  grep -- '->' | cut -d' ' -f1 | sort -u | head -1)
if [[ -z "$EXPOSED" ]]; then
  echo "No hay ningún contenedor del proyecto con puertos publicados: docker compose up -d --wait" >&2
  exit 1
fi
export CID
CID=$(docker ps -q --filter "label=com.docker.compose.project=$PROJECT" --filter "label=com.docker.compose.service=$EXPOSED")

{
  printf '# Contenedor expuesto a Internet: %s\n\n' "$EXPOSED"
  run "docker ps --filter label=com.docker.compose.project=$PROJECT --format '{{.Label \"com.docker.compose.service\"}}  {{.Ports}}' | grep -- '->'"
  run "docker inspect -f '{{range \$k, \$v := .NetworkSettings.Networks}}{{\$k}} {{end}}' \$CID"
  run "docker inspect -f '{{range .Mounts}}{{.Source}} -> {{.Destination}}{{println}}{{end}}' \$CID"
  run "docker inspect -f 'ReadonlyRootfs={{.HostConfig.ReadonlyRootfs}} CapDrop={{.HostConfig.CapDrop}} SecurityOpt={{.HostConfig.SecurityOpt}}' \$CID"
  run "docker exec \$CID wget -q -T 3 -O- http://lobby-manager:8000/healthz"
  run "docker exec \$CID wget -q -T 3 -O- http://auth:8080/healthz"
  run "docker exec \$CID wget -q -T 3 -O /dev/null http://postgres:5432/"
} >"$D/01-borde.txt"

{
  for path in /internal/games /dashboard/ /api/rawdata /metrics /apiloquesea; do
    run "curl -s -i \$TLS --max-time 5 \"\$BASE$path\" | grep -iE '^(HTTP/|server:)|not_found|page not found|Not Found|detail'"
  done
  run "curl -s -i \$TLS --max-time 5 --path-as-is \"\$BASE/api/../internal/games\" | grep -iE '^(HTTP/|server:)|not_found|page not found|Not Found|detail'"
  run "head -c 102400 /dev/zero | tr '\\0' a | curl -s -o /dev/null \$TLS --max-time 5 -w '%{http_code}\\n' -H 'Content-Type: application/json' --data-binary @- \$BASE/auth/login"
  run "curl -s -I \$TLS --max-time 5 \$BASE/menu/ | grep -iE '^(server|x-powered-by|strict-transport-security|x-content-type-options|x-frame-options|referrer-policy):'"
  run "curl -s -i \$TLS --max-time 5 \$BASE/api/no-existe \$BASE/dashboard/ \$BASE/menu/ | grep -ci traefik"
} >"$D/02-rutas.txt"

./scripts/latency.sh "$BASE" 200 >"$D/03-latencia.txt" 2>&1

cat "$D"/0[1-3]-*.txt
