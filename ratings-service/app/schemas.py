from __future__ import annotations

from datetime import datetime
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field
from pydantic.alias_generators import to_camel

GameMode = Literal["CLASSIC", "CHAMELEON"]


class ApiModel(BaseModel):
    model_config = ConfigDict(alias_generator=to_camel, populate_by_name=True)


class FinishedPlayer(ApiModel):
    player_id: str
    account_id: str | None = None
    nickname: str = Field(min_length=1, max_length=20)
    rank: int = Field(ge=1)


class GameFinished(ApiModel):
    """Evento game.finished que publica el Lobby Manager."""

    game_id: str = Field(min_length=1)
    mode: GameMode
    finished_at: datetime
    # No se deduce de `rank`: Camaleón puede terminar sin ganador con todos en rank 1.
    winners: list[str] = Field(default_factory=list)
    players: list[FinishedPlayer]


class LeaderboardEntry(ApiModel):
    position: int
    account_id: str
    nickname: str
    rating: int
    games: int
    wins: int


class Leaderboard(ApiModel):
    mode: GameMode
    players: list[LeaderboardEntry]


class ModeRating(ApiModel):
    mode: GameMode
    position: int
    rating: int
    games: int
    wins: int


class PlayerRatings(ApiModel):
    account_id: str
    nickname: str | None = None
    ratings: list[ModeRating]


class RatingChange(ApiModel):
    game_id: str
    mode: GameMode
    rank: int
    rating_before: int
    rating_after: int
    finished_at: datetime


class RatingHistory(ApiModel):
    account_id: str
    changes: list[RatingChange]
