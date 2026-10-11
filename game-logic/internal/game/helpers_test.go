package game

import (
	"math/rand/v2"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

var testPlayers = []PlayerID{"ana", "beto", "caro", "dani"}

// nc devuelve la k-ésima carta (0..7) de la figura f.
func nc(f Figure, k int) Card {
	return Card{ID: CardID(int(f-1)*CardsPerFigure + k), Figure: f}
}

// jk devuelve el k-ésimo comodín (0..3).
func jk(k int) Card { return Card{ID: CardID(NumFigures*CardsPerFigure + k), Joker: true} }

func ids(cards ...Card) []CardID {
	out := make([]CardID, len(cards))
	for i, c := range cards {
		out[i] = c.ID
	}
	return out
}

func newTestGame(t *testing.T, seed uint64) (*Game, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	g, err := New(testPlayers, Config{Rand: rand.New(rand.NewPCG(seed, seed)), Clock: clk})
	if err != nil {
		t.Fatal(err)
	}
	return g, clk
}

func startedGame(t *testing.T, seed uint64) (*Game, *fakeClock) {
	t.Helper()
	g, clk := newTestGame(t, seed)
	if _, err := g.Start(); err != nil {
		t.Fatal(err)
	}
	return g, clk
}

// table describe una mesa controlada para escenarios deterministas.
type table struct {
	round         int // 1 por defecto
	turn          int
	hands         [NumPlayers][]Card
	hidden        []Card // la próxima carta a tomar es la última
	discard       []Card // la última botada es la última
	lastDiscarder int    // solo se usa si discard no está vacío
	firstTurn     bool
}

// rig prepara una partida iniciada con la mesa indicada, en fase de tomar carta.
func rig(t *testing.T, tb table) (*Game, *fakeClock) {
	t.Helper()
	g, clk := startedGame(t, 1)
	g.pending = nil
	if tb.round == 0 {
		tb.round = 1
	}
	g.round = tb.round
	for i, p := range g.players {
		p.hand = append([]Card(nil), tb.hands[i]...)
		p.laidDown = false
	}
	g.hidden = append([]Card(nil), tb.hidden...)
	g.discard = append([]Card(nil), tb.discard...)
	g.discardAvailable = len(tb.discard) > 0
	g.lastDiscarder = -1
	if len(tb.discard) > 0 {
		g.lastDiscarder = tb.lastDiscarder
	}
	g.turn = tb.turn
	g.phase = PhaseAwaitingDraw
	g.firstTurn = tb.firstTurn
	g.banks = nil
	g.turnStarted = clk.Now()
	return g, clk
}

// ok(t)(g.Draw(...)) falla la prueba si la acción devuelve error.
func ok(t *testing.T) func([]Event, error) []Event {
	t.Helper()
	return func(evs []Event, err error) []Event {
		t.Helper()
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		return evs
	}
}

func expectCode(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("se esperaba error %q y no hubo error", code)
	}
	if got := CodeOf(err); got != code {
		t.Fatalf("se esperaba código %q, se obtuvo %q (%v)", code, got, err)
	}
}

func hasEvent(evs []Event, typ EventType) bool {
	return findEvent(evs, typ) != nil
}

func findEvent(evs []Event, typ EventType) *Event {
	for i := range evs {
		if evs[i].Type == typ {
			return &evs[i]
		}
	}
	return nil
}

func handIDs(g *Game, seat int) []CardID { return ids(g.players[seat].hand...) }

func holds(g *Game, seat int, id CardID) bool {
	return indexOfCard(g.players[seat].hand, id) >= 0
}
