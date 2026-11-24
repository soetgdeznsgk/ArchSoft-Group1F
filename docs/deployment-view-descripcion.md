# Estructura de despliegue · Descripción

| | |
|---|---|
| Estructura | De asignación (*allocation*): despliegue (Bass, Clements y Kazman) |
| Diagrama | [`deployment-view.drawio`](deployment-view.drawio) · catálogo técnico completo en [`deployment-view.md`](deployment-view.md) |
| Fuente | `docker-compose.yml`, `reverse-proxy/`, `api-gateway/traefik.yml`, `rabbitmq/topology.json` y el `Dockerfile` de cada componente, en `main` @ `53c479d` |

## Contenido

1. [La vista de despliegue](#1-la-vista-de-despliegue)
2. [Elementos arquitectónicos](#2-elementos-arquitectónicos)
3. [Relaciones](#3-relaciones)
4. [Patrones arquitectónicos](#4-patrones-arquitectónicos)
5. [Fuentes](#5-fuentes)

---

## 1. La vista de despliegue

En Bass, Clements y Kazman la estructura de despliegue es una **estructura de asignación**: muestra cómo los elementos de software se asignan al entorno en el que se ejecutan. Responde dos preguntas:

- **Dónde** se ejecuta cada elemento de software, y qué necesita ese entorno (imagen, runtime, puertos, redes, almacenamiento).
- **Qué nodos están conectados** entre sí y por qué canal.

No responde **cómo interactúan** los componentes. Protocolos, mensajes y direcciones son conectores y pertenecen a la vista de componentes y conectores (C&C). Por eso esta vista toma los elementos de software de la vista C&C, los ubica en el entorno y une los nodos solo con caminos de comunicación que indican el canal.

En BOLOLó el entorno es un único host con Docker Engine y Docker Compose v2. Ahí corren 13 contenedores conectados a 3 redes Docker, con 5 volúmenes y 4 secretos. El dispositivo del jugador queda fuera del sistema.

## 2. Elementos arquitectónicos

Un elemento arquitectónico tiene un conjunto claro de **responsabilidades**, una **frontera** definida y un conjunto de **interfaces**. En esta vista hay dos clases de elementos: los de software, que se asignan, y los del entorno, que reciben la asignación.

### 2.1 Elementos de software

Son los mismos de la vista C&C y en el diagrama se dibujan **fuera** de la infraestructura.

| Elemento | Tipo | Responsabilidad | Tecnología |
|---|---|---|---|
| `bololo_lobby_fe` | Componente (presentación) | Ingreso, salas y sala de espera | JavaScript, p5.js 1.10.0 |
| `bololo_game_fe` | Componente (presentación) | Mesas de juego de los dos modos | JavaScript, p5.js 1.10.0 |
| `bololo_web_ssr` | Componente (presentación) | Genera el HTML inicial y sirve los archivos de los dos frontends | JavaScript sobre Node.js |
| `bololo_reverse_proxy` | Componente (borde) | Único punto expuesto: TLS, filtrado de rutas y límite de tasa | Configuración de Nginx |
| `bololo_gat` | Componente (borde) | Enruta cada petición al servicio que corresponde | Configuración de Traefik |
| `bololo_auth` | Componente (lógica) | Cuentas, contraseñas y emisión de sesiones | Java 21, Spring Boot 3.5.5 |
| `bololo_lobby` | Componente (lógica) | Sesiones, salas, inicio de partidas y resultados | Python 3.12, FastAPI 0.143.0 |
| `bololo_game` | Componente (lógica) | Reglas y estado de las partidas | Go, gorilla/websocket 1.5.3 |
| `bololo_bots` | Componente (lógica) | Llena salas con bots y los hace jugar | Elixir 1.17.3 |
| `bololo_ratings` | Componente (lógica) | Ranking Elo por modo | Python 3.12, FastAPI 0.143.0 |
| `bololo_mq` | Componente (mensajería) | Exchange, colas y bindings de los eventos | RabbitMQ 3.13.7, `topology.json` |
| `bololo_auth_db`, `bololo_lobby_db`, `bololo_ratings_db` | Almacén de datos | Datos persistentes de cada servicio | PostgreSQL 16.15 |
| `bololo_game_db` | Almacén de datos | Estado de las partidas en curso | Redis 7.4.11 |
| Certificado TLS | Especificación de despliegue | Identidad del servidor en HTTPS | `certs/bololo.crt` + `.key` |
| `docker-compose.yml` + `.env` + secretos | Especificación de despliegue | Declara contenedores, redes, volúmenes, puertos y secretos | Docker Compose v2 |
| Navegador web | Componente externo | Permite al jugador interactuar con el sistema. No es parte de BOLOLó ni lo asigna el sistema | Chrome, Firefox, Edge |

### 2.2 Elementos del entorno

**Nodos.** Equipos y contenedores donde se ejecuta el software.

| Elemento | Tipo | Responsabilidad | Frontera | Interfaces |
|---|---|---|---|---|
| PC, portátil o teléfono del jugador | «device», externo | Ejecutar el navegador del jugador | Fuera del sistema, en Internet o la LAN | Conexión TCP/IP hacia los puertos 80 y 443 del host |
| Host Docker | «device» | Alojar todos los contenedores, redes, volúmenes y secretos | La máquina física. Solo los puertos 80 y 443 son visibles desde fuera | Puertos 80 y 443 |
| `reverse-proxy` | «container» | Alojar `bololo_reverse_proxy` y el certificado | Redes `public` y `edge`. Sistema de archivos de solo lectura y capacidades mínimas | Puertos de host 80 y 443 |
| `traefik` | «container» | Alojar `bololo_gat` | Redes `edge` e `internal`. Sin puertos en el host | 8000/tcp en la red interna; socket de Docker |
| `frontend` | «container» | Alojar `bololo_web_ssr` y los archivos de los dos frontends | Red `internal` | 3000/tcp |
| `auth`, `lobby-manager`, `game-logic`, `bot-service`, `ratings-service` | «container» | Alojar cada uno un servicio de lógica | Red `internal`; usuario sin privilegios | 8080, 8000, 8081, 8090 y 8000/tcp |
| `rabbitmq` | «container» | Alojar el broker | Red `internal`; volumen propio | 5672/tcp (15672 sin ruta) |
| `auth-postgres`, `postgres`, `ratings-postgres`, `redis` | «container» | Alojar cada uno un almacén de datos | Red `internal`; volumen propio | 5432/tcp (×3), 6379/tcp |

**Entornos de ejecución.** Son el software de base que, dentro de cada nodo, ejecuta el elemento asignado.

| Entorno de ejecución | Nodo | Ejecuta |
|---|---|---|
| Docker Engine | Host Docker | Los 13 contenedores, sus healthchecks y reinicios |
| Nginx 1.30.5 | `reverse-proxy` | `bololo_reverse_proxy` |
| Traefik 3.7.14 | `traefik` | `bololo_gat` |
| Node.js 22.14.0 | `frontend` | `bololo_web_ssr`; aloja los archivos de `bololo_lobby_fe` y `bololo_game_fe` |
| CPython 3.12 + Uvicorn 0.54.0 | `lobby-manager`, `ratings-service` | `bololo_lobby`, `bololo_ratings` |
| Runtime de Go 1.27.2 (binario estático) | `game-logic` | `bololo_game` |
| Erlang/OTP 27.1.2 | `bot-service` | `bololo_bots` |
| JVM Temurin JRE 21 | `auth` | `bololo_auth` |
| Erlang/OTP 26 + servidor RabbitMQ 3.13.7 | `rabbitmq` | `bololo_mq` |
| PostgreSQL 16.15 (×3) y Redis 7.4.11 | contenedores de datos | Los cuatro almacenes de datos |

**Canales y almacenamiento.**

| Elemento | Tipo | Responsabilidad | Frontera |
|---|---|---|---|
| Internet / LAN | Canal físico | Llevar el tráfico del jugador hasta el host | Externo, no confiable |
| `public` | Red Docker (bridge) | Recibir el tráfico de los puertos 80 y 443 | Solo `reverse-proxy` |
| `edge` | Red Docker, `internal: true` | Unir el proxy con el gateway | Solo `reverse-proxy` (10.250.0.2) y `traefik` (10.250.0.3). Sin salida a Internet |
| `internal` | Red Docker, `internal: true` | Unir el gateway, los servicios y los datos | Sin salida a Internet y sin puertos mapeables al host |
| 5 volúmenes con nombre | Almacenamiento | Persistir datos de PostgreSQL, Redis y RabbitMQ | Disco del host; sobreviven al reinicio de los contenedores |
| 4 secretos Docker | Almacenamiento | Entregar las llaves RS256 solo a `auth`, `lobby-manager` y `game-logic` | Archivos en `/run/secrets` de cada contenedor |

## 3. Relaciones

| Relación | Entre | Significado | Notación en el diagrama |
|---|---|---|---|
| **Asignado a** (`«deployed_in»`) | Elemento de software → entorno de ejecución | El elemento se instala y se ejecuta en ese entorno | Flecha roja punteada |
| **Contención** | Entorno de ejecución ⊂ contenedor ⊂ host | Un entorno vive dentro de un nodo, y los contenedores dentro del host | Anidamiento de cajas |
| **Camino de comunicación** | Nodo — nodo | Los dos nodos intercambian datos en ejecución por ese canal. No indica protocolo ni dirección | Línea sin punta, con estereotipo |
| **Conexión a red** | Contenedor — red Docker | El contenedor tiene una interfaz en esa red | Caja dentro de la zona; en el borde si está en dos redes |
| **Mapeo de puertos** | Puerto del host → puerto del contenedor | El host reenvía el puerto al contenedor | Flecha negra (`443 → 443`, `80 → 80`) |
| **Montaje** | Contenedor — volumen o archivo del host | El contenedor guarda o lee datos en el host | Línea amarilla punteada |
| **Se ejecuta en (externo)** | Navegador → dispositivo del jugador | Solo contexto: el sistema no asigna este elemento | Flecha gris punteada |

### 3.1 Asignación (`«deployed_in»`)

| Elemento de software | Entorno de ejecución | Nodo |
|---|---|---|
| `bololo_reverse_proxy`, certificado TLS | Nginx 1.30.5 | `reverse-proxy` |
| `bololo_gat` | Traefik 3.7.14 | `traefik` |
| `bololo_lobby_fe`, `bololo_game_fe`, `bololo_web_ssr` | Node.js 22.14.0 | `frontend` |
| `bololo_auth` | JVM Temurin JRE 21 | `auth` |
| `bololo_lobby` | CPython 3.12 + Uvicorn | `lobby-manager` |
| `bololo_game` | Go 1.27.2 | `game-logic` |
| `bololo_bots` | Erlang/OTP 27.1.2 | `bot-service` |
| `bololo_ratings` | CPython 3.12 + Uvicorn | `ratings-service` |
| `bololo_mq` | Erlang/OTP 26 + RabbitMQ | `rabbitmq` |
| `bololo_auth_db`, `bololo_lobby_db`, `bololo_ratings_db` | PostgreSQL 16.15 | `auth-postgres`, `postgres`, `ratings-postgres` |
| `bololo_game_db` | Redis 7.4.11 | `redis` |
| `docker-compose.yml` + `.env` + secretos | Docker Engine | Host Docker |

Propiedades de la asignación:

- Cada elemento de software tiene **exactamente un** destino. Un nodo puede alojar varios elementos: `frontend` aloja los dos frontends y `bololo_web_ssr`.
- Es **estática**: ningún elemento migra de nodo en ejecución y cada contenedor tiene una sola instancia.
- `bololo_lobby_fe` y `bololo_game_fe` se asignan a `frontend` y no al navegador. El navegador solo descarga sus archivos cuando el jugador abre una página.

### 3.2 Caminos de comunicación

| Nodo | Nodo | Estereotipo | Multiplicidad |
|---|---|---|---|
| Dispositivo del jugador | `reverse-proxy` | `«internet»` | `*` a `1` |
| `reverse-proxy` | `traefik` | `«edge network»` | 1 a 1 |
| `traefik` | `frontend`, `auth`, `lobby-manager`, `game-logic`, `bot-service`, `ratings-service` | `«internal network»` | 1 a 1 cada uno |
| `frontend` | `ratings-service` | `«internal network»` | 1 a 1 |
| `bot-service` | `auth`, `lobby-manager`, `game-logic` | `«internal network»` | 1 a 1 cada uno |
| `lobby-manager` | `game-logic` | `«internal network»` | 1 a 1 |
| `game-logic`, `lobby-manager`, `ratings-service` | `rabbitmq` | `«internal network»` | 1 a 1 cada uno |
| Cada servicio | Su contenedor de datos | `«internal network»` | 1 a 1 |
| `traefik` | Docker Engine | `«local socket»` | 1 a 1 |

## 4. Patrones arquitectónicos

Un patrón arquitectónico estructural es una colección con nombre de decisiones de diseño que resuelve un problema estructural recurrente. Se clasifican en patrones de **organización** (cómo se agrupan los componentes), de **comunicación** (cómo interactúan) y de **distribución** (cómo y dónde se ejecutan). Esta vista documenta los de **distribución**, que cubren la asignación, la ejecución autónoma y la infraestructura. Los de organización y comunicación se describen en las vistas de capas y C&C.

### 4.1 Cliente-servidor de tres niveles (*three-tier*)

| | |
|---|---|
| Problema | Separar dónde se ejecuta la interfaz, la lógica y los datos para que cada parte tenga su propio entorno y su propia frontera de seguridad |
| Solución en BOLOLó | **T1 · Cliente**: el dispositivo del jugador, que ejecuta los frontends en su navegador. **T2 · Servidor de aplicación**: `reverse-proxy`, `traefik`, `frontend` y los cinco servicios. **T3 · Datos y mensajería**: los cuatro contenedores de datos y `rabbitmq`. Los mismos niveles de la vista de capas |
| Elementos | Dispositivo del jugador, host Docker, todos los contenedores |
| Consecuencias | Cada nivel cambia o escala sin tocar los otros. T2 y T3 están en el mismo host, así que la separación entre ellos la dan las redes y no el hardware |

### 4.2 Un servicio por contenedor (*Service Instance per Container*)

| | |
|---|---|
| Problema | Desplegar componentes escritos en lenguajes y runtimes distintos (Java, Python, Go, Elixir, Node.js) sin que sus dependencias choquen |
| Solución en BOLOLó | Cada componente tiene su propia imagen con el entorno de ejecución que necesita, su healthcheck y su política `restart: unless-stopped`. Docker Compose los arranca en orden con `depends_on: service_healthy`. Los seis componentes propios (presentación, auth, lobby, game, bots y ratings) tienen su `Dockerfile` y su workflow de integración continua; Nginx, Traefik, RabbitMQ, PostgreSQL y Redis usan imágenes oficiales con versión fija |
| Elementos | Los 13 contenedores y sus entornos de ejecución; Docker Engine |
| Consecuencias | Ejecución autónoma: un contenedor se reconstruye, reinicia o reemplaza sin afectar a los demás. Hay una excepción deliberada: `frontend` aloja tres elementos de presentación, porque los dos frontends son archivos estáticos que sirve `bololo_web_ssr` |

### 4.3 Base de datos por servicio (*Database per Service*)

| | |
|---|---|
| Problema | Evitar que un servicio dependa del esquema o la disponibilidad de los datos de otro |
| Solución en BOLOLó | Cada almacén tiene su propio contenedor de base de datos y su propio volumen: `auth-postgres` para `bololo_auth`, `postgres` para `bololo_lobby`, `ratings-postgres` para `bololo_ratings` y `redis` para `bololo_game`. Cada uno usa credenciales propias |
| Elementos | 4 contenedores de datos, 4 volúmenes |
| Consecuencias | Un fallo o una migración en una base no afecta a las otras. La separación la imponen las credenciales, no la red: los cuatro contenedores están en la red `internal` junto con todos los servicios |

### 4.4 Segmentación de red por zonas de confianza

| | |
|---|---|
| Problema | Exponer el sistema a Internet sin exponer los servicios internos ni el componente que tiene acceso al socket de Docker |
| Solución en BOLOLó | Tres redes Docker en cadena. `public` solo tiene a `reverse-proxy`, que es el único contenedor con puertos en el host. `edge` (`internal: true`, IP fijas) une `reverse-proxy` con `traefik`. `internal` (`internal: true`) une `traefik` con los servicios y los datos. Traefik solo acepta cabeceras reenviadas desde 10.250.0.2 |
| Elementos | Redes `public`, `edge` e `internal`; `reverse-proxy` y `traefik` en los bordes entre zonas |
| Consecuencias | Un atacante en Internet solo alcanza Nginx, que no tiene el socket de Docker. Ningún contenedor de `edge` o `internal` puede salir a Internet ni publicarse por error |

### 4.5 Punto único de entrada en el borde (*Reverse Proxy* + *API Gateway*)

| | |
|---|---|
| Problema | Dar a los clientes una sola dirección y concentrar en el borde lo que todas las peticiones necesitan |
| Solución en BOLOLó | Dos nodos en serie con responsabilidades distintas. `reverse-proxy` (Nginx) termina TLS, filtra rutas y limita la tasa por IP. `traefik` enruta por ruta a cada servicio. Los servicios no tienen puertos en el host |
| Elementos | Puertos 80 y 443 del host, `reverse-proxy`, `traefik` |
| Consecuencias | El cliente solo conoce `https://<host>`. El borde es el único lugar donde se configuran el certificado, HSTS y los límites. Cada petición pasa por un salto más |

### 4.6 Configuración externalizada (*Externalized Configuration*)

| | |
|---|---|
| Problema | Usar la misma imagen en distintas máquinas sin incluir en ella contraseñas, llaves ni certificados |
| Solución en BOLOLó | `docker-compose.yml` y `.env` declaran variables y credenciales. La configuración de Nginx, Traefik y RabbitMQ se monta de solo lectura. Las llaves RS256 llegan como secretos Docker solo a los contenedores que las usan, y el certificado TLS se genera en cada máquina con `scripts/gen-certs.sh` |
| Elementos | Especificaciones de despliegue (compose, `.env`, certificado), secretos, montajes |
| Consecuencias | Cambiar contraseñas, certificado o llaves no requiere reconstruir imágenes. Compose no arranca si falta una variable obligatoria (`${VAR:?}`) |

### 4.7 Patrones de calidad que se apoyan en este despliegue

No son patrones estructurales, pero la vista de despliegue muestra dónde quedan:

| Patrón | Escenario | Dónde se despliega |
|---|---|---|
| Secure Channel (TLS + HSTS) | Escenario 1 | En el nodo `reverse-proxy`, puerto 443 |
| Reverse Proxy | Escenario 2 | `reverse-proxy` delante de `traefik`, en las redes `public` y `edge` |
| Rate Limiting | Escenario 5 | En el nodo `reverse-proxy`, límites por IP |

### 4.8 Limitaciones del despliegue actual

- **Un solo host.** Si el host cae, cae todo el sistema; no hay réplicas.
- **Instancia única.** `lobby-manager` y `game-logic` guardan estado en memoria (suscripciones SSE y partidas activas), así que hoy no se pueden replicar sin cambiar el diseño.
- **Sin límites de recursos.** Los contenedores comparten la CPU y la RAM del host sin cuotas.

## 5. Fuentes

- L. Bass, P. Clements y R. Kazman, *Software Architecture in Practice*, 4.ª ed., cap. 1 (estructuras de asignación: despliegue).
- J. A. Vergara Vargas, *Architectural Elements, Relations and Properties* y *Structural Architectural Patterns*, Arquitectura de Software 2026-II, UNAL.
- C. Richardson, Microservice Architecture patterns: [Service Instance per Container](https://microservices.io/patterns/deployment/service-per-container.html), [Database per Service](https://microservices.io/patterns/data/database-per-service.html), [Externalized Configuration](https://microservices.io/patterns/externalized-configuration.html).
- Escenarios del equipo: [`p3/esc1.md`](p3/esc1.md), [`p3/esc2.md`](p3/esc2.md), [`p3/esc5.md`](p3/esc5.md); vista de capas: [`layered-view.md`](layered-view.md).
