package game

import (
	"math/rand/v2"
	"testing"
	"time"
)

func timerTable() table {
	return table{
		turn: 1,
		hands: [4][]Card{
			{nc(1, 0), nc(1, 1)},
			{nc(2, 0), nc(2, 1)},
			{nc(3, 0), nc(3, 1)},
			{nc(4, 0), nc(4, 1)},
		},
		hidden: []Card{nc(5, 0), nc(5, 1), nc(5, 2), nc(5, 3), nc(5, 4), nc(5, 5), nc(5, 6), nc(5, 7)},
	}
}

// RF-17, RF-20: al vencer los 30 s se toma del banco de ocultas y se bota la primera carta.
func TestTimeoutAutoExchange(t *testing.T) {
	g, clk := rig(t, timerTable())
	clk.Advance(29 * time.Second)
	if evs := g.Tick(); evs != nil {
		t.Fatal("antes de 30 s no debe actuar")
	}
	clk.Advance(time.Second)
	evs := g.Tick()
	ae := findEvent(evs, EvAutoExchange)
	if ae == nil || ae.Player != "beto" || ae.Reason != string(AutoTimeout) {
		t.Fatalf("evento: %+v", ae)
	}
	if g.discard[len(g.discard)-1].ID != nc(2, 0).ID {
		t.Fatal("debe botar la primera carta de la mano")
	}
	if len(g.players[1].hand) != 2 || !holds(g, 1, nc(5, 7).ID) || len(g.hidden) != 7 {
		t.Fatalf("mano tras el intercambio: %v", handIDs(g, 1))
	}
	if g.CurrentPlayer() != "caro" {
		t.Fatal("el turno debe pasar")
	}
	if evs := g.Tick(); evs != nil {
		t.Fatal("el reloj se reinicia con el nuevo turno")
	}
}

// RF-20: si ya tomó carta y se le acaba el tiempo, solo se bota la primera carta.
func TestTimeoutAfterDrawOnlyDiscards(t *testing.T) {
	g, clk := rig(t, timerTable())
	ok(t)(g.Draw("beto", SourceHidden))
	clk.Advance(DefaultTurnDuration)
	ok(t)(g.Tick(), nil)
	if len(g.hidden) != 7 || len(g.players[1].hand) != 2 || g.discard[0].ID != nc(2, 0).ID {
		t.Fatal("no debe tomar otra carta")
	}
}

// Si completó el intercambio antes de 30 s no se aplica el automático.
func TestTimerResetsEachTurn(t *testing.T) {
	g, clk := rig(t, timerTable())
	clk.Advance(20 * time.Second)
	ok(t)(g.Draw("beto", SourceHidden))
	ok(t)(g.Discard("beto", nc(2, 0).ID))
	clk.Advance(20 * time.Second)
	if evs := g.Tick(); evs != nil {
		t.Fatal("caro aún tiene 10 s")
	}
	dl, ok := g.TurnDeadline()
	if !ok || !dl.Equal(clk.Now().Add(10*time.Second)) {
		t.Fatalf("plazo %v", dl)
	}
	v, _ := g.ViewFor("ana")
	if v.TurnRemainingMs != 10_000 {
		t.Fatalf("tiempo restante %d", v.TurnRemainingMs)
	}
}

func TestCustomTurnDurationAndTimeoutWithEmptyHidden(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	g, err := New(testPlayers, Config{TurnDuration: 5 * time.Second, Clock: clk, Rand: rand.New(rand.NewPCG(1, 1))})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := g.TurnDeadline(); ok {
		t.Fatal("sin plazo antes de iniciar")
	}
	if g.Tick() != nil {
		t.Fatal("Tick antes de iniciar")
	}
	ok(t)(g.Start())
	g.hidden = nil
	clk.Advance(5 * time.Second)
	evs := g.Tick()
	if !hasEvent(evs, EvAutoExchange) || !hasEvent(evs, EvRoundEnded) || g.Phase() != PhaseRoundOver {
		t.Fatal("intercambio automático sin ocultas debe cerrar la ronda")
	}
}

// Propuesta 14.2: desconectado => intercambio automático inmediato en cada turno suyo.
func TestDisconnectedPlayerAutoExchange(t *testing.T) {
	g, _ := rig(t, timerTable())

	// Desconexión de un jugador que no está en turno: no pasa nada todavía.
	evs := ok(t)(g.SetConnected("caro", false))
	if !hasEvent(evs, EvPlayerConnection) || hasEvent(evs, EvAutoExchange) {
		t.Fatal("solo se informa la desconexión")
	}
	if evs, err := g.SetConnected("caro", false); evs != nil || err != nil {
		t.Fatal("repetir el mismo estado no genera eventos")
	}

	// Cuando beto bota, el turno de caro se resuelve solo y pasa a dani.
	ok(t)(g.Draw("beto", SourceHidden))
	evs = ok(t)(g.Discard("beto", nc(2, 0).ID))
	ae := findEvent(evs, EvAutoExchange)
	if ae == nil || ae.Player != "caro" || ae.Reason != string(AutoDisconnected) || g.CurrentPlayer() != "dani" {
		t.Fatalf("auto=%+v turno=%s", ae, g.CurrentPlayer())
	}

	// Desconexión del jugador en turno: se resuelve de inmediato.
	evs = ok(t)(g.SetConnected("dani", false))
	if !hasEvent(evs, EvAutoExchange) || g.CurrentPlayer() != "ana" {
		t.Fatalf("turno=%s", g.CurrentPlayer())
	}

	// Reconexión: vuelve a jugar normalmente.
	ok(t)(g.SetConnected("caro", true))
	ok(t)(g.Draw("ana", SourceHidden))
	ok(t)(g.Discard("ana", nc(1, 0).ID))
	ok(t)(g.Draw("beto", SourceHidden))
	ok(t)(g.Discard("beto", g.players[1].hand[0].ID))
	if g.CurrentPlayer() != "caro" {
		t.Fatal("caro reconectado debe jugar su turno")
	}
	v, _ := g.ViewFor("ana")
	if v.Players[3].Connected || !v.Players[2].Connected {
		t.Fatal("la vista debe reflejar la conexión")
	}
	_, err := g.SetConnected("zoe", false)
	expectCode(t, err, CodeUnknownPlayer)
}

// Si todos se desconectan la ronda avanza sola hasta agotar las ocultas (termina).
func TestAllDisconnectedRoundTerminates(t *testing.T) {
	g, _ := startedGame(t, 4)
	for _, p := range testPlayers {
		ok(t)(g.SetConnected(p, false))
	}
	if g.Phase() != PhaseRoundOver || g.results[0].Reason != EndHiddenExhausted {
		t.Fatalf("fase %s", g.Phase())
	}
	if err := g.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
	// La siguiente ronda también se resuelve sola mientras sigan desconectados.
	ok(t)(g.StartNextRound())
	if g.Phase() != PhaseRoundOver || g.Round() != 2 {
		t.Fatal("ronda 2 automática")
	}
}

// Un jugador desconectado antes de iniciar que resulta ser el inicial.
func TestDisconnectedStarter(t *testing.T) {
	g, _ := newTestGame(t, 8)
	for _, p := range testPlayers[1:] {
		ok(t)(g.SetConnected(p, false))
	}
	evs := ok(t)(g.Start())
	if g.CurrentPlayer() != "ana" {
		t.Fatalf("turno de %s; los desconectados se saltan", g.CurrentPlayer())
	}
	if g.players[0].connected != true || !hasEvent(evs, EvRoundStarted) {
		t.Fatal("inicio")
	}
}
