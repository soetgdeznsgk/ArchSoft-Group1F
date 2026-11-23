#!/usr/bin/env bash
# Regenera la evidencia del escenario 1 (Secure Channel) en docs/p3/evidencia/esc1/<fase>/.
# Cada archivo muestra el comando exacto ($ ...) seguido de su salida.
#
# Uso: ./scripts/esc1-evidence.sh antes|despues [latencia]
#   antes    stack levantado desde main (solo HTTP en :80)
#   despues  stack levantado desde feature/p3-esc1 (HTTPS en :443; :80 solo redirige)
#   latencia regenera solo el archivo de latencia de esa fase
#
# Requiere docker, curl y openssl (Git Bash en Windows) y el stack arriba.
# Cada ejecución deja 2 salas de prueba en IN_GAME hasta 3 h (ver sniff-channel.sh).

set -uo pipefail
export MSYS_NO_PATHCONV=1
ROOT="${ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$ROOT" || exit 1

PHASE="${1:-}"
ONLY="${2:-}"
if [[ "$PHASE" != "antes" && "$PHASE" != "despues" ]] || [[ -n "$ONLY" && "$ONLY" != "latencia" ]]; then
  echo "uso: $0 antes|despues [latencia]" >&2
  exit 2
fi
D="docs/p3/evidencia/esc1/$PHASE"
mkdir -p "$D"
CA=api-gateway/ca/bololo-dev-ca.crt
TLS="--cacert $CA --ssl-revoke-best-effort"
ORIGIN_HEADERS="-H 'Connection: Upgrade' -H 'Upgrade: websocket' -H 'Sec-WebSocket-Version: 13' -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ=='"

# Git Bash: con MSYS_NO_PATHCONV=1 el curl nativo no puede abrir /dev/null (sale con 23
# y deja -w a medias). Se exporta para los "bash -c" de run().
case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) NULL_DEV=nul ;; *) NULL_DEV=/dev/null ;; esac
export NULL_DEV
curl() {
  local args=() a
  for a in "$@"; do [[ $a == /dev/null ]] && a=$NULL_DEV; args+=("$a"); done
  command curl "${args[@]}"
}
export -f curl

# Imprime el comando y su salida. stdin cerrado para que docker no se coma el script.
run() {
  printf '$ %s\n' "$1"
  bash -c "$1" 2>&1 </dev/null
  printf '\n'
}

# Latencia con dos clientes:
#   A. curl + OpenSSL en un contenedor de la red public, directo al proxy inverso. Aísla el costo
#      del borde (proxy inverso y gateway) y negocia HTTP/2 como un navegador.
#   B. curl del host por el puerto publicado. En Windows es curl con Schannel, que suma
#      su propio costo de cliente (verificación con --cacert, tickets de TLS 1.3).
latency() {
  local base=$1 out=$2 dir
  dir=$(pwd -W 2>/dev/null || pwd)
  bash scripts/bench-image.sh
  {
    echo "# A. Cliente Linux (curl + OpenSSL) en la red public, directo al proxy inverso"
    docker run --rm --network bololo-integrado_public -v "$dir:/w" -w /w \
      -e EXTRA_CURL_OPTS="--connect-to localhost:443:reverse-proxy:443 --connect-to localhost:80:reverse-proxy:80" \
      bololo-bench:curl bash scripts/latency.sh "$base" 200 </dev/null
    echo
    echo "# B. Cliente del host por el puerto publicado"
    ./scripts/latency.sh "$base" 200 </dev/null
  } >"$out" 2>&1
}

if [[ "$ONLY" == "latencia" ]]; then
  if [[ "$PHASE" == "antes" ]]; then latency http://localhost "$D/06-latencia.txt"; else latency https://localhost "$D/07-latencia.txt"; fi
  cat "$D"/0[67]-latencia.txt
  exit 0
fi

# Partida de 2 jugadores (modo camaleón) para probar el WebSocket con un token válido.
new_game() {
  local base=$1 tls=$2 a b lobby code start
  a=$(curl -s $tls -X POST -H 'Content-Type: application/json' -d '{"nickname":"EvidA"}' "$base/auth/guest" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
  b=$(curl -s $tls -X POST -H 'Content-Type: application/json' -d '{"nickname":"EvidB"}' "$base/auth/guest" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
  lobby=$(curl -s $tls -X POST -H 'Content-Type: application/json' -H "Authorization: Bearer $a" -d '{"mode":"CHAMELEON"}' "$base/api/lobbies")
  code=$(grep -o '"code":"[^"]*"' <<<"$lobby" | cut -d'"' -f4)
  curl -s $tls -X POST -H 'Content-Type: application/json' -H "Authorization: Bearer $b" -d "{\"code\":\"$code\"}" "$base/api/lobbies/join" >/dev/null
  start=$(curl -s $tls -X POST -H "Authorization: Bearer $a" "$base/api/lobbies/$(grep -o '"id":"l_[^"]*"' <<<"$lobby" | cut -d'"' -f4)/start")
  GAME_ID=$(grep -o '"gameId":"[^"]*"' <<<"$start" | head -1 | cut -d'"' -f4)
  GAME_TOKEN=$(grep -o '"game":{[^}]*}' <<<"$start" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
}

if [[ "$PHASE" == "antes" ]]; then
  ./scripts/sniff-channel.sh http://localhost >"$D/01-sniff.txt" 2>&1

  {
    run "curl -sI http://localhost/"
    run "curl -sI http://localhost/menu/ | tr -d '\r' | grep -iE '^(HTTP|content-type|strict-transport-security)'"
    run "curl -s -X POST -H 'Content-Type: application/json' -d '{\"nickname\":\"Evidencia\"}' -w '\n[HTTP %{http_code} sobre %{scheme}]\n' http://localhost/auth/guest | sed -E 's/(eyJ[A-Za-z0-9_-]{10})[A-Za-z0-9._-]*/\1...[recortado]/g'"
  } >"$D/02-http.txt"

  run "openssl s_client -connect localhost:443 -servername localhost -brief </dev/null; echo \"[exit \$?]\"" >"$D/03-tls.txt"

  new_game http://localhost ""
  {
    for origin in http://localhost https://evil.example; do
      printf 'ws:// con Origin %-22s -> ' "$origin"
      bash -c "curl -s -o /dev/null -w 'HTTP %{http_code}\n' --max-time 2 --http1.1 $ORIGIN_HEADERS -H 'Origin: $origin' 'http://localhost/ws/games/$GAME_ID?token=$GAME_TOKEN'" </dev/null
    done
  } >"$D/04-websocket.txt" 2>&1

  run "docker compose ps --format '{{.Service}}\t{{.Ports}}' | sort" >"$D/05-puertos.txt"
  latency http://localhost "$D/06-latencia.txt"
else
  ./scripts/sniff-channel.sh https://localhost >"$D/01-sniff.txt" 2>&1

  {
    run "curl -sI http://localhost/"
    run "curl -sI http://localhost/ws/games/g_x?token=abc | tr -d '\r' | grep -iE '^(HTTP|location)'"
    run "curl -s -X POST -H 'Content-Type: application/json' -d '{\"nickname\":\"Evidencia\"}' -w '\n[HTTP %{http_code}, Location: %{redirect_url}]\n' http://localhost/auth/guest"
    run "curl -sI $TLS https://localhost/menu/ | tr -d '\r' | grep -iE '^(HTTP|content-type|strict-transport-security)'"
    run "curl -s $TLS -o /dev/null -w 'HTTP/%{http_version}, ssl_verify_result=%{ssl_verify_result} (0 = certificado valido)\n' https://localhost/menu/"
  } >"$D/02-redireccion-hsts.txt"

  {
    printf '# Negociación normal: versión, suite, intercambio de claves, ALPN y verificación del certificado.\n'
    run "openssl s_client -connect localhost:443 -servername localhost -CAfile $CA -verify_hostname localhost -alpn h2,http/1.1 </dev/null 2>&1 | grep -aE '^(subject|issuer)=|Peer signature type|Server Temp Key|New, TLS|ALPN protocol|Verify return code' | sort -u"
    printf '# Sin SNI, por IP (como un jugador de la LAN que entra con https://<IP>):\n'
    run "openssl s_client -connect 127.0.0.1:443 -CAfile $CA -verify_ip 127.0.0.1 </dev/null 2>&1 | grep -aE 'Verify return code' | sort -u"
    printf '# TLS 1.2 con una suite permitida:\n'
    run "openssl s_client -connect localhost:443 -servername localhost -tls1_2 -cipher ECDHE-ECDSA-AES128-GCM-SHA256 </dev/null 2>&1 | grep -aoE 'New, TLSv[0-9.]+, Cipher is [^ ]+'"
    printf '# Negociaciones débiles. SECLEVEL=0 hace que el cliente sí las ofrezca: el rechazo lo envía el servidor (alerta TLS).\n'
    for p in tls1 tls1_1; do
      run "openssl s_client -connect localhost:443 -servername localhost -$p -cipher 'DEFAULT:@SECLEVEL=0' </dev/null 2>&1 | grep -aoE 'alert [a-z ]+|Cipher is [^ ]+' | head -1"
    done
    for c in AES128-SHA ECDHE-ECDSA-AES128-SHA ECDHE-ECDSA-AES256-SHA384; do
      run "openssl s_client -connect localhost:443 -servername localhost -tls1_2 -cipher '$c:@SECLEVEL=0' </dev/null 2>&1 | grep -aoE 'alert [a-z ]+|Cipher is [^ ]+' | head -1"
    done
  } >"$D/03-tls.txt"

  {
    run "openssl x509 -in api-gateway/certs/bololo.crt -noout -subject -issuer -dates -ext subjectAltName,keyUsage,extendedKeyUsage,basicConstraints"
    run "openssl x509 -in api-gateway/certs/bololo.crt -noout -text | grep -E 'Public Key Algorithm|Public-Key|ASN1 OID|Signature Algorithm' | sort -u"
    run "openssl x509 -in $CA -noout -subject -dates -ext basicConstraints,keyUsage,nameConstraints"
    run "openssl verify -CAfile $CA api-gateway/certs/bololo.crt"
    # Si alguien roba la clave de la CA local, no le sirve para suplantar sitios reales.
    # Temporal relativo y dentro de api-gateway/ca (ignorada por Git): el openssl nativo
    # de Git Bash no entiende rutas /tmp de MSYS.
    tmp=$(mktemp -d api-gateway/ca/.tmp.XXXXXX)
    printf '[ext]\nsubjectAltName=DNS:www.google.com\nbasicConstraints=critical,CA:FALSE\nextendedKeyUsage=serverAuth\n' >"$tmp/ext.cnf"
    openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$tmp/k.pem" 2>/dev/null
    openssl req -new -key "$tmp/k.pem" -subj "/CN=www.google.com" -out "$tmp/r.csr" 2>/dev/null
    openssl x509 -req -in "$tmp/r.csr" -CA "$CA" -CAkey api-gateway/ca/bololo-dev-ca.key -set_serial 1 -days 1 \
      -extfile "$tmp/ext.cnf" -extensions ext -out "$tmp/google.crt" 2>/dev/null
    printf '# Certificado falso para www.google.com firmado con la clave de la CA local:\n'
    run "openssl verify -CAfile $CA $tmp/google.crt"
    rm -rf "$tmp"
  } >"$D/04-certificado.txt"

  new_game https://localhost "$TLS"
  {
    for origin in https://localhost http://localhost https://evil.example; do
      printf 'wss:// con Origin %-22s -> ' "$origin"
      bash -c "curl -s $TLS -o /dev/null -w 'HTTP %{http_code}\n' --max-time 2 --http1.1 $ORIGIN_HEADERS -H 'Origin: $origin' 'https://localhost/ws/games/$GAME_ID?token=$GAME_TOKEN'" </dev/null
    done
    printf 'ws://  (texto plano)                   -> '
    bash -c "curl -s -o /dev/null -w 'HTTP %{http_code}, Location: %{redirect_url}\n' --max-time 2 --http1.1 $ORIGIN_HEADERS -H 'Origin: http://localhost' 'http://localhost/ws/games/$GAME_ID?token=x'" </dev/null
  } >"$D/05-websocket.txt" 2>&1

  run "docker compose ps --format '{{.Service}}\t{{.Ports}}' | sort" >"$D/06-puertos.txt"
  latency https://localhost "$D/07-latencia.txt"
fi

for f in "$D"/*.txt; do
  printf '\n===== %s\n' "$f"
  cat "$f"
done
