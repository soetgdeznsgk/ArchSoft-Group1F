# Evidencia · Escenario 5 (Rate Limiting)

Salidas de comandos del 22 de noviembre de 2026. Cada archivo muestra el comando (`$ ...`) y su salida. El análisis está en [`../../esc5.md` §7](../../esc5.md#7-evidencia-antes-y-después).

| Carpeta | Stack | Cómo se regenera |
|---|---|---|
| [`antes/`](antes/) | `main` @ `eaae0db`, sin límite de tasa | `./scripts/esc5-evidence.sh antes` |
| [`despues/`](despues/) | `feature/p3-esc5`, límite por IP en `reverse-proxy` | `./scripts/esc5-evidence.sh despues` (reinicia el proxy para empezar con los contadores en cero y al final corre `e2e.sh`) |

**El atacante** es un contenedor (`bololo-bench:curl`, Alpine con curl) conectado a la red `public`. Llega al proxy como un cliente de Internet y tiene su propia IP (`172.20.0.3`). **El usuario legítimo** es el host, que con Docker Desktop llega con la IP del gateway de esa red (`172.20.0.1`).

| Archivo | Qué muestra |
|---|---|
| `01-fuerza-bruta.txt` | 100 logins seguidos, cada uno con un usuario distinto (el bloqueo por cuenta de `auth` nunca salta), y cuántos de cada código |
| `02-inundacion-api.txt` | 20 s de lotes de 500 `GET /api/ratings/leaderboard` con 50 en paralelo (HTTP/2): códigos y peticiones por segundo |
| `03-usuario-legitimo.txt` | El host pide el leaderboard cada 0,1 s durante 20 s, primero sin ataque y luego mientras corre la inundación de `02`: códigos y latencia (mediana, p95, p99) |
| `04-respuesta-429.txt` | Cabeceras y cuerpo del `429`, líneas del log con `upstream_status` vacío, IP con la que el proxy ve a cada cliente y directivas generadas en `bololo.conf` |
| `05-interruptor.txt` (después) | `RATE_LIMIT=off`: 40 logins, todos llegan a `auth`. Luego vuelve a `on` |
| `06-e2e.txt` (después) | `scripts/e2e.sh` completo |

`06-e2e.txt` da 0 fallas. La única prueba omitida (fin de la partida) necesita `DEMO_MODE`.
