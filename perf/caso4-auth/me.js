import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';
import { Counter } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'https://localhost';
const VUS = Number(__ENV.VUS || 1);
const RAMPA = __ENV.RAMPA || '15s';
const SOSTENIDA = __ENV.SOSTENIDA || '60s';
const RAMPA_MS = parseInt(RAMPA, 10) * 1000;

const sinRespuesta = new Counter('errores_sin_respuesta');
const errores4xx = new Counter('errores_4xx');
const errores5xx = new Counter('errores_5xx');
const cuerpoInvalido = new Counter('errores_cuerpo');

export function setup() {
  let res = http.post(`${BASE_URL}/auth/register`, JSON.stringify({
    username: 'perf-me',
    password: 'perf-pass-123',
  }), { headers: { 'Content-Type': 'application/json' } });
  if (res.status === 409) {
    res = http.post(`${BASE_URL}/auth/login`, JSON.stringify({
      username: 'perf-me',
      password: 'perf-pass-123',
    }), { headers: { 'Content-Type': 'application/json' } });
  }
  if (res.status !== 201 && res.status !== 200) {
    throw new Error(`No se pudo preparar /auth/me: HTTP ${res.status}`);
  }
  return { token: res.json('token') };
}

export const options = {
  scenarios: { me: { executor: 'ramping-vus', startVUs: 0, stages: [
    { duration: RAMPA, target: VUS },
    { duration: SOSTENIDA, target: VUS },
  ], gracefulRampDown: '0s', gracefulStop: '30s' } },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

export default function (data) {
  const fase = exec.instance.currentTestRunDuration < RAMPA_MS ? 'rampa' : 'sostenida';
  const tags = { fase };
  const res = http.get(`${BASE_URL}/auth/me`, {
    headers: { Authorization: `Bearer ${data.token}` },
    tags,
  });

  if (res.status === 0) sinRespuesta.add(1, tags);
  else if (res.status >= 500) errores5xx.add(1, tags);
  else if (res.status >= 400) errores4xx.add(1, tags);

  const ok = check(res, {
    'me status 200': (r) => r.status === 200,
    'me trae username': (r) => r.status === 200 &&
      r.json('username') === 'perf-me',
  }, tags);
  if (!ok) cuerpoInvalido.add(1, tags);

  sleep(1);
}
