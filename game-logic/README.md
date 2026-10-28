# Bololó: Game Logic Component (Go + Redis)

Servicio autoritativo de las partidas de Bololó. Tiene dos modos de juego, cada uno con su motor:

| Modo | Motor | Jugadores | Reglas |
|---|---|---|---|
| `CLASSIC` | `internal/game` | 4 | [docs/DISENO.md](docs/DISENO.md) |
| `CHAMELEON` | `internal/chameleon` | 2 a 4 | [docs/MODO_CAMALEON.md](docs/MODO_CAMALEON.md) |

`cmd/server` expone ambos por REST interno y WebSocket y guarda snapshots de las partidas activas en Redis. El servidor no conoce las reglas: habla con cada motor a través de la interfaz `engine` (`internal/server/engine.go`). Para agregar un modo nuevo basta con un paquete de motor, un adaptador en `engine.go` y su entrada en `playerLimits`. Los detalles de integración están en [docs/RED_Y_PERSISTENCIA.md](docs/RED_Y_PERSISTENCIA.md).

## Estructura

```
game-logic/
├── cmd/
│   ├── server/          servicio: REST interno, WebSocket, Redis
│   └── simulate/        CLI que juega partidas clásicas completas
├── internal/
│   ├── game/            motor del modo clásico (+ pruebas)
│   ├── chameleon/       motor del modo camaleón (+ pruebas)
│   ├── server/          HTTP, WebSocket, ticks, callback al Lobby; engine.go adapta cada modo
│   ├── store/redis/     persistencia de snapshots
│   └── bot/             jugador automático para simulaciones
├── docs/                PLAN, DISENO, PRUEBAS, RED_Y_PERSISTENCIA, MODO_CAMALEON
└── Dockerfile
```

## Uso rápido

Requiere Go 1.22 o superior.

```powershell
go test ./... -cover
go run ./cmd/simulate -games 100
```

Con `make` (Linux/macOS/CI): `make check`, `make test`, `make race`, `make fuzz`, `make bench`, `make sim`.

### Servicio y Redis en desarrollo

Se requiere Docker Compose. Crea `game-logic/.env` con secretos locales no
versionados:

```dotenv
REDIS_PASSWORD=valor_hexadecimal
JWT_SECRET=secreto_hexadecimal
SERVICE_SECRET=otro_secreto_hexadecimal
```

Genera valores aleatorios, por ejemplo, con `openssl rand -hex 32`. Desde la
raíz del repositorio:

```powershell
docker compose -f game-logic/compose.dev.yaml up --build --wait
```

El WebSocket queda disponible en `ws://localhost:18081/ws/games/{gameId}` y
`/healthz` comprueba Redis. El puerto del servicio se publica solo en
`127.0.0.1`; Redis no publica puertos. Para construir solo el servicio:

```powershell
docker build --build-arg APP=server -t bololo/game-logic game-logic
```

### Docker

```powershell
docker build -t bololo/game-logic .              # corre gofmt, vet y pruebas con -race; falla si algo no pasa
docker run --rm bololo/game-logic -games 200     # ejecuta el simulador
docker build --target test .                     # solo la etapa de pruebas
```

La imagen final usa `distroless/static` (sin shell, usuario `nonroot`). El
Dockerfile mantiene el simulador como ejecutable predeterminado; para el
servicio usa `--build-arg APP=server`.

## Ejemplo

```go
m, _ := game.NewMatch([]game.PlayerID{"ana", "beto", "caro", "dani"}, game.Config{})
events, _ := m.Start()

_, err := m.Apply(game.Action{Type: game.ActDraw, Player: "ana", Source: game.SourceHidden})
if err != nil {
    // game.CodeOf(err) -> "not_your_turn", "steal_not_allowed", ...
}
view, _ := m.ViewFor("ana") // mano propia, conteos ajenos, bancos, turno, tiempo, Can*
```

## Documentación

- [docs/PLAN.md](docs/PLAN.md): alcance, ciclo de vida, riesgos, definición de terminado.
- [docs/DISENO.md](docs/DISENO.md): arquitectura, máquina de estados, API, decisiones sobre reglas.
- [docs/PRUEBAS.md](docs/PRUEBAS.md): estrategia, trazabilidad RF/RNF → pruebas, resultados.
- [docs/RED_Y_PERSISTENCIA.md](docs/RED_Y_PERSISTENCIA.md): APIs del servicio, WebSocket, snapshots y recuperación.
- [docs/MODO_CAMALEON.md](docs/MODO_CAMALEON.md): reglas, protocolo y parámetros del modo camaleón.

Las pruebas del store contra Redis real se activan con `TEST_REDIS_URL` (por ejemplo `redis://127.0.0.1:16379/15`); sin esa variable se omiten.
