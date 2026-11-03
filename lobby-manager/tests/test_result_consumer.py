"""Decisión del consumidor de game.ended sobre cada mensaje: ack, a mensajes muertos o de vuelta a la cola."""

from __future__ import annotations

import json

from app import errors
from app.result_consumer import GameEndedConsumer
from app.schemas import ResultOut


class FakeMessage:
    def __init__(self, body: bytes) -> None:
        self.body = body
        self.message_id = "g_1"
        self.outcome: tuple | None = None

    async def ack(self) -> None:
        self.outcome = ("ack",)

    async def reject(self, requeue: bool = False) -> None:
        self.outcome = ("reject", requeue)

    async def nack(self, requeue: bool = True) -> None:
        self.outcome = ("nack", requeue)


class FakeService:
    def __init__(self, outcome) -> None:
        self.outcome = outcome
        self.calls: list[tuple[str, str]] = []

    async def record_result(self, lobby_id: str, result) -> ResultOut:
        self.calls.append((lobby_id, result.game_id))
        if isinstance(self.outcome, Exception):
            raise self.outcome
        return self.outcome


def event(**overrides) -> bytes:
    body = {"gameId": "g_1", "lobbyId": "l_1", "mode": "CLASSIC", "winners": ["p_1"], "endReason": "completed"}
    body.update(overrides)
    return json.dumps(body).encode()


async def handle(service: FakeService, body: bytes) -> FakeMessage:
    message = FakeMessage(body)
    await GameEndedConsumer("amqp://no-se-usa", service, retry_s=0).handle(message)
    return message


async def test_confirma_el_mensaje_despues_de_registrar():
    service = FakeService(ResultOut(game_id="g_1", lobby_id="l_1", recorded=True))
    message = await handle(service, event())
    assert service.calls == [("l_1", "g_1")]
    assert message.outcome == ("ack",)


async def test_un_reintento_ya_registrado_tambien_se_confirma():
    message = await handle(FakeService(ResultOut(game_id="g_1", lobby_id="l_1", recorded=False)), event())
    assert message.outcome == ("ack",)


async def test_un_mensaje_invalido_va_a_mensajes_muertos():
    service = FakeService(ResultOut(game_id="g_1", lobby_id="l_1", recorded=True))
    assert (await handle(service, b"no es json")).outcome == ("reject", False)
    assert (await handle(service, event(lobbyId=None))).outcome == ("reject", False)
    assert service.calls == []


async def test_un_resultado_que_no_corresponde_va_a_mensajes_muertos():
    message = await handle(FakeService(errors.game_mismatch()), event())
    assert message.outcome == ("reject", False)


async def test_una_falla_transitoria_devuelve_el_mensaje_a_la_cola():
    message = await handle(FakeService(ConnectionError("PostgreSQL no responde")), event())
    assert message.outcome == ("nack", True)
