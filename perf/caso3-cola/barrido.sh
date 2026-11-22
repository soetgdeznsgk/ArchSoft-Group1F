#!/usr/bin/env bash
# Caso 3 (cola de resultados): publica game.finished a cada tasa y mide cuánto tarda ratings en procesarlos.
# Uso: ./perf/caso3-cola/barrido.sh <etiqueta> <tasas por segundo...>
# Variables: REPS (corridas por tasa, 1), SEGUNDOS (60 de publicación), DRENAJE_MAX (600 s), PAUSA (10 s).
set -uo pipefail
AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$AQUI/../.." && pwd)"
ETIQUETA="${1:?falta la etiqueta}"; shift
TASAS="${*:?faltan las tasas}"
REPS="${REPS:-1}"
SEGUNDOS="${SEGUNDOS:-60}"
DRENAJE_MAX="${DRENAJE_MAX:-600}"
PAUSA="${PAUSA:-10}"
export DOCKER_CONTEXT="${DOCKER_CONTEXT:-desktop-linux}"
OUT="$AQUI/resultados/$ETIQUETA"
CSV="$AQUI/resultados/$ETIQUETA.csv"
mkdir -p "$OUT"
cd "$REPO" || exit 1

psql_ratings() {
  docker compose exec -T ratings-postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -AtF"|" -c "$0"' "$1"
}

reinicios() {
  docker inspect -f '{{.RestartCount}}' $(docker compose ps -q) 2>/dev/null | awk '{s+=$1} END{print s+0}'
}

echo "=== $(date +%T) $ETIQUETA · tasas: $TASAS · reps: $REPS · publicación ${SEGUNDOS}s"
for tasa in $TASAS; do
  for rep in $(seq 1 "$REPS"); do
    corrida="${ETIQUETA}-${tasa}-r${rep}"
    base="$OUT/tasa${tasa}_r${rep}"
    antes=$(reinicios)
    (
      while true; do
        echo "--- $(date +%T)"
        docker stats --no-stream --format '{{.Name}} cpu={{.CPUPerc}} mem={{.MemUsage}}'
        sleep 5
      done
    ) >"$base.stats.txt" 2>&1 &
    monitor=$!
    docker compose exec -T lobby-manager python - "$((SEGUNDOS + 5))" "$((SEGUNDOS + DRENAJE_MAX))" \
      <"$AQUI/vigilar_cola.py" >"$base.cola.txt" 2>&1 &
    vigilante=$!
    docker compose exec -T lobby-manager python - "$tasa" "$SEGUNDOS" "$corrida" \
      <"$AQUI/publicar.py" >"$base.publicacion.json" 2>&1
    wait "$vigilante"
    kill "$monitor" 2>/dev/null; wait "$monitor" 2>/dev/null
    psql_ratings "SELECT count(*),
        percentile_cont(0.5) WITHIN GROUP (ORDER BY d), percentile_cont(0.95) WITHIN GROUP (ORDER BY d),
        percentile_cont(0.99) WITHIN GROUP (ORDER BY d), max(d), count(*) FILTER (WHERE d > 60),
        extract(epoch FROM max(processed_at) - min(processed_at))
      FROM (SELECT extract(epoch FROM processed_at - finished_at) AS d, processed_at
            FROM processed_games WHERE game_id LIKE 'perf-${corrida}-%') x" >"$base.bd.txt"
    python3 -I "$AQUI/resumir_cola.py" "$CSV" "$ETIQUETA" "$tasa" "$rep" "$base.publicacion.json" \
      "$base.cola.txt" "$base.bd.txt" "$base.stats.txt" "$(( $(reinicios) - antes ))"
    sleep "$PAUSA"
  done
done
echo "=== $(date +%T) fin"
