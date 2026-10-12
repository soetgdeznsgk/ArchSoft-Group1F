"""Game Logic simulado, para probar el Lobby Manager mientras Go no tiene capa de red.

Implementa lo mínimo del REST interno de Go (docs/game-logic-design.md §7.2 y §7.6):

    POST /internal/games            acepta la partida (201; 200 si se repite el gameId)
    POST /finish/{gameId}           envía el resultado al Lobby, como haría Go al terminar
                                    (?winner=<playerId>; por defecto gana el primer jugador)

Solo usa la biblioteca estándar:  python tools/fake_game_logic.py
Variables: PORT (8081), SERVICE_SECRET, LOBBY_CALLBACK_URL (http://lobby-manager:8000).
"""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

PORT = int(os.environ.get("PORT", "8081"))
SERVICE_SECRET = os.environ.get("SERVICE_SECRET", "")
LOBBY_CALLBACK_URL = os.environ.get("LOBBY_CALLBACK_URL", "http://lobby-manager:8000")

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
            if not body.get("gameId") or not body.get("lobbyId") or len(players) != 4:
                return self.reply(400, {"error": "se requieren gameId, lobbyId y 4 jugadores"})
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
            status, answer = send_result(game, winner)
            return self.reply(200, {"callbackStatus": status, "callbackBody": answer})

        self.reply(404, {"error": "not found"})

    def reply(self, status: int, body: dict) -> None:
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def send_result(game: dict, winner: str) -> tuple[int, dict]:
    result = {"gameId": game["gameId"], "winnerId": winner, "winners": [winner], "endReason": "WIN"}
    request = urllib.request.Request(
        f"{LOBBY_CALLBACK_URL}/internal/lobbies/{game['lobbyId']}/result",
        data=json.dumps(result).encode(),
        headers={"Content-Type": "application/json", "X-Service-Token": SERVICE_SECRET},
    )
    try:
        with urllib.request.urlopen(request, timeout=5) as response:
            return response.status, json.loads(response.read())
    except urllib.error.HTTPError as exc:
        return exc.code, json.loads(exc.read())


if __name__ == "__main__":
    print(f"Game Logic simulado en :{PORT}; resultados hacia {LOBBY_CALLBACK_URL}", flush=True)
    ThreadingHTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
