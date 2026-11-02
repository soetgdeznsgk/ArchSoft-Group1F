"""Consumidor de game.finished. El contrato con el Lobby está en docs/ratings-service.md.

Un mensaje solo se confirma (ack) después de hacer commit. Si falla la base de datos vuelve
a la cola; si es inválido nunca podrá procesarse y pasa a la cola de mensajes muertos, para
que no bloquee a los demás.
"""

from __future__ import annotations

import asyncio
import logging

import aio_pika
from aio_pika.abc import AbstractIncomingMessage, AbstractQueue
from pydantic import ValidationError

from .config import Settings
from .schemas import GameFinished
from .service import RatingsService

log = logging.getLogger("ratings.consumer")

EXCHANGE = "bololo.events"
ROUTING_KEY = "game.finished"
QUEUE = "ratings.game-finished"
DEAD_EXCHANGE = "bololo.events.dead"
DEAD_QUEUE = "ratings.game-finished.dead"


class ResultConsumer:
    def __init__(self, settings: Settings, service: RatingsService) -> None:
        self._settings = settings
        self._service = service
        self.connected = False

    async def run(self) -> None:
        """Consume para siempre: si RabbitMQ cae, reconecta. La API de lectura sigue funcionando mientras tanto."""
        while True:
            try:
                connection = await aio_pika.connect(self._settings.rabbitmq_url)
                async with connection:
                    queue = await self._declare_topology(connection)
                    self.connected = True
                    log.info("Conectado a RabbitMQ; esperando partidas en %s", QUEUE)
                    async with queue.iterator() as messages:
                        async for message in messages:
                            await self.handle(message)
            except Exception as exc:
                log.warning("Conexión con RabbitMQ perdida (%r); reintento", exc)
            finally:
                self.connected = False
            await asyncio.sleep(self._settings.broker_retry_s)

    async def handle(self, message: AbstractIncomingMessage) -> None:
        try:
            game = GameFinished.model_validate_json(message.body)
        except ValidationError as exc:
            log.error("Evento inválido %s: %s", message.message_id, exc)
            await message.reject(requeue=False)
            return
        try:
            changed = await self._service.apply_game(game)
        except Exception:
            log.exception("No se pudo aplicar la partida %s; vuelve a la cola", game.game_id)
            await asyncio.sleep(self._settings.broker_retry_s)
            await message.nack(requeue=True)
            return
        await message.ack()
        log.info("Partida %s procesada (cambió el ranking: %s)", game.game_id, changed)

    @staticmethod
    async def _declare_topology(connection: aio_pika.abc.AbstractConnection) -> AbstractQueue:
        channel = await connection.channel()
        await channel.set_qos(prefetch_count=1)
        exchange = await channel.declare_exchange(EXCHANGE, aio_pika.ExchangeType.TOPIC, durable=True)
        dead_exchange = await channel.declare_exchange(DEAD_EXCHANGE, aio_pika.ExchangeType.FANOUT, durable=True)
        dead_queue = await channel.declare_queue(DEAD_QUEUE, durable=True)
        await dead_queue.bind(dead_exchange)
        queue = await channel.declare_queue(
            QUEUE, durable=True, arguments={"x-dead-letter-exchange": DEAD_EXCHANGE}
        )
        await queue.bind(exchange, routing_key=ROUTING_KEY)
        return queue
