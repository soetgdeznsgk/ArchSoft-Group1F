"""Difusión en memoria de los eventos de sala hacia los clientes SSE.

Vive en el proceso, por eso el servicio corre con un solo worker de uvicorn.
"""

from __future__ import annotations

import asyncio
import json
import logging
from collections.abc import Awaitable, Callable
from typing import Any

log = logging.getLogger("lobby.events")

Event = tuple[str, Any]


def format_sse(event: str, data: Any) -> str:
    return f"event: {event}\ndata: {json.dumps(data, ensure_ascii=False)}\n\n"


class Subscription:
    def __init__(self, lobby_id: str, player_id: str) -> None:
        self.lobby_id = lobby_id
        self.player_id = player_id
        # None es la señal de cierre del stream.
        self.queue: asyncio.Queue[Event | None] = asyncio.Queue()


class LobbyEvents:
    def __init__(self, grace_s: float) -> None:
        self._grace_s = grace_s
        self._subs: dict[str, set[Subscription]] = {}
        self._tasks: set[asyncio.Task] = set()
        # Se llama cuando un jugador lleva grace_s sin ningún stream abierto en la sala.
        self.on_absent: Callable[[str, str], Awaitable[None]] | None = None

    def subscribe(self, lobby_id: str, player_id: str) -> Subscription:
        sub = Subscription(lobby_id, player_id)
        self._subs.setdefault(lobby_id, set()).add(sub)
        return sub

    def unsubscribe(self, sub: Subscription) -> None:
        subs = self._subs.get(sub.lobby_id)
        if subs is not None:
            subs.discard(sub)
            if not subs:
                del self._subs[sub.lobby_id]
        if self.on_absent and not self.is_present(sub.lobby_id, sub.player_id):
            task = asyncio.create_task(self._absent_after_grace(sub.lobby_id, sub.player_id))
            self._tasks.add(task)
            task.add_done_callback(self._tasks.discard)

    def is_present(self, lobby_id: str, player_id: str) -> bool:
        return any(s.player_id == player_id for s in self._subs.get(lobby_id, ()))

    def publish(self, lobby_id: str, event: str, data: Any) -> None:
        for sub in self._subs.get(lobby_id, ()):
            sub.queue.put_nowait((event, data))

    def publish_each(self, lobby_id: str, event: str, build: Callable[[str], Any]) -> None:
        """Como publish, pero con un contenido distinto por jugador (p. ej. su token)."""
        for sub in self._subs.get(lobby_id, ()):
            sub.queue.put_nowait((event, build(sub.player_id)))

    def close(self, lobby_id: str, player_id: str | None = None) -> None:
        """Termina los streams de la sala, o solo los de un jugador."""
        for sub in self._subs.get(lobby_id, ()):
            if player_id is None or sub.player_id == player_id:
                sub.queue.put_nowait(None)

    async def aclose(self) -> None:
        tasks = list(self._tasks)
        for task in tasks:
            task.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        for lobby_id in list(self._subs):
            self.close(lobby_id)

    async def _absent_after_grace(self, lobby_id: str, player_id: str) -> None:
        await asyncio.sleep(self._grace_s)
        if self.is_present(lobby_id, player_id) or self.on_absent is None:
            return
        try:
            await self.on_absent(lobby_id, player_id)
        except Exception:
            log.exception("No se pudo sacar de la sala %s al jugador ausente %s", lobby_id, player_id)
