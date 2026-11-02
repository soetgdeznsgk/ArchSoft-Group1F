"""Consumo de game.finished: idempotencia, quién puntúa y qué pasa con los mensajes malos."""

from __future__ import annotations

import asyncpg
import pytest

from tests.conftest import game_event


async def test_una_partida_actualiza_puntajes_partidas_y_victorias(ctx):
    message = await ctx.deliver(game_event([("ana", 1), ("beto", 2)]))

    assert message.outcome == "ack"
    board = await ctx.board()
    assert [(p["nickname"], p["rating"], p["games"], p["wins"], p["position"]) for p in board] == [
        ("ana", 1016, 1, 1, 1),
        ("beto", 984, 1, 0, 2),
    ]


async def test_un_evento_repetido_no_suma_dos_veces(ctx):
    event = game_event([("ana", 1), ("beto", 2)])

    first = await ctx.deliver(event)
    second = await ctx.deliver(event)

    assert first.outcome == second.outcome == "ack"
    assert [(p["rating"], p["games"]) for p in await ctx.board()] == [(1016, 1), (984, 1)]


async def test_invitados_y_bots_no_puntuan_ni_regalan_puntos(ctx):
    await ctx.deliver(game_event([("ana", 1), (None, 2), (None, 3), ("beto", 4)]))

    assert {p["nickname"]: p["rating"] for p in await ctx.board()} == {"ana": 1016, "beto": 984}


async def test_una_cuenta_contra_bots_no_entra_al_ranking(ctx):
    message = await ctx.deliver(game_event([("ana", 1), (None, 2), (None, 3), (None, 4)]))

    assert message.outcome == "ack"
    assert await ctx.board() == []
    assert await ctx.app.state.pool.fetchval("SELECT rated FROM processed_games") is False


async def test_cada_modo_tiene_su_propio_ranking(ctx):
    await ctx.deliver(game_event([("ana", 1), ("beto", 2)], mode="CHAMELEON"))

    assert await ctx.board("CLASSIC") == []
    assert [p["nickname"] for p in await ctx.board("CHAMELEON")] == ["ana", "beto"]


async def test_el_empate_en_primer_lugar_cuenta_como_victoria_de_ambos(ctx):
    await ctx.deliver(game_event([("ana", 1), ("beto", 1), ("caro", 3)]))

    wins = {p["nickname"]: p["wins"] for p in await ctx.board()}
    assert wins == {"ana": 1, "beto": 1, "caro": 0}


async def test_una_partida_sin_ganador_no_da_victorias(ctx):
    # Camaleón al acabarse el tiempo con 0 colores: empate en rank 1 y winners vacío.
    await ctx.deliver(game_event([("ana", 1), ("beto", 1)], mode="CHAMELEON", winners=[]))

    assert [(p["rating"], p["games"], p["wins"]) for p in await ctx.board("CHAMELEON")] == [
        (1000, 1, 0),
        (1000, 1, 0),
    ]


@pytest.mark.parametrize(
    "body",
    [
        b"esto no es json",
        b'{"gameId": "g_1"}',
        b'{"gameId": "g_1", "mode": "POKER", "finishedAt": "2026-11-01T00:00:00Z", "players": []}',
    ],
)
async def test_un_mensaje_invalido_va_a_mensajes_muertos(ctx, body):
    message = await ctx.deliver(body)

    assert message.outcome == "dead"
    assert await ctx.app.state.pool.fetchval("SELECT count(*) FROM processed_games") == 0


async def test_si_falla_la_base_el_mensaje_vuelve_a_la_cola_sin_efectos_parciales(ctx, monkeypatch):
    service = ctx.app.state.service
    original = service._update_ratings
    event = game_event([("ana", 1), ("beto", 2)])

    async def failing(*args):
        raise asyncpg.PostgresConnectionError("base caída")

    monkeypatch.setattr(service, "_update_ratings", failing)
    failed = await ctx.deliver(event)

    assert failed.outcome == "requeue"
    assert await ctx.app.state.pool.fetchval("SELECT count(*) FROM processed_games") == 0

    monkeypatch.setattr(service, "_update_ratings", original)
    retried = await ctx.deliver(event)

    assert retried.outcome == "ack"
    assert [p["rating"] for p in await ctx.board()] == [1016, 984]
