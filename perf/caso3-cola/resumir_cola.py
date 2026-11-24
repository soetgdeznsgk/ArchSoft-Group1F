"""Une la salida del publicador, del vigilante de la cola y de PostgreSQL en una fila del CSV del caso 3.

Uso: python3 resumir_cola.py <csv> <modo> <tasa> <rep> <publicacion.json> <cola.txt> <bd.txt> <stats.txt> <reinicios>
"""
import csv
import json
import os
import re
import sys
from collections import defaultdict

CAMPOS = ["modo", "tasa", "rep", "objetivo", "confirmados", "fallidos_pub", "procesados", "tardios", "no_procesados",
          "dlq", "error_pct", "tasa_real", "proc_s", "p50_s", "p95_s", "p99_s", "max_s", "backlog_max", "drenado_s",
          "cpu_max_ratings", "cpu_max_pg", "cpu_max_rabbit", "mem_max_rabbit_mib", "reinicios"]
LIMITE_S = 60


def cpu_mem(path):
    cpu, mem = defaultdict(float), defaultdict(float)
    for linea in open(path, encoding="utf-8", errors="replace"):
        m = re.match(r"(\S+) cpu=([\d.]+)% mem=([\d.]+)(KiB|MiB|GiB)", linea)
        if m:
            n = re.sub(r"^bololo-integrado-|-\d+$", "", m.group(1))
            cpu[n] = max(cpu[n], float(m.group(2)))
            mem[n] = max(mem[n], float(m.group(3)) * {"KiB": 1 / 1024, "MiB": 1, "GiB": 1024}[m.group(4)])
    return cpu, mem


def main():
    csv_path, modo, tasa, rep, pub, cola, bd, stats, reinicios = sys.argv[1:10]
    p = json.loads(open(pub, encoding="utf-8").read().strip().splitlines()[-1])
    resumen = {}
    for linea in open(cola, encoding="utf-8", errors="replace"):
        if linea.startswith("RESUMEN "):
            resumen = json.loads(linea[8:])
    procesados, p50, p95, p99, mx, tardios, segundos = (open(bd, encoding="utf-8").read().strip().split("|") + [""] * 7)[:7]
    procesados = int(procesados or 0)
    tardios = int(tardios or 0)
    no_procesados = p["objetivo"] - procesados
    errores = no_procesados + tardios
    cpu, mem = cpu_mem(stats)
    f = lambda v: f"{float(v):.2f}" if v not in ("", None) else ""
    fila = {
        "modo": modo, "tasa": tasa, "rep": rep, "objetivo": p["objetivo"], "confirmados": p["confirmados"],
        "fallidos_pub": p["fallidos"], "procesados": procesados, "tardios": tardios, "no_procesados": no_procesados,
        "dlq": resumen.get("dlq_nuevos", ""), "error_pct": f"{100 * errores / max(p['objetivo'], 1):.3f}",
        "tasa_real": p["tasa_real"],
        "proc_s": f"{procesados / float(segundos):.1f}" if segundos and float(segundos) > 0 else "",
        "p50_s": f(p50), "p95_s": f(p95), "p99_s": f(p99), "max_s": f(mx),
        "backlog_max": resumen.get("backlog_max", ""), "drenado_s": resumen.get("drenado_s", ""),
        "cpu_max_ratings": f"{cpu.get('ratings-service', 0):.1f}", "cpu_max_pg": f"{cpu.get('ratings-postgres', 0):.1f}",
        "cpu_max_rabbit": f"{cpu.get('rabbitmq', 0):.1f}", "mem_max_rabbit_mib": f"{mem.get('rabbitmq', 0):.1f}",
        "reinicios": reinicios,
    }
    nuevo = not os.path.exists(csv_path)
    with open(csv_path, "a", newline="", encoding="utf-8") as fh:
        w = csv.DictWriter(fh, fieldnames=CAMPOS)
        if nuevo:
            w.writeheader()
        w.writerow(fila)
    print("  " + "  ".join(f"{k}={fila[k]}" for k in ("tasa", "rep", "tasa_real", "procesados", "proc_s", "error_pct",
                                                        "tardios", "no_procesados", "p50_s", "p95_s", "max_s",
                                                        "backlog_max", "drenado_s", "cpu_max_ratings")))


if __name__ == "__main__":
    main()
