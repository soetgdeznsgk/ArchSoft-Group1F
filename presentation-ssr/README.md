# Presentation SSR

This component provides the server-rendered HTML shell for the web
presentation. It renders `/menu/`, `/ingame/`, and `/chameleon/` on the server
and serves the existing client assets without changing their REST, SSE, or
WebSocket contracts.

The browser takes over after the initial HTML is received. The lobby and game
clients remain responsible for interactive state and real-time communication.

## Run locally

From the repository root:

```powershell
node presentation-ssr/server.js
```

The server listens on `http://localhost:3000`, but local execution requires the
presentation assets to be copied into `presentation-ssr/public/`. Docker Compose
is the supported execution path because the Dockerfile performs that copy.
