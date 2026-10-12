"""Tokens: sesión del jugador, token de partida para Go y secreto entre servicios."""

from __future__ import annotations

import hmac
import time

import jwt

from . import errors
from .config import Settings

ALGORITHM = "HS256"
SESSION_SCOPE = "session"


def issue_session_token(settings: Settings, player_id: str) -> str:
    now = int(time.time())
    claims = {"sub": player_id, "scope": SESSION_SCOPE, "iat": now, "exp": now + settings.session_ttl_s}
    return jwt.encode(claims, settings.jwt_secret, algorithm=ALGORITHM)


def read_session_token(settings: Settings, token: str) -> str:
    """Devuelve el playerId. El token de partida no sirve como sesión: no trae scope."""
    try:
        claims = jwt.decode(
            token, settings.jwt_secret, algorithms=[ALGORITHM], options={"require": ["exp", "sub"]}
        )
    except jwt.InvalidTokenError:
        raise errors.invalid_session() from None
    if claims.get("scope") != SESSION_SCOPE:
        raise errors.invalid_session()
    return claims["sub"]


def issue_game_token(settings: Settings, game_id: str, player_id: str) -> str:
    """JWT que Go valida en el upgrade del WebSocket (docs/game-logic-design.md §7.3)."""
    now = int(time.time())
    claims = {"gameId": game_id, "playerId": player_id, "iat": now, "exp": now + settings.game_token_ttl_s}
    return jwt.encode(claims, settings.jwt_secret, algorithm=ALGORITHM)


def check_service_token(settings: Settings, token: str | None) -> None:
    if not token or not hmac.compare_digest(token.encode(), settings.service_secret.encode()):
        raise errors.unauthorized_service()
