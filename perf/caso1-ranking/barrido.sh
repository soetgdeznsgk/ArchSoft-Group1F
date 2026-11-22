#!/usr/bin/env bash
AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "$AQUI/../lib/barrido.sh" "$AQUI/ranking.js" "$AQUI/resultados" "$@"
