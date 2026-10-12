# syntax=docker/dockerfile:1

# Lobby Manager (Python / FastAPI).
#
#   docker build -t bololo/lobby-manager .
#
# Necesita PostgreSQL y las variables del README para arrancar.

FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /srv

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app/ app/

USER nobody
EXPOSE 8000

# Un solo worker: las suscripciones SSE viven en la memoria del proceso.
# --no-access-log: el token de sesión viaja en ?token= del SSE y no debe quedar en los logs.
# --timeout-graceful-shutdown: los streams SSE no terminan solos y bloquearían el apagado.
CMD ["uvicorn", "app.main:create_app", "--factory", \
     "--host", "0.0.0.0", "--port", "8000", \
     "--proxy-headers", "--no-access-log", "--timeout-graceful-shutdown", "5"]
