#!/usr/bin/env bash
# Corre un script de k6 por niveles de usuarios y guarda una fila por corrida en <resultados>/<etiqueta>.csv.
# Uso: perf/lib/barrido.sh <script.js> <carpeta_resultados> <etiqueta> <niveles...>
# Variables: REPS (corridas por nivel, 1), RAMPA (15s), SOSTENIDA (60s), PAUSA (20 s entre corridas), REPOSO_MAX (300 s),
#            INTERVALO_STATS (10 s), K6_CPUS (núcleos del host para k6, 4-7), BASE_URL (https://localhost),
#            SERVICIOS (contenedores vigilados, "ratings-service traefik reverse-proxy").
set -uo pipefail
SCRIPT="$(realpath "${1:?falta el script de k6}")"; shift
RESULTADOS="$(realpath -m "${1:?falta la carpeta de resultados}")"; shift
ETIQUETA="${1:?falta la etiqueta}"; shift
NIVELES="${*:-1 50 100 200 300 400 500}"
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPS="${REPS:-1}"
RAMPA="${RAMPA:-15s}"
SOSTENIDA="${SOSTENIDA:-60s}"
PAUSA="${PAUSA:-20}"
K6_CPUS="${K6_CPUS:-4-7}"
BASE_URL="${BASE_URL:-https://localhost}"
SERVICIOS="${SERVICIOS:-ratings-service traefik reverse-proxy}"
export DOCKER_CONTEXT="${DOCKER_CONTEXT:-desktop-linux}"
export SSL_CERT_FILE="$REPO/api-gateway/ca/bololo-dev-ca.crt"
OUT="$RESULTADOS/$ETIQUETA"
CSV="$RESULTADOS/$ETIQUETA.csv"
mkdir -p "$OUT"
cd "$REPO" || exit 1

vigilados() {
  docker compose ps -q $SERVICIOS
}

esperar_reposo() {
  local inicio=$SECONDS quietos=0 linea
  while (( SECONDS - inicio < ${REPOSO_MAX:-300} )); do
    linea=$(docker stats --no-stream --format '{{.Name}} {{.CPUPerc}}' $(vigilados) | tr -d '%')
    if awk '$2 >= 5 {exit 1}' <<<"$linea" && ! docker inspect -f '{{.State.Health.Status}}' $(vigilados) | grep -qv '^healthy$'; then
      quietos=$((quietos + 1))
      (( quietos >= 2 )) && { echo "  reposo en $((SECONDS - inicio)) s"; return; }
    else
      quietos=0
    fi
    sleep 5
  done
  echo "  sin reposo tras ${REPOSO_MAX:-300} s"
}

reinicios() {
  docker inspect -f '{{.RestartCount}}' $(docker compose ps -q) 2>/dev/null | awk '{s+=$1} END{print s+0}'
}

echo "=== $(date +%T) $ETIQUETA · $(basename "$SCRIPT") · niveles: $NIVELES · reps: $REPS · rampa $RAMPA · sostenida $SOSTENIDA"
for vus in $NIVELES; do
  for rep in $(seq 1 "$REPS"); do
    base="$OUT/vus${vus}_r${rep}"
    antes=$(reinicios)
    desde=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    (
      while true; do
        echo "--- $(date +%T)"
        docker stats --no-stream --format '{{.Name}} cpu={{.CPUPerc}} mem={{.MemUsage}}'
        sleep "${INTERVALO_STATS:-10}"
      done
    ) >"$base.stats.txt" 2>&1 &
    monitor=$!
    (
      while true; do
        echo "$(date +%T) $(docker inspect -f '{{.Name}}={{.State.Health.Status}}' $(vigilados) | tr '\n' ' ')"
        sleep 2
      done
    ) >"$base.salud.txt" 2>&1 &
    salud=$!
    /usr/bin/time -v -o "$base.time.txt" taskset -c "$K6_CPUS" \
      k6 run --no-color -q -e VUS="$vus" -e RAMPA="$RAMPA" -e SOSTENIDA="$SOSTENIDA" -e BASE_URL="$BASE_URL" \
      --summary-export "$base.json" "$SCRIPT" >"$base.k6.txt" 2>&1
    kill "$monitor" "$salud" 2>/dev/null; wait "$monitor" "$salud" 2>/dev/null
    grep -E 'unhealthy|starting' "$base.salud.txt" | head -3
    docker compose logs --no-log-prefix --since "$desde" reverse-proxy >"$base.nginx.txt" 2>/dev/null
    REINICIOS=$(( $(reinicios) - antes )) python3 -I "$REPO/perf/lib/resumir.py" "$CSV" "$ETIQUETA" "$vus" "$rep" \
      "${SOSTENIDA%s}" "$base.json" "$base.stats.txt" "$base.nginx.txt" "$base.time.txt"
    grep -o '"status":5[0-9][0-9]' "$base.nginx.txt" | sort | uniq -c | head -3
    grep -E '\[(crit|alert|emerg)\]' "$base.nginx.txt" | sed 's/^.*\[\(crit\|alert\|emerg\)\]/[\1]/' | sort | uniq -c | head -3
    gzip -f "$base.nginx.txt"
    sleep "$PAUSA"
    esperar_reposo
  done
done
echo "=== $(date +%T) fin"
