# Caso 5 · Varias partidas simultáneas

| Pregunta | Respuesta corta |
|---|---|
| ¿Qué se probó? | Flujo completo de partidas `CLASSIC` con anfitriones humanos y tres bots |
| ¿Con qué? | k6, HTTP, WebSocket, Auth, Lobby, Bot Service, Game Logic, Redis y RabbitMQ |
| ¿Capacidad conocida? | Hasta 10 salas activas simultáneas, 40 jugadores |
| ¿Estado? | Barrido grueso completado: 1, 2, 4, 6, 8 y 10 partidas |

## 1. Objetivo

Medir el desempeño del sistema con varias partidas abiertas al mismo tiempo. El límite reglamentario es 10 salas activas; una partida clásica tiene exactamente 4 jugadores.

## 2. Método

Por cada nivel de 1, 2, 4, 6, 8 y 10 partidas:

1. Crear invitado y sala.
2. Añadir tres bots.
3. Iniciar la partida clásica.
4. Mantener una conexión WebSocket humana.
5. Esperar `game_ended`.
6. Comprobar el resultado en Lobby y Ratings.
7. Registrar recursos y fallos.

## 3. Datos crudos

Los scripts y resultados se conservan en [`perf/caso5-partidas/`](../../../perf/caso5-partidas/).

## 4. Resultados del barrido

| Partidas | Terminadas | Timeout | Duración promedio |
|---|---:|---:|---:|
| 1 | 1 | 0 | 522 s |
| 2 | 2 | 0 | 598 s |
| 4 | 3 | 1 | 569 s |
| 6 | 4 | 2 | 567 s |
| 8 | 7 | 1 | 546 s |
| 10 | 5 | 5 | 574 s |

La degradación aparece desde 4 partidas: una de ellas excedió el timeout. Con 6 partidas el 33 % excedió el límite y con 10 partidas el 50 %.

La preparación subió de aproximadamente 384–393 ms para una partida a 2268 ms para diez partidas. El tiempo HTTP promedio subió de aproximadamente 81–85 ms a 528 ms.

## 5. Observaciones

- Los 10 resultados quedaron registrados al terminar la corrida de 10 partidas.
- Aparecieron advertencias `websocket consumer is too slow` en `game-logic`.
- El nivel de 10 partidas tuvo picos elevados de CPU en Auth, RabbitMQ, Lobby, Traefik y PostgreSQL durante la creación.
- Redis y `ratings-postgres` tuvieron cargas menores en esta corrida.

## 6. Conclusión preliminar

El límite reglamentario de 10 partidas no fue estable: solo 5 de las 10 terminaron dentro del timeout de 660 s. La creación fue exitosa, pero el desempeño de las partidas terminó degradado.

Esta es una sola corrida por nivel. Las repeticiones de 8 y 10 partidas son necesarias antes de definir una rodilla estadística.

## 7. Gráficas

![Finalización por partida](../../../perf/caso5-partidas/img/caso5-finalizacion.png)

![Duración de las partidas](../../../perf/caso5-partidas/img/caso5-duracion.png)

![Preparación y HTTP](../../../perf/caso5-partidas/img/caso5-preparacion.png)
