# Certificado TLS del gateway

Esta carpeta se monta en Traefik como `/certs` (solo lectura). Debe contener:

- `bololo.crt`: certificado del gateway, firmado por la CA local.
- `bololo.key`: su clave privada.

Ninguno se versiona (`.gitignore`). Se generan en cada máquina con:

```bash
./scripts/gen-certs.sh            # localhost, 127.0.0.1 y ::1
./scripts/gen-certs.sh 192.168.1.20   # además, la IP de la LAN para jugar desde otro equipo
./scripts/gen-certs.sh --trust    # además, confía en la CA local en este equipo
```

Sin estos dos archivos Traefik no arranca: así no puede servir por error su certificado autofirmado de ejemplo. Detalles en [`docs/p3/esc1.md`](../../docs/p3/esc1.md).
