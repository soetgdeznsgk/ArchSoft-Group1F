# Estructura de descomposición

## Vista de descomposición

Diagrama: [descomposicion.drawio](descomposicion.drawio). Se abre en
<https://app.diagrams.net> (Archivo → Abrir desde → Dispositivo) y se exporta a
PNG o PDF desde ahí.

### Convención

| Elemento | Representación |
|---|---|
| Raíz | Rectángulo naranja: el sistema completo |
| Módulo | Rectángulo azul |
| Submódulo | Rectángulo morado |
| Funcionalidad | Rectángulo blanco |
| Es parte de | Línea sin flecha del hijo al padre |

## Propósito

Muestra cómo se divide Bololó en módulos funcionales, desde las capacidades
generales del sistema hasta las funcionalidades concretas de cada módulo. Cada
módulo agrupa funcionalidades con una misma responsabilidad. La única relación de
la vista es **es parte de**.

El API Gateway (Traefik) no aparece como módulo porque no agrupa funcionalidades
del sistema: solo enruta peticiones y cifra el canal. Se describe en las vistas
de componentes y conectores y de despliegue.

## Jerarquía de descomposición

1. **auth**
   - Crear sesión de invitado con un nickname
   - Registrar cuenta con usuario y contraseña
   - Iniciar sesión con una cuenta
   - Cambiar la contraseña de la cuenta
   - Bloquear la cuenta tras 5 intentos fallidos
   - Emitir el token de sesión firmado (RS256)
2. **lobby**
   - Registrar al jugador a partir de su sesión
   - Crear sala eligiendo el modo (Clásico o Camaleón)
   - Listar las salas disponibles
   - Entrar a una sala con su código
   - Salir de la sala
   - Actualizar la sala de espera en tiempo real (SSE)
   - Iniciar la partida y emitir un token por jugador
   - Registrar el resultado una sola vez y cerrar la sala
   - Cerrar las salas cuya partida no reportó resultado
3. **game**
   - **classic** (submódulo)
     - Repartir las cartas de cada una de las 5 rondas
     - Tomar carta del mazo oculto o del descarte
     - Botar una carta
     - Robar la carta descartada fuera de turno
     - Bajar las jugadas de la ronda (tríos y cuartetos)
     - Depositar cartas en los bancos de la mesa
     - Controlar el turno de 30 s con intercambio automático
     - Calcular los puntos y el ganador de la partida
   - **chameleon** (submódulo)
     - Tirar cartas en posiciones aleatorias de la mesa
     - Tomar una carta de la mesa (gana el primero)
     - Penalizar un color repetido (pierde ambas cartas)
     - Penalizar la carta camaleón (pierde toda la mano)
     - Terminar al reunir los 8 colores o al acabarse el tiempo
   - **match** (submódulo)
     - Validar el token de partida al abrir el WebSocket
     - Enviar a cada jugador solo su vista (Limited View)
     - Difundir los eventos públicos de la partida
     - Guardar y restaurar las partidas activas (Redis)
     - Publicar el resultado (`game.ended`) al terminar
4. **bots**
   - Llenar una sala con bots
   - Quitar los bots de una sala
   - Consultar los bots de una sala
   - Jugar la partida completa como un jugador más
   - Decidir la jugada a partir de la vista recibida
5. **ratings**
   - Recibir las partidas terminadas (`game.finished`)
   - Calcular el puntaje Elo por modo
   - Procesar cada partida una sola vez
   - Consultar el ranking por modo
   - Consultar el puntaje de un jugador
   - Consultar el historial de puntaje de un jugador
6. **ui**
   - Mostrar el menú con el ranking renderizado en el servidor (SSR)
   - Mostrar la pantalla de ingreso y salas
   - Mostrar la mesa del modo clásico
   - Mostrar la mesa del modo camaleón

## Descripción de elementos y relaciones arquitectónicas

| Elemento | Tipo | Descripción | Relaciones |
|---|---|---|---|
| Bololó | Raíz | Juego de cartas multijugador en línea con dos modos, Clásico y Camaleón. | Todos los módulos son parte de él. |
| auth | Módulo | Gestiona la identidad de los jugadores: sesiones de invitado, cuentas, contraseñas y bloqueo por intentos fallidos. Emite todos los tokens de sesión. | Es parte de Bololó. lobby verifica las sesiones que emite y bots las usa para entrar como jugadores. |
| lobby | Módulo | Gestiona las salas desde que el jugador entra hasta que la partida termina: crear, listar, entrar, salir, sala de espera en tiempo real, inicio y registro del resultado. | Es parte de Bololó. Usa las sesiones de auth, pide a game que cree la partida y recibe su resultado. Entrega a ratings las partidas terminadas. |
| game | Módulo | Ejecuta las partidas en curso de forma autoritativa: valida cada acción contra el estado del juego. | Es parte de Bololó. Contiene a classic, chameleon y match. lobby le crea las partidas y él le reporta el resultado. |
| classic | Submódulo | Reglas del modo Clásico: 5 rondas de tríos y cuartetos con 4 jugadores por turnos. | Es parte de game. match lo usa a través de una interfaz común de motor. |
| chameleon | Submódulo | Reglas del modo Camaleón: en tiempo real, de 2 a 4 jugadores compiten por reunir los 8 colores. | Es parte de game. match lo usa a través de la misma interfaz que classic. |
| match | Submódulo | Lo común a los dos modos: conexión WebSocket, autorización del jugador con su token, vistas limitadas, persistencia y aviso del resultado. | Es parte de game. Usa a classic y chameleon sin conocer sus reglas, así que agregar un modo es agregar un submódulo hermano. |
| bots | Módulo | Completa salas con bots que juegan la partida completa como jugadores normales. | Es parte de Bololó. Usa a auth para sus sesiones, a lobby para entrar a la sala y a game para jugar. |
| ratings | Módulo | Calcula y expone el ranking Elo por modo de los jugadores con cuenta. | Es parte de Bololó. Recibe de lobby las partidas terminadas. ui consulta el ranking para el menú. |
| ui | Módulo | Presentación que ve el jugador en el navegador: menú con ranking, ingreso y salas, y las mesas de cada modo. | Es parte de Bololó. Usa a auth, lobby y game para el flujo del jugador, y a ratings para el ranking. |

Cada funcionalidad es parte de un solo módulo o submódulo, y cada submódulo es
parte de un solo módulo, así que la descomposición no tiene solapamientos.

## Criterios de la descomposición

- **Por responsabilidad del dominio**: identidad, salas, partida, bots, ranking y
  presentación. Cada módulo tiene una razón de cambio distinta.
- **Separar reglas de infraestructura en game**: las reglas de cada modo (classic,
  chameleon) no dependen de la red ni de la persistencia, que se concentran en
  match. Así las reglas se prueban sin red.
- **Solo módulos que agrupan funcionalidades**: los elementos que no ofrecen
  funcionalidades propias, como el API Gateway, quedan fuera de esta vista.
