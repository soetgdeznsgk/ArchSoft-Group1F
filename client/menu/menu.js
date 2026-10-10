// We're going the state machine route with this script, make sure this is documented
let MENUSTATE = {
    set changeTo(newState) {
        switch(newState) {
            case "START":
                onStartMenuState();
                break;
            //case "LOBBY_PREVIEW"
        }
    }
}
let username = ""

function setup() {
    createCanvas(800,600)
    MENUSTATE = "START";
}

function draw() {
    background('darkgreen');

    switch (MENUSTATE){
        case "START":
            drawStart();
            break;
        case "LOBBY_PREVIEW":
            drawLobbyPreview();
            break;

        default:
            // crash
            throw new Error("Menu was given an invalid state");
    }
}

// State setup behavior
function onStartMenuState()
{
    usernameInput = createInput("");
    usernameInput.addClass();
    
}

// State draw behavior
function drawStart() {
    // prompt the user for a username, check if the name is valid
}

function drawLobbyPreview() {

}

