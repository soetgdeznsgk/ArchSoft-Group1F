// We're going the state machine route with this script, make sure this is documented
let MENUSTATE
let WINDOW_HEIGHT = 800
let WINDOW_LENGTH = 1280

function setup() {
    createCanvas(WINDOW_LENGTH, WINDOW_HEIGHT)
    enterMenuState()
}

function draw() {
    clear();

    switch (MENUSTATE){
        case "START":
            drawStart();
            break;
        case "LOBBY_PREVIEW":
            drawLobbyPreview();
            break;
        default:
            throw new Error("Menu was given an invalid state");
    }
}

// State setup behavior
async function enterMenuState()
{
    MENUSTATE = "START"

    menuContainer = createDiv();
    menuContainer.addClass("menu-container")
    menuContainer.position(WINDOW_LENGTH / 2, WINDOW_HEIGHT / 3 )

    const logo = createImg('assets/logo.svg', "Logo");
    logo.addClass("logo")
    
    usernameInput = createInput("");
    usernameInput.attribute("placeholder", "USERNAMé")
    usernameInput.addClass("menu-input");
    usernameInput.elt.addEventListener("keydown", (event) => {
        if (event.key === "Enter")
        {
            validateUsername(usernameInput.value())
        }
    })

    continueButton = createButton("SéT USERNAMé");
    continueButton.mousePressed(
        () => validateUsername(usernameInput.value())
    );
    continueButton.addClass("menu-navigation-button")

    menuContainer.child(logo)
    menuContainer.child(usernameInput);
    menuContainer.child(continueButton);
}

// State draw behavior
function drawStart() {
    // DOM nodes handle the draw operation, but for escalability's sake this stays
}

function drawLobbyPreview() {
}

// Username validation
function validateUsername(nameToCheck)
{
    connectSession(nameToCheck.trim());
}

// DATA LAYER
// Conexión del menú con el Lobby Manager (REST y SSE por /api/, a través de Traefik).
// Solo conecta: sesión, crear o unirse a una sala, sala de espera e inicio de la partida.

const SESSION_KEY = "bololo.session";
const GAME_KEY = "bololo.game";

let session = null;
let lobbyEvents = null;
let currentLobby = null;

async function api(method, path, body) {
  const headers = { "Content-Type": "application/json" };
  if (session) headers.Authorization = "Bearer " + session.token;
  const res = await fetch(`/api${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.status === 204) return null;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.message || `Error ${res.status}`);
  return data;
}

async function connectSession(nickname) {
  try {
    const data = await api("POST", "/session", { nickname });
    session = { token: data.token, player: data.player };
    sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
    enterLobbyPreview();
  } catch (err) {
    showStatus(err.message);
  }
}

function enterLobbyPreview() {
  MENUSTATE = "LOBBY_PREVIEW";
  menuContainer.html("");

  const title = createSpan(`Hola, ${session.player.nickname}`);
  const create = createButton("CRéAR SALA");
  create.addClass("menu-navigation-button");
  create.mousePressed(() => joinLobby(() => api("POST", "/lobbies")));

  const code = createInput("");
  code.attribute("placeholder", "CÓDIGO DE SALA");
  code.addClass("menu-input");
  const join = createButton("UNIRSé");
  join.addClass("menu-navigation-button");
  join.mousePressed(() =>
    joinLobby(() => api("POST", "/lobbies/join", { code: code.value().trim().toUpperCase() }))
  );

  const seats = createDiv("");
  seats.id("lobby-seats");
  const start = createButton("INICIAR PARTIDA");
  start.id("start-game");
  start.addClass("menu-navigation-button");
  start.hide();
  start.mousePressed(startGame);

  const status = createSpan("");
  status.id("menu-status");

  for (const el of [title, create, code, join, seats, start, status]) menuContainer.child(el);
}

async function joinLobby(request) {
  try {
    const lobby = await request();
    renderLobby(lobby);
    openLobbyEvents(lobby.id);
  } catch (err) {
    showStatus(err.message);
  }
}

function openLobbyEvents(lobbyId) {
  if (lobbyEvents) lobbyEvents.close();
  lobbyEvents = new EventSource(`/api/lobbies/${lobbyId}/events?token=${encodeURIComponent(session.token)}`);
  lobbyEvents.addEventListener("lobby", (e) => renderLobby(JSON.parse(e.data)));
  lobbyEvents.addEventListener("game_starting", (e) => goToGame(JSON.parse(e.data)));
  lobbyEvents.addEventListener("lobby_closed", () => showStatus("La sala se cerró."));
}

function renderLobby(lobby) {
  currentLobby = lobby;
  const names = lobby.players.map((p) => p.nickname).join(", ");
  select("#lobby-seats").html(`Sala ${lobby.code} · ${lobby.playerCount}/${lobby.maxPlayers}: ${names}`);
  const start = select("#start-game");
  if (lobby.playerCount === lobby.maxPlayers) start.show();
  else start.hide();
}

async function startGame() {
  try {
    const lobby = await api("POST", `/lobbies/${currentLobby.id}/start`);
    if (lobby.game) goToGame(lobby.game);
  } catch (err) {
    showStatus(err.message);
  }
}

function goToGame(game) {
  if (lobbyEvents) lobbyEvents.close();
  sessionStorage.setItem(
    GAME_KEY,
    JSON.stringify({
      gameId: game.gameId,
      wsPath: game.wsPath,
      token: game.token,
      me: session.player,
      players: currentLobby ? currentLobby.players : [],
    })
  );
  window.location.href = "/ingame/";
}

function showStatus(text) {
  const status = select("#menu-status");
  if (status) status.html(text);
  else console.warn(text);
}