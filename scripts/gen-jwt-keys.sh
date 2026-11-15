#!/usr/bin/env bash
# Genera los pares de claves RS256 con que se firman los JWT (decisión 5).
#
# Cada tipo de token tiene un solo emisor, y solo el emisor recibe la clave privada:
#
#   secrets/jwt/session-private.pem  Firma las sesiones. Solo la monta auth
#   secrets/jwt/session-public.pem   Verifica las sesiones. La montan auth y lobby-manager
#   secrets/jwt/game-private.pem     Firma los tokens de partida. Solo la monta lobby-manager
#   secrets/jwt/game-public.pem      Verifica los tokens de partida. La monta game-logic
#
# docker-compose.yml las monta como secrets en /run/secrets/. Nunca se versionan.
#
# Uso: ./scripts/gen-jwt-keys.sh [--rotate]
#   --rotate  Reemplaza las claves existentes. Las sesiones y partidas abiertas dejan
#             de ser válidas; después: docker compose up -d --force-recreate.
# Variable: KEY_BITS (2048).
# Requiere openssl (Git Bash lo incluye en Windows).

set -euo pipefail

ROOT="${ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$ROOT"

DIR=secrets/jwt
KEY_BITS="${KEY_BITS:-2048}"

ROTATE=false
for arg in "$@"; do
  case "$arg" in
    --rotate) ROTATE=true ;;
    -h | --help) sed -n '2,17p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "Opción desconocida: $arg" >&2; exit 2 ;;
  esac
done

command -v openssl >/dev/null || { echo "Falta openssl (en Windows, ejecuta este script desde Git Bash)." >&2; exit 1; }

umask 077
mkdir -p "$DIR"
chmod 700 "$DIR"

for name in session game; do
  private="$DIR/$name-private.pem"
  public="$DIR/$name-public.pem"
  if [[ -s "$private" && -s "$public" ]] && ! $ROTATE; then
    echo "Ya existen: $private y $public"
    continue
  fi
  tmp=$(mktemp "$DIR/.tmp.XXXXXX")
  openssl genpkey -algorithm RSA -pkeyopt "rsa_keygen_bits:$KEY_BITS" -out "$tmp" 2>/dev/null
  openssl pkey -in "$tmp" -pubout -out "$public.tmp"
  mv -f "$tmp" "$private"
  mv -f "$public.tmp" "$public"
  # Los servicios corren con usuarios sin privilegios que no son dueños del archivo.
  # La carpeta (700) es la que impide que otros usuarios de esta máquina lo lean.
  chmod 644 "$private" "$public"
  echo "Claves nuevas: $private y $public"
done
