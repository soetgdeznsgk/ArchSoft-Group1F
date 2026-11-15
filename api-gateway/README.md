# API Gateway (Traefik)

Punto de entrada único desde el navegador. Es el único contenedor con puertos publicados.

| Archivo | Contenido |
|---|---|
| [`traefik.yml`](traefik.yml) | Configuración estática: entrypoints, providers, logs |
| [`dynamic/`](dynamic/) | Configuración dinámica que se carga desde archivos (se recarga sola). [`secure-channel.yml`](dynamic/secure-channel.yml): certificado, política TLS y HSTS |
| [`certs/`](certs/) | Certificado y clave del gateway. No se versionan; los genera `scripts/gen-certs.sh` |
| `ca/` | CA local de desarrollo. No se versiona ni se monta en el contenedor |

Las rutas van como labels en [`docker-compose.yml`](../docker-compose.yml):

| Ruta | Destino |
|---|---|
| `/` | `frontend` (SSR y assets) |
| `/auth/` | `auth` |
| `/api/` (REST y SSE de las salas) | `lobby-manager` |
| `/api/bots/` | `bot-service` |
| `/api/ratings/` | `ratings-service` |
| `/ws/games/` (WebSocket de la partida) | `game-logic` |

## Entrypoints

| Entrypoint | Puerto | Uso |
|---|---|---|
| `websecure` | 443 | Todo el tráfico de la aplicación. TLS 1.2/1.3 (ECDHE + AEAD), HTTP/2 y HSTS. Es el entrypoint por defecto de los routers |
| `web` | 80 | Solo redirige a `https://` con la misma ruta (`301` para GET, `308` para el resto). No llega a ningún servicio |
| `traefik` | 8080, no publicado | `/ping` para el healthcheck |

Antes del primer `docker compose up` hay que generar el certificado: `./scripts/gen-certs.sh --trust`. Sin él Traefik no arranca. Diseño, amenazas y evidencia: [`docs/p3/esc1.md`](../docs/p3/esc1.md).

Para agregar un middleware a todas las rutas (por ejemplo, rate limiting), se define en un archivo nuevo de `dynamic/` y se agrega a `entryPoints.websecure.http.middlewares` en `traefik.yml`.
