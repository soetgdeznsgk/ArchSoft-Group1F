
const game = {
    players: [
        { name: "Tú", cards: [], local: true },
        { name: "Jugador 2", cards: [1, 2, 3] },
        { name: "Jugador 3", cards: [1, 2, 3] },
        { name: "Jugador 4", cards: [1, 2, 3] }
    ],
    hiddenCount: 48,
    visibleCard: null,
    selected: new Set(),
    hasDrawn: false,
    timer: 30,
    lastTick: 0,
    message: "Selecciona una carta para comenzar.",
    nextCardId: 1
};

const figures = [
    { symbol: "●", color: "#c95f52" },
    { symbol: "◆", color: "#4388d5" },
    { symbol: "▲", color: "#e0b84e" },
    { symbol: "■", color: "#8a69c9" },
    { symbol: "✦", color: "#3da98a" },
    { symbol: "⬟", color: "#e28b48" },
    { symbol: "✿", color: "#df83b0" }
];

function setup() {
    const host = document.getElementById("game-canvas");
    const canvas = createCanvas(host.clientWidth, 500);

    canvas.parent("game-canvas");
    textFont("Arial");

    // Cartas iniciales de demostración.
    for (let i = 0; i < 3; i++) {
        game.players[0].cards.push(makeCard());
    }

    bindButtons();
    game.lastTick = millis();

    window.addEventListener("resize", resizeGame);
    updateInterface();
}

function makeCard() {
    const id = game.nextCardId++;

    // Cuatro cartas de cada figura y comodines simulados.
    const joker = id % 15 === 0;

    return {
        id,
        figure: joker ? -1 : (id - 1) % figures.length,
        joker
    };
}

function draw() {
    background("#174c39");

    drawTableDecoration();
    drawOtherPlayers();
    drawPiles();
    drawDeposits();
    drawLocalHand();
    drawTurnIndicator();
    updateTimer();
}

function drawTableDecoration() {
    noFill();
    stroke("#ffffff18");
    strokeWeight(2);
    ellipse(width / 2, height / 2 - 15, width * 0.72, height * 0.68);

    noStroke();
    fill("#e4c77d");
    textAlign(CENTER, CENTER);
    textSize(19);
    text("BOLoló", width / 2, 32);

    fill("#d9e8df");
    textSize(12);
    text("MESA DE DEMOSTRACIÓN", width / 2, 53);
}

function drawOtherPlayers() {
    const positions = [
        { x: width * 0.25, y: 95 },
        { x: width * 0.75, y: 95 },
        { x: width * 0.12, y: 205 }
    ];

    for (let i = 1; i < game.players.length; i++) {
        const p = game.players[i];
        const pos = positions[i - 1];

        fill("#102d25");
        stroke("#54836c");
        strokeWeight(1);
        rect(pos.x - 62, pos.y - 20, 124, 48, 10);

        noStroke();
        fill("#ffffff");
        textAlign(CENTER, CENTER);
        textSize(13);
        text(p.name, pos.x, pos.y - 5);

        fill("#c8d9ce");
        textSize(11);
        text(`${p.cards.length} cartas`, pos.x, pos.y + 12);

        for (let j = 0; j < Math.min(p.cards.length, 4); j++) {
            drawCard(
                pos.x - 27 + j * 18,
                pos.y + 34,
                31,
                43,
                null,
                false
            );
        }
    }
}

function drawPiles() {
    const centerX = width / 2;
    const y = height * 0.40;

    drawCard(
        centerX - 105, y, 70, 95,
        null, false
    );

    noStroke();
    fill("#ffffff");
    textAlign(CENTER, CENTER);
    textSize(12);
    text("Mazo oculto", centerX - 70, y + 62);
    text(`${game.hiddenCount} cartas`, centerX - 70, y + 78);

    drawCard(
        centerX + 35, y, 70, 95,
        game.visibleCard, true
    );

    fill("#ffffff");
    text("Carta vista", centerX + 70, y + 112);
}

function drawDeposits() {
    const x = width * 0.73;
    const y = height * 0.40;

    noFill();
    stroke("#c7b477");
    strokeWeight(1.5);
    rect(x - 43, y - 7, 86, 75, 8);

    noStroke();
    fill("#f3dfab");
    textAlign(CENTER, CENTER);
    textSize(12);
    text("Depósitos", x, y + 82);
    textSize(10);
    text("Sin jugadas", x, y + 98);
}

function drawLocalHand() {
    const cards = game.players[0].cards;
    const cardWidth = Math.min(68, (width - 30) / Math.max(cards.length, 1) - 6);
    const cardHeight = cardWidth * 1.36;
    const gap = 7;
    const total = cards.length * cardWidth +
        Math.max(0, cards.length - 1) * gap;

    const startX = (width - total) / 2;
    const y = height - cardHeight - 40;

    for (let i = 0; i < cards.length; i++) {
        const card = cards[i];
        const selected = game.selected.has(card.id);

        drawCard(
            startX + i * (cardWidth + gap),
            y - (selected ? 13 : 0),
            cardWidth,
            cardHeight,
            card,
            false
        );
    }

    noStroke();
    fill("#f5e7bd");
    textAlign(CENTER, CENTER);
    textSize(13);
    text("TU MANO · Haz clic en una carta para seleccionarla",
        width / 2, height - 15);
}

function drawCard(x, y, w, h, card, visible) {
    rectMode(CORNER);
    stroke("#e2d5b2");
    strokeWeight(1.5);
    fill("#fffaf0");
    rect(x, y, w, h, 7);

    if (!card || (!visible && card.id === undefined)) {
        noStroke();
        fill("#244b3a");
        rect(x + 5, y + 5, w - 10, h - 10, 4);

        stroke("#d8bc77");
        noFill();
        rect(x + 9, y + 9, w - 18, h - 18, 3);
        noStroke();
        fill("#e4c77d");
        textAlign(CENTER, CENTER);
        textSize(Math.min(22, w * 0.35));
        text("B", x + w / 2, y + h / 2);
        return;
    }

    if (visible && !card) {
        fill("#244b3a");
        noStroke();
        rect(x + 5, y + 5, w - 10, h - 10, 4);
        return;
    }

    if (!card) return;

    if (card.joker) {
        fill("#6c4b91");
        noStroke();
        textAlign(CENTER, CENTER);
        textSize(w * 0.35);
        text("★", x + w / 2, y + h * 0.42);
        fill("#352447");
        textSize(10);
        text("COMODÍN", x + w / 2, y + h * 0.76);
        return;
    }

    const f = figures[card.figure];

    fill(f.color);
    noStroke();
    textAlign(CENTER, CENTER);
    textSize(w * 0.4);
    text(f.symbol, x + w / 2, y + h * 0.45);

    fill("#3b433c");
    textSize(11);
    text(String(card.figure + 1), x + w / 2, y + h * 0.8);
}

function drawTurnIndicator() {
    noStroke();
    fill("#e4c77d");
    textAlign(CENTER, CENTER);
    textSize(12);
    text(game.hasDrawn ? "Termina tu turno: bota una carta" :
        "Tu turno: puedes bajar o depositar antes de tomar",
        width / 2, height - 3);
}

function mousePressed() {
    const cards = game.players[0].cards;
    const cardWidth = Math.min(68, (width - 30) / Math.max(cards.length, 1) - 6);
    const cardHeight = cardWidth * 1.36;
    const gap = 7;
    const total = cards.length * cardWidth +
        Math.max(0, cards.length - 1) * gap;
    const startX = (width - total) / 2;
    const y = height - cardHeight - 40;

    for (let i = 0; i < cards.length; i++) {
        const x = startX + i * (cardWidth + gap);
        const selectedY = y - (game.selected.has(cards[i].id) ? 13 : 0);

        if (mouseX >= x && mouseX <= x + cardWidth &&
            mouseY >= selectedY && mouseY <= selectedY + cardHeight) {
            const id = cards[i].id;

            if (game.selected.has(id)) {
                game.selected.delete(id);
            } else {
                game.selected.add(id);
            }

            game.message = "Selección actualizada.";
            updateInterface();
            return false;
        }
    }

    return false;
}

function bindButtons() {
    document.getElementById("draw-hidden").onclick = () => {
        if (game.hasDrawn) {
            say("Ya tomaste una carta. Debes botar una para terminar.");
            return;
        }

        if (game.hiddenCount <= 0) {
            say("El mazo oculto se agotó. La ronda debe terminar sin ganador.");
            return;
        }

        game.hiddenCount--;
        game.players[0].cards.push(makeCard());
        game.selected.clear();
        game.hasDrawn = true;
        say("Tomaste una carta del mazo oculto.");
        updateInterface();
    };

    document.getElementById("draw-visible").onclick = () => {
        if (game.hasDrawn) {
            say("Ya tomaste una carta en este turno.");
            return;
        }

        if (!game.visibleCard) {
            say("No hay ninguna carta visible para tomar.");
            return;
        }

        game.players[0].cards.push(game.visibleCard);
        game.visibleCard = null;
        game.selected.clear();
        game.hasDrawn = true;
        say("Tomaste la última carta vista.");
        updateInterface();
    };

    document.getElementById("discard").onclick = () => {
        if (!game.hasDrawn) {
            say("Primero debes tomar una carta.");
            return;
        }

        if (game.selected.size !== 1) {
            say("Selecciona exactamente una carta para botar.");
            return;
        }

        const id = [...game.selected][0];
        const cards = game.players[0].cards;
        const index = cards.findIndex(c => c.id === id);

        if (index < 0) {
            say("No se encontró la carta seleccionada.");
            return;
        }

        game.visibleCard = cards.splice(index, 1)[0];
        game.selected.clear();
        game.hasDrawn = false;

        say("Carta botada. La mesa está lista para el siguiente turno simulado.");
        game.timer = 30;
        game.lastTick = millis();
        updateInterface();
    };

    document.getElementById("meld").onclick = () => {
        say("La validación completa de tríos y cuartetos se implementará en la siguiente fase.");
    };

    document.getElementById("deposit").onclick = () => {
        say("Los depósitos se habilitarán cuando esté implementada la validación de jugadas.");
    };
}

function updateTimer() {
    const now = millis();
    const elapsed = (now - game.lastTick) / 1000;

    if (elapsed >= 1) {
        game.timer = Math.max(0, game.timer - Math.floor(elapsed));
        game.lastTick = now;
        updateInterface();
    }

    if (game.timer === 0) {
        game.message = "Tiempo agotado. El intercambio automático aún no está implementado.";
        updateInterface();
    }
}

function updateInterface() {
    document.getElementById("timer").textContent =
        `00:${String(game.timer).padStart(2, "0")}`;

    document.getElementById("draw-hidden").disabled =
        game.hasDrawn || game.hiddenCount === 0;

    document.getElementById("draw-visible").disabled =
        game.hasDrawn || !game.visibleCard;

    document.getElementById("discard").disabled = !game.hasDrawn;

    document.getElementById("game-message").textContent = game.message;

    redraw();
}

function say(message) {
    game.message = message;
    updateInterface();
}

function resizeGame() {
    const host = document.getElementById("game-canvas");
    resizeCanvas(host.clientWidth, 500);
}
