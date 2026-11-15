"""RF-01: la sesión la emite Auth; el Lobby la verifica y crea al jugador la primera vez que lo ve."""

from __future__ import annotations

import base64
import hashlib
import hmac
import json
import time

import pytest

from app import security

from .conftest import OTHER_PRIVATE_KEY, SESSION_PUBLIC_KEY, session_token


def bearer(token: str) -> dict[str, str]:
    return {"Authorization": f"Bearer {token}"}


def hs256(secret: str) -> str:
    """Sesión firmada con HMAC, a mano: PyJWT se niega a usar una clave PEM como secreto."""

    def part(data: dict) -> bytes:
        return base64.urlsafe_b64encode(json.dumps(data).encode()).rstrip(b"=")

    now = int(time.time())
    claims = {"sub": "p_x", "scope": "session", "nickname": "Ana", "iat": now, "exp": now + 3600}
    signing_input = part({"alg": "HS256", "typ": "JWT"}) + b"." + part(claims)
    signature = base64.urlsafe_b64encode(hmac.new(secret.encode(), signing_input, hashlib.sha256).digest())
    return (signing_input + b"." + signature.rstrip(b"=")).decode()


async def test_el_jugador_de_un_invitado_se_crea_con_su_primera_sesion(ctx):
    token = session_token("p_invitado01", "Ana María")

    me = await ctx.client.get("/api/session", headers=bearer(token))
    again = await ctx.client.get("/api/session", headers=bearer(token))

    assert me.status_code == again.status_code == 200
    assert me.json() == {"player": {"id": "p_invitado01", "nickname": "Ana María"}, "lobby": None}
    assert await ctx.pool.fetchval("SELECT count(*) FROM players") == 1


async def test_dos_invitados_pueden_usar_el_mismo_nombre(ctx):
    a = await ctx.login("Ana")
    b = await ctx.login("Ana")
    assert a.id != b.id


async def test_una_cuenta_es_siempre_el_mismo_jugador(ctx):
    first = session_token("p_cuenta", "ana", "account-ana")
    second = session_token("p_cuenta", "ana", "account-ana")

    a = (await ctx.client.get("/api/session", headers=bearer(first))).json()["player"]
    b = (await ctx.client.get("/api/session", headers=bearer(second))).json()["player"]

    assert a == b == {"id": "p_cuenta", "nickname": "ana"}


async def test_una_cuenta_anterior_conserva_su_jugador(ctx):
    await ctx.pool.execute(
        "INSERT INTO players (id, nickname, account_id) VALUES ('p_antiguo', 'ana', 'account-ana')"
    )

    r = await ctx.client.get("/api/session", headers=bearer(session_token("p_nuevo", "ana", "account-ana")))

    assert r.json()["player"] == {"id": "p_antiguo", "nickname": "ana"}


async def test_un_invitado_no_puede_tomar_el_jugador_de_una_cuenta(ctx):
    await ctx.login("ana", "account-ana")
    account_player = await ctx.pool.fetchval("SELECT id FROM players WHERE account_id = 'account-ana'")

    r = await ctx.client.get("/api/session", headers=bearer(session_token(account_player, "intruso")))

    assert r.status_code == 401


async def test_sin_token_o_con_token_invalido_responde_401(ctx):
    assert (await ctx.client.get("/api/lobbies")).status_code == 401
    r = await ctx.client.get("/api/lobbies", headers=bearer("no-es-un-jwt"))
    assert r.status_code == 401
    assert r.json()["code"] == "invalid_session"


@pytest.mark.parametrize(
    "token",
    [
        session_token("p_x", "Ana", key=OTHER_PRIVATE_KEY),
        hs256(SESSION_PUBLIC_KEY),
        hs256("el-antiguo-jwt-secret-compartido-0123456789"),
        session_token("p_x", "Ana", exp=1),
        session_token("p_x", "Ana", scope="otra"),
        session_token("p_x", ""),
        session_token("p_x", "x" * 21),
    ],
    ids=["otra-clave", "hs256-con-la-clave-publica", "hs256-secreto", "expirado", "sin-scope", "sin-nombre", "nombre-largo"],
)
async def test_rechaza_sesiones_falsas_o_incompletas(ctx, token):
    r = await ctx.client.get("/api/session", headers=bearer(token))
    assert r.status_code == 401
    assert await ctx.pool.fetchval("SELECT count(*) FROM players") == 0


async def test_el_token_de_partida_no_sirve_como_sesion(ctx):
    user = await ctx.login("Ana")
    game_token = security.issue_game_token(ctx.settings, "g_x", user.id)
    r = await ctx.client.get("/api/session", headers=bearer(game_token))
    assert r.status_code == 401


async def test_el_lobby_ya_no_emite_sesiones(ctx):
    assert (await ctx.client.post("/api/session", json={"nickname": "Ana"})).status_code == 405
    r = await ctx.client.post("/internal/auth/session", json={"accountId": "a", "nickname": "ana"})
    assert r.status_code == 404


async def test_healthz(ctx):
    r = await ctx.client.get("/healthz")
    assert r.status_code == 200
    assert r.json() == {"status": "ok", "postgres": "ok"}
