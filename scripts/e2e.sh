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
BASE_URL="${BASE_URL:-https://localhost}"
# Mismo host por el puerto 80, que solo debe redirigir (escenario 1, Secure Channel).
HTTP_URL="${HTTP_URL:-http://${BASE_URL#*://}}"
CA_CERT="${CA_CERT:-api-gateway/ca/bololo-dev-ca.crt}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-300}"
P5_PATH="${P5_PATH:-/libraries/p5.min.js}"
cd "$ROOT" || exit 1

# Todas las llamadas validan el certificado contra la CA local, como el navegador
# (sin -k). --ssl-revoke-best-effort: la CA local no publica CRL y curl con
# Schannel (Windows) fallaría; en curl con OpenSSL no tiene efecto.
# En Git Bash, con MSYS_NO_PATHCONV=1 el curl nativo de Windows no puede abrir
# /dev/null: sale con código 23 y deja -w a medias (por ejemplo %{redirect_url}).
# Por eso /dev/null se traduce a "nul" allí.
TLS_OPTS=()
[[ "$BASE_URL" == https://* ]] && TLS_OPTS=(--cacert "$CA_CERT" --ssl-revoke-best-effort)
case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) NULL_DEV=nul ;; *) NULL_DEV=/dev/null ;; esac
curl() {
  local args=() a
  for a in "$@"; do [[ $a == /dev/null ]] && a=$NULL_DEV; args+=("$a"); done
  command curl ${TLS_OPTS[@]+"${TLS_OPTS[@]}"} "${args[@]}"
}
TLS_NAME="${BASE_URL#*://}"
TLS_NAME="${TLS_NAME%%[:/]*}"
TLS_PORT=443
[[ "${BASE_URL#*://}" =~ :([0-9]+) ]] && TLS_PORT="${BASH_REMATCH[1]}"
# Solo texto imprimible: el ticket de sesión trae bytes binarios que rompen las regex de bash.
tls_probe() { openssl s_client -connect "$TLS_NAME:$TLS_PORT" -servername "$TLS_NAME" "$@" </dev/null 2>&1 | tr -cd '[:print:]\n'; }
# Valor de una cabecera de respuesta.
header_of() { local name=$1; shift; curl -s -o /dev/null -D - --max-time 5 "$@" | tr -d '\r' | awk -v h="$name:" 'tolower($1)==h{print $2}'; }

# Un handshake que el servidor debe rechazar. Si es el cliente openssl el que no
# puede ofrecer esa versión o suite, la prueba se omite en lugar de pasar sola.
tls_rejected() {
  local name=$1 out
  shift
  out=$(tls_probe "$@")
  if grep -aqE 'alert (protocol version|handshake failure|insufficient security)' <<<"$out"; then
    ok "$name"
  elif grep -aqE 'Cipher is [A-Z0-9]|Protocol version: TLS' <<<"$out"; then
    fail "$name" "el servidor aceptó: $(grep -aoE 'Cipher is [^ ]+' <<<"$out" | head -1)"
  else
    skip "$name" "el cliente openssl no puede ofrecerlo"
  fi
}

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

auth_api() {
  local method=$1 path=$2 token=${3:-} body=${4:-}
  local args=(-s --max-time 10 -X "$method" -H 'Content-Type: application/json')
  [[ -n "$token" ]] && args+=(-H "Authorization: Bearer $token")
  [[ -n "$body" ]] && args+=(-d "$body")
  curl "${args[@]}" -w '\n%{http_code}' "$BASE_URL/auth$path"
}

# Sesión de invitado: la emite Auth (decisión 4).
guest() { auth_api POST /guest "" "{\"nickname\":\"$1\"}"; }

# Publica game.ended en RabbitMQ como lo haría Go (exchange bololo.events, con confirmación).
publish_game_ended() {
  docker compose exec -T lobby-manager python -c "
import asyncio, json, os, sys
import aio_pika

async def main():
    body = json.loads(sys.argv[2])
    body['lobbyId'] = sys.argv[1]
    connection = await aio_pika.connect(os.environ['RABBITMQ_URL'])
    async with connection:
        channel = await connection.channel(publisher_confirms=True, on_return_raises=True)
        exchange = await channel.get_exchange('bololo.events')
        message = aio_pika.Message(json.dumps(body).encode(), content_type='application/json',
                                   delivery_mode=aio_pika.DeliveryMode.PERSISTENT, message_id=body['gameId'])
        await exchange.publish(message, routing_key='game.ended', mandatory=True)
    print('publicado')

asyncio.run(main())
" "$1" "$2" 2>&1
}

results_of() {
  docker compose exec -T postgres sh -c "psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -Atc \"select count(*) from game_results where game_id = '$1'\"" | tr -d '\r'
}

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
if [[ "$BASE_URL" == https://* ]]; then
  if [[ -s "$CA_CERT" && -s api-gateway/certs/bololo.crt && -s api-gateway/certs/bololo.key ]]; then
    ok "certificado del proxy y CA local generados"
  else
    fail "certificado del proxy y CA local generados" "ejecuta ./scripts/gen-certs.sh"
    exit 1
  fi
fi
if ls secrets/jwt/{session,game}-{private,public}.pem >/dev/null 2>&1; then
  ok "claves de los JWT generadas"
else
  fail "claves de los JWT generadas" "ejecuta ./scripts/gen-jwt-keys.sh"
  exit 1
fi
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
check "solo reverse-proxy publica puertos" "reverse-proxy " "$published"

section "Canal seguro (TLS, escenario 1)"
if [[ "$BASE_URL" != https://* ]]; then
  skip "canal seguro" "BASE_URL no es https"
else
  check "reverse-proxy publica el 443" ".*:443" "$(docker compose port reverse-proxy 443 2>/dev/null)"
  check "GET por http:// redirige a https" "301|308" "$(http_code "$HTTP_URL/menu/")"
  check "la redirección conserva ruta y query" "$BASE_URL/api/lobbies\?x=1" "$(header_of location "$HTTP_URL/api/lobbies?x=1")"
  res=$(curl -s --max-time 5 -X POST -H 'Content-Type: application/json' -d '{"nickname":"EnClaro"}' -w '\n%{http_code}' "$HTTP_URL/auth/guest")
  check "POST por http:// no llega al servicio" "308" "$(status_of "$res")"
  check "POST por http:// no entrega token" "0" "$(body_of "$res" | grep -c token)"
  check "cabecera HSTS en https" "max-age=[1-9][0-9]*" "$(header_of strict-transport-security "$BASE_URL/menu/")"
  check "sin HSTS en http (el navegador la ignoraría)" "" "$(header_of strict-transport-security "$HTTP_URL/")"
  probe=$(tls_probe -CAfile "$CA_CERT" -verify_hostname "$TLS_NAME" -verify_return_error -alpn h2,http/1.1)
  check "certificado válido para $TLS_NAME (CA local)" "Verify return code: 0 \(ok\)" \
    "$(grep -aoE 'Verify return code: [0-9]+ \([^)]*\)' <<<"$probe" | head -1)"
  check "TLS 1.3 negociado" "New, TLSv1\.3" "$(grep -aoE 'New, TLSv[0-9.]+' <<<"$probe" | head -1)"
  check "HTTP/2 negociado por ALPN" "ALPN protocol: h2" "$(grep -aoE 'ALPN protocol: [^ ]+|No ALPN negotiated' <<<"$probe" | head -1)"
  check "TLS 1.2 con ECDHE + AES-GCM aceptado" "Cipher is ECDHE-ECDSA-AES128-GCM-SHA256" \
    "$(tls_probe -tls1_2 -cipher ECDHE-ECDSA-AES128-GCM-SHA256 | grep -aoE 'Cipher is [^ ]+' | head -1)"
  tls_rejected "TLS 1.0 rechazado" -tls1 -cipher 'DEFAULT:@SECLEVEL=0'
  tls_rejected "TLS 1.1 rechazado" -tls1_1 -cipher 'DEFAULT:@SECLEVEL=0'
  tls_rejected "suite sin ECDHE (AES128-SHA) rechazada" -tls1_2 -cipher 'AES128-SHA:@SECLEVEL=0'
  tls_rejected "suite CBC (ECDHE-ECDSA-AES128-SHA) rechazada" -tls1_2 -cipher 'ECDHE-ECDSA-AES128-SHA:@SECLEVEL=0'
  check "reverse-proxy solo monta el certificado, no la CA" "README.md bololo.crt bololo.key " \
    "$(docker compose exec -T reverse-proxy ls /certs 2>/dev/null | tr -d '\r' | LC_ALL=C sort | tr '\n' ' ')"
fi

section "Reverse Proxy (escenario 2)"
# El 404 propio del proxy lleva este cuerpo; si la petición llegara al gateway, respondería otro.
PROXY_404='\{"error":"not_found"\}'
mounts_of() { docker inspect -f '{{range .Mounts}}{{.Source}}->{{.Destination}} {{end}}' "$(docker compose ps -q "$1")"; }
networks_of() { docker inspect -f '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}' "$(docker compose ps -q "$1")"; }
for path in /internal/games /dashboard/ /apiloquesea /ws/otra /metrics /healthz; do
  res=$(curl -s --max-time 5 -w '\n%{http_code}' "$BASE_URL$path")
  check "ruta no publicada $path: 404 del proxy" "404 $PROXY_404" "$(status_of "$res") $(body_of "$res")"
done
res=$(curl -s --max-time 5 --path-as-is -w '\n%{http_code}' "$BASE_URL/api/../internal/games")
check "/api/../internal no escapa de la lista" "404 $PROXY_404" "$(status_of "$res") $(body_of "$res")"
res=$(curl -s --max-time 5 --path-as-is -w '\n%{http_code}' "$BASE_URL/api/%2e%2e/internal/games")
check "/api/%2e%2e/internal no escapa de la lista" "404 $PROXY_404" "$(status_of "$res") $(body_of "$res")"
check "/menu redirige a /menu/" "301 $BASE_URL/menu/" \
  "$(curl -s -o /dev/null --max-time 5 -w '%{http_code} %{redirect_url}' "$BASE_URL/menu")"
check "cuerpo de 100 KB rechazado en el borde" "413" \
  "$(head -c 102400 /dev/zero | tr '\0' a | curl -s -o /dev/null --max-time 5 -w '%{http_code}' \
      -H 'Content-Type: application/json' --data-binary @- "$BASE_URL/auth/login")"
check "Server sin version ni nombre del gateway" "nginx" "$(header_of server "$BASE_URL/menu/")"
check "Server de los servicios oculto (antes: uvicorn)" "nginx" "$(header_of server "$BASE_URL/api/no-existe")"
for page in /menu/ /api/no-existe /internal/games; do
  check "nosniff en $page" "nosniff" "$(header_of x-content-type-options "$BASE_URL$page")"
  check "X-Frame-Options DENY en $page" "DENY" "$(header_of x-frame-options "$BASE_URL$page")"
  check "Referrer-Policy no-referrer en $page" "no-referrer" "$(header_of referrer-policy "$BASE_URL$page")"
done
check "ninguna respuesta nombra a Traefik" "0" \
  "$(curl -s -i --max-time 5 "$BASE_URL/api/no-existe" "$BASE_URL/dashboard/" "$BASE_URL/menu/" | grep -ci traefik)"
check "el lobby redirige a https (X-Forwarded-Proto del proxy)" "$BASE_URL/api/lobbies" \
  "$(curl -s -o /dev/null --max-time 5 -w '%{redirect_url}' "$BASE_URL/api/lobbies/")"
check "traefik no publica puertos" "" "$(docker compose port traefik 8000 2>/dev/null)"
check "traefik no esta en la red public" "0" "$(networks_of traefik | grep -c public)"
check "el proxy llega al gateway" "200" \
  "$(docker compose exec -T reverse-proxy wget -q -T 3 -S -O /dev/null http://traefik:8000/menu/ 2>&1 | awk '/HTTP\//{print $2; exit}')"
check "el proxy no ve los servicios" ".*bad address.*" \
  "$(docker compose exec -T reverse-proxy wget -q -T 3 -O /dev/null http://lobby-manager:8000/healthz 2>&1 | tr -d '\r')"
check "el proxy no ve las bases de datos" ".*bad address.*" \
  "$(docker compose exec -T reverse-proxy wget -q -T 3 -O /dev/null http://postgres:5432/ 2>&1 | tr -d '\r')"
check "el proxy no monta el socket de Docker" "0" "$(mounts_of reverse-proxy | grep -c docker.sock)"
check "el proxy corre con disco de solo lectura" "true" \
  "$(docker inspect -f '{{.HostConfig.ReadonlyRootfs}}' "$(docker compose ps -q reverse-proxy)")"
check "el proxy no tiene las claves de los JWT" "0" "$(mounts_of reverse-proxy | grep -c secrets)"

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
menu_html=$(curl -s --max-time 10 "$BASE_URL/menu/" | tr '\n' ' ')
check "el menu llega con datos renderizados en el servidor (SSR)" '.*id="ssr-data".*"leaderboard":\{"mode":"CLASSIC".*' "$menu_html"
check "el menu incluye la tabla del ranking" '.*<h3>Ranking · Clásico</h3>.*|.*Aún no hay rankings disponibles.*' "$menu_html"
check "GET /ingame/" "200" "$(http_code "$BASE_URL/ingame/")"
check "GET /chameleon/ (modo camaleon)" "200" "$(http_code "$BASE_URL/chameleon/")"
check "GET /chameleon/chameleon.js" "200" "$(http_code "$BASE_URL/chameleon/chameleon.js")"
check "GET /internal/games no enrutado" "404" "$(http_code "$BASE_URL/internal/games")"
encoding=$(curl -s -o /dev/null -D - --max-time 5 -H 'Accept-Encoding: gzip' "$BASE_URL$P5_PATH" | tr -d '\r' | awk 'tolower($1)=="content-encoding:"{print $2}')
check "p5.js servido con gzip" "gzip" "$encoding"

section "Lobby manager (REST y SSE)"
declare -a TOKENS
for name in Ana Beto Caro Dani; do
  res=$(guest "$name")
  TOKENS+=("$(body_of "$res" | field token)")
  check "POST /auth/guest $name" "201" "$(status_of "$res")"
done
check "el Lobby ya no emite sesiones" "405" "$(status_of "$(api POST /session "" '{"nickname":"Ana"}')")"
check "GET /api/session crea al jugador del invitado" ".*\"nickname\":\"Ana\".*" "$(body_of "$(api GET /session "${TOKENS[0]}")")"
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
extra=$(body_of "$(guest Quinto)" | field token)
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
http_code "$HTTP_URL/ws/games/prueba?token=$secret" >/dev/null
http_code "$BASE_URL/api/lobbies/$LOBBY_ID/events?token=$secret" >/dev/null
sleep 1
check "tokens ausentes en logs de reverse-proxy" "0" "$(docker compose logs reverse-proxy 2>&1 | grep -c "$secret")"
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
      -H "Origin: ${3:-$BASE_URL}" "$BASE_URL/ws/games/$1?token=$2"
  }
  if [[ "$BASE_URL" == https://* ]]; then
    check "upgrade desde una pagina http:// rechazado" "403" "$(ws "$GAME_ID" "$GAME_TOKEN" "$HTTP_URL")"
  fi
  check "upgrade con token valido" "101" "$(ws "$GAME_ID" "$GAME_TOKEN")"
  check "token invalido rechazado" "401" "$(ws "$GAME_ID" "invalido")"
  check "token de sesion rechazado" "401" "$(ws "$GAME_ID" "${TOKENS[0]}")"
  sleep 1
  check "token de partida ausente en logs de reverse-proxy" "0" "$(docker compose logs reverse-proxy 2>&1 | grep -c "$GAME_TOKEN")"
  check "token de partida ausente en logs de traefik" "0" "$(docker compose logs traefik 2>&1 | grep -c "$GAME_TOKEN")"

  section "Fin de la partida (Game Logic -> RabbitMQ -> Lobby)"
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
  CH_TOKENS+=("$(body_of "$(guest "$name")" | field token)")
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

section "Auth (Java, Spring Security y su propia base)"
SUFFIX=$(openssl rand -hex 4)
USER_A="e2e_a_$SUFFIX"
USER_B="e2e_b_$SUFFIX"
USER_L="e2e_l_$SUFFIX"
PASSWORD="e2e-pass-$SUFFIX"
res=$(auth_api POST /register "" "{\"username\":\"$USER_A\",\"password\":\"$PASSWORD\"}")
check "POST /auth/register" "201" "$(status_of "$res")"
TOKEN_A=$(body_of "$res" | field token)
ACCOUNT_A=$(body_of "$res" | grep -o '"account":{[^}]*}' | field id)
res=$(auth_api POST /register "" "{\"username\":\"$USER_B\",\"password\":\"$PASSWORD\"}")
TOKEN_B=$(body_of "$res" | field token)
ACCOUNT_B=$(body_of "$res" | grep -o '"account":{[^}]*}' | field id)
check "usuario repetido rechazado" "409" "$(status_of "$(auth_api POST /register "" "{\"username\":\"$USER_A\",\"password\":\"$PASSWORD\"}")")"
check "POST /auth/login" "200" "$(status_of "$(auth_api POST /login "" "{\"username\":\"$USER_A\",\"password\":\"$PASSWORD\"}")")"
check "login con contrasena incorrecta" "401" "$(status_of "$(auth_api POST /login "" "{\"username\":\"$USER_A\",\"password\":\"incorrecta-123\"}")")"
res=$(auth_api GET /me "$TOKEN_A")
check "GET /auth/me con sesion de cuenta" "200" "$(status_of "$res")"
check "/auth/me identifica la cuenta" "$USER_A" "$(body_of "$res" | field username)"
check "GET /auth/me sin token" "401" "$(status_of "$(auth_api GET /me)")"
check "GET /auth/me con sesion de invitado" "403" "$(status_of "$(auth_api GET /me "${TOKENS[0]}")")"
check "la sesion de cuenta sirve en el lobby" "200" "$(status_of "$(api GET /lobbies "$TOKEN_A")")"
auth_api POST /register "" "{\"username\":\"$USER_L\",\"password\":\"$PASSWORD\"}" >/dev/null
for _ in 1 2 3 4 5; do
  auth_api POST /login "" "{\"username\":\"$USER_L\",\"password\":\"incorrecta-123\"}" >/dev/null
done
check "bloqueo tras 5 intentos fallidos" "429" "$(status_of "$(auth_api POST /login "" "{\"username\":\"$USER_L\",\"password\":\"$PASSWORD\"}")")"
check "contrasena guardada con BCrypt" '\$2[aby]\$12\$.*' \
  "$(docker compose exec -T auth-postgres sh -c "psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -Atc \"select password_hash from accounts where username = '$USER_A'\"" | tr -d '\r')"

section "Resultado y ratings (Game Logic -> RabbitMQ -> Lobby -> RabbitMQ -> Ratings)"
check "GET /api/ratings/leaderboard" "200" "$(status_of "$(api GET '/ratings/leaderboard?mode=CLASSIC')")"
check "modo desconocido en el ranking" "422" "$(status_of "$(api GET '/ratings/leaderboard?mode=POKER')")"
check "ruta desconocida en ratings" "404" "$(http_code "$BASE_URL/api/ratings/internal")"
if ! $GO_REAL; then
  skip "ranking de punta a punta" "game-logic es un relleno"
else
  res=$(api POST /lobbies "$TOKEN_A" '{"mode":"CHAMELEON"}')
  R_LOBBY=$(body_of "$res" | grep -o '"id":"l_[^"]*"' | cut -d'"' -f4)
  R_CODE=$(body_of "$res" | field code)
  api POST /lobbies/join "$TOKEN_B" "{\"code\":\"$R_CODE\"}" >/dev/null
  res=$(api POST "/lobbies/$R_LOBBY/start" "$TOKEN_A")
  check "partida entre dos cuentas iniciada" "200" "$(status_of "$res")"
  R_GAME=$(body_of "$res" | field gameId)
  R_PLAYER_A=$(body_of "$(api GET /session "$TOKEN_A")" | grep -o '"id":"p_[^"]*"' | head -1 | cut -d'"' -f4)
  R_PLAYER_B=$(body_of "$(api GET /session "$TOKEN_B")" | grep -o '"id":"p_[^"]*"' | head -1 | cut -d'"' -f4)
  R_BODY="{\"gameId\":\"$R_GAME\",\"mode\":\"CHAMELEON\",\"winnerId\":\"$R_PLAYER_A\",\"winners\":[\"$R_PLAYER_A\"],\"endReason\":\"all_colors\",\"standings\":[{\"player\":\"$R_PLAYER_A\",\"colors\":8,\"rank\":1},{\"player\":\"$R_PLAYER_B\",\"colors\":5,\"rank\":2}]}"
  check "Go publica game.ended en RabbitMQ" "publicado" "$(publish_game_ended "$R_LOBBY" "$R_BODY")"
  recorded=""
  for _ in $(seq 1 20); do
    recorded=$(results_of "$R_GAME")
    [[ "$recorded" == "1" ]] && break
    sleep 0.5
  done
  check "el Lobby consume game.ended y registra el resultado" "1" "$recorded"
  check "la sala se cierra" "CLOSED" "$(body_of "$(api GET "/lobbies/$R_LOBBY" "$TOKEN_A")" | field status)"
  rating=""
  for _ in $(seq 1 30); do
    rating=$(body_of "$(api GET "/ratings/players/$ACCOUNT_A")" | grep -o '"rating":[0-9]*' | cut -d: -f2)
    [[ -n "$rating" ]] && break
    sleep 0.5
  done
  check "el ganador sube a 1016" "1016" "$rating"
  check "el perdedor baja a 984" ".*\"rating\":984.*" "$(body_of "$(api GET "/ratings/players/$ACCOUNT_B")")"
  publish_game_ended "$R_LOBBY" "$R_BODY" >/dev/null
  sleep 3
  check "un game.ended repetido no registra otra vez" "1" "$(results_of "$R_GAME")"
  check "el reintento no suma dos veces" ".*\"games\":1.*" "$(body_of "$(api GET "/ratings/players/$ACCOUNT_A")")"
  check "el historial guarda el cambio" ".*\"ratingBefore\":1000,\"ratingAfter\":1016.*" "$(body_of "$(api GET "/ratings/players/$ACCOUNT_A/history")")"
fi
queues=$(docker compose exec -T rabbitmq rabbitmqctl -q list_queues name durable messages 2>&1 | tr -d '\r' | tr '\t' ' ')
check "cola de ratings durable y vacia" ".*ratings\.game-finished true 0.*" "$(tr '\n' ';' <<<"$queues")"
check "sin mensajes muertos" ".*ratings\.game-finished\.dead true 0.*" "$(tr '\n' ';' <<<"$queues")"
check "cola de resultados del lobby durable y vacia" ".*lobby\.game-ended true 0.*" "$(tr '\n' ';' <<<"$queues")"
check "sin resultados muertos en el lobby" ".*lobby\.game-ended\.dead true 0.*" "$(tr '\n' ';' <<<"$queues")"
check "Go no conoce la URL del Lobby" "0" "$(docker compose config game-logic 2>/dev/null | grep -c LOBBY_CALLBACK_URL)"

section "Claves de los JWT y topologia del broker (decisiones 5 y 6)"
# Se leen los montajes del contenedor: la imagen de game-logic es distroless y no trae ls.
secrets_of() {
  docker inspect -f '{{range .Mounts}}{{.Destination}}{{"\n"}}{{end}}' "$(docker compose ps -q "$1")" 2>/dev/null |
    sed -n 's#^/run/secrets/##p' | LC_ALL=C sort | tr '\n' ' '
}
check "auth solo recibe las claves de sesion" "session_private_key session_public_key " \
  "$(secrets_of auth)"
check "lobby-manager no recibe la clave privada de sesiones" "game_private_key session_public_key " \
  "$(secrets_of lobby-manager)"
check "game-logic solo recibe la clave publica de partidas" "game_public_key " \
  "$(secrets_of game-logic)"
queues=$(docker compose exec -T rabbitmq rabbitmqctl -q list_queues name 2>/dev/null | tr -d '\r' | LC_ALL=C sort | tr '\n' ' ')
check "el broker declara las colas de sus consumidores" \
  "lobby.game-ended lobby.game-ended.dead name ratings.game-finished ratings.game-finished.dead |lobby.game-ended lobby.game-ended.dead ratings.game-finished ratings.game-finished.dead " \
  "$queues"

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
