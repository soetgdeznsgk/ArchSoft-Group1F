#!/usr/bin/env bash
# Prueba de extremo a extremo del despliegue (docs/deployment.md, paso 7).
# Uso: ./scripts/e2e.sh [--down]
# Si game-logic todavía es el relleno, las comprobaciones que necesitan Go se omiten.

set -uo pipefail
# En Git Bash (Windows), MSYS convierte argumentos como "/healthz" en rutas de
# Windows ("C:/Program Files/Git/healthz") antes de pasarlos a docker. Sin esto,
# la detección de Go y las llamadas internas fallan. En Linux no tiene efecto.
export MSYS_NO_PATHCONV=1

ROOT="${ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
BASE_URL="${BASE_URL:-http://localhost}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-300}"
P5_PATH="${P5_PATH:-/libraries/p5.min.js}"
cd "$ROOT" || exit 1

PASS=0
FAIL=0
SKIP=0

ok()      { PASS=$((PASS + 1)); printf '  \e[32mOK\e[0m     %s\n' "$1"; }
fail()    { FAIL=$((FAIL + 1)); printf '  \e[31mFALLA\e[0m  %s (%s)\n' "$1" "$2"; }
skip()    { SKIP=$((SKIP + 1)); printf '  \e[33mOMITE\e[0m  %s (%s)\n' "$1" "$2"; }
section() { printf '\n== %s\n' "$1"; }

check() {
  local name=$1 expected=$2 actual=$3
  if [[ "$actual" =~ ^($expected)$ ]]; then
    ok "$name"
  else
    fail "$name" "esperado $expected, obtenido '$actual'"
  fi
}

http_code() { curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$@"; }
env_keys()  { grep -E '^[A-Z_]+=' "$1" | cut -d= -f1 | sort; }
field()     { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }

api() {
  local method=$1 path=$2 token=${3:-} body=${4:-}
  local args=(-s --max-time 10 -X "$method" -H 'Content-Type: application/json')
  [[ -n "$token" ]] && args+=(-H "Authorization: Bearer $token")
  [[ -n "$body" ]] && args+=(-d "$body")
  curl "${args[@]}" -w '\n%{http_code}' "$BASE_URL/api$path"
}
status_of() { tail -1 <<<"$1"; }
body_of()   { sed '$d' <<<"$1"; }

# Llama al REST interno de Go desde dentro de la red (no pasa por Traefik).
internal() {
  docker compose exec -T lobby-manager python -c "
import os, sys, urllib.request as u, urllib.error as e
req = u.Request('http://game-logic:8081' + sys.argv[2], method=sys.argv[1], headers={'X-Service-Token': os.environ['SERVICE_SECRET']})
try:
    print(u.urlopen(req, timeout=10).read().decode())
except e.HTTPError as err:
    print(err.code)
except Exception as err:
    print('sin-respuesta')
" "$1" "$2" 2>&1
}

section "Requisitos"
for cmd in docker curl openssl; do
  if command -v "$cmd" >/dev/null; then ok "$cmd instalado"; else fail "$cmd instalado" "no encontrado"; fi
done
if [[ ! -f .env ]]; then
  fail ".env existe" "copia .env.example a .env"
  exit 1
fi
ok ".env existe"
if diff <(env_keys .env) <(env_keys .env.example) >/dev/null; then
  ok ".env y .env.example tienen las mismas claves"
else
  fail ".env y .env.example tienen las mismas claves" "$(diff <(env_keys .env) <(env_keys .env.example) | grep '^[<>]' | tr '\n' ' ')"
fi

section "Arranque"
if docker compose up -d --build --wait --wait-timeout "$WAIT_TIMEOUT" >/dev/null 2>&1; then
  ok "docker compose up --wait"
else
  fail "docker compose up --wait" "ver docker compose ps y docker compose logs"
  docker compose ps
  exit 1
fi
for svc in $(docker compose ps --services); do
  health=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$(docker compose ps -q "$svc")")
  case "$health" in
    healthy) ok "$svc healthy" ;;
    none)    skip "$svc healthy" "sin healthcheck" ;;
    *)       fail "$svc healthy" "$health" ;;
  esac
done
published=$(docker compose ps --format '{{.Service}} {{.Ports}}' | grep -- '->' | cut -d' ' -f1 | sort -u | tr '\n' ' ')
check "solo traefik publica puertos" "traefik " "$published"

GO_REAL=false
[[ "$(internal GET /healthz)" == *'"status":"ok"'* ]] && GO_REAL=true
if $GO_REAL; then
  printf '  game-logic: servicio real\n'
else
  printf '  game-logic: relleno (se omiten las pruebas que necesitan Go)\n'
fi

section "Presentacion"
check "GET / redirige" "302" "$(http_code "$BASE_URL/")"
location=$(curl -sI --max-time 5 "$BASE_URL/" | tr -d '\r' | awk 'tolower($1)=="location:"{print $2}')
check "Location relativa a /menu/" "/menu/" "$location"
check "GET /menu/" "200" "$(http_code "$BASE_URL/menu/")"
check "GET /ingame/" "200" "$(http_code "$BASE_URL/ingame/")"
check "GET /chameleon/ (modo camaleon)" "200" "$(http_code "$BASE_URL/chameleon/")"
check "GET /chameleon/chameleon.js" "200" "$(http_code "$BASE_URL/chameleon/chameleon.js")"
check "GET /internal/games no enrutado" "404" "$(http_code "$BASE_URL/internal/games")"
encoding=$(curl -s -o /dev/null -D - --max-time 5 -H 'Accept-Encoding: gzip' "$BASE_URL$P5_PATH" | tr -d '\r' | awk 'tolower($1)=="content-encoding:"{print $2}')
check "p5.js servido con gzip" "gzip" "$encoding"

section "Lobby manager (REST y SSE)"
declare -a TOKENS
for name in Ana Beto Caro Dani; do
  res=$(api POST /session "" "{\"nickname\":\"$name\"}")
  TOKENS+=("$(body_of "$res" | field token)")
  check "POST /api/session $name" "201" "$(status_of "$res")"
done
check "GET /api/lobbies sin sesion" "401" "$(status_of "$(api GET /lobbies)")"
check "GET /api/lobbies con sesion" "200" "$(status_of "$(api GET /lobbies "${TOKENS[0]}")")"
res=$(api POST /lobbies "${TOKENS[0]}")
check "POST /api/lobbies crea la sala" "201" "$(status_of "$res")"
LOBBY_ID=$(body_of "$res" | grep -o '"id":"l_[^"]*"' | cut -d'"' -f4)
CODE=$(body_of "$res" | field code)
check "POST /start con 1 jugador" "409" "$(status_of "$(api POST "/lobbies/$LOBBY_ID/start" "${TOKENS[0]}")")"
for i in 1 2 3; do
  check "jugador $((i + 1)) entra con el codigo" "200" "$(status_of "$(api POST /lobbies/join "${TOKENS[$i]}" "{\"code\":\"$CODE\"}")")"
done
extra=$(body_of "$(api POST /session "" '{"nickname":"Quinto"}')" | field token)
check "quinto jugador rechazado (sala llena)" "409" "$(status_of "$(api POST /lobbies/join "$extra" "{\"code\":\"$CODE\"}")")"
headers=$(curl -s -N -o /dev/null -D - --max-time 2 -H 'Accept-Encoding: gzip' "$BASE_URL/api/lobbies/$LOBBY_ID/events?token=${TOKENS[0]}" | tr -d '\r')
check "SSE Content-Type" "text/event-stream.*" "$(awk 'tolower($1)=="content-type:"{print $2}' <<<"$headers")"
check "SSE sin compresion" "" "$(awk 'tolower($1)=="content-encoding:"{print $2}' <<<"$headers")"
events=$(curl -s -N --max-time 2 "$BASE_URL/api/lobbies/$LOBBY_ID/events?token=${TOKENS[0]}")
check "SSE entrega el estado de la sala" ".*event: lobby.*" "$(tr '\n' ' ' <<<"$events")"

section "API Gateway"
if $GO_REAL; then
  check "/ws/games/ llega a game-logic (sin token: 401)" "401" "$(http_code "$BASE_URL/ws/games/prueba")"
else
  check "/ws/games/ llega al relleno de game-logic" "200" "$(http_code "$BASE_URL/ws/games/prueba")"
fi
secret="e2e-$(openssl rand -hex 8)"
http_code "$BASE_URL/ws/games/prueba?token=$secret" >/dev/null
http_code "$BASE_URL/api/lobbies/$LOBBY_ID/events?token=$secret" >/dev/null
sleep 1
check "tokens ausentes en logs de traefik" "0" "$(docker compose logs traefik 2>&1 | grep -c "$secret")"
check "tokens ausentes en logs del lobby" "0" "$(docker compose logs lobby-manager 2>&1 | grep -c "$secret")"

section "Inicio de la partida (Lobby -> Game Logic)"
res=$(api POST "/lobbies/$LOBBY_ID/start" "${TOKENS[1]}")
if ! $GO_REAL; then
  check "POST /start sin Go responde 502" "502" "$(status_of "$res")"
  check "la sala sigue en espera" "WAITING" "$(body_of "$(api GET "/lobbies/$LOBBY_ID" "${TOKENS[0]}")" | field status)"
  for i in 0 1 2 3; do
    api POST "/lobbies/$LOBBY_ID/leave" "${TOKENS[$i]}" >/dev/null
  done
  check "la sala se borra al salir todos" "404" "$(status_of "$(api GET "/lobbies/$LOBBY_ID" "${TOKENS[0]}")")"
  skip "WebSocket de la partida" "game-logic es un relleno"
  skip "fin de la partida y resultado" "game-logic es un relleno"
else
  check "POST /start" "200" "$(status_of "$res")"
  GAME_ID=$(body_of "$res" | field gameId)
  GAME_TOKEN=$(body_of "$res" | grep -o '"game":{[^}]*}' | field token)
  check "la partida tiene gameId" "g_.+" "$GAME_ID"
  check "Game Logic conoce la partida" ".*\"status\":\"(WAITING|PLAYING)\".*" "$(internal GET "/internal/games/$GAME_ID")"

  section "Game logic (WebSocket)"
  ws() {
    curl -s -o /dev/null -w '%{http_code}' --max-time 2 --http1.1 \
      -H 'Connection: Upgrade' -H 'Upgrade: websocket' \
      -H 'Sec-WebSocket-Version: 13' -H "Sec-WebSocket-Key: $(openssl rand -base64 16)" \
      -H "Origin: $BASE_URL" "$BASE_URL/ws/games/$1?token=$2"
  }
  check "upgrade con token valido" "101" "$(ws "$GAME_ID" "$GAME_TOKEN")"
  check "token invalido rechazado" "401" "$(ws "$GAME_ID" "invalido")"
  check "token de sesion rechazado" "401" "$(ws "$GAME_ID" "${TOKENS[0]}")"
  sleep 1
  check "token de partida ausente en logs de traefik" "0" "$(docker compose logs traefik 2>&1 | grep -c "$GAME_TOKEN")"

  section "Fin de la partida (Game Logic -> Lobby)"
  sim=$(internal POST "/internal/games/$GAME_ID/simulate")
  if [[ "$sim" == "403" || "$sim" == "404" ]]; then
    skip "fin de la partida y resultado" "simulacion no disponible (DEMO_MODE)"
  else
    check "simulacion hasta el final" ".*\"status\":\"FINISHED\".*" "$sim"
    closed=""
    for _ in $(seq 1 20); do
      closed=$(body_of "$(api GET "/lobbies/$LOBBY_ID" "${TOKENS[0]}")" | field status)
      [[ "$closed" == "CLOSED" ]] && break
      sleep 0.5
    done
    check "la sala se cierra al registrar el resultado" "CLOSED" "$closed"
    check "resultado guardado una vez en PostgreSQL" "1" \
      "$(docker compose exec -T postgres sh -c "psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -Atc \"select count(*) from game_results where game_id = '$GAME_ID'\"" | tr -d '\r')"
    check "la partida sale de games:active" "0" "$(docker compose exec -T redis redis-cli sismember bololo:games:active "$GAME_ID" | tr -d '\r')"
    check "los jugadores pueden crear otra sala" "201" "$(status_of "$(api POST /lobbies "${TOKENS[2]}")")"
  fi
fi

section "Modo camaleon (2 a 4 jugadores)"
declare -a CH_TOKENS
for name in Eva Fede; do
  CH_TOKENS+=("$(body_of "$(api POST /session "" "{\"nickname\":\"$name\"}")" | field token)")
done
res=$(api POST /lobbies "${CH_TOKENS[0]}" '{"mode":"CHAMELEON"}')
check "POST /api/lobbies con mode CHAMELEON" "201" "$(status_of "$res")"
check "la sala informa su modo" ".*\"mode\":\"CHAMELEON\".*\"minPlayers\":2.*" "$(body_of "$res")"
CH_LOBBY=$(body_of "$res" | grep -o '"id":"l_[^"]*"' | cut -d'"' -f4)
CH_CODE=$(body_of "$res" | field code)
check "modo desconocido rechazado" "422" "$(status_of "$(api POST /lobbies "$extra" '{"mode":"POKER"}')")"
check "camaleon no inicia con 1 jugador" "409" "$(status_of "$(api POST "/lobbies/$CH_LOBBY/start" "${CH_TOKENS[0]}")")"
check "segundo jugador entra con el codigo" "200" "$(status_of "$(api POST /lobbies/join "${CH_TOKENS[1]}" "{\"code\":\"$CH_CODE\"}")")"
res=$(api POST "/lobbies/$CH_LOBBY/start" "${CH_TOKENS[0]}")
if ! $GO_REAL; then
  check "POST /start sin Go responde 502" "502" "$(status_of "$res")"
  skip "partida camaleon en Go" "game-logic es un relleno"
else
  check "POST /start con 2 jugadores" "200" "$(status_of "$res")"
  CH_GAME=$(body_of "$res" | field gameId)
  CH_GAME_TOKEN=$(body_of "$res" | grep -o '"game":{[^}]*}' | field token)
  check "el acceso a la partida trae el modo" ".*\"game\":\{[^}]*\"mode\":\"CHAMELEON\".*" "$(body_of "$res")"
  check "Game Logic crea la partida camaleon" ".*\"mode\":\"CHAMELEON\".*\"status\":\"WAITING\".*" "$(internal GET "/internal/games/$CH_GAME")"
  check "Redis guarda el modo de la partida" "CHAMELEON" \
    "$(docker compose exec -T redis redis-cli hget "bololo:game:$CH_GAME" mode | tr -d '\r')"
  check "WebSocket camaleon por el gateway" "101" "$(ws "$CH_GAME" "$CH_GAME_TOKEN")"
fi

section "Datos"
check "redis exige contrasena" "NOAUTH.*" "$(docker compose exec -T redis sh -c 'unset REDISCLI_AUTH; redis-cli ping' 2>&1 | tr -d '\r')"
check "redis AOF activo" "yes" "$(docker compose exec -T redis redis-cli config get appendonly | tail -1 | tr -d '\r')"
check "redis appendfsync" "everysec" "$(docker compose exec -T redis redis-cli config get appendfsync | tail -1 | tr -d '\r')"
check "postgres acepta la contrasena de .env" "1" \
  "$(docker compose exec -T postgres sh -c 'PGPASSWORD="$POSTGRES_PASSWORD" psql -h postgres -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "select 1"' 2>&1 | tr -d '\r')"
check "postgres rechaza otra contrasena" ".*password authentication failed.*" \
  "$(docker compose exec -T postgres sh -c 'PGPASSWORD=incorrecta psql -h postgres -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "select 1"' 2>&1 | tr -d '\r' | tail -1)"

section "Resumen"
printf '  %d OK, %d fallas, %d omitidas\n' "$PASS" "$FAIL" "$SKIP"

if [[ "${1:-}" == "--down" ]]; then
  docker compose down >/dev/null 2>&1 && printf '  stack detenido (volumenes conservados)\n'
fi

[[ $FAIL -eq 0 ]]
