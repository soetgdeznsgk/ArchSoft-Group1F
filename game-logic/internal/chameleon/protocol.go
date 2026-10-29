package chameleon

import (
	"errors"
	"fmt"
	"slices"
)

// ActionType identifica una acción del jugador.
type ActionType string

// ActTake toma una carta de la mesa: {"type":"take","card":12}.
const ActTake ActionType = "take"

// Action es la forma serializable de una acción.
type Action struct {
	Type   ActionType `json:"type"`
	Player string     `json:"player,omitempty"` // lo fija el servidor a partir del token
	Card   int        `json:"card"`
}

// EventType identifica un cambio de estado público.
type EventType string

const (
	EvGameStarted      EventType = "game_started"
	EvCardSpawned      EventType = "card_spawned"
	EvCardExpired      EventType = "card_expired"
	EvCardTaken        EventType = "card_taken"
	EvColorRepeated    EventType = "color_repeated"  // tomó un color que ya tenía: pierde ambas
	EvChameleonTaken   EventType = "chameleon_taken" // tomó el camaleón: pierde toda la mano
	EvPlayerConnection EventType = "player_connection"
	EvGameEnded        EventType = "game_ended"
)

// Event es un cambio de estado. En este modo todo es público: las manos de
// todos se ven, así que los eventos incluyen las cartas.
type Event struct {
	Seq       uint64       `json:"seq"`
	Type      EventType    `json:"type"`
	Mode      string       `json:"mode"`
	Player    string       `json:"player,omitempty"`
	Card      *BoardCard   `json:"card,omitempty"`
	Lost      []Card       `json:"lost,omitempty"`
	Connected *bool        `json:"connected,omitempty"`
	Final     *FinalResult `json:"final,omitempty"`
}

// MarshalJSON fija el campo mode para que el cliente distinga el origen.
func (e Event) MarshalJSON() ([]byte, error) {
	type plain Event
	e.Mode = Mode
	return jsonMarshal(plain(e))
}

// PlayerState es lo que todos ven de un jugador.
type PlayerState struct {
	ID        string `json:"id"`
	Seat      int    `json:"seat"`
	Connected bool   `json:"connected"`
	Cards     []Card `json:"cards"`
	Colors    int    `json:"colors"` // colores distintos que tiene (= len(cards))
}

// BoardView es una carta de la mesa vista por el cliente.
type BoardView struct {
	Card
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	ExpiresInMs int64   `json:"expires_in_ms"`
}

// View es la mesa vista por un jugador. Las manos son públicas.
type View struct {
	Mode        string        `json:"mode"`
	You         string        `json:"you"`
	Phase       Phase         `json:"phase"`
	RemainingMs int64         `json:"remaining_ms"` // tiempo restante de la partida
	Colors      []Color       `json:"colors"`       // los 8 colores a reunir
	Board       []BoardView   `json:"board"`
	Players     []PlayerState `json:"players"`
	CanTake     bool          `json:"can_take"`
	Final       *FinalResult  `json:"final,omitempty"`
}

// ViewFor construye la vista para el jugador p.
func (m *Match) ViewFor(p string) (View, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.seat[p]; !ok {
		return View{}, ErrUnknownPlayer
	}
	now := m.cfg.Clock.Now()
	v := View{
		Mode: Mode, You: p, Phase: m.phase, Colors: slices.Clone(Colors),
		Board: make([]BoardView, 0, len(m.board)), Players: make([]PlayerState, len(m.players)),
		CanTake: m.phase == PhasePlaying, Final: m.final.clone(),
	}
	if m.phase == PhasePlaying {
		v.RemainingMs = max(0, m.endsAt.Sub(now).Milliseconds())
		for _, b := range m.board {
			if !now.Before(b.ExpiresAt) {
				continue
			}
			v.Board = append(v.Board, BoardView{Card: b.Card, X: b.X, Y: b.Y, ExpiresInMs: b.ExpiresAt.Sub(now).Milliseconds()})
		}
	}
	for i, pl := range m.players {
		cards := slices.Clone(pl.hand)
		if cards == nil {
			cards = []Card{}
		}
		v.Players[i] = PlayerState{ID: pl.id, Seat: i, Connected: pl.connected, Cards: cards, Colors: len(cards)}
	}
	return v, nil
}

// ErrorCode es un código estable para el cliente.
type ErrorCode string

const (
	CodeInvalidPlayers   ErrorCode = "invalid_players"
	CodeAlreadyStarted   ErrorCode = "game_already_started"
	CodeNotStarted       ErrorCode = "game_not_started"
	CodeGameOver         ErrorCode = "game_over"
	CodeUnknownPlayer    ErrorCode = "unknown_player"
	CodeCardNotAvailable ErrorCode = "card_not_available"
	CodeInvalidAction    ErrorCode = "invalid_action"
)

// Error es el error de dominio de las acciones rechazadas. El estado no cambia.
type Error struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

// Is compara por código: errors.Is(err, chameleon.ErrGameOver).
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

func newError(code ErrorCode, msg string) *Error { return &Error{Code: code, Message: msg} }

func newErrorf(code ErrorCode, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// CodeOf extrae el código de un error de dominio ("" si no lo es).
func CodeOf(err error) ErrorCode {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

var (
	ErrAlreadyStarted   = newError(CodeAlreadyStarted, "la partida ya inició")
	ErrNotStarted       = newError(CodeNotStarted, "la partida no ha iniciado; esperando jugadores")
	ErrGameOver         = newError(CodeGameOver, "la partida terminó")
	ErrUnknownPlayer    = newError(CodeUnknownPlayer, "el jugador no pertenece a esta partida")
	ErrCardNotAvailable = newError(CodeCardNotAvailable, "esa carta ya no está en la mesa")
	ErrInvalidAction    = newError(CodeInvalidAction, "acción desconocida; usa {\"type\":\"take\",\"card\":<id>}")
)
