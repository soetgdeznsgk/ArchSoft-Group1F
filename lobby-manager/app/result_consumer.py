"""Consumidor de game.ended: Game Logic publica el resultado final y el Lobby lo registra.

Reemplaza al antiguo POST /internal/lobbies/{id}/result. Game Logic ya no conoce al Lobby,
así que entre los dos solo queda una dependencia (Lobby -> Game Logic al crear la partida).

Un mensaje solo se confirma (ack) después del commit. Si falla la base de datos vuelve a la
cola; si nunca podrá registrarse (sala inexistente, partida que no corresponde, JSON inválido)
pasa a la cola de mensajes muertos para que no bloquee a los demás.
"""

from __future__ import annotations

import asyncio
import logging

import aio_pika
from aio_pika.abc import AbstractIncomingMessage, AbstractQueue
from pydantic import ValidationError

from .errors import ApiError
from .schemas import GameEndedIn, ResultOut
from .service import LobbyService

log = logging.getLogger("lobby.results")

EXCHANGE = "bololo.events"
ROUTING_KEY = "game.ended"
QUEUE = "lobby.game-ended"
DEAD_EXCHANGE = "lobby.dead"
DEAD_QUEUE = "lobby.game-ended.dead"
RETRY_S = 2


async def apply_game_ended(service: LobbyService, body: bytes) -> ResultOut:
    """Registra el resultado. Lanza ValidationError o ApiError si el mensaje nunca podrá registrarse."""
    event = GameEndedIn.model_validate_json(body)
    return await service.record_result(event.lobby_id, event)


class GameEndedConsumer:
    def __init__(self, url: str, service: LobbyService, retry_s: float = RETRY_S) -> None:
        self._url = url
        self._service = service
        self._retry_s = retry_s

    async def run(self) -> None:
        """Consume para siempre: si RabbitMQ cae, reconecta. Las salas siguen funcionando mientras tanto."""
        while True:
            try:
                connection = await aio_pika.connect(self._url)
                async with connection:
                    queue = await self._declare_topology(connection)
                    log.info("Conectado a RabbitMQ; esperando resultados en %s", QUEUE)
                    async with queue.iterator() as messages:
                        async for message in messages:
                            await self.handle(message)
            except asyncio.CancelledError:
                raise
            except Exception as exc:
                log.warning("Conexión con RabbitMQ perdida (%r); reintento", exc)
            await asyncio.sleep(self._retry_s)

    async def handle(self, message: AbstractIncomingMessage) -> None:
        try:
            result = await apply_game_ended(self._service, message.body)
        except (ValidationError, ApiError) as exc:
            log.error("Resultado que no se puede registrar %s: %s", message.message_id, exc)
            await message.reject(requeue=False)
            return
        except Exception:
            log.exception("No se pudo registrar el resultado %s; vuelve a la cola", message.message_id)
            await asyncio.sleep(self._retry_s)
            await message.nack(requeue=True)
            return
        await message.ack()
        log.info("Resultado de %s registrado (nuevo: %s)", result.game_id, result.recorded)

    @staticmethod
    async def _declare_topology(connection: aio_pika.abc.AbstractConnection) -> AbstractQueue:
        channel = await connection.channel()
        await channel.set_qos(prefetch_count=1)
        exchange = await channel.declare_exchange(EXCHANGE, aio_pika.ExchangeType.TOPIC, durable=True)
        dead_exchange = await channel.declare_exchange(DEAD_EXCHANGE, aio_pika.ExchangeType.FANOUT, durable=True)
        dead_queue = await channel.declare_queue(DEAD_QUEUE, durable=True)
        await dead_queue.bind(dead_exchange)
        queue = await channel.declare_queue(QUEUE, durable=True, arguments={"x-dead-letter-exchange": DEAD_EXCHANGE})
        await queue.bind(exchange, routing_key=ROUTING_KEY)
        return queue
