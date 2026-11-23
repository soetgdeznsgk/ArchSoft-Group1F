#!/usr/bin/env bash
# Cliente de pruebas (bash + curl) que corre en la red public con IP propia. Lo usan e2e.sh y los scripts de evidencia.
docker image inspect bololo-bench:curl >/dev/null 2>&1 ||
  printf 'FROM alpine:3.20.3\nRUN apk add --no-cache bash curl\n' | docker build -q -t bololo-bench:curl - >/dev/null
