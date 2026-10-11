package game

// ActionType identifica una acción de un jugador.
type ActionType string

const (
	ActDraw    ActionType = "draw"
	ActDiscard ActionType = "discard"
	ActSteal   ActionType = "steal"
	ActLayDown ActionType = "lay_down"
	ActDeposit ActionType = "deposit"
)

// Action es la forma serializable de una acción, pensada para que la capa de
// red (WebSocket) la decodifique desde JSON y la entregue a Match.Apply.
//
//	{"type":"draw","player":"ana","source":"hidden"}
//	{"type":"discard","player":"ana","card":12}
//	{"type":"steal","player":"beto"}
//	{"type":"lay_down","player":"ana","melds":[[1,2,56]]}
//	{"type":"deposit","player":"ana","card":3,"bank":0}
type Action struct {
	Type   ActionType `json:"type"`
	Player PlayerID   `json:"player"`
	Source Source     `json:"source,omitempty"`
	Card   CardID     `json:"card"`
	Bank   int        `json:"bank"`
	Melds  [][]CardID `json:"melds,omitempty"`
}

// Apply despacha la acción al método correspondiente.
func (g *Game) Apply(a Action) ([]Event, error) {
	switch a.Type {
	case ActDraw:
		return g.Draw(a.Player, a.Source)
	case ActDiscard:
		return g.Discard(a.Player, a.Card)
	case ActSteal:
		return g.Steal(a.Player)
	case ActLayDown:
		return g.LayDown(a.Player, a.Melds)
	case ActDeposit:
		return g.Deposit(a.Player, a.Card, a.Bank)
	}
	return nil, ErrInvalidAction
}
