# syntax=docker/dockerfile:1

# Servicio "init" de docker-compose.yml: antes de que arranque el stack genera, si faltan,
# las claves de los JWT (secrets/jwt/) y el certificado TLS del borde (api-gateway/certs/).
# Usa los mismos scripts del repositorio, que Compose monta en /repo/scripts.
FROM alpine:3.20.3
RUN apk add --no-cache bash openssl
COPY init.sh /init.sh
ENTRYPOINT ["bash", "/init.sh"]
