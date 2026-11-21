# Vista de descomposición — Bololó

Diagrama: [descomposicion.drawio](descomposicion.drawio). Se abre en
<https://app.diagrams.net> (Archivo → Abrir desde → Dispositivo) o con la
extensión *Draw.io Integration* de VS Code/Kiro. Desde ahí se exporta a PNG o PDF.

Es una vista de módulos con un solo tipo de relación, **es parte de**: cada
funcionalidad es parte de un módulo o submódulo, y cada módulo es parte del
sistema. No muestra cómo se comunican los módulos en ejecución; eso lo cubre la
vista de componentes y conectores (C&C) del [README](../README.md).

## Convención

| Elemento | Representación |
|---|---|
| Raíz | Rectángulo naranja: el sistema completo |
| Módulo | Rectángulo azul: unidad de implementación con una responsabilidad |
| Submódulo | Rectángulo morado: parte de un módulo con responsabilidad propia |
| Funcionalidad | Rectángulo blanco: capacidad que ofrece el módulo |
| Es parte de | Línea sin flecha del hijo al padre |

## Elementos

### Raíz

**Bololó**: juego de cartas multijugador en línea con dos modos, Clásico y
Camaleón, construido como sistema distribuido.

### Módulos

1. **BL_auth**: todas las funcionalidades de identidad. Emite las sesiones de
   invitados y de cuentas, y gestiona las credenciales (`auth-service/`, Java +
   Spring Boot).
2. **BL_lobby**: todas las funcionalidades de las salas, desde que el jugador
   entra hasta que la partida termina y se registra su resultado
   (`lobby-manager/`, FastAPI + PostgreSQL).
3. **BL_game**: todas las funcionalidades de la partida en curso. El servidor es
   autoritativo: valida cada acción contra el estado (`game-logic/`, Go + Redis).
   Tiene tres submódulos:
   - **BL_classic**: reglas del modo Clásico, 5 rondas de tríos y cuartetos con 4
     jugadores por turnos (`internal/game`).
   - **BL_chameleon**: reglas del modo Camaleón, en tiempo real con 2 a 4
     jugadores que compiten por reunir los 8 colores (`internal/chameleon`).
   - **BL_match**: lo común a los dos modos. Conexión WebSocket, autorización
     del jugador, vistas, persistencia y aviso del resultado (`internal/server`,
     `internal/store`, `internal/broker`).
4. **BL_bots**: funcionalidades de los bots que completan salas y juegan solos.
   Para el resto del sistema son jugadores normales (`bot-service/`, Elixir/OTP).
5. **BL_ratings**: funcionalidades del ranking de jugadores con cuenta, calculado
   a partir de las partidas terminadas (`ratings-service/`, FastAPI + PostgreSQL).
6. **BL_ui**: funcionalidades de presentación que ve el jugador en el navegador
   (`presentation-ssr/` y `presentation/`, Node.js + p5.js).
7. **BL_gateway**: funcionalidades del punto único de entrada al sistema
   (`api-gateway/`, Traefik).

### Funcionalidades por módulo

| Módulo / submódulo | Funcionalidades |
|---|---|
| BL_auth | Crear sesión de invitado con un nickname · Registrar cuenta con usuario y contraseña · Iniciar sesión con una cuenta · Cambiar la contraseña · Bloquear la cuenta tras 5 intentos fallidos · Emitir el token de sesión firmado (RS256) |
| BL_lobby | Registrar al jugador a partir de su sesión · Crear sala eligiendo el modo · Listar las salas disponibles · Entrar a una sala con su código · Salir de la sala · Actualizar la sala de espera en tiempo real (SSE) · Iniciar la partida y emitir un token por jugador · Registrar el resultado una sola vez y cerrar la sala · Cerrar las salas cuya partida no reportó resultado |
| BL_classic | Repartir las cartas de cada una de las 5 rondas · Tomar carta del mazo oculto o del descarte · Botar una carta · Robar la carta descartada fuera de turno · Bajar las jugadas de la ronda · Depositar cartas en los bancos · Controlar el turno de 30 s con intercambio automático · Calcular los puntos y el ganador |
| BL_chameleon | Tirar cartas en posiciones aleatorias de la mesa · Tomar una carta de la mesa (gana el primero) · Penalizar un color repetido · Penalizar la carta camaleón · Terminar al reunir los 8 colores o al acabarse el tiempo |
| BL_match | Validar el token de partida al abrir el WebSocket · Enviar a cada jugador solo su vista (Limited View) · Difundir los eventos públicos · Guardar y restaurar las partidas activas (Redis) · Publicar el resultado (`game.ended`) |
| BL_bots | Llenar una sala con bots · Quitar los bots de una sala · Consultar los bots de una sala · Jugar la partida completa como un jugador más · Decidir la jugada a partir de la vista recibida |
| BL_ratings | Recibir las partidas terminadas (`game.finished`) · Calcular el puntaje Elo por modo · Procesar cada partida una sola vez · Consultar el ranking por modo · Consultar el puntaje de un jugador · Consultar su historial de puntaje |
| BL_ui | Mostrar el menú con el ranking renderizado en el servidor (SSR) · Mostrar la pantalla de ingreso y salas · Mostrar la mesa del modo clásico · Mostrar la mesa del modo camaleón |
| BL_gateway | Enrutar cada petición al servicio según su ruta · Cifrar el canal con TLS 1.2/1.3 y HSTS · Redirigir HTTP a HTTPS |

## Relaciones

- **Es parte de** (única relación de la vista). Cada funcionalidad pertenece a un
  solo módulo o submódulo, y cada submódulo a un solo módulo. La descomposición
  no tiene solapamientos: ninguna funcionalidad aparece en dos lugares.
- BL_classic, BL_chameleon y BL_match son parte de BL_game. Cada modo de juego es
  un submódulo intercambiable: BL_match no conoce las reglas y habla con cada
  modo a través de una misma interfaz (`engine`), así que agregar un modo nuevo
  es agregar un submódulo hermano.
- Cada módulo de primer nivel se construye y despliega como un contenedor
  independiente, y cada base de datos pertenece a un solo módulo (BL_auth,
  BL_lobby y BL_ratings tienen su PostgreSQL; BL_game tiene Redis).

## Criterios de la descomposición

- **Por responsabilidad del dominio**: identidad, salas, partida, bots, ranking,
  presentación y entrada. Cada módulo tiene una razón de cambio distinta.
- **Separar reglas de infraestructura en BL_game**: las reglas de cada modo
  (BL_classic, BL_chameleon) no dependen de la red ni de la persistencia; eso lo
  concentra BL_match. Así las reglas se prueban sin red.
- **Seguridad en el borde y en el servidor de juego**: BL_gateway cifra el canal
  y BL_match autoriza a cada jugador con su token, de modo que el cliente nunca
  decide qué ve ni a nombre de quién actúa.
