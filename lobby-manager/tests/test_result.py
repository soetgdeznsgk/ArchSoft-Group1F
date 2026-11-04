"""RF-36 y RNF-04: registrar el resultado una sola vez y cerrar la sala."""

from __future__ import annotations

import json
from dataclasses import replace

from .conftest import make_settings, running


def result_body(lobby: dict, users: list, **overrides) -> dict:
    body = {
        "gameId": lobby["game"]["gameId"],
        "winnerId": users[1].id,
        "endReason": "WIN",
        "standings": [
            {"player": u.id, "points": 10 * i, "rounds_won": 4 - i, "last_round_points": i, "rank": i + 1}
            for i, u in enumerate([users[1], users[0], users[2], users[3]])
        ],
    }
    body.update(overrides)
    return body


async def test_el_resultado_ya_no_se_recibe_por_http(ctx):
    """Go publica game.ended en RabbitMQ: el Lobby no expone una ruta que Go deba conocer."""
    lobby, users = await ctx.started_lobby()
    r = await ctx.client.post(f"/internal/lobbies/{lobby['id']}/result", json=result_body(lobby, users))
    assert r.status_code in (404, 405)
    assert await ctx.pool.fetchval("SELECT count(*) FROM game_results") == 0


async def test_un_evento_sin_sala_o_mal_formado_no_se_registra(ctx):
    lobby, users = await ctx.started_lobby()
    r = await ctx.report_result(lobby["id"], {"winnerId": users[0].id})
    assert (r.status_code, r.json()["code"]) == (422, "invalid_request")
    assert await ctx.pool.fetchval("SELECT count(*) FROM game_results") == 0


async def test_registra_el_resultado_y_cierra_la_sala(ctx):
    lobby, users = await ctx.started_lobby()
    body = result_body(lobby, users)

    r = await ctx.report_result(lobby["id"], body)
    assert r.status_code == 200
    assert r.json() == {"gameId": body["gameId"], "lobbyId": lobby["id"], "recorded": True}

    row = await ctx.pool.fetchrow("SELECT * FROM game_results WHERE game_id = $1", body["gameId"])
    assert (row["lobby_id"], row["winner_id"], row["end_reason"]) == (lobby["id"], users[1].id, "WIN")
    # Lo que Go envíe de más (aquí, la tabla de posiciones) se conserva tal cual.
    assert json.loads(row["payload"])["standings"] == body["standings"]

    closed = (await ctx.client.get(f"/api/lobbies/{lobby['id']}", headers=users[0].headers)).json()
    assert closed["status"] == "CLOSED" and closed["game"] is None

    # Los jugadores quedan libres para otra sala.
    assert (await ctx.client.get("/api/session", headers=users[0].headers)).json()["lobby"] is None
    assert (await ctx.client.post("/api/lobbies", headers=users[0].headers)).status_code == 201


async def test_un_reintento_no_registra_el_resultado_dos_veces(ctx):
    lobby, users = await ctx.started_lobby()
    lobby_id = lobby["id"]
    body = result_body(lobby, users)

    first = await ctx.report_result(lobby_id, body)
    # El reintento llega con otro contenido: gana lo que se registró primero.
    retry = await ctx.report_result(lobby_id, result_body(lobby, users, winnerId=users[2].id))

    assert (first.status_code, first.json()["recorded"]) == (200, True)
    assert (retry.status_code, retry.json()["recorded"]) == (200, False)
    rows = await ctx.pool.fetch("SELECT winner_id FROM game_results")
    assert [r["winner_id"] for r in rows] == [users[1].id]


async def test_cerrar_la_sala_libera_su_cupo_entre_las_10(ctx):
    lobby, users = await ctx.started_lobby()
    for i in range(9):
        owner = await ctx.login(f"Host{i}")
        assert (await ctx.client.post("/api/lobbies", headers=owner.headers)).status_code == 201

    extra = await ctx.login("Extra")
    # Una sala con la partida en curso sigue ocupando cupo.
    assert (await ctx.client.post("/api/lobbies", headers=extra.headers)).json()["code"] == "lobby_limit_reached"

    r = await ctx.report_result(lobby["id"], result_body(lobby, users))
    assert r.status_code == 200
    assert (await ctx.client.post("/api/lobbies", headers=extra.headers)).status_code == 201


async def test_rechaza_un_resultado_que_no_corresponde(ctx):
    lobby, users = await ctx.started_lobby()
    waiting, _ = await ctx.lobby_with(2)
    body = result_body(lobby, users)

    r = await ctx.report_result("l_nada", body)
    assert (r.status_code, r.json()["code"]) == (404, "lobby_not_found")

    r = await ctx.report_result(lobby["id"], result_body(lobby, users, gameId="g_otro"))
    assert (r.status_code, r.json()["code"]) == (409, "game_mismatch")

    # Una sala que aún no inició no tiene partida.
    r = await ctx.report_result(waiting["id"], body)
    assert r.status_code == 409

    r = await ctx.report_result(lobby["id"], result_body(lobby, users, winnerId="p_intruso"))
    assert (r.status_code, r.json()["code"]) == (422, "invalid_result")

    assert await ctx.pool.fetchval("SELECT count(*) FROM game_results") == 0
    assert (await ctx.client.get(f"/api/lobbies/{lobby['id']}", headers=users[0].headers)).json()["status"] == "IN_GAME"


async def test_partida_sin_ganador_unico(ctx):
    lobby, users = await ctx.started_lobby()
    lobby_id = lobby["id"]

    # Empate persistente: Go reporta varios ganadores y no hay winnerId.
    body = result_body(lobby, users, winnerId=None, winners=[users[0].id, users[3].id])
    assert (await ctx.report_result(lobby_id, body)).status_code == 200
    row = await ctx.pool.fetchrow("SELECT winner_id, payload FROM game_results")
    assert row["winner_id"] is None
    assert json.loads(row["payload"])["winners"] == [users[0].id, users[3].id]


async def test_un_solo_ganador_en_winners_se_guarda_como_winner_id(ctx):
    lobby, users = await ctx.started_lobby()
    body = {"gameId": lobby["game"]["gameId"], "winners": [users[2].id]}
    r = await ctx.report_result(lobby["id"], body)
    assert r.status_code == 200
    assert await ctx.pool.fetchval("SELECT winner_id FROM game_results") == users[2].id


async def test_una_partida_que_nunca_reporta_resultado_se_cierra_sola():
    async with running(replace(make_settings(), game_ttl_s=3600)) as ctx:
        stale, users = await ctx.started_lobby()
        fresh, _ = await ctx.started_lobby()
        await ctx.pool.execute(
            "UPDATE lobbies SET started_at = now() - interval '2 hours' WHERE id = $1", stale["id"]
        )

        assert await ctx.app.state.service.expire_stale_games() == 1

        statuses = dict(await ctx.pool.fetch("SELECT id, status FROM lobbies"))
        assert statuses == {stale["id"]: "CLOSED", fresh["id"]: "IN_GAME"}

        # Si el resultado llega tarde, igual se registra.
        r = await ctx.report_result(stale["id"], result_body(stale, users))
        assert (r.status_code, r.json()["recorded"]) == (200, True)
