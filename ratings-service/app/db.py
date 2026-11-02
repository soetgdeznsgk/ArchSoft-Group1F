from __future__ import annotations

import asyncio
import logging
from pathlib import Path

import asyncpg

from .config import Settings

log = logging.getLogger("ratings.db")

SCHEMA = (Path(__file__).parent / "schema.sql").read_text(encoding="utf-8")

SCHEMA_LOCK = 7_200_001


async def create_pool(settings: Settings) -> asyncpg.Pool:
    """Abre el pool reintentando: depends_on solo garantiza Postgres al arrancar."""
    loop = asyncio.get_running_loop()
    deadline = loop.time() + settings.db_connect_timeout_s
    delay = 0.5
    while True:
        try:
            pool = await asyncpg.create_pool(settings.database_url, min_size=1, max_size=10)
            break
        except (OSError, asyncpg.PostgresError) as exc:
            if loop.time() + delay > deadline:
                raise
            log.warning("PostgreSQL no disponible (%s); reintento en %.1f s", exc, delay)
            await asyncio.sleep(delay)
            delay = min(delay * 2, 5)

    async with pool.acquire() as conn, conn.transaction():
        await conn.execute("SELECT pg_advisory_xact_lock($1)", SCHEMA_LOCK)
        await conn.execute(SCHEMA)
    return pool
