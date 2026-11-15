#!/usr/bin/env bash
# Costo del canal en latencia (escenario 1, Secure Channel).
# Mide N peticiones GET a una ruta liviana del gateway en dos casos:
#   - conexión nueva por petición (paga TCP + TLS cada vez: peor caso, primera visita)
#   - conexión reutilizada (keep-alive / HTTP/2: lo normal dentro de una página)
# Uso: ./scripts/latency.sh [BASE_URL] [N]     (https://localhost y 100 por defecto)

set -uo pipefail
export MSYS_NO_PATHCONV=1
ROOT="${ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$ROOT" || exit 1

BASE_URL="${1:-${BASE_URL:-https://localhost}}"
N="${2:-100}"
URL_PATH="${URL_PATH:-/api/ratings/leaderboard?mode=CLASSIC&limit=1}"
CA_CERT="${CA_CERT:-api-gateway/ca/bololo-dev-ca.crt}"
URL="$BASE_URL$URL_PATH"
# Git Bash: con MSYS_NO_PATHCONV=1 el curl nativo no puede abrir /dev/null (sale con 23).
case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) NULL_DEV=nul ;; *) NULL_DEV=/dev/null ;; esac
curl() {
  local args=() a
  for a in "$@"; do [[ $a == /dev/null ]] && a=$NULL_DEV; args+=("$a"); done
  command curl "${args[@]}"
}
CURL=(curl -s --max-time 5)
[[ "$BASE_URL" == https://* ]] && CURL+=(--cacert "$CA_CERT" --ssl-revoke-best-effort)
# Opciones extra de curl, separadas por espacios. Ejemplo, desde un contenedor de la
# red public: EXTRA_CURL_OPTS="--connect-to localhost:443:traefik:443"
# shellcheck disable=SC2206
[[ -n "${EXTRA_CURL_OPTS:-}" ]] && CURL+=($EXTRA_CURL_OPTS)

# Percentiles en milisegundos de una lista de segundos (uno por línea).
stats() {
  sort -n | awk '{ v[NR] = $1 * 1000 } END {
    if (NR == 0) { print "sin datos"; exit }
    printf "mediana %6.2f ms   p95 %6.2f ms   min %6.2f ms   max %6.2f ms", v[int((NR + 1) / 2)], v[int(NR * 0.95 + 0.5)], v[1], v[NR]
  }'
}

"${CURL[@]}" -o /dev/null "$URL" # calentamiento

new_conn=$(for _ in $(seq 1 "$N"); do
  "${CURL[@]}" -o /dev/null -w '%{time_connect} %{time_appconnect} %{time_total}\n' "$URL"
done)

# Una sola invocación de curl con N+1 URLs: reutiliza la conexión. Se descarta la
# primera transferencia, que es la que abre la conexión. "-o" va con cada URL.
urls=()
for _ in $(seq 0 "$N"); do urls+=(-o /dev/null "$URL"); done
reused=$("${CURL[@]}" -w '%{time_total}\n' "${urls[@]}" | tail -n +2)

printf 'Latencia por el gateway: %s (N=%s)\n' "$URL" "$N"
printf '  cliente                      %s\n' "$(command curl -V | head -1 | cut -d' ' -f1-5)"
printf '  protocolo negociado          %s\n' "$("${CURL[@]}" -o /dev/null -w 'HTTP/%{http_version}' "$URL")"
printf '  conexión nueva, TCP          %s\n' "$(awk '{print $1}' <<<"$new_conn" | stats)"
printf '  conexión nueva, TCP + TLS    %s\n' "$(awk '{print ($2 > 0 ? $2 : $1)}' <<<"$new_conn" | stats)"
printf '  conexión nueva, total        %s\n' "$(awk '{print $3}' <<<"$new_conn" | stats)"
printf '  conexión reutilizada, total  %s\n' "$(stats <<<"$reused")"
