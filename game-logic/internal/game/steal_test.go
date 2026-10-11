package game

import (
	"errors"
	"testing"
)

// Mesa típica de robo: ana (0) botó F2#1 y ahora es el turno de beto (1).
func stealTable() table {
	return table{
		turn:          1,
		hands:         [4][]Card{{nc(1, 0)}, {nc(3, 0)}, {nc(2, 0), nc(2, 2)}, {nc(4, 0)}},
		hidden:        []Card{nc(6, 0), nc(5, 0)},
		discard:       []Card{nc(7, 0), nc(2, 1)},
		lastDiscarder: 0,
	}
}

// RF-21, RF-23, RF-24.
func TestStealGivesCardPenaltyAndKeepsTurn(t *testing.T) {
	g, _ := rig(t, stealTable())
	evs := ok(t)(g.Steal("caro"))
	if !holds(g, 2, nc(2, 1).ID) || !holds(g, 2, nc(5, 0).ID) || len(g.players[2].hand) != 4 {
		t.Fatalf("mano del ladrón: %v", handIDs(g, 2))
	}
	if len(g.hidden) != 1 || len(g.discard) != 1 || g.discardAvailable {
		t.Fatal("bancos tras el robo")
	}
	if e := findEvent(evs, EvCardStolen); e == nil || e.Player != "caro" || e.Card.ID != nc(2, 1).ID {
		t.Fatalf("evento de robo: %+v", e)
	}
	if e := findEvent(evs, EvPenaltyDrawn); e == nil || e.Card != nil {
		t.Fatal("la penalización no debe revelar la carta")
	}
	if g.CurrentPlayer() != "beto" || g.Phase() != PhaseAwaitingDraw {
		t.Fatal("RF-24: el jugador afectado conserva el turno")
	}
	_, err := g.Draw("beto", SourceDiscard)
	expectCode(t, err, CodeDiscardUnavailable)
	ok(t)(g.Draw("beto", SourceHidden))
}

// RF-22: restricciones del robo.
func TestStealRestrictions(t *testing.T) {
	t.Run("quien botó la carta", func(t *testing.T) {
		g, _ := rig(t, stealTable())
		_, err := g.Steal("ana")
		expectCode(t, err, CodeStealNotAllowed)
		var e *Error
		if !errors.As(err, &e) || e.Message != errStealOwnDiscard.Message {
			t.Fatalf("mensaje: %v", err)
		}
	})
	t.Run("jugador en turno", func(t *testing.T) {
		g, _ := rig(t, stealTable())
		_, err := g.Steal("beto")
		expectCode(t, err, CodeStealNotAllowed)
	})
	t.Run("banco de vistas vacío", func(t *testing.T) {
		tb := stealTable()
		tb.discard = nil
		g, _ := rig(t, tb)
		_, err := g.Steal("caro")
		expectCode(t, err, CodeStealNotAllowed)
	})
	t.Run("después de que el jugador en turno tomó", func(t *testing.T) {
		g, _ := rig(t, stealTable())
		ok(t)(g.Draw("beto", SourceHidden))
		_, err := g.Steal("caro")
		expectCode(t, err, CodeStealNotAllowed)
	})
	t.Run("segundo robo sobre la misma carta", func(t *testing.T) {
		g, _ := rig(t, stealTable())
		ok(t)(g.Steal("caro"))
		_, err := g.Steal("dani")
		expectCode(t, err, CodeStealNotAllowed)
		if holds(g, 3, nc(2, 1).ID) {
			t.Fatal("la carta quedó asignada dos veces")
		}
	})
	t.Run("quien ya bajó sí puede robar", func(t *testing.T) {
		g, _ := rig(t, stealTable())
		g.players[3].laidDown = true
		ok(t)(g.Steal("dani"))
	})
}

// RF-31: la penalización con el banco de ocultas vacío cierra la ronda sin ganador.
func TestStealWithEmptyHiddenEndsRound(t *testing.T) {
	tb := stealTable()
	tb.hidden = nil
	g, _ := rig(t, tb)
	evs := ok(t)(g.Steal("caro"))
	if g.Phase() != PhaseRoundOver {
		t.Fatalf("fase %s", g.Phase())
	}
	e := findEvent(evs, EvRoundEnded)
	if e == nil || e.Result.Winner != "" || e.Result.Reason != EndHiddenExhausted {
		t.Fatalf("resultado: %+v", e)
	}
	// La carta robada cuenta en la mano: 3 normales.
	if e.Result.Points["caro"] != 3 {
		t.Fatalf("puntos de caro: %d", e.Result.Points["caro"])
	}
}

// Un robo seguido de un nuevo botado vuelve a abrir el robo.
func TestStealReopensAfterNextDiscard(t *testing.T) {
	g, _ := rig(t, stealTable())
	ok(t)(g.Steal("caro"))
	ok(t)(g.Draw("beto", SourceHidden))
	ok(t)(g.Discard("beto", nc(3, 0).ID))
	if !g.canSteal(0) || !g.canSteal(3) || g.canSteal(1) || g.canSteal(2) {
		t.Fatal("tras botar beto, pueden robar ana y dani (caro tiene el turno)")
	}
}
