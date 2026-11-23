# Caso 4 · Rendimiento de autenticación

| Pregunta | Respuesta corta |
|---|---|
| ¿Qué se probó? | Invitados, registro, login y lectura de sesión a través de `auth-service` |
| ¿Con qué? | k6 v2.4.0 desde el host, por HTTPS → Nginx → Traefik → Auth → PostgreSQL |
| ¿Dónde está la rodilla? | Invitados: 800 VUs presentan el primer error aislado; registro y login: aproximadamente 400 VUs; `/auth/me`: aproximadamente 2000 VUs |
| ¿Cuello de botella? | BCrypt y Spring saturan para registro/login; la verificación JWT llega más lejos |

Fecha de las mediciones: 22 de noviembre de 2026. Scripts y resultados crudos: [`perf/caso4-auth/`](../../../perf/caso4-auth/).

## 1. Objetivo y alcance

Medir por separado las operaciones principales de Auth:

| Subcaso | Funcionalidad |
|---|---|
| 4A | Crear invitado (`POST /auth/guest`) |
| 4B | Registrar cuenta (`POST /auth/register`) |
| 4C | Iniciar sesión (`POST /auth/login`) |
| 4D | Leer sesión (`GET /auth/me`) |

Cada subcaso usa una curva independiente. No se mezclan operaciones dentro de una misma corrida.

## 2. Entorno de la prueba

El entorno es el mismo usado en los casos anteriores: portátil AMD Ryzen 5 3500U con Docker Desktop, VM de 4 CPU y 1,4 GB de RAM, k6 en el host y el sistema real con TLS y reverse proxy.

Se usan específicamente:

| Elemento | Detalle |
|---|---|
| Emisor de sesiones | `auth-service`, Java 21 / Spring Boot |
| Firma | RS256 con las claves RSA de sesión |
| Hash de contraseña | BCrypt |
| Base de datos | PostgreSQL 16.15 para cuentas |
| Gateway | Traefik |
| Proxy externo | Nginx con TLS |

## 3. Metodología

Cada corrida usa:

- Rampa de 15 segundos.
- Fase sostenida de 60 segundos.
- Métricas tomadas solo durante la fase sostenida.
- `sleep(1)` entre iteraciones.
- Reposo entre corridas.
- Barrido grueso y refinamiento alrededor de la primera rodilla.
- Tres corridas en los niveles decisivos.
- Mediana para resultados con varias corridas.

La definición de error es:

- Sin respuesta o timeout.
- Estado `4xx` o `5xx` inesperado.
- Registro o login válido que no devuelve el estado esperado.
- Sesión emitida que no pasa la validación de `/auth/me`.
- Cuerpo o claim inválido.

## 4. Resultados

### 4.1 Resumen por subcaso

| Subcaso | Último nivel estable observado | Capacidad aproximada | Primer nivel con degradación |
|---|---|---|---|
| Invitado | 400 VUs: 451 peticiones/s | 900–1350 peticiones/s | 800 VUs, errores HTTP/2 aislados; 2000 VUs, ~1 % |
| Registro | 100–200 VUs: ~45 peticiones/s | ~50 peticiones/s | 400 VUs; latencia superior a 8 s y errores intermitentes |
| Login válido | 100–200 VUs: ~45 peticiones/s | ~50–60 peticiones/s | 400 VUs; 800 VUs pierde respuestas por 404/502 |
| `/auth/me` | 1200 VUs: 1354 peticiones/s | ~2227 peticiones/s | 2000 VUs, latencia mayor y pérdida en algunas corridas |

### 4.2 Invitado

| VUs | Peticiones/s | Promedio | p95 | Errores |
|---|---|---|---|---|
| 400 | 451 | 3.8 ms | 5.2 ms | 0 % |
| 800 | 901 | 4.7 ms | 8.3 ms | 0 / 0,002 / 0 % |
| 1200 | 1345 | 10,1 ms | 35,0 ms | 0 / 0 / 0,004 % |
| 2000 | 1986–2022 | 121–194 ms | 445–804 ms | 0,831 / 1,190 / 0,831 % |

El primer error apareció en una corrida de 800 VUs, por una interrupción HTTP/2. A 1200 VUs siguió siendo inestable con errores menores. A 2000 VUs hubo degradación clara.

### 4.3 Registro

| VUs | Peticiones/s | Promedio | p95 | CPU máxima Auth |
|---|---|---|---|---|
| 1 | 0,8 | 239 ms | 260 ms | 25 % |
| 10 | 9,2 | 228 ms | 240 ms | 197 % |
| 25 | 22,6 | 246 ms | 272 ms | 552 % |
| 50 | 40,5 | 400 ms | 684 ms | 1177 % |
| 100 | 44,5 | 1572 ms | 2468 ms | 1238 % |
| 200 | 46,4 | 3986 ms | 5739 ms | 1236 % |
| 400 | 49,3 | 8881 ms | 11747 ms | 1313 % |
| 800 | 53,6 | 18207 ms | 25102 ms | 1203 % |

La saturación comienza cerca de 50–100 VUs. `Auth` alcanza varios cientos de porcentaje de CPU porque Docker reporta uso agregado sobre varios núcleos. PostgreSQL permanece cercano a una carga baja. La latencia crece por la encolación de bcrypt y trabajo Spring.

En las repeticiones decisivas apareció una mezcla de `201`, `404` y latencias largas. Una corrida con 400 VUs tuvo `404`; las tres a 800 VUs tuvieron rutas desaparecidas o respuestas largas. Esto indica que el servicio se vuelve inestable antes de quedar completamente saturado.

### 4.4 Login válido

| VUs | Peticiones/s | Promedio | p95 | CPU máxima Auth |
|---|---|---|---|---|
| 1 | 0,8 | 232 ms | 244 ms | 35 % |
| 10 | 9,2 | 228 ms | 240 ms | 215 % |
| 25 | 22,6 | 250 ms | 295 ms | 551 % |
| 50 | 40,4 | 403 ms | 707 ms | 1265 % |
| 100 | 44,4 | 1586 ms | 2492 ms | 1234 % |
| 200 | 46,4 | 4011 ms | 5739 ms | 1207 % |
| 400 | 49,8 | 8685 ms | 11344 ms | 1361 % |
| 800 | 83,5 | 10311 ms | 22041 ms | 1220 % |

Login válido sigue la curva de BCrypt de registro. A 800 VUs aparecen ~2025 respuestas `404` en una corrida confirmada.

El usuario inexistente fue comprobado con una corrida corta separada: 9 respuestas `401`, 227 ms promedio y 262 ms p95. La validación tomó BCrypt mediante el decoy hash y no reveló una diferencia grande frente a login válido.

### 4.5 `/auth/me`

| VUs | Peticiones/s | Promedio | p95 | CPU máxima Auth | CPU máxima PostgreSQL |
|---|---|---|---|---|---|
| 1 | 1,0 | 8,9 ms | 8,1 ms | 5,9 % | 1 % |
| 50 | 56,4 | 3,0 ms | 3,6 ms | 12,2 % | 4,4 % |
| 100 | 112,9 | 2,9 ms | 3,5 ms | 30,1 % | 3,2 % |
| 200 | 225,8 | 2,9 ms | 3,7 ms | 17,2 % | 10,4 % |
| 400 | 451,3 | 3,6 ms | 5,1 ms | 43,9 % | 16,1 % |
| 800 | 902,3 | 4,0 ms | 6,4 ms | 73,6 % | 27,5 % |
| 1200 | 1353,7 | 4,1 ms | 7,6 ms | 97,7 % | 37,3 % |
| 2000 | 2227,0 | 16,3 ms | 70,9 ms | 192,4 % | 62,7 % |

La verificación `/auth/me` no ejecuta BCrypt y sigue creciendo hasta al menos 1200 VUs. A 2000 VUs empieza a acumular latencia y usa más CPU y PostgreSQL.

## 5. Cuello de botella

Las operaciones con contraseña tienen el patrón más claro:

- Registro: BCrypt de fuerza por defecto + inserción PostgreSQL + firma RS256.
- Login: consulta PostgreSQL + BCrypt + firma RS256.
- Invitado: no usa PostgreSQL ni BCrypt; solo genera un jugador y firma RS256.
- `/auth/me`: verifica el JWT y consulta la cuenta.

Registro y login llegan a aproximadamente 45–50 solicitudes/s antes de que la cola crezca. Guest puede superar las 900 solicitudes/s y mantener casi todo sin errores hasta la zona 800–1200 VUs. `/auth/me` llega por encima de 1350 solicitudes/s sin degradación visible.

## 6. Gráficas

![Invitado: latencia y errores](img/guest-latencia.png)

![Registro: latencia y errores](img/register-latencia.png)

![Login: latencia y errores](img/login-latencia.png)

![Sesión: latencia y errores](img/me-latencia.png)
