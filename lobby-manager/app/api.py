"""Rutas HTTP. Las públicas van bajo /api/ (Traefik no recorta el prefijo); las internas no se enrutan."""

from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator
from typing import Annotated

from fastapi import APIRouter, Depends, Header, Query, Request, Response
from fastapi.responses import StreamingResponse
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer

from . import errors, security
from .events import format_sse
from .schemas import (
    ErrorOut,
    AuthSessionIn,
    JoinByCodeIn,
    Lobby,
    LobbySummary,
    MeOut,
    Player,
    ResultIn,
    ResultOut,
    SessionIn,
    SessionOut,
)
from .service import LobbyService, game_starting_event, lobby_event

public = APIRouter(prefix="/api")
internal = APIRouter(prefix="/internal", tags=["interno (Game Logic)"])

bearer = HTTPBearer(auto_error=False, description="Token de sesión devuelto por POST /api/session.")


def _error(*statuses: int) -> dict:
    return {status: {"model": ErrorOut} for status in statuses}


def get_service(request: Request) -> LobbyService:
    return request.app.state.service


Service = Annotated[LobbyService, Depends(get_service)]


async def _player_from(request: Request, token: str | None) -> Player:
    if not token:
        raise errors.invalid_session()
    player_id = security.read_session_token(request.app.state.settings, token)
    player = await request.app.state.service.get_player(player_id)
    if player is None:  # p. ej. la base se recreó y el navegador conserva un token viejo
        raise errors.invalid_session()
    return player


async def current_player(
    request: Request, credentials: Annotated[HTTPAuthorizationCredentials | None, Depends(bearer)]
) -> Player:
    return await _player_from(request, credentials.credentials if credentials else None)


async def current_player_sse(
    request: Request,
    credentials: Annotated[HTTPAuthorizationCredentials | None, Depends(bearer)],
    token: Annotated[
        str | None, Query(description="Token de sesión. EventSource no permite enviar cabeceras.")
    ] = None,
) -> Player:
    return await _player_from(request, credentials.credentials if credentials else token)


CurrentPlayer = Annotated[Player, Depends(current_player)]


# --- sesión (RF-01) ----------------------------------------------------------


@public.post("/session", status_code=201, tags=["sesión"], responses=_error(422))
async def create_session(body: SessionIn, service: Service) -> SessionOut:
    """Ingresa con un nombre de jugador, sin cuenta ni contraseña."""
    return await service.create_session(body.nickname)


@internal.post("/auth/session", status_code=201, responses=_error(401, 422))
async def create_account_session(
    body: AuthSessionIn,
    request: Request,
    service: Service,
    x_service_token: Annotated[str | None, Header()] = None,
) -> SessionOut:
    """Provisiona idempotentemente un jugador para una cuenta autenticada por Auth."""
    security.check_service_token(request.app.state.settings, x_service_token)
    return await service.create_account_session(body.account_id, body.nickname)


@public.get("/session", tags=["sesión"], responses=_error(401))
async def get_session(player: CurrentPlayer, service: Service) -> MeOut:
    """Quién soy y en qué sala estoy. Sirve para restaurar la pantalla tras recargar la página."""
    return MeOut(player=player, lobby=await service.current_lobby(player.id))


# --- salas (RF-02 a RF-09) ---------------------------------------------------


@public.get("/lobbies", tags=["salas"], responses=_error(401))
async def list_lobbies(player: CurrentPlayer, service: Service) -> list[LobbySummary]:
    """Salas en espera, con su código y cantidad de jugadores. Las llenas traen playerCount == maxPlayers."""
    return await service.list_lobbies()


@public.post("/lobbies", status_code=201, tags=["salas"], responses=_error(401, 409))
async def create_lobby(player: CurrentPlayer, service: Service) -> Lobby:
    """Crea una sala y deja al jugador dentro. 409 `lobby_limit_reached` si ya hay 10 salas."""
    return await service.create_lobby(player)


@public.post("/lobbies/join", tags=["salas"], responses=_error(401, 404, 409))
async def join_by_code(body: JoinByCodeIn, player: CurrentPlayer, service: Service) -> Lobby:
    """Entra a una sala escribiendo su código (no distingue mayúsculas)."""
    return await service.join_lobby(player, code=body.code)


@public.get("/lobbies/{lobby_id}", tags=["salas"], responses=_error(401, 404))
async def get_lobby(lobby_id: str, player: CurrentPlayer, service: Service) -> Lobby:
    """Estado de la sala. Si la partida ya inició y eres jugador, `game` trae tu acceso al WebSocket."""
    return await service.get_lobby(lobby_id, viewer=player.id)


@public.post("/lobbies/{lobby_id}/join", tags=["salas"], responses=_error(401, 404, 409))
async def join_lobby(lobby_id: str, player: CurrentPlayer, service: Service) -> Lobby:
    """Entra a una sala de la lista. 409 `lobby_full`, `lobby_started` o `already_in_lobby`."""
    return await service.join_lobby(player, lobby_id=lobby_id)


@public.post("/lobbies/{lobby_id}/leave", status_code=204, tags=["salas"], responses=_error(401, 403, 404, 409))
async def leave_lobby(lobby_id: str, player: CurrentPlayer, service: Service) -> Response:
    """Sale de la sala antes de que inicie la partida. La sala se elimina si queda vacía."""
    await service.leave_lobby(player.id, lobby_id)
    return Response(status_code=204)


@public.post("/lobbies/{lobby_id}/start", tags=["salas"], responses=_error(401, 403, 404, 409, 502))
async def start_game(lobby_id: str, player: CurrentPlayer, service: Service) -> Lobby:
    """Inicia la partida. Cualquier jugador de la sala puede hacerlo cuando están los 4.

    Los demás se enteran por el evento SSE `game_starting`, que trae el token de cada uno.
    """
    return await service.start_game(player, lobby_id)


@public.get(
    "/lobbies/{lobby_id}/events",
    tags=["salas"],
    response_class=StreamingResponse,
    responses={
        200: {"content": {"text/event-stream": {}}, "description": "Eventos `lobby`, `game_starting` y `lobby_closed`."},
        **_error(401, 403, 404),
    },
)
async def lobby_events(
    lobby_id: str, request: Request, player: Annotated[Player, Depends(current_player_sse)], service: Service
) -> StreamingResponse:
    """Sala de espera en tiempo real (SSE). Solo para jugadores de la sala.

    Al conectar llega el estado actual, así que reconectar nunca pierde información.
    """
    lobby = await service.get_lobby(lobby_id, viewer=player.id)
    if all(p.id != player.id for p in lobby.players):
        raise errors.not_in_lobby()
    return StreamingResponse(
        _event_stream(request, lobby_id, player.id),
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
    )


async def _event_stream(request: Request, lobby_id: str, player_id: str) -> AsyncIterator[str]:
    service: LobbyService = request.app.state.service
    events = request.app.state.events
    keepalive = request.app.state.settings.sse_keepalive_s

    # Primero suscribirse y después leer: así ningún cambio cae entre la foto inicial y el stream.
    sub = events.subscribe(lobby_id, player_id)
    try:
        try:
            lobby = await service.get_lobby(lobby_id, viewer=player_id)
        except errors.ApiError:
            return
        yield format_sse("lobby", lobby_event(lobby))
        if lobby.game is not None:
            yield format_sse("game_starting", game_starting_event(lobby_id, lobby.game))
        if lobby.status == "CLOSED":
            yield format_sse("lobby_closed", {"lobbyId": lobby_id, "reason": "finished"})
            return
        while True:
            try:
                item = await asyncio.wait_for(sub.queue.get(), timeout=keepalive)
            except TimeoutError:
                yield ": ping\n\n"  # mantiene viva la conexión a través del gateway
                continue
            if item is None:
                return
            yield format_sse(*item)
    finally:
        events.unsubscribe(sub)


# --- interno: resultado de la partida (RF-36, RNF-04) ------------------------


@internal.post("/lobbies/{lobby_id}/result", responses=_error(401, 404, 409, 422))
async def record_result(
    lobby_id: str,
    body: ResultIn,
    request: Request,
    service: Service,
    x_service_token: Annotated[str | None, Header()] = None,
) -> ResultOut:
    """Go reporta el resultado final. Idempotente por `gameId`: un reintento responde 200 con `recorded: false`."""
    security.check_service_token(request.app.state.settings, x_service_token)
    return await service.record_result(lobby_id, body)
