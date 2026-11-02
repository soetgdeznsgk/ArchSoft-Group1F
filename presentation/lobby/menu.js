// We're going the state machine route with this script, make sure this is documented
let MENUSTATE
let WINDOW_HEIGHT = 800
let WINDOW_LENGTH = 1280
let errorWindow;
let errorWindowTimeout;
let errorCharacter;
let errorGroup;
let logoHoverAudio;
let logoHoverAudioTimeout;

function setup() {
    document.getElementById("ssr-shell")?.remove();
    createCanvas(WINDOW_LENGTH, WINDOW_HEIGHT)
    enterMenuState()

    errorWindow = createSpan("");
    errorWindow.addClass("menu-error-msg");
    errorWindow.addClass("section-label");

    errorCharacter = createImg("assets/personaje.png", "Personaje");
    errorCharacter.addClass("menu-error-character");

    errorGroup = createDiv("");
    errorGroup.addClass("menu-error-group");
    errorCharacter.parent(errorGroup);
    errorWindow.parent(errorGroup);
    errorGroup.hide();
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

// State draw behavior
function drawStart() {
    // DOM nodes handle the draw operation, but for escalability's sake this stays
}

// State setup behavior
async function enterMenuState()
{
    MENUSTATE = "START"

    menuContainer = createDiv();
    menuContainer.addClass("menu-container")

    const logo = createImg('assets/logo.png', "Logo");
    logo.addClass("logo")
    logoHoverAudio = new Audio("assets/tribu.mp3");
    logoHoverAudio.preload = "auto";
    logoHoverAudio.volume = 0.5;
    logo.elt.addEventListener("mouseenter", () => {
        clearTimeout(logoHoverAudioTimeout);
        logoHoverAudio.pause();
        logoHoverAudio.currentTime = 0;
        logoHoverAudio.play().catch((error) => {
            console.warn("No se pudo reproducir el audio del logo.", error);
        });
        logoHoverAudioTimeout = setTimeout(() => {
            logoHoverAudio.pause();
            logoHoverAudio.currentTime = 0;
        }, 5000);
    });
    
    const infoGuest = createSpan("MODO INVITADO")
    infoGuest.addClass("section-label")

    usernameGuestInput = createInput("");
    usernameGuestInput.attribute("placeholder", "NOMBRE DE INVITADO")
    usernameGuestInput.addClass("menu-input");

    continueButton = createButton("ENTRAR COMO INVITADO");
    continueButton.mousePressed(
        () => validateGuestname(usernameGuestInput.value())
    );
    continueButton.addClass("menu-navigation-button")
    continueButton.addClass("guest-image-button")
    continueButton.attribute("aria-label", "Entrar como invitado")

    const infoUser = createSpan("RéAL?")

    username = createInput("");
    username.attribute("placeholder", "USERNAMé")
    username.addClass("menu-input")
    pw = createInput("", "password");
    pw.attribute("placeholder", "PáSSWóRD")
    pw.addClass("menu-input")

    const buttonRow = createDiv();
    buttonRow.addClass("button-row");

    loginButton = createButton("LóGIN AS RéAL");
    loginButton.addClass("menu-navigation-button");
    loginButton.mousePressed(() => loginRequested(username.value(), pw.value()));

    signinButton = createButton("SíGN IN AS RéAL");
    signinButton.addClass("menu-navigation-button");
    signinButton.mousePressed(() => signinRequested(username.value(), pw.value()));

    buttonRow.child(loginButton);
    buttonRow.child(signinButton);

    menuContainer.child(logo)
    menuContainer.child(infoGuest)
    menuContainer.child(usernameGuestInput);
    menuContainer.child(continueButton);
    menuContainer.child(infoUser);
    menuContainer.child(username);
    menuContainer.child(pw);
    menuContainer.child(buttonRow);
}

// Username validation
function validateGuestname(nameToCheck)
{
    const nickname = nameToCheck.trim();
    if (!nickname) {
        showAuthError("Ingresa un nombre válido.");
        return;
    }
    connectSession(nickname);
}

async function signinRequested(username, password) {
  await authenticate("register", username, password);
}

async function loginRequested(username, password) {
  await authenticate("login", username, password);
}

// DATA LAYER
// Conexión del menú con el Lobby Manager (REST y SSE por /api/, a través de Traefik).
// Solo conecta: sesión, crear o unirse a una sala, sala de espera e inicio de la partida.

const SESSION_KEY = "bololo.session";
const GAME_KEY = "bololo.game";
// Modos de juego del Lobby y la pantalla que abre cada uno.
const MODE_LABELS = { CLASSIC: "Clásico (4 jugadores)", CHAMELEON: "Camaleón (2 a 4 jugadores)" };
const GAME_PAGES = { CLASSIC: "/ingame/", CHAMELEON: "/chameleon/" };

let session = null;
let lobbyEvents = null;
let currentLobby = null;
let rankingPanel = null;
let rankingButton = null;
let rankingMessageTimeout = null;

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
    await beginSession(data, { playWelcome: true });
  } catch (err) {
    showAuthError(err.message);
  }
}

async function authenticate(action, username, password) {
  if (!username || !password) {
    showAuthError("FíLL THE FIéLDS!");
    return;
  }
  try {
    const response = await fetch(`/auth/${action}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: username.trim(), password }),
    });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.message || `Error ${response.status}`);
    beginSession(data);
  } catch (err) {
    showAuthError(err.message);
  }
}

async function beginSession(data, options = {}) {
  session = { token: data.token, player: data.player, account: data.account };
  sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
  errorWindow.hide();
  if (options.playWelcome) await playWelcomeSound();
  enterLobbyPreview();
}

async function playWelcomeSound() {
  const welcomeAudio = new Audio("assets/welcome.mp3");
  welcomeAudio.preload = "auto";

  try {
    await welcomeAudio.play();
    await new Promise((resolve) => {
      welcomeAudio.addEventListener("ended", resolve, { once: true });
    });
  } catch (error) {
    console.error("No se pudo reproducir el sonido de bienvenida.", error);
    showAuthError("No se pudo reproducir el sonido de bienvenida.");
  }
}

function showAuthError(message) {
  errorWindow.elt.textContent = message;
  errorGroup.show();
  clearTimeout(errorWindowTimeout);
  errorWindowTimeout = setTimeout(() => {
    errorGroup.hide();
  }, 3000);
}

function enterLobbyPreview() {
  MENUSTATE = "LOBBY_PREVIEW";
  menuContainer.html("");

  const previousBackButton = document.querySelector(".back-image-button");
  if (previousBackButton) previousBackButton.remove();

  const backButton = createButton("Volver al menú");
  backButton.addClass("back-image-button");
  backButton.attribute("aria-label", "Volver al menú");
  backButton.parent(document.body);
  backButton.mousePressed(() => {
    sessionStorage.removeItem(SESSION_KEY);
    sessionStorage.removeItem(GAME_KEY);
    window.location.assign("/menu/");
  });

  const title = createSpan(`Hola, ${session.player.nickname}`);
  title.addClass("section-label");
  // Modo de juego de la sala que se va a crear (quien se une por código no elige: hereda el de la sala).
  const modeLabel = createSpan("SELECCIONA EL MODO DE JUEGO");
  modeLabel.addClass("section-label");
  const mode = createSelect();
  mode.id("lobby-mode");
  mode.addClass("menu-input");
  mode.attribute("aria-label", "Modo de juego");
  for (const [value, label] of Object.entries(MODE_LABELS)) mode.option(label, value);
  mode.selected("CLASSIC");
  mode.changed(() => {
    if (rankingPanel && rankingPanel.elt.style.display !== "none") loadRanking(mode.value());
  });

  rankingButton = createButton("Ver rankings");
  rankingButton.addClass("ranking-image-button");
  rankingButton.attribute("aria-label", "Ver rankings");
  rankingButton.mousePressed(() => {
    if (rankingPanel.elt.style.display === "none") {
      rankingPanel.show();
      loadRanking(mode.value());
    } else {
      rankingPanel.hide();
    }
  });

  const create = createButton("CRéAR SALA");
  create.addClass("menu-navigation-button");
  create.mousePressed(() => joinLobby(() => api("POST", "/lobbies", { mode: mode.value() })));

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
  const fillBots = createButton("LLéNAR CóN BóTS");
  fillBots.id("fill-bots");
  fillBots.addClass("menu-navigation-button");
  fillBots.hide();
  fillBots.mousePressed(fillWithBots);
  const start = createButton("INICIAR PARTIDA");
  start.id("start-game");
  start.addClass("menu-navigation-button");
  start.hide();
  start.mousePressed(startGame);

  const status = createSpan("");
  status.id("menu-status");

  rankingPanel = createDiv("");
  rankingPanel.addClass("ranking-panel");
  rankingPanel.hide();

  for (const el of [title, modeLabel, mode, create, code, join, rankingButton, rankingPanel, seats, fillBots, start, status]) {
    menuContainer.child(el);
  }
}

// Ratings Service (/api/ratings/, ver docs/ratings-service.md): ranking del modo elegido.
// Si el servicio no responde, el menú sigue funcionando: solo se avisa en el panel.
async function loadRanking(mode) {
  const panel = rankingPanel.elt;
  clearTimeout(rankingMessageTimeout);
  panel.replaceChildren(rankingLine("p", "Cargando rankings..."));
  try {
    const [board, mine] = await Promise.all([
      api("GET", `/ratings/leaderboard?mode=${mode}&limit=10`),
      session.account ? api("GET", `/ratings/players/${session.account.id}`) : null,
    ]);
    const players = Array.isArray(board.players) ? board.players : [];
    panel.replaceChildren(
      rankingLine("h3", `Ranking · ${MODE_LABELS[mode]}`),
      rankingTable(players),
      myRatingLine(mine, mode)
    );
    if (players.length === 0) {
      rankingMessageTimeout = setTimeout(() => rankingPanel.hide(), 5000);
    }
  } catch (err) {
    panel.replaceChildren(rankingLine("p", "El ranking no está disponible por ahora."));
  }
}

function rankingLine(tag, text) {
  const line = document.createElement(tag);
  line.textContent = text;
  return line;
}

function rankingTable(players) {
  if (players.length === 0) return rankingLine("p", "Aún no hay rankings disponibles.");
  const table = document.createElement("table");
  const headers = ["#", "Jugador", "Puntos", "Partidas", "Victorias"];
  const rows = players.map((p) => [p.position, p.nickname, p.rating, p.games, p.wins]);
  for (const [i, cells] of [headers, ...rows].entries()) {
    const row = table.insertRow();
    for (const cell of cells) row.appendChild(rankingLine(i === 0 ? "th" : "td", cell));
  }
  return table;
}

function myRatingLine(mine, mode) {
  if (!session.account) return rankingLine("p", "Entra con una cuenta para aparecer en el ranking.");
  const rating = mine.ratings.find((r) => r.mode === mode);
  if (!rating) return rankingLine("p", "Juega con otra cuenta en la sala para entrar al ranking.");
  return rankingLine("p", `Tu puesto: #${rating.position} · ${rating.rating} puntos`);
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
  const modeLabel = MODE_LABELS[lobby.mode] || lobby.mode || MODE_LABELS.CLASSIC;
  select("#lobby-seats").html(
    `Sala ${lobby.code} · ${modeLabel} · ${lobby.playerCount}/${lobby.maxPlayers}: ${names}`
  );
  const start = select("#start-game");
  // CLASSIC necesita la sala llena (4); CHAMELEON puede iniciar desde 2.
  if (lobby.playerCount >= (lobby.minPlayers ?? lobby.maxPlayers)) start.show();
  else start.hide();
  // Solo el anfitrión (el de menor asiento; la lista llega ordenada) puede llenar con bots.
  const fill = select("#fill-bots");
  const isHost = lobby.players.length > 0 && lobby.players[0].id === session.player.id;
  if (isHost && lobby.status === "WAITING" && lobby.playerCount < lobby.maxPlayers) fill.show();
  else fill.hide();
}

// Bot Service (/api/bots/, ver docs/bot-service.md): ocupa los cupos libres con bots.
async function fillWithBots() {
  try {
    const result = await api("POST", `/bots/lobbies/${currentLobby.id}/fill`);
    renderLobby(result.lobby);
  } catch (err) {
    showStatus(err.message);
  }
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
  const mode = game.mode || (currentLobby && currentLobby.mode) || "CLASSIC";
  sessionStorage.setItem(
    GAME_KEY,
    JSON.stringify({
      gameId: game.gameId,
      mode,
      wsPath: game.wsPath,
      token: game.token,
      me: session.player,
      players: currentLobby ? currentLobby.players : [],
    })
  );
  window.location.href = GAME_PAGES[mode] || GAME_PAGES.CLASSIC;
}

function showStatus(text) {
  const status = select("#menu-status");
  if (status) status.html(text);
  else console.warn(text);
}