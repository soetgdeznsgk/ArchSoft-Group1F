"""Game Logic simulado, para probar el Lobby Manager mientras Go no tiene capa de red.

Implementa lo mínimo del lado de Go (docs/game-logic-design.md §7.2 y §7.6):

    POST /internal/games            acepta la partida (201; 200 si se repite el gameId)
    POST /finish/{gameId}           publica game.ended en RabbitMQ, como haría Go al terminar
                                    (?winner=<playerId>; por defecto gana el primer jugador)

Necesita aio-pika (viene en la imagen del Lobby):  python tools/fake_game_logic.py
Variables: PORT (8081), SERVICE_SECRET, RABBITMQ_URL.
"""

from __future__ import annotations

import asyncio
import json
import os
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

PORT = int(os.environ.get("PORT", "8081"))
SERVICE_SECRET = os.environ.get("SERVICE_SECRET", "")
RABBITMQ_URL = os.environ.get("RABBITMQ_URL", "amqp://bololo:dev-password@rabbitmq:5672/")

games: dict[str, dict] = {}


class Handler(BaseHTTPRequestHandler):
    def do_POST(self) -> None:
        url = urlparse(self.path)
        length = int(self.headers.get("Content-Length") or 0)
        body = json.loads(self.rfile.read(length) or b"{}")

        if url.path == "/internal/games":
            if self.headers.get("X-Service-Token") != SERVICE_SECRET:
                return self.reply(401, {"error": "unauthorized"})
            players = body.get("players") or []
            mode = body.get("mode") or "CLASSIC"
            low, high = {"CLASSIC": (4, 4), "CHAMELEON": (2, 4)}.get(mode, (0, -1))
            if not body.get("gameId") or not body.get("lobbyId") or not low <= len(players) <= high:
                return self.reply(400, {"error": f"se requieren gameId, lobbyId y {low}-{high} jugadores ({mode})"})
            existing = games.get(body["gameId"])
            if existing and existing["lobbyId"] != body["lobbyId"]:
                return self.reply(409, {"error": "game exists"})
            games[body["gameId"]] = body
            reply = {"gameId": body["gameId"], "status": "WAITING", "wsPath": f"/ws/games/{body['gameId']}"}
            return self.reply(200 if existing else 201, reply)

        if url.path.startswith("/finish/"):
            game = games.get(url.path.removeprefix("/finish/"))
            if game is None:
                return self.reply(404, {"error": "partida desconocida"})
            winner = parse_qs(url.query).get("winner", [game["players"][0]["id"]])[0]
            try:
                event = asyncio.run(publish_result(game, winner))
            except Exception as exc:
                return self.reply(502, {"error": f"no se pudo publicar game.ended: {exc!r}"})
            return self.reply(200, {"published": event})

        self.reply(404, {"error": "not found"})

    def reply(self, status: int, body: dict) -> None:
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


async def publish_result(game: dict, winner: str) -> dict:
    import aio_pika

    event = {
        "gameId": game["gameId"],
        "lobbyId": game["lobbyId"],
        "mode": game.get("mode") or "CLASSIC",
        "winnerId": winner,
        "winners": [winner],
        "endReason": "WIN",
        "finishedAt": datetime.now(timezone.utc).isoformat(),
    }
    connection = await aio_pika.connect(RABBITMQ_URL)
    async with connection:
        channel = await connection.channel(publisher_confirms=True, on_return_raises=True)
        exchange = await channel.declare_exchange("bololo.events", aio_pika.ExchangeType.TOPIC, durable=True)
        message = aio_pika.Message(
            json.dumps(event).encode(),
            content_type="application/json",
            delivery_mode=aio_pika.DeliveryMode.PERSISTENT,
            message_id=event["gameId"],
        )
        await exchange.publish(message, routing_key="game.ended", mandatory=True)
    return event


if __name__ == "__main__":
    print(f"Game Logic simulado en :{PORT}; resultados como game.ended en RabbitMQ", flush=True)
    ThreadingHTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
