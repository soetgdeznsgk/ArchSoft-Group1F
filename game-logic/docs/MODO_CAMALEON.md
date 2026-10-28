# Modo Camaleón

Segundo modo de juego de Bololó, junto al clásico de 5 rondas. Es en tiempo real y sin turnos: un repartidor automático tira cartas sobre la mesa y todos compiten por tomarlas. Gana quien reúne los **8 colores** sin repetir.

| | |
|---|---|
| Identificador | `CHAMELEON` (Lobby, Go, Redis y vistas) |
| Jugadores | 2 a 4 |
| Duración máxima | 3 minutos |
| Código | `game-logic/internal/chameleon/` (motor) y `internal/server/engine.go` (adaptador) |
| Pantalla básica | `presentation/chameleon/` → `/chameleon/` |

## 1. Cartas

- **8 colores**: `red`, `orange`, `yellow`, `green`, `cyan`, `blue`, `purple`, `pink` (rojo, naranja, amarillo, verde, cian, azul, morado, rosa).
- **Camaleón** (`chameleon`): carta trampa.
- No hay mazo finito. Cada carta se genera al azar: 12 % de probabilidad de camaleón y, si no, uno de los 8 colores con igual probabilidad. Cada carta tiene un `id` único en la partida.

## 2. El repartidor

Mientras la partida está en juego, el servidor:

1. Espera un intervalo **aleatorio entre 0,8 y 2,5 s** y tira una carta.
2. La deja en una **posición aleatoria** de la mesa. `x` e `y` van normalizadas en `[0.08, 0.92]` (fracción del ancho y alto de la mesa); el cliente las escala a su pantalla. Intenta no encimarla con las que ya están.
3. Cada carta queda en la mesa un tiempo **aleatorio entre 0,8 y 2 s** (nunca más de 2 s). Así nadie sabe cuánto le queda a una carta y se premia la velocidad. Si nadie la toma, se retira (evento `card_expired`). El cliente recibe `expires_at` en el evento y `expires_in_ms` en la vista.
4. Nunca hay más de **6 cartas** a la vez. Con la mesa llena, espera al siguiente intervalo.

La primera carta cae 0,8 s después de que inicia la partida.

## 3. Tomar una carta

Cualquier jugador puede tomar cualquier carta de la mesa en cualquier momento; no hay turnos. Al tomarla pasa una de tres cosas:

| La carta es… | Resultado | Evento |
|---|---|---|
| Un color que **no** tiene | Se agrega a su mano | `card_taken` |
| Un color que **ya** tiene | Pierde **ambas**: la de su mano y la que tomó. Puede volver a conseguir ese color después | `color_repeated` (trae `lost` con las 2 cartas) |
| El **camaleón** | Pierde **todas** sus cartas | `chameleon_taken` (trae `lost`) |

Las cartas perdidas salen del juego. Por estas reglas una mano nunca tiene colores repetidos ni camaleones: tiene de 0 a 8 cartas, todas de colores distintos.

**Tomas simultáneas.** Si dos jugadores tocan la misma carta casi a la vez, se la queda quien llegue primero al servidor (`Match` serializa con un mutex). El otro recibe el error `card_not_available`. Lo mismo si la carta ya venció aunque el cliente todavía la muestre.

## 4. Fin de la partida

| Cómo termina | Ganadores | `reason` / `endReason` |
|---|---|---|
| Un jugador reúne los 8 colores | Ese jugador, de inmediato | `all_colors` |
| Pasan 3 minutos | Quienes tengan **más colores**. Si empatan, ganan todos los empatados. Si todos tienen 0, no hay ganador | `time_up` |

**Posiciones** (`standings`): ordenadas por cantidad de colores, de mayor a menor. Los empates comparten posición (`rank`). Cada entrada es `{player, colors, rank}`.

Al terminar, la mesa se vacía, Go envía el resultado al Lobby y la sala se cierra (igual que en el modo clásico).

## 5. Inicio, conexión y reinicio

- **Inicio**: la partida empieza cuando se conectan por WebSocket **todos** los jugadores que envió el Lobby (2, 3 o 4). Antes, cada conexión recibe `game_waiting` con `requiredPlayers`.
- **Desconexión**: no hay intercambio automático porque no hay turnos. El jugador desconectado conserva sus cartas, no toma nuevas y puede reconectarse cuando quiera. El reloj de la partida sigue corriendo.
- **Reinicio de Go**: el estado se guarda en Redis tras cada cambio (snapshot JSON versionado, `mode = CHAMELEON`). Al arrancar, Go restaura la partida con el motor de su modo y marca desconectados a todos. Los plazos son absolutos: el tiempo de la partida y el de las cartas siguió corriendo durante la caída.

## 6. Protocolo (WebSocket `/ws/games/{gameId}?token=<JWT>`)

### Acción del cliente

```json
{"type": "take", "card": 12}
```

Es la única acción. El jugador sale del token, nunca del mensaje. Un mensaje con otros campos o tipos se rechaza con `invalid_action`.

### Eventos (a todos los jugadores)

Todos llevan `seq` (monotónico), `type` y `mode: "CHAMELEON"`.

| `type` | Campos extra |
|---|---|
| `game_started` | — |
| `card_spawned` | `card: {id, color, x, y, spawned_at, expires_at}` |
| `card_expired` | `card` |
| `card_taken` | `player`, `card` |
| `color_repeated` | `player`, `card`, `lost: [{id, color}, {id, color}]` |
| `chameleon_taken` | `player`, `card`, `lost: [...]` |
| `player_connection` | `player`, `connected` |
| `game_ended` | `final: {reason, winners, standings}` |

### Vista (`{"type": "view", "view": {...}}`, tras cada lote de eventos)

En este modo **todas las manos son públicas**: cada jugador ve las cartas de todos.

```json
{
  "mode": "CHAMELEON",
  "you": "p_1",
  "phase": "playing",
  "remaining_ms": 143250,
  "colors": ["red", "orange", "yellow", "green", "cyan", "blue", "purple", "pink"],
  "board": [{"id": 12, "color": "blue", "x": 0.41, "y": 0.66, "expires_in_ms": 3120}],
  "players": [{"id": "p_1", "seat": 0, "connected": true, "cards": [{"id": 3, "color": "red"}], "colors": 1}],
  "can_take": true,
  "final": null
}
```

`phase` es `waiting`, `playing` o `game_over`. `board` solo incluye cartas vigentes. El cliente puede descontar `remaining_ms` y `expires_in_ms` localmente entre vistas.

### Errores (solo al remitente)

`{"type": "error", "code": "...", "message": "..."}` con `code`:

| Código | Cuándo |
|---|---|
| `card_not_available` | La carta ya la tomó otro, venció o no existe |
| `game_not_started` | Aún faltan jugadores por conectarse |
| `game_over` | La partida terminó (o se acabó el tiempo y el servidor aún no la cierra) |
| `invalid_action` | JSON inválido o acción desconocida |

## 7. Integración con el resto del sistema

| Paso | Detalle |
|---|---|
| Crear la sala | `POST /api/lobbies` con `{"mode": "CHAMELEON"}`. Sin cuerpo la sala es `CLASSIC` |
| Iniciar | Desde 2 jugadores (`minPlayers`), máximo 4 (`maxPlayers`) |
| Lobby → Go | `POST /internal/games {gameId, lobbyId, mode: "CHAMELEON", players}` |
| Cliente | `game.mode` (respuesta de `/start` y evento SSE `game_starting`) decide la pantalla: `/chameleon/` |
| Go → Lobby | `POST /internal/lobbies/{id}/result {gameId, mode, winnerId?, winners, endReason, standings}`. El Lobby lo guarda en `game_results.payload` |
| Redis | Hash `bololo:game:{gameId}` con el campo `mode` además del `snapshot` |
| Gateway | Sin rutas nuevas: reutiliza `/api/`, `/ws/games/` y `/` (frontend) |

## 8. Parámetros

Están en `chameleon.Config` (valores por defecto en `internal/chameleon/chameleon.go`). Solo la duración se puede cambiar desde el entorno:

| Parámetro | Por defecto | Variable de entorno |
|---|---|---|
| Duración de la partida | 3 min | `CHAMELEON_GAME_DURATION` (p. ej. `90s`) |
| Intervalo entre cartas | 0,8 a 2,5 s | — |
| Vida de una carta | aleatoria entre 0,8 y 2 s (`CardLifetimeMin`, `CardLifetimeMax`) | — |
| Máximo de cartas en la mesa | 6 | — |
| Probabilidad de camaleón | 12 % | — |

Los parámetros de cada partida se guardan en su snapshot, así que cambiarlos no afecta a las partidas en curso.

## 9. Decisiones por confirmar con el equipo

Las reglas originales no fijaban estos puntos. Cada uno se cambia en un solo lugar del motor:

1. Límite de tiempo de 3 min y desempate por cantidad de colores (sin él, una partida podría no terminar nunca).
2. Vida aleatoria de 0,8 a 2 s por carta y máximo de 6 en la mesa. Las partidas guardadas antes de este cambio conservan su vida fija al restaurarse.
3. 12 % de camaleones.
4. 2 a 4 jugadores (el modo clásico exige 4).
5. Tras un color repetido se puede volver a conseguir ese color.

## 10. Pruebas

| Nivel | Archivo |
|---|---|
| Reglas, repartidor, tiempo, tomas simultáneas (300 carreras), snapshot | `internal/chameleon/chameleon_test.go` |
| HTTP, WebSocket, fin de partida, callback al Lobby, restauración | `internal/server/chameleon_test.go` |
| Campo `mode` en Redis (con Redis real, `TEST_REDIS_URL`) | `internal/store/redis/store_test.go` |
| Salas con modo, límites, resultado | `lobby-manager/tests/test_modes.py` |
| Extremo a extremo por el gateway | `scripts/e2e.sh`, sección "Modo camaleon" |
