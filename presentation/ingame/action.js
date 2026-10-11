
const scoreUrl = "http://localhost:3000/score"; // placeholder en lo que tenemos el back ajjajaja
let score = 0;
let getScoreButton;
let postScoreButton;
let personalScoreLabel;

function scoreButtons() {
  getScoreButton = createButton("GET score");
  getScoreButton.position(width - 110, 10);
  getScoreButton.mousePressed(getScore);

  postScoreButton = createButton("POST score");
  postScoreButton.position(width - 550, 10);
  postScoreButton.mousePressed(postScore);

  personalScoreLabel = createSpan("score: " + score);
  personalScoreLabel.position(width - 110, 40);
  personalScoreLabel.style("color", "white");
}

async function getScore() {
  try {
    const res = await fetch(scoreUrl);
    if (!res.ok) throw new Error("GET " + res.status);
    const data = await res.json();
    score = data.score;
    personalScoreLabel.html("score: " + score);
  } catch (err) {
    console.error(err);
    personalScoreLabel.html("GET failed");
  }
}

async function postScore() {


    // provisional para ver una puntuacion random
    // luego ya le metemos los juegos que arme c/ jugador o como funcione ajsjkasj

  score = 0;
  for (let i = 0; i < hand.length; i++) {
    score += hand[i].number;
  }

  try {
    const res = await fetch(scoreUrl, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ score: score }),
    });
    if (!res.ok) throw new Error("POST " + res.status);
    scoreLabel.html("sent: " + score);
  } catch (err) {
    console.error(err);
    scoreLabel.html("POST failed");
  }
}