package game

import (
	"testing"
)

// RF-26, RF-30: con exactamente las cartas de la jugada, al bajar gana la ronda.
func TestLayDownExactHandWinsRound(t *testing.T) {
	g, _ := rig(t, table{
		turn: 0,
		hands: [4][]Card{
			{nc(1, 0), nc(1, 1), jk(0)},
			{nc(2, 0), nc(3, 0), jk(1)}, // 1 + 1 + 5
			{nc(4, 0)},
			{jk(2), jk(3)},
		},
		hidden: []Card{nc(5, 0)},
	})
	evs := ok(t)(g.LayDown("ana", [][]CardID{ids(nc(1, 0), jk(0), nc(1, 1))}))
	if e := findEvent(evs, EvMeldsLaidDown); e == nil || len(e.Banks) != 1 || e.Banks[0].Figure != 1 || e.Banks[0].Owner != "ana" {
		t.Fatalf("evento de bajada: %+v", e)
	}
	e := findEvent(evs, EvRoundEnded)
	if e == nil || e.Result.Winner != "ana" || e.Result.Reason != EndPlayerWentOut {
		t.Fatalf("resultado: %+v", e)
	}
	want := map[PlayerID]int{"ana": 0, "beto": 7, "caro": 1, "dani": 10}
	for p, pts := range want {
		if e.Result.Points[p] != pts {
			t.Fatalf("RF-32: %s tiene %d puntos, se esperaban %d", p, e.Result.Points[p], pts)
		}
	}
	if g.players[0].roundsWon != 1 || g.players[3].total != 10 || g.Phase() != PhaseRoundOver {
		t.Fatal("estadísticas acumuladas")
	}
}

// RF-26: hay que bajar todas las jugadas de la ronda a la vez, en cualquier orden.
func TestLayDownRequiresAllMelds(t *testing.T) {
	quartet := []Card{nc(1, 0), nc(1, 1), nc(1, 2), nc(1, 3)}
	trio := []Card{nc(2, 0), nc(2, 1), jk(0)}
	hand := append(append([]Card{}, quartet...), append(trio, nc(3, 0))...)
	g, _ := rig(t, table{round: 4, turn: 0, hands: [4][]Card{hand, {nc(4, 0)}, {nc(5, 0)}, {nc(6, 0)}}, hidden: []Card{nc(7, 0)}})

	_, err := g.LayDown("ana", [][]CardID{ids(quartet...)})
	expectCode(t, err, CodeMeldsMismatch)
	_, err = g.LayDown("ana", [][]CardID{ids(trio...), ids(trio...)})
	expectCode(t, err, CodeMeldsMismatch)

	ok(t)(g.LayDown("ana", [][]CardID{ids(trio...), ids(quartet...)}))
	if len(g.banks) != 2 || g.banks[0].Figure != 2 || g.banks[1].Figure != 1 || g.banks[1].ID != 1 {
		t.Fatalf("bancos: %+v %+v", g.banks[0], g.banks[1])
	}
	if !g.players[0].laidDown || len(g.players[0].hand) != 1 || g.Phase() != PhaseAwaitingDraw {
		t.Fatal("al sobrar cartas el turno continúa antes de tomar")
	}
	_, err = g.LayDown("ana", [][]CardID{ids(trio...), ids(quartet...)})
	expectCode(t, err, CodeAlreadyLaidDown)
}

// RF-26, RF-27: rechazos con motivo.
func TestLayDownRejections(t *testing.T) {
	hand := []Card{nc(1, 0), nc(1, 1), nc(2, 0), jk(0), jk(1), jk(2)}
	base := table{turn: 0, hands: [4][]Card{hand, {nc(4, 0)}, {nc(5, 0)}, {nc(6, 0)}}, hidden: []Card{nc(7, 0)}}
	cases := []struct {
		name   string
		player PlayerID
		melds  [][]CardID
		code   ErrorCode
	}{
		{"no es su turno", "beto", [][]CardID{ids(nc(4, 0), nc(4, 1), nc(4, 2))}, CodeNotYourTurn},
		{"carta ajena", "ana", [][]CardID{ids(nc(1, 0), nc(1, 1), nc(1, 2))}, CodeCardNotInHand},
		{"carta repetida", "ana", [][]CardID{ids(nc(1, 0), nc(1, 0), nc(1, 1))}, CodeDuplicateCard},
		{"figuras mezcladas", "ana", [][]CardID{ids(nc(1, 0), nc(2, 0), nc(1, 1))}, CodeInvalidMeld},
		{"solo comodines", "ana", [][]CardID{ids(jk(0), jk(1), jk(2))}, CodeInvalidMeld},
		{"tamaño incorrecto", "ana", [][]CardID{ids(nc(1, 0), nc(1, 1), jk(0), jk(1))}, CodeMeldsMismatch},
		{"sin jugadas", "ana", nil, CodeMeldsMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := rig(t, base)
			_, err := g.LayDown(tc.player, tc.melds)
			expectCode(t, err, tc.code)
			if len(g.players[0].hand) != len(hand) || len(g.banks) != 0 || g.players[0].laidDown {
				t.Fatal("una acción rechazada no debe modificar el estado")
			}
		})
	}
	t.Run("después de tomar carta", func(t *testing.T) {
		g, _ := rig(t, base)
		ok(t)(g.Draw("ana", SourceHidden))
		_, err := g.LayDown("ana", [][]CardID{ids(nc(1, 0), nc(1, 1), jk(0))})
		expectCode(t, err, CodeAlreadyDrew)
	})
}

// RF-27, reglamento §10: un jugador con los 4 comodines puede usarlos todos respetando los límites.
func TestLayDownWithAllJokers(t *testing.T) {
	hand := []Card{nc(1, 0), jk(0), jk(1), jk(2), nc(2, 0), nc(2, 1), nc(2, 2), jk(3)}
	g, _ := rig(t, table{round: 5, turn: 2, hands: [4][]Card{{nc(3, 0)}, {nc(4, 0)}, hand, {nc(5, 0)}}, hidden: []Card{nc(6, 0)}})
	evs := ok(t)(g.LayDown("caro", [][]CardID{
		ids(nc(1, 0), jk(0), jk(1), jk(2)),
		ids(nc(2, 0), jk(3), nc(2, 1), nc(2, 2)),
	}))
	if e := findEvent(evs, EvRoundEnded); e == nil || e.Result.Winner != "caro" {
		t.Fatal("caro debe ganar al bajar las 8 cartas")
	}
}

// RF-28, RF-29, RF-30: depositar las sobrantes en el mismo turno para ganar.
func TestDepositLeftoversToWin(t *testing.T) {
	hand := []Card{nc(1, 0), nc(1, 1), nc(1, 2), nc(1, 3), jk(0), nc(2, 0)}
	g, _ := rig(t, table{turn: 0, hands: [4][]Card{hand, {nc(2, 1), nc(2, 2), nc(2, 3)}, {nc(5, 0)}, {nc(6, 0)}}, hidden: []Card{nc(7, 0)}})
	ok(t)(g.LayDown("ana", [][]CardID{ids(nc(1, 0), nc(1, 1), nc(1, 2))}))

	_, err := g.Deposit("ana", nc(2, 0).ID, 0)
	expectCode(t, err, CodeCardDoesNotFitBank)
	_, err = g.Deposit("ana", nc(1, 3).ID, 1)
	expectCode(t, err, CodeUnknownBank)
	_, err = g.Deposit("ana", nc(1, 3).ID, -1)
	expectCode(t, err, CodeUnknownBank)
	_, err = g.Deposit("ana", nc(3, 0).ID, 0)
	expectCode(t, err, CodeCardNotInHand)
	_, err = g.Deposit("beto", nc(2, 1).ID, 0)
	expectCode(t, err, CodeNotYourTurn)

	evs := ok(t)(g.Deposit("ana", nc(1, 3).ID, 0))
	if e := findEvent(evs, EvCardDeposited); e == nil || *e.Bank != 0 || e.Card.ID != nc(1, 3).ID {
		t.Fatalf("evento de depósito: %+v", e)
	}
	ok(t)(g.Deposit("ana", jk(0).ID, 0)) // el comodín va en cualquier banco
	if len(g.banks[0].Cards) != 5 || g.Phase() != PhaseAwaitingDraw {
		t.Fatal("depósitos no aplicados")
	}
	// Le queda F2#0, que no encaja: sigue el turno normal.
	ok(t)(g.Draw("ana", SourceHidden))
	_, err = g.Deposit("ana", nc(7, 0).ID, 0)
	expectCode(t, err, CodeAlreadyDrew)
	ok(t)(g.Discard("ana", nc(7, 0).ID))

	// beto aún no bajó: no puede depositar.
	_, err = g.Deposit("beto", nc(2, 1).ID, 0)
	expectCode(t, err, CodeNotLaidDown)
}

// RF-28: en turnos posteriores se puede depositar en bancos de cualquier jugador,
// y al quedar en 0 se gana la ronda.
func TestDepositOnOtherBanksInLaterTurn(t *testing.T) {
	g, _ := rig(t, table{
		turn: 0,
		hands: [4][]Card{
			{nc(1, 0), nc(1, 1), nc(1, 2), nc(3, 0)},
			{nc(2, 0), nc(2, 1), nc(2, 2), nc(1, 3)},
			{nc(5, 0)},
			{nc(6, 0)},
		},
		hidden: []Card{nc(7, 1), nc(7, 0), nc(2, 3)},
	})
	ok(t)(g.LayDown("ana", [][]CardID{ids(nc(1, 0), nc(1, 1), nc(1, 2))}))
	ok(t)(g.Draw("ana", SourceHidden)) // toma F2#3
	ok(t)(g.Discard("ana", nc(3, 0).ID))

	ok(t)(g.LayDown("beto", [][]CardID{ids(nc(2, 0), nc(2, 1), nc(2, 2))}))
	ok(t)(g.Deposit("beto", nc(1, 3).ID, 0)) // banco de ana
	if g.Phase() != PhaseRoundOver || g.results[0].Winner != "beto" {
		t.Fatal("beto debe ganar al depositar su última carta")
	}
	// ana conserva F2#3 en la mano.
	if g.results[0].Points["ana"] != 1 {
		t.Fatalf("puntos de ana: %d", g.results[0].Points["ana"])
	}
}
