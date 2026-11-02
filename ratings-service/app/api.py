"""Rutas públicas. Son solo de lectura: el ranking cambia únicamente por eventos del broker."""

from __future__ import annotations

from typing import Annotated

from fastapi import APIRouter, Depends, Query, Request

from .schemas import GameMode, Leaderboard, PlayerRatings, RatingHistory
from .service import RatingsService

public = APIRouter(prefix="/api/ratings")


def get_service(request: Request) -> RatingsService:
    return request.app.state.service


Service = Annotated[RatingsService, Depends(get_service)]
Limit = Annotated[int, Query(ge=1, le=100)]


@public.get("/leaderboard", tags=["ranking"])
async def leaderboard(service: Service, mode: GameMode = "CLASSIC", limit: Limit = 20) -> Leaderboard:
    """Mejores jugadores de un modo. Los empates comparten posición."""
    return await service.leaderboard(mode, limit)


@public.get("/players/{account_id}", tags=["ranking"])
async def player(account_id: str, service: Service) -> PlayerRatings:
    """Puntajes de una cuenta en cada modo. Sin partidas puntuadas, `ratings` llega vacío."""
    return await service.player(account_id)


@public.get("/players/{account_id}/history", tags=["ranking"])
async def history(account_id: str, service: Service, limit: Limit = 20) -> RatingHistory:
    """Cambios de puntaje de una cuenta, del más reciente al más antiguo."""
    return await service.history(account_id, limit)
