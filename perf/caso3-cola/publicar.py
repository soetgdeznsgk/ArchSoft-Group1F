"""Publica eventos game.finished a una tasa fija y reporta cuántos confirmó RabbitMQ.

Corre dentro de un contenedor con aio_pika y RABBITMQ_URL (lobby-manager):
  docker compose exec -T lobby-manager python - <tasa_por_s> <segundos> <corrida> < publicar.py
Cada partida usa 4 cuentas perf- distintas, como una partida clásica con 4 jugadores registrados.
"""
import asyncio
import json
import os
import random
import sys
import time
from datetime import UTC, datetime

import aio_pika

TASA = float(sys.argv[1])
SEGUNDOS = float(sys.argv[2])
CORRIDA = sys.argv[3]
CUENTAS = 5000
TIMEOUT_CONFIRMACION = 30


def evento(n):
    cuentas = random.sample(range(1, CUENTAS + 1), 4)
    jugadores = [
        {"playerId": f"p_{i}", "accountId": f"perf-{c:05d}", "nickname": f"jugador_{c}", "rank": i + 1}
        for i, c in enumerate(cuentas)
    ]
    return {
        "gameId": f"perf-{CORRIDA}-{n:07d}",
        "lobbyId": "l_perf",
        "mode": "CLASSIC",
        "endReason": "completed",
        "finishedAt": datetime.now(UTC).isoformat(),
        "winners": ["p_0"],
        "players": jugadores,
    }


async def main():
    conexion = await aio_pika.connect(os.environ["RABBITMQ_URL"])
    confirmados = fallidos = 0
    pendientes = set()

    async def publicar(exchange, n):
        nonlocal confirmados, fallidos
        cuerpo = json.dumps(evento(n)).encode()
        mensaje = aio_pika.Message(cuerpo, content_type="application/json",
                                   delivery_mode=aio_pika.DeliveryMode.PERSISTENT, message_id=f"perf-{CORRIDA}-{n}")
        try:
            await asyncio.wait_for(exchange.publish(mensaje, routing_key="game.finished", mandatory=True),
                                   TIMEOUT_CONFIRMACION)
            confirmados += 1
        except Exception:
            fallidos += 1

    async with conexion:
        canal = await conexion.channel(publisher_confirms=True, on_return_raises=True)
        exchange = await canal.get_exchange("bololo.events")
        total = int(TASA * SEGUNDOS)
        inicio = time.monotonic()
        for n in range(total):
            objetivo = inicio + n / TASA
            espera = objetivo - time.monotonic()
            if espera > 0:
                await asyncio.sleep(espera)
            tarea = asyncio.create_task(publicar(exchange, n))
            pendientes.add(tarea)
            tarea.add_done_callback(pendientes.discard)
        fin_envio = time.monotonic()
        if pendientes:
            await asyncio.gather(*pendientes)
        fin = time.monotonic()
    print(json.dumps({
        "objetivo": total, "confirmados": confirmados, "fallidos": fallidos,
        "segundos_envio": round(fin_envio - inicio, 2), "segundos_confirmacion": round(fin - inicio, 2),
        "tasa_real": round(total / max(fin_envio - inicio, 1e-9), 1),
    }))


asyncio.run(main())
