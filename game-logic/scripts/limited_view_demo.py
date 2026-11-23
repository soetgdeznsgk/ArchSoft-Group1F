#!/usr/bin/env python3
"""Demo del escenario 4 (Limited View) con clientes WebSocket reales.

Sigue el flujo legítimo contra el stack que ya corre, siempre a través del
gateway (Traefik, HTTPS validado contra la CA local):

  Auth emite sesiones de invitado -> el Lobby crea la sala -> los jugadores
  entran -> se inicia la partida -> cada jugador pide su token de partida.

Con esos tokens abre un WebSocket por jugador y comprueba:

  A. CLASSIC (4 clientes)
     1. Cada jugador recibe una vista cuyo "you" es su propio id.
     2. Solo trae su propia mano; de los demás, solo card_count.
     3. Una acción con "player" falso se evalúa como el jugador del token.
  B. CHAMELEON (2 clientes)
     4. Cada jugador recibe su propia vista (las manos son públicas por diseño).
     5. Un "take" con "player" falso se atribuye al jugador del token.
  C. Handshake
     6. Token de otra partida, token de sesión y token inválido: 401.

Uso (desde la raíz del repo, con el stack arriba), ver docs/LIMITED_VIEW.md:

  docker run --rm --network bololo-integrado_public -e GATEWAY_HOST=traefik \
    -v "${PWD}:/work" -w /work python:3.12-slim \
    sh -c "pip install -q websockets==15.0.1 && python game-logic/scripts/limited_view_demo.py"
"""

from __future__ import annotations

import asyncio
import json
import os
import socket
import ssl
import sys
import urllib.error
import urllib.request
from pathlib import Path

from websockets.asyncio.client import connect
from websockets.exceptions import InvalidStatus

ROOT = Path(__file__).resolve().parents[2]
BASE_URL = os.environ.get("BASE_URL", "https://localhost").rstrip("/")
HOST = BASE_URL.split("://", 1)[1].split("/")[0].split(":")[0]
WS_URL = ("wss://" if BASE_URL.startswith("https://") else "ws://") + BASE_URL.split("://", 1)[1]
CA_CERT = os.environ.get("CA_CERT", str(ROOT / "api-gateway" / "ca" / "bololo-dev-ca.crt"))
GATEWAY_HOST = os.environ.get("GATEWAY_HOST")  # p. ej. "traefik" dentro de la red de Docker

# Dentro de un contenedor, "localhost" no es el gateway. Se redirige solo la
# resolución de nombres: TLS sigue validando el certificado para "localhost".
if GATEWAY_HOST:
    _getaddrinfo = socket.getaddrinfo

    def _to_gateway(host, *args, **kwargs):
        return _getaddrinfo(GATEWAY_HOST if host == HOST else host, *args, **kwargs)

    socket.getaddrinfo = _to_gateway

TLS = ssl.create_default_context(cafile=CA_CERT) if BASE_URL.startswith("https://") else None

TAKE_EVENTS = {"card_taken", "color_repeated", "chameleon_taken"}

PASS = 0
FAIL = 0


def ok(text: str) -> None:
    global PASS
    PASS += 1
    print(f"  \033[32mOK\033[0m     {text}")


def fail(text: str, why: str) -> None:
    global FAIL
    FAIL += 1
    print(f"  \033[31mFALLA\033[0m  {text} ({why})")


def check(condition: bool, text: str, why: str = "") -> None:
    ok(text) if condition else fail(text, why)


def section(title: str) -> None:
    print(f"\n== {title}")


# --- REST ----------------------------------------------------------------------


def http(method: str, path: str, token: str | None = None, body: dict | None = None):
    data = json.dumps(body).encode() if body is not None else None
    request = urllib.request.Request(BASE_URL + path, data=data, method=method)
    request.add_header("Content-Type", "application/json")
    if token:
        request.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(request, context=TLS, timeout=10) as response:
            raw = response.read()
            return response.status, json.loads(raw) if raw else None
    except urllib.error.HTTPError as error:
        raw = error.read()
        try:
            return error.code, json.loads(raw)
        except ValueError:
            return error.code, raw.decode(errors="replace")


def must(status_body, expected: int, what: str):
    status, body = status_body
    if status != expected:
        print(f"ERROR: {what}: HTTP {status}, se esperaba {expected}: {body}", file=sys.stderr)
        sys.exit(1)
    return body


def guest(nickname: str) -> dict:
    session = must(http("POST", "/auth/guest", body={"nickname": nickname}), 201, f"sesión de {nickname}")
    token = session["token"]
    me = must(http("GET", "/api/session", token), 200, f"jugador de {nickname} en el Lobby")
    return {"name": nickname, "session": token, "id": me["player"]["id"]}


def start_match(mode: str, names: list[str]) -> tuple[str, list[dict]]:
    """Crea la sala, mete a los jugadores, inicia la partida y reparte los tokens."""
    players = [guest(name) for name in names]
    lobby = must(http("POST", "/api/lobbies", players[0]["session"], {"mode": mode}), 201, "crear sala")
    for p in players[1:]:
        must(http("POST", "/api/lobbies/join", p["session"], {"code": lobby["code"]}), 200, f"{p['name']} entra")
    must(http("POST", f"/api/lobbies/{lobby['id']}/start", players[0]["session"]), 200, "iniciar partida")
    game_id = None
    for p in players:
        # El Lobby firma un token de partida distinto para cada jugador que consulta su sala.
        game = must(http("GET", f"/api/lobbies/{lobby['id']}", p["session"]), 200, "leer sala")["game"]
        p["game_token"] = game["token"]
        game_id = game["gameId"]
    return game_id, players


# --- WebSocket -----------------------------------------------------------------


async def open_ws(token: str, game_id: str):
    return await connect(
        f"{WS_URL}/ws/games/{game_id}?token={token}",
        ssl=TLS,
        origin=BASE_URL,
        open_timeout=10,
        max_queue=None,
    )


async def read_until(ws, match, timeout: float = 20):
    async def loop():
        while True:
            message = json.loads(await ws.recv())
            if match(message):
                return message

    return await asyncio.wait_for(loop(), timeout)


def is_type(*types: str):
    return lambda m: m.get("type") in types


async def handshake_status(token: str, game_id: str) -> int:
    try:
        ws = await open_ws(token, game_id)
    except InvalidStatus as error:
        return error.response.status_code
    await ws.close()
    return 101


def card_ids(hand: list) -> set:
    return {card["id"] for card in hand}


# --- Demo ----------------------------------------------------------------------


async def demo_classic() -> tuple[str, dict]:
    section("A. CLASSIC con 4 clientes WebSocket")
    game_id, players = start_match("CLASSIC", ["Ana", "Beto", "Caro", "Dani"])
    print(f"  partida {game_id}; cada jugador tiene su propio token de partida")
    conns = [await open_ws(p["game_token"], game_id) for p in players]
    try:
        views = [(await read_until(c, is_type("view")))["view"] for c in conns]

        check(
            all(v["you"] == p["id"] for p, v in zip(players, views)),
            '1. cada cliente recibe la vista de su propio jugador ("you" = id del token)',
            str([(p["id"], v["you"]) for p, v in zip(players, views)]),
        )

        summaries_clean = all(
            not ({"hand", "cards"} & set(s)) for v in views for s in v["players"]
        )
        hands = [card_ids(v["hand"]) for v in views]
        disjoint = all(not (hands[i] & hands[j]) for i in range(4) for j in range(i + 1, 4))
        ana, beto = views[0], views[1]
        beto_seen_by_ana = next(s for s in ana["players"] if s["id"] == players[1]["id"])
        check(
            summaries_clean and disjoint and beto_seen_by_ana["card_count"] == len(beto["hand"]),
            "2. cada vista trae solo la mano propia; de los demás, solo card_count",
            f"resúmenes sin cartas={summaries_clean}, manos disjuntas={disjoint}",
        )
        print(
            f"     Ana ve {len(ana['hand'])} cartas suyas y de Beto solo "
            f'"card_count": {beto_seen_by_ana["card_count"]}'
        )

        current = views[0]["current_turn"]
        attacker = next(i for i, p in enumerate(players) if p["id"] != current)
        await conns[attacker].send(json.dumps({"type": "draw", "player": current, "source": "hidden"}))
        error = await read_until(conns[attacker], is_type("error"))
        check(
            error.get("code") == "not_your_turn",
            f'3. {players[attacker]["name"]} manda "draw" diciendo ser el jugador en turno: rechazado',
            f"respuesta {error}",
        )
        return game_id, players[0]
    finally:
        for c in conns:
            await c.close()


async def demo_chameleon() -> dict:
    section("B. CHAMELEON con 2 clientes WebSocket")
    game_id, (eva, fede) = start_match("CHAMELEON", ["Eva", "Fede"])
    print(f"  partida {game_id}")
    eva_ws = await open_ws(eva["game_token"], game_id)
    fede_ws = await open_ws(fede["game_token"], game_id)
    try:
        eva_view = (await read_until(eva_ws, is_type("view")))["view"]
        fede_view = (await read_until(fede_ws, is_type("view")))["view"]
        check(
            eva_view["you"] == eva["id"] and fede_view["you"] == fede["id"],
            '4. cada cliente recibe su propia vista ("you" = id del token)',
            f"{eva_view['you']} / {fede_view['you']}",
        )
        print("     en este modo las manos de todos son públicas por diseño (ver docs/LIMITED_VIEW.md)")

        taken = None
        for _ in range(5):
            spawned = await read_until(fede_ws, is_type("card_spawned"))
            card_id = spawned["card"]["id"]
            # Eva toma la carta pero dice ser Fede en el cuerpo del mensaje.
            await eva_ws.send(json.dumps({"type": "take", "player": fede["id"], "card": card_id}))
            # Según la carta, el motor emite card_taken, color_repeated o
            # chameleon_taken; los tres dicen a nombre de quién quedó la toma.
            reply = await read_until(
                eva_ws,
                lambda m: m.get("type") == "error"
                or (m.get("type") in TAKE_EVENTS and (m.get("card") or {}).get("id") == card_id),
            )
            if reply["type"] in TAKE_EVENTS:
                taken = reply
                break
        check(
            taken is not None and taken.get("player") == eva["id"],
            '5. Eva manda "take" diciendo ser Fede: la carta queda a nombre de Eva',
            f"evento {taken}",
        )
        eva["game_id"] = game_id
        return eva
    finally:
        await eva_ws.close()
        await fede_ws.close()


async def demo_handshake(classic_game: str, ana: dict, eva: dict) -> None:
    section("C. Handshake del WebSocket")
    status = await handshake_status(eva["game_token"], classic_game)
    check(status == 401, "6a. token real de la partida camaleón usado en la partida clásica: 401", f"HTTP {status}")
    status = await handshake_status(ana["session"], classic_game)
    check(status == 401, "6b. token de sesión (no de partida): 401", f"HTTP {status}")
    status = await handshake_status("token.invalido.xxx", classic_game)
    check(status == 401, "6c. token con firma inválida: 401", f"HTTP {status}")
    print("     token expirado y jugador fuera del roster: ver las pruebas de Go (el Lobby no emite esos tokens)")


async def main() -> int:
    print(f"Demo Limited View contra {BASE_URL}" + (f" (gateway: {GATEWAY_HOST})" if GATEWAY_HOST else ""))
    classic_game, ana = await demo_classic()
    eva = await demo_chameleon()
    await demo_handshake(classic_game, ana, eva)
    section("Resumen")
    print(f"  {PASS} OK, {FAIL} fallas")
    return 0 if FAIL == 0 else 1


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
