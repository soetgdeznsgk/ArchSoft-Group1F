#!/usr/bin/env bash
# Regenera la evidencia del escenario 4 (Limited View) en docs/p3/evidencia/esc4/<fase>/.
# Cada archivo muestra el comando exacto ($ ...) seguido de su salida.
#
# Uso: ./scripts/esc4-evidence.sh antes|despues
#   antes    solo las pruebas que ya existían en main (base del escenario)
#   despues  las pruebas nuevas, con -v, y la demo con clientes WebSocket
#
# Requiere docker y el stack arriba (para la demo). Las pruebas Go corren en un
# contenedor golang, así que no hace falta Go instalado en el host.

set -uo pipefail
export MSYS_NO_PATHCONV=1
ROOT="${ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$ROOT" || exit 1

PHASE="${1:-}"
if [[ "$PHASE" != "antes" && "$PHASE" != "despues" ]]; then
  echo "uso: $0 antes|despues" >&2
  exit 2
fi
D="docs/p3/evidencia/esc4/$PHASE"
mkdir -p "$D"

PROJECT="${PROJECT:-bololo-integrado}"
GO_IMAGE="${GO_IMAGE:-golang:1.27.2-bookworm}"
# Nombre de la red pública del proyecto y del contenedor del proxy (para la demo).
NET="${NET:-${PROJECT}_public}"
GATEWAY_HOST="${GATEWAY_HOST:-reverse-proxy}"

run() {
  printf '$ %s\n' "$1"
  bash -c "$1" 2>&1 </dev/null | sed "s#$ROOT#.#g"
  printf '\n'
}

if [[ "$PHASE" == "antes" ]]; then
  # El tipo PlayerSummary no tiene campo de cartas: la fuga es imposible por diseño.
  {
    run "sed -n '/type PlayerSummary struct/,/^}/p' game-logic/internal/game/view.go"
    run "grep -n 'verifyGameToken\\|slices.Contains\\|StatusForbidden\\|StatusUnauthorized\\|engine.Apply(c.player' game-logic/internal/server/websocket.go"
  } >"$D/01-codigo.txt"

  # Solo la prueba que ya existía antes de este escenario.
  run "docker run --rm -v \"$ROOT/game-logic:/src\" -w /src $GO_IMAGE go test -run TestViewFor ./internal/game/" >"$D/02-tests-antes.txt"
  echo "Evidencia 'antes' en $D/"
  exit 0
fi

# --- despues ---
# Las cuatro pruebas del escenario, con detalle, más la suite completa de los dos paquetes.
{
  run "docker run --rm -v \"$ROOT/game-logic:/src\" -w /src $GO_IMAGE go test -v -run 'TestViewHidesOtherPlayersCards|TestWebSocketRejectsTokensFromAnotherGameAndRoster|TestChameleonActionUsesTokenIdentityNotMessage|TestChameleonViewIsPublicByDesign' ./internal/game/ ./internal/server/"
  run "docker run --rm -v \"$ROOT/game-logic:/src\" -w /src $GO_IMAGE go test ./internal/game/ ./internal/server/"
} >"$D/01-tests.txt"

# Demo con clientes WebSocket reales a través del proxy (necesita el stack arriba).
run "docker run --rm --network $NET -e GATEWAY_HOST=$GATEWAY_HOST -e PYTHONIOENCODING=utf-8 -e PYTHONUNBUFFERED=1 -v \"$ROOT:/work\" -w /work python:3.12-slim sh -c 'pip install -q --root-user-action=ignore websockets==15.0.1 && python game-logic/scripts/limited_view_demo.py'" >"$D/02-demo.txt"

echo "Evidencia 'despues' en $D/"
