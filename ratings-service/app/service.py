"""Aplica partidas terminadas al ranking y responde las consultas.

Cada partida se aplica en una transacción: si el mismo evento llega dos veces, el
INSERT en processed_games no inserta nada y la partida no vuelve a sumar puntos.
"""

from __future__ import annotations

import asyncpg

from .config import Settings
from .elo import Entry, rating_changes
from .schemas import (
    FinishedPlayer,
    GameFinished,
    GameMode,
    Leaderboard,
    LeaderboardEntry,
    ModeRating,
    PlayerRatings,
    RatingChange,
    RatingHistory,
)


class RatingsService:
    def __init__(self, pool: asyncpg.Pool, settings: Settings) -> None:
        self._pool = pool
        self._settings = settings

    async def apply_game(self, game: GameFinished) -> bool:
        """Devuelve True si la partida cambió el ranking."""
        accounts = self._accounts_of(game)
        rated = len(accounts) >= self._settings.min_rated_accounts

        async with self._pool.acquire() as conn, conn.transaction():
            is_new = await conn.fetchval(
                """
                INSERT INTO processed_games (game_id, mode, rated, finished_at)
                VALUES ($1, $2, $3, $4)
                ON CONFLICT (game_id) DO NOTHING
                RETURNING true
                """,
                game.game_id,
                game.mode,
                rated,
                game.finished_at,
            )
            if not is_new or not rated:
                return False
            await self._update_ratings(conn, game, accounts)
        return True

    async def leaderboard(self, mode: GameMode, limit: int) -> Leaderboard:
        rows = await self._pool.fetch(
            """
            SELECT account_id, nickname, rating, games, wins,
                   rank() OVER (ORDER BY rating DESC) AS position
            FROM ratings
            WHERE mode = $1
            ORDER BY rating DESC, nickname
            LIMIT $2
            """,
            mode,
            limit,
        )
        return Leaderboard(
            mode=mode,
            players=[
                LeaderboardEntry(
                    position=row["position"],
                    account_id=row["account_id"],
                    nickname=row["nickname"],
                    rating=round(row["rating"]),
                    games=row["games"],
                    wins=row["wins"],
                )
                for row in rows
            ],
        )

    async def player(self, account_id: str) -> PlayerRatings:
        rows = await self._pool.fetch(
            """
            SELECT * FROM (
                SELECT account_id, mode, nickname, rating, games, wins,
                       rank() OVER (PARTITION BY mode ORDER BY rating DESC) AS position
                FROM ratings
            ) ranked
            WHERE account_id = $1
            ORDER BY mode
            """,
            account_id,
        )
        return PlayerRatings(
            account_id=account_id,
            nickname=rows[0]["nickname"] if rows else None,
            ratings=[
                ModeRating(
                    mode=row["mode"],
                    position=row["position"],
                    rating=round(row["rating"]),
                    games=row["games"],
                    wins=row["wins"],
                )
                for row in rows
            ],
        )

    async def history(self, account_id: str, limit: int) -> RatingHistory:
        rows = await self._pool.fetch(
            """
            SELECT c.game_id, c.mode, c.rank, c.rating_before, c.rating_after, g.finished_at
            FROM rating_changes c JOIN processed_games g ON g.game_id = c.game_id
            WHERE c.account_id = $1
            ORDER BY g.finished_at DESC
            LIMIT $2
            """,
            account_id,
            limit,
        )
        return RatingHistory(
            account_id=account_id,
            changes=[
                RatingChange(
                    game_id=row["game_id"],
                    mode=row["mode"],
                    rank=row["rank"],
                    rating_before=round(row["rating_before"]),
                    rating_after=round(row["rating_after"]),
                    finished_at=row["finished_at"],
                )
                for row in rows
            ],
        )

    @staticmethod
    def _accounts_of(game: GameFinished) -> list[FinishedPlayer]:
        return [player for player in game.players if player.account_id]

    async def _update_ratings(
        self, conn: asyncpg.Connection, game: GameFinished, accounts: list[FinishedPlayer]
    ) -> None:
        # Se bloquea en orden de account_id: dos partidas simultáneas no pueden hacer un deadlock.
        accounts = sorted(accounts, key=lambda player: player.account_id)
        ids = [player.account_id for player in accounts]
        await conn.executemany(
            """
            INSERT INTO ratings (account_id, mode, nickname, rating)
            VALUES ($1, $2, $3, $4)
            ON CONFLICT (account_id, mode) DO NOTHING
            """,
            [(p.account_id, game.mode, p.nickname, self._settings.initial_rating) for p in accounts],
        )
        rows = await conn.fetch(
            """
            SELECT account_id, rating FROM ratings
            WHERE mode = $1 AND account_id = ANY($2::text[])
            ORDER BY account_id
            FOR UPDATE
            """,
            game.mode,
            ids,
        )
        before = {row["account_id"]: row["rating"] for row in rows}
        changes = rating_changes(
            [Entry(p.account_id, before[p.account_id], p.rank) for p in accounts], self._settings.k_factor
        )
        for player in accounts:
            after = before[player.account_id] + changes[player.account_id]
            await conn.execute(
                """
                UPDATE ratings
                SET rating = $3, nickname = $4, games = games + 1, wins = wins + $5, updated_at = now()
                WHERE account_id = $1 AND mode = $2
                """,
                player.account_id,
                game.mode,
                after,
                player.nickname,
                1 if player.player_id in game.winners else 0,
            )
            await conn.execute(
                """
                INSERT INTO rating_changes (game_id, account_id, mode, rank, rating_before, rating_after)
                VALUES ($1, $2, $3, $4, $5, $6)
                """,
                game.game_id,
                player.account_id,
                game.mode,
                player.rank,
                before[player.account_id],
                after,
            )
