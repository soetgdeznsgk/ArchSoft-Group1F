"""Las pruebas hablan con el servicio real por HTTP, contra un PostgreSQL de verdad
(TEST_DATABASE_URL). Los mensajes se entregan directamente al consumidor, sin RabbitMQ."""

from __future__ import annotations

import os
from dataclasses import dataclass
from datetime import UTC, datetime
from uuid import uuid4

import httpx
import pytest

from app.config import Settings
from app.main import create_app

TEST_DATABASE_URL = os.environ.get("TEST_DATABASE_URL")


def game_event(
    ranking: list[tuple[str | None, int]],
    mode: str = "CLASSIC",
    game_id: str | None = None,
    winners: list[str] | None = None,
) -> dict:
    """`ranking` son pares (accountId, posición); accountId None representa a un invitado o bot.

    Sin `winners`, ganan quienes quedaron de primeros.
    """
    return {
        "gameId": game_id or f"g_{uuid4().hex}",
        "lobbyId": "l_prueba",
        "mode": mode,
        "endReason": "completed",
        "finishedAt": datetime.now(UTC).isoformat(),
        "winners": [f"p_{i}" for i, (_, rank) in enumerate(ranking) if rank == 1] if winners is None else winners,
        "players": [
            {"playerId": f"p_{i}", "accountId": account, "nickname": account or f"Bot {i}", "rank": rank}
            for i, (account, rank) in enumerate(ranking)
        ],
    }


class FakeMessage:
    """La parte de un mensaje de aio-pika que usa el consumidor."""

    def __init__(self, body: bytes) -> None:
        self.body = body
        self.message_id = "prueba"
        self.outcome: str | None = None

    async def ack(self) -> None:
        self.outcome = "ack"

    async def nack(self, requeue: bool = True) -> None:
        self.outcome = "requeue" if requeue else "dead"

    async def reject(self, requeue: bool = False) -> None:
        self.outcome = "requeue" if requeue else "dead"


@dataclass
class Ctx:
    app: object
    client: httpx.AsyncClient

    async def deliver(self, body: dict | bytes) -> FakeMessage:
        raw = body if isinstance(body, bytes) else httpx.Request("POST", "/", json=body).content
        message = FakeMessage(raw)
        await self.app.state.consumer.handle(message)
        return message

    async def board(self, mode: str = "CLASSIC") -> list[dict]:
        r = await self.client.get("/api/ratings/leaderboard", params={"mode": mode})
        assert r.status_code == 200, r.text
        return r.json()["players"]


@pytest.fixture
async def ctx():
    if not TEST_DATABASE_URL:
        pytest.skip("Define TEST_DATABASE_URL para correr las pruebas (ver README)")
    settings = Settings(
        database_url=TEST_DATABASE_URL, rabbitmq_url="amqp://no-se-usa", db_connect_timeout_s=5, broker_retry_s=0
    )
    app = create_app(settings, consume=False)
    async with app.router.lifespan_context(app):
        await app.state.pool.execute("TRUNCATE rating_changes, processed_games, ratings")
        transport = httpx.ASGITransport(app=app)
        async with httpx.AsyncClient(transport=transport, base_url="http://ratings.test") as client:
            yield Ctx(app, client)
