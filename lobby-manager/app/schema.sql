-- Esquema del Lobby Manager. Se aplica al arrancar el servicio y es idempotente.
-- Descripción de cada tabla: docs/MODELO_DATOS.md

CREATE TABLE IF NOT EXISTS players (
    id          TEXT PRIMARY KEY,
    nickname    TEXT NOT NULL CHECK (char_length(nickname) BETWEEN 1 AND 20),
    account_id  TEXT UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE players ADD COLUMN IF NOT EXISTS account_id TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS players_account_id ON players (account_id);

CREATE TABLE IF NOT EXISTS lobbies (
    id          TEXT PRIMARY KEY,
    code        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'WAITING' CHECK (status IN ('WAITING', 'IN_GAME', 'CLOSED')),
    game_id     TEXT UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at  TIMESTAMPTZ,
    closed_at   TIMESTAMPTZ,
    CHECK ((status = 'WAITING') = (game_id IS NULL))
);

-- El código solo debe ser único entre las salas vivas; una sala cerrada lo libera.
CREATE UNIQUE INDEX IF NOT EXISTS lobbies_active_code ON lobbies (code) WHERE status <> 'CLOSED';

CREATE TABLE IF NOT EXISTS lobby_players (
    lobby_id    TEXT NOT NULL REFERENCES lobbies (id) ON DELETE CASCADE,
    player_id   TEXT NOT NULL REFERENCES players (id),
    seat        SMALLINT NOT NULL,
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (lobby_id, player_id),
    UNIQUE (lobby_id, seat)
);

CREATE INDEX IF NOT EXISTS lobby_players_player ON lobby_players (player_id);

-- game_id como clave primaria es lo que hace idempotente el registro del resultado (RNF-04).
CREATE TABLE IF NOT EXISTS game_results (
    game_id     TEXT PRIMARY KEY,
    lobby_id    TEXT NOT NULL UNIQUE REFERENCES lobbies (id),
    winner_id   TEXT REFERENCES players (id),
    end_reason  TEXT,
    payload     JSONB NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
