# BOLoló — Frontend de la mesa de juego

## 1. Descripción

Este módulo corresponde al frontend de la mesa de juego del proyecto BOLoló, desarrollado como parte del repositorio grupal `ArchSoft-Group1F`.

Su objetivo es proporcionar la interfaz gráfica de la partida, representar las cartas y los jugadores, gestionar las interacciones del usuario y preparar la comunicación en tiempo real con el servidor del juego.

El desarrollo corresponde al **rol 4: Frontend, mesa de juego**.

## 2. Alcance del desarrollo

La implementación inicial proporciona una estructura para trabajar en la interfaz de la partida utilizando HTML, CSS, JavaScript y p5.js.

El frontend se desarrolla de forma independiente del motor de reglas en Go para permitir pruebas locales mientras el backend se encuentra en desarrollo.

El módulo debe evolucionar para cumplir los requisitos asignados:

- Canvas con proporción 16:9 y coordenadas normalizadas.
- Representación de los nueve tipos de carta definidos por el proyecto.
- Medición del tiempo de reacción mediante `performance.now()`.
- Cliente WebSocket para intercambiar mensajes con el servidor.
- Paneles para representar las manos de los jugadores.
- Servidor WebSocket de prueba para validar la comunicación sin depender del motor Go.

**Estado:** prototipo en desarrollo. La presencia de un archivo o componente no significa que todas las funcionalidades estén implementadas o probadas.

## 3. Tecnologías

- HTML5: estructura de la interfaz.
- CSS3: presentación visual.
- JavaScript: lógica de interacción del cliente.
- p5.js: dibujo de la mesa y los elementos gráficos mediante Canvas.
- WebSocket: comunicación bidireccional en tiempo real.
- Git y GitHub: control de versiones y colaboración.

## 4. Estructura del módulo

```text
presentation/
└── frontend/
    ├── index.html
    ├── style.css
    ├── README.md
    ├── client/
    │   └── ingame/
    │       ├── sketch.js
    │       └── game-socket.js
    ├── assets/
    │   └── cards/
    └── libraries/
        ├── p5.min.js
        └── p5.sound.min.js
```

La estructura anterior describe los archivos previstos para el módulo. Se debe verificar la ubicación real de los recursos y las dependencias en cada entorno.

### Responsabilidad de los archivos

- `index.html`: punto de entrada de la interfaz.
- `style.css`: estilos de la página y los componentes.
- `client/ingame/sketch.js`: representación gráfica e interacciones de la mesa.
- `client/ingame/game-socket.js`: módulo de comunicación WebSocket.
- `assets/cards/`: recursos gráficos de las cartas, cuando se utilicen.
- `libraries/`: bibliotecas locales, incluida p5.js si se carga desde esta carpeta.

## 5. Ejecución local

### Requisitos

- Visual Studio Code o un editor equivalente.
- Un navegador web moderno.
- La extensión Live Server, o un servidor HTTP estático equivalente.

### Pasos

1. Clonar el repositorio del grupo o abrir la copia local existente.
2. Abrir el repositorio en Visual Studio Code.
3. Navegar hasta `presentation/frontend/index.html`.
4. Ejecutar el archivo mediante Live Server.
5. Abrir en el navegador la dirección local proporcionada por la extensión.
6. Comprobar que la página carga correctamente y revisar la consola del navegador si aparecen errores.

La ruta de ejecución puede cambiar si se modifica la estructura del proyecto. No se requiere el servidor Go para abrir una versión del frontend que funcione en modo local.

## 6. Arquitectura y responsabilidades

El frontend es responsable de representar la información recibida y registrar las acciones del usuario.

El backend Go es responsable de la lógica autoritativa de la partida, las validaciones, la resolución de las reclamaciones, los tiempos y el estado compartido del juego.

La interfaz no debe asumir que una acción fue aceptada hasta recibir la respuesta correspondiente del servidor.

Para facilitar el desarrollo, el cliente debe poder probarse contra un servidor WebSocket simulado antes de conectarse al backend real.

## 7. Integración WebSocket

### Objetivo

El módulo `client/ingame/game-socket.js` está destinado a encapsular la conexión WebSocket para separar la comunicación de la representación gráfica de la mesa.

Antes de conectar el frontend al servidor Go, el equipo debe acordar el contrato de mensajes y la dirección de conexión.

### Configuración de la conexión

La URL debe ser configurable según el entorno. Por ejemplo, un servidor local de pruebas podría utilizar una dirección como:

```text
ws://localhost:8081
```

Esta dirección es solamente un ejemplo; debe sustituirse por la dirección y la ruta reales del servidor.

En un despliegue HTTPS, normalmente se utilizará `wss://`.

### Flujo esperado

1. El cliente abre la conexión WebSocket.
2. El servidor comunica o confirma el estado inicial de la partida.
3. El cliente representa la información recibida.
4. El usuario realiza una acción sobre la mesa.
5. El cliente envía un mensaje con la acción y los datos acordados.
6. El servidor valida y procesa la acción.
7. El servidor comunica el resultado y las actualizaciones correspondientes.
8. El cliente actualiza la interfaz.

### Contrato de mensajes

El formato definitivo debe acordarse con el responsable del motor Go.

La tabla siguiente sirve como lista de eventos que deben definirse; no representa nombres de mensajes ya implementados.

| Evento o propósito | Dirección | Información por acordar |
|---|---|---|
| Conexión a la partida | Cliente → servidor | Identificador de partida y autenticación, si aplica |
| Estado inicial | Servidor → cliente | Jugadores, cartas visibles y estado de la partida |
| Aparición de carta | Servidor → cliente | Tipo de carta y posición normalizada |
| Reacción del jugador | Cliente → servidor | Carta o evento objetivo y tiempo de reacción |
| Resultado de la reclamación | Servidor → cliente | Resultado de la acción y actualización de estado |
| Actualización de manos | Servidor → cliente | Datos necesarios para representar las manos |
| Error o acción rechazada | Servidor → cliente | Código de error y motivo |
| Fin de partida o ronda | Servidor → cliente | Resultado y estado final |

Los nombres de los mensajes, sus campos, los identificadores y los tipos de datos deben definirse en el contrato compartido. No se deben asumir nombres de eventos ni formatos que el backend todavía no haya confirmado.

## 8. Medición del tiempo de reacción

El requisito del frontend establece utilizar `performance.now()` para medir el tiempo de reacción.

La medición debe basarse en una referencia temporal monotónica, por ejemplo:

```javascript
const startedAt = performance.now();

// Al producirse la reacción del usuario:
const reactionMs = performance.now() - startedAt;
```

Este fragmento es ilustrativo. La implementación definitiva debe definir cuándo comienza la medición, qué evento cuenta como reacción válida y cómo se comunica el resultado al servidor.

El servidor debe determinar la validez de la reclamación de acuerdo con las reglas oficiales.

## 9. Servidor WebSocket de prueba

Se debe disponer de un servidor WebSocket de prueba que permita validar el cliente sin depender del motor Go.

Este servidor deberá permitir, como mínimo:

- Aceptar conexiones del cliente.
- Enviar mensajes de prueba con cartas y estados.
- Recibir las reacciones del usuario.
- Simular respuestas aceptadas y rechazadas.
- Verificar el comportamiento ante mensajes inválidos y desconexiones.

El servidor simulado es una herramienta de desarrollo y no sustituye el backend real.

## 10. Pruebas pendientes

La entrega debe incluir pruebas para comprobar:

- Carga de la interfaz.
- Adaptación del Canvas a la proporción 16:9.
- Representación de los nueve tipos de carta.
- Registro del tiempo de reacción.
- Conexión y desconexión WebSocket.
- Recepción y envío de mensajes.
- Manejo de mensajes inválidos.
- Actualización de la mesa y de las manos.
- Comportamiento cuando el servidor no está disponible.

Los resultados de las pruebas deben documentarse y no marcarse como satisfactorios hasta ejecutarlas.

## 11. Documentación complementaria

La documentación específica de la implementación debe incluirse también en `docs/`, según la distribución de responsabilidades del equipo.

Se deben documentar la arquitectura del cliente, el contrato de mensajes, las instrucciones de ejecución, las pruebas realizadas y las diferencias entre el servidor simulado y el backend real.

La entrega debe incluir el `Dockerfile` solicitado para este rol, coordinando su configuración con el equipo de integración y despliegue.

## 12. Estado de la integración

La integración con el backend Go se considera pendiente hasta que el cliente se conecte al servidor real, utilice el contrato acordado y se verifique el intercambio de mensajes.

El servidor WebSocket de prueba permitirá avanzar en el desarrollo y las pruebas del frontend de manera independiente.

## 13. Control de versiones

Los cambios se realizan en la rama de trabajo del frontend y se publican mediante commits descriptivos.

La integración a `main` se realiza a través del proceso de revisión y Pull Request acordado por el equipo.
