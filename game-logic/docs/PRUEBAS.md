# Plan y reporte de pruebas

## 1. Estrategia

| Nivel | Qué verifica | Archivos |
|---|---|---|
| Unitarias | Mazo, tabla de rondas, validación de jugadas, errores | `domain_test.go` |
| Escenarios (caja blanca) | Cada regla con mesas armadas a mano (`rig`) y reloj falso | `setup_test.go`, `turn_test.go`, `steal_test.go`, `meldplay_test.go`, `round_end_test.go`, `timer_test.go`, `view_test.go` |
| Concurrencia | Robos simultáneos y toma del jugador en turno compiten por la misma carta (300 repeticiones) | `view_test.go` (`TestConcurrentStealOnlyOneWins`) |
| Propiedades / simulación | 300 partidas completas con bots y desconexiones; invariantes tras cada acción | `sim_test.go`, `export_test.go` |
| Fuzz | Ninguna jugada inválida es aceptada | `FuzzValidateMeld` |
| Rendimiento | Latencia por acción con 10 partidas en paralelo; benchmarks | `sim_test.go` |

Invariantes revisadas después de cada acción simulada (`CheckInvariants`):

1. Las 60 cartas existen exactamente una vez entre manos, ocultas, vistas y depósitos.
2. Cada banco de depósito tiene al menos una carta normal y solo cartas de su figura o comodines.
3. El robo nunca queda abierto después de que el jugador en turno tomó carta.
4. Los puntos y rondas ganadas acumulados coinciden con los resultados por ronda; el ganador tiene 0.
5. Con la ronda en juego nadie tiene la mano vacía.

## 2. Cómo ejecutar

```powershell
go test ./... -cover                                  # suite completa
go test ./... -short                                  # versión rápida (40 partidas simuladas)
go test -race ./...                                   # requiere gcc de 64 bits
docker build --target test .                          # alternativa: -race dentro del contenedor
go test ./internal/game -run '^$' -fuzz FuzzValidateMeld -fuzztime 30s
go test ./internal/game -run '^$' -bench . -benchmem
go run ./cmd/simulate -games 300 -disconnect 0.05
```

## 3. Matriz de trazabilidad

| Requisito | Pruebas |
|---|---|
| RF-10 Validación en servidor | `TestExchangeValidation`, `TestActionsOutsideRound`, `TestLayDownRejections` (verifica además que el estado no cambia) |
| RF-11 Eventos | `TestStartDealsRoundOne`, `TestFirstTurnIsExchangeOnly`, `TestEventJSON` |
| RF-12 Mazo y reparto | `TestNewDeckComposition`, `TestStartDealsRoundOne`, `TestDealSizesPerRound` |
| RF-13 Jugada objetivo por ronda | `TestRoundSpecs`, `TestDealSizesPerRound` |
| RF-14 Jugador inicial | `TestFirstStarterIsRandom`, `TestNextRoundStartedByWinner`, `TestNextRoundRandomStarterWithoutWinner` |
| RF-15 Turno inicial | `TestFirstTurnIsExchangeOnly` |
| RF-16 Turnos a la derecha | `TestTurnsAdvanceToTheRight` |
| RF-17 30 s por turno | `TestTimeoutAutoExchange`, `TestTimerResetsEachTurn`, `TestCustomTurnDurationAndTimeoutWithEmptyHidden` |
| RF-18 Tomar carta | `TestExchangeValidation`, `TestTakeFromDiscard` |
| RF-19 Botar carta | `TestFirstTurnIsExchangeOnly`, `TestExchangeValidation` |
| RF-20 Intercambio automático | `TestTimeoutAutoExchange`, `TestTimeoutAfterDrawOnlyDiscards`, `TestDisconnectedPlayerAutoExchange`, `TestAllDisconnectedRoundTerminates` |
| RF-21 Robo | `TestStealGivesCardPenaltyAndKeepsTurn` |
| RF-22 Restricciones del robo | `TestStealRestrictions`, `TestStealReopensAfterNextDiscard`, `TestViewFor` (`CanSteal`) |
| RF-23 Penalización | `TestStealGivesCardPenaltyAndKeepsTurn`, `TestStealWithEmptyHiddenEndsRound` |
| RF-24 Turno del afectado | `TestStealGivesCardPenaltyAndKeepsTurn` |
| RF-25 Robos simultáneos | `TestConcurrentStealOnlyOneWins` |
| RF-26 Bajada | `TestLayDownRequiresAllMelds`, `TestLayDownRejections` |
| RF-27 Comodines | `TestValidateMeld`, `FuzzValidateMeld`, `TestLayDownWithAllJokers` |
| RF-28 Depósito | `TestDepositLeftoversToWin`, `TestDepositOnOtherBanksInLaterTurn` |
| RF-29 Validación de depósitos | `TestDepositLeftoversToWin`, `TestViewBanksAreCopiesAndDepositFlag` |
| RF-30 Fin con ganador | `TestLayDownExactHandWinsRound`, `TestDepositOnOtherBanksInLaterTurn`, `TestLayDownWithAllJokers` |
| RF-31 Fin sin ganador | `TestNoWinnerWhenHiddenExhausted`, `TestStealWithEmptyHiddenEndsRound` |
| RF-32 Puntaje | `TestPenalty`, `TestLayDownExactHandWinsRound`, `TestNoWinnerWhenHiddenExhausted` |
| RF-33 Visualización (datos) | `TestViewFor`, `TestViewBeforeStartAndAfterEnd` |
| RF-34 Estadísticas | `TestViewFor`, `TestDisconnectedPlayerAutoExchange`, `checkFinal` en simulación |
| RF-35 Fin de partida y desempate | `TestFullGameEndsAfterFiveRounds`, `TestFinalStandingsTiebreaks`, `TestRandomGamesKeepInvariants` |
| RNF-01 Tiempo de confirmación (parte de la lógica) | `TestActionLatencyBudget`, `BenchmarkExchange` |
| RNF-02 Estado único | `Match` + `TestRandomGamesKeepInvariants` |
| RNF-03 Resolución única del robo | `TestConcurrentStealOnlyOneWins` |
| RNF-05 10 partidas simultáneas | `TestTenConcurrentMatchesAreIsolated`, `TestActionLatencyBudget` |

RF-01..RF-09, RF-36, RNF-04 y RNF-06..RNF-13 no son de este módulo (ver [PLAN.md](PLAN.md)).

## 4. Resultados (10/10/2026, Go 1.27.2, Windows amd64)

| Verificación | Resultado |
|---|---|
| `go vet ./...` / `gofmt -l .` | Sin hallazgos |
| `go test ./... -cover` | OK. Cobertura `game`: **100 %** de sentencias |
| Simulación (300 partidas) | 1451 rondas con ganador, 49 sin ganador, 7004 robos; invariantes OK |
| Fuzz `FuzzValidateMeld` (15 s) | ~1,29 M ejecuciones, sin fallos |
| `BenchmarkExchange` | ~1,0 µs por intercambio (tomar + botar), 9 asignaciones |
| `BenchmarkFullGame` | ~0,87 ms por partida completa con bots |
| Latencia con 10 partidas en paralelo | media ~14 µs, p95 < 1 ms (incluye la decisión del bot); presupuesto RNF-01: 200 ms |
| `go test -race` | OK dentro de la etapa `test` del Dockerfile (golang:1.27.2-bookworm). En Windows local no corre porque el gcc instalado es de 32 bits |

La latencia medida es solo el procesamiento del motor. El RNF-01 completo (red + difusión) debe medirse en la integración.
