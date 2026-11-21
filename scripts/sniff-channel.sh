#!/usr/bin/env bash
# Escenario 1 (Secure Channel): simula a un atacante que escucha la red entre el
# navegador y Bololó (por ejemplo, alguien conectado a la misma Wi-Fi).
#
# Captura con tcpdump el tráfico de los puertos 80 y 443 del contenedor reverse-proxy,
# es decir, el tramo navegador <-> borde. Mientras captura, hace el recorrido de
# un jugador: primera visita escrita como http://, registro con contraseña, sesión
# de invitado, sala de espera por SSE (?token=) y WebSocket de la partida (?token=).
# Al final cuenta cuántas veces aparecen en claro la contraseña y los tokens.
#
# Uso:  ./scripts/sniff-channel.sh [BASE_URL] [carpeta-de-salida]
#   BASE_URL     https://localhost por defecto. Para el "antes" (solo HTTP): http://localhost
#   carpeta      opcional. Guarda captura.pcap (para abrir en Wireshark) y captura.txt.
#                La captura del "antes" contiene la contraseña y JWT completos: no la subas a Git.
#
# Requiere docker, curl y openssl, y el stack arriba (docker compose up -d --wait).
# En Git Bash (Windows) funciona igual; MSYS_NO_PATHCONV evita que convierta rutas.
#
# Cada ejecución deja una sala de prueba en IN_GAME (la partida espera al segundo
# jugador) hasta que el Lobby la cierra sola a las 3 h (GAME_TTL_SECONDS). El Lobby
# admite 10 salas activas: no lo ejecutes en bucle antes de la demo.

set -uo pipefail
export MSYS_NO_PATHCONV=1

ROOT="${ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$ROOT" || exit 1

BASE_URL="${1:-${BASE_URL:-https://localhost}}"
OUT_DIR="${2:-}"
CA_CERT="${CA_CERT:-api-gateway/ca/bololo-dev-ca.crt}"
HOST="${BASE_URL#*://}"
SNIFFER=bololo-sniffer
SNIFFER_IMAGE=bololo-sniffer:tcpdump

# Git Bash: con MSYS_NO_PATHCONV=1 el curl nativo no puede abrir /dev/null (sale con 23).
case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) NULL_DEV=nul ;; *) NULL_DEV=/dev/null ;; esac
curl() {
  local args=() a
  for a in "$@"; do [[ $a == /dev/null ]] && a=$NULL_DEV; args+=("$a"); done
  command curl "${args[@]}"
}

CURL=(curl -s --max-time 5)
if [[ "$BASE_URL" == https://* ]]; then
  if [[ ! -f "$CA_CERT" ]]; then
    echo "Falta $CA_CERT. Ejecuta ./scripts/gen-certs.sh" >&2
    exit 1
  fi
  # --ssl-revoke-best-effort: la CA local no publica CRL; sin esto curl con Schannel (Windows) falla.
  CURL+=(--cacert "$CA_CERT" --ssl-revoke-best-effort)
fi

field() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }

post() {
  local path=$1 body=${2:-} token=${3:-}
  local args=(-X POST -H 'Content-Type: application/json')
  [[ -n "$token" ]] && args+=(-H "Authorization: Bearer $token")
  [[ -n "$body" ]] && args+=(-d "$body")
  "${CURL[@]}" "${args[@]}" "$BASE_URL$path"
}

cleanup() { docker rm -f "$SNIFFER" >/dev/null 2>&1; }
trap cleanup EXIT

proxy_id=$(docker compose ps -q reverse-proxy)
if [[ -z "$proxy_id" ]]; then
  echo "reverse-proxy no está corriendo: docker compose up -d --wait" >&2
  exit 1
fi

# Imagen mínima con tcpdump. Se construye una vez y queda en caché.
if ! docker image inspect "$SNIFFER_IMAGE" >/dev/null 2>&1; then
  printf 'FROM alpine:3.20.3\nRUN apk add --no-cache tcpdump\n' | docker build -q -t "$SNIFFER_IMAGE" - >/dev/null || exit 1
fi

# El sniffer comparte la interfaz de red de reverse-proxy: ve exactamente lo que ve
# alguien en el camino entre el navegador y el borde. Solo puertos 80 y 443,
# así que el tramo interno reverse-proxy -> traefik (puerto 8000) no entra.
cleanup
docker run -d --name "$SNIFFER" --network "container:$proxy_id" --cap-add NET_RAW --cap-add NET_ADMIN \
  "$SNIFFER_IMAGE" sh -c "tcpdump -i any -nn -U -s0 -w /tmp/captura.pcap 'tcp port 80 or tcp port 443' & sleep 600" >/dev/null || exit 1
for _ in $(seq 1 20); do
  docker logs "$SNIFFER" 2>&1 | grep -q 'listening on' && break
  sleep 0.5
done

SUFFIX=$(openssl rand -hex 4)
USERNAME="sniff_$SUFFIX"
PASSWORD="Secreto-$SUFFIX-$(openssl rand -hex 4)"

# 1. Primera visita escrita a mano con http:// (lo que haría un jugador nuevo).
first=$("${CURL[@]}" -o /dev/null -w '%{http_code} %{redirect_url}' "http://$HOST/")

# 2. Registro: usuario y contraseña viajan en el cuerpo del POST.
ACCOUNT_TOKEN=$(post /auth/register "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}" | field token)

# 3. Sesión de invitado.
GUEST_TOKEN=$(post /auth/guest '{"nickname":"Espiado"}' | field token)

# 4. Sala de espera: EventSource no admite cabeceras, el token va en la URL.
lobby=$(post /api/lobbies '{"mode":"CHAMELEON"}' "$ACCOUNT_TOKEN")
LOBBY_ID=$(grep -o '"id":"l_[^"]*"' <<<"$lobby" | cut -d'"' -f4)
CODE=$(field code <<<"$lobby")
"${CURL[@]}" -N --max-time 2 "$BASE_URL/api/lobbies/$LOBBY_ID/events?token=$ACCOUNT_TOKEN" >/dev/null

# 5. Partida: el invitado entra, se inicia y se abre el WebSocket con el token en la URL.
post /api/lobbies/join "{\"code\":\"$CODE\"}" "$GUEST_TOKEN" >/dev/null
start=$(post "/api/lobbies/$LOBBY_ID/start" "" "$ACCOUNT_TOKEN")
GAME_ID=$(field gameId <<<"$start")
GAME_TOKEN=$(grep -o '"game":{[^}]*}' <<<"$start" | field token)
ws_code=$("${CURL[@]}" -o /dev/null -w '%{http_code}' --max-time 2 --http1.1 \
  -H 'Connection: Upgrade' -H 'Upgrade: websocket' -H 'Sec-WebSocket-Version: 13' \
  -H "Sec-WebSocket-Key: $(openssl rand -base64 16)" -H "Origin: $BASE_URL" \
  "$BASE_URL/ws/games/$GAME_ID?token=$GAME_TOKEN")

sleep 1
docker exec "$SNIFFER" pkill -INT tcpdump >/dev/null 2>&1
sleep 1
# -q: sin el resumen "HTTP: GET ..." de tcpdump, para contar solo lo que va en los paquetes.
text=$(docker exec "$SNIFFER" tcpdump -nn -q -A -r /tmp/captura.pcap 2>/dev/null)
packets_80=$(docker exec "$SNIFFER" tcpdump -nn -r /tmp/captura.pcap 'tcp port 80' 2>/dev/null | wc -l | tr -d ' ')
packets_443=$(docker exec "$SNIFFER" tcpdump -nn -r /tmp/captura.pcap 'tcp port 443' 2>/dev/null | wc -l | tr -d ' ')

if [[ -n "$OUT_DIR" ]]; then
  mkdir -p "$OUT_DIR"
  docker cp "$SNIFFER:/tmp/captura.pcap" "$OUT_DIR/captura.pcap" >/dev/null
  printf '%s\n' "$text" >"$OUT_DIR/captura.txt"
fi

# Veces que un secreto aparece legible en la captura ("sin dato" si el paso falló).
seen() {
  if [[ -z "$1" ]]; then echo "sin dato"; return; fi
  grep -aoF -- "$1" <<<"$text" | wc -l | tr -d ' '
}
requests=$(grep -aoE '(GET|POST|PUT|PATCH|DELETE) /[^ ]* HTTP/1\.1' <<<"$text" | wc -l | tr -d ' ')

# Fila "etiqueta  valor" alineada por caracteres (printf %-Ns cuenta bytes y descuadra la ñ).
row() {
  local label=$1 pad
  pad=$((36 - $(printf '%s' "$label" | wc -m)))
  ((pad < 1)) && pad=1
  printf '  %s%*s%s\n' "$label" "$pad" "" "$2"
}

printf '\nCaptura del tramo navegador <-> borde (reverse-proxy, puertos 80 y 443)\n'
row "BASE_URL" "$BASE_URL"
row "primera visita a http://$HOST/" "$first"
row "upgrade del WebSocket" "$ws_code"
row "paquetes en :80 (HTTP)" "$packets_80"
row "paquetes en :443 (TLS)" "$packets_443"
row "peticiones HTTP legibles" "$requests"
printf '\n'
row "Secreto" "Veces legible en la captura"
row "contraseña de la cuenta" "$(seen "$PASSWORD")"
row "token de sesión de la cuenta" "$(seen "$ACCOUNT_TOKEN")"
row "token de sesión de invitado" "$(seen "$GUEST_TOKEN")"
row "token de partida (WebSocket)" "$(seen "$GAME_TOKEN")"

printf '\n  Peticiones legibles en la captura (JWT recortados):\n'
grep -aoE '(GET|POST|PUT|PATCH|DELETE) /[^ ]* HTTP/1\.1' <<<"$text" \
  | sed -E 's/(eyJ[A-Za-z0-9_-]{10})[A-Za-z0-9._-]*/\1...[recortado]/g' | sort | uniq -c | sed 's/^/   /'
leak=$(grep -aoE '"password":"[^"]*"' <<<"$text" | head -1)
[[ -n "$leak" ]] && printf '\n  Cuerpo del registro legible: {"username":"%s",%s}\n' "$USERNAME" "$leak"

leaked=0
for secret in "$PASSWORD" "$ACCOUNT_TOKEN" "$GUEST_TOKEN" "$GAME_TOKEN"; do
  [[ -n "$secret" && "$(seen "$secret")" != "0" ]] && leaked=1
done
if [[ $leaked -eq 0 ]]; then
  printf '\n  RESULTADO: ningún secreto viaja en claro.\n'
else
  printf '\n  RESULTADO: hay secretos legibles para cualquiera en la red.\n'
fi
[[ $leaked -eq 0 ]]
