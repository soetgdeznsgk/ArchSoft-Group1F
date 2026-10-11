package game

import (
	"slices"
	"testing"
	"time"
)

func TestNewValidatesPlayers(t *testing.T) {
	cases := [][]PlayerID{
		{"a", "b", "c"},
		{"a", "b", "c", "d", "e"},
		{"a", "b", "c", "a"},
		{"a", "", "c", "d"},
	}
	for _, players := range cases {
		_, err := New(players, Config{})
		expectCode(t, err, CodeInvalidPlayers)
	}
	g, err := New(testPlayers, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if g.turnDuration != DefaultTurnDuration || g.rng == nil || g.clock == nil {
		t.Fatal("valores por defecto no aplicados")
	}
	if g.Phase() != PhaseWaiting || g.Round() != 0 || g.CurrentPlayer() != "" {
		t.Fatal("estado inicial incorrecto")
	}
	if !slices.Equal(g.Players(), testPlayers) {
		t.Fatal("orden de asientos")
	}
	if now := g.clock.Now(); time.Since(now) > time.Minute {
		t.Fatal("reloj del sistema")
	}
}

// RF-12, RF-14, historia "Inicio de la partida y reparto de cartas".
func TestStartDealsRoundOne(t *testing.T) {
	g, _ := newTestGame(t, 7)
	evs := ok(t)(g.Start())
	if g.Round() != 1 || g.Phase() != PhaseAwaitingDraw || !g.firstTurn {
		t.Fatalf("ronda=%d fase=%s", g.Round(), g.Phase())
	}
	for _, p := range g.players {
		if len(p.hand) != 3 {
			t.Fatalf("%s recibió %d cartas", p.id, len(p.hand))
		}
	}
	if len(g.hidden) != 60-12 || len(g.discard) != 0 || g.discardAvailable {
		t.Fatalf("ocultas=%d vistas=%d", len(g.hidden), len(g.discard))
	}
	rs := findEvent(evs, EvRoundStarted)
	if rs == nil || rs.Spec == nil || rs.Spec.Describe() != "un trío" || rs.Player != g.CurrentPlayer() {
		t.Fatalf("evento de inicio: %+v", rs)
	}
	if !hasEvent(evs, EvTurnStarted) || evs[0].Seq != 1 || evs[1].Seq != 2 {
		t.Fatal("eventos de inicio incompletos o sin secuencia")
	}
	if err := g.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
	_, err := g.Start()
	expectCode(t, err, CodeGameAlreadyStarted)
}

// RF-12, RF-13: tamaño del reparto en cada ronda y conservación del mazo.
func TestDealSizesPerRound(t *testing.T) {
	g, _ := startedGame(t, 3)
	for r := 1; r <= TotalRounds; r++ {
		g.round = r
		g.startRound(r % NumPlayers)
		spec, _ := Spec(r)
		for _, p := range g.players {
			if len(p.hand) != spec.CardsPerPlayer || p.laidDown {
				t.Fatalf("ronda %d: %s tiene %d cartas", r, p.id, len(p.hand))
			}
		}
		if len(g.hidden) != DeckSize-NumPlayers*spec.CardsPerPlayer {
			t.Fatalf("ronda %d: %d ocultas", r, len(g.hidden))
		}
		if err := g.CheckInvariants(); err != nil {
			t.Fatalf("ronda %d: %v", r, err)
		}
	}
}

// RF-14: el jugador inicial de la primera ronda es aleatorio.
func TestFirstStarterIsRandom(t *testing.T) {
	seen := map[int]bool{}
	for seed := uint64(0); seed < 64; seed++ {
		g, _ := startedGame(t, seed)
		seen[g.turn] = true
	}
	if len(seen) != NumPlayers {
		t.Fatalf("solo iniciaron los asientos %v", seen)
	}
}

func TestSameSeedSameDeal(t *testing.T) {
	a, _ := startedGame(t, 11)
	b, _ := startedGame(t, 11)
	c, _ := startedGame(t, 12)
	if !slices.Equal(handIDs(a, 0), handIDs(b, 0)) || a.turn != b.turn {
		t.Fatal("misma semilla debe producir el mismo reparto")
	}
	if slices.Equal(handIDs(a, 0), handIDs(c, 0)) && slices.Equal(handIDs(a, 1), handIDs(c, 1)) {
		t.Fatal("semillas distintas produjeron el mismo reparto")
	}
}

// RF-10: acciones fuera del momento permitido.
func TestActionsOutsideRound(t *testing.T) {
	g, _ := newTestGame(t, 1)
	_, err := g.Draw("ana", SourceHidden)
	expectCode(t, err, CodeGameNotStarted)
	_, err = g.Steal("ana")
	expectCode(t, err, CodeGameNotStarted)
	_, err = g.StartNextRound()
	expectCode(t, err, CodeGameNotStarted)

	g, _ = rig(t, table{hidden: nil, hands: [4][]Card{{nc(1, 0)}, {nc(2, 0)}, {nc(3, 0)}, {nc(4, 0)}}})
	_, err = g.Draw("zoe", SourceHidden)
	expectCode(t, err, CodeUnknownPlayer)
	_, err = g.Steal("zoe")
	expectCode(t, err, CodeUnknownPlayer)
	_, err = g.StartNextRound()
	expectCode(t, err, CodeRoundStillInPlay)

	ok(t)(g.Draw("ana", SourceHidden)) // banco de ocultas vacío: termina la ronda
	if g.Phase() != PhaseRoundOver {
		t.Fatalf("fase %s", g.Phase())
	}
	_, err = g.Discard("ana", nc(1, 0).ID)
	expectCode(t, err, CodeRoundNotInPlay)
	_, err = g.Steal("beto")
	expectCode(t, err, CodeRoundNotInPlay)
	_, err = g.Deposit("ana", 0, 0)
	expectCode(t, err, CodeRoundNotInPlay)
}
