#!/usr/bin/env bash
AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICIOS="${SERVICIOS:-frontend ratings-service traefik reverse-proxy}" exec "$AQUI/../lib/barrido.sh" "$AQUI/menu.js" "$AQUI/resultados" "$@"
