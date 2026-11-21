# API Gateway (Traefik)

Enruta hacia los servicios las peticiones que ya pasaron por el [proxy inverso](../reverse-proxy/README.md). Desde el escenario 2 no publica puertos ni maneja TLS: solo recibe tráfico del proxy por la red `edge` en `traefik:8000`. Es el único componente con el socket de Docker, que usa para descubrir los servicios por sus labels, y ya no está expuesto a Internet.

| Archivo | Contenido |
|---|---|
| [`traefik.yml`](traefik.yml) | Configuración estática: entrypoint, provider de Docker y logs |
| [`certs/`](certs/) | Certificado y clave del borde. Los monta el proxy inverso, no Traefik. No se versionan; los genera `scripts/gen-certs.sh` |
| `ca/` | CA local de desarrollo. No se versiona ni se monta en ningún contenedor |

Las rutas van como labels en [`docker-compose.yml`](../docker-compose.yml):

| Ruta | Destino |
|---|---|
| `/` | `frontend` (SSR y assets) |
| `/auth/` | `auth` |
| `/api/` (REST y SSE de las salas) | `lobby-manager` |
| `/api/bots/` | `bot-service` |
| `/api/ratings/` | `ratings-service` |
| `/ws/games/` (WebSocket de la partida) | `game-logic` |

Un label nuevo no basta para publicar una ruta: también tiene que estar en la lista del proxy inverso.

## Entrypoints

| Entrypoint | Puerto | Uso |
|---|---|---|
| `web` | 8000, sin publicar | Todo el tráfico que llega desde el proxy. Es el entrypoint por defecto de los routers. Sin límites de tiempo de lectura ni de escritura, para no cortar el SSE ni el WebSocket |
| `traefik` | 8080, sin publicar | `/ping` para el healthcheck |

`forwardedHeaders.trustedIPs` solo incluye la IP fija del proxy (`10.250.0.2`). Por eso Traefik conserva el `X-Forwarded-Proto: https` que pone el proxy y los servicios generan redirecciones a `https://`. Si la cabecera llega desde cualquier otro origen, Traefik la reemplaza.

Diseño del escenario: [`docs/p3/esc2.md`](../docs/p3/esc2.md). El canal seguro está en el proxy: [`docs/p3/esc1.md`](../docs/p3/esc1.md).
