const { readFile } = require("node:fs/promises");
const path = require("node:path");

const VIEW_FILES = {
  menu: "menu.html",
  ingame: "ingame.html",
  chameleon: "chameleon.html",
};

const RATINGS_URL = process.env.RATINGS_URL || "http://ratings-service:8000";
const DATA_TIMEOUT_MS = Number.parseInt(process.env.SSR_DATA_TIMEOUT_MS || "800", 10);
const HEADERS = ["#", "Jugador", "Puntos", "Partidas", "Victorias"];

// El ranking es público, así que se puede pedir sin la sesión del jugador.
// Si Ratings no responde a tiempo, el menú se entrega igual, sin ranking.
async function fetchLeaderboard() {
  try {
    const response = await fetch(`${RATINGS_URL}/api/ratings/leaderboard?mode=CLASSIC&limit=10`, {
      signal: AbortSignal.timeout(DATA_TIMEOUT_MS),
    });
    if (!response.ok) return null;
    const board = await response.json();
    return Array.isArray(board.players) ? board : null;
  } catch {
    return null;
  }
}

function escapeHtml(value) {
  return String(value).replace(/[&<>"']/g, (ch) => (
    { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]
  ));
}

function rankingHtml(board) {
  if (!board) return "<p>El ranking no está disponible por ahora.</p>";
  if (board.players.length === 0) return "<p>Aún no hay rankings disponibles.</p>";
  const head = HEADERS.map((h) => `<th>${escapeHtml(h)}</th>`).join("");
  const rows = board.players.map((p) => {
    const cells = [p.position, p.nickname, p.rating, p.games, p.wins];
    return `<tr>${cells.map((c) => `<td>${escapeHtml(c)}</td>`).join("")}</tr>`;
  }).join("");
  return `<h3>Ranking · Clásico</h3><table><tr>${head}</tr>${rows}</table>`;
}

// JSON dentro de <script>: se escapa "<" para que un apodo no pueda cerrar la etiqueta.
function dataScript(data) {
  const json = JSON.stringify(data).replace(/</g, "\\u003c");
  return `<script id="ssr-data" type="application/json">${json}</script>`;
}

async function renderPage(page, viewsRoot) {
  const fileName = VIEW_FILES[page];
  if (!fileName) throw new Error(`Unknown SSR page: ${page}`);
  const template = await readFile(path.join(viewsRoot, fileName), "utf8");
  if (page !== "menu") return template;
  const leaderboard = await fetchLeaderboard();
  return template
    .replace("<!--ssr:ranking-->", rankingHtml(leaderboard))
    .replace("<!--ssr:data-->", dataScript({ leaderboard }));
}

module.exports = { renderPage, rankingHtml, dataScript };
