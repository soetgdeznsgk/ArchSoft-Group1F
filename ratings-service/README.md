# Ratings Service

Ranking de jugadores con cuenta. Consume el evento `game.finished` de RabbitMQ, calcula el cambio de puntaje con Elo y expone el resultado por REST. El contrato está en [docs/ratings-service.md](../docs/ratings-service.md).

## Estructura

```
ratings-service/
├── app/
│   ├── main.py        arma FastAPI y arranca el consumidor
│   ├── api.py         rutas públicas de lectura
│   ├── consumer.py    consumidor de RabbitMQ y topología de colas
│   ├── service.py     aplica partidas y resuelve consultas
│   ├── elo.py         cálculo del puntaje, sin dependencias
│   ├── schemas.py     evento y respuestas
│   ├── db.py          pool de PostgreSQL y esquema
│   └── schema.sql
└── tests/
```

## Base de datos

| Tabla | Contenido |
|---|---|
| `ratings` | Puntaje, partidas y victorias de cada cuenta en cada modo |
| `processed_games` | Partidas ya procesadas. Su clave primaria hace idempotente el consumo |
| `rating_changes` | Historial: puntaje antes y después de cada partida |

## Configuración

| Variable | Obligatoria | Descripción |
|---|---|---|
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Sí | Credenciales de `ratings-postgres` |
| `POSTGRES_HOST`, `POSTGRES_PORT` | No | Por defecto `ratings-postgres` y `5432` |
| `RABBITMQ_URL` | Sí | `amqp://usuario:clave@rabbitmq:5672/` |
| `DATABASE_URL` | No | Reemplaza a las variables `POSTGRES_*` |

## Pruebas

Las pruebas usan un PostgreSQL real. Los mensajes se entregan directamente al consumidor, así que no necesitan RabbitMQ; `scripts/e2e.sh` prueba el recorrido completo con el broker.

```bash
python -m venv .venv && . .venv/bin/activate
pip install -r requirements-dev.txt
TEST_DATABASE_URL=postgresql://test:test@localhost:5432/ratings_test pytest -q
```
