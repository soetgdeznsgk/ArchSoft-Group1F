"""Operaciones de sala.

Cada operación es una transacción. Las que compiten por la misma sala la bloquean con
SELECT ... FOR UPDATE, así que se aplican una a una. Los eventos SSE se publican después
del commit, nunca dentro de la transacción.
"""

from __future__ import annotations

import json
import logging
import secrets
from typing import Any

import asyncpg

from . import errors, security
from .config import Settings
from .db import LOBBY_LIMIT_LOCK
from .events import LobbyEvents
from .game_client import GameLogicClient, GameServiceError
from .schemas import GameAccess, Lobby, LobbyPlayer, LobbySummary, Player, ResultIn, ResultOut, SessionOut

log = logging.getLogger("lobby.service")

NICKNAME_MAX = 20
# Sin 0/O ni 1/I/L, que se confunden al dictar el código.
CODE_ALPHABET = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
CODE_LENGTH = 6


def _new_id(prefix: str) -> str:
    return f"{prefix}_{secrets.token_hex(6)}"


def _new_code() -> str:
    return "".join(secrets.choice(CODE_ALPHABET) for _ in range(CODE_LENGTH))


def lobby_event(lobby: Lobby) -> dict[str, Any]:
    """Estado de la sala tal como se difunde a todos: nunca lleva el token de nadie."""
    return lobby.model_copy(update={"game": None}).model_dump(mode="json", by_alias=True)


def game_starting_event(lobby_id: str, access: GameAccess) -> dict[str, Any]:
    return {"lobbyId": lobby_id, **access.model_dump(by_alias=True)}


class LobbyService:
    def __init__(
        self, pool: asyncpg.Pool, settings: Settings, events: LobbyEvents, game_client: GameLogicClient
    ) -> None:
        self._pool = pool
        self._settings = settings
        self._events = events
        self._game_client = game_client

    # --- jugadores -----------------------------------------------------------

    async def create_session(self, nickname: str) -> SessionOut:
        nickname = " ".join(nickname.split())
        if not nickname:
            raise errors.invalid_nickname("Escribe un nombre de jugador.")
        if len(nickname) > NICKNAME_MAX:
            raise errors.invalid_nickname(f"El nombre puede tener máximo {NICKNAME_MAX} caracteres.")
        player = Player(id=_new_id("p"), nickname=nickname)
        await self._pool.execute("INSERT INTO players (id, nickname) VALUES ($1, $2)", player.id, player.nickname)
        return SessionOut(player=player, token=security.issue_session_token(self._settings, player.id))

    async def get_player(self, player_id: str) -> Player | None:
        row = await self._pool.fetchrow("SELECT id, nickname FROM players WHERE id = $1", player_id)
        return Player(id=row["id"], nickname=row["nickname"]) if row else None

    async def current_lobby(self, player_id: str) -> Lobby | None:
        async with self._pool.acquire() as conn:
            lobby_id = await self._active_lobby_of(conn, player_id)
            return await self._read_lobby(conn, lobby_id, viewer=player_id) if lobby_id else None

    # --- salas ---------------------------------------------------------------

    async def list_lobbies(self) -> list[LobbySummary]:
        rows = await self._pool.fetch(
            """
            SELECT l.id, l.code, l.created_at, count(lp.player_id) AS player_count
            FROM lobbies l
            LEFT JOIN lobby_players lp ON lp.lobby_id = l.id
            WHERE l.status = 'WAITING'
            GROUP BY l.id
            ORDER BY l.created_at
            """
        )
        return [
            LobbySummary(
                id=r["id"],
                code=r["code"],
                player_count=r["player_count"],
                max_players=self._settings.lobby_size,
                created_at=r["created_at"],
            )
            for r in rows
        ]

    async def get_lobby(self, lobby_id: str, viewer: str) -> Lobby:
        async with self._pool.acquire() as conn:
            lobby = await self._read_lobby(conn, lobby_id, viewer=viewer)
        if lobby is None:
            raise errors.lobby_not_found()
        return lobby

    async def create_lobby(self, player: Player) -> Lobby:
        async with self._pool.acquire() as conn, conn.transaction():
            if await self._lock_player(conn, player.id):
                raise errors.already_in_lobby()
            # Serializa las creaciones: sin esto, dos peticiones que ven 9 salas crearían la 10 y la 11.
            await conn.execute("SELECT pg_advisory_xact_lock($1)", LOBBY_LIMIT_LOCK)
            active = await conn.fetchval("SELECT count(*) FROM lobbies WHERE status <> 'CLOSED'")
            if active >= self._settings.max_lobbies:
                raise errors.lobby_limit_reached(self._settings.max_lobbies)

            code = _new_code()
            while await conn.fetchval("SELECT 1 FROM lobbies WHERE code = $1 AND status <> 'CLOSED'", code):
                code = _new_code()
            lobby_id = _new_id("l")
            await conn.execute("INSERT INTO lobbies (id, code) VALUES ($1, $2)", lobby_id, code)
            await conn.execute(
                "INSERT INTO lobby_players (lobby_id, player_id, seat) VALUES ($1, $2, 0)", lobby_id, player.id
            )
            return await self._read_lobby(conn, lobby_id, viewer=player.id)

    async def join_lobby(self, player: Player, *, lobby_id: str | None = None, code: str | None = None) -> Lobby:
        async with self._pool.acquire() as conn, conn.transaction():
            current = await self._lock_player(conn, player.id)
            # FOR UPDATE: quien llegue después espera aquí y cuenta los cupos con el estado ya actualizado.
            if code is not None:
                row = await conn.fetchrow(
                    "SELECT id, status FROM lobbies WHERE code = $1 AND status <> 'CLOSED' FOR UPDATE",
                    code.strip().upper(),
                )
            else:
                row = await conn.fetchrow(
                    "SELECT id, status FROM lobbies WHERE id = $1 AND status <> 'CLOSED' FOR UPDATE", lobby_id
                )
            if row is None:
                raise errors.lobby_not_found(by_code=code is not None)
            lobby_id = row["id"]

            if current == lobby_id:  # repetir el ingreso no es un error
                return await self._read_lobby(conn, lobby_id, viewer=player.id)
            if current is not None:
                raise errors.already_in_lobby()
            if row["status"] != "WAITING":
                raise errors.lobby_started()
            seats = [r["seat"] for r in await conn.fetch("SELECT seat FROM lobby_players WHERE lobby_id = $1", lobby_id)]
            if len(seats) >= self._settings.lobby_size:
                raise errors.lobby_full()
            await conn.execute(
                "INSERT INTO lobby_players (lobby_id, player_id, seat) VALUES ($1, $2, $3)",
                lobby_id,
                player.id,
                max(seats, default=-1) + 1,
            )
            lobby = await self._read_lobby(conn, lobby_id, viewer=player.id)

        self._events.publish(lobby_id, "lobby", lobby_event(lobby))
        return lobby

    async def leave_lobby(self, player_id: str, lobby_id: str) -> None:
        async with self._pool.acquire() as conn, conn.transaction():
            row = await conn.fetchrow("SELECT status FROM lobbies WHERE id = $1 FOR UPDATE", lobby_id)
            if row is None:
                raise errors.lobby_not_found()
            is_member = await conn.fetchval(
                "SELECT 1 FROM lobby_players WHERE lobby_id = $1 AND player_id = $2", lobby_id, player_id
            )
            if not is_member:
                raise errors.not_in_lobby()
            if row["status"] != "WAITING":
                raise errors.lobby_started()
            await conn.execute(
                "DELETE FROM lobby_players WHERE lobby_id = $1 AND player_id = $2", lobby_id, player_id
            )
            remaining = await conn.fetchval("SELECT count(*) FROM lobby_players WHERE lobby_id = $1", lobby_id)
            if remaining == 0:
                await conn.execute("DELETE FROM lobbies WHERE id = $1", lobby_id)
                lobby = None
            else:
                lobby = await self._read_lobby(conn, lobby_id)

        self._events.close(lobby_id, player_id)
        if lobby is not None:
            self._events.publish(lobby_id, "lobby", lobby_event(lobby))

    async def auto_leave(self, lobby_id: str, player_id: str) -> None:
        """Saca de la sala de espera a un jugador que cerró la página sin salir."""
        try:
            await self.leave_lobby(player_id, lobby_id)
            log.info("Jugador %s retirado de la sala %s por ausencia", player_id, lobby_id)
        except errors.ApiError:
            pass  # ya salió, la sala ya no existe o la partida inició

    async def start_game(self, player: Player, lobby_id: str) -> Lobby:
        async with self._pool.acquire() as conn, conn.transaction():
            row = await conn.fetchrow(
                "SELECT status FROM lobbies WHERE id = $1 AND status <> 'CLOSED' FOR UPDATE", lobby_id
            )
            if row is None:
                raise errors.lobby_not_found()
            lobby = await self._read_lobby(conn, lobby_id, viewer=player.id)
            if all(p.id != player.id for p in lobby.players):
                raise errors.not_in_lobby()
            if row["status"] == "IN_GAME":  # otro jugador la inició primero
                return lobby
            missing = self._settings.lobby_size - len(lobby.players)
            if missing > 0:
                raise errors.not_enough_players(missing)

            # La llamada a Go va dentro de la transacción: si falla, o si este proceso se cae,
            # la sala sigue en WAITING y nadie pudo entrar ni salir mientras tanto.
            game_id = _new_id("g")
            roster = [{"id": p.id, "name": p.nickname} for p in lobby.players]
            try:
                await self._game_client.create_game(game_id, lobby_id, roster)
            except GameServiceError:
                raise errors.game_service_unavailable() from None
            await conn.execute(
                "UPDATE lobbies SET status = 'IN_GAME', game_id = $2, started_at = now() WHERE id = $1",
                lobby_id,
                game_id,
            )
            lobby = await self._read_lobby(conn, lobby_id, viewer=player.id)

        self._events.publish(lobby_id, "lobby", lobby_event(lobby))
        self._events.publish_each(
            lobby_id, "game_starting", lambda pid: game_starting_event(lobby_id, self._game_access(game_id, pid))
        )
        return lobby

    # --- resultado -----------------------------------------------------------

    async def record_result(self, lobby_id: str, result: ResultIn) -> ResultOut:
        winners = result.winners or ([result.winner_id] if result.winner_id else [])
        winner_id = result.winner_id or (winners[0] if len(winners) == 1 else None)

        async with self._pool.acquire() as conn, conn.transaction():
            row = await conn.fetchrow("SELECT status, game_id FROM lobbies WHERE id = $1 FOR UPDATE", lobby_id)
            if row is None:
                raise errors.lobby_not_found()
            if row["game_id"] != result.game_id:
                raise errors.game_mismatch()
            roster = {
                r["player_id"]
                for r in await conn.fetch("SELECT player_id FROM lobby_players WHERE lobby_id = $1", lobby_id)
            }
            unknown = sorted(set(winners) - roster)
            if unknown:
                raise errors.invalid_result(f"Ganador que no pertenece a la sala: {', '.join(unknown)}")

            # ON CONFLICT DO NOTHING: un reintento de Go no crea un segundo registro (RNF-04).
            recorded = await conn.fetchval(
                """
                INSERT INTO game_results (game_id, lobby_id, winner_id, end_reason, payload)
                VALUES ($1, $2, $3, $4, $5::jsonb)
                ON CONFLICT (game_id) DO NOTHING
                RETURNING true
                """,
                result.game_id,
                lobby_id,
                winner_id,
                result.end_reason,
                json.dumps(result.model_dump(by_alias=True)),
            )
            closed_now = row["status"] != "CLOSED"
            if closed_now:
                await conn.execute("UPDATE lobbies SET status = 'CLOSED', closed_at = now() WHERE id = $1", lobby_id)

        if closed_now:
            self._publish_closed(
                lobby_id,
                {
                    "gameId": result.game_id,
                    "reason": "finished",
                    "winnerId": winner_id,
                    "winners": winners,
                    "endReason": result.end_reason,
                },
            )
        return ResultOut(game_id=result.game_id, lobby_id=lobby_id, recorded=bool(recorded))

    async def expire_stale_games(self) -> int:
        """Cierra las salas cuya partida nunca reportó resultado, para que no ocupen cupo."""
        rows = await self._pool.fetch(
            """
            UPDATE lobbies SET status = 'CLOSED', closed_at = now()
            WHERE status = 'IN_GAME' AND started_at < now() - make_interval(secs => $1)
            RETURNING id, game_id
            """,
            float(self._settings.game_ttl_s),
        )
        for row in rows:
            log.warning("Sala %s cerrada: la partida %s no reportó resultado", row["id"], row["game_id"])
            self._publish_closed(row["id"], {"gameId": row["game_id"], "reason": "expired"})
        return len(rows)

    # --- internos ------------------------------------------------------------

    def _publish_closed(self, lobby_id: str, data: dict[str, Any]) -> None:
        self._events.publish(lobby_id, "lobby_closed", {"lobbyId": lobby_id, **data})
        self._events.close(lobby_id)

    def _game_access(self, game_id: str, player_id: str) -> GameAccess:
        return GameAccess(
            game_id=game_id,
            ws_path=f"/ws/games/{game_id}",
            token=security.issue_game_token(self._settings, game_id, player_id),
        )

    async def _active_lobby_of(self, conn: asyncpg.Connection, player_id: str) -> str | None:
        return await conn.fetchval(
            """
            SELECT l.id FROM lobby_players lp
            JOIN lobbies l ON l.id = lp.lobby_id
            WHERE lp.player_id = $1 AND l.status <> 'CLOSED'
            """,
            player_id,
        )

    async def _lock_player(self, conn: asyncpg.Connection, player_id: str) -> str | None:
        """Bloquea al jugador y devuelve la sala viva en la que está, si hay alguna.

        El candado impide que dos peticiones simultáneas del mismo jugador lo metan en dos salas.
        """
        await conn.execute("SELECT 1 FROM players WHERE id = $1 FOR UPDATE", player_id)
        return await self._active_lobby_of(conn, player_id)

    async def _read_lobby(self, conn: asyncpg.Connection, lobby_id: str, viewer: str | None = None) -> Lobby | None:
        row = await conn.fetchrow("SELECT id, code, status, game_id, created_at FROM lobbies WHERE id = $1", lobby_id)
        if row is None:
            return None
        members = await conn.fetch(
            """
            SELECT p.id, p.nickname, lp.seat FROM lobby_players lp
            JOIN players p ON p.id = lp.player_id
            WHERE lp.lobby_id = $1
            ORDER BY lp.seat
            """,
            lobby_id,
        )
        players = [LobbyPlayer(id=m["id"], nickname=m["nickname"], seat=m["seat"]) for m in members]
        game = None
        if row["status"] == "IN_GAME" and any(p.id == viewer for p in players):
            game = self._game_access(row["game_id"], viewer)
        return Lobby(
            id=row["id"],
            code=row["code"],
            status=row["status"],
            players=players,
            player_count=len(players),
            max_players=self._settings.lobby_size,
            created_at=row["created_at"],
            game=game,
        )
