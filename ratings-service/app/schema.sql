-- Esquema del Ratings Service. Se aplica al arrancar el servicio y es idempotente.

-- Puntaje actual de cada cuenta. Cada modo tiene su propio ranking.
CREATE TABLE IF NOT EXISTS ratings (
    account_id  TEXT NOT NULL,
    mode        TEXT NOT NULL CHECK (mode IN ('CLASSIC', 'CHAMELEON')),
    nickname    TEXT NOT NULL,
    rating      DOUBLE PRECISION NOT NULL,
    games       INTEGER NOT NULL DEFAULT 0,
    wins        INTEGER NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, mode)
);

CREATE INDEX IF NOT EXISTS ratings_leaderboard ON ratings (mode, rating DESC);

-- game_id como clave primaria hace idempotente el consumo: RabbitMQ entrega al menos una vez.
CREATE TABLE IF NOT EXISTS processed_games (
    game_id      TEXT PRIMARY KEY,
    mode         TEXT NOT NULL,
    rated        BOOLEAN NOT NULL,
    finished_at  TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Historial: cómo cambió el puntaje de cada cuenta en cada partida.
CREATE TABLE IF NOT EXISTS rating_changes (
    game_id       TEXT NOT NULL REFERENCES processed_games (game_id),
    account_id    TEXT NOT NULL,
    mode          TEXT NOT NULL,
    rank          SMALLINT NOT NULL,
    rating_before DOUBLE PRECISION NOT NULL,
    rating_after  DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (game_id, account_id)
);

CREATE INDEX IF NOT EXISTS rating_changes_account ON rating_changes (account_id, mode);
