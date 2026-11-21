# Latencia pareada: antes y después del proxy, en la misma sesión

Medido el 2026-11-21 entre las 16:21 y las 16:31, en la misma máquina y con los mismos volúmenes.

- **Antes:** `main` @ `878cec4`, levantado desde un `git worktree`. Traefik es el punto de entrada.
- **Después:** `feature/p3-esc2`. Nginx delante de Traefik.

Orden: antes, después, antes, después (rondas `r1` y `r2`). En cada fase:

1. `docker compose down` del otro stack y `docker compose up -d --build --wait`.
2. 15 s de espera y 30 peticiones de calentamiento.
3. 3 corridas de `./scripts/latency.sh https://localhost 200` (archivos `c1`–`c3`).

`rN-fase-borde.txt` muestra qué contenedor publicaba los puertos en cada fase.

| Caso (mediana de cada corrida, promedio de 6) | Antes | Después | Diferencia |
|---|---|---|---|
| Conexión nueva: TCP + TLS | 10,22 ms (8,34–12,27) | 10,87 ms (8,82–13,14) | +0,65 ms |
| Conexión nueva: petición completa | 19,91 ms (16,90–23,47) | 21,58 ms (18,64–24,53) | +1,67 ms |
| Conexión reutilizada: petición completa | 7,95 ms (6,53–11,14) | 8,55 ms (7,09–10,04) | +0,60 ms |
