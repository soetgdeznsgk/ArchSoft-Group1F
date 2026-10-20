// Conexión de la mesa con Game Logic por WebSocket (/ws/games/{id}, a través de Traefik).
// Solo conecta: traduce la vista del servidor al estado `game` de sketch.js y
// envía los botones como acciones. Las reglas las decide el servidor.

import { GameSocket } from "./game-socket.js";

const access = JSON.parse(sessionStorage.getItem("bololo.game") || "null");
const names = Object.fromEntries((access?.players || []).map((p) => [p.id, p.nickname]));
const meldNames = { 3: "trío", 4: "cuarteto" };
let socket = null;

function nameOf(id) {
  return names[id] || id;
}

function toCard(card) {
  return { id: card.id, figure: card.joker ? -1 : card.figure - 1, joker: card.joker };
}

function setConnection(text) {
  const el = document.querySelector(".connection");
  if (el) el.lastChild.textContent = ` ${text}`;
}

function applyView(view) {
  const seats = [...view.players].sort((a, b) => a.seat - b.seat);
  const me = seats.findIndex((p) => p.id === view.you);
  const ordered = seats.slice(me).concat(seats.slice(0, me));

  game.players = ordered.map((p) => ({
    name: nameOf(p.id) + (p.is_turn ? " ▶" : ""),
    cards: p.id === view.you ? view.hand.map(toCard) : Array(p.card_count).fill(null),
    local: p.id === view.you,
  }));
  game.hiddenCount = view.hidden_count;
  game.visibleCard = view.top_discard ? toCard(view.top_discard) : null;
  game.hasDrawn = view.can_discard;
  game.timer = Math.ceil(view.turn_remaining_ms / 1000);
  game.lastTick = millis();
  const handIds = new Set(view.hand.map((c) => c.id));
  for (const id of [...game.selected]) if (!handIds.has(id)) game.selected.delete(id);

  if (view.spec) {
    document.getElementById("round-number").textContent = `${view.round} de 5`;
    document.getElementById("round-target").textContent = view.spec.melds.map((k) => meldNames[k]).join(" y ");
  }
  if (view.final) {
    say(`Partida terminada. Gana: ${view.final.winners.map(nameOf).join(", ")}`);
  } else if (view.current_turn) {
    say(view.current_turn === view.you ? "Es tu turno." : `Turno de ${nameOf(view.current_turn)}.`);
  } else {
    updateInterface();
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
    default:
      if (msg.player) console.info(`[mesa] ${nameOf(msg.player)}: ${msg.type}`);
  }
}

function selectedCards() {
  return [...game.selected];
}

function send(action) {
  if (!socket || !socket.send(action)) say("Sin conexión con el servidor.");
}

function connectButtons() {
  document.getElementById("draw-hidden").onclick = () => send({ type: "draw", source: "hidden" });
  document.getElementById("draw-visible").onclick = () => send({ type: "draw", source: "discard" });
  document.getElementById("discard").onclick = () => {
    const cards = selectedCards();
    if (cards.length !== 1) return say("Selecciona exactamente una carta para botar.");
    send({ type: "discard", card: cards[0] });
  };
  document.getElementById("meld").onclick = () => {
    const cards = selectedCards();
    if (cards.length === 0) return say("Selecciona las cartas de tu jugada.");
    send({ type: "lay_down", melds: [cards] });
  };
  document.getElementById("deposit").onclick = () => {
    const cards = selectedCards();
    if (cards.length !== 1) return say("Selecciona una carta para depositar.");
    send({ type: "deposit", card: cards[0], bank: 0 });
  };
}

if (!access) {
  window.location.href = "/menu/";
} else {
  window.bindButtons = connectButtons;
  window.addEventListener("load", () => {
    const scheme = location.protocol === "https:" ? "wss" : "ws";
    socket = new GameSocket(`${scheme}://${location.host}${access.wsPath}?token=${encodeURIComponent(access.token)}`);
    socket.onMessage = onMessage;
    socket.onStatus = (status) => setConnection(status === "connected" ? "Conectado" : status);
    socket.connect();
  });
}
