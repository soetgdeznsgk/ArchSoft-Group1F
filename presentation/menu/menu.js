// We're going the state machine route with this script, make sure this is documented
let MENUSTATE
let WINDOW_HEIGHT = 800
let WINDOW_LENGTH = 1280
let username;
let menuContainer;

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

async function enterLobbyPreviewState()
{
    try {
        await createSession(username);
        const lobbies = await getLobbyList();
        print(lobbies.length ," were recieved from api");

        menuContainer.hide();
        menuContainer = createDiv();
        menuContainer.addClass("menu-container")
        menuContainer.position(WINDOW_LENGTH / 2, WINDOW_HEIGHT / 3 )

        const title = createElement("h2", "Available Lobbies");
        menuContainer.child(title);

        if (lobbies.length === 0) {
            menuContainer.child(createP("No lobbies available."));
        }

        for (const lobby of lobbies) {
            const lobbyButton = createButton(
                `${lobby.code} (${lobby.playerCount}/${lobby.maxPlayers})`
            );
            lobbyButton.addClass("menu-navigation-button");
            menuContainer.child(lobbyButton);
        }
        
        createLobby = createButton("CRéATE LóBBY");
        createLobby.addClass("menu-navigation-button");
        createLobby.mousePressed(
            () => 
        )

        MENUSTATE = "LOBBY_PREVIEW";
    }
    catch (error)
    {
        console.error("Failed to load lobby list:", error);
    }
}

// State draw behavior
function drawStart() {
    // DOM nodes handle the draw operation, but for escalability's sake this stays
}

function drawLobbyPreview() {

}

async function validateUsername(nameToCheck)
{
    print(nameToCheck);
    // Afterwards, await for backend's confirmation
    username = nameToCheck.trim();
    if (!username) return;
    enterLobbyPreviewState();
}

// Communication with api gateway

async function createSession(nickname)
{
    const response = await fetch("/api/session", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ nickname }),
    });

    if (!response.ok) {
        throw new Error(`Could not create session (${response.status})`);
    }
    const session = await response.json();
    localStorage.setItem("sessionToken", session.token);
    return session;
}

async function getLobbyList() {
    const token = localStorage.getItem("sessionToken");
    if (!token) {
        throw new Error("Create or restore a session before loading lobbies.");
    }

    const response = await fetch("/api/lobbies", {
        method: "GET",
        headers: {
            Authorization: `Bearer ${token}`,
        },
    });

    if (!response.ok) {
        throw new Error(`Could not load lobbies (${response.status})`);
    }

    return response.json();
}

async function createLobby() {
    const response = await fetch("api/lobbies", {
        method: "POST",
        headers: {
            
        } 
    })
}