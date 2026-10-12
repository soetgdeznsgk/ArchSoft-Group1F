"""Configuración del servicio, leída del entorno (docs/deployment.md §8)."""

from __future__ import annotations

import os
from dataclasses import dataclass
from urllib.parse import quote


@dataclass(frozen=True)
class Settings:
    database_url: str
    jwt_secret: str
    service_secret: str
    game_logic_url: str = "http://game-logic:8081"
    cors_origins: tuple[str, ...] = ()

    # Límites del reglamento (RF-03, RF-06). El motor de Go exige 4 jugadores.
    max_lobbies: int = 10
    lobby_size: int = 4

    session_ttl_s: int = 12 * 3600
    game_token_ttl_s: int = 2 * 3600  # igual al JWT_LIFETIME que espera Go
    sse_keepalive_s: float = 15
    # Tiempo que se espera a un jugador que cerró su SSE antes de sacarlo de la sala.
    presence_grace_s: float = 30
    # Una sala IN_GAME sin resultado se cierra sola pasado este tiempo.
    game_ttl_s: float = 3 * 3600
    game_request_timeout_s: float = 3
    db_connect_timeout_s: float = 30

    @classmethod
    def from_env(cls) -> Settings:
        def required(name: str) -> str:
            value = os.environ.get(name, "")
            if not value:
                raise RuntimeError(f"Falta la variable de entorno {name}")
            return value

        database_url = os.environ.get("DATABASE_URL")
        if not database_url:
            user = quote(required("POSTGRES_USER"), safe="")
            password = quote(required("POSTGRES_PASSWORD"), safe="")
            host = os.environ.get("POSTGRES_HOST", "postgres")
            port = os.environ.get("POSTGRES_PORT", "5432")
            database = required("POSTGRES_DB")
            database_url = f"postgresql://{user}:{password}@{host}:{port}/{database}"

        origins = tuple(o.strip() for o in os.environ.get("CORS_ORIGINS", "").split(",") if o.strip())
        optional = {}
        for env, field, cast in (
            ("GAME_LOGIC_URL", "game_logic_url", str),
            ("PRESENCE_GRACE_SECONDS", "presence_grace_s", float),
            ("GAME_TTL_SECONDS", "game_ttl_s", float),
        ):
            if os.environ.get(env):
                optional[field] = cast(os.environ[env])

        return cls(
            database_url=database_url,
            jwt_secret=required("JWT_SECRET"),
            service_secret=required("SERVICE_SECRET"),
            cors_origins=origins,
            **optional,
        )
