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
let soundToggle;
let soundEnabled = localStorage.getItem("bololo.sound-enabled") !== "True";
const activeAudios = new Set();

function setup() {
    document.getElementById("ssr-shell")?.remove();
    createCanvas(WINDOW_LENGTH, WINDOW_HEIGHT)
    createSoundToggle();
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
            print("Entered lobby preview state")
            //drawLobbyPreview();
            break;
        default:
            throw new Error("Menu was given an invalid state");
    }
}

// State draw behavior
function drawStart() {
    // DOM nodes handle the draw operation, but for escalability's sake this stays
}

function createSoundToggle() {
    soundToggle = createButton("");
    soundToggle.addClass("sound-toggle");
    soundToggle.attribute("aria-label", soundEnabled ? "Desactivar sonidos" : "Activar sonidos");
    soundToggle.attribute("aria-pressed", String(!soundEnabled));
    soundToggle.parent(document.body);
    soundToggle.mousePressed(toggleSound);
    updateSoundToggle();
}

function updateSoundToggle() {
    soundToggle?.toggleClass("is-muted", !soundEnabled);
    soundToggle?.attribute("aria-label", soundEnabled ? "Desactivar sonidos" : "Activar sonidos");
    soundToggle?.attribute("aria-pressed", String(!soundEnabled));
}

function toggleSound() {
    soundEnabled = !soundEnabled;
    localStorage.setItem("bololo.sound-enabled", String(soundEnabled));

    if (!soundEnabled) {
        for (const audio of activeAudios) {
            audio.pause();
            audio.currentTime = 0;
        }
    }

    updateSoundToggle();
}

function registerAudio(audio) {
    activeAudios.add(audio);
    audio.addEventListener("ended", () => activeAudios.delete(audio), { once: true });
    return audio;
}

// State setup behavior
async function enterMenuState()
{
    MENUSTATE = "START"

    menuContainer = createDiv();
    menuContainer.addClass("menu-container")

    const logo = createImg('assets/logo.png', "Logo");
    logo.addClass("logo")
    logoHoverAudio = registerAudio(new Audio("assets/tribu.mp3"));
    logoHoverAudio.preload = "auto";
    logoHoverAudio.volume = 0.5;
    logo.elt.addEventListener("mouseenter", () => {
        if (!soundEnabled) return;
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
let lobbyModeSelect = null;

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

// Las sesiones las emite Auth, también las de invitado (solo con un nombre).
async function authRequest(path, body) {
  const response = await fetch(`/auth/${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.message || `Error ${response.status}`);
  return data;
}

async function connectSession(nickname) {
  try {
    const data = await authRequest("guest", { nickname });
    await beginSession(data, { playWelcome: true });
  } catch (err) {
    showAuthError(err.message);
  }
}

async function authenticate(action, username, password) {
  const normalizedUsername = username.trim();
  if (!/^[A-Za-z0-9_.-]+$/.test(normalizedUsername)) {
    showAuthError("USERNAMé debe estar escrito usando letras, números, punto, guion o guion bajo.");
    return;
  }
  if (normalizedUsername.length < 3 || normalizedUsername.length > 20) {
    showAuthError("USERNAMé debe tener entre 3 y 20 carácteres");
    return
  }
  const passwordBytes = new TextEncoder().encode(password).length;
  if (passwordBytes < 8) {
    showAuthError("La contraseña debe tener al menos 8 caracteres");
    return;
  }
  if (passwordBytes > 72) {
    showAuthError("La contraseña es demasiado larga");
    return;
  }
  try {
    const data = await authRequest(action, { username: normalizedUsername, password });
    await beginSession(data);
  } catch (err) {
    showAuthError("Servidor :" + err.message);
  }
}

// El jugador que aparece en las salas es el del Lobby: se pide con la sesión recién emitida.
async function beginSession(data, options = {}) {
  session = { token: data.token, player: data.player, account: data.account ?? null };
  try {
    const me = await api("GET", "/session");
    session.player = me.player;
  } catch (err) {
    session = null;
    throw err;
  }
  sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
  errorWindow.hide();
  if (options.playWelcome) await playWelcomeSound();
  enterLobbyPreview();
}

async function playWelcomeSound() {
  if (!soundEnabled) return;

  const welcomeAudio = registerAudio(new Audio("assets/welcome.mp3"));
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
  menuContainer.addClass("lobby-preview");

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
  title.addClass("lobby-user");
  title.parent(document.body);
  // Modo de juego de la sala que se va a crear (quien se une por código no elige: hereda el de la sala).
  const modeLabel = createSpan("ELIGE TU MODO DE JUEGO");
  modeLabel.addClass("lobby-panel-title");
  const separator = createDiv("");
  separator.addClass("lobby-title-separator");
  const mode = createSelect();
  mode.id("lobby-mode");
  lobbyModeSelect = mode;
  mode.addClass("lobby-mode-native");
  mode.hide();
  mode.attribute("aria-label", "Modo de juego");
  for (const [value, label] of Object.entries(MODE_LABELS)) mode.option(label, value);
  mode.selected("CLASSIC");

  const modeSection = createDiv("");
  modeSection.addClass("lobby-mode-section");
  const modeHeading = createSpan("MODO");
  modeHeading.addClass("lobby-field-heading");
  modeSection.child(modeHeading);

  const modeCards = createDiv("");
  modeCards.addClass("lobby-mode-cards");
  const classicCard = createButton(
    '<strong>Clásico</strong><span>4 jugadores</span>'
  );
  classicCard.id("classic-mode-card");
  classicCard.addClass("lobby-mode-card");
  classicCard.attribute("type", "button");
  classicCard.elt.innerHTML = "<strong>Clásico</strong><span>4 jugadores</span>";
  classicCard.mousePressed(() => selectLobbyMode("CLASSIC"));

  const chameleonCard = createButton(
    '<strong>Camaleón</strong><span>8 colores, sin repetir</span>'
  );
  chameleonCard.id("chameleon-mode-card");
  chameleonCard.addClass("lobby-mode-card");
  chameleonCard.attribute("type", "button");
  chameleonCard.elt.innerHTML = "<strong>Camaleón</strong><span>8 colores, sin repetir</span>";
  chameleonCard.mousePressed(() => selectLobbyMode("CHAMELEON"));
  modeCards.child(classicCard);
  modeCards.child(chameleonCard);
  modeSection.child(modeCards);

  async function selectLobbyMode(nextMode) {
    mode.selected(nextMode);
    updateModeCards(nextMode);
    if (currentLobby && currentLobby.status === "WAITING") {
      try {
        const lobby = await api("POST", `/lobbies/${currentLobby.id}/mode`, { mode: nextMode });
        currentLobby = lobby;
        renderLobby(lobby);
        showStatus(`Modo cambiado a ${MODE_LABELS[lobby.mode]}.`);
      } catch (err) {
        mode.selected(currentLobby.mode);
        updateModeCards(currentLobby.mode);
        showStatus(err.message);
      }
    }
    if (rankingPanel && rankingPanel.elt.style.display !== "none") loadRanking(nextMode);
  }

  mode.changed(() => selectLobbyMode(mode.value()));

  rankingButton = createButton("Ver rankings");
  rankingButton.addClass("ranking-image-button");
  rankingButton.addClass("lobby-ranking-button");
  rankingButton.parent(document.body);
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
  create.addClass("lobby-create-button");
  create.mousePressed(() => joinLobby(() => api("POST", "/lobbies", { mode: mode.value() })));

  const code = createInput("");
  code.attribute("placeholder", "CÓDIGO DE SALA");
  code.addClass("lobby-code-input");
  const join = createButton("UNIRSé");
  join.addClass("lobby-join-button");
  join.mousePressed(() =>
    joinLobby(() => api("POST", "/lobbies/join", { code: code.value().trim().toUpperCase() }))
  );

  const actions = createDiv("");
  actions.addClass("lobby-actions");
  const newGame = createDiv("");
  newGame.addClass("lobby-action-column");
  const newGameHeading = createSpan("NUEVA PARTIDA");
  newGameHeading.addClass("lobby-field-heading");
  newGame.child(newGameHeading);
  newGame.child(create);

  const joinGroup = createDiv("");
  joinGroup.addClass("lobby-action-column");
  const joinHeading = createSpan("TENGO UN CóDIGO");
  joinHeading.addClass("lobby-field-heading");
  joinGroup.child(joinHeading);
  joinGroup.child(code);
  joinGroup.child(join);
  actions.child(newGame);
  actions.child(joinGroup);

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
  rankingPanel.addClass("lobby-ranking-panel");
  rankingPanel.parent(document.body);
  rankingPanel.hide();

  for (const el of [modeLabel, separator, mode, modeSection, actions, seats, fillBots, start, status]) {
    menuContainer.child(el);
  }
  updateModeCards("CLASSIC");
}

// Ranking del modo clásico que el servidor SSR ya insertó en el HTML (presentation-ssr/src/views.js).
// Se usa una sola vez: la primera apertura del panel no vuelve a consultar a Ratings.
let ssrLeaderboard = readSsrLeaderboard();

function readSsrLeaderboard() {
  try {
    return JSON.parse(document.getElementById("ssr-data")?.textContent || "{}").leaderboard || null;
  } catch {
    return null;
  }
}

// Ratings Service (/api/ratings/, ver docs/ratings-service.md): ranking del modo elegido.
// Si el servicio no responde, el menú sigue funcionando: solo se avisa en el panel.
async function loadRanking(mode) {
  const panel = rankingPanel.elt;
  clearTimeout(rankingMessageTimeout);
  panel.replaceChildren(rankingLine("p", "Cargando rankings..."));
  const preloaded = mode === "CLASSIC" ? ssrLeaderboard : null;
  ssrLeaderboard = null;
  try {
    const [board, mine] = await Promise.all([
      preloaded || api("GET", `/ratings/leaderboard?mode=${mode}&limit=10`),
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

function updateModeCards(mode) {
  const classic = document.getElementById("classic-mode-card");
  const chameleon = document.getElementById("chameleon-mode-card");
  if (classic) classic.classList.toggle("selected", mode === "CLASSIC");
  if (chameleon) chameleon.classList.toggle("selected", mode === "CHAMELEON");
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
  if (lobbyModeSelect) {
    lobbyModeSelect.selected(lobby.mode);
    updateModeCards(lobby.mode);
    const isHost = lobby.players.length > 0 && lobby.players[0].id === session.player.id;
    lobbyModeSelect.elt.disabled = lobby.status !== "WAITING" || !isHost;
    const classicCard = document.getElementById("classic-mode-card");
    const chameleonCard = document.getElementById("chameleon-mode-card");
    if (classicCard) classicCard.disabled = lobby.status !== "WAITING" || !isHost;
    if (chameleonCard) chameleonCard.disabled = lobby.status !== "WAITING" || !isHost;
  }
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