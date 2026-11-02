"""Elo para partidas de varios jugadores.

Cada partida se reparte en duelos entre todos los pares de jugadores: quedar por encima
de un rival vale 1, empatar 0.5 y quedar por debajo 0. La diferencia entre lo obtenido y
lo esperado según los puntajes se escala con K / (n - 1), para que el cambio máximo de
una partida no dependa de cuántos jugadores tuvo.
"""

from __future__ import annotations

from dataclasses import dataclass

SCALE = 400


@dataclass(frozen=True)
class Entry:
    account_id: str
    rating: float
    rank: int  # 1 = ganador; los empates comparten posición


def expected_score(rating: float, opponent: float) -> float:
    """Probabilidad de ganarle al rival: 0.5 con puntajes iguales."""
    return 1 / (1 + 10 ** ((opponent - rating) / SCALE))


def _duel_score(rank: int, opponent_rank: int) -> float:
    if rank == opponent_rank:
        return 0.5
    return 1.0 if rank < opponent_rank else 0.0


def rating_changes(entries: list[Entry], k_factor: float) -> dict[str, float]:
    """Cambio de puntaje de cada cuenta. La suma de todos los cambios es 0."""
    weight = k_factor / max(len(entries) - 1, 1)
    return {
        entry.account_id: weight
        * sum(
            _duel_score(entry.rank, other.rank) - expected_score(entry.rating, other.rating)
            for other in entries
            if other is not entry
        )
        for entry in entries
    }
