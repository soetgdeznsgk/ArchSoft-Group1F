"""Modelos de entrada y salida de la API. El JSON usa camelCase, igual que el contrato con Go."""

from __future__ import annotations

from datetime import datetime
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field
from pydantic.alias_generators import to_camel


class ApiModel(BaseModel):
    model_config = ConfigDict(alias_generator=to_camel, populate_by_name=True)


class ErrorOut(ApiModel):
    code: str = Field(examples=["lobby_full"])
    message: str = Field(examples=["La sala está llena."])


class SessionIn(ApiModel):
    nickname: str = Field(examples=["Ana"])


class AuthSessionIn(ApiModel):
    account_id: str = Field(examples=["34f1f808-6934-4c15-a267-b6f427cef015"])
    nickname: str = Field(examples=["ana"])


class Player(ApiModel):
    id: str = Field(examples=["p_3f9a1c2b7d4e"])
    nickname: str = Field(examples=["Ana"])


class GameAccess(ApiModel):
    """Lo que el cliente necesita para abrir el WebSocket de la partida."""

    game_id: str = Field(examples=["g_7f3a9c1e2b4d"])
    ws_path: str = Field(examples=["/ws/games/g_7f3a9c1e2b4d"])
    token: str = Field(description="JWT del jugador para Go; va en ?token= al abrir el WebSocket.")


class LobbyPlayer(Player):
    seat: int = Field(description="Orden de llegada a la sala; define el orden de la mesa.")


class Lobby(ApiModel):
    id: str = Field(examples=["l_0c5d8e1f2a3b"])
    code: str = Field(examples=["K7QM2X"])
    status: Literal["WAITING", "IN_GAME", "CLOSED"]
    players: list[LobbyPlayer]
    player_count: int
    max_players: int
    created_at: datetime
    game: GameAccess | None = Field(
        default=None, description="Solo si la partida inició y quien consulta es jugador de la sala."
    )


class LobbySummary(ApiModel):
    id: str
    code: str
    player_count: int
    max_players: int
    created_at: datetime


class SessionOut(ApiModel):
    player: Player
    token: str = Field(description="Token de sesión. Se envía como Authorization: Bearer <token>.")


class MeOut(ApiModel):
    player: Player
    lobby: Lobby | None = Field(description="Sala en la que está el jugador, si hay alguna.")


class JoinByCodeIn(ApiModel):
    code: str = Field(examples=["K7QM2X"])


class ResultIn(ApiModel):
    """Resultado que reporta Go. Los campos no listados se guardan tal cual en payload."""

    model_config = ConfigDict(alias_generator=to_camel, populate_by_name=True, extra="allow")

    game_id: str
    winner_id: str | None = None
    winners: list[str] = Field(default_factory=list, description="Más de uno solo si persiste el empate.")
    end_reason: str | None = None


class ResultOut(ApiModel):
    game_id: str
    lobby_id: str
    recorded: bool = Field(description="false si el resultado ya estaba registrado (reintento).")
