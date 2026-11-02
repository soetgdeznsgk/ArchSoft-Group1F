"""Publicación de eventos en RabbitMQ. El contrato está en docs/ratings-service.md."""

from __future__ import annotations

import json
from typing import Any, Protocol

import aio_pika
from aio_pika.abc import AbstractExchange, AbstractRobustConnection

EXCHANGE = "bololo.events"
ROUTING_KEY = "game.finished"
PUBLISH_TIMEOUT_S = 5


class ResultBroker(Protocol):
    async def publish(self, event: dict[str, Any]) -> None: ...

    async def aclose(self) -> None: ...


class RabbitResultBroker:
    def __init__(self, url: str) -> None:
        self._url = url
        self._connection: AbstractRobustConnection | None = None
        self._exchange: AbstractExchange | None = None

    async def publish(self, event: dict[str, Any]) -> None:
        """Retorna cuando RabbitMQ confirmó el mensaje y lo entregó a una cola; si no, lanza una excepción."""
        exchange = await self._connected_exchange()
        message = aio_pika.Message(
            json.dumps(event).encode(),
            content_type="application/json",
            delivery_mode=aio_pika.DeliveryMode.PERSISTENT,
            message_id=event["gameId"],
        )
        await exchange.publish(message, routing_key=ROUTING_KEY, mandatory=True, timeout=PUBLISH_TIMEOUT_S)

    async def aclose(self) -> None:
        if self._connection:
            await self._connection.close()

    async def _connected_exchange(self) -> AbstractExchange:
        if self._exchange is None:
            self._connection = await aio_pika.connect_robust(self._url, timeout=PUBLISH_TIMEOUT_S)
            # on_return_raises: sin cola enlazada (Ratings aún no arrancó) el mensaje vuelve y debe contar como fallo.
            channel = await self._connection.channel(publisher_confirms=True, on_return_raises=True)
            self._exchange = await channel.declare_exchange(EXCHANGE, aio_pika.ExchangeType.TOPIC, durable=True)
        return self._exchange
