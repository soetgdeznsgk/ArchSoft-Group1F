# Decisión: Elixir para el Bot Service

| | |
|---|---|
| Estado | Propuesta (rama `feature/bots`) |
| Fecha | Octubre 2026 |
| Requisito relacionado | "Al menos cinco lenguajes de propósito general" (Prototipo 2) |

## Contexto

El sistema ya usa Go (Game Logic), Python (Lobby Manager), JavaScript (Presentación) y Java (Auth). El Bot Service tenía que estar en un lenguaje distinto, y el problema que resuelve tiene una forma particular:

- **Muchos agentes independientes y de larga vida.** Hasta 30 bots a la vez (10 salas × 3 bots), cada uno con dos conexiones abiertas durante minutos: un SSE en la sala de espera y luego un WebSocket en la partida.
- **Orientado a eventos y con tiempos.** Cada bot reacciona a mensajes que llegan en cualquier momento (vistas, errores, robos de otros) y programa acciones a futuro (la pausa de "pensar", reintentos con espera creciente, el límite de espera en la sala).
- **Aislamiento de fallas.** Un bot que falla (red caída, mensaje inesperado, un error de programación) no puede tumbar a los demás ni dejar una partida colgada.
- **Poco cómputo.** Decidir una jugada con una mano de 3 a 12 cartas es trivial. El rendimiento bruto no es el criterio.

## Decisión

**Elixir sobre la máquina virtual de Erlang (BEAM), con OTP.**

La razón es que el modelo de ejecución de Elixir coincide uno a uno con el problema:

| Necesidad | Cómo la cubre Elixir/OTP |
|---|---|
| Un agente por bot | Un proceso BEAM (`GenServer`) por bot. Pesa unos pocos KB y tiene su propio estado y su propio recolector de basura |
| Reaccionar a eventos | Las conexiones de Mint en modo activo entregan los datos del socket como mensajes al buzón del proceso. SSE, WebSocket y temporizadores llegan todos por `handle_info`: no hay hilos, ni callbacks anidados, ni candados |
| Acciones a futuro | `Process.send_after/3` para la pausa de pensar, reintentos y la espera máxima |
| Aislamiento de fallas | Los procesos no comparten memoria: si uno falla, solo muere ese |
| Recuperación | `DynamicSupervisor` con reinicio `:transient`: un bot que falla se reinicia y retoma la partida desde el estado de la sala. Es el patrón *supervisor* de OTP, que viene en el lenguaje, no hay que construirlo |
| Encontrar los bots de una sala | `Registry` (diccionario de procesos incluido en la librería estándar) |

El resultado es un servicio sin base de datos, sin colas y sin hilos explícitos, donde la estructura del código (un proceso por bot bajo un supervisor) es la misma que la del diagrama de componentes.

## Alternativas consideradas

| Opción | A favor | En contra | Veredicto |
|---|---|---|---|
| **Rust** (tokio + tungstenite) | Rendimiento y seguridad de memoria; imagen muy pequeña | Curva de aprendizaje alta para el equipo; aislamiento de fallas y reinicio hay que construirlos a mano (una tarea que entra en pánico no se reinicia sola); compilación lenta en Docker. El rendimiento extra no se aprovecha | Descartada: optimiza lo que no hace falta |
| **C# / .NET** | `async/await` maduro, `ClientWebSocket` incluido | Mismo paradigma que Java (ya en el sistema): aporta poca diversidad; imagen y memoria más pesadas; aislamiento por bot hay que diseñarlo | Viable, menos adecuada |
| **Kotlin** (corrutinas) | Corrutinas estructuradas cómodas | Corre en la JVM como Java; para el requisito de cinco lenguajes es discutible que cuente como "distinto" en espíritu | Descartada |
| **TypeScript / Node** | Rápido de escribir, el equipo ya conoce JS | Es un superconjunto de JavaScript (ya usado): probablemente no cuente como lenguaje nuevo; además es la opción natural para el SSR y podría chocar con otro componente | Descartada |
| **Ruby / PHP** | Sintaxis sencilla | Modelos de concurrencia poco aptos para cientos de conexiones de larga vida | Descartada |
| **Escribir los bots en Go**, dentro de Game Logic (ya existe `internal/bot`) | Reutiliza el motor; cero red | No suma un lenguaje; mezcla el árbitro con los jugadores (un bot en el mismo proceso podría ver estado oculto); no es un componente lógico nuevo | Descartada por la arquitectura |

## Consecuencias

- **Positivas:** tolerancia a fallas por diseño; un bot es un cliente más, así que Lobby y Game Logic no cambiaron; el componente escala a miles de bots en un solo contenedor si hiciera falta; suma el quinto lenguaje.
- **Negativas:** Elixir es nuevo para el equipo (sintaxis funcional, pattern matching). Se mitiga con módulos pequeños y documentados, y separando las decisiones del juego (`Strategy`, `Melds`: funciones puras con pruebas) de la parte de procesos (`Bot`).
- **Imagen:** release de Mix sobre Alpine, ~57 MB con la máquina virtual de Erlang incluida. Se construye en etapas como el resto de servicios y no se genera si fallan las pruebas.
