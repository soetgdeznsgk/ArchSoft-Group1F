# Bololó: Game Logic Component (Go + Redis)

Servicio autoritativo de las partidas de Bololó (cinco rondas, cuatro jugadores). El motor de reglas se conserva en `internal/game`; `cmd/server` lo expone por REST interno y WebSocket y guarda snapshots de las partidas activas en Redis. Los detalles de integración están en [docs/RED_Y_PERSISTENCIA.md](docs/RED_Y_PERSISTENCIA.md).

## Estructura

```
game-logic/
├── cmd/
│   └── simulate/        CLI que juega partidas completas
├── internal/
│   ├── game/            motor: reglas, eventos, vistas, Match concurrente (+ pruebas)
│   └── bot/             jugador automático para simulaciones
├── docs/                PLAN, DISENO, PRUEBAS
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
