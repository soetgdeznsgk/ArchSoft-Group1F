"""Ratings Service: API de lectura del ranking y consumidor de partidas terminadas.

Arranque: uvicorn app.main:create_app --factory --host 0.0.0.0 --port 8000
"""

from __future__ import annotations

import asyncio
import logging
from contextlib import asynccontextmanager, suppress

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse

from .api import public
from .config import Settings
from .consumer import ResultConsumer
from .db import create_pool
from .service import RatingsService


def create_app(settings: Settings | None = None, consume: bool = True) -> FastAPI:
    """`consume=False` permite a las pruebas entregar los mensajes sin RabbitMQ."""
    settings = settings or Settings.from_env()
    logging.basicConfig(level=logging.INFO, format="%(levelname)s:     %(name)s: %(message)s")

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        pool = await create_pool(settings)
        service = RatingsService(pool, settings)
        consumer = ResultConsumer(settings, service)
        app.state.pool = pool
        app.state.service = service
        app.state.consumer = consumer
        task = asyncio.create_task(consumer.run()) if consume else None
        try:
            yield
        finally:
            if task:
                task.cancel()
                with suppress(asyncio.CancelledError):
                    await task
            await pool.close()

    app = FastAPI(
        title="Bololó - Ratings Service",
        version="1.0.0",
        description="Ranking de jugadores con cuenta, calculado a partir de las partidas terminadas.",
        lifespan=lifespan,
        docs_url="/api/ratings/docs",
        redoc_url=None,
        openapi_url="/api/ratings/openapi.json",
    )

    @app.exception_handler(RequestValidationError)
    async def validation_error(request: Request, exc: RequestValidationError) -> JSONResponse:
        first = exc.errors()[0]
        field = ".".join(str(part) for part in first["loc"][1:]) or "body"
        return JSONResponse(
            {"code": "invalid_request", "message": f"Solicitud inválida ({field}): {first['msg']}"}, status_code=422
        )

    @app.get("/healthz", tags=["salud"])
    async def healthz() -> JSONResponse:
        """Solo para Docker: no se publica en el gateway. Sin broker el servicio sigue sano: aún puede leer."""
        broker = "ok" if app.state.consumer.connected else "down"
        try:
            await asyncio.wait_for(app.state.pool.fetchval("SELECT 1"), timeout=2)
        except Exception:
            return JSONResponse({"status": "unavailable", "postgres": "down", "broker": broker}, status_code=503)
        return JSONResponse({"status": "ok", "postgres": "ok", "broker": broker})

    app.include_router(public)
    return app
