# Integración de las ramas del repositorio

Copia de `ArchSoft-Group1F` donde se unieron **todas las ramas del remoto** y se conectaron entre sí. **No se desarrolló nada nuevo**: solo el pegamento necesario para que los componentes se hablen. Nada de esto se ha subido al remoto.

## Ramas

Rama local: `integracion/ramas`, creada desde `origin/main` (`d026fb8`, que ya incluye lobby-manager, game-logic-engine, devops y e2e).

| Rama | Autor | Cómo se integró |
|---|---|---|
| `origin/feature/game-logic-network` (`2fca99f`) | jjossa | `git merge`, sin conflictos (commit local `1d4168e`) |
| `origin/feature/frontend-mesa` (`9f37414`) | mauro1121 | `git merge`, sin conflictos (commit local `d60fc2c`) |
| `feature/lobby-manager`, `feature/game-logic-engine`, `feature/devops`, `feature/e2e` | — | Ya estaban en `main` |

El pegamento **no está commiteado**: `git status` y `git diff` muestran exactamente lo que se agregó encima de las ramas.

## Pegamento agregado

| Archivo | Cambio | Para qué |
|---|---|---|
| `docker-compose.yml` | `game-logic`: de relleno (`whoami`) al servicio real (`build` con `APP=server`, variables, healthcheck `/app -healthcheck`). Traefik apunta al puerto **8081** | Go de la rama atiende WebSocket, REST interno y `/healthz` en un solo puerto (`GAME_LOGIC_ADDR=:8081`) |
| `docker-compose.yml`, `api-gateway/traefik.yml` | Proyecto `bololo-integrado` y red `bololo-integrado_internal` | No mezclar contenedores ni volúmenes con el stack original |
| `presentation/Dockerfile` | `/ingame/` sirve `presentation/frontend/` (la mesa de `frontend-mesa`) en lugar del `ingame/` antiguo | La mesa nueva es la del rol de mesa de juego. El `ingame/` antiguo sigue en el repo, sin tocar |
| `presentation/menu/connection.js` (nuevo) | Sesión (`POST /api/session`), crear sala, unirse por código, sala de espera por SSE, iniciar y pasar a `/ingame/` | El menú solo imprimía el nombre |
| `presentation/menu/menu.js` | `validateUsername` llama a `connectSession` en vez de `print` | Única línea cambiada del menú; usa su estado `LOBBY_PREVIEW` |
| `presentation/menu/index.html`, `style.css` | Carga `connection.js`; texto blanco en el contenedor | Legibilidad sobre el fondo oscuro |
| `presentation/frontend/client/ingame/connection.js` (nuevo) | Abre el WebSocket con el `GameSocket` que ya existía; traduce el mensaje `view` de Go al estado `game` de `sketch.js`; los botones envían acciones (`draw`, `discard`, `lay_down`, `deposit`) | `sketch.js` era una partida local simulada. **`sketch.js` no se modificó** |
| `presentation/frontend/index.html` | Carga `connection.js` como módulo | |

Los contratos entre ramas ya coincidían y no hubo que cambiarlos: el Lobby crea la partida con `POST /internal/games {gameId, lobbyId, players:[{id,name}]}`, firma el token con `gameId`/`playerId`/`iat`/`exp`, y Go reporta a `POST /internal/lobbies/{id}/result` con `X-Service-Token`.

## Verificado

- `docker compose up --build --wait`: los 6 servicios en `healthy`. La imagen de Go corre sus pruebas (`gofmt`, `vet`, `test -race`) antes de construirse.
- `./scripts/e2e.sh`: **49 OK, 0 fallas, 1 omitida**. Detecta el Go real: inicio Lobby → Go, `101` en el WebSocket, `401` con token inválido o de sesión, tokens ausentes de los logs. La omitida es la simulación del final, que esta rama de Go no tiene.
- Navegador con 4 pestañas (Ana, Beto, Caro, Dani):
  - nombre → sala `V7XWQM` → los otros tres entran con el código y la sala se actualiza en vivo (SSE) → **Iniciar** → las 4 pestañas pasan solas a `/ingame/`;
  - la mesa muestra "Conectado", ronda 1 de 5, objetivo "trío", nombres, 3 cartas por jugador, mazo de 48;
  - acción inválida: Go la rechaza con su mensaje de regla;
  - turno vencido: Go hace el intercambio automático y todas las pestañas lo ven;
  - turno completo de Beto (tomar y botar): mano 3 → 4 → 3, mazo 46, turno a Caro, idéntico en la pestaña de Dani;
  - sala `IN_GAME` en PostgreSQL, partida en Redis (`bololo:game:{id}`), sin errores en la consola.

**No verificado de punta a punta:** el aviso del resultado al Lobby al terminar las 5 rondas. Jugar la partida completa a mano toma mucho tiempo y la rama de Go no trae un modo de simulación. El contrato coincide con el del Lobby.

## Lo que la mesa no cubre (no se desarrolló)

- No hay botón **Robar**: la mesa de `frontend-mesa` no lo tiene.
- **Bajar jugadas** envía las cartas seleccionadas como **una sola** jugada. Sirve en las rondas de una jugada (1 y 2); las rondas con dos jugadas necesitarían elegir cómo agrupar.
- **Depositar** usa siempre el banco 0.
- No hay pantalla de fin de ronda ni de fin de partida, ni botón para volver al menú. El resultado final se muestra como mensaje.
- El menú no tiene lista de salas (solo crear y unirse por código) ni recupera la sesión al recargar.
- La mesa carga p5.js desde un CDN (así venía en la rama). Necesita internet.

## Cómo ejecutar

```bash
cd ArchSoft-Group1F-integrado
docker compose up -d --build --wait
./scripts/e2e.sh
```

Abrir `http://localhost` en 4 pestañas: cada una es un jugador (`sessionStorage` es por pestaña).

Este stack, el original (`ArchSoft-Group1F`) y la copia completa (`ArchSoft-Group1F-completo`) usan el puerto 80: hay que detener uno (`docker compose stop`) antes de levantar otro.
