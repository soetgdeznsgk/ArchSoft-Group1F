# Presentation SSR

This component provides the server-rendered HTML shell for the web
presentation. It renders `/menu/`, `/ingame/`, and `/chameleon/` on the server
and serves the existing client assets without changing their REST, SSE, or
WebSocket contracts.

The browser takes over after the initial HTML is received. The lobby and game
clients remain responsible for interactive state and real-time communication.

## Server-side data

`/menu/` is rendered with data fetched on the server: the classic-mode
leaderboard from the Ratings Service (`RATINGS_URL`, by default
`http://ratings-service:8000`, over the internal network). The server writes
it twice into the page:

- as an HTML table inside `#ssr-shell`, so the first paint already shows it;
- as JSON in `<script id="ssr-data">`, which `menu.js` reuses the first time
  the ranking panel opens instead of calling the API again.

If Ratings does not answer within `SSR_DATA_TIMEOUT_MS` (800 ms by default),
the page is still rendered, with a "not available" message instead of the
table. The menu never depends on Ratings being up.

## Run locally

From the repository root:

```powershell
node presentation-ssr/server.js
```

The server listens on `http://localhost:3000`, but local execution requires the
presentation assets to be copied into `presentation-ssr/public/`. Docker Compose
is the supported execution path because the Dockerfile performs that copy.
