"""Utilidades de las pruebas.

Cada prueba levanta el servicio real con uvicorn en un puerto libre y le habla por HTTP,
contra un PostgreSQL de verdad (TEST_DATABASE_URL). Game Logic se simula con FakeGameLogic.
"""

from __future__ import annotations

import asyncio
import json
import os
from contextlib import asynccontextmanager
from dataclasses import dataclass, replace

import httpx
import pytest
import uvicorn

from app.config import Settings
from app.main import create_app

TEST_DATABASE_URL = os.environ.get("TEST_DATABASE_URL")
JWT_SECRET = "secreto-jwt-solo-para-pruebas-0123456789abcdef"
SERVICE_SECRET = "secreto-servicio-solo-para-pruebas-0123456789"
SERVICE_HEADERS = {"X-Service-Token": SERVICE_SECRET}


class FakeGameLogic:
    """Simula POST /internal/games de Go y guarda lo que recibió."""

    def __init__(self) -> None:
        self.requests: list[httpx.Request] = []
        self.responses: list[int | Exception] = []  # respuestas en cola; después de agotarlas, 201

    @property
    def bodies(self) -> list[dict]:
        return [json.loads(r.content) for r in self.requests]

    async def handle(self, request: httpx.Request) -> httpx.Response:
        self.requests.append(request)
        await asyncio.sleep(0.05)  # deja que las peticiones simultáneas se encuentren
        outcome = self.responses.pop(0) if self.responses else 201
        if isinstance(outcome, Exception):
            raise outcome
        body = json.loads(request.content)
        return httpx.Response(outcome, json={"gameId": body["gameId"], "status": "WAITING"})


@dataclass
class User:
    id: str
    nickname: str
    token: str

    @property
    def headers(self) -> dict[str, str]:
        return {"Authorization": f"Bearer {self.token}"}


class SseReader:
    def __init__(self, response: httpx.Response) -> None:
        self._lines = response.aiter_lines()
        self.pings = 0

    async def next(self, timeout: float = 5) -> tuple[str, dict] | None:
        """Siguiente evento (nombre, datos), o None si el servidor cerró el stream."""
        return await asyncio.wait_for(self._read(), timeout)

    async def next_named(self, name: str, timeout: float = 5) -> dict:
        """Avanza hasta el siguiente evento con ese nombre y devuelve sus datos."""

        async def read() -> dict:
            while True:
                event = await self._read()
                assert event is not None, f"el stream terminó sin enviar {name}"
                if event[0] == name:
                    return event[1]

        return await asyncio.wait_for(read(), timeout)

    async def _read(self) -> tuple[str, dict] | None:
        event, data = "", []
        async for line in self._lines:
            if line.startswith(":"):
                self.pings += 1
            elif line.startswith("event:"):
                event = line[6:].strip()
            elif line.startswith("data:"):
                data.append(line[5:].strip())
            elif line == "" and event:
                return event, json.loads("\n".join(data))
        return None


class Ctx:
    def __init__(self, app, client: httpx.AsyncClient, game: FakeGameLogic, settings: Settings) -> None:
        self.app = app
        self.client = client
        self.game = game
        self.settings = settings

    @property
    def pool(self):
        return self.app.state.pool

    async def login(self, nickname: str) -> User:
        r = await self.client.post("/api/session", json={"nickname": nickname})
        assert r.status_code == 201, r.text
        body = r.json()
        return User(body["player"]["id"], body["player"]["nickname"], body["token"])

    async def lobby_with(self, players: int, mode: str | None = None) -> tuple[dict, list[User]]:
        """Crea una sala con esa cantidad de jugadores. Devuelve la sala y los jugadores en orden de asiento.

        Sin `mode` la sala se crea sin cuerpo, como los clientes anteriores a los modos (CLASSIC).
        """
        users = [await self.login(f"Jugador{i + 1}") for i in range(players)]
        body = {"json": {"mode": mode}} if mode else {}
        r = await self.client.post("/api/lobbies", headers=users[0].headers, **body)
        assert r.status_code == 201, r.text
        lobby = r.json()
        for user in users[1:]:
            r = await self.client.post(f"/api/lobbies/{lobby['id']}/join", headers=user.headers)
            assert r.status_code == 200, r.text
            lobby = r.json()
        return lobby, users

    async def started_lobby(self) -> tuple[dict, list[User]]:
        lobby, users = await self.lobby_with(4)
        r = await self.client.post(f"/api/lobbies/{lobby['id']}/start", headers=users[0].headers)
        assert r.status_code == 200, r.text
        return r.json(), users

    @asynccontextmanager
    async def events(self, user: User, lobby_id: str):
        async with self.client.stream(
            "GET", f"/api/lobbies/{lobby_id}/events", params={"token": user.token}, timeout=None
        ) as response:
            assert response.status_code == 200, await response.aread()
            yield SseReader(response)


def make_settings(**overrides) -> Settings:
    base = Settings(
        database_url=TEST_DATABASE_URL or "",
        jwt_secret=JWT_SECRET,
        service_secret=SERVICE_SECRET,
        game_logic_url="http://game-logic.test:8081",
        db_connect_timeout_s=5,
    )
    return replace(base, **overrides)


@asynccontextmanager
async def running(settings: Settings):
    if not TEST_DATABASE_URL:
        pytest.skip("Define TEST_DATABASE_URL para correr las pruebas (ver README)")
    game = FakeGameLogic()
    app = create_app(settings, game_transport=httpx.MockTransport(game.handle))
    config = uvicorn.Config(app, host="127.0.0.1", port=0, log_level="warning", timeout_graceful_shutdown=1)
    server = uvicorn.Server(config)
    task = asyncio.create_task(server.serve())
    while not server.started:
        if task.done():
            task.result()
        await asyncio.sleep(0.01)
    port = server.servers[0].sockets[0].getsockname()[1]
    try:
        await app.state.pool.execute("TRUNCATE game_results, lobby_players, lobbies, players")
        async with httpx.AsyncClient(base_url=f"http://127.0.0.1:{port}", timeout=10) as client:
            yield Ctx(app, client, game, settings)
    finally:
        server.should_exit = True
        await task


@pytest.fixture
async def ctx():
    async with running(make_settings()) as c:
        yield c
