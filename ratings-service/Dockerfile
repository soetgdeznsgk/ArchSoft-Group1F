# syntax=docker/dockerfile:1

# Ratings Service (Python / FastAPI + consumidor de RabbitMQ).
#
#   docker build -t bololo/ratings-service .
#
# Necesita PostgreSQL, RabbitMQ y las variables del README para arrancar.

FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /srv

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app/ app/

USER nobody
EXPOSE 8000

CMD ["uvicorn", "app.main:create_app", "--factory", \
     "--host", "0.0.0.0", "--port", "8000", \
     "--proxy-headers", "--no-access-log", "--timeout-graceful-shutdown", "5"]
