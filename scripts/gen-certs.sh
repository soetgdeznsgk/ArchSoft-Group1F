#!/usr/bin/env bash
# Genera el certificado TLS del API Gateway (escenario 1, Secure Channel).
#
# Crea, si no existe, una CA local de desarrollo y firma con ella el certificado
# que sirve Traefik. La CA lleva la extensión Name Constraints: solo puede firmar
# localhost, *.test, *.local, *.home.arpa e IPs privadas o de loopback. Si alguien
# roba su clave, no le sirve para suplantar sitios reales de Internet.
#
#   api-gateway/ca/bololo-dev-ca.crt  CA local. Se instala como confiable en cada equipo/navegador
#   api-gateway/ca/bololo-dev-ca.key  Clave de la CA. No sale de esta máquina ni entra al contenedor
#   api-gateway/certs/bololo.crt      Certificado del gateway (ECDSA P-256)
#   api-gateway/certs/bololo.key      Su clave privada. Solo la lee Traefik (montaje :ro)
#
# Uso: ./scripts/gen-certs.sh [--trust] [--new-ca] [host ...]
#   host      Nombres o IPs extra, por ejemplo la IP de la LAN para jugar desde otro equipo:
#             ./scripts/gen-certs.sh 192.168.1.20
#   --trust   Instala la CA como raíz confiable del usuario (Windows, macOS). Pide confirmación.
#   --new-ca  Crea una CA nueva aunque ya exista (hay que volver a confiarla en cada equipo).
# Variables: CERT_DAYS (397), CA_DAYS (825).
#
# Tras renovar el certificado: docker compose restart reverse-proxy.
# Requiere openssl (Git Bash lo incluye en Windows).

set -euo pipefail

ROOT="${ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$ROOT"

CA_DIR=api-gateway/ca
CERT_DIR=api-gateway/certs
CA_CRT="$CA_DIR/bololo-dev-ca.crt"
CA_KEY="$CA_DIR/bololo-dev-ca.key"
CRT="$CERT_DIR/bololo.crt"
KEY="$CERT_DIR/bololo.key"
CERT_DAYS="${CERT_DAYS:-397}"
CA_DAYS="${CA_DAYS:-825}"

TRUST=false
NEW_CA=false
HOSTS=()
for arg in "$@"; do
  case "$arg" in
    --trust) TRUST=true ;;
    --new-ca) NEW_CA=true ;;
    -h | --help) sed -n '2,24p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    -*) echo "Opción desconocida: $arg" >&2; exit 2 ;;
    *) HOSTS+=("$arg") ;;
  esac
done

command -v openssl >/dev/null || { echo "Falta openssl (en Windows, ejecuta este script desde Git Bash)." >&2; exit 1; }

is_ipv4() {
  [[ $1 =~ ^([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})$ ]] || return 1
  local octet
  for octet in "${BASH_REMATCH[@]:1}"; do ((10#$octet <= 255)) || return 1; done
}

# Solo nombres e IPs que la CA puede firmar (los mismos de su Name Constraints).
allowed_host() {
  local host=${1,,}
  if is_ipv4 "$host"; then
    [[ $host == 127.* || $host == 10.* || $host == 192.168.* ]] && return 0
    [[ $host =~ ^172\.(1[6-9]|2[0-9]|3[01])\. ]] && return 0
    return 1
  fi
  [[ $host == localhost || $host == *.localhost || $host == *.test || $host == *.local || $host == *.home.arpa ]]
}

for host in "${HOSTS[@]}"; do
  if ! allowed_host "$host"; then
    echo "No se puede incluir '$host': la CA solo firma localhost, *.test, *.local, *.home.arpa" >&2
    echo "e IPs privadas (10/8, 172.16/12, 192.168/16) o de loopback (127/8, ::1)." >&2
    exit 2
  fi
done

umask 077
mkdir -p "$CA_DIR" "$CERT_DIR"
# Temporales dentro de api-gateway/ca (ignorada por Git) y con rutas relativas:
# así el openssl nativo de Git Bash no tropieza con rutas /tmp de MSYS.
TMP=$(mktemp -d "$CA_DIR/.tmp.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

# 1. CA local (se reutiliza si existe y vence después que el certificado nuevo).
if [[ -s "$CA_CRT" && -s "$CA_KEY" ]] && ! $NEW_CA; then
  if ! openssl x509 -in "$CA_CRT" -noout -checkend $((CERT_DAYS * 86400)) >/dev/null; then
    echo "La CA $CA_CRT vence antes que el certificado nuevo. Usa --new-ca." >&2
    exit 1
  fi
  echo "CA local existente: $CA_CRT"
else
  cat >"$TMP/ca.cnf" <<EOF
[req]
prompt = no
distinguished_name = dn
[dn]
O = Bololo 1F (desarrollo local)
CN = Bololo Dev CA - $(hostname 2>/dev/null || echo local)
[v3_ca]
basicConstraints = critical, CA:TRUE, pathlen:0
keyUsage = critical, keyCertSign, cRLSign
subjectKeyIdentifier = hash
nameConstraints = critical, @nc
[nc]
permitted;DNS.1 = localhost
permitted;DNS.2 = test
permitted;DNS.3 = local
permitted;DNS.4 = home.arpa
permitted;IP.1 = 127.0.0.0/255.0.0.0
permitted;IP.2 = 10.0.0.0/255.0.0.0
permitted;IP.3 = 172.16.0.0/255.240.0.0
permitted;IP.4 = 192.168.0.0/255.255.0.0
permitted;IP.5 = 0:0:0:0:0:0:0:1/FFFF:FFFF:FFFF:FFFF:FFFF:FFFF:FFFF:FFFF
EOF
  openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$TMP/ca.key" 2>/dev/null
  openssl req -x509 -new -key "$TMP/ca.key" -sha256 -days "$CA_DAYS" \
    -config "$TMP/ca.cnf" -extensions v3_ca -out "$TMP/ca.crt"
  mv -f "$TMP/ca.key" "$CA_KEY"
  mv -f "$TMP/ca.crt" "$CA_CRT"
  chmod 600 "$CA_KEY"
  chmod 644 "$CA_CRT"
  echo "CA local nueva: $CA_CRT"
fi

# 2. Certificado del gateway firmado por la CA.
{
  cat <<EOF
[req]
prompt = no
distinguished_name = dn
[dn]
O = Bololo 1F
CN = localhost
[v3_srv]
basicConstraints = critical, CA:FALSE
keyUsage = critical, digitalSignature
extendedKeyUsage = serverAuth
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always
subjectAltName = @alt
[alt]
DNS.1 = localhost
IP.1 = 127.0.0.1
IP.2 = 0:0:0:0:0:0:0:1
EOF
  dns=1
  ip=2
  for host in "${HOSTS[@]}"; do
    if is_ipv4 "$host"; then
      ip=$((ip + 1))
      echo "IP.$ip = $host"
    else
      dns=$((dns + 1))
      echo "DNS.$dns = ${host,,}"
    fi
  done
} >"$TMP/srv.cnf"

openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$TMP/srv.key" 2>/dev/null
openssl req -new -key "$TMP/srv.key" -config "$TMP/srv.cnf" -out "$TMP/srv.csr"
openssl x509 -req -in "$TMP/srv.csr" -CA "$CA_CRT" -CAkey "$CA_KEY" \
  -set_serial "0x$(openssl rand -hex 16)" -days "$CERT_DAYS" -sha256 \
  -extfile "$TMP/srv.cnf" -extensions v3_srv -out "$TMP/srv.crt" 2>/dev/null
openssl verify -CAfile "$CA_CRT" "$TMP/srv.crt" >/dev/null
mv -f "$TMP/srv.key" "$KEY"
mv -f "$TMP/srv.crt" "$CRT"
chmod 600 "$KEY"
chmod 644 "$CRT"

echo "Certificado del gateway: $CRT"
openssl x509 -in "$CRT" -noout -ext subjectAltName | sed -n '2p' | sed 's/^ */  nombres: /'
openssl x509 -in "$CRT" -noout -enddate | sed 's/notAfter=/  vence:   /'
openssl x509 -in "$CA_CRT" -noout -fingerprint -sha256 | sed 's/.*=/  huella SHA-256 de la CA: /'

# 3. Opcional: confiar en la CA en este equipo.
if $TRUST; then
  case "$(uname -s)" in
    MINGW* | MSYS* | CYGWIN*)
      echo "Windows pedirá confirmar la instalación de la CA en el almacén del usuario actual."
      certutil.exe -user -addstore Root "$(cygpath -w "$CA_CRT")" >/dev/null
      echo "CA instalada (Edge y Chrome la usan; Firefox también si security.enterprise_roots.enabled=true)."
      ;;
    Darwin)
      security add-trusted-cert -r trustRoot -k "$HOME/Library/Keychains/login.keychain-db" "$CA_CRT"
      echo "CA instalada en el llavero de inicio de sesión."
      ;;
    *)
      echo "Linux: sudo cp $CA_CRT /usr/local/share/ca-certificates/bololo-dev-ca.crt && sudo update-ca-certificates"
      echo "Chrome/Firefox (NSS): certutil -d sql:\$HOME/.pki/nssdb -A -t C,, -n 'Bololo Dev CA' -i $CA_CRT"
      ;;
  esac
else
  echo "Para que el navegador no advierta, confía en la CA una vez: ./scripts/gen-certs.sh --trust"
  echo "(o importa $CA_CRT como autoridad raíz en el navegador del otro equipo)."
fi
echo "Si el stack ya estaba arriba: docker compose restart reverse-proxy"
