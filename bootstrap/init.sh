#!/usr/bin/env bash
# Prepara los archivos que el stack necesita y que no se versionan (.gitignore):
#   secrets/jwt/*.pem            claves RS256 de los JWT  (scripts/gen-jwt-keys.sh)
#   api-gateway/certs/bololo.*   certificado TLS del borde (scripts/gen-certs.sh)
# Corre como servicio "init" antes que reverse-proxy, auth, lobby-manager y game-logic.
# Si los archivos ya existen no cambia nada, así que se puede ejecutar en cada "up".
set -euo pipefail
cd /repo

KEYS=(secrets/jwt/session-private.pem secrets/jwt/session-public.pem
      secrets/jwt/game-private.pem secrets/jwt/game-public.pem)
CERTS=(api-gateway/certs/bololo.crt api-gateway/certs/bololo.key)

# Si el stack arrancó alguna vez sin estos archivos, Docker creó en su lugar carpetas
# vacías con el mismo nombre, y los contenedores fallan con "is a directory".
for f in "${KEYS[@]}" "${CERTS[@]}"; do
  if [[ -d $f ]]; then
    if rmdir "$f" 2>/dev/null; then
      echo "Carpeta vacía eliminada: $f"
    else
      echo "ERROR: $f es una carpeta con contenido en lugar de un archivo. Bórrala a mano." >&2
      exit 1
    fi
  fi
done

# Verdadero si alguno de los archivos falta o está vacío.
missing() {
  local f
  for f in "$@"; do [[ -s $f ]] || return 0; done
  return 1
}

created=false
if missing "${KEYS[@]}"; then
  ROOT=/repo bash scripts/gen-jwt-keys.sh
  created=true
else
  echo "Claves de los JWT: ya existen"
fi

if missing "${CERTS[@]}"; then
  ROOT=/repo bash scripts/gen-certs.sh >/dev/null
  echo "Certificado TLS generado: api-gateway/certs/bololo.crt (CA local: api-gateway/ca/bololo-dev-ca.crt)"
  echo "Para que el navegador no advierta, confía en la CA una vez en este equipo:"
  echo "  Windows:       certutil -user -addstore Root api-gateway\\ca\\bololo-dev-ca.crt"
  echo "  Git Bash/macOS: ./scripts/gen-certs.sh --trust   (y luego: docker compose restart reverse-proxy)"
  created=true
else
  echo "Certificado TLS: ya existe"
fi

# En Linux el contenedor escribe como root: devuelve los archivos al dueño del repositorio
# para que pueda rotarlos sin sudo. En Docker Desktop (Windows, macOS) no hace falta.
owner=$(stat -c '%u:%g' scripts)
if $created && [[ $owner != 0:0 ]]; then
  chown -R "$owner" secrets api-gateway/certs api-gateway/ca 2>/dev/null || true
fi
