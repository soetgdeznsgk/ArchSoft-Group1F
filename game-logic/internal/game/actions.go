package game

import "slices"

// Validaciones comunes (RF-10). Ninguna acción modifica el estado si falla.

func (g *Game) seatOf(p PlayerID) (int, error) {
	i, ok := g.seat[p]
	if !ok {
		return -1, ErrUnknownPlayer
	}
	return i, nil
}

func (g *Game) stateError() error {
	switch g.phase {
	case PhaseWaiting:
		return ErrGameNotStarted
	case PhaseRoundOver:
		return ErrRoundNotInPlay
	case PhaseGameOver:
		return ErrGameOver
	}
	return nil
}

// requireTurn valida que p exista, que haya ronda en juego y que sea su turno.
func (g *Game) requireTurn(p PlayerID) (int, error) {
	i, err := g.seatOf(p)
	if err != nil {
		return -1, err
	}
	if err := g.stateError(); err != nil {
		return -1, err
	}
	if i != g.turn {
		return -1, ErrNotYourTurn
	}
	return i, nil
}

// requireBeforeDraw valida además que el jugador aún no haya tomado carta.
func (g *Game) requireBeforeDraw(p PlayerID) (int, error) {
	i, err := g.requireTurn(p)
	if err != nil {
		return -1, err
	}
	if g.phase != PhaseAwaitingDraw {
		return -1, ErrAlreadyDrew
	}
	return i, nil
}

// Draw toma una carta del banco de ocultas o la última del banco de vistas
// (RF-18). Cierra la posibilidad de robar. Si el banco de ocultas está vacío,
// la ronda termina sin ganador (RF-31).
func (g *Game) Draw(p PlayerID, src Source) ([]Event, error) {
	if _, err := g.requireBeforeDraw(p); err != nil {
		return nil, err
	}
	switch src {
	case SourceHidden:
	case SourceDiscard:
		if !g.discardAvailable {
			return nil, ErrDiscardUnavailable
		}
	default:
		return nil, ErrInvalidSource
	}
	g.doDraw(src)
	return g.flush(), nil
}

// doDraw aplica una toma ya validada. Devuelve false si la ronda terminó.
func (g *Game) doDraw(src Source) bool {
	p := g.players[g.turn]
	if src == SourceDiscard {
		c := g.popDiscard()
		p.hand = append(p.hand, c)
		g.emit(Event{Type: EvCardDrawn, Player: p.id, Source: SourceDiscard, Card: &c})
	} else {
		c, ok := g.popHidden()
		if !ok {
			g.endRound(-1, EndHiddenExhausted)
			return false
		}
		p.hand = append(p.hand, c)
		g.emit(Event{Type: EvCardDrawn, Player: p.id, Source: SourceHidden})
	}
	g.discardAvailable = false
	g.phase = PhaseAwaitingDiscard
	return true
}

// Discard bota una carta de la mano al banco de vistas, completa el
// intercambio y pasa el turno a la derecha (RF-16, RF-19).
func (g *Game) Discard(p PlayerID, id CardID) ([]Event, error) {
	i, err := g.requireTurn(p)
	if err != nil {
		return nil, err
	}
	if g.phase != PhaseAwaitingDiscard {
		return nil, ErrMustDrawFirst
	}
	idx := indexOfCard(g.players[i].hand, id)
	if idx < 0 {
		return nil, ErrCardNotInHand
	}
	g.doDiscard(idx)
	g.runAutoTurns()
	return g.flush(), nil
}

func (g *Game) doDiscard(idx int) {
	p := g.players[g.turn]
	c := p.hand[idx]
	p.hand = slices.Delete(p.hand, idx, idx+1)
	g.discard = append(g.discard, c)
	g.discardAvailable = true
	g.lastDiscarder = g.turn
	g.firstTurn = false
	g.emit(Event{Type: EvCardDiscarded, Player: p.id, Card: &c})
	g.advanceTurn()
}

func (g *Game) advanceTurn() {
	g.turn = (g.turn + 1) % NumPlayers
	g.phase = PhaseAwaitingDraw
	g.turnStarted = g.clock.Now()
	g.emit(Event{Type: EvTurnStarted, Player: g.players[g.turn].id})
}

// stealError devuelve por qué el asiento i no puede robar, o nil (RF-22).
func (g *Game) stealError(i int) error {
	switch {
	case i == g.turn:
		return errStealOwnTurn
	case g.phase != PhaseAwaitingDraw:
		return errStealClosed
	case !g.discardAvailable:
		return errStealNothing
	case i == g.lastDiscarder:
		return errStealOwnDiscard
	}
	return nil
}

func (g *Game) canSteal(i int) bool { return g.inPlay() && g.stealError(i) == nil }

// Steal roba la última carta botada (RF-21..RF-24). El ladrón recibe además
// una carta del banco de ocultas como penalización y no bota. El jugador en
// turno conserva el turno, pero ya no puede tomar del banco de vistas.
// Si no hay cartas ocultas para la penalización, la ronda termina sin ganador.
func (g *Game) Steal(p PlayerID) ([]Event, error) {
	i, err := g.seatOf(p)
	if err != nil {
		return nil, err
	}
	if err := g.stateError(); err != nil {
		return nil, err
	}
	if err := g.stealError(i); err != nil {
		return nil, err
	}
	thief := g.players[i]
	c := g.popDiscard()
	g.discardAvailable = false
	thief.hand = append(thief.hand, c)
	g.emit(Event{Type: EvCardStolen, Player: thief.id, Card: &c})

	if pen, ok := g.popHidden(); ok {
		thief.hand = append(thief.hand, pen)
		g.emit(Event{Type: EvPenaltyDrawn, Player: thief.id, Source: SourceHidden})
	} else {
		g.endRound(-1, EndHiddenExhausted)
	}
	return g.flush(), nil
}

// LayDown baja todas las jugadas objetivo de la ronda (RF-26, RF-27). Cada
// jugada es una lista de IDs de cartas de la mano; el orden de las jugadas no
// importa. Las jugadas bajadas se convierten en bancos de depósito. Si la
// mano queda vacía, el jugador gana la ronda (RF-30).
func (g *Game) LayDown(p PlayerID, melds [][]CardID) ([]Event, error) {
	i, err := g.requireBeforeDraw(p)
	if err != nil {
		return nil, err
	}
	if g.firstTurn {
		return nil, ErrFirstTurnExchangeOnly
	}
	pl := g.players[i]
	if pl.laidDown {
		return nil, ErrAlreadyLaidDown
	}
	spec, _ := Spec(g.round)
	sizes := make([]int, len(melds))
	for k, m := range melds {
		sizes[k] = len(m)
	}
	if !meldsMatchSpec(spec, sizes) {
		return nil, newErrorf(CodeMeldsMismatch,
			"la ronda %d exige bajar %s, todas a la vez", g.round, spec.Describe())
	}

	used := make(map[CardID]bool)
	resolved := make([][]Card, len(melds))
	figures := make([]Figure, len(melds))
	for k, m := range melds {
		cards := make([]Card, 0, len(m))
		for _, id := range m {
			if used[id] {
				return nil, ErrDuplicateCard
			}
			used[id] = true
			idx := indexOfCard(pl.hand, id)
			if idx < 0 {
				return nil, ErrCardNotInHand
			}
			cards = append(cards, pl.hand[idx])
		}
		kind, _ := kindForSize(len(cards))
		fig, err := ValidateMeld(kind, cards)
		if err != nil {
			return nil, err
		}
		resolved[k] = cards
		figures[k] = fig
	}

	pl.hand = slices.DeleteFunc(pl.hand, func(c Card) bool { return used[c.ID] })
	newBanks := make([]DepositBank, 0, len(melds))
	for k := range resolved {
		b := &DepositBank{ID: len(g.banks), Owner: pl.id, Figure: figures[k], Cards: resolved[k]}
		g.banks = append(g.banks, b)
		newBanks = append(newBanks, b.clone())
	}
	pl.laidDown = true
	g.emit(Event{Type: EvMeldsLaidDown, Player: pl.id, Banks: newBanks})
	if len(pl.hand) == 0 {
		g.endRound(i, EndPlayerWentOut)
	}
	return g.flush(), nil
}

// Deposit deja una carta de la mano en un banco de depósito (RF-28, RF-29).
// Solo para quien ya bajó, en su turno y antes de tomar carta. Se aceptan
// cartas de la figura del banco o comodines. Si la mano queda vacía, el
// jugador gana la ronda (RF-30).
func (g *Game) Deposit(p PlayerID, id CardID, bankID int) ([]Event, error) {
	i, err := g.requireBeforeDraw(p)
	if err != nil {
		return nil, err
	}
	pl := g.players[i]
	if !pl.laidDown {
		return nil, ErrNotLaidDown
	}
	if bankID < 0 || bankID >= len(g.banks) {
		return nil, ErrUnknownBank
	}
	idx := indexOfCard(pl.hand, id)
	if idx < 0 {
		return nil, ErrCardNotInHand
	}
	c := pl.hand[idx]
	b := g.banks[bankID]
	if !b.accepts(c) {
		return nil, newErrorf(CodeCardDoesNotFitBank,
			"la carta %s no corresponde a la figura F%d del banco %d", c, b.Figure, bankID)
	}
	pl.hand = slices.Delete(pl.hand, idx, idx+1)
	b.Cards = append(b.Cards, c)
	g.emit(Event{Type: EvCardDeposited, Player: pl.id, Card: &c, Bank: &bankID})
	if len(pl.hand) == 0 {
		g.endRound(i, EndPlayerWentOut)
	}
	return g.flush(), nil
}
