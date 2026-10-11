package game

import "time"

// AutoReason indica por qué se aplicó un intercambio automático.
type AutoReason string

const (
	AutoTimeout      AutoReason = "timeout"
	AutoDisconnected AutoReason = "disconnected"
)

// TurnDeadline devuelve el instante en que vence el turno actual.
func (g *Game) TurnDeadline() (time.Time, bool) {
	if !g.inPlay() {
		return time.Time{}, false
	}
	return g.turnStarted.Add(g.turnDuration), true
}

// Tick debe invocarse periódicamente (p. ej. cada 250 ms) por la capa que
// aloja la partida. Si el turno venció, aplica el intercambio automático
// (RF-17, RF-20). Si no venció, no hace nada.
func (g *Game) Tick() []Event {
	deadline, ok := g.TurnDeadline()
	if !ok || g.clock.Now().Before(deadline) {
		return nil
	}
	g.autoExchange(AutoTimeout)
	g.runAutoTurns()
	return g.flush()
}

// SetConnected registra la conexión o desconexión de un jugador. Mientras
// esté desconectado, cada uno de sus turnos se resuelve de inmediato con un
// intercambio automático (propuesta 14.2). Puede reconectarse en cualquier
// momento.
func (g *Game) SetConnected(p PlayerID, connected bool) ([]Event, error) {
	i, err := g.seatOf(p)
	if err != nil {
		return nil, err
	}
	pl := g.players[i]
	if pl.connected == connected {
		return nil, nil
	}
	pl.connected = connected
	g.emit(Event{Type: EvPlayerConnection, Player: pl.id, Connected: &connected})
	g.runAutoTurns()
	return g.flush(), nil
}

// autoExchange toma del banco de ocultas (si aún no tomó) y bota la primera
// carta de la mano del jugador en turno (RF-20).
func (g *Game) autoExchange(reason AutoReason) {
	p := g.players[g.turn]
	g.emit(Event{Type: EvAutoExchange, Player: p.id, Reason: string(reason)})
	if g.phase == PhaseAwaitingDraw && !g.doDraw(SourceHidden) {
		return // se agotó el banco de ocultas: la ronda terminó
	}
	g.doDiscard(0)
}

// runAutoTurns resuelve los turnos de jugadores desconectados. Termina
// porque cada iteración consume una carta oculta o cierra la ronda.
func (g *Game) runAutoTurns() {
	for g.inPlay() && !g.players[g.turn].connected {
		g.autoExchange(AutoDisconnected)
	}
}
