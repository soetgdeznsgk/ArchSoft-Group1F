# Escenario 4 — Limited View

Táctica: autorizar actores. Patrón: Limited View, con la identidad tomada del token.

## Objetivo

En un juego de cartas, ver la mano ajena o actuar como otro jugador es hacer
trampa. El servidor manda a cada jugador **solo lo que le corresponde** y no
confía en el jugador que dice el cliente: la identidad se reemplaza por la del
token del WebSocket. Este escenario revisa, completa y demuestra esa garantía
sobre el código que ya existe.

## Dónde está el código

| Garantía | Archivo | Qué hace |
|---|---|---|
| Vista privada (CLASSIC) | `internal/game/view.go` → `ViewFor` | Incluye `Hand` solo del jugador `p`; de los demás arma `PlayerSummary` con `CardCount`, sin cartas. |
| Vista pública (CHAMELEON) | `internal/chameleon/protocol.go` → `ViewFor` | Incluye las cartas de todos **a propósito** (ver abajo). `You` es el jugador que pide la vista. |
| Identidad del token | `internal/server/websocket.go` → `serveWebSocket` | Saca `gameID`/`playerID` de `verifyGameToken`; exige que `gameID` coincida con la ruta y que el jugador esté en el roster (`403`). |
| Acción con identidad del token | `internal/server/engine.go` → `classicEngine.Apply` y `chameleonEngine.Apply` | Fijan `action.Player = player` (el del token) y descartan el `player` del mensaje. |
| Verificación del token | `internal/server/app.go` → `verifyGameToken` | Solo RS256 con la clave pública del Lobby; rechaza expirado (`exp <= now`), sin `iat`/`exp`, o con `gameId`/`playerId` vacíos. |
| Cada conexión recibe su vista | `internal/server/websocket.go` → `sendView` | Llama `engine.View(client.player)`: cada socket recibe la vista de su propio jugador. |

## CHAMELEON es público por diseño

El modo camaleón es en tiempo real: todos ven los colores que cada jugador va
reuniendo sobre la mesa. Por eso `chameleon.ViewFor` expone las cartas de
todos; el comentario del código lo dice: *"En este modo todo es público"*.
Esto **no es una fuga que corregir**. El límite de "Limited View" en camaleón
es el otro: nadie puede **actuar como otro** ni **pedir la vista de otro**. Eso
sí se cumple y se prueba (`TestChameleonActionUsesTokenIdentityNotMessage`,
`TestChameleonViewIsPublicByDesign`).

Si el equipo decidiera ocultar manos también en camaleón, sería un cambio de
reglas del modo, no una corrección de seguridad, y habría que acordarlo antes.

## Pruebas

| Caso | Prueba | Archivo |
|---|---|---|
| CLASSIC: la mano ajena no se serializa | `TestViewFor`, `TestViewHidesOtherPlayersCards` | `internal/game/view_test.go` |
| CLASSIC: `playerId` falso en una acción usa el del token | `TestWebSocketStartsMatchAndSendsPrivateViews` | `internal/server/app_test.go` |
| CHAMELEON: `playerId` falso en una acción usa el del token | `TestChameleonActionUsesTokenIdentityNotMessage` | `internal/server/limited_view_test.go` |
| CHAMELEON: manos públicas por diseño; `you` es el del token | `TestChameleonViewIsPublicByDesign` | `internal/server/limited_view_test.go` |
| Token de otra partida | `TestWebSocketRejectsTokensFromAnotherGameAndRoster` | `internal/server/limited_view_test.go` |
| Token expirado | `TestWebSocketRejectsTokensFromAnotherGameAndRoster` (+ `TestVerifyGameTokenRejectsTamperedAndExpiredTokens`) | `internal/server/limited_view_test.go`, `app_test.go` |
| Jugador fuera del roster | `TestWebSocketRejectsTokensFromAnotherGameAndRoster` | `internal/server/limited_view_test.go` |
| Firma inválida / algoritmo distinto | `TestVerifyGameTokenOnlyAcceptsRS256FromTheLobbyKey` | `internal/server/app_test.go` |
| Eventos públicos sin cartas ocultas (CLASSIC) | `TestEventJSON` | `internal/game/view_test.go` |

Ejecutar:

```powershell
go test ./internal/game/ ./internal/server/
# o, sin Go local, dentro de un contenedor:
docker run --rm -v "${PWD}:/src" -w /src golang:1.26 go test ./internal/game/ ./internal/server/
```

## Demo con clientes WebSocket reales

`scripts/limited_view_demo.py` sigue el flujo legítimo por el gateway (Auth →
Lobby → inicio de partida → un token de partida por jugador) y abre un
WebSocket por jugador:

| Paso | Qué muestra |
|---|---|
| A1 | CLASSIC, 4 clientes: cada uno recibe la vista con `"you"` = su id. |
| A2 | Cada vista trae solo la mano propia; de los demás, solo `card_count`. |
| A3 | Un `draw` con `"player"` del jugador en turno, enviado por otro, se rechaza (`not_your_turn`). |
| B4 | CHAMELEON, 2 clientes: cada uno recibe su propia vista. |
| B5 | Un `take` con `"player"` falso queda a nombre del jugador del token. |
| C6 | Handshake rechazado (`401`): token de otra partida, token de sesión, firma inválida. |

El token expirado y el jugador fuera del roster no se pueden producir con el
flujo legítimo (el Lobby no emite esos tokens); los cubren las pruebas de Go.

Requisitos: solo Docker y el stack arriba (`docker compose up -d --wait` en la
raíz). La demo corre en un contenedor de Python dentro de la red del gateway y
valida el certificado con la CA local. Desde la raíz del repo, en PowerShell:

```powershell
docker run --rm --network bololo-integrado_public -e GATEWAY_HOST=traefik `
  -e PYTHONIOENCODING=utf-8 -e PYTHONUNBUFFERED=1 -v "${PWD}:/work" -w /work python:3.12-slim `
  sh -c "pip install -q --root-user-action=ignore --disable-pip-version-check websockets==15.0.1 && python game-logic/scripts/limited_view_demo.py"
```

Resultado esperado: `8 OK, 0 fallas`.
