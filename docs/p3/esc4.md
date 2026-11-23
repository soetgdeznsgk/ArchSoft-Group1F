# Escenario 4 · Limited View (vista limitada e identidad tomada del token)

| | |
|---|---|
| Atributo de calidad | Seguridad: confidencialidad de la mano de cada jugador e integridad de las acciones (no actuar en nombre de otro) |
| Táctica | Resistir ataques → **Autorizar actores**: el servidor decide qué ve y qué puede hacer cada jugador, a partir de su identidad verificada |
| Patrón | **Limited View**: a cada actor se le entrega solo la parte del estado que le corresponde. Se combina con **identidad tomada del token**: el jugador de cada acción sale del JWT del WebSocket, nunca del mensaje del cliente |
| Rama | `feature/p3-esc4` (desde `main`) |
| Responsable | ______________________ |
| Evidencia | [`docs/p3/evidencia/esc4/`](evidencia/esc4/) |

## Contenido

1. [Estado antes del cambio](#1-estado-antes-del-cambio)
2. [Amenazas identificadas antes de implementar](#2-amenazas-identificadas-antes-de-implementar)
3. [Escenario de calidad](#3-escenario-de-calidad)
4. [Opciones evaluadas y decisión](#4-opciones-evaluadas-y-decisión)
5. [Implementación](#5-implementación)
6. [Qué cambia en la arquitectura](#6-qué-cambia-en-la-arquitectura)
7. [Evidencia: antes y después](#7-evidencia-antes-y-después)
8. [Compromisos](#8-compromisos)
9. [Riesgos residuales y trabajo futuro](#9-riesgos-residuales-y-trabajo-futuro)
10. [Cómo usarlo (equipo)](#10-cómo-usarlo-equipo)
11. [Guion de la demo (3 a 4 minutos)](#11-guion-de-la-demo-3-a-4-minutos)
12. [Referencias](#12-referencias)

---

## 1. Estado antes del cambio

Medido sobre `main` con el stack completo arriba. La evidencia está en [`evidencia/esc4/antes/`](evidencia/esc4/antes/).

A diferencia de los escenarios 1 y 2, este no parte de cero: el motor ya aplica Limited View en el modo clásico. La tarea es **revisarlo, verificar que el modo camaleón también cumple, cerrar los huecos de prueba y demostrarlo**. El estado "antes" es, entonces, qué garantiza ya el código y qué no estaba comprobado.

| Hecho | Por qué importa | Evidencia |
|---|---|---|
| `game-logic/internal/game/view.go` define `PlayerView.Hand` (la mano propia) y `PlayerView.Players []PlayerSummary`. `PlayerSummary` **no tiene campo de cartas**: solo `CardCount int` | De los demás jugadores el servidor solo puede informar un conteo; la mano ajena ni siquiera existe en el tipo que se serializa | [01-codigo.txt](evidencia/esc4/antes/01-codigo.txt) |
| `ViewFor(p)` llena `Hand` con `slices.Clone(me.hand)` solo para el jugador `p` | La vista es una copia de la mano propia; modificarla no afecta la partida | 01-codigo.txt |
| `internal/server/websocket.go` obtiene `playerID` de `verifyGameToken(...)` y, en `readPump`, aplica cada acción con `engine.Apply(c.player, message)` | La identidad sale del token, no del mensaje: el campo `player` del JSON se descarta | 01-codigo.txt |
| El handshake rechaza con `401` si el token es inválido o el `gameID` no coincide, y con `403` si el jugador no está en el roster (`slices.Contains`) | Un token de otra partida o de alguien ajeno no abre la conexión | 01-codigo.txt |
| El modo **camaleón** (`internal/chameleon/protocol.go`) expone las cartas de todos en `ViewFor` | Es correcto **a propósito**: en camaleón las manos son públicas (se ven los colores que reúne cada quien). El límite aquí no es ocultar manos, sino la identidad del token | 01-codigo.txt |
| Pruebas que ya existían: `TestViewFor` verifica que la mano ajena no se serializa; `TestVerifyGameTokenRejectsTamperedAndExpiredTokens` cubre token manipulado y expirado a nivel de función | La base está probada, pero de forma parcial | [02-tests-antes.txt](evidencia/esc4/antes/02-tests-antes.txt) |
| **Sin prueba** de extremo a extremo: token de otra partida en el handshake, jugador fuera del roster, `playerId` falso en una acción de camaleón, ni una prueba que fije que camaleón es público por diseño | Lo que no se prueba se puede romper sin que nadie lo note en el siguiente cambio | 02-tests-antes.txt |

Lo que ya había y se conserva: el canal seguro (escenario 1), el proxy inverso (escenario 2) y el JWT asimétrico RS256 con un par de claves por emisor (decisión 5). El token de partida lo firma el Lobby y lo verifica game-logic con la clave pública.

## 2. Amenazas identificadas antes de implementar

Modelo de atacante: un **jugador legítimo y malintencionado**. Tiene una sesión válida y está en una partida; usa un cliente modificado (o `websocat`) para pedir lo que no le toca. No necesita romper el TLS ni el proxy: actúa dentro del WebSocket, ya autenticado.

Clasificación STRIDE. La columna "¿Lo cubre?" indica si este escenario la mitiga.

| ID | Amenaza (STRIDE) | Cómo ocurriría en Bololó | Impacto | ¿Lo cubre? |
|---|---|---|---|---|
| T1 | **Ver la mano ajena** (Information disclosure) | El cliente espera que el mensaje `view` traiga las cartas de los rivales, o pide una vista que las incluya | Trampa: el jugador conoce las manos y gana siempre | Sí (clásico): `PlayerSummary` no tiene campo de cartas; es imposible serializarlas. Se prueba con `TestViewFor` y `TestViewHidesOtherPlayersCards` |
| T2 | **Actuar como otro jugador** (Spoofing) | El cliente envía `{"type":"draw","player":"otro",...}` o un `take` a nombre de otro para robar su turno o su carta | Trampa: mover o robar por cuenta ajena | Sí: `Apply(c.player, …)` sobrescribe el `player` del mensaje con el del token. Se prueba en los dos modos |
| T3 | **Entrar con un token de otra partida** (Spoofing) | Un token válido, firmado para `game_otro`, se usa en el WebSocket de `game_1` | Espiar o interferir en una partida ajena | Sí: el handshake compara el `gameID` del token con la ruta y responde `401` |
| T4 | **Entrar sin pertenecer al roster** (Elevation of privilege) | Un token firmado para un jugador que no está en la lista de esa partida | Un espectador se cuela como jugador | Sí: `slices.Contains(Players(), playerID)` → `403` |
| T5 | **Token expirado o manipulado** (Spoofing / Tampering) | Reusar un token viejo o alterar sus *claims* | Suplantar una identidad fuera de tiempo | Sí: `verifyGameToken` valida firma RS256, `exp` e `iat`; el handshake lo rechaza con `401` |
| T6 | **Cartas ocultas en eventos públicos** (Information disclosure) | Un evento difundido a todos (por ejemplo "jugador robó carta") incluye qué carta era | Fuga indirecta de información privada | Sí (clásico): `TestEventJSON` verifica que el evento `card_drawn` no trae el campo `card` |
| T7 | Camaleón tratado como clásico (Information disclosure) | Alguien asume que camaleón debe ocultar manos y lo "corrige", rompiendo la mecánica; o al revés, se filtra en clásico creyendo que da igual | Romper el juego o abrir una fuga | Sí: queda documentado y fijado con `TestChameleonViewIsPublicByDesign` que camaleón es público por diseño y clásico es privado |

Riesgo antes del cambio: T1, T2 y T6 ya estaban mitigados en el clásico por diseño del código, pero T2 no tenía prueba en camaleón y T3, T4 y T7 no tenían prueba de extremo a extremo. El riesgo real era de **regresión**: un cambio futuro podía reintroducir la fuga sin que ninguna prueba fallara.

## 3. Escenario de calidad

| | |
|---|---|
| **Fuente** | Jugador legítimo y malintencionado, con sesión válida y un cliente modificado |
| **Estímulo** | Intenta ver las cartas de otro jugador, enviar una acción con el identificador de otro, o conectarse con un token que no corresponde a esa partida o jugador |
| **Artefacto** | Servicio `game-logic`: el WebSocket (`internal/server/websocket.go`, conector C3 de la vista C&C) y el motor de cada modo (`internal/game`, `internal/chameleon`) |
| **Entorno** | Partida en curso, operación normal |
| **Respuesta** | El servidor envía a cada conexión solo su propia vista (su mano; de los demás, solo el conteo). Fija el jugador de cada acción con la identidad del token y descarta el `player` del mensaje. Rechaza en el handshake los tokens de otra partida, expirados o de alguien fuera del roster |
| **Medida** | 0 cartas ajenas en el mensaje `view` del modo clásico; 100 % de las acciones suplantadas se ejecutan como el dueño del token (clásico y camaleón); 100 % de los handshakes con token de otra partida, de sesión o inválido reciben `401`, y los de jugador fuera del roster `403`. Pruebas Go: todas PASS |
| **Táctica** | Resistir ataques: autorizar actores (la vista y la identidad las decide el servidor, no el cliente) |
| **Patrón** | Limited View + identidad tomada del token |
| **Implementación** | Pruebas nuevas en `internal/server/limited_view_test.go` y `internal/game/view_test.go`; demo con dos clientes WebSocket en `game-logic/scripts/limited_view_demo.py`. No cambia la lógica del juego. Detalle en §5 |
| **Evidencia** | `go test` de los dos paquetes; corrida de la demo contra el stack real. Detalle en §7 |
| **Compromisos** | Se gana una garantía comprobada y protegida contra regresiones, sin tocar la lógica. Cuesta mantener las pruebas y aceptar que en camaleón las manos son públicas por diseño (§8) |

## 4. Opciones evaluadas y decisión

La táctica es **autorizar actores** del catálogo de seguridad. El patrón de referencia es **Limited View**: cada actor recibe una proyección del estado restringida a lo que le corresponde. Las opciones se compararon según lo que importa aquí: que la mano ajena sea imposible de filtrar (T1), que el cliente no pueda elegir identidad (T2) y que todo quede protegido contra regresiones.

### 4.1 Dónde se decide qué ve cada jugador

| Opción | A favor | En contra | Decisión |
|---|---|---|---|
| **A. El servidor construye una vista por jugador (`ViewFor`) y nunca envía el estado completo** | La mano ajena no existe en el tipo serializado (`PlayerSummary` sin cartas): la fuga es imposible, no solo improbable. Ya está implementado | Hay que construir una vista por conexión en cada cambio de estado | **Elegida** (ya implementada; este escenario la verifica y la prueba) |
| B. Enviar el estado completo y que el cliente oculte lo ajeno | Menos código en el servidor | El cliente recibe las cartas de todos: cualquiera las lee con las herramientas del navegador. Es exactamente la trampa que se quiere evitar | Descartada |
| C. Cifrar la mano de cada jugador con una clave distinta | Defensa en profundidad | Complejidad enorme (gestión de claves por jugador) para un dato que el servidor ya puede, simplemente, no enviar | Descartada |

### 4.2 De dónde sale la identidad de cada acción

| Opción | A favor | En contra | Decisión |
|---|---|---|---|
| **A. Del token del WebSocket (`c.player`), ignorando el `player` del mensaje** | El cliente no puede actuar como otro: el servidor reescribe el campo. Ya está implementado en `readPump` | — | **Elegida** |
| B. Del campo `player` del mensaje, validándolo contra el token | Formato de mensaje uniforme | Si alguna ruta olvida validar, se abre T2. Es más seguro no confiar nunca en ese campo | Descartada |

### 4.3 Qué se agrega sobre lo que ya había, y por qué

| Agregado | Amenaza | Motivo |
|---|---|---|
| `TestViewHidesOtherPlayersCards` (paquete `game`) | T1 | Refuerza `TestViewFor` con una aserción directa: ningún resumen ajeno serializa cartas, en la vista de cada uno de los cuatro jugadores |
| `TestWebSocketRejectsTokensFromAnotherGameAndRoster` (paquete `server`) | T3, T4, T5 | Prueba de extremo a extremo del handshake: token de otra partida (`401`), token expirado (`401`), token de sesión en vez de partida (`401`) y jugador fuera del roster (`403`) |
| `TestChameleonActionUsesTokenIdentityNotMessage` | T2 | Verifica en **camaleón** que un `take` con `player` falso queda a nombre del dueño del token. El clásico ya se cubría en `TestWebSocketStartsMatchAndSendsPrivateViews` |
| `TestChameleonViewIsPublicByDesign` | T7 | Fija por escrito la decisión de diseño: en camaleón las manos son públicas y `you` es la identidad del token. Si alguien lo cambia, la prueba se lo recuerda |
| Demo `limited_view_demo.py` (dos clientes WebSocket reales) | T1, T2, T3 | Demostración viva a través del proxy (`wss://localhost/ws/games/{id}?token=…`), no solo pruebas unitarias |

**Nota sobre el esquema de la guía.** El ejemplo de la guía usa `view.Opponents` y `other.Hand`. El código real no tiene `Opponents` ni un campo `Hand` en los rivales: usa `view.Players []PlayerSummary` y `PlayerSummary` solo lleva `CardCount`. Eso es una garantía más fuerte que la del esquema: no hay que recorrer a los oponentes comprobando que su mano esté vacía, porque **el tipo ni siquiera puede contener una mano ajena**. Las pruebas se escribieron contra el tipo real.

## 5. Implementación

### 5.1 Flujo de una conexión y una acción

```mermaid
sequenceDiagram
    autonumber
    participant C as Cliente (navegador o websocat)
    participant P as reverse-proxy
    participant T as traefik
    participant G as game-logic (bololo_game)
    C->>P: wss://localhost/ws/games/game_1?token=JWT (TLS)
    P->>T: Upgrade WebSocket (red edge)
    T->>G: Upgrade (red internal)
    Note over G: verifyGameToken(JWT): firma RS256, exp, iat. gameID == ruta. slices.Contains(roster, playerID)
    alt token de otra partida / expirado / inválido
        G-->>C: 401 (no abre)
    else jugador fuera del roster
        G-->>C: 403 (no abre)
    else token válido del roster
        G-->>C: 101 Switching Protocols
        G-->>C: {"type":"view","view":{ you, hand (solo la propia), players:[{card_count}] }}
        C->>G: {"type":"draw","player":"OTRO","source":"hidden"}
        Note over G: Apply(c.player, msg): el player del mensaje se descarta; se usa el del token
        G-->>C: error not_your_turn (si no es su turno) o estado actualizado a nombre del dueño del token
    end
```

### 5.2 Archivos

| Archivo | Cambio |
|---|---|
| [`game-logic/internal/server/limited_view_test.go`](../../game-logic/internal/server/limited_view_test.go) (nuevo) | Tres pruebas: rechazo del handshake (token de otra partida, expirado, fuera del roster), identidad del token en una acción de camaleón, y camaleón público por diseño. Incluye el helper `signExpiredGameToken` |
| [`game-logic/internal/game/view_test.go`](../../game-logic/internal/game/view_test.go) | Prueba nueva `TestViewHidesOtherPlayersCards`: ninguna vista serializa la clave de cartas de un resumen ajeno |
| [`game-logic/docs/LIMITED_VIEW.md`](../../game-logic/docs/LIMITED_VIEW.md) (nuevo) | Documento del componente: dónde está el código, qué garantiza cada parte y la decisión de camaleón |
| [`game-logic/scripts/limited_view_demo.py`](../../game-logic/scripts/limited_view_demo.py) (nuevo) | Demo con clientes WebSocket reales a través del proxy, siguiendo el flujo legítimo (Auth → Lobby → inicio → token por jugador) |
| [`scripts/e2e.sh`](../../scripts/e2e.sh) | Sección nueva "Limited View (escenario 4)" que corre las pruebas Go del paquete relevante |
| [`scripts/esc4-evidence.sh`](../../scripts/esc4-evidence.sh) (nuevo) | Regenera la evidencia de §7 |

**Ningún archivo de lógica del juego cambió.** El escenario es de verificación: las garantías ya estaban en `view.go` y `websocket.go`; lo nuevo son pruebas, documentación y la demo.

### 5.3 Hallazgos durante la implementación

- **El esquema de la guía no compila contra el código real.** `view.Opponents` y `other.Hand` no existen. El tipo real (`PlayerView.Players []PlayerSummary`, con `PlayerSummary.CardCount` y sin cartas) da una garantía más fuerte, así que las pruebas se escribieron contra él.
- **Camaleón es público por diseño.** `chameleon.ViewFor` sí incluye las cartas de todos. Al principio parece una fuga, pero es la mecánica del modo (todos ven los colores sobre la mesa). En vez de "corregirlo", se documentó y se fijó con una prueba, para que nadie lo cambie por error.
- **La demo necesita el flujo legítimo completo.** Un token de partida solo lo emite el Lobby al iniciar, y uno por jugador. La demo hace `POST /auth/guest`, crea la sala, une a los jugadores, inicia y lee el token de cada uno; así prueba el camino real, no uno simulado.
- **En clásico la partida no arranca hasta tener los 4 jugadores.** Por eso la demo abre las 4 conexiones del modo clásico, y usa el modo camaleón (arranca con 2) para la parte de suplantación de identidad.

## 6. Qué cambia en la arquitectura

### 6.1 Antes y después

| Aspecto | Antes (`main`) | Después (`feature/p3-esc4`) |
|---|---|---|
| Vista por jugador en clásico | Implementada (`ViewFor`) | Igual, ahora con prueba directa de no fuga |
| Identidad de la acción | Del token (`Apply(c.player, …)`) | Igual, ahora probada también en camaleón |
| Handshake (token de otra partida, expirado, roster) | Implementado | Igual, ahora con prueba de extremo a extremo |
| Camaleón: manos públicas | Sin documentar ni probar | Documentado y fijado con una prueba |
| Demo | No había | Dos clientes WebSocket reales por el proxy |
| Lógica del juego | — | Sin cambios |

### 6.2 Cambios para las vistas

**Vista C&C.** No cambia la estructura: no hay componentes ni conectores nuevos. El escenario documenta una garantía del componente `bololo_game` sobre el conector **C3** (`WebSocket`), ya existente. Conviene anotar en la descripción de C3 que la identidad del actor se toma del token y que la vista es por jugador.

**Vista de despliegue.** No cambia: no hay contenedores, redes ni puertos nuevos.

Este es el único escenario de los cinco que no altera las vistas, porque trabaja dentro de un componente que ya existía.

## 7. Evidencia: antes y después

Todo se regenera con `./scripts/esc4-evidence.sh antes` (solo las pruebas que ya existían) y `./scripts/esc4-evidence.sh despues` (con las nuevas y la demo). Cada archivo muestra el comando exacto seguido de su salida.

| Prueba | Antes | Después | Archivos |
|---|---|---|---|
| `PlayerSummary` tiene campo de cartas | No (solo `CardCount`) | No (sin cambios): la fuga es imposible por tipo | [antes/01-codigo.txt](evidencia/esc4/antes/01-codigo.txt) |
| Pruebas de vista e identidad | `TestViewFor` pasa; el resto no existe | Todas PASS (ver abajo) | [despues/01-tests.txt](evidencia/esc4/despues/01-tests.txt) |
| Token de otra partida en el handshake | Sin prueba | `401` — `TestWebSocketRejectsTokensFromAnotherGameAndRoster` | despues/01-tests.txt |
| Token expirado en el handshake | Sin prueba de extremo a extremo | `401` — misma prueba | despues/01-tests.txt |
| Jugador fuera del roster | Sin prueba | `403` — misma prueba | despues/01-tests.txt |
| `playerId` falso en una acción (clásico) | `TestWebSocketStartsMatchAndSendsPrivateViews` | PASS (sin cambios) | despues/01-tests.txt |
| `playerId` falso en una acción (camaleón) | Sin prueba | PASS — `TestChameleonActionUsesTokenIdentityNotMessage` | despues/01-tests.txt |
| Mano ajena no serializada (clásico) | `TestViewFor` | PASS + `TestViewHidesOtherPlayersCards` | despues/01-tests.txt |
| Camaleón público por diseño | Sin prueba | PASS — `TestChameleonViewIsPublicByDesign` | despues/01-tests.txt |
| Demo con dos clientes WebSocket | No había | 8 OK, 0 fallas (cada `view` trae solo su mano; suplantación atribuida al token; handshakes `401`) | [despues/02-demo.txt](evidencia/esc4/despues/02-demo.txt) |

Resultado de `go test ./internal/game/ ./internal/server/`:

```
ok   github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/game
ok   github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/server
```

Las cuatro pruebas nuevas, con `-v`:

```
--- PASS: TestViewHidesOtherPlayersCards
--- PASS: TestWebSocketRejectsTokensFromAnotherGameAndRoster
--- PASS: TestChameleonActionUsesTokenIdentityNotMessage
--- PASS: TestChameleonViewIsPublicByDesign
```

## 8. Compromisos

### 8.1 Rendimiento

El escenario no agrega trabajo en caliente: la vista por jugador ya se construía en cada cambio de estado y la verificación del token ya ocurría en el handshake. No hay medición de latencia porque no cambia el camino de ejecución; lo nuevo son pruebas y una demo, que corren fuera de producción.

Construir una vista por conexión tiene un costo (una copia de la mano y un recorrido de los jugadores por cada mensaje `view`), pero es el que ya tenía el sistema y es la única forma de garantizar que la mano ajena no viaje.

### 8.2 Resumen

| Se gana | Cuesta |
|---|---|
| La mano ajena es imposible de filtrar en clásico: no existe en el tipo serializado (T1) | Construir una vista por jugador en cada cambio de estado (costo que ya existía) |
| Nadie puede actuar en nombre de otro: la identidad sale del token (T2) | El formato de mensaje incluye un campo `player` que el servidor ignora; puede confundir a quien lea el protocolo sin contexto |
| Token de otra partida, expirado o de un ajeno no abre la conexión (T3, T4, T5) | — |
| Las garantías quedan protegidas contra regresiones: un cambio que las rompa hace fallar una prueba | Mantener las pruebas al día si cambia el motor |
| La decisión de camaleón queda explícita y fijada (T7) | Hay que explicar que en camaleón las manos son públicas **a propósito**; a primera vista parece una fuga |
| Ningún código de lógica cambió; los escenarios 1, 2 y 3 siguen igual | — |

## 9. Riesgos residuales y trabajo futuro

| Riesgo o pendiente | Situación | Siguiente paso |
|---|---|---|
| Camaleón expone las manos de todos | Es por diseño del modo, no una fuga. Está documentado y probado | Si el enunciado del juego cambiara y las manos debieran ocultarse, sería un cambio de reglas: construir una vista por jugador también en `chameleon.ViewFor` |
| Todos los servicios comparten el mismo secreto de RabbitMQ | Fuera del alcance de este escenario (es el escenario alternativo que menciona la guía) | Secretos por servicio y usuarios de RabbitMQ con mínimos privilegios |
| La demo depende del stack completo arriba | Necesita Auth, Lobby y game-logic sanos | Documentado en §10; si falla, revisar `docker compose ps` |
| Un cambio futuro podría exponer una carta en un evento nuevo | `TestEventJSON` solo cubre `card_drawn` | Al agregar un evento, añadir su comprobación de que no trae cartas ocultas |

## 10. Cómo usarlo (equipo)

Las pruebas no necesitan el stack; la demo sí.

```powershell
# 1. Pruebas (no requieren Docker si hay Go local; si no, dentro de un contenedor):
cd game-logic
docker run --rm -v "${PWD}:/src" -w /src golang:1.27.2-bookworm `
  go test ./internal/game/ ./internal/server/

# 2. Solo las cuatro pruebas del escenario, con detalle:
docker run --rm -v "${PWD}:/src" -w /src golang:1.27.2-bookworm `
  go test -v -run "TestViewHidesOtherPlayersCards|TestWebSocketRejectsTokensFromAnotherGameAndRoster|TestChameleonActionUsesTokenIdentityNotMessage|TestChameleonViewIsPublicByDesign" `
  ./internal/game/ ./internal/server/

# 3. Demo con clientes WebSocket (desde la raíz del repo, con el stack arriba):
docker compose up -d --wait
docker run --rm --network bololo-integrado_public -e GATEWAY_HOST=reverse-proxy `
  -e PYTHONIOENCODING=utf-8 -e PYTHONUNBUFFERED=1 -v "${PWD}:/work" -w /work python:3.12-slim `
  sh -c "pip install -q websockets==15.0.1 && python game-logic/scripts/limited_view_demo.py"
```

| Problema | Causa | Solución |
|---|---|---|
| La demo no conecta | El stack no está arriba o el nombre de la red cambió | `docker compose ps`; ajustar `--network` y `GATEWAY_HOST` al nombre real del proxy |
| `go test` tarda mucho la primera vez | El contenedor descarga dependencias | Normal; corridas siguientes reutilizan la caché |
| Acentos raros en la salida de la demo | Codificación de la consola de Windows | Cosmético; no afecta el resultado |
| La demo falla en el paso de camaleón | El motor no emitió carta a tiempo | Reintentar; depende del repartidor aleatorio |

## 11. Guion de la demo (3 a 4 minutos)

**Sin la defensa (lo que se ve):**

1. (30 s) Levantar el stack y correr las cuatro pruebas con `-v`. Mostrar los cuatro `PASS`.
2. (60 s) Correr `limited_view_demo.py`. Señalar en la salida: cada jugador recibe un `view` con `you` = su id; la vista de Ana trae sus 3 cartas y de Beto solo `card_count`.
3. (45 s) En la salida de la demo, el paso 3: Beto manda un `draw` diciendo ser el jugador en turno y el servidor responde `not_your_turn`. El paso 5: Eva manda un `take` diciendo ser Fede y la carta queda a nombre de Eva.
4. (30 s) El bloque C: token de otra partida, token de sesión y token inválido, los tres `401`.

**Con la defensa (qué decir):**

- La táctica es **autorizar actores**: el servidor, no el cliente, decide qué ve y qué puede hacer cada jugador. El patrón es **Limited View** más identidad tomada del token.
- La garantía más fuerte es de **tipo**: `PlayerSummary` no tiene campo de cartas, así que la mano ajena no es que esté oculta, es que no existe en lo que se serializa.
- En camaleón las manos son públicas **a propósito**; lo demostramos y lo fijamos con una prueba para que nadie lo rompa.
- Ninguna línea de la lógica del juego cambió: el escenario verifica y blinda lo que ya estaba, y lo protege contra regresiones.

## 12. Referencias

- Bass, Clements, Kazman. *Software Architecture in Practice*, 3.ª ed. Tácticas de seguridad: *Resist attacks → Authorize actors*.
- Diapositivas del curso, *Architectural Patterns for Security*: Limited View.
- RFC 6455, *The WebSocket Protocol*.
- RFC 7519, *JSON Web Token (JWT)*.
- [`game-logic/docs/LIMITED_VIEW.md`](../../game-logic/docs/LIMITED_VIEW.md): documento del componente.
- Código: [`game-logic/internal/game/view.go`](../../game-logic/internal/game/view.go), [`internal/server/websocket.go`](../../game-logic/internal/server/websocket.go).
