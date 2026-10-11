package game

import (
	"slices"
	"testing"
)

// RF-31, RF-32: sin cartas ocultas, la ronda termina sin ganador y se cuentan todas las manos.
func TestNoWinnerWhenHiddenExhausted(t *testing.T) {
	g, _ := rig(t, table{
		turn: 2,
		hands: [4][]Card{
			{nc(1, 0), jk(0)},
			{nc(2, 0)},
			{nc(3, 0), nc(3, 1), nc(3, 2), nc(3, 3)},
			{jk(1)},
		},
	})
	evs := ok(t)(g.Draw("caro", SourceHidden))
	e := findEvent(evs, EvRoundEnded)
	if e == nil || e.Result.Winner != "" || e.Result.Reason != EndHiddenExhausted || e.Reason != string(EndHiddenExhausted) {
		t.Fatalf("resultado: %+v", e)
	}
	want := map[PlayerID]int{"ana": 6, "beto": 1, "caro": 4, "dani": 5}
	for p, pts := range want {
		if e.Result.Points[p] != pts {
			t.Fatalf("%s: %d puntos, se esperaban %d", p, e.Result.Points[p], pts)
		}
	}
	for _, p := range g.players {
		if p.roundsWon != 0 {
			t.Fatal("nadie gana la ronda")
		}
	}
}

// RF-14: la siguiente ronda la inicia el ganador anterior.
func TestNextRoundStartedByWinner(t *testing.T) {
	g, _ := rig(t, table{
		turn:   3,
		hands:  [4][]Card{{nc(1, 0)}, {nc(2, 0)}, {nc(3, 0)}, {nc(4, 0), nc(4, 1), nc(4, 2)}},
		hidden: []Card{nc(5, 0)},
	})
	ok(t)(g.LayDown("dani", [][]CardID{ids(nc(4, 0), nc(4, 1), nc(4, 2))}))
	evs := ok(t)(g.StartNextRound())
	if g.Round() != 2 || g.CurrentPlayer() != "dani" || !g.firstTurn {
		t.Fatalf("ronda=%d inicia=%s", g.Round(), g.CurrentPlayer())
	}
	rs := findEvent(evs, EvRoundStarted)
	if rs == nil || rs.Round != 2 || rs.Spec.Describe() != "un cuarteto" {
		t.Fatalf("evento: %+v", rs)
	}
	if len(g.banks) != 0 || len(g.discard) != 0 || g.lastDiscarder != -1 {
		t.Fatal("los bancos deben reiniciarse")
	}
	for _, p := range g.players {
		if len(p.hand) != 4 || p.laidDown {
			t.Fatalf("%s: %d cartas, bajó=%v", p.id, len(p.hand), p.laidDown)
		}
	}
	if err := g.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
}

// RF-14: sin ganador, la siguiente ronda la inicia un jugador al azar.
func TestNextRoundRandomStarterWithoutWinner(t *testing.T) {
	seen := map[int]bool{}
	for seed := uint64(0); seed < 64; seed++ {
		g, _ := startedGame(t, seed)
		g.hidden = nil
		ok(t)(g.Draw(g.CurrentPlayer(), SourceHidden))
		ok(t)(g.StartNextRound())
		seen[g.turn] = true
	}
	if len(seen) < 3 {
		t.Fatalf("inicio poco aleatorio: %v", seen)
	}
}

// RF-35: partida completa de 5 rondas y cierre.
func TestFullGameEndsAfterFiveRounds(t *testing.T) {
	g, _ := startedGame(t, 21)
	for r := 1; r <= TotalRounds; r++ {
		if g.Round() != r {
			t.Fatalf("ronda %d, se esperaba %d", g.Round(), r)
		}
		g.hidden = nil
		evs := ok(t)(g.Draw(g.CurrentPlayer(), SourceHidden))
		if r < TotalRounds {
			if g.Phase() != PhaseRoundOver || hasEvent(evs, EvGameEnded) {
				t.Fatalf("ronda %d: fase %s", r, g.Phase())
			}
			ok(t)(g.StartNextRound())
			continue
		}
		ge := findEvent(evs, EvGameEnded)
		if g.Phase() != PhaseGameOver || ge == nil || ge.Final == nil || len(ge.Final.Winners) == 0 {
			t.Fatalf("fin de partida: %+v", ge)
		}
	}
	if len(g.Results()) != TotalRounds || g.Final() == nil {
		t.Fatal("resultados incompletos")
	}
	total := 0
	for _, r := range g.Results() {
		total += r.Points["ana"]
	}
	for _, s := range g.Final().Standings {
		if s.Player == "ana" && s.Points != total {
			t.Fatalf("RF-35: puntos de ana %d, suma de rondas %d", s.Points, total)
		}
	}
	_, err := g.StartNextRound()
	expectCode(t, err, CodeGameOver)
	_, err = g.Draw("ana", SourceHidden)
	expectCode(t, err, CodeGameOver)
	if g.CurrentPlayer() != "" {
		t.Fatal("sin turno al terminar")
	}
	if evs := g.Tick(); evs != nil {
		t.Fatal("Tick no debe actuar al terminar")
	}
}

// RF-35, propuesta 14.1: orden final y desempates.
func TestFinalStandingsTiebreaks(t *testing.T) {
	type stat struct{ total, won, last int }
	cases := []struct {
		name    string
		stats   [4]stat
		winners []PlayerID
		ranks   []int // en orden de asiento
	}{
		{"menos puntos gana", [4]stat{{10, 1, 0}, {5, 0, 3}, {20, 2, 0}, {7, 2, 1}}, []PlayerID{"beto"}, []int{3, 1, 4, 2}},
		{"empate por rondas ganadas", [4]stat{{5, 1, 0}, {5, 2, 4}, {9, 2, 0}, {12, 0, 1}}, []PlayerID{"beto"}, []int{2, 1, 3, 4}},
		{"empate por última ronda", [4]stat{{5, 1, 2}, {5, 1, 1}, {9, 2, 0}, {12, 0, 1}}, []PlayerID{"beto"}, []int{2, 1, 3, 4}},
		{"empate persistente", [4]stat{{5, 1, 2}, {5, 1, 2}, {9, 2, 0}, {9, 2, 0}}, []PlayerID{"ana", "beto"}, []int{1, 1, 3, 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := newTestGame(t, 1)
			for i, s := range tc.stats {
				g.players[i].total, g.players[i].roundsWon, g.players[i].lastRound = s.total, s.won, s.last
			}
			f := g.computeFinal()
			if !slices.Equal(f.Winners, tc.winners) {
				t.Fatalf("ganadores %v, se esperaban %v", f.Winners, tc.winners)
			}
			for _, s := range f.Standings {
				if want := tc.ranks[g.seat[s.Player]]; s.Rank != want {
					t.Fatalf("%s: posición %d, se esperaba %d", s.Player, s.Rank, want)
				}
			}
			for i := 1; i < len(f.Standings); i++ {
				if f.Standings[i].Rank < f.Standings[i-1].Rank {
					t.Fatal("posiciones desordenadas")
				}
			}
		})
	}
}

// Las copias devueltas no comparten memoria con el estado interno.
func TestResultsAreCopies(t *testing.T) {
	g, _ := startedGame(t, 2)
	g.hidden = nil
	ok(t)(g.Draw(g.CurrentPlayer(), SourceHidden))
	r := g.Results()
	r[0].Points["ana"] = 999
	if g.results[0].Points["ana"] == 999 {
		t.Fatal("Results debe devolver copias")
	}
	if g.Final() != nil {
		t.Fatal("sin resultado final antes de la ronda 5")
	}
}
