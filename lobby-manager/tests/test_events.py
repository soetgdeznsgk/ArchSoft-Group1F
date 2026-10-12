"""RF-08: sala de espera en tiempo real por SSE, y aviso de inicio y de cierre."""

from __future__ import annotations

import asyncio
from dataclasses import replace

import jwt

from .conftest import JWT_SECRET, SERVICE_HEADERS, make_settings, running
from .test_result import result_body


def nicknames(lobby: dict) -> list[str]:
    return [p["nickname"] for p in lobby["players"]]


async def test_la_sala_de_espera_se_actualiza_cuando_alguien_entra_o_sale(ctx):
    lobby, (ana,) = await ctx.lobby_with(1)
    beto = await ctx.login("Beto")

    async with ctx.events(ana, lobby["id"]) as stream:
        name, data = await stream.next()
        assert name == "lobby" and nicknames(data) == ["Jugador1"]

        await ctx.client.post(f"/api/lobbies/{lobby['id']}/join", headers=beto.headers)
        name, data = await stream.next()
        assert name == "lobby" and nicknames(data) == ["Jugador1", "Beto"]
        assert data["playerCount"] == 2

        await ctx.client.post(f"/api/lobbies/{lobby['id']}/leave", headers=beto.headers)
        name, data = await stream.next()
        assert name == "lobby" and nicknames(data) == ["Jugador1"]


async def test_el_stream_tiene_las_cabeceras_de_sse(ctx):
    lobby, (ana,) = await ctx.lobby_with(1)
    async with ctx.client.stream(
        "GET", f"/api/lobbies/{lobby['id']}/events", headers=ana.headers, timeout=None
    ) as response:
        assert response.status_code == 200
        assert response.headers["content-type"].startswith("text/event-stream")
        assert response.headers["cache-control"] == "no-cache"
        assert "content-encoding" not in response.headers


async def test_solo_los_jugadores_de_la_sala_pueden_escuchar(ctx):
    lobby, _ = await ctx.lobby_with(1)
    ajeno = await ctx.login("Ajeno")
    url = f"/api/lobbies/{lobby['id']}/events"

    assert (await ctx.client.get(url)).status_code == 401
    assert (await ctx.client.get(url, params={"token": "basura"})).status_code == 401
    r = await ctx.client.get(url, params={"token": ajeno.token})
    assert (r.status_code, r.json()["code"]) == (403, "not_in_lobby")
    r = await ctx.client.get("/api/lobbies/l_nada/events", params={"token": ajeno.token})
    assert r.status_code == 404


async def test_al_iniciar_cada_jugador_recibe_su_propio_token(ctx):
    lobby, users = await ctx.lobby_with(4)

    async with ctx.events(users[0], lobby["id"]) as first, ctx.events(users[3], lobby["id"]) as last:
        await first.next()
        await last.next()
        r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/start", headers=users[1].headers)
        game_id = r.json()["game"]["gameId"]

        for stream, user in ((first, users[0]), (last, users[3])):
            name, data = await stream.next()
            assert name == "lobby" and data["status"] == "IN_GAME"
            assert data["game"] is None  # el estado difundido no lleva el token de nadie

            name, data = await stream.next()
            assert name == "game_starting"
            assert (data["lobbyId"], data["gameId"], data["wsPath"]) == (lobby["id"], game_id, f"/ws/games/{game_id}")
            claims = jwt.decode(data["token"], JWT_SECRET, algorithms=["HS256"])
            assert (claims["gameId"], claims["playerId"]) == (game_id, user.id)


async def test_quien_se_conecta_con_la_partida_iniciada_recibe_el_aviso_de_inicio(ctx):
    lobby, users = await ctx.started_lobby()
    async with ctx.events(users[2], lobby["id"]) as stream:
        name, data = await stream.next()
        assert name == "lobby" and data["status"] == "IN_GAME"
        name, data = await stream.next()
        assert name == "game_starting"
        assert jwt.decode(data["token"], JWT_SECRET, algorithms=["HS256"])["playerId"] == users[2].id


async def test_el_resultado_avisa_el_cierre_y_termina_el_stream(ctx):
    lobby, users = await ctx.started_lobby()
    async with ctx.events(users[0], lobby["id"]) as stream:
        await stream.next_named("game_starting")
        body = result_body(lobby, users)
        await ctx.client.post(f"/internal/lobbies/{lobby['id']}/result", json=body, headers=SERVICE_HEADERS)

        name, data = await stream.next()
        assert name == "lobby_closed"
        assert data == {
            "lobbyId": lobby["id"],
            "gameId": body["gameId"],
            "reason": "finished",
            "winnerId": users[1].id,
            "winners": [users[1].id],
            "endReason": "WIN",
        }
        assert await stream.next() is None


async def test_salir_de_la_sala_termina_el_stream_propio(ctx):
    lobby, (ana, beto) = await ctx.lobby_with(2)
    async with ctx.events(beto, lobby["id"]) as stream:
        await stream.next()
        await ctx.client.post(f"/api/lobbies/{lobby['id']}/leave", headers=beto.headers)
        assert await stream.next() is None


async def test_envia_un_keepalive_cuando_no_hay_eventos():
    async with running(replace(make_settings(), sse_keepalive_s=0.1)) as ctx:
        lobby, (ana,) = await ctx.lobby_with(1)
        beto = await ctx.login("Beto")
        async with ctx.events(ana, lobby["id"]) as stream:
            await stream.next()
            await asyncio.sleep(0.35)
            await ctx.client.post(f"/api/lobbies/{lobby['id']}/join", headers=beto.headers)
            await stream.next()
            assert stream.pings >= 2


async def test_quien_cierra_la_pagina_sale_de_la_sala_tras_el_plazo_de_gracia():
    async with running(replace(make_settings(), presence_grace_s=0.3)) as ctx:
        lobby, (ana, beto) = await ctx.lobby_with(2)

        async with ctx.events(ana, lobby["id"]) as ana_stream:
            await ana_stream.next()
            async with ctx.events(beto, lobby["id"]) as beto_stream:
                await beto_stream.next()
            # Beto cerró la página sin pulsar "salir".
            name, data = await ana_stream.next()
            assert name == "lobby" and [p["id"] for p in data["players"]] == [ana.id]

        # Ana también se va: la sala queda vacía y se elimina, liberando su cupo.
        await asyncio.sleep(0.8)
        assert await ctx.pool.fetchval("SELECT count(*) FROM lobbies") == 0


async def test_recargar_la_pagina_dentro_del_plazo_no_saca_al_jugador():
    async with running(replace(make_settings(), presence_grace_s=0.4)) as ctx:
        lobby, (ana, beto) = await ctx.lobby_with(2)

        async with ctx.events(beto, lobby["id"]) as stream:
            await stream.next()
        async with ctx.events(beto, lobby["id"]) as stream:
            await stream.next()
            await asyncio.sleep(0.8)
            current = (await ctx.client.get(f"/api/lobbies/{lobby['id']}", headers=ana.headers)).json()
            assert [p["id"] for p in current["players"]] == [ana.id, beto.id]


async def test_ir_a_la_mesa_de_juego_no_saca_a_nadie_de_la_sala():
    async with running(replace(make_settings(), presence_grace_s=0.2)) as ctx:
        lobby, users = await ctx.lobby_with(4)
        async with ctx.events(users[0], lobby["id"]) as stream:
            await stream.next()
            await ctx.client.post(f"/api/lobbies/{lobby['id']}/start", headers=users[0].headers)
            await stream.next_named("game_starting")
        # El cliente cambia de pantalla y cierra el SSE; la sala sigue con sus 4 jugadores.
        await asyncio.sleep(0.6)
        current = (await ctx.client.get(f"/api/lobbies/{lobby['id']}", headers=users[0].headers)).json()
        assert (current["status"], current["playerCount"]) == ("IN_GAME", 4)
