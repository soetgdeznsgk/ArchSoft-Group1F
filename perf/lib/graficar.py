"""Gráficas de un caso a partir de uno o varios CSV de barrido.

Uso: python3 graficar.py <salida_prefijo> <titulo> <csv> [<csv> ...]
Genera <prefijo>-latencia.png, -throughput.png, -cpu.png y -errores.png.
Cada nivel se resume con la mediana de sus corridas.
Variables: SATURACION (rango "a-b" donde el throughput deja de crecer), RODILLA (primer nivel con error),
           ZOOM (rango "a-b" de usuarios para la gráfica de errores por corrida).
"""
import csv
import os
import sys
from collections import defaultdict
from statistics import median

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
from matplotlib.ticker import FuncFormatter

AZUL, NARANJA, ROJO, GRIS, VERDE, MORADO = "#2563eb", "#ea580c", "#dc2626", "#6b7280", "#16a34a", "#7c3aed"


def leer(rutas):
    filas = defaultdict(list)
    for ruta in rutas:
        for f in csv.DictReader(open(ruta, encoding="utf-8")):
            filas[int(f["vus"])].append(f)
    return filas


def resumir(filas):
    datos = []
    for vus in sorted(filas):
        g = filas[vus]
        med = lambda k: median(float(x.get(k) or 0) for x in g)
        deg = median(100 * float(x.get("degradadas") or 0) / max(float(x["peticiones"] or 0), 1) for x in g)
        datos.append({
            "vus": vus, "n": len(g),
            "avg": med("avg_ms"), "med": med("med_ms"), "p95": med("p95_ms"),
            "pet_s": med("pet_s"), "err": med("error_pct"),
            "cpu_ratings": med("cpu_max_ratings"), "cpu_proxy": med("cpu_max_proxy"),
            "cpu_traefik": med("cpu_max_traefik"), "cpu_pg": med("cpu_max_pg"),
            "cpu_frontend": med("cpu_max_frontend"), "deg": deg,
        })
    return datos


def marcar(ax, etiquetas=True):
    sat = os.environ.get("SATURACION")
    rodilla = os.environ.get("RODILLA")
    if sat:
        a, _, b = sat.partition("-")
        ax.axvspan(float(a), float(b), color=GRIS, alpha=0.15,
                   label=f"inicio de saturación ({sat} usuarios)" if etiquetas else None)
    if rodilla:
        ax.axvline(float(rodilla), color=ROJO, ls="--", lw=1.6,
                   label=f"rodilla: primer error ({rodilla} usuarios)" if etiquetas else None)


def miles(ax):
    ax.xaxis.set_major_formatter(FuncFormatter(lambda v, _: f"{int(v):,}".replace(",", ".")))


def main():
    prefijo, titulo, rutas = sys.argv[1], sys.argv[2], sys.argv[3:]
    filas = leer(rutas)
    d = resumir(filas)
    x = [r["vus"] for r in d]
    plt.rcParams.update({"font.size": 10, "axes.spines.top": False, "axes.spines.right": False})

    fig, ax = plt.subplots(figsize=(9.5, 5.2))
    ax.plot(x, [r["avg"] for r in d], "o-", color=AZUL, label="promedio")
    ax.plot(x, [r["med"] for r in d], "s-", color=VERDE, ms=4, label="mediana")
    ax.plot(x, [r["p95"] for r in d], "^-", color=NARANJA, ms=5, label="p95")
    ax.set_yscale("log")
    ax.yaxis.set_major_formatter(FuncFormatter(lambda v, _: f"{v / 1000:g} s" if v >= 1000 else f"{v:g} ms"))
    ax.set_xlabel("usuarios concurrentes (VUs)")
    ax.set_ylabel("tiempo de respuesta (escala logarítmica)")
    miles(ax)
    marcar(ax)
    ax2 = ax.twinx()
    ax2.spines["right"].set_visible(True)
    ax2.bar(x, [r["err"] for r in d], width=40, color=ROJO, alpha=0.35, label="peticiones fallidas (%)")
    ax2.set_ylabel("peticiones fallidas (%)", color=ROJO)
    ax2.tick_params(axis="y", colors=ROJO)
    ax2.set_ylim(0, 105)
    h1, l1 = ax.get_legend_handles_labels()
    h2, l2 = ax2.get_legend_handles_labels()
    ax.legend(h1 + h2, l1 + l2, loc="upper left", frameon=False, fontsize=9)
    ax.set_title(f"{titulo}: tiempo de respuesta y errores")
    fig.tight_layout()
    fig.savefig(f"{prefijo}-latencia.png", dpi=150)

    fig, ax = plt.subplots(figsize=(9.5, 5.2))
    tope = max(r["pet_s"] for r in d)
    ax.plot(x, x, ls=":", color=GRIS, label="ideal (1 petición/s por usuario)")
    ax.plot(x, [r["pet_s"] for r in d], "o-", color=AZUL, label="atendidas (mediana)")
    ax.set_ylim(0, tope * 1.5)
    ax.set_xlabel("usuarios concurrentes (VUs)")
    ax.set_ylabel("peticiones atendidas por segundo")
    miles(ax)
    marcar(ax)
    ax.legend(loc="upper right", frameon=False, fontsize=9)
    ax.set_title(f"{titulo}: throughput")
    fig.tight_layout()
    fig.savefig(f"{prefijo}-throughput.png", dpi=150)

    fig, ax = plt.subplots(figsize=(9.5, 5.2))
    series = [("cpu_ratings", AZUL, "ratings-service"), ("cpu_proxy", NARANJA, "reverse-proxy (Nginx)"),
              ("cpu_traefik", MORADO, "traefik"), ("cpu_pg", VERDE, "ratings-postgres")]
    if any(r["cpu_frontend"] for r in d):
        series.insert(0, ("cpu_frontend", ROJO, "frontend (Node SSR)"))
    for k, c, nombre in series:
        ax.plot(x, [r[k] for r in d], "o-", color=c, ms=4, label=nombre)
    ax.axhline(100, color=GRIS, lw=0.8)
    ax.text(x[-1], 103, "100 % = 1 núcleo", color=GRIS, ha="right", fontsize=8)
    ax.set_xlabel("usuarios concurrentes (VUs)")
    ax.set_ylabel("CPU máxima por contenedor (%)")
    miles(ax)
    marcar(ax, etiquetas=False)
    ax.legend(loc="lower right", frameon=False, fontsize=9)
    ax.set_title(f"{titulo}: CPU por contenedor (docker stats)")
    fig.tight_layout()
    fig.savefig(f"{prefijo}-cpu.png", dpi=150)

    if any(r["deg"] for r in d):
        fig, ax = plt.subplots(figsize=(9.5, 5.2))
        err = [r["err"] for r in d]
        deg = [min(r["deg"], 100 - e) for r, e in zip(d, err)]
        comp = [max(0, 100 - a - b) for a, b in zip(err, deg)]
        ax.stackplot(x, comp, deg, err, colors=[VERDE, NARANJA, ROJO], alpha=0.75,
                     labels=["menú completo (con ranking)", "menú degradado (sin ranking)", "petición fallida"])
        ax.set_ylim(0, 100)
        ax.set_xlabel("usuarios concurrentes (VUs)")
        ax.set_ylabel("% de las peticiones")
        miles(ax)
        rodilla = os.environ.get("RODILLA")
        if rodilla:
            ax.axvline(float(rodilla), color="black", ls="--", lw=1.4, label=f"rodilla ({rodilla} usuarios)")
        ax.legend(loc="center right", frameon=True, fontsize=9)
        ax.set_title(f"{titulo}: qué recibe el usuario")
        fig.tight_layout()
        fig.savefig(f"{prefijo}-respuestas.png", dpi=150)

    zoom = os.environ.get("ZOOM")
    if zoom:
        a, _, b = zoom.partition("-")
        a, b = float(a), float(b)
        fig, ax = plt.subplots(figsize=(9.5, 4.6))
        for vus in sorted(filas):
            if not a <= vus <= b:
                continue
            for f in filas[vus]:
                e = float(f["error_pct"])
                ax.scatter(vus, e if e > 0 else 0.001, s=55, color=ROJO if e > 0 else VERDE,
                           edgecolors="black", linewidths=0.4, zorder=3)
        ax.set_yscale("log")
        ax.set_ylim(0.0005, 200)
        ax.yaxis.set_major_formatter(FuncFormatter(lambda v, _: "0 %" if v < 0.002 else f"{v:g} %"))
        ax.scatter([], [], color=VERDE, edgecolors="black", label="corrida sin errores")
        ax.scatter([], [], color=ROJO, edgecolors="black", label="corrida con errores")
        rodilla = os.environ.get("RODILLA")
        if rodilla:
            ax.axvline(float(rodilla), color=ROJO, ls="--", lw=1.6, label=f"rodilla ({rodilla} usuarios)")
        ax.set_xlabel("usuarios concurrentes (VUs)")
        ax.set_ylabel("peticiones fallidas por corrida (escala log.)")
        miles(ax)
        ax.legend(loc="upper left", frameon=False, fontsize=9)
        ax.set_title(f"{titulo}: errores de cada corrida alrededor de la rodilla")
        fig.tight_layout()
        fig.savefig(f"{prefijo}-errores.png", dpi=150)
    print("ok", prefijo)


if __name__ == "__main__":
    main()
