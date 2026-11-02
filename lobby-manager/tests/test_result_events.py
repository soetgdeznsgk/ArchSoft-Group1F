"""Salida de resultados hacia el broker: se publican sin perderse y sin frenar el cierre de la sala."""

from __future__ import annotations

import asyncio

import jwt

from tests.conftest import JWT_SECRET, SERVICE_HEADERS


async def account_user(ctx, account_id: str, nickname: str) -> dict:
    r = await ctx.client.post(
        "/internal/auth/session", json={"accountId": account_id, "nickname": nickname}, headers=SERVICE_HEADERS
    )
    assert r.status_code == 201, r.text
    return r.json()


async def finish(ctx, lobby: dict, ranking: list[str]) -> str:
    game_id = lobby["game"]["gameId"]
    body = {
        "gameId": game_id,
        "mode": "CLASSIC",
        "winners": ranking[:1],
        "endReason": "completed",
        "standings": [{"player": player, "points": i, "rank": i + 1} for i, player in enumerate(ranking)],
    }
    r = await ctx.client.post(f"/internal/lobbies/{lobby['id']}/result", json=body, headers=SERVICE_HEADERS)
    assert r.status_code == 200, r.text
    return game_id


async def until(condition, timeout: float = 3) -> None:
    async def wait() -> None:
        while not await condition():
            await asyncio.sleep(0.01)

    await asyncio.wait_for(wait(), timeout)


async def published_count(ctx) -> int:
    return await ctx.pool.fetchval("SELECT count(*) FROM game_results WHERE published_at IS NOT NULL")


async def test_el_resultado_se_publica_una_vez_con_las_cuentas_de_los_jugadores(ctx):
    ana = await account_user(ctx, "account-ana", "ana")
    beto = await ctx.login("Beto")
    lobby, users = await ctx.lobby_with(2)
    await ctx.client.post(f"/api/lobbies/{lobby['id']}/leave", headers=users[1].headers)
    for headers in ({"Authorization": f"Bearer {ana['token']}"}, beto.headers):
        r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/join", headers=headers)
        assert r.status_code == 200, r.text
    third = await ctx.login("Caro")
    await ctx.client.post(f"/api/lobbies/{lobby['id']}/join", headers=third.headers)
    r = await ctx.client.post(f"/api/lobbies/{lobby['id']}/start", headers=users[0].headers)
    assert r.status_code == 200, r.text
    started = r.json()

    ranking = [ana["player"]["id"], users[0].id, beto.id, third.id]
    game_id = await finish(ctx, started, ranking)

    async def published() -> bool:
        return await published_count(ctx) == 1

    await until(published)
    await asyncio.sleep(0.1)  # da tiempo a una segunda publicación, que no debe ocurrir

    assert len(ctx.broker.events) == 1
    event = ctx.broker.events[0]
    assert (event["gameId"], event["lobbyId"], event["mode"]) == (game_id, lobby["id"], "CLASSIC")
    assert event["endReason"] == "completed"
    assert event["winners"] == [ana["player"]["id"]]
    assert [p["rank"] for p in event["players"]] == [1, 2, 3, 4]
    assert event["players"][0] == {
        "playerId": ana["player"]["id"],
        "accountId": "account-ana",
        "nickname": "ana",
        "rank": 1,
    }
    assert event["players"][2]["accountId"] is None


async def test_con_el_broker_caido_la_sala_cierra_y_el_evento_sale_al_volver(ctx):
    ctx.broker.down = True
    lobby, users = await ctx.started_lobby()

    await finish(ctx, lobby, [u.id for u in users])

    closed = (await ctx.client.get(f"/api/lobbies/{lobby['id']}", headers=users[0].headers)).json()
    assert closed["status"] == "CLOSED"
    await asyncio.sleep(0.1)
    assert ctx.broker.events == [] and await published_count(ctx) == 0

    ctx.broker.down = False

    async def published() -> bool:
        return await published_count(ctx) == 1

    await until(published)
    assert [e["gameId"] for e in ctx.broker.events] == [lobby["game"]["gameId"]]


async def test_la_sesion_con_cuenta_lleva_el_account_id_y_la_de_invitado_no(ctx):
    ana = await account_user(ctx, "account-ana", "ana")
    guest = await ctx.login("Invitado")

    claims = jwt.decode(ana["token"], JWT_SECRET, algorithms=["HS256"])
    guest_claims = jwt.decode(guest.token, JWT_SECRET, algorithms=["HS256"])

    assert claims["accountId"] == "account-ana" and claims["scope"] == "session"
    assert "accountId" not in guest_claims
