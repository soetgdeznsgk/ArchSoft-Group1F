// Pantalla básica del modo camaleón (/chameleon/).
//
// Solo dibuja lo que manda el servidor y envía {"type":"take","card":id} al
// tocar una carta. Las reglas y el arbitraje (quién tomó primero) los decide
// Game Logic. Contrato de mensajes: game-logic/docs/MODO_CAMALEON.md §6.
//
// El acceso a la partida lo deja el menú en sessionStorage["bololo.game"].

import { GameSocket } from "/ingame/client/ingame/game-socket.js";

const COLORS = {
  red: { name: "Rojo", hex: "#e53935" },
  orange: { name: "Naranja", hex: "#fb8c00" },
  yellow: { name: "Amarillo", hex: "#fdd835" },
  green: { name: "Verde", hex: "#43a047" },
  cyan: { name: "Cian", hex: "#00acc1" },
  blue: { name: "Azul", hex: "#1e88e5" },
  purple: { name: "Morado", hex: "#8e24aa" },
  pink: { name: "Rosa", hex: "#ec407a" },
  chameleon: { name: "Camaleón", hex: "#263238" },
};
// Vida máxima de una carta en el servidor (chameleon.DefaultCardLifetimeMax). La barra
// arranca más corta cuanto menos le queda a la carta.
const MAX_LIFETIME_MS = 2000;

const access = JSON.parse(sessionStorage.getItem("bololo.game") || "null");
const names = Object.fromEntries((access?.players || []).map((p) => [p.id, p.nickname]));
const $ = (id) => document.getElementById(id);

let socket = null;
let remainingMs = 0;
let viewAt = 0;
let you = access?.me?.id;

const nameOf = (id) => (id === you ? `${names[id] || id} (tú)` : names[id] || id);
const colorName = (c) => (COLORS[c] || { name: c }).name;

function say(text) {
  $("message").textContent = text;
}

function log(text) {
  const item = document.createElement("li");
  item.textContent = text;
  $("log").prepend(item);
  while ($("log").children.length > 12) $("log").lastChild.remove();
}

function renderBoard(view) {
  const board = $("board");
  board.replaceChildren();
  for (const card of view.board) {
    const color = COLORS[card.color] || { name: card.color, hex: "#777" };
    const button = document.createElement("button");
    button.className = "card" + (card.color === "chameleon" ? " chameleon" : "");
    // x, y ∈ [0, 1]; el CSS los ubica dentro de la mesa sin que la carta se corte en los bordes.
    button.style.setProperty("--x", card.x);
    button.style.setProperty("--y", card.y);
    button.style.setProperty("--card", color.hex);
    button.disabled = !view.can_take;
    button.setAttribute("aria-label", `Tomar carta ${color.name}`);
    button.innerHTML = `<span class="label">${color.name}</span><span class="life"></span>`;
    button.addEventListener("click", () => send({ type: "take", card: card.id }));
    board.append(button);
    // Barra de vida: animación CSS que se vacía en el tiempo que le queda a la carta.
    const life = button.querySelector(".life");
    life.style.setProperty("--from", `${Math.min(100, (card.expires_in_ms / MAX_LIFETIME_MS) * 100)}%`);
    life.style.setProperty("--ms", `${card.expires_in_ms}ms`);
  }
}

function renderPlayers(view) {
  const list = $("players");
  list.replaceChildren();
  for (const p of view.players) {
    const owned = new Set(p.cards.map((c) => c.color));
    const item = document.createElement("li");
    item.className = "player" + (p.id === view.you ? " me" : "") + (p.connected ? "" : " offline");
    const slots = view.colors
      .map((c) => {
        const has = owned.has(c);
        return `<span class="slot${has ? " has" : ""}" style="--card:${COLORS[c].hex}" title="${colorName(c)}${has ? "" : " (falta)"}"></span>`;
      })
      .join("");
    item.innerHTML = `
      <div class="player-head">
        <span class="name"></span>
        <span class="count">${p.colors}/8${p.connected ? "" : " · desconectado"}</span>
      </div>
      <div class="slots" aria-label="${p.colors} de 8 colores: ${[...owned].map(colorName).join(", ") || "ninguno"}">${slots}</div>`;
    item.querySelector(".name").textContent = nameOf(p.id); // textContent: el nombre lo escribe el jugador
    list.append(item);
  }
}

function renderFinal(final) {
  const winners = final.winners.map(nameOf);
  $("final-title").textContent =
    final.reason === "all_colors"
      ? `¡${winners[0]} reunió los 8 colores!`
      : winners.length
        ? `Se acabó el tiempo. Gana: ${winners.join(", ")}`
        : "Se acabó el tiempo. Nadie tiene cartas: no hay ganador.";
  const list = $("final-standings");
  list.replaceChildren();
  for (const s of final.standings) {
    const item = document.createElement("li");
    item.textContent = `#${s.rank} ${nameOf(s.player)} · ${s.colors}/8 colores`;
    list.append(item);
  }
  if (!$("final").open) $("final").showModal();
}

function applyView(view) {
  you = view.you;
  remainingMs = view.remaining_ms;
  viewAt = performance.now();
  renderBoard(view);
  renderPlayers(view);
  if (view.final) renderFinal(view.final);
  else if (view.phase === "playing") say("¡A jugar! Toca una carta para tomarla.");
}

function describe(event) {
  const who = event.player ? nameOf(event.player) : "";
  const color = event.card ? colorName(event.card.color) : "";
  switch (event.type) {
    case "game_started":
      return "Empieza la partida.";
    case "card_taken":
      return `${who} tomó ${color}.`;
    case "color_repeated":
      return `${who} repitió ${color} y pierde ambas cartas.`;
    case "chameleon_taken":
      return `${who} tomó el camaleón y pierde ${event.lost?.length || 0} carta(s).`;
    case "player_connection":
      return `${who} ${event.connected ? "se conectó" : "se desconectó"}.`;
    case "game_ended":
      return "Fin de la partida.";
    default:
      return null; // card_spawned y card_expired ya se ven en la mesa
  }
}

function onMessage(msg) {
  switch (msg.type) {
    case "view":
      applyView(msg.view);
      break;
    case "game_waiting":
      say(`Esperando jugadores: ${msg.connectedPlayers.length}/${msg.requiredPlayers}`);
      break;
    case "error":
      say(msg.message);
      break;
    default: {
      const text = describe(msg);
      if (text) log(text);
    }
  }
}

function send(action) {
  if (!socket || !socket.send(action)) say("Sin conexión con el servidor.");
}

function tickTimer() {
  const left = Math.max(0, remainingMs - (performance.now() - viewAt));
  const s = Math.ceil(left / 1000);
  $("timer").textContent = viewAt ? `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}` : "--:--";
}

if (!access || (access.mode && access.mode !== "CHAMELEON")) {
  window.location.href = "/menu/";
} else {
  const scheme = location.protocol === "https:" ? "wss" : "ws";
  socket = new GameSocket(`${scheme}://${location.host}${access.wsPath}?token=${encodeURIComponent(access.token)}`);
  socket.onMessage = onMessage;
  socket.onStatus = (status) => {
    $("connection").textContent = { connected: "Conectado", connecting: "Conectando…" }[status] || status;
  };
  socket.connect();
  setInterval(tickTimer, 250);
}
