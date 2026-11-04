"""Lobby Manager: arma la aplicación FastAPI.

Arranque: uvicorn app.main:create_app --factory --host 0.0.0.0 --port 8000
"""

from __future__ import annotations

import asyncio
import logging
from contextlib import asynccontextmanager, suppress

import httpx
from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from .api import internal, public
from .broker import ResultBroker, RabbitResultBroker
from .config import Settings
from .db import create_pool
from .errors import ApiError
from .events import LobbyEvents
from .result_consumer import GameEndedConsumer
from .game_client import GameLogicClient
from .service import LobbyService

log = logging.getLogger("lobby")

SWEEP_INTERVAL_S = 60


def create_app(
    settings: Settings | None = None,
    game_transport: httpx.AsyncBaseTransport | None = None,
    result_broker: ResultBroker | None = None,
) -> FastAPI:
    """`game_transport` y `result_broker` permiten a las pruebas simular a Game Logic y a RabbitMQ sin red."""
    settings = settings or Settings.from_env()
    if result_broker is None and settings.rabbitmq_url:
        result_broker = RabbitResultBroker(settings.rabbitmq_url)
    logging.basicConfig(level=logging.INFO, format="%(levelname)s:     %(name)s: %(message)s")

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        pool = await create_pool(settings)
        game_client = GameLogicClient(
            settings.game_logic_url, settings.service_secret, settings.game_request_timeout_s, game_transport
        )
        events = LobbyEvents(settings.presence_grace_s)
        service = LobbyService(pool, settings, events, game_client)
        events.on_absent = service.auto_leave
        app.state.pool = pool
        app.state.events = events
        app.state.service = service
        tasks = [asyncio.create_task(_sweep(service))]
        if settings.rabbitmq_url:
            tasks.append(asyncio.create_task(GameEndedConsumer(settings.rabbitmq_url, service).run()))
        if result_broker:
            tasks.append(
                asyncio.create_task(_publish_results(service, result_broker, settings.result_publish_interval_s))
            )
        try:
            yield
        finally:
            for task in tasks:
                task.cancel()
                with suppress(asyncio.CancelledError):
                    await task
            if result_broker:
                await result_broker.aclose()
            await events.aclose()
            await game_client.aclose()
            await pool.close()

    app = FastAPI(
        title="Bololó - Lobby Manager",
        version="1.0.0",
        description="Gestión de salas: ingreso, salas, sala de espera (SSE), inicio de la partida y registro del resultado.",
        lifespan=lifespan,
        docs_url="/api/docs",
        redoc_url=None,
        openapi_url="/api/openapi.json",
    )
    app.state.settings = settings

    if settings.cors_origins:
        app.add_middleware(
            CORSMiddleware, allow_origins=list(settings.cors_origins), allow_methods=["*"], allow_headers=["*"]
        )

    @app.exception_handler(ApiError)
    async def api_error(request: Request, exc: ApiError) -> JSONResponse:
        return JSONResponse({"code": exc.code, "message": exc.message}, status_code=exc.status)

    @app.exception_handler(RequestValidationError)
    async def validation_error(request: Request, exc: RequestValidationError) -> JSONResponse:
        first = exc.errors()[0]
        field = ".".join(str(part) for part in first["loc"][1:]) or "body"
        return JSONResponse(
            {"code": "invalid_request", "message": f"Solicitud inválida ({field}): {first['msg']}"}, status_code=422
        )

    @app.get("/healthz", tags=["salud"])
    async def healthz() -> JSONResponse:
        """Solo para Docker: no se publica en el gateway."""
        try:
            await asyncio.wait_for(app.state.pool.fetchval("SELECT 1"), timeout=2)
        except Exception:
            return JSONResponse({"status": "unavailable", "postgres": "down"}, status_code=503)
        return JSONResponse({"status": "ok", "postgres": "ok"})

    app.include_router(public)
    app.include_router(internal)
    return app


async def _sweep(service: LobbyService) -> None:
    while True:
        try:
            await service.expire_stale_games()
        except Exception:
            log.exception("Falló la limpieza de salas sin resultado")
        await asyncio.sleep(SWEEP_INTERVAL_S)


async def _publish_results(service: LobbyService, broker: ResultBroker, interval_s: float) -> None:
    """Saca los resultados hacia el broker. Si RabbitMQ no responde, siguen pendientes y se reintentan."""
    while True:
        try:
            for event in await service.unpublished_results():
                await broker.publish(event)
                await service.mark_result_published(event["gameId"])
        except Exception as exc:
            log.warning("No se pudo publicar un resultado en el broker: %r", exc)
        await asyncio.sleep(interval_s)
