"""RF-09: inicio de la partida, entrega a Game Logic y token por jugador."""

from __future__ import annotations

import httpx
import jwt

from .conftest import JWT_SECRET, SERVICE_SECRET


async def test_no_inicia_con_menos_de_4_jugadores_e_informa_cuantos_faltan(ctx):
    lobby, users = await ctx.lobby_with(2)
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/start", headers=users[0].headers)
    assert r.status_code == 409
    assert r.json() == {"code": "not_enough_players", "message": "Faltan 2 jugadores para iniciar la partida."}
    assert ctx.game.requests == []

    tercero = await ctx.login("Tercero")
    await ctx.client.post(f"/api/lobbies/{lobby['id']}/join", headers=tercero.headers)
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/start", headers=users[0].headers)
    assert r.json()["message"] == "Falta 1 jugador para iniciar la partida."


async def test_solo_un_jugador_de_la_sala_puede_iniciar(ctx):
    lobby, _ = await ctx.lobby_with(4)
    ajeno = await ctx.login("Ajeno")
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/start", headers=ajeno.headers)
    assert (r.status_code, r.json()["code"]) == (403, "not_in_lobby")
    assert ctx.game.requests == []


async def test_iniciar_entrega_la_sala_a_go_en_orden_de_asiento(ctx):
    lobby, users = await ctx.lobby_with(4)
    # Cualquier jugador de la sala puede iniciar, no solo quien la creó.
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/start", headers=users[2].headers)
    assert r.status_code == 200
    started = r.json()
    assert started["status"] == "IN_GAME"
    game_id = started["game"]["gameId"]
    assert started["game"]["wsPath"] == f"/ws/games/{game_id}"

    (request,) = ctx.game.requests
    assert request.method == "POST" and request.url.path == "/internal/games"
    assert request.headers["X-Service-Token"] == SERVICE_SECRET
    assert ctx.game.bodies[0] == {
        "gameId": game_id,
        "lobbyId": lobby["id"],
        "players": [{"id": u.id, "name": u.nickname} for u in users],
    }

    # La sala deja de aparecer como disponible.
    assert (await ctx.client.get("/api/lobbies", headers=users[0].headers)).json() == []


async def test_cada_jugador_obtiene_su_propio_token_de_partida(ctx):
    lobby, users = await ctx.started_lobby()
    game_id = lobby["game"]["gameId"]

    for user in users:
        game = (await ctx.client.get(f"/api/lobbies/{lobby['id']}", headers=user.headers)).json()["game"]
        claims = jwt.decode(game["token"], JWT_SECRET, algorithms=["HS256"])
        assert claims["gameId"] == game_id
        assert claims["playerId"] == user.id
        assert claims["exp"] - claims["iat"] == 2 * 3600

    ajeno = await ctx.login("Ajeno")
    seen = (await ctx.client.get(f"/api/lobbies/{lobby['id']}", headers=ajeno.headers)).json()
    assert seen["status"] == "IN_GAME" and seen["game"] is None


async def test_si_go_falla_la_sala_sigue_en_espera_y_se_puede_reintentar(ctx):
    lobby, users = await ctx.lobby_with(4)
    url = f"/api/lobbies/{lobby['id']}"

    ctx.game.responses = [503, 503]
    r = await ctx.client.post(f"{url}/start", headers=users[0].headers)
    assert (r.status_code, r.json()["code"]) == (502, "game_service_unavailable")
    assert (await ctx.client.get(url, headers=users[0].headers)).json()["status"] == "WAITING"

    ctx.game.responses = [httpx.ConnectError("sin conexión"), httpx.ConnectError("sin conexión")]
    r = await ctx.client.post(f"{url}/start", headers=users[0].headers)
    assert r.status_code == 502

    r = await ctx.client.post(f"{url}/start", headers=users[0].headers)
    assert r.status_code == 200 and r.json()["status"] == "IN_GAME"


async def test_un_fallo_pasajero_de_go_se_reintenta_con_el_mismo_gameid(ctx):
    lobby, users = await ctx.lobby_with(4)
    ctx.game.responses = [500]
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/start", headers=users[0].headers)
    assert r.status_code == 200
    first, second = ctx.game.bodies
    assert first == second


async def test_un_rechazo_4xx_de_go_no_se_reintenta(ctx):
    lobby, users = await ctx.lobby_with(4)
    ctx.game.responses = [400]
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/start", headers=users[0].headers)
    assert r.status_code == 502
    assert len(ctx.game.requests) == 1


async def test_iniciar_una_sala_ya_iniciada_devuelve_la_misma_partida(ctx):
    lobby, users = await ctx.started_lobby()
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/start", headers=users[3].headers)
    assert r.status_code == 200
    assert r.json()["game"]["gameId"] == lobby["game"]["gameId"]
    assert len(ctx.game.requests) == 1
