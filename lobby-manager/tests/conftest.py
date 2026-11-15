"""Utilidades de las pruebas.

Cada prueba levanta el servicio real con uvicorn en un puerto libre y le habla por HTTP,
contra un PostgreSQL de verdad (TEST_DATABASE_URL). Game Logic se simula con FakeGameLogic.
"""

from __future__ import annotations

import asyncio
import json
import os
import secrets
import time
from contextlib import asynccontextmanager
from dataclasses import dataclass, replace

import httpx
import jwt
import pytest
import uvicorn
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import rsa

from pydantic import ValidationError

from app.config import Settings
from app.errors import ApiError
from app.main import create_app
from app.result_consumer import apply_game_ended

TEST_DATABASE_URL = os.environ.get("TEST_DATABASE_URL")
SERVICE_SECRET = "secreto-servicio-solo-para-pruebas-0123456789"


def _rsa_pair() -> tuple[str, str]:
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    private = key.private_bytes(
        serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8, serialization.NoEncryption()
    ).decode()
    public = key.public_key().public_bytes(
        serialization.Encoding.PEM, serialization.PublicFormat.SubjectPublicKeyInfo
    ).decode()
    return private, public


# Las de sesión hacen de Auth; las de partida son las del Lobby y su pública la tendría Go.
SESSION_PRIVATE_KEY, SESSION_PUBLIC_KEY = _rsa_pair()
GAME_PRIVATE_KEY, GAME_PUBLIC_KEY = _rsa_pair()
OTHER_PRIVATE_KEY, _ = _rsa_pair()


def session_token(
    player_id: str,
    nickname: str,
    account_id: str | None = None,
    *,
    key: str = SESSION_PRIVATE_KEY,
    algorithm: str = "RS256",
    **extra,
) -> str:
    """Token de sesión como lo emite Auth."""
    now = int(time.time())
    claims = {"sub": player_id, "scope": "session", "nickname": nickname, "iat": now, "exp": now + 3600, **extra}
    if account_id is not None:
        claims["accountId"] = account_id
    return jwt.encode(claims, key, algorithm=algorithm)


def read_game_token(token: str) -> dict:
    """Lo que verifica Go: RS256 con la clave pública de partidas."""
    return jwt.decode(token, GAME_PUBLIC_KEY, algorithms=["RS256"])


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


class FakeBroker:
    """Simula RabbitMQ: guarda lo publicado o falla mientras `down` sea True."""

    def __init__(self) -> None:
        self.events: list[dict] = []
        self.down = False

    async def publish(self, event: dict) -> None:
        if self.down:
            raise ConnectionError("broker caído")
        self.events.append(event)

    async def aclose(self) -> None:
        pass


@dataclass
class Reply:
    """Resultado de entregar un game.ended, con la misma forma que una respuesta HTTP del Lobby."""

    status_code: int
    body: dict

    def json(self) -> dict:
        return self.body

    @property
    def text(self) -> str:
        return json.dumps(self.body)


class Ctx:
    def __init__(
        self, app, client: httpx.AsyncClient, game: FakeGameLogic, settings: Settings, broker: FakeBroker
    ) -> None:
        self.app = app
        self.client = client
        self.game = game
        self.settings = settings
        self.broker = broker

    @property
    def pool(self):
        return self.app.state.pool

    async def report_result(self, lobby_id: str, body: dict) -> Reply:
        """Entrega un game.ended como lo haría RabbitMQ, por el mismo camino que usa el consumidor."""
        message = json.dumps({**body, "lobbyId": lobby_id}).encode()
        try:
            result = await apply_game_ended(self.app.state.service, message)
        except ApiError as exc:
            return Reply(exc.status, {"code": exc.code, "message": exc.message})
        except ValidationError as exc:
            return Reply(422, {"code": "invalid_request", "message": str(exc)})
        return Reply(200, result.model_dump(by_alias=True))

    async def login(self, nickname: str, account_id: str | None = None) -> User:
        """Recibe una sesión de Auth y la presenta al Lobby, como hace el menú."""
        token = session_token(f"p_{secrets.token_hex(12)}", nickname, account_id)
        r = await self.client.get("/api/session", headers={"Authorization": f"Bearer {token}"})
        assert r.status_code == 200, r.text
        player = r.json()["player"]
        return User(player["id"], player["nickname"], token)

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
        session_public_key=SESSION_PUBLIC_KEY,
        game_private_key=GAME_PRIVATE_KEY,
        service_secret=SERVICE_SECRET,
        game_logic_url="http://game-logic.test:8081",
        db_connect_timeout_s=5,
        result_publish_interval_s=0.02,
    )
    return replace(base, **overrides)


@asynccontextmanager
async def running(settings: Settings):
    if not TEST_DATABASE_URL:
        pytest.skip("Define TEST_DATABASE_URL para correr las pruebas (ver README)")
    game = FakeGameLogic()
    broker = FakeBroker()
    app = create_app(settings, game_transport=httpx.MockTransport(game.handle), result_broker=broker)
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
            yield Ctx(app, client, game, settings, broker)
    finally:
        server.should_exit = True
        await task


@pytest.fixture
async def ctx():
    async with running(make_settings()) as c:
        yield c
