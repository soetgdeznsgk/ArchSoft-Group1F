import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';
import { Counter } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'https://localhost';
const VUS = Number(__ENV.VUS || 1);
const RAMPA = __ENV.RAMPA || '15s';
const SOSTENIDA = __ENV.SOSTENIDA || '60s';
const MODE = __ENV.MODE || 'valid';
const RAMPA_MS = parseInt(RAMPA, 10) * 1000;

const sinRespuesta = new Counter('errores_sin_respuesta');
const errores4xx = new Counter('errores_4xx');
const errores5xx = new Counter('errores_5xx');
const cuerpoInvalido = new Counter('errores_cuerpo');

export function setup() {
  if (MODE !== 'valid') return;
  const res = http.post(`${BASE_URL}/auth/register`, JSON.stringify({
    username: 'perf-login',
    password: 'perf-pass-123',
  }), { headers: { 'Content-Type': 'application/json' } });
  if (res.status !== 201 && res.status !== 409) {
    throw new Error(`No se pudo preparar login válido: HTTP ${res.status}`);
  }
}

export const options = {
  scenarios: { login: { executor: 'ramping-vus', startVUs: 0, stages: [
    { duration: RAMPA, target: VUS },
    { duration: SOSTENIDA, target: VUS },
  ], gracefulRampDown: '0s', gracefulStop: '30s' } },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

export default function () {
  const fase = exec.instance.currentTestRunDuration < RAMPA_MS ? 'rampa' : 'sostenida';
  const tags = { fase };
  const username = MODE === 'valid' ? 'perf-login' : 'perf-nadie';
  const password = MODE === 'wrong' ? 'incorrect-pass' : 'perf-pass-123';
  const res = http.post(`${BASE_URL}/auth/login`, JSON.stringify({ username, password }), {
    headers: { 'Content-Type': 'application/json' },
    tags,
  });

  if (res.status === 0) sinRespuesta.add(1, tags);
  else if (res.status >= 500) errores5xx.add(1, tags);
  else if (res.status >= 400) errores4xx.add(1, tags);

  const ok = MODE === 'valid'
    ? check(res, { 'login valid status 200': (r) => r.status === 200,
        'login trae token': (r) => r.status === 200 && Boolean(r.json('token')) }, tags)
    : check(res, { 'login invalid status 401': (r) => r.status === 401,
        'login devuelve codigo': (r) => r.status === 401 &&
          r.json('code') === 'invalid_credentials' }, tags);
  if (!ok) cuerpoInvalido.add(1, tags);

  sleep(1);
}
