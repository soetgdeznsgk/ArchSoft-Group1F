package game

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// RF-33, RF-34, RF-22: la vista muestra la mano propia, conteos de los demás y botones.
func TestViewFor(t *testing.T) {
	g, _ := rig(t, stealTable())
	v, err := g.ViewFor("caro")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Hand) != 2 || v.Players[0].CardCount != 1 || v.Players[2].CardCount != 2 {
		t.Fatalf("mano/conteos: %+v", v)
	}
	if v.CurrentTurn != "beto" || !v.Players[1].IsTurn || v.Spec == nil || v.Spec.Number != 1 {
		t.Fatal("turno y ronda")
	}
	if v.TopDiscard == nil || v.TopDiscard.ID != nc(2, 1).ID || !v.DiscardAvailable || v.DiscardCount != 2 || v.HiddenCount != 2 {
		t.Fatal("bancos")
	}
	if !v.CanSteal || v.CanTakeDiscard || v.CanLayDown || v.CanDeposit || v.CanDiscard {
		t.Fatalf("botones de caro: %+v", v)
	}
	if v.TurnRemainingMs != DefaultTurnDuration.Milliseconds() {
		t.Fatalf("tiempo restante %d", v.TurnRemainingMs)
	}

	cases := map[PlayerID]bool{"ana": false, "beto": false, "dani": true} // RF-22
	for p, want := range cases {
		v, _ := g.ViewFor(p)
		if v.CanSteal != want {
			t.Fatalf("CanSteal(%s) = %v", p, v.CanSteal)
		}
	}
	vb, _ := g.ViewFor("beto")
	if !vb.CanTakeDiscard || !vb.CanLayDown || vb.CanDeposit || vb.CanDiscard {
		t.Fatalf("botones de beto: %+v", vb)
	}

	// Modificar la vista no altera la partida.
	v.Hand[0] = jk(0)
	if g.players[2].hand[0].Joker {
		t.Fatal("la vista debe ser una copia")
	}

	// La mano de los demás no se serializa en la vista.
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), `"id":`+itoa(int(nc(1, 0).ID))+`,"figure":1`) {
		t.Fatal("la vista filtra cartas de otro jugador")
	}

	ok(t)(g.Draw("beto", SourceHidden))
	vb, _ = g.ViewFor("beto")
	vc, _ := g.ViewFor("caro")
	if !vb.CanDiscard || vb.CanTakeDiscard || vc.CanSteal || vc.DiscardAvailable {
		t.Fatal("tras tomar, se cierra el robo y solo puede botar")
	}

	_, err = g.ViewFor("zoe")
	expectCode(t, err, CodeUnknownPlayer)
}

func TestViewBanksAreCopiesAndDepositFlag(t *testing.T) {
	g, _ := rig(t, table{turn: 0, hands: [4][]Card{{nc(1, 0), nc(1, 1), nc(1, 2), nc(2, 0)}, {nc(3, 0)}, {nc(4, 0)}, {nc(5, 0)}}, hidden: []Card{nc(6, 0)}})
	ok(t)(g.LayDown("ana", [][]CardID{ids(nc(1, 0), nc(1, 1), nc(1, 2))}))
	v, _ := g.ViewFor("ana")
	if !v.CanDeposit || v.CanLayDown || !v.LaidDown || len(v.Banks) != 1 || !v.Players[0].LaidDown {
		t.Fatalf("vista tras bajar: %+v", v)
	}
	v.Banks[0].Cards[0] = jk(0)
	if g.banks[0].Cards[0].Joker {
		t.Fatal("los bancos de la vista deben ser copias")
	}
}

func TestViewBeforeStartAndAfterEnd(t *testing.T) {
	g, _ := newTestGame(t, 1)
	v, err := g.ViewFor("ana")
	if err != nil || v.Spec != nil || v.Phase != PhaseWaiting || v.Hand == nil || v.CurrentTurn != "" {
		t.Fatalf("vista inicial: %+v", v)
	}
	ok(t)(g.Start())
	g.hidden = nil
	ok(t)(g.Draw(g.CurrentPlayer(), SourceHidden))
	v, _ = g.ViewFor("ana")
	if v.CanSteal || v.CurrentTurn != "" || len(v.Results) != 1 || v.TurnRemainingMs != 0 {
		t.Fatalf("vista de ronda terminada: %+v", v)
	}
}

// Apply despacha las acciones serializadas por la capa de red.
func TestApplyDispatch(t *testing.T) {
	g, _ := rig(t, table{
		turn:   0,
		hands:  [4][]Card{{nc(1, 0), nc(1, 1), nc(1, 2), nc(1, 3), nc(2, 0)}, {nc(3, 0)}, {nc(4, 0)}, {nc(5, 0)}},
		hidden: []Card{nc(6, 0), nc(6, 1), nc(6, 2)},
	})
	var a Action
	payloads := []string{
		`{"type":"lay_down","player":"ana","melds":[[0,1,2]]}`,
		`{"type":"deposit","player":"ana","card":3,"bank":0}`,
		`{"type":"draw","player":"ana","source":"hidden"}`,
		`{"type":"discard","player":"ana","card":8}`,
		`{"type":"steal","player":"caro"}`,
	}
	for _, p := range payloads {
		a = Action{}
		if err := json.Unmarshal([]byte(p), &a); err != nil {
			t.Fatal(err)
		}
		ok(t)(g.Apply(a))
	}
	if !holds(g, 2, nc(2, 0).ID) || len(g.banks[0].Cards) != 4 {
		t.Fatal("acciones no aplicadas")
	}
	_, err := g.Apply(Action{Type: "volar", Player: "ana"})
	expectCode(t, err, CodeInvalidAction)
}

// Los eventos se serializan sin campos vacíos.
func TestEventJSON(t *testing.T) {
	g, _ := startedGame(t, 1)
	evs := ok(t)(g.Draw(g.CurrentPlayer(), SourceHidden))
	raw, err := json.Marshal(evs[0])
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `"type":"card_drawn"`) || strings.Contains(s, `"card"`) || strings.Contains(s, `"banks"`) {
		t.Fatalf("JSON: %s", s)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// RNF-03, RF-25: ante robos simultáneos y la toma del jugador en turno, la
// carta disputada se asigna a una única acción, la primera procesada.
func TestConcurrentStealOnlyOneWins(t *testing.T) {
	const iterations = 300
	contested := nc(2, 1).ID
	for it := 0; it < iterations; it++ {
		g, _ := rig(t, stealTable())
		m := &Match{g: g}
		actions := []Action{
			{Type: ActSteal, Player: "caro"},
			{Type: ActSteal, Player: "dani"},
			{Type: ActDraw, Player: "beto", Source: SourceDiscard},
		}
		var wins atomic.Int32
		var wg sync.WaitGroup
		gate := make(chan struct{})
		for _, a := range actions {
			wg.Add(1)
			go func(a Action) {
				defer wg.Done()
				<-gate
				if _, err := m.Apply(a); err == nil {
					wins.Add(1)
				} else if c := CodeOf(err); c != CodeStealNotAllowed && c != CodeDiscardUnavailable {
					t.Errorf("rechazo inesperado: %v", err)
				}
			}(a)
		}
		close(gate)
		wg.Wait()
		if wins.Load() != 1 {
			t.Fatalf("iteración %d: %d acciones obtuvieron la carta", it, wins.Load())
		}
		owners := 0
		for seat := range g.players {
			if holds(g, seat, contested) {
				owners++
			}
		}
		if owners != 1 {
			t.Fatalf("iteración %d: la carta está en %d manos", it, owners)
		}
	}
}

func TestMatchWrappers(t *testing.T) {
	_, err := NewMatch([]PlayerID{"a"}, Config{})
	expectCode(t, err, CodeInvalidPlayers)
	m, err := NewMatch(testPlayers, Config{})
	if err != nil {
		t.Fatal(err)
	}
	ok(t)(m.Start())
	if m.Round() != 1 || m.Phase() != PhaseAwaitingDraw || m.CurrentPlayer() == "" || len(m.Players()) != 4 {
		t.Fatal("estado del Match")
	}
	if _, ok := m.TurnDeadline(); !ok {
		t.Fatal("plazo")
	}
	if m.Tick() != nil {
		t.Fatal("Tick sin vencer")
	}
	ok(t)(m.SetConnected("ana", true))
	if _, err := m.ViewFor("ana"); err != nil {
		t.Fatal(err)
	}
	_, err = m.StartNextRound()
	expectCode(t, err, CodeRoundStillInPlay)
	if len(m.Results()) != 0 || m.Final() != nil {
		t.Fatal("sin resultados aún")
	}
}
