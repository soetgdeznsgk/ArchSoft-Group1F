package game

// EventType identifica un cambio de estado que debe difundirse (RF-11).
type EventType string

const (
	EvRoundStarted     EventType = "round_started"
	EvTurnStarted      EventType = "turn_started"
	EvCardDrawn        EventType = "card_drawn"
	EvCardDiscarded    EventType = "card_discarded"
	EvCardStolen       EventType = "card_stolen"
	EvPenaltyDrawn     EventType = "penalty_drawn"
	EvMeldsLaidDown    EventType = "melds_laid_down"
	EvCardDeposited    EventType = "card_deposited"
	EvAutoExchange     EventType = "auto_exchange"
	EvPlayerConnection EventType = "player_connection"
	EvRoundEnded       EventType = "round_ended"
	EvGameEnded        EventType = "game_ended"
)

// Event es un cambio de estado público. Los eventos nunca revelan cartas
// ocultas: una carta tomada del banco de ocultas o recibida como penalización
// no se incluye. Cada jugador obtiene su mano mediante Game.ViewFor.
//
// Seq es monotónico por partida y permite a los clientes ordenar y detectar
// eventos perdidos.
type Event struct {
	Seq       uint64        `json:"seq"`
	Type      EventType     `json:"type"`
	Round     int           `json:"round"`
	Player    PlayerID      `json:"player,omitempty"`
	Card      *Card         `json:"card,omitempty"`
	Source    Source        `json:"source,omitempty"`
	Bank      *int          `json:"bank,omitempty"`
	Banks     []DepositBank `json:"banks,omitempty"`
	Spec      *RoundSpec    `json:"spec,omitempty"`
	Reason    string        `json:"reason,omitempty"`
	Connected *bool         `json:"connected,omitempty"`
	Result    *RoundResult  `json:"result,omitempty"`
	Final     *FinalResult  `json:"final,omitempty"`
}
