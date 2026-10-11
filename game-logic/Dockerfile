# syntax=docker/dockerfile:1

# Game Logic Component (Go).
#
#   docker build -t bololo/game-logic .                          # pruebas + imagen
#   docker run --rm bololo/game-logic -games 200 -disconnect 0.05
#   docker build --target test .                                 # solo pruebas
#
# Hoy el único ejecutable es cmd/simulate. Cuando exista la capa de red
# (cmd/server), se construye con: --build-arg APP=server

ARG GO_VERSION=1.27.2

# --- base: código y dependencias ---------------------------------------------
FROM golang:${GO_VERSION}-bookworm AS base
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .

# --- test: formato, vet y pruebas con detector de carreras -------------------
# La imagen Debian trae gcc de 64 bits, necesario para -race.
FROM base AS test
RUN --mount=type=cache,target=/root/.cache/go-build \
    test -z "$(gofmt -l .)" \
    && go vet ./... \
    && go test -race -cover ./... \
    && touch /tmp/tests-passed

# --- build: binario estático -------------------------------------------------
FROM base AS build
ARG APP=simulate
# Obliga a que la etapa de pruebas pase antes de construir la imagen final.
COPY --from=test /tmp/tests-passed /tmp/tests-passed
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${APP}

# --- runtime: imagen mínima sin shell, usuario sin privilegios ---------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
