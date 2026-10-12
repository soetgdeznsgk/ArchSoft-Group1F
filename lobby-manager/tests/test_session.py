"""RF-01: ingreso con nombre de jugador, sin cuenta ni contraseña."""

from __future__ import annotations

import pytest

from app import security


async def test_ingresa_con_un_nombre_y_recibe_un_token(ctx):
    r = await ctx.client.post("/api/session", json={"nickname": "  Ana   María "})
    assert r.status_code == 201
    body = r.json()
    assert body["player"]["nickname"] == "Ana María"
    assert body["player"]["id"].startswith("p_")

    me = await ctx.client.get("/api/session", headers={"Authorization": f"Bearer {body['token']}"})
    assert me.status_code == 200
    assert me.json() == {"player": body["player"], "lobby": None}


@pytest.mark.parametrize("nickname", ["", "    ", "x" * 21])
async def test_rechaza_un_nombre_vacio_o_demasiado_largo(ctx, nickname):
    r = await ctx.client.post("/api/session", json={"nickname": nickname})
    assert r.status_code == 422
    assert r.json()["code"] == "invalid_nickname"
    assert r.json()["message"]


async def test_dos_jugadores_pueden_usar_el_mismo_nombre(ctx):
    a = await ctx.login("Ana")
    b = await ctx.login("Ana")
    assert a.id != b.id


async def test_solicitud_mal_formada_responde_con_el_formato_de_error_comun(ctx):
    r = await ctx.client.post("/api/session", json={})
    assert r.status_code == 422
    assert r.json()["code"] == "invalid_request"


async def test_sin_token_o_con_token_invalido_responde_401(ctx):
    assert (await ctx.client.get("/api/lobbies")).status_code == 401
    r = await ctx.client.get("/api/lobbies", headers={"Authorization": "Bearer no-es-un-jwt"})
    assert r.status_code == 401
    assert r.json()["code"] == "invalid_session"


async def test_el_token_de_partida_no_sirve_como_sesion(ctx):
    user = await ctx.login("Ana")
    game_token = security.issue_game_token(ctx.settings, "g_x", user.id)
    r = await ctx.client.get("/api/session", headers={"Authorization": f"Bearer {game_token}"})
    assert r.status_code == 401


async def test_token_de_un_jugador_que_ya_no_existe_responde_401(ctx):
    user = await ctx.login("Ana")
    await ctx.pool.execute("DELETE FROM players WHERE id = $1", user.id)
    r = await ctx.client.get("/api/session", headers=user.headers)
    assert r.status_code == 401


async def test_healthz(ctx):
    r = await ctx.client.get("/healthz")
    assert r.status_code == 200
    assert r.json() == {"status": "ok", "postgres": "ok"}
