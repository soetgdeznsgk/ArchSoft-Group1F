# Bololó: Game Logic Component (Go)

Motor autoritativo de la partida de Bololó (5 rondas de tríos y cuartetos, 4 jugadores). Solo lógica de juego: no incluye red ni persistencia, que se conectan encima (ver [docs/DISENO.md §7](docs/DISENO.md#7-guía-de-integración-para-la-capa-de-red)).

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

### Docker

```powershell
docker build -t bololo/game-logic .              # corre gofmt, vet y pruebas con -race; falla si algo no pasa
docker run --rm bololo/game-logic -games 200     # ejecuta el simulador
docker build --target test .                     # solo la etapa de pruebas
```

La imagen final usa `distroless/static` (~1,5 MB, sin shell, usuario `nonroot`). Cuando exista `cmd/server`, se construye con `--build-arg APP=server`.

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
