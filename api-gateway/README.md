# API Gateway (Traefik)

Punto de entrada único desde el navegador. Aún no implementado.

Rutas previstas, según la vista C&C:

| Tráfico | Destino |
|---|---|
| REST y SSE (salas) | `lobby-manager` |
| WebSocket (partida) | `game-logic` |

La configuración estática y dinámica de Traefik va en esta carpeta.
