package game

import (
	"errors"
	"fmt"
)

// ErrorCode es un código estable que el cliente puede usar para mostrar el
// motivo del rechazo (RF-10, RNF-09). El mensaje es legible para el jugador.
type ErrorCode string

const (
	CodeInvalidPlayers        ErrorCode = "invalid_players"
	CodeGameNotStarted        ErrorCode = "game_not_started"
	CodeGameAlreadyStarted    ErrorCode = "game_already_started"
	CodeRoundNotInPlay        ErrorCode = "round_not_in_play"
	CodeRoundStillInPlay      ErrorCode = "round_still_in_play"
	CodeGameOver              ErrorCode = "game_over"
	CodeUnknownPlayer         ErrorCode = "unknown_player"
	CodeNotYourTurn           ErrorCode = "not_your_turn"
	CodeAlreadyDrew           ErrorCode = "already_drew"
	CodeMustDrawFirst         ErrorCode = "must_draw_first"
	CodeInvalidSource         ErrorCode = "invalid_source"
	CodeDiscardUnavailable    ErrorCode = "discard_unavailable"
	CodeCardNotInHand         ErrorCode = "card_not_in_hand"
	CodeStealNotAllowed       ErrorCode = "steal_not_allowed"
	CodeFirstTurnExchangeOnly ErrorCode = "first_turn_exchange_only"
	CodeAlreadyLaidDown       ErrorCode = "already_laid_down"
	CodeNotLaidDown           ErrorCode = "not_laid_down"
	CodeInvalidMeld           ErrorCode = "invalid_meld"
	CodeMeldsMismatch         ErrorCode = "melds_mismatch"
	CodeDuplicateCard         ErrorCode = "duplicate_card"
	CodeUnknownBank           ErrorCode = "unknown_bank"
	CodeCardDoesNotFitBank    ErrorCode = "card_does_not_fit_bank"
	CodeInvalidAction         ErrorCode = "invalid_action"
)

// Error es el error de dominio que devuelven todas las acciones rechazadas.
// Dos *Error son equivalentes para errors.Is si tienen el mismo código.
type Error struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

// Is permite comparar por código: errors.Is(err, game.ErrNotYourTurn).
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

// Errores de dominio predefinidos.
var (
	ErrGameNotStarted        = newError(CodeGameNotStarted, "la partida no ha iniciado")
	ErrGameAlreadyStarted    = newError(CodeGameAlreadyStarted, "la partida ya inició")
	ErrRoundNotInPlay        = newError(CodeRoundNotInPlay, "la ronda terminó; espera el inicio de la siguiente")
	ErrRoundStillInPlay      = newError(CodeRoundStillInPlay, "la ronda actual aún no termina")
	ErrGameOver              = newError(CodeGameOver, "la partida terminó")
	ErrUnknownPlayer         = newError(CodeUnknownPlayer, "el jugador no pertenece a esta partida")
	ErrNotYourTurn           = newError(CodeNotYourTurn, "no es tu turno")
	ErrAlreadyDrew           = newError(CodeAlreadyDrew, "ya tomaste carta en este turno; solo puedes botar")
	ErrMustDrawFirst         = newError(CodeMustDrawFirst, "debes tomar una carta antes de botar")
	ErrInvalidSource         = newError(CodeInvalidSource, "origen de carta inválido: usa \"hidden\" o \"discard\"")
	ErrDiscardUnavailable    = newError(CodeDiscardUnavailable, "no hay carta disponible en el banco de cartas vistas; toma del banco de cartas ocultas")
	ErrCardNotInHand         = newError(CodeCardNotInHand, "la carta no está en tu mano")
	ErrFirstTurnExchangeOnly = newError(CodeFirstTurnExchangeOnly, "en el turno inicial de la ronda solo se toma del banco de cartas ocultas y se bota una carta")
	ErrAlreadyLaidDown       = newError(CodeAlreadyLaidDown, "ya bajaste tus jugadas en esta ronda")
	ErrNotLaidDown           = newError(CodeNotLaidDown, "debes bajar tus jugadas antes de depositar")
	ErrDuplicateCard         = newError(CodeDuplicateCard, "una misma carta aparece más de una vez")
	ErrUnknownBank           = newError(CodeUnknownBank, "el banco de depósito no existe")
	ErrInvalidAction         = newError(CodeInvalidAction, "acción desconocida")

	errStealOwnTurn    = newError(CodeStealNotAllowed, "el jugador en turno no puede robar; puede tomar la carta del banco de cartas vistas")
	errStealClosed     = newError(CodeStealNotAllowed, "el jugador en turno ya tomó carta; el robo está cerrado")
	errStealNothing    = newError(CodeStealNotAllowed, "no hay carta para robar en el banco de cartas vistas")
	errStealOwnDiscard = newError(CodeStealNotAllowed, "no puedes robar la carta que acabas de botar")
)

// ErrStealNotAllowed sirve para comparar con errors.Is cualquier rechazo de robo.
var ErrStealNotAllowed = newError(CodeStealNotAllowed, "robo no permitido")
