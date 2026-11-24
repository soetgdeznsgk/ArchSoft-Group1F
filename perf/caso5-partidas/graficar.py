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

plt.rcParams.update({"font.size": 10, "axes.spines.top": False, "axes.spines.right": False})

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
    ax.plot(setup_x, setup_y, "o-", color="#ea580c",
            label="flujo de preparación (invitado, lobby, bots e inicio)")
if http_y:
    ax.plot(http_x, http_y, "s-", color="#7c3aed",
            label="HTTP promedio del flujo de preparación")
ax.set_xticks(x)
ax.set_xlabel("partidas simultáneas")
ax.set_ylabel("latencia (ms)")
ax.set_title("Caso 5: latencia al preparar e iniciar un lobby")
ax.legend(loc="upper left", frameon=False)
fig.tight_layout()
fig.savefig(IMG / "caso5-latencia-lobby.png", dpi=150)
plt.close(fig)
