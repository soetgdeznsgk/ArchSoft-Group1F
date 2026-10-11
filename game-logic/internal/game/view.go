package game

import "slices"

// PlayerSummary es la información pública de un jugador (RF-33, RF-34).
type PlayerSummary struct {
	ID          PlayerID `json:"id"`
	Seat        int      `json:"seat"`
	CardCount   int      `json:"card_count"`
	LaidDown    bool     `json:"laid_down"`
	Connected   bool     `json:"connected"`
	IsTurn      bool     `json:"is_turn"`
	TotalPoints int      `json:"total_points"`
	RoundsWon   int      `json:"rounds_won"`
}

// PlayerView es lo que un jugador puede ver de la mesa. Solo incluye su
// propia mano; de los demás muestra la cantidad de cartas. Los indicadores
// Can* permiten habilitar o deshabilitar botones en la interfaz (RF-22).
type PlayerView struct {
	You              PlayerID        `json:"you"`
	Round            int             `json:"round"`
	Spec             *RoundSpec      `json:"spec,omitempty"`
	Phase            Phase           `json:"phase"`
	CurrentTurn      PlayerID        `json:"current_turn,omitempty"`
	TurnRemainingMs  int64           `json:"turn_remaining_ms"`
	Hand             []Card          `json:"hand"`
	LaidDown         bool            `json:"laid_down"`
	TopDiscard       *Card           `json:"top_discard,omitempty"`
	DiscardAvailable bool            `json:"discard_available"`
	DiscardCount     int             `json:"discard_count"`
	HiddenCount      int             `json:"hidden_count"`
	Banks            []DepositBank   `json:"banks"`
	Players          []PlayerSummary `json:"players"`
	CanSteal         bool            `json:"can_steal"`
	CanTakeDiscard   bool            `json:"can_take_discard"`
	CanLayDown       bool            `json:"can_lay_down"` // momento permitido; no verifica que tenga las jugadas
	CanDeposit       bool            `json:"can_deposit"`
	CanDiscard       bool            `json:"can_discard"`
	Results          []RoundResult   `json:"results"`
	Final            *FinalResult    `json:"final,omitempty"`
}

// ViewFor construye la vista de la mesa para el jugador p. La vista es una
// copia: modificarla no afecta la partida.
func (g *Game) ViewFor(p PlayerID) (PlayerView, error) {
	i, err := g.seatOf(p)
	if err != nil {
		return PlayerView{}, err
	}
	me := g.players[i]
	v := PlayerView{
		You:          p,
		Round:        g.round,
		Phase:        g.phase,
		Hand:         slices.Clone(me.hand),
		LaidDown:     me.laidDown,
		DiscardCount: len(g.discard),
		HiddenCount:  len(g.hidden),
		Banks:        make([]DepositBank, len(g.banks)),
		Players:      make([]PlayerSummary, len(g.players)),
		Results:      g.Results(),
		Final:        g.final.clone(),
	}
	if v.Hand == nil {
		v.Hand = []Card{}
	}
	if spec, ok := Spec(g.round); ok {
		v.Spec = &spec
	}
	if n := len(g.discard); n > 0 {
		top := g.discard[n-1]
		v.TopDiscard = &top
		v.DiscardAvailable = g.discardAvailable
	}
	for k, b := range g.banks {
		v.Banks[k] = b.clone()
	}
	inPlay := g.inPlay()
	for k, pl := range g.players {
		v.Players[k] = PlayerSummary{
			ID: pl.id, Seat: k, CardCount: len(pl.hand), LaidDown: pl.laidDown,
			Connected: pl.connected, IsTurn: inPlay && k == g.turn,
			TotalPoints: pl.total, RoundsWon: pl.roundsWon,
		}
	}
	if inPlay {
		v.CurrentTurn = g.players[g.turn].id
		if rem := g.turnStarted.Add(g.turnDuration).Sub(g.clock.Now()); rem > 0 {
			v.TurnRemainingMs = rem.Milliseconds()
		}
		myTurn := i == g.turn
		beforeDraw := myTurn && g.phase == PhaseAwaitingDraw
		v.CanSteal = g.canSteal(i)
		v.CanTakeDiscard = beforeDraw && g.discardAvailable
		v.CanLayDown = beforeDraw && !g.firstTurn && !me.laidDown
		v.CanDeposit = beforeDraw && me.laidDown
		v.CanDiscard = myTurn && g.phase == PhaseAwaitingDiscard
	}
	return v, nil
}
