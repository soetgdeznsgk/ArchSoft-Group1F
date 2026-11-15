# Evidencia · Escenario 1 (Secure Channel)

Salidas de comandos del 14 de noviembre de 2026. Cada archivo muestra el comando (`$ ...`) y su salida. El análisis está en [`../../esc1.md` §7](../../esc1.md#7-evidencia-antes-y-después).

| Carpeta | Stack | Cómo se regenera |
|---|---|---|
| [`antes/`](antes/) | `main` @ `fbf1c10`, solo HTTP en :80 | `./scripts/esc1-evidence.sh antes` |
| [`despues/`](despues/) | `feature/p3-esc1`, HTTPS en :443 y :80 solo redirige | `./scripts/esc1-evidence.sh despues` |

| Archivo (antes) | Archivo (después) | Qué muestra |
|---|---|---|
| `01-sniff.txt` | `01-sniff.txt` | Captura `tcpdump` del tramo navegador ↔ gateway y cuántas veces se leen la contraseña y los JWT |
| `02-http.txt` | `02-redireccion-hsts.txt` | Respuestas por `http://`: antes sirve la página y entrega tokens; después, `301`/`308` a `https://` y HSTS |
| `03-tls.txt` | `03-tls.txt` | `openssl s_client`: antes no hay nada en el 443; después, versión, suite, ALPN, verificación y los rechazos de TLS viejo y suites débiles |
| — | `04-certificado.txt` | Certificado del gateway, CA local con *Name Constraints* y el certificado falso de `www.google.com` rechazado |
| `04-websocket.txt` | `05-websocket.txt` | *Upgrade* del WebSocket según el `Origin` y el esquema |
| `05-puertos.txt` | `06-puertos.txt` | Puertos publicados por Compose |
| `06-latencia.txt` | `07-latencia.txt` | Latencia por el gateway con dos clientes |
| — | `08-falla-cerrado.txt` | Traefik sin certificado no arranca |
| `07-e2e.txt` | `09-e2e.txt` | `scripts/e2e.sh` completo |

En los archivos los JWT van recortados (`eyJhbGciOiJIU...[recortado]`). Las capturas `.pcap` no se guardan aquí: la del antes tiene la contraseña y los JWT completos.
