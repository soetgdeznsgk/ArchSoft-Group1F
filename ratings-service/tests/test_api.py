"""Rutas de consulta."""

from __future__ import annotations

from tests.conftest import game_event


async def test_el_ranking_vacio_responde_lista_vacia(ctx):
    r = await ctx.client.get("/api/ratings/leaderboard")

    assert r.status_code == 200
    assert r.json() == {"mode": "CLASSIC", "players": []}


async def test_modo_desconocido_y_limite_invalido_se_rechazan(ctx):
    mode = await ctx.client.get("/api/ratings/leaderboard", params={"mode": "POKER"})
    limit = await ctx.client.get("/api/ratings/leaderboard", params={"limit": 0})

    assert mode.status_code == limit.status_code == 422
    assert mode.json()["code"] == "invalid_request"


async def test_el_limite_recorta_el_ranking_y_conserva_la_posicion(ctx):
    await ctx.deliver(game_event([("ana", 1), ("beto", 2), ("caro", 3), ("dani", 4)]))

    r = await ctx.client.get("/api/ratings/leaderboard", params={"limit": 2})

    assert [(p["position"], p["nickname"]) for p in r.json()["players"]] == [(1, "ana"), (2, "beto")]


async def test_la_ficha_de_un_jugador_trae_cada_modo(ctx):
    await ctx.deliver(game_event([("ana", 1), ("beto", 2)]))
    await ctx.deliver(game_event([("ana", 2), ("beto", 1)], mode="CHAMELEON"))

    r = await ctx.client.get("/api/ratings/players/ana")

    body = r.json()
    assert body["nickname"] == "ana"
    assert [(m["mode"], m["position"], m["rating"]) for m in body["ratings"]] == [
        ("CHAMELEON", 2, 984),
        ("CLASSIC", 1, 1016),
    ]


async def test_una_cuenta_sin_partidas_responde_sin_puntajes(ctx):
    r = await ctx.client.get("/api/ratings/players/nadie")

    assert r.status_code == 200
    assert r.json() == {"accountId": "nadie", "nickname": None, "ratings": []}


async def test_el_historial_llega_del_mas_reciente_al_mas_antiguo(ctx):
    await ctx.deliver(game_event([("ana", 1), ("beto", 2)], game_id="g_primera"))
    await ctx.deliver(game_event([("ana", 2), ("beto", 1)], game_id="g_segunda"))

    r = await ctx.client.get("/api/ratings/players/ana/history")

    changes = r.json()["changes"]
    assert [c["gameId"] for c in changes] == ["g_segunda", "g_primera"]
    assert (changes[1]["ratingBefore"], changes[1]["ratingAfter"]) == (1000, 1016)
    assert changes[0]["ratingBefore"] == 1016


async def test_healthz_informa_el_estado_del_broker(ctx):
    r = await ctx.client.get("/healthz")

    assert r.status_code == 200
    assert r.json() == {"status": "ok", "postgres": "ok", "broker": "down"}
