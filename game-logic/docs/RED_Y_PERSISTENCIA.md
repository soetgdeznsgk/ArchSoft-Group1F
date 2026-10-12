# Red y persistencia de Game Logic

Esta capa envuelve el motor existente en `internal/game`. No implementa reglas
de cartas, turnos ni puntajes: `game.Match` continúa siendo la autoridad de
cada partida.

## Responsabilidades y ciclo de vida

- `POST /internal/games`, autenticado con `X-Service-Token`, crea y persiste
  una partida. `gameId`, `lobbyId` y la lista de cuatro jugadores deben ser
  válidos; el orden de `players` es el orden de asientos del motor.
- La creación es idempotente para el mismo `gameId`, `lobbyId` y roster.
  Reutilizar el identificador con otro lobby o jugadores responde `409`.
- El estado inicial se conserva en Redis como `WAITING`. La partida comienza
  cuando los cuatro jugadores de la lista conectan a su WebSocket.
- `GET /internal/games/{gameId}` devuelve el estado operativo, jugadores,
  conexión y cantidad de cartas, sin exponer las manos.
- `GET /healthz` informa `503` si Redis no responde o si hay snapshots
  pendientes de persistir.

Redis contiene un registro por partida (`bololo:game:{gameId}`) y el conjunto
`bololo:games:active`. La creación usa un script Lua atómico; cada transición
del motor persiste una instantánea y mantiene el identificador en el conjunto
activo. Al confirmarse el resultado por el Lobby, la partida sale del conjunto
y su registro expira después de una hora.

## WebSocket

Ruta: `GET /ws/games/{gameId}?token=<JWT>`. El token HS256 debe llevar
`gameId`, `playerId`, `iat` y `exp`; Go verifica firma, expiración, partida y
pertenencia del jugador antes de aceptar la conexión. El jugador en una acción
siempre se reemplaza por la identidad del token.

El navegador envía directamente las acciones JSON de `game.Action` (`draw`,
`discard`, `steal`, `lay_down` y `deposit`). El servidor responde con:

- eventos públicos del motor (`game.Event`), conservando el número de
  secuencia y sin incluir cartas ocultas;
- un mensaje `{"type":"view","view":...}` dirigido individualmente a cada
  conexión. La vista viene de `Match.ViewFor` y solo incluye la mano propia;
- errores de acción dirigidos al remitente con `type`, `code` y `message`.

Las conexiones tienen una cola acotada; un consumidor que no alcanza a leer se
cierra con `4008`. Una conexión nueva del mismo jugador reemplaza la anterior
con `4001`. El proceso llama `Match.Tick` cada 250 ms. Tras `round_ended`,
espera `ROUND_PAUSE` y llama `StartNextRound` si hay al menos un jugador
conectado. `SetConnected` comunica altas y bajas al motor.

## Snapshot y recuperación

`Match.Snapshot` produce JSON versionado con el estado completo del motor:
roster, manos, bancos, turno/fase, reloj, estadísticas, resultados y secuencia
de eventos. `RestoreMatch` valida el tamaño, versión, roster, cartas únicas y
consistencia del estado antes de reconstruir el `Match`. Una partida activa se
restaura al iniciar el servicio; como sus sockets no sobreviven a un reinicio,
el servicio marca desconectados a los jugadores guardados como conectados sin
ejecutar turnos automáticos durante la restauración. A partir de ahí se aplica
el reloj normal o las reglas de reconexión del motor.

La instantánea conserva exactamente la ronda en curso, pero el flujo aleatorio
interno se vuelve a sembrar al restaurar. Esto no cambia las cartas ya
repartidas ni las reglas; una futura ronda sin ganador puede elegir un nuevo
iniciador, como permite la regla del motor.

Redis es el almacenamiento durable de snapshots, configurado con AOF y
`appendfsync everysec`. La memoria del proceso es la fuente autoritativa entre
escrituras; si Redis falla, el juego sigue en memoria, el error se registra y
la salud queda degradada. El snapshot vuelve a intentarse al siguiente cambio
del motor. Un cierre abrupto durante una caída de Redis puede perder los
cambios posteriores al último snapshot exitoso.

## Resultado y configuración

Al llegar `game_ended`, Go publica el `FinalResult` al Lobby en
`POST /internal/lobbies/{lobbyId}/result`, usando el mismo `gameId` para que
los reintentos sean idempotentes. Se reintentan errores de red y HTTP 5xx con
backoff exponencial; la partida permanece activa en Redis hasta que el Lobby
acepta el resultado.

Variables requeridas: `REDIS_URL`, `JWT_SECRET` y `SERVICE_SECRET`. Opcionales:
`GAME_LOGIC_ADDR` (`:8081`), `REDIS_KEY_PREFIX`, `LOBBY_CALLBACK_URL`,
`ALLOWED_ORIGINS`, `TICK_INTERVAL`, `ROUND_PAUSE`, `REDIS_WRITE_TIMEOUT` y
`LOBBY_CALLBACK_TIMEOUT`. Use secretos generados en el entorno, no los guarde
en Git. Para desarrollo aislado se puede usar `compose.dev.yaml`; publica el
servicio solo en loopback y no publica el puerto de Redis.

## Verificación

```powershell
go test ./...
go test -race ./...
docker compose -f compose.dev.yaml up --build --wait
```

Los tests del motor cubren las reglas. Los nuevos tests verifican la
serialización/restauración, el contrato REST idempotente, autenticación JWT y
el ciclo WebSocket con vistas privadas y jugador derivado del token. El
contenedor de desarrollo valida el servicio con `/healthz`.
