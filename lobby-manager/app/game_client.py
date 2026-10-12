"""Cliente del REST interno de Game Logic (docs/game-logic-design.md §7.2)."""

from __future__ import annotations

import logging

import httpx

log = logging.getLogger("lobby.game_client")


class GameServiceError(Exception):
    pass


class GameLogicClient:
    def __init__(
        self,
        base_url: str,
        service_secret: str,
        timeout_s: float = 3,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> None:
        self._http = httpx.AsyncClient(
            base_url=base_url,
            timeout=timeout_s,
            transport=transport,
            headers={"X-Service-Token": service_secret},
        )

    async def create_game(self, game_id: str, lobby_id: str, players: list[dict[str, str]]) -> None:
        """Entrega la sala a Go. `players` va en orden de asiento: [{"id", "name"}, ...].

        Go responde 201 al crear y 200 si el mismo gameId ya existía para la misma sala,
        así que reintentar una vez ante un fallo de red o un 5xx es seguro.
        """
        body = {"gameId": game_id, "lobbyId": lobby_id, "players": players}
        last_error = ""
        for attempt in (1, 2):
            try:
                response = await self._http.post("/internal/games", json=body)
            except httpx.TransportError as exc:
                last_error = f"{type(exc).__name__}: {exc}"
                log.warning("Game Logic no responde (intento %d): %s", attempt, last_error)
                continue
            if response.status_code in (200, 201):
                return
            last_error = f"HTTP {response.status_code}: {response.text[:200]}"
            log.warning("Game Logic rechazó la partida %s (intento %d): %s", game_id, attempt, last_error)
            if response.status_code < 500:
                break
        raise GameServiceError(last_error)

    async def aclose(self) -> None:
        await self._http.aclose()
