"""Gráficas del caso 3 a partir de los CSV del barrido de tasas.

Uso: RODILLA=<tasa> python3 graficar_cola.py <salida_prefijo> <csv> [<csv> ...]
Genera <prefijo>-retraso.png, -throughput.png, -cola.png y -cpu.png. Cada tasa se resume con la mediana de sus corridas.
"""
import csv
import os
import sys
from collections import defaultdict
from statistics import median

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt

AZUL, NARANJA, ROJO, GRIS, VERDE, MORADO = "#2563eb", "#ea580c", "#dc2626", "#6b7280", "#16a34a", "#7c3aed"
TITULO = "Caso 3 · Cola de resultados"


def cargar(rutas):
    filas = defaultdict(list)
    for ruta in rutas:
        for f in csv.DictReader(open(ruta, encoding="utf-8")):
            filas[float(f["tasa"])].append(f)
    datos = []
    for tasa in sorted(filas):
        g = filas[tasa]
        med = lambda k: median(float(x[k] or 0) for x in g)
        datos.append({"tasa": tasa, "p50": med("p50_s"), "p95": med("p95_s"), "max": med("max_s"),
                      "err": med("error_pct"), "proc": med("proc_s"), "backlog": med("backlog_max"),
                      "drenado": med("drenado_s"), "cpu_r": med("cpu_max_ratings"), "cpu_pg": med("cpu_max_pg"),
                      "cpu_mq": med("cpu_max_rabbit"), "errores": [float(x["error_pct"]) for x in g]})
    return datos


def rodilla(ax):
    r = os.environ.get("RODILLA")
    if r:
        ax.axvline(float(r), color=ROJO, ls="--", lw=1.6, label=f"rodilla: primer error ({r} partidas/s)")


def main():
    prefijo, rutas = sys.argv[1], sys.argv[2:]
    d = cargar(rutas)
    x = [r["tasa"] for r in d]
    capacidad = median(r["proc"] for r in d if r["tasa"] >= 70)
    plt.rcParams.update({"font.size": 10, "axes.spines.top": False, "axes.spines.right": False})

    fig, ax = plt.subplots(figsize=(9.5, 5.2))
    ax.plot(x, [r["p50"] for r in d], "s-", color=VERDE, ms=4, label="retraso p50")
    ax.plot(x, [r["p95"] for r in d], "^-", color=NARANJA, ms=5, label="retraso p95")
    ax.plot(x, [r["max"] for r in d], "o-", color=AZUL, label="retraso máximo")
    ax.axhline(60, color=GRIS, ls=":", lw=1.2, label="límite: 60 s")
    ax.axvline(capacidad, color=GRIS, lw=0.8)
    ax.text(capacidad + 1, 2, f"capacidad ≈ {capacidad:.0f}/s", color=GRIS, fontsize=8)
    rodilla(ax)
    ax.set_xlabel("partidas terminadas publicadas por segundo")
    ax.set_ylabel("publicación → ranking actualizado (s)")
    ax2 = ax.twinx()
    ax2.spines["right"].set_visible(True)
    ax2.bar(x, [r["err"] for r in d], width=2.5, color=ROJO, alpha=0.35, label="resultados tardíos (%)")
    ax2.set_ylim(0, 100)
    ax2.set_ylabel("resultados con más de 60 s (%)", color=ROJO)
    ax2.tick_params(axis="y", colors=ROJO)
    h1, l1 = ax.get_legend_handles_labels()
    h2, l2 = ax2.get_legend_handles_labels()
    ax.legend(h1 + h2, l1 + l2, loc="upper left", frameon=False, fontsize=9)
    ax.set_title(f"{TITULO}: retraso y errores")
    fig.tight_layout()
    fig.savefig(f"{prefijo}-retraso.png", dpi=150)

    fig, ax = plt.subplots(figsize=(9.5, 5.2))
    ax.plot(x, x, ls=":", color=GRIS, label="ideal (procesa todo lo que llega)")
    ax.plot(x, [r["proc"] for r in d], "o-", color=AZUL, label="procesadas por segundo")
    rodilla(ax)
    ax.set_xlabel("partidas terminadas publicadas por segundo")
    ax.set_ylabel("partidas procesadas por segundo")
    ax.legend(loc="upper left", frameon=False, fontsize=9)
    ax.set_title(f"{TITULO}: throughput del consumidor")
    fig.tight_layout()
    fig.savefig(f"{prefijo}-throughput.png", dpi=150)

    fig, ax = plt.subplots(figsize=(9.5, 5.2))
    ax.plot(x, [r["backlog"] for r in d], "o-", color=MORADO, label="mensajes en cola (máximo)")
    ax.set_xlabel("partidas terminadas publicadas por segundo")
    ax.set_ylabel("mensajes en cola")
    ax2 = ax.twinx()
    ax2.spines["right"].set_visible(True)
    ax2.plot(x, [r["drenado"] for r in d], "s--", color=NARANJA, ms=4, label="cola vacía a los (s desde el inicio)")
    ax2.set_ylabel("cola vacía a los (s)", color=NARANJA)
    ax2.tick_params(axis="y", colors=NARANJA)
    rodilla(ax)
    h1, l1 = ax.get_legend_handles_labels()
    h2, l2 = ax2.get_legend_handles_labels()
    ax.legend(h1 + h2, l1 + l2, loc="upper left", frameon=False, fontsize=9)
    ax.set_title(f"{TITULO}: acumulación en RabbitMQ")
    fig.tight_layout()
    fig.savefig(f"{prefijo}-cola.png", dpi=150)

    fig, ax = plt.subplots(figsize=(9.5, 5.2))
    for k, c, n in (("cpu_r", AZUL, "ratings-service"), ("cpu_pg", VERDE, "ratings-postgres"), ("cpu_mq", MORADO, "rabbitmq")):
        ax.plot(x, [r[k] for r in d], "o-", color=c, ms=4, label=n)
    ax.axhline(100, color=GRIS, lw=0.8)
    ax.text(x[-1], 103, "100 % = 1 núcleo", color=GRIS, ha="right", fontsize=8)
    rodilla(ax)
    ax.set_xlabel("partidas terminadas publicadas por segundo")
    ax.set_ylabel("CPU máxima por contenedor (%)")
    ax.legend(loc="upper left", frameon=False, fontsize=9)
    ax.set_title(f"{TITULO}: CPU por contenedor")
    fig.tight_layout()
    fig.savefig(f"{prefijo}-cpu.png", dpi=150)
    print("ok", prefijo, "capacidad", round(capacidad, 1))


if __name__ == "__main__":
    main()
