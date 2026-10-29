"""Modos de juego: CLASSIC (4 jugadores) y CHAMELEON (2 a 4 jugadores)."""

from __future__ import annotations

import json

import jwt

from .conftest import JWT_SECRET, SERVICE_HEADERS


async def test_sin_cuerpo_la_sala_es_clasica(ctx):
    lobby, _ = await ctx.lobby_with(1)
    assert (lobby["mode"], lobby["minPlayers"], lobby["maxPlayers"]) == ("CLASSIC", 4, 4)


async def test_crear_sala_camaleon_y_listarla_con_su_modo(ctx):
    camaleon, users = await ctx.lobby_with(1, mode="CHAMELEON")
    assert (camaleon["mode"], camaleon["minPlayers"], camaleon["maxPlayers"]) == ("CHAMELEON", 2, 4)
    clasica, _ = await ctx.lobby_with(1, mode="CLASSIC")
    listed = (await ctx.client.get("/api/lobbies", headers=users[0].headers)).json()
    assert [(l["id"], l["mode"], l["minPlayers"], l["maxPlayers"]) for l in listed] == [
        (camaleon["id"], "CHAMELEON", 2, 4),
        (clasica["id"], "CLASSIC", 4, 4),
    ]
    # El estado de la sala por GET y por SSE también trae el modo.
    r = await ctx.client.get(f"/api/lobbies/{camaleon['id']}", headers=users[0].headers)
    assert r.json()["mode"] == "CHAMELEON"
    async with ctx.events(users[0], camaleon["id"]) as stream:
        name, data = await stream.next()
        assert (name, data["mode"], data["minPlayers"]) == ("lobby", "CHAMELEON", 2)


async def test_modo_desconocido_se_rechaza(ctx):
    ana = await ctx.login("Ana")
    r = await ctx.client.post("/api/lobbies", headers=ana.headers, json={"mode": "POKER"})
    assert (r.status_code, r.json()["code"]) == (422, "invalid_request")
    assert (await ctx.client.get("/api/lobbies", headers=ana.headers)).json() == []


async def test_camaleon_no_inicia_con_un_solo_jugador(ctx):
    lobby, users = await ctx.lobby_with(1, mode="CHAMELEON")
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/start", headers=users[0].headers)
    assert r.status_code == 409
    assert r.json() == {"code": "not_enough_players", "message": "Falta 1 jugador para iniciar la partida."}
    assert ctx.game.requests == []


async def test_camaleon_inicia_con_dos_y_entrega_el_modo_a_go(ctx):
    lobby, users = await ctx.lobby_with(2, mode="CHAMELEON")
    async with ctx.events(users[1], lobby["id"]) as stream:
        await stream.next()
        r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/start", headers=users[0].headers)
        assert r.status_code == 200, r.text
        started = r.json()
        assert started["status"] == "IN_GAME" and started["game"]["mode"] == "CHAMELEON"
        # El aviso a los demás jugadores trae el modo para abrir la pantalla correcta.
        data = await stream.next_named("game_starting")
        assert data["mode"] == "CHAMELEON" and data["gameId"] == started["game"]["gameId"]
        assert jwt.decode(data["token"], JWT_SECRET, algorithms=["HS256"])["playerId"] == users[1].id

    assert ctx.game.bodies[0] == {
        "gameId": started["game"]["gameId"],
        "lobbyId": lobby["id"],
        "mode": "CHAMELEON",
        "players": [{"id": u.id, "name": u.nickname} for u in users],
    }


async def test_camaleon_admite_hasta_4_jugadores(ctx):
    lobby, _ = await ctx.lobby_with(4, mode="CHAMELEON")
    assert lobby["playerCount"] == 4
    quinto = await ctx.login("Quinto")
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/join", headers=quinto.headers)
    assert (r.status_code, r.json()["code"]) == (409, "lobby_full")


async def test_resultado_del_camaleon_se_guarda_con_sus_posiciones(ctx):
    lobby, users = await ctx.lobby_with(3, mode="CHAMELEON")
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/start", headers=users[0].headers)
    game_id = r.json()["game"]["gameId"]
    body = {
        "gameId": game_id,
        "mode": "CHAMELEON",
        "winnerId": users[2].id,
        "winners": [users[2].id],
        "endReason": "all_colors",
        "standings": [
            {"player": users[2].id, "colors": 8, "rank": 1},
            {"player": users[0].id, "colors": 3, "rank": 2},
            {"player": users[1].id, "colors": 0, "rank": 3},
        ],
    }
    r = await ctx.client.post(f"/internal/lobbies/{lobby['id']}/result", json=body, headers=SERVICE_HEADERS)
    assert r.status_code == 200 and r.json()["recorded"] is True
    row = await ctx.pool.fetchrow("SELECT winner_id, end_reason, payload FROM game_results WHERE game_id = $1", game_id)
    assert (row["winner_id"], row["end_reason"]) == (users[2].id, "all_colors")
    payload = json.loads(row["payload"])
    assert payload["mode"] == "CHAMELEON" and payload["standings"] == body["standings"]
    lobby_now = (await ctx.client.get(f"/api/lobbies/{lobby['id']}", headers=users[0].headers)).json()
    assert lobby_now["status"] == "CLOSED"


async def test_columna_mode_se_agrega_a_una_base_existente(ctx):
    """El esquema se aplica en cada arranque: volver a aplicarlo no falla y conserva el modo."""
    from app.db import SCHEMA

    lobby, _ = await ctx.lobby_with(1, mode="CHAMELEON")
    async with ctx.pool.acquire() as conn:
        await conn.execute(SCHEMA)
        assert await conn.fetchval("SELECT mode FROM lobbies WHERE id = $1", lobby["id"]) == "CHAMELEON"
