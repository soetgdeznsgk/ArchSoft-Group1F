package game

import (
	"testing"
)

// RF-15: el turno inicial solo toma del banco de ocultas y bota.
func TestFirstTurnIsExchangeOnly(t *testing.T) {
	g, _ := startedGame(t, 5)
	cur := g.CurrentPlayer()
	s := g.turn
	other := g.players[(s+1)%NumPlayers].id

	_, err := g.Draw(cur, SourceDiscard)
	expectCode(t, err, CodeDiscardUnavailable)
	_, err = g.LayDown(cur, [][]CardID{handIDs(g, s)})
	expectCode(t, err, CodeFirstTurnExchangeOnly)
	_, err = g.Draw(other, SourceHidden)
	expectCode(t, err, CodeNotYourTurn)

	hidden := len(g.hidden)
	evs := ok(t)(g.Draw(cur, SourceHidden))
	if len(g.hidden) != hidden-1 || len(g.players[s].hand) != 4 || g.Phase() != PhaseAwaitingDiscard {
		t.Fatal("la toma del banco de ocultas no se aplicó")
	}
	if e := findEvent(evs, EvCardDrawn); e == nil || e.Card != nil || e.Source != SourceHidden {
		t.Fatalf("el evento público no debe revelar la carta oculta: %+v", e)
	}
	card := g.players[s].hand[0]
	evs = ok(t)(g.Discard(cur, card.ID))
	if g.CurrentPlayer() != other || g.firstTurn || !g.discardAvailable || g.lastDiscarder != s {
		t.Fatal("el turno debe pasar a la derecha y abrir el banco de vistas")
	}
	if e := findEvent(evs, EvCardDiscarded); e == nil || e.Card.ID != card.ID {
		t.Fatalf("evento de carta botada: %+v", e)
	}
	if e := findEvent(evs, EvTurnStarted); e == nil || e.Player != other {
		t.Fatal("falta evento de cambio de turno")
	}
}

// RF-16: los turnos avanzan hacia la derecha (asiento + 1).
func TestTurnsAdvanceToTheRight(t *testing.T) {
	g, _ := startedGame(t, 9)
	start := g.turn
	for k := 0; k < 2*NumPlayers; k++ {
		want := (start + k) % NumPlayers
		if g.turn != want {
			t.Fatalf("turno %d: asiento %d, se esperaba %d", k, g.turn, want)
		}
		p := g.CurrentPlayer()
		ok(t)(g.Draw(p, SourceHidden))
		ok(t)(g.Discard(p, g.players[g.turn].hand[0].ID))
	}
	if err := g.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
}

// RF-18, RF-19 y validaciones de RF-10 del intercambio.
func TestExchangeValidation(t *testing.T) {
	g, _ := rig(t, table{
		turn:   1,
		hands:  [4][]Card{{nc(1, 0)}, {nc(2, 0), nc(2, 1)}, {nc(3, 0)}, {nc(4, 0)}},
		hidden: []Card{nc(5, 0), nc(5, 1)},
	})
	_, err := g.Discard("beto", nc(2, 0).ID)
	expectCode(t, err, CodeMustDrawFirst)
	_, err = g.Draw("beto", Source("mesa"))
	expectCode(t, err, CodeInvalidSource)
	_, err = g.Draw("beto", SourceDiscard)
	expectCode(t, err, CodeDiscardUnavailable)

	ok(t)(g.Draw("beto", SourceHidden))
	if !holds(g, 1, nc(5, 1).ID) {
		t.Fatal("debe tomar la carta superior del banco de ocultas")
	}
	_, err = g.Draw("beto", SourceHidden)
	expectCode(t, err, CodeAlreadyDrew)
	_, err = g.Discard("ana", nc(1, 0).ID)
	expectCode(t, err, CodeNotYourTurn)
	_, err = g.Discard("beto", nc(1, 0).ID)
	expectCode(t, err, CodeCardNotInHand)
	ok(t)(g.Discard("beto", nc(5, 1).ID)) // puede botar la carta recién tomada
	if g.discard[len(g.discard)-1].ID != nc(5, 1).ID || g.CurrentPlayer() != "caro" {
		t.Fatal("intercambio no aplicado")
	}
}

// RF-18: tomar la última carta botada del banco de vistas.
func TestTakeFromDiscard(t *testing.T) {
	g, _ := rig(t, table{
		turn:          1,
		hands:         [4][]Card{{nc(1, 0)}, {nc(2, 0)}, {nc(3, 0)}, {nc(4, 0)}},
		hidden:        []Card{nc(5, 0)},
		discard:       []Card{nc(6, 0), nc(2, 1)},
		lastDiscarder: 0,
	})
	evs := ok(t)(g.Draw("beto", SourceDiscard))
	if !holds(g, 1, nc(2, 1).ID) || len(g.discard) != 1 || g.discardAvailable {
		t.Fatal("debe tomar solo la última botada")
	}
	if e := findEvent(evs, EvCardDrawn); e == nil || e.Card == nil || e.Card.ID != nc(2, 1).ID || e.Source != SourceDiscard {
		t.Fatalf("evento: %+v", e)
	}
	_, err := g.Steal("caro")
	expectCode(t, err, CodeStealNotAllowed)
}
