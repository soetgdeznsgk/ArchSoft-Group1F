package game

import (
	"errors"
	"testing"
)

// RF-12: mazo de 60 cartas, 56 normales (7 figuras x 8) y 4 comodines.
func TestNewDeckComposition(t *testing.T) {
	deck := NewDeck()
	if len(deck) != 60 {
		t.Fatalf("el mazo tiene %d cartas", len(deck))
	}
	perFigure := map[Figure]int{}
	jokers := 0
	for i, c := range deck {
		if c.ID != CardID(i) {
			t.Fatalf("ID %d en la posición %d", c.ID, i)
		}
		if c.Joker {
			jokers++
			if c.Figure != NoFigure {
				t.Fatalf("comodín con figura: %+v", c)
			}
		} else {
			perFigure[c.Figure]++
		}
	}
	if jokers != 4 || len(perFigure) != 7 {
		t.Fatalf("comodines=%d figuras=%d", jokers, len(perFigure))
	}
	for f, n := range perFigure {
		if n != 8 || f < 1 || f > 7 {
			t.Fatalf("figura %d tiene %d cartas", f, n)
		}
	}
}

// RF-32: 1 punto por carta normal y 5 por comodín.
func TestPenalty(t *testing.T) {
	if nc(1, 0).Penalty() != 1 || jk(0).Penalty() != 5 {
		t.Fatal("penalización incorrecta")
	}
	if got := HandPenalty([]Card{nc(1, 0), nc(2, 0), jk(0)}); got != 7 {
		t.Fatalf("HandPenalty = %d, se esperaba 7", got)
	}
	if nc(3, 1).String() != "F3#17" || jk(1).String() != "comodín#57" {
		t.Fatalf("String: %s %s", nc(3, 1), jk(1))
	}
}

// RF-13: tabla de rondas.
func TestRoundSpecs(t *testing.T) {
	want := []struct {
		cards int
		melds []MeldKind
		text  string
	}{
		{3, []MeldKind{Trio}, "un trío"},
		{4, []MeldKind{Quartet}, "un cuarteto"},
		{6, []MeldKind{Trio, Trio}, "un trío y un trío"},
		{7, []MeldKind{Quartet, Trio}, "un cuarteto y un trío"},
		{8, []MeldKind{Quartet, Quartet}, "un cuarteto y un cuarteto"},
	}
	for r := 1; r <= TotalRounds; r++ {
		s, ok := Spec(r)
		if !ok || s.Number != r || s.CardsPerPlayer != want[r-1].cards || s.Describe() != want[r-1].text {
			t.Fatalf("ronda %d: %+v", r, s)
		}
		sum := 0
		for i, k := range s.Melds {
			if k != want[r-1].melds[i] {
				t.Fatalf("ronda %d: jugadas %v", r, s.Melds)
			}
			sum += k.Size()
		}
		if sum != s.CardsPerPlayer {
			t.Fatalf("ronda %d: las jugadas suman %d cartas, se reparten %d", r, sum, s.CardsPerPlayer)
		}
	}
	if _, ok := Spec(0); ok {
		t.Fatal("Spec(0) debería fallar")
	}
	if _, ok := Spec(6); ok {
		t.Fatal("Spec(6) debería fallar")
	}
	s, _ := Spec(1)
	s.Melds[0] = Quartet
	if s2, _ := Spec(1); s2.Melds[0] != Trio {
		t.Fatal("Spec debe devolver una copia")
	}
	if Trio.String() != "trío" || Quartet.String() != "cuarteto" || MeldKind(5).String() != "jugada(5)" {
		t.Fatal("MeldKind.String")
	}
	if Trio.MaxJokers() != 2 || Quartet.MaxJokers() != 3 {
		t.Fatal("RF-27: máximo 2 comodines en trío y 3 en cuarteto")
	}
}

// RF-26, RF-27: validación de jugadas con comodines.
func TestValidateMeld(t *testing.T) {
	cases := []struct {
		name  string
		kind  MeldKind
		cards []Card
		fig   Figure
		ok    bool
	}{
		{"trío normal", Trio, []Card{nc(2, 0), nc(2, 1), nc(2, 2)}, 2, true},
		{"trío con 1 comodín", Trio, []Card{nc(2, 0), jk(0), nc(2, 2)}, 2, true},
		{"trío con 2 comodines", Trio, []Card{jk(0), jk(1), nc(5, 0)}, 5, true},
		{"trío con 3 comodines", Trio, []Card{jk(0), jk(1), jk(2)}, 0, false},
		{"cuarteto normal", Quartet, []Card{nc(7, 0), nc(7, 1), nc(7, 2), nc(7, 3)}, 7, true},
		{"cuarteto con 3 comodines", Quartet, []Card{jk(0), nc(1, 0), jk(2), jk(3)}, 1, true},
		{"cuarteto con 4 comodines", Quartet, []Card{jk(0), jk(1), jk(2), jk(3)}, 0, false},
		{"figuras mezcladas", Trio, []Card{nc(1, 0), nc(2, 0), nc(1, 1)}, 0, false},
		{"figuras mezcladas con comodín", Quartet, []Card{nc(1, 0), jk(0), nc(2, 0), nc(1, 1)}, 0, false},
		{"trío corto", Trio, []Card{nc(1, 0), nc(1, 1)}, 0, false},
		{"trío largo", Trio, []Card{nc(1, 0), nc(1, 1), nc(1, 2), nc(1, 3)}, 0, false},
		{"tipo desconocido", MeldKind(5), []Card{nc(1, 0)}, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fig, err := ValidateMeld(tc.kind, tc.cards)
			if tc.ok {
				if err != nil || fig != tc.fig {
					t.Fatalf("fig=%d err=%v", fig, err)
				}
				return
			}
			expectCode(t, err, CodeInvalidMeld)
		})
	}
}

func TestMeldsMatchSpec(t *testing.T) {
	r4, _ := Spec(4)
	if !meldsMatchSpec(r4, []int{3, 4}) || !meldsMatchSpec(r4, []int{4, 3}) {
		t.Fatal("el orden de las jugadas no debe importar")
	}
	for _, bad := range [][]int{{4}, {3}, {4, 4}, {3, 3}, {4, 3, 3}, {}} {
		if meldsMatchSpec(r4, bad) {
			t.Fatalf("%v no debería cumplir la ronda 4", bad)
		}
	}
	for n, want := range map[int]bool{2: false, 3: true, 4: true, 5: false} {
		if _, ok := kindForSize(n); ok != want {
			t.Fatalf("kindForSize(%d) = %v", n, ok)
		}
	}
}

func TestErrorHelpers(t *testing.T) {
	err := newErrorf(CodeNotYourTurn, "otro mensaje %d", 1)
	if !errors.Is(err, ErrNotYourTurn) || errors.Is(err, ErrGameOver) {
		t.Fatal("errors.Is debe comparar por código")
	}
	if !errors.Is(errStealOwnDiscard, ErrStealNotAllowed) {
		t.Fatal("los rechazos de robo deben coincidir con ErrStealNotAllowed")
	}
	if CodeOf(errors.New("x")) != "" || CodeOf(err) != CodeNotYourTurn {
		t.Fatal("CodeOf")
	}
	if err.Error() != "not_your_turn: otro mensaje 1" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if (&Error{}).Is(errors.New("x")) {
		t.Fatal("Is con error ajeno")
	}
}

// FuzzValidateMeld: cualquier jugada aceptada cumple las reglas de RF-27.
func FuzzValidateMeld(f *testing.F) {
	f.Add([]byte{0, 1, 2})
	f.Add([]byte{56, 57, 3})
	f.Add([]byte{56, 57, 58, 59})
	f.Add([]byte{0, 8, 16, 24})
	deck := NewDeck()
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 6 {
			raw = raw[:6]
		}
		cards := make([]Card, len(raw))
		for i, b := range raw {
			cards[i] = deck[int(b)%DeckSize]
		}
		for _, kind := range []MeldKind{Trio, Quartet} {
			fig, err := ValidateMeld(kind, cards)
			if err != nil {
				continue
			}
			jokers := 0
			for _, c := range cards {
				if c.Joker {
					jokers++
				} else if c.Figure != fig {
					t.Fatalf("figura mezclada aceptada: %v", cards)
				}
			}
			if len(cards) != kind.Size() || jokers > kind.MaxJokers() || fig == NoFigure {
				t.Fatalf("jugada inválida aceptada como %s: %v", kind, cards)
			}
		}
	})
}
