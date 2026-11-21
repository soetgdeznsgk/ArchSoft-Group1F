# Reverse Proxy (Nginx)

Único punto de entrada desde Internet ([escenario 2](../docs/p3/esc2.md)). Es el único contenedor con puertos publicados (80 y 443). Recibe al navegador, termina el TLS y reenvía al API Gateway (`traefik:8000`) solo las rutas públicas. No conoce los servicios ni tiene privilegios sobre Docker.

| Archivo | Contenido |
|---|---|
| [`nginx.conf`](nginx.conf) | Configuración global: límites de tamaño y de tiempo, `server_tokens off` y log JSON sin *query string* |
| [`templates/bololo.conf.template`](templates/bololo.conf.template) | Servidores: redirección del 80, canal seguro en el 443, lista de rutas permitidas y salud interna |
| [`proxy-headers.conf`](proxy-headers.conf) | Cabeceras que se envían al gateway; se incluye en cada ruta |

La imagen oficial (`nginx:1.30.5-alpine`) convierte la plantilla en `/etc/nginx/conf.d/bololo.conf` al arrancar. Solo sustituye `HSTS_MAX_AGE` (`NGINX_ENVSUBST_FILTER`); las variables de Nginx, como `$host`, quedan intactas.

## Rutas permitidas

| Ruta | Respuesta |
|---|---|
| `/` (exacta) | Gateway → `frontend`, que redirige a `/menu/` |
| `/menu`, `/ingame`, `/chameleon` | `301` a la misma ruta con barra final |
| `/menu/`, `/ingame/`, `/chameleon/`, `/libraries/` | Gateway → `frontend` |
| `/auth/` | Gateway → `auth` |
| `/api/` (REST y SSE) | Gateway → `lobby-manager`, `bot-service` o `ratings-service` según el prefijo |
| `/ws/games/` (WebSocket) | Gateway → `game-logic`. Única ruta que reenvía `Upgrade` |
| Cualquier otra | `404` con `{"error":"not_found"}`, sin llegar al gateway |

La ruta se compara después de que Nginx decodifica y normaliza la URL, así que `/api/../internal/...` o `/api/%2e%2e/internal/...` caen en la última fila.

Agregar una ruta pública exige dos cambios: el label del servicio en `docker-compose.yml` (para que el gateway sepa a dónde va) y una `location` aquí (para que llegue desde Internet). Un label olvidado ya no publica nada por sí solo.

## Canal seguro (escenario 1)

- **Puerto 443:** TLS 1.2 y 1.3, solo suites ECDHE con AES-GCM o ChaCha20-Poly1305, HTTP/2 por ALPN y HSTS (`max-age=${HSTS_MAX_AGE}`).
- **Cabeceras seguras en todas las respuestas del 443**, incluidos los `404` y `413` del proxy: `X-Content-Type-Options: nosniff` (el navegador no adivina el tipo de un archivo), `X-Frame-Options: DENY` (ninguna página puede meter a Bololó en un `iframe`, contra *clickjacking*) y `Referrer-Policy: no-referrer` (la URL, que puede llevar `?token=`, no se envía a otros sitios). Si un servicio envía las suyas, el proxy las reemplaza.
- **Puerto 80:** no sirve contenido. Responde `301` a `https://` para GET y HEAD, y `308` para el resto, que conserva el método y el cuerpo.
- **Certificado:** se monta desde `api-gateway/certs/` como `/certs`, solo lectura. Si falta, Nginx no arranca.

## Límites

| Límite | Valor | Motivo |
|---|---|---|
| Cuerpo de la petición | 64 KB (`413` si se pasa) | Las peticiones de la API son JSON de pocos cientos de bytes |
| Cabeceras | 1 KB por defecto, hasta 4 × 8 KB | Caben el JWT en `Authorization` y en `?token=` |
| Recibir cabeceras y cuerpo | 10 s | Corta clientes que envían muy despacio |
| Respuesta del gateway | 30 s; 75 s en `/api/` y `/ws/games/` | El SSE manda un *ping* cada 15 s y el WebSocket cada 25 s |
| Conexiones por worker | 4096 (`worker_rlimit_nofile` y `ulimits.nofile` en 16384), con `reuseport` en 80 y 443 | Cada petición en curso ocupa dos conexiones (cliente y gateway). Con 1024, el Lab 6 falló en el proxy con 2000 usuarios; `reuseport` reparte las conexiones parejo entre los workers |

## Aislamiento

- **Redes:** está en `public`, donde publica sus puertos, y en `edge` (`10.250.0.0/28`), que solo comparte con el gateway. No está en `internal`, así que no resuelve ni alcanza los servicios ni las bases de datos.
- **Cabeceras reenviadas:** `X-Forwarded-For` se reemplaza con la IP real del cliente en lugar de agregarse, para que un cliente no pueda inventarla. `X-Forwarded-Proto` siempre es `https`. Traefik solo confía en esas cabeceras si vienen de la IP fija del proxy (`10.250.0.2`).
- **Contenedor:**
  - disco de solo lectura (`read_only`), con `tmpfs` donde Nginx necesita escribir;
  - todas las *capabilities* quitadas menos las cinco que necesita para arrancar;
  - `no-new-privileges`.
- **Sin el socket de Docker ni las claves de los JWT.**

## Logs

- **Acceso:** JSON por stdout, con la ruta sin *query string*. Así el `?token=` del SSE y del WebSocket nunca queda escrito.
- **Errores:** en nivel `crit`. En los niveles inferiores Nginx copia la línea de la petición completa, con el token. Las fallas del gateway se siguen viendo en el log de acceso (`upstream_status`).

## Salud

`/healthz` solo escucha en `127.0.0.1:8080` dentro del contenedor. El healthcheck de Compose lo consulta con `wget`. No se publica.
