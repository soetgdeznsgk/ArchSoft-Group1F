# Plan de desarrollo: Game Logic Component (Go)

## 1. Alcance

Componente de lógica de la partida de Bololó (modo de 5 rondas con tríos y cuartetos), según la vista C&C: es el "Game Logic Component (Golang)" detrás del API Gateway.

Incluido:

- Reglas completas del juego: mazo, reparto, rondas, turnos, tomar/botar, robo, bajada, depósito, comodines, fin de ronda, puntaje, fin de partida y desempate.
- Validación autoritativa de cada acción con mensajes informativos (RF-10).
- Eventos de cambio de estado para difundir (RF-11) y vistas por jugador (RF-33, RF-34).
- Árbitro único y concurrente por partida (RNF-02, RNF-03) y aislamiento entre partidas (RNF-05).
- Límite de 30 s por turno e intercambio automático (RF-17, RF-20), con reloj inyectable.

Fuera de alcance (lo hacen otras piezas):

| Tema | Responsable |
|---|---|
| WebSocket, serialización en la red, Traefik | Capa de red del componente Go |
| Persistencia del estado en Redis | Capa de datos del componente Go |
| Salas, ingreso, inicio desde la sala (RF-01..RF-09) | Lobby Manager (FastAPI) |
| Registro único del resultado y cierre de la sala (RF-36, RNF-04) | Lobby Manager + REST interno |
| Interfaz (RNF-08..RNF-10) | Presentación (p5.js) |

## 2. Ciclo de vida

| Fase | Actividades | Entregable | Estado |
|---|---|---|---|
| 1. Requisitos | Análisis de la descripción del juego y de RF/RNF; identificación de ambigüedades | Sección 5 de [DISENO.md](DISENO.md) | Hecho |
| 2. Diseño | Modelo de dominio, máquina de estados, API, eventos, errores, concurrencia | [DISENO.md](DISENO.md) | Hecho |
| 3. Implementación | Paquete `game`, bot de pruebas, simulador CLI | `internal/game/`, `internal/bot/`, `cmd/simulate/` | Hecho |
| 4. Pruebas | Unitarias, escenarios, concurrencia, propiedades, fuzz, rendimiento | [PRUEBAS.md](PRUEBAS.md) | Hecho |
| 5. Integración continua | gofmt, vet, pruebas con `-race`, fuzz corto | `.github/workflows/game-logic.yml` | Hecho |
| 6. Integración | Conectar WebSocket, Redis y REST con el Lobby (guía en DISENO §7) | Fuera de este entregable | Pendiente |
| 7. Mantenimiento | Resolver puntos pendientes del reglamento (DISENO §5) | Cambios acotados en `internal/game/` | Pendiente |

## 3. Iteraciones realizadas

1. Dominio: cartas, mazo, tabla de rondas, validación de jugadas y comodines.
2. Motor: máquina de estados, intercambio, robo, bajada, depósito, cierre de ronda y partida.
3. Tiempo y conexión: plazo de turno, `Tick`, intercambio automático, desconexión.
4. Integración interna: vistas, `Action`/`Apply` para JSON, `Match` concurrente.
5. Verificación: suite de pruebas, simulación masiva con bots, fuzz y benchmarks.

## 4. Definición de terminado

- `gofmt -l .` sin salida y `go vet ./...` sin hallazgos.
- `go test ./...` en verde; cobertura del paquete `game` ≥ 90 % (actual: 100 %).
- Cada RF/RNF del alcance tiene al menos una prueba en la matriz de [PRUEBAS.md](PRUEBAS.md).
- Las decisiones sobre reglas ambiguas están documentadas.

## 5. Riesgos

| Riesgo | Mitigación |
|---|---|
| Reglas ambiguas o aún por confirmar (reglamento §14) | Documentadas como decisiones; cada una vive en un solo punto del código |
| Condiciones de carrera en robos simultáneos | `Match` serializa todo con un mutex; prueba con 300 carreras y CI con `-race` |
| Partida bloqueada por un jugador ausente | Plazo de 30 s con `Tick` e intercambio automático inmediato si está desconectado |
| Fugas de información (cartas ocultas) | Los eventos nunca incluyen cartas ocultas; cada jugador recibe solo su `ViewFor` |
| Divergencia con el cliente | Códigos de error estables y JSON documentado en `Action`, `Event` y `PlayerView` |
