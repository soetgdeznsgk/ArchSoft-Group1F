
let deck = [];
let hand = [];
let handGraphic = [];
let cardW = 60;
let cardH = 100;
let drawButton;
let play = [];
let playGraphic = [];
let landPlayed = false;
let cardType = ["land", "creature"];
let landCount = 0;
let manaCount = 0;
let nextTurn;

class card {
  constructor(status, animal, color, number) {
    this.status = status;
    this.animal = animal;
    this.color = color;
    this.number = number;
    this.xpos = undefined;
    this.position = undefined;
  }

  display(position) {
    if (this.status == "hand") {
      this.position = position;
      this.xpos =
        (width - cardW) / (2 * hand.length) +
        (position * (width - cardW)) / hand.length;
      handGraphic[position].size(cardW, cardH);
      handGraphic[position].position(this.xpos, height - cardH);
      //handGraphic[position].mousePressed();
    } else if (this.status == "play") {
      this.position = position;
      this.xpos =
        (width - cardW) / (2 * play.length) +
        (position * (width - cardW)) / play.length;

      playGraphic[position].size(cardW, cardH);
      playGraphic[position].position(this.xpos, height - cardH * 2 - 10);
    } else if (this.status == "deck") {
      rect(cardW / 2, (cardH * 3) / 4 - position, cardW, cardH);
    }
  }
}

function cardButton(array, arrayGraphic, position) {
  arrayGraphic[position] = createButton(array[position].number);
  arrayGraphic[position].html("<br>" + array[position].animal, true);
  arrayGraphic[position].html("<br>color: " + array[position].color, true);
}


// Constructor

function setup() {
  createCanvas(600, 400);
  
  hand = [ new card(
      "hand",
      "Snake",
      "yellow",
      floor(random(1, 5))), 
      new card(
      "hand",
      "Ocelot",
      "red",
      floor(random(1, 5)))]

    for (let i = 0; i < hand.length; i++){
      cardButton(hand, handGraphic, i)
    }
    
}

function draw() {
  background('darkgreen');
  for (let i = 0; i < hand.length; i++) {
    hand[i].display(i);
  }
}
