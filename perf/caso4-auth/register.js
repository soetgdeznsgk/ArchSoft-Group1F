import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';
import { Counter } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'https://localhost';
const VUS = Number(__ENV.VUS || 1);
const RAMPA = __ENV.RAMPA || '15s';
const SOSTENIDA = __ENV.SOSTENIDA || '60s';
const RAMPA_MS = parseInt(RAMPA, 10) * 1000;
const RUN_ID = (__ENV.RUN_ID || 'r1').replace(/[^a-zA-Z0-9]/g, '').slice(0, 8);

const sinRespuesta = new Counter('errores_sin_respuesta');
const errores4xx = new Counter('errores_4xx');
const errores5xx = new Counter('errores_5xx');
const cuerpoInvalido = new Counter('errores_cuerpo');

export const options = {
  scenarios: { register: { executor: 'ramping-vus', startVUs: 0, stages: [
    { duration: RAMPA, target: VUS },
    { duration: SOSTENIDA, target: VUS },
  ], gracefulRampDown: '0s', gracefulStop: '30s' } },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

export default function () {
  const fase = exec.instance.currentTestRunDuration < RAMPA_MS ? 'rampa' : 'sostenida';
  const tags = { fase };
  const username = `r${RUN_ID}p${__VU}i${exec.scenario.iterationInTest}`.slice(0, 20);
  const res = http.post(`${BASE_URL}/auth/register`, JSON.stringify({
    username,
    password: 'perf-pass-123',
  }), { headers: { 'Content-Type': 'application/json' }, tags });

  if (res.status === 0) sinRespuesta.add(1, tags);
  else if (res.status >= 500) errores5xx.add(1, tags);
  else if (res.status >= 400) errores4xx.add(1, tags);

  const ok = check(res, {
    'register status 201': (r) => r.status === 201,
    'register trae cuenta y token': (r) => r.status === 201 &&
      Boolean(r.json('account')) && Boolean(r.json('token')),
  }, tags);
  if (res.status === 201 && !ok) cuerpoInvalido.add(1, tags);

  sleep(1);
}
