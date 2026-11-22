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

## Frontend tests

Unit and integration tests use Vitest and are kept with the presentation package:

```powershell
cd presentation-ssr
npm install
npm run test:unit
npm run test:integration
npm test
```

### Unit tests

Unit tests verify small frontend functions independently and do not require a running
browser or presentation server.

#### SSR ranking helpers — `tests/unit/views.test.js`

- **Populated ranking:** verifies that `rankingHtml()` adds the "Ranking · Clásico"
  heading and renders a player rating in an HTML table cell.
- **Empty and unavailable rankings:** verifies that an empty player list produces
  "Aún no hay rankings disponibles." and that a null leaderboard produces
  "El ranking no está disponible por ahora."
- **HTML escaping:** verifies that names containing HTML characters are escaped
  before being inserted into ranking HTML.
- **SSR JSON escaping:** verifies that data inserted into `ssr-data` replaces `<`
  characters so a nickname cannot close the script element.

#### Lobby menu utilities — `tests/unit/menu-utils.test.js`

- **Guest-name normalization:** removes surrounding whitespace from guest names.
- **Room-code normalization:** removes whitespace and converts room codes to uppercase.
- **Valid credentials:** accepts a normalized username and a sufficiently long password.
- **Invalid credentials:** rejects usernames containing unsupported characters and
  passwords shorter than the minimum length.

#### Game WebSocket client — `tests/unit/game-socket.test.js`

These tests replace the browser's `WebSocket` implementation with a fake socket.

- **Open socket:** verifies that `GameSocket.connect()` creates the client and that
  `send()` returns `true` and forwards JSON for an open socket.
- **Disconnected socket:** verifies that `send()` returns `false` without trying to
  send a message when no socket is available.

### Integration tests

Integration tests use a temporary HTTP server and temporary view/public directories.
They verify collaboration between the server modules and the filesystem without
depending on Docker or the complete backend stack.

#### Presentation HTTP server — `tests/integration/server.test.js`

- **Root redirect and health check:** verifies that `GET /` redirects to `/menu/`
  with status 302 and that `GET /healthz` returns `ok\n`.
- **Page templates:** verifies that `/menu/`, `/ingame/`, and `/chameleon/` return
  their expected HTML and that the menu template includes the SSR data placeholder.
- **Static assets:** verifies that a JavaScript file under `/menu/` is served with
  status 200 and a JavaScript content type.
- **Unknown paths:** verifies that paths outside a valid route or public asset return
  status 404.

### Test locations

| File | Level | What it verifies |
|---|---|---|
| `tests/unit/views.test.js` | Unit | SSR ranking HTML and safe JSON |
| `tests/unit/menu-utils.test.js` | Unit | Lobby input validation and normalization |
| `tests/unit/game-socket.test.js` | Unit | WebSocket send behavior |
| `tests/integration/server.test.js` | Integration | HTTP routes, health, templates, assets, and 404 responses |

The GitHub Actions [presentation workflow](../../.github/workflows/presentation.yml)
runs the unit suite and integration suite on changes to `presentation/` or
`presentation-ssr/`.
