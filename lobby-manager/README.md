
Responsabilidades: RF-01 a RF-09 (ingreso, creación, listado, ingreso a una sala, salida, sala de espera en tiempo real e inicio de la partida) y RF-36 / RNF-04 (registrar una sola vez el resultado y cerrar la sala).


# Bololo Lobby Manager (FastAPI)

Componente de logica encargado de las salas: ingreso con nombre, crear, listar, entrar y salir de salas, sala de espera en tiempo real , inicio de la partida y registro unico del resultado

Requisitos que cubre: RF-01 a RF-09, RF-36 y RNF-04


## Estructura

lobby-manager/
├── app/
│   ├── main.py          arma la aplicacion y el /healthz
│   ├── api.py           rutas en /api/ e /internal/
│   ├── service.py       reglas de la sala
│   ├── events.py        Sala de espera y el disconect
│   ├── game_client.py   llamada a Go
│   ├── security.py      manejo de tokens y secretos
│   ├── schemas.py       modelos JSON de entrada y salida
│   ├── db.py            Crea el pool para arrancar el Postgres
│   ├── schema.sql       tablas
│   ├── config.py        config xd
│   └── errors.py        errores
├── tests/               pruebas contra el servicio real y PostgreSQL :3
├── tools/               Game Logic simulado y exportador de OpenAPI
├── docs/                MODELO_DATOS.md y openapi.json
├── compose.dev.yaml     compose
└── Dockerfile

## Uso rápido

Solo requiere Docker

docker compose -f lobby-manager/compose.dev.yaml up --build


- API interactiva (Swagger): <http://localhost:8000/api/docs>
- Detener y borrar todo: `docker compose -f lobby-manager/compose.dev.yaml down`

Recorrido con `curl` (en Git Bash):

```bash
B=http://localhost:8000
# RF-01: ingresar con un nombre. Devuelve el jugador y su token de sesión.
curl -s -X POST $B/api/session -H 'Content-Type: application/json' -d '{"nickname":"Ana"}'
T=<token>

curl -s -X POST $B/api/lobbies -H "Authorization: Bearer $T"          # crear sala
curl -s $B/api/lobbies -H "Authorization: Bearer $T"                  # listar salas
curl -sN "$B/api/lobbies/<lobbyId>/events?token=$T"                   # sala de espera (SSE)
curl -s -X POST $B/api/lobbies/join -H "Authorization: Bearer $T2" \
     -H 'Content-Type: application/json' -d '{"code":"K7QM2X"}'       # entrar por código
curl -s -X POST $B/api/lobbies/<lobbyId>/start -H "Authorization: Bearer $T"   # iniciar (con 4)

# Simular que Go terminó la partida y reportó el resultado:
curl -s -X POST http://localhost:18081/finish/<gameId>
```

El contrato completo (rutas, eventos SSE, errores y lo que se intercambia con Go) está en [docs/lobby-manager.md](../docs/lobby-manager.md) y en [docs/openapi.json](docs/openapi.json).

Auth dispone de `POST /internal/auth/session`, protegido por `X-Service-Token`, para obtener de forma idempotente un jugador asociado al UUID de cuenta y emitir el mismo token de sesión que usan los invitados. La ruta solo se consume por la red interna; Auth gestiona las contraseñas en su propia base.

## Configuración

| Variable | Obligatoria | Ejemplo | Uso |
|---|---|---|---|
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Sí | `bololo` | Credenciales de PostgreSQL |
| `POSTGRES_HOST`, `POSTGRES_PORT` | No | `postgres`, `5432` | Por defecto, el servicio `postgres` de Compose |
| `DATABASE_URL` | No | `postgresql://u:p@host:5432/db` | Reemplaza a las cinco anteriores si está definida |
| `JWT_SECRET` | Sí | hex aleatorio | Firma los tokens. El mismo valor que usa Game Logic para verificarlos |
| `SERVICE_SECRET` | Sí | hex aleatorio | Cabecera `X-Service-Token` del REST interno, en ambas direcciones |
| `GAME_LOGIC_URL` | No | `http://game-logic:8081` | Base del REST interno de Go |
| `CORS_ORIGINS` | No | `http://localhost:8080` | Solo para desarrollo, si las pantallas se sirven desde otro origen. Detrás del gateway no hace falta |
| `PRESENCE_GRACE_SECONDS` | No | `30` | Espera antes de sacar de la sala a quien cerró la página |
| `GAME_TTL_SECONDS` | No | `10800` | Una sala con partida iniciada y sin resultado se cierra pasado este tiempo |

## Pruebas

Las pruebas levantan el servicio real en un puerto libre y le hablan por HTTP contra un PostgreSQL de verdad, solo Game Logic está simulado. **Vacían las tablas** de la base indicada en `TEST_DATABASE_URL`, así que no debe apuntar a una base con datos que importen.

```powershell
docker compose -f lobby-manager/compose.dev.yaml up -d postgres
cd lobby-manager
python -m venv .venv; .venv\Scripts\Activate.ps1
pip install -r requirements-dev.txt
$env:TEST_DATABASE_URL = "postgresql://bololo:dev-password@127.0.0.1:15432/bololo"
pytest
```

| Archivo | Qué verifica | Requisitos |
|---|---|---|
| `test_session.py` | Ingreso con nombre, nombres inválidos, tokens inválidos | RF-01 |
| `test_lobbies.py` | Crear, listar, entrar (lista y código), límites de 10 salas y 4 jugadores, salir, eliminar la sala vacía | RF-02 a RF-07 |
| `test_events.py` | Sala de espera por SSE, aviso de inicio con token propio, cierre, keep-alive, jugador que cierra la página | RF-08 |
| `test_start.py` | Inicio solo con 4, entrega a Go en orden de asiento, token por jugador, fallos de Go | RF-09 |
| `test_result.py` | Token de servicio, registro único, cierre de la sala y liberación del cupo | RF-36, RNF-04 |
| `test_concurrency.py` | 8 jugadores por 3 cupos, 15 creaciones a la vez, 4 inicios a la vez, resultado reintentado en paralelo | RF-03, RF-06, RNF-04 |

Resultado actual: 54 pruebas en verde, en Python 3.12 (imagen del contenedor) y 3.13.

## Documentación

- [docs/lobby-manager.md](../docs/lobby-manager.md): contrato con Presentación, Game Logic y despliegue.
- [docs/MODELO_DATOS.md](docs/MODELO_DATOS.md): tablas, estados de la sala y control de concurrencia.
- [docs/openapi.json](docs/openapi.json): especificación OpenAPI. Se regenera con `python tools/export_openapi.py`.
