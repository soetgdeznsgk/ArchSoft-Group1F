"""Configuración del servicio, leída del entorno."""

from __future__ import annotations

import os
from dataclasses import dataclass
from urllib.parse import quote


@dataclass(frozen=True)
class Settings:
    database_url: str
    rabbitmq_url: str

    initial_rating: float = 1000
    # Máximo que se puede ganar o perder en una partida.
    k_factor: float = 32
    # Con menos cuentas no hay rivales reales: los bots e invitados no dan puntos.
    min_rated_accounts: int = 2

    db_connect_timeout_s: float = 30
    broker_retry_s: float = 3

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
            host = os.environ.get("POSTGRES_HOST", "ratings-postgres")
            port = os.environ.get("POSTGRES_PORT", "5432")
            database = required("POSTGRES_DB")
            database_url = f"postgresql://{user}:{password}@{host}:{port}/{database}"

        return cls(database_url=database_url, rabbitmq_url=required("RABBITMQ_URL"))
