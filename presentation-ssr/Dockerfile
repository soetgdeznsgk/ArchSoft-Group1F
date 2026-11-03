FROM node:22.14.0-alpine

WORKDIR /app

COPY presentation-ssr/server.js ./
COPY presentation-ssr/src ./src
COPY presentation-ssr/views ./views
COPY presentation/libraries ./public/libraries
COPY presentation/lobby ./public/menu
COPY presentation/game ./public/ingame
COPY presentation/chameleon ./public/chameleon

RUN addgroup -S app && adduser -S -G app app \
    && chown -R app:app /app

USER app
ENV PORT=3000
EXPOSE 3000

CMD ["node", "server.js"]
