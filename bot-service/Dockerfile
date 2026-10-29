# syntax=docker/dockerfile:1

# Bot Service (Elixir/OTP).
#
#   docker build -t bololo/bot-service .           # pruebas + imagen
#   docker build --target test .                   # solo pruebas
#
# La etapa de compilación y la de ejecución usan la misma versión de Alpine:
# el release incluye la máquina virtual de Erlang, que enlaza con su OpenSSL.

ARG ELIXIR_IMAGE=hexpm/elixir:1.17.3-erlang-27.1.2-alpine-3.20.3
ARG RUNTIME_IMAGE=alpine:3.20.3

# --- base: dependencias ------------------------------------------------------
FROM ${ELIXIR_IMAGE} AS base
WORKDIR /src
RUN mix local.hex --force && mix local.rebar --force
COPY mix.exs mix.lock ./
RUN mix deps.get

# --- test: formato, advertencias como error y pruebas ------------------------
FROM base AS test
ENV MIX_ENV=test
COPY . .
RUN mix format --check-formatted \
    && mix compile --warnings-as-errors \
    && mix test \
    && touch /tmp/tests-passed

# --- build: release de producción --------------------------------------------
FROM base AS build
ENV MIX_ENV=prod
# Obliga a que la etapa de pruebas pase antes de construir la imagen final.
COPY --from=test /tmp/tests-passed /tmp/tests-passed
COPY config config
COPY lib lib
RUN mix compile --warnings-as-errors && mix release

# --- runtime: Alpine con el release, usuario sin privilegios -----------------
FROM ${RUNTIME_IMAGE}
RUN apk add --no-cache libstdc++ ncurses-libs openssl \
    && adduser -D -H -u 10001 bot
WORKDIR /app
COPY --from=build --chown=bot:bot /src/_build/prod/rel/bot_service ./
USER bot
ENV PORT=8090
EXPOSE 8090
CMD ["/app/bin/bot_service", "start"]
