"""Vigila la cola ratings.game-finished por la API de administración de RabbitMQ hasta que se vacía.

Corre dentro de lobby-manager (usa RABBITMQ_URL para las credenciales):
  docker compose exec -T lobby-manager python - <segundos_minimos> <segundos_maximos> < vigilar_cola.py
Imprime una línea por segundo y al final un resumen JSON. Termina cuando, pasados los segundos mínimos,
la cola lleva 3 lecturas seguidas vacía y sin mensajes en proceso.
"""
import base64
import json
import os
import sys
import time
import urllib.request
from urllib.parse import urlparse

MINIMO = float(sys.argv[1])
MAXIMO = float(sys.argv[2])
url = urlparse(os.environ["RABBITMQ_URL"])
credencial = base64.b64encode(f"{url.username}:{url.password}".encode()).decode()


def cola(nombre):
    pedido = urllib.request.Request(f"http://rabbitmq:15672/api/queues/%2F/{nombre}",
                                    headers={"Authorization": f"Basic {credencial}"})
    with urllib.request.urlopen(pedido, timeout=5) as r:
        datos = json.load(r)
    return datos.get("messages_ready", 0), datos.get("messages_unacknowledged", 0), datos.get("messages", 0)


dlq_inicial = cola("ratings.game-finished.dead")[2]
inicio = time.monotonic()
maximo = 0
vacias = 0
drenado = None
while True:
    t = time.monotonic() - inicio
    try:
        listos, sin_ack, _ = cola("ratings.game-finished")
    except Exception as exc:
        print(f"{t:7.1f} error {exc!r}", flush=True)
        time.sleep(1)
        continue
    maximo = max(maximo, listos + sin_ack)
    print(f"{t:7.1f} listos={listos} en_proceso={sin_ack}", flush=True)
    vacias = vacias + 1 if listos == 0 and sin_ack == 0 else 0
    if t >= MINIMO and vacias >= 3:
        drenado = round(t, 1)
        break
    if t >= MAXIMO:
        break
    time.sleep(1)
print("RESUMEN " + json.dumps({"backlog_max": maximo, "drenado_s": drenado,
                               "dlq_nuevos": cola("ratings.game-finished.dead")[2] - dlq_inicial}), flush=True)
