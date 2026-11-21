# Certificado TLS del borde

Esta carpeta se monta en el proxy inverso (`reverse-proxy`) como `/certs` (solo lectura). Desde el escenario 2 es él quien termina el TLS; Traefik ya no la monta. Debe contener:

- `bololo.crt`: certificado del borde, firmado por la CA local.
- `bololo.key`: su clave privada.

Ninguno se versiona (`.gitignore`). Se generan en cada máquina con:

```bash
./scripts/gen-certs.sh            # localhost, 127.0.0.1 y ::1
./scripts/gen-certs.sh 192.168.1.20   # además, la IP de la LAN para jugar desde otro equipo
./scripts/gen-certs.sh --trust    # además, confía en la CA local en este equipo
```

Sin estos dos archivos Nginx no arranca: el sistema falla cerrado en vez de servir sin TLS. Detalles en [`docs/p3/esc1.md`](../../docs/p3/esc1.md).
