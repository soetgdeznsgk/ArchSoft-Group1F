# Escenario 3 · Network Segmentation

| | |
|---|---|
| Atributo de calidad | Seguridad: confidencialidad de los datos y limitación del movimiento lateral |
| Tácticas | **Separar entidades** · **Limitar el acceso** |
| Patrón | **Network Segmentation** |
| Estado | Implementado; validación en §7 |
| Rama | `feature/mis-cambios` |
| Responsable | ______________________ |

## Contenido

1. [Estado antes del cambio](#1-estado-antes-del-cambio)
2. [Amenazas identificadas](#2-amenazas-identificadas)
3. [Escenario de calidad](#3-escenario-de-calidad)
4. [Decisión](#4-decisión)
5. [Implementación](#5-implementación)
6. [Cambios en la arquitectura](#6-cambios-en-la-arquitectura)
7. [Evidencia](#7-evidencia)
8. [Compromisos](#8-compromisos)
9. [Riesgos residuales](#9-riesgos-residuales)
10. [Cómo usarlo](#10-cómo-usarlo)
11. [Guion de demo](#11-guion-de-demo)
12. [Referencias](#12-referencias)

---

## 1. Estado antes del cambio

Traefik, la aplicación, PostgreSQL, Redis y RabbitMQ compartían la red Docker `internal`. Por eso, un contenedor comprometido podía resolver y abrir conexiones hacia almacenes y servicios que no necesitaba. `lobby-manager` y `ratings-service` también aceptaban `X-Forwarded-*` desde cualquier IP.

Las redes `public` y `edge` del escenario 2 se conservan: `reverse-proxy` sigue publicando 80/443 en `public`; `reverse-proxy` (`10.250.0.2`) y Traefik (`10.250.0.3`) siguen en `edge` (`10.250.0.0/28`).

## 2. Amenazas identificadas

Modelo de atacante: logra ejecutar código dentro de un contenedor de aplicación, por ejemplo `frontend`, sin controlar Docker Engine ni el host.

| Amenaza | Consecuencia |
|---|---|
| `frontend` resuelve `postgres`, `redis` o `rabbitmq` | Reconocimiento y acceso de red lateral a almacenes/broker |
| Servicio sin necesidad de eventos alcanza RabbitMQ | Interacción innecesaria con el broker |
| Contenedor interno suplanta `X-Forwarded-*` | IP o esquema de la petición falsificados ante Uvicorn |
| Traefik elige una red diferente para el backend | La ruta del gateway falla con 504 |

## 3. Escenario de calidad

| Elemento | Definición |
|---|---|
| Fuente | Atacante con ejecución de código en un contenedor de `services` |
| Estímulo | Intenta resolver y conectarse a una base de datos, Redis o RabbitMQ |
| Artefacto | Bases de datos y broker |
| Entorno | Operación normal |
| Respuesta | El contenedor no resuelve los nombres de servicios que no comparten una red |
| Medida | Cero conexiones exitosas desde un servicio no autorizado a datos/broker |

## 4. Decisión

Se eligió una red común para el tráfico entre servicios y redes adicionales limitadas a las dependencias necesarias. Una red por servicio aislaría más, pero exigiría gestionar una red para cada llamada válida. Mantener la red compartida `internal` no reduciría el movimiento lateral.

La red `services` tiene subnet fija `10.251.0.0/24`; las IP dinámicas se asignan desde `10.251.0.128/25` para reservar `10.251.0.2` para Traefik. Esa IP fija se usa también en `FORWARDED_ALLOW_IPS`. `edge` conserva su subnet y sus IPs del escenario 2.

## 5. Implementación

### 5.1 Membresía de redes

| Red | Miembros |
|---|---|
| `public` | `reverse-proxy` (sin cambios) |
| `edge` | `reverse-proxy`, `traefik` (sin cambios) |
| `services` | `traefik`, `frontend`, `auth`, `lobby-manager`, `game-logic`, `bot-service`, `ratings-service` |
| `auth-data` | `auth`, `auth-postgres` |
| `lobby-data` | `lobby-manager`, `postgres` |
| `ratings-data` | `ratings-service`, `ratings-postgres` |
| `game-data` | `game-logic`, `redis` |
| `messaging` | `rabbitmq`, `lobby-manager`, `game-logic`, `ratings-service` |

Todas menos `public` tienen `internal: true`; la definición `internal` desaparece.

### 5.2 Archivos

| Archivo | Cambio |
|---|---|
| [`docker-compose.yml`](../../docker-compose.yml) | Nuevas redes, subnet e IP de Traefik; reasignación de todos los servicios |
| [`api-gateway/traefik.yml`](../../api-gateway/traefik.yml) | Provider Docker fijado a `bololo-integrado_services` |
| [`scripts/e2e.sh`](../../scripts/e2e.sh) | Pruebas de DNS aislado, membresía de `game-data` e IP confiable |
| [`docs/deployment-view.md`](../deployment-view.md) y [`docs/deployment-view.drawio`](../deployment-view.drawio) | Diagrama y membresías de red actualizados |

Se mantienen los nombres DNS usados por la aplicación: SSR → `ratings-service`; bots → `auth`, `lobby-manager`, `game-logic`; Lobby → Game Logic; cada servicio → su almacén; Lobby, Game Logic y Ratings → RabbitMQ.

## 6. Cambios en la arquitectura

### 6.1 Antes y después

| Aspecto | Antes | Después |
|---|---|---|
| Red de servicios y datos | `internal`, compartida | `services` y redes acotadas por dependencia |
| `frontend` → datos/broker | Puede resolverlos | No comparte sus redes |
| Miembros de `game-data` | Servicios de toda la aplicación | Solo `game-logic` y `redis` |
| IP confiable para forwarded headers | `*` | Traefik, `10.251.0.2` |
| `public` y `edge` | Escenario 2 | Sin cambios |

### 6.2 Vista de despliegue

La vista Mermaid y la página `Network Segmentation (P3 Escenario 3)` del diagrama draw.io muestran los miembros de cada red. La matriz también está en [`docs/deployment-view.md` §6](../deployment-view.md#6-networks-ports-and-volumes).

## 7. Evidencia

Pruebas reproducibles después de levantar el stack:

```bash
docker compose exec frontend sh -c 'nc -zv -w2 postgres 5432'
docker compose exec lobby-manager python -c "import socket; socket.create_connection(('redis',6379),2)"
docker network inspect bololo-integrado_game-data --format '{{range .Containers}}{{.Name}} {{end}}'
./scripts/e2e.sh
```

Resultado esperado: las conexiones no resuelven el nombre; `game-data` muestra solo `game-logic` y `redis`. El E2E confirma también que Traefik, SSR, bots, Lobby, partidas y Ratings mantienen su conectividad permitida.

Validación realizada en `feature/mis-cambios`:

| Comprobación | Resultado |
|---|---|
| `docker compose config --quiet` | Correcto |
| `docker compose up --wait` | Los 13 servicios quedan sanos |
| Pruebas Network Segmentation del E2E | 11/11 correctas; Traefik ocupa `10.251.0.2`, los servicios dinámicos reciben IP desde `10.251.0.128/25` |
| `scripts/e2e.sh` | Última ejecución: 148 OK, 19 fallas, 1 omitida. La falla inicial es la paridad de `.env`; las siguientes reflejan estado persistente de ejecuciones anteriores (lobbies activos y mensajes dead-letter) |

La falla de paridad es del `.env` local: contiene cuatro variables BOT y `JWT_SECRET` que no están en `.env.example`. No se modificaron valores locales ni se borraron volúmenes o datos para forzar una ejecución limpia. El arranque, las 11 comprobaciones de aislamiento, las rutas, SSR, Lobby y Auth pasaron.

## 8. Compromisos

- Los servicios de aplicación continúan compartiendo `services`; un compromiso allí puede alcanzar otros servicios de aplicación.
- Las dependencias deben añadirse explícitamente a las redes pertinentes al incorporar servicios.
- Subnet `10.251.0.0/24`, IP de Traefik y `FORWARDED_ALLOW_IPS` deben permanecer coordinados.
- `internal: true` limita el egreso de red; las imágenes se descargan durante el build desde el host.

## 9. Riesgos residuales

| Riesgo | Tratamiento |
|---|---|
| Movimiento entre servicios dentro de `services` | La separación aquí es entre aplicaciones y almacenes; la autorización entre servicios sigue dependiendo de cada API |
| Credenciales de RabbitMQ compartidas | Las redes limitan quién llega al broker, pero no reemplazan permisos AMQP por servicio |
| Socket Docker en Traefik | No está expuesto en `public`; un socket proxy restringido sería una mejora independiente |
| HTTP entre proxy y gateway | `edge` conserva solo esos dos miembros; TLS interno queda fuera de este escenario |

## 10. Cómo usarlo

```bash
docker compose config
docker compose up -d --build --wait
./scripts/e2e.sh
```

Al migrar desde el Compose anterior, detén el stack y elimina la red antigua solo si quedó vacía:

```bash
docker compose down
docker network inspect bololo-integrado_internal --format '{{json .Containers}}'
docker network rm bololo-integrado_internal
docker compose up -d --build --wait
```

`docker compose down` conserva volúmenes; no usar `down -v`. No retires la red si `docker network inspect` muestra contenedores conectados.

## 11. Guion de demo

| Tiempo | Paso | Qué mostrar |
|---|---|---|
| 0:00 | Contexto | Antes todos los contenedores compartían `internal` |
| 0:35 | Mostrar Compose y el diagrama | `services`, redes `*-data`, `messaging`; `public` y `edge` siguen iguales |
| 1:15 | Probar `frontend` → `postgres` | `bad address`: no resuelve el nombre |
| 1:45 | Probar `lobby-manager` → `redis` | Falla porque Redis solo está en `game-data` con Game Logic |
| 2:15 | Inspeccionar `bololo-integrado_game-data` | Solo `game-logic` y `redis` |
| 2:40 | Ejecutar `scripts/e2e.sh` | Rutas y dependencias permitidas siguen funcionando |
| 3:25 | Cierre | Cero conexiones de frontend a datos/broker fuera de sus redes |

## 12. Referencias

- Docker, [Compose file reference: networks](https://docs.docker.com/reference/compose-file/networks/).
- Docker, [Networking in Compose](https://docs.docker.com/compose/how-tos/networking/).
- Uvicorn, [Settings: `FORWARDED_ALLOW_IPS`](https://www.uvicorn.org/settings/).
- Bass, Clements y Kazman, *Software Architecture in Practice*, 4.ª ed., cap. 11.
