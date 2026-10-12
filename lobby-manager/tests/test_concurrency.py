"""Peticiones simultáneas: los límites se cumplen aunque todos lleguen a la vez."""

from __future__ import annotations

import asyncio
from collections import Counter

from .conftest import SERVICE_HEADERS
from .test_result import result_body


async def test_ocho_jugadores_compiten_por_los_tres_cupos_de_una_sala(ctx):
    lobby, _ = await ctx.lobby_with(1)
    rivals = [await ctx.login(f"Rival{i}") for i in range(8)]

    responses = await asyncio.gather(
        *(ctx.client.post(f"/api/lobbies/{lobby['id']}/join", headers=u.headers) for u in rivals)
    )

    assert Counter(r.status_code for r in responses) == {200: 3, 409: 5}
    assert {r.json()["code"] for r in responses if r.status_code == 409} == {"lobby_full"}
    seats = await ctx.pool.fetch("SELECT seat FROM lobby_players WHERE lobby_id = $1 ORDER BY seat", lobby["id"])
    assert [s["seat"] for s in seats] == [0, 1, 2, 3]


async def test_quince_creaciones_simultaneas_dejan_exactamente_10_salas(ctx):
    owners = [await ctx.login(f"Host{i}") for i in range(15)]

    responses = await asyncio.gather(*(ctx.client.post("/api/lobbies", headers=u.headers) for u in owners))

    assert Counter(r.status_code for r in responses) == {201: 10, 409: 5}
    assert await ctx.pool.fetchval("SELECT count(*) FROM lobbies") == 10
    assert len({r.json()["code"] for r in responses if r.status_code == 201}) == 10


async def test_un_jugador_no_queda_en_dos_salas_por_pedirlo_a_la_vez(ctx):
    lobbies = [(await ctx.lobby_with(1))[0] for _ in range(4)]
    ana = await ctx.login("Ana")

    responses = await asyncio.gather(
        *(ctx.client.post(f"/api/lobbies/{l['id']}/join", headers=ana.headers) for l in lobbies)
    )

    assert Counter(r.status_code for r in responses) == {200: 1, 409: 3}
    assert await ctx.pool.fetchval("SELECT count(*) FROM lobby_players WHERE player_id = $1", ana.id) == 1


async def test_cuatro_inicios_simultaneos_crean_una_sola_partida(ctx):
    lobby, users = await ctx.lobby_with(4)

    responses = await asyncio.gather(
        *(ctx.client.post(f"/api/lobbies/{lobby['id']}/start", headers=u.headers) for u in users)
    )

    assert [r.status_code for r in responses] == [200] * 4
    assert len({r.json()["game"]["gameId"] for r in responses}) == 1
    assert len(ctx.game.requests) == 1


async def test_entrar_y_salir_a_la_vez_no_deja_salas_huerfanas(ctx):
    lobby, (ana,) = await ctx.lobby_with(1)
    beto = await ctx.login("Beto")

    leave, join = await asyncio.gather(
        ctx.client.post(f"/api/lobbies/{lobby['id']}/leave", headers=ana.headers),
        ctx.client.post(f"/api/lobbies/{lobby['id']}/join", headers=beto.headers),
    )

    assert leave.status_code == 204
    # Según quién llegue primero: Beto queda solo en la sala, o la sala ya no existía.
    assert join.status_code in (200, 404)
    members = await ctx.pool.fetch("SELECT player_id FROM lobby_players")
    assert [m["player_id"] for m in members] == ([beto.id] if join.status_code == 200 else [])
    assert await ctx.pool.fetchval("SELECT count(*) FROM lobbies") == len(members)


async def test_el_resultado_reintentado_en_paralelo_se_registra_una_vez(ctx):
    lobby, users = await ctx.started_lobby()
    body = result_body(lobby, users)

    responses = await asyncio.gather(
        *(
            ctx.client.post(f"/internal/lobbies/{lobby['id']}/result", json=body, headers=SERVICE_HEADERS)
            for _ in range(6)
        )
    )

    assert [r.status_code for r in responses] == [200] * 6
    assert Counter(r.json()["recorded"] for r in responses) == {True: 1, False: 5}
    assert await ctx.pool.fetchval("SELECT count(*) FROM game_results") == 1
