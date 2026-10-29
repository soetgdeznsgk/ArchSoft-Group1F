"""Modos de juego que ofrece el Lobby. Los nombres coinciden con los de Game Logic (Go).

- CLASSIC: 5 rondas de tríos y cuartetos. Exactamente `Settings.lobby_size` jugadores (4).
- CHAMELEON: un repartidor tira cartas de colores; gana quien reúne los 8. De 2 a 4 jugadores.
  Reglas: game-logic/docs/MODO_CAMALEON.md
"""

from __future__ import annotations

from typing import Literal

from .config import Settings

GameMode = Literal["CLASSIC", "CHAMELEON"]
DEFAULT_MODE: GameMode = "CLASSIC"

CHAMELEON_MIN_PLAYERS = 2
CHAMELEON_MAX_PLAYERS = 4


def player_limits(settings: Settings, mode: str) -> tuple[int, int]:
    """(mínimo para iniciar, máximo de la sala) del modo."""
    if mode == "CHAMELEON":
        return CHAMELEON_MIN_PLAYERS, CHAMELEON_MAX_PLAYERS
    return settings.lobby_size, settings.lobby_size
