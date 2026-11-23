import http from 'k6/http';
import ws from 'k6/ws';
import exec from 'k6/execution';
import { Counter, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'https://localhost';
const WS_URL = __ENV.WS_URL || 'wss://localhost';
const PARTIDAS = Number(__ENV.PARTIDAS || 1);
const TIMEOUT_MS = Number(__ENV.TIMEOUT_MS || 600000);

const fallosSetup = new Counter('partidas_fallos_setup');
const partidasTerminadas = new Counter('partidas_terminadas');
const partidasTimeout = new Counter('partidas_timeout');
const partidasWsPerdidas = new Counter('partidas_ws_perdidas');
const setupMs = new Trend('partida_setup_ms');
const duracionMs = new Trend('partida_duracion_ms');

export const options = {
  scenarios: {
    partidas: {
      executor: 'shared-iterations',
      vus: PARTIDAS,
      iterations: PARTIDAS,
      maxDuration: '15m',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

function headers(token) {
  return {
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
  };
}

function checkResponse(res, esperado, nombre) {
  if (res.status !== esperado) {
    fallosSetup.add(1, { operacion: nombre });
    console.error(`${nombre}: HTTP ${res.status} body=${res.body}`);
    return null;
  }
  return res.json();
}

export default function () {
  const started = Date.now();
  const nickname = `Perf-${__VU}-${exec.scenario.iterationInTest}`.slice(0, 20);

  const guestRes = http.post(
    `${BASE_URL}/auth/guest`,
    JSON.stringify({ nickname }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  const guest = checkResponse(guestRes, 201, 'guest');
  if (!guest) return;

  const token = guest.token;
  const lobbyRes = http.post(
    `${BASE_URL}/api/lobbies`,
    JSON.stringify({ mode: 'CLASSIC' }),
    headers(token),
  );
  const lobby = checkResponse(lobbyRes, 201, 'crear_lobby');
  if (!lobby) return;

  const lobbyId = lobby.id;
  const fillRes = http.post(
    `${BASE_URL}/api/bots/lobbies/${lobbyId}/fill`,
    JSON.stringify({ count: 3 }),
    headers(token),
  );
  const filled = checkResponse(fillRes, 201, 'llenar_bots');
  if (!filled) return;

  const startRes = http.post(
    `${BASE_URL}/api/lobbies/${lobbyId}/start`,
    '{}',
    headers(token),
  );
  const startedLobby = checkResponse(startRes, 200, 'iniciar_partida');
  if (!startedLobby || !startedLobby.game) return;

  setupMs.add(Date.now() - started);
  const game = startedLobby.game;
  const gameStarted = Date.now();
  const wsUrl = `${WS_URL}${game.wsPath}?token=${encodeURIComponent(game.token)}`;
  let terminado = false;

  const response = ws.connect(wsUrl, { tags: { partida: lobbyId } }, (socket) => {
    socket.on('open', () => {
      socket.setTimeout(() => {
        if (!terminado) {
          partidasTimeout.add(1, { partida: lobbyId });
          socket.close(4000, 'performance timeout');
        }
      }, TIMEOUT_MS);
    });

    socket.on('message', (message) => {
      let data;
      try {
        data = JSON.parse(message);
      } catch (_) {
        return;
      }
      if (data.type === 'game_ended') {
        terminado = true;
        partidasTerminadas.add(1, { partida: lobbyId });
        duracionMs.add(Date.now() - gameStarted);
        socket.close(1000, 'game ended');
      }
    });

    socket.on('error', () => {
      partidasWsPerdidas.add(1, { partida: lobbyId });
    });

    socket.on('close', (code) => {
      if (!terminado && code !== 4000) {
        partidasWsPerdidas.add(1, { partida: lobbyId });
      }
    });
  });

  if (!response || response.status !== 101) {
    partidasWsPerdidas.add(1, { partida: lobbyId });
  }
}
