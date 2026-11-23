import csv
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt


ROOT = Path(__file__).resolve().parent
CSV = ROOT / "resultados" / "partidas.csv"
IMG = ROOT / "img"
IMG.mkdir(parents=True, exist_ok=True)

with CSV.open(encoding="utf-8", newline="") as f:
    rows = list(csv.DictReader(f))

x = [int(r["partidas"]) for r in rows]
terminadas = [int(r["terminadas"]) for r in rows]
timeouts = [int(r["timeout"]) for r in rows]
duracion = [float(r["duracion_promedio_ms"]) for r in rows]

plt.rcParams.update({"font.size": 10, "axes.spines.top": False, "axes.spines.right": False})

fig, ax = plt.subplots(figsize=(9.5, 5.2))
ax.plot(x, terminadas, "o-", color="#16a34a", label="terminadas")
ax.plot(x, timeouts, "o-", color="#dc2626", label="timeout")
ax.set_xticks(x)
ax.set_ylim(0, 11)
ax.set_xlabel("partidas simultáneas")
ax.set_ylabel("número de partidas")
ax.set_title("Caso 5: finalización dentro del timeout")
ax.legend(loc="upper left", frameon=False)
fig.tight_layout()
fig.savefig(IMG / "caso5-finalizacion.png", dpi=150)
plt.close(fig)

fig, ax = plt.subplots(figsize=(9.5, 5.2))
ax.plot(x, [v / 1000 for v in duracion], "o-", color="#2563eb")
ax.axhline(660, color="#dc2626", ls="--", lw=1.5, label="timeout 660 s")
ax.set_xticks(x)
ax.set_xlabel("partidas simultáneas")
ax.set_ylabel("duración promedio (s)")
ax.set_title("Caso 5: duración de las partidas")
ax.legend(loc="upper left", frameon=False)
fig.tight_layout()
fig.savefig(IMG / "caso5-duracion.png", dpi=150)
plt.close(fig)

setup = []
http_avg = []
for r in rows:
    setup.append(float(r["setup_promedio_ms"]) if r["setup_promedio_ms"] else None)
    http_avg.append(float(r["http_avg_ms"]) if r["http_avg_ms"] else None)

fig, ax = plt.subplots(figsize=(9.5, 5.2))
setup_x = [v for v, y in zip(x, setup) if y is not None]
setup_y = [y for y in setup if y is not None]
http_x = [v for v, y in zip(x, http_avg) if y is not None]
http_y = [y for y in http_avg if y is not None]
if setup_y:
    ax.plot(setup_x, setup_y, "o-", color="#ea580c", label="preparación")
if http_y:
    ax.plot(http_x, http_y, "s-", color="#7c3aed", label="HTTP promedio")
ax.set_xticks(x)
ax.set_xlabel("partidas simultáneas")
ax.set_ylabel("tiempo (ms)")
ax.set_title("Caso 5: preparación y operaciones HTTP")
ax.legend(loc="upper left", frameon=False)
fig.tight_layout()
fig.savefig(IMG / "caso5-preparacion.png", dpi=150)
plt.close(fig)
