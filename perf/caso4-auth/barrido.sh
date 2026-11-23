#!/usr/bin/env bash
set -euo pipefail
AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="${1:?uso: barrido.sh <guest|register|login|me> <niveles...>}"
shift
case "$SCRIPT" in
  guest|register|login|me) ;;
  *) echo "Script inválido: $SCRIPT" >&2; exit 2 ;;
esac
SERVICIOS="${SERVICIOS:-auth auth-postgres traefik reverse-proxy}" \
  exec "$AQUI/../lib/barrido.sh" "$AQUI/$SCRIPT.js" "$AQUI/resultados" "$SCRIPT" "$@"
