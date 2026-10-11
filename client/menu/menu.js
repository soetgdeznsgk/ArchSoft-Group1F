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
    print(nameToCheck);
    // Afterwards, await for backend's confirmation
}