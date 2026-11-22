"""Convierte la salida de una corrida de k6 en una fila del CSV de resultados.

Uso: python3 resumir.py <csv> <modo> <vus> <rep> <segundos_sostenida> <summary.json> <stats.txt> <nginx.txt> <time.txt>
"""
import csv
import json
import os
import re
import sys
from collections import Counter, defaultdict

CAMPOS = [
    "modo", "vus", "rep", "peticiones", "pet_s", "error_pct", "sin_respuesta", "e4xx", "e5xx", "cuerpo",
    "avg_ms", "med_ms", "p90_ms", "p95_ms", "p99_ms", "max_ms",
    "cpu_max_ratings", "cpu_max_proxy", "cpu_max_traefik", "cpu_max_pg", "mem_max_proxy_mib",
    "nginx_estados", "k6_rss_mib", "reinicios", "degradadas", "cpu_max_frontend",
]


def metrica(m, nombre):
    return m.get(f"{nombre}{{fase:sostenida}}", {})


def cpu_max(stats_path):
    maximos = defaultdict(float)
    memoria = defaultdict(float)
    if not os.path.exists(stats_path):
        return maximos, memoria
    for linea in open(stats_path, encoding="utf-8", errors="replace"):
        m = re.match(r"(\S+) cpu=([\d.]+)% mem=([\d.]+)(KiB|MiB|GiB)", linea)
        if not m:
            continue
        nombre = re.sub(r"^bololo-integrado-|-\d+$", "", m.group(1))
        maximos[nombre] = max(maximos[nombre], float(m.group(2)))
        factor = {"KiB": 1 / 1024, "MiB": 1, "GiB": 1024}[m.group(4)]
        memoria[nombre] = max(memoria[nombre], float(m.group(3)) * factor)
    return maximos, memoria


def estados_nginx(path):
    if not os.path.exists(path):
        return ""
    cuenta = Counter(re.findall(r'"status":(\d+)', open(path, encoding="utf-8", errors="replace").read()))
    return " ".join(f"{k}:{v}" for k, v in sorted(cuenta.items()))


def rss_k6(path):
    if not os.path.exists(path):
        return ""
    m = re.search(r"Maximum resident set size \(kbytes\): (\d+)", open(path, encoding="utf-8").read())
    return f"{int(m.group(1)) / 1024:.0f}" if m else ""


def main():
    csv_path, modo, vus, rep, segundos, summary, stats, nginx, tiempo = sys.argv[1:10]
    reinicios = os.environ.get("REINICIOS", "")
    m = json.load(open(summary, encoding="utf-8"))["metrics"]
    dur = metrica(m, "http_req_duration")
    fallos = metrica(m, "http_req_failed")
    reqs = metrica(m, "http_reqs").get("count", 0)
    cpu, mem = cpu_max(stats)
    fila = {
        "modo": modo, "vus": vus, "rep": rep, "peticiones": reqs,
        "pet_s": f"{reqs / float(segundos):.1f}",
        "error_pct": f"{100 * fallos.get('value', 0):.3f}",
        "sin_respuesta": metrica(m, "errores_sin_respuesta").get("count", 0),
        "e4xx": metrica(m, "errores_4xx").get("count", 0),
        "e5xx": metrica(m, "errores_5xx").get("count", 0),
        "cuerpo": metrica(m, "errores_cuerpo").get("count", 0),
        "avg_ms": f"{dur.get('avg', 0):.1f}", "med_ms": f"{dur.get('med', 0):.1f}",
        "p90_ms": f"{dur.get('p(90)', 0):.1f}", "p95_ms": f"{dur.get('p(95)', 0):.1f}",
        "p99_ms": f"{dur.get('p(99)', 0):.1f}", "max_ms": f"{dur.get('max', 0):.1f}",
        "cpu_max_ratings": f"{cpu.get('ratings-service', 0):.1f}",
        "cpu_max_proxy": f"{cpu.get('reverse-proxy', 0):.1f}",
        "cpu_max_traefik": f"{cpu.get('traefik', 0):.1f}",
        "cpu_max_pg": f"{cpu.get('ratings-postgres', 0):.1f}",
        "mem_max_proxy_mib": f"{mem.get('reverse-proxy', 0):.1f}",
        "nginx_estados": estados_nginx(nginx),
        "k6_rss_mib": rss_k6(tiempo),
        "reinicios": reinicios,
        "degradadas": metrica(m, "paginas_degradadas").get("count", 0),
        "cpu_max_frontend": f"{cpu.get('frontend', 0):.1f}",
    }
    nuevo = not os.path.exists(csv_path)
    with open(csv_path, "a", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=CAMPOS)
        if nuevo:
            w.writeheader()
        w.writerow(fila)
    print("  " + "  ".join(f"{k}={fila[k]}" for k in ("vus", "rep", "pet_s", "error_pct", "degradadas", "avg_ms", "p95_ms", "cpu_max_ratings", "cpu_max_frontend", "nginx_estados", "k6_rss_mib")))


if __name__ == "__main__":
    main()
