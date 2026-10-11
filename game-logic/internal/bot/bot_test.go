package bot

import (
	"math/rand/v2"
	"testing"

	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/game"
)

func card(f game.Figure, k int) game.Card {
	return game.Card{ID: game.CardID(int(f-1)*game.CardsPerFigure + k), Figure: f}
}

func joker(k int) game.Card {
	return game.Card{ID: game.CardID(game.NumFigures*game.CardsPerFigure + k), Joker: true}
}

func TestFindMeldsProducesValidMelds(t *testing.T) {
	r4, _ := game.Spec(4)
	hand := []game.Card{card(1, 0), card(1, 1), card(1, 2), card(2, 0), card(2, 1), joker(0), joker(1), card(3, 0)}
	melds := FindMelds(hand, r4)
	if len(melds) != 2 {
		t.Fatalf("jugadas: %v", melds)
	}
	byID := map[game.CardID]game.Card{}
	for _, c := range hand {
		byID[c.ID] = c
	}
	for _, m := range melds {
		cards := make([]game.Card, len(m))
		for i, id := range m {
			cards[i] = byID[id]
		}
		kind := game.MeldKind(len(m))
		if _, err := game.ValidateMeld(kind, cards); err != nil {
			t.Fatalf("jugada inválida %v: %v", m, err)
		}
	}
}

func TestFindMeldsFails(t *testing.T) {
	r1, _ := game.Spec(1)
	if FindMelds([]game.Card{card(1, 0), card(2, 0), card(3, 0)}, r1) != nil {
		t.Fatal("no hay trío")
	}
	if FindMelds([]game.Card{joker(0), joker(1), joker(2)}, r1) != nil {
		t.Fatal("solo comodines no forman trío")
	}
}

func TestPickDiscardAvoidsJokers(t *testing.T) {
	v := game.PlayerView{Hand: []game.Card{joker(0), card(1, 0), card(1, 1), card(2, 0)}}
	if got := pickDiscard(v, rand.New(rand.NewPCG(1, 1))); got != card(2, 0).ID {
		t.Fatalf("debería botar la figura menos repetida, botó %d", got)
	}
}
