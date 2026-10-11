# Lobby Manager (FastAPI)

Componente de lógica encargado de las salas. Aún no implementado.

Responsabilidades: RF-01 a RF-09 (ingreso, creación, listado, ingreso a una sala, salida, sala de espera en tiempo real e inicio de la partida) y RF-36 / RNF-04 (registrar una sola vez el resultado y cerrar la sala).

Conectores, según la vista C&C:

| Con | Conector |
|---|---|
| Presentación (vía API Gateway) | REST y SSE para la sala de espera |
| Game Logic | REST interno: crear la partida al iniciar y recibir el resultado final |
| PostgreSQL | Conector de base de datos; los scripts de esquema van en esta carpeta |
