#!/usr/bin/env bash
# Regenera la evidencia del escenario 5 (Rate Limiting) en docs/p3/evidencia/esc5/<fase>/.
# Cada archivo muestra el comando exacto ($ ...) seguido de su salida.
#
# Uso: ./scripts/esc5-evidence.sh antes|despues
#   antes    stack levantado desde main (sin límite de tasa)
#   despues  stack levantado desde feature/p3-esc5 (límite por IP en reverse-proxy)
#
# El atacante corre en un contenedor de la red public: llega al proxy como un cliente
# de Internet, con su propia IP. El usuario legítimo es el host. Así se ve que el límite
# frena al atacante sin tocar a los demás, y el cupo del host no se gasta.
# Requiere docker y curl, el stack arriba y el certificado de scripts/gen-certs.sh.

set -uo pipefail
export MSYS_NO_PATHCONV=1
export LC_ALL=C
ROOT="${ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$ROOT" || exit 1

PHASE="${1:-}"
if [[ "$PHASE" != "antes" && "$PHASE" != "despues" ]]; then
  echo "uso: $0 antes|despues" >&2
  exit 2
fi
D="docs/p3/evidencia/esc5/$PHASE"
mkdir -p "$D"
PROJECT=bololo-integrado
FLOOD_SECONDS="${FLOOD_SECONDS:-20}"

bash scripts/bench-image.sh
HOST_DIR=$(pwd -W 2>/dev/null || pwd)
# Dentro del contenedor: valida el certificado contra la CA local y conecta al proxy por su nombre en la red public.
export C="--cacert api-gateway/ca/bololo-dev-ca.crt --connect-to localhost:443:reverse-proxy:443"
# Desde el host (Git Bash usa curl con Schannel: la CA local no publica CRL).
export H="--cacert api-gateway/ca/bololo-dev-ca.crt --ssl-revoke-best-effort"
case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) NULL_DEV=nul ;; *) NULL_DEV=/dev/null ;; esac
export LEADERBOARD="https://localhost/api/ratings/leaderboard?mode=CLASSIC&limit=10"
export ATTACKER="docker run --rm -i --network ${PROJECT}_public -e C -e LEADERBOARD -v $HOST_DIR:/w -w /w bololo-bench:curl"

# Los contadores viven en la memoria compartida de Nginx: un atacante de una corrida anterior
# (por ejemplo, e2e.sh) que reciba la misma IP empezaría con el cupo gastado. Se reinician.
if [[ "$PHASE" == "despues" ]]; then
  docker compose restart reverse-proxy >/dev/null 2>&1 && docker compose up -d --wait reverse-proxy >/dev/null 2>&1
fi

run() {
  printf '$ %s\n' "$1"
  bash -c "$1" 2>&1 </dev/null | sed "s#$ROOT#.#g"
  printf '\n'
}

# El usuario legítimo: una petición cada 0,1 s desde el host durante FLOOD_SECONDS.
# Imprime cuántas respuestas hubo por código y los percentiles de latencia.
legit_sample() {
  local end=$((SECONDS + FLOOD_SECONDS)) out
  # shellcheck disable=SC2086
  out=$(while ((SECONDS < end)); do
    curl -s $H --max-time 5 -o "$NULL_DEV" -w '%{http_code} %{time_total}\n' "$LEADERBOARD"
    sleep 0.1
  done)
  awk '{ c[$1]++ } END { for (k in c) printf "  %s: %d respuestas\n", k, c[k] }' <<<"$out"
  awk '{ print $2 * 1000 }' <<<"$out" | sort -n | awk '{ v[NR] = $1 } END {
    printf "  latencia (ms): mediana %.1f · p95 %.1f · p99 %.1f · max %.1f (n=%d)\n", v[int((NR + 1) / 2)], v[int(NR * 0.95 + 0.5)], v[int(NR * 0.99 + 0.5)], v[NR], NR
  }'
}

# El atacante: lotes de 500 peticiones en paralelo (50 a la vez) durante FLOOD_SECONDS.
FLOOD_CMD='end=$((SECONDS + '"$FLOOD_SECONDS"')); total=0
while ((SECONDS < end)); do
  args=(); for i in $(seq 1 500); do args+=(-o /dev/null "$LEADERBOARD&n=$i"); done
  curl -s $C --parallel --parallel-max 50 --max-time 10 -w "%{http_code}\n" "${args[@]}"
done | sort | uniq -c | awk "{ print \"  \" \$2 \": \" \$1 \" respuestas\"; t += \$1 } END { printf \"  total: %d en ~%d s (%.0f peticiones/s)\n\", t, '"$FLOOD_SECONDS"', t / '"$FLOOD_SECONDS"' }"'

{
  printf '# Relleno de credenciales: 100 logins seguidos con usuarios distintos, desde la red public.\n'
  printf '# Cada usuario existe una sola vez, así que el bloqueo por cuenta de auth (5 fallos) nunca salta.\n\n'
  run "\$ATTACKER bash -c 'for i in \$(seq 1 100); do curl -s -o /dev/null \$C -w \"%{http_code}\\n\" -X POST https://localhost/auth/login -H \"Content-Type: application/json\" -d \"{\\\"username\\\":\\\"rl_\${RANDOM}_\$i\\\",\\\"password\\\":\\\"incorrecta-123\\\"}\"; done | sort | uniq -c'"
} >"$D/01-fuerza-bruta.txt"

{
  printf '# Inundación de la API: lotes de 500 GET al leaderboard (50 en paralelo, HTTP/2) durante %s s, desde la red public.\n' "$FLOOD_SECONDS"
  printf '# ratings-service es el cuello de botella medido en el Lab 6.\n\n'
  printf '$ $ATTACKER bash -c <inundación de %s s>\n' "$FLOOD_SECONDS"
  $ATTACKER bash -c "$FLOOD_CMD" 2>&1 </dev/null
  printf '\n'
} >"$D/02-inundacion-api.txt"

{
  printf '# Usuario legítimo (el host): una petición al leaderboard cada 0,1 s durante %s s.\n\n' "$FLOOD_SECONDS"
  printf '## A. Sin ataque (línea base)\n'
  legit_sample
  printf '\n## B. Mientras el contenedor atacante inunda la API\n'
  $ATTACKER bash -c "$FLOOD_CMD" >"$D/.flood.tmp" 2>&1 </dev/null &
  flood_pid=$!
  sleep 1
  legit_sample
  wait "$flood_pid"
  printf '\n## Lo que recibió el atacante en ese mismo intervalo\n'
  cat "$D/.flood.tmp"
  rm -f "$D/.flood.tmp"
} >"$D/03-usuario-legitimo.txt"

{
  printf '# Cómo responde el borde a quien excede el límite, y qué IP ve el proxy de cada cliente.\n\n'
  run "\$ATTACKER bash -c 'for i in \$(seq 1 30); do curl -s -o /dev/null \$C -X POST https://localhost/auth/login -H \"Content-Type: application/json\" -d \"{\\\"username\\\":\\\"rl_head_\$i\\\",\\\"password\\\":\\\"incorrecta-123\\\"}\"; done; curl -s -i \$C -X POST https://localhost/auth/login -H \"Content-Type: application/json\" -d \"{\\\"username\\\":\\\"rl_head_x\\\",\\\"password\\\":\\\"incorrecta-123\\\"}\"' | tr -d '\\r' | grep -viE '^(date|content-length):'"
  run "docker compose logs --no-log-prefix reverse-proxy 2>/dev/null | grep '\"status\":429' | tail -3"
  run "docker compose logs --no-log-prefix reverse-proxy 2>/dev/null | grep -c '\"status\":429.*\"upstream_status\":\"\"'"
  run "docker compose logs --no-log-prefix reverse-proxy 2>/dev/null | grep -oE '\"client\":\"[^\"]+\"' | sort | uniq -c"
  run "docker network inspect ${PROJECT}_public -f '{{range .IPAM.Config}}subred {{.Subnet}} · gateway {{.Gateway}}{{end}}'"
  run "docker compose exec -T reverse-proxy sh -c 'grep -hE \"limit_(req|conn)|map .* \\\$rl_key|error_page 429\" /etc/nginx/conf.d/*.conf'"
} >"$D/04-respuesta-429.txt"

if [[ "$PHASE" == "despues" ]]; then
  # Si el script se corta con el límite apagado, el proxy vuelve a on al salir.
  trap 'docker compose up -d reverse-proxy >/dev/null 2>&1' EXIT
  {
    printf '# Interruptor para las pruebas de carga: RATE_LIMIT=off quita el límite sin tocar la plantilla.\n\n'
    run "RATE_LIMIT=off docker compose up -d --wait reverse-proxy >/dev/null 2>&1; docker compose exec -T reverse-proxy printenv RATE_LIMIT"
    run "\$ATTACKER bash -c 'for i in \$(seq 1 40); do curl -s -o /dev/null \$C -w \"%{http_code}\\n\" -X POST https://localhost/auth/login -H \"Content-Type: application/json\" -d \"{\\\"username\\\":\\\"rl_off_\${RANDOM}_\$i\\\",\\\"password\\\":\\\"incorrecta-123\\\"}\"; done | sort | uniq -c'"
    run "docker compose up -d --wait reverse-proxy >/dev/null 2>&1; docker compose exec -T reverse-proxy printenv RATE_LIMIT"
  } >"$D/05-interruptor.txt"
  ./scripts/e2e.sh >"$D/06-e2e.txt" 2>&1
fi

cat "$D"/0[1-5]-*.txt
