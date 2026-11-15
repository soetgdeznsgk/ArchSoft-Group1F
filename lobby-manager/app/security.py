"""Tokens: verificación de la sesión que emite Auth y emisión del token de partida para Go.

Cada tipo de token tiene un solo emisor y se firma con RS256 (decisiones 4 y 5):
el Lobby solo tiene la clave pública de sesiones, así que puede verificarlas pero no crearlas,
y es el único que tiene la clave privada de los tokens de partida.
"""

from __future__ import annotations

import time
from dataclasses import dataclass
from functools import lru_cache

import jwt
from cryptography.hazmat.primitives.asymmetric.rsa import RSAPrivateKey, RSAPublicKey
from cryptography.hazmat.primitives.serialization import load_pem_private_key, load_pem_public_key

from . import errors
from .config import Settings

ALGORITHM = "RS256"
SESSION_SCOPE = "session"
NICKNAME_MAX = 20


@dataclass(frozen=True)
class Session:
    """Lo que el Lobby sabe del jugador por su token de sesión."""

    player_id: str
    nickname: str
    account_id: str | None = None


@lru_cache(maxsize=4)
def _public_key(pem: str) -> RSAPublicKey:
    key = load_pem_public_key(pem.encode())
    if not isinstance(key, RSAPublicKey):
        raise ValueError("La clave pública de sesiones no es RSA")
    return key


@lru_cache(maxsize=4)
def _private_key(pem: str) -> RSAPrivateKey:
    key = load_pem_private_key(pem.encode(), password=None)
    if not isinstance(key, RSAPrivateKey):
        raise ValueError("La clave privada de partidas no es RSA")
    return key


def load_keys(settings: Settings) -> None:
    """Falla al arrancar, y no en la primera petición, si alguna clave no sirve."""
    _public_key(settings.session_public_key)
    _private_key(settings.game_private_key)


def read_session_token(settings: Settings, token: str) -> Session:
    """El token de partida no sirve como sesión: lo firma otra clave y no trae scope."""
    try:
        claims = jwt.decode(
            token,
            _public_key(settings.session_public_key),
            algorithms=[ALGORITHM],
            options={"require": ["exp", "sub"]},
        )
    except jwt.InvalidTokenError:
        raise errors.invalid_session() from None
    player_id, nickname, account_id = claims["sub"], claims.get("nickname"), claims.get("accountId")
    if (
        claims.get("scope") != SESSION_SCOPE
        or not isinstance(player_id, str)
        or not player_id
        or not isinstance(nickname, str)
        or not 1 <= len(nickname) <= NICKNAME_MAX
        or (account_id is not None and not isinstance(account_id, str))
    ):
        raise errors.invalid_session()
    return Session(player_id=player_id, nickname=nickname, account_id=account_id)


def issue_game_token(settings: Settings, game_id: str, player_id: str) -> str:
    """JWT que Go valida en el upgrade del WebSocket con la clave pública de partidas."""
    now = int(time.time())
    claims = {"gameId": game_id, "playerId": player_id, "iat": now, "exp": now + settings.game_token_ttl_s}
    return jwt.encode(claims, _private_key(settings.game_private_key), algorithm=ALGORITHM)
