package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/chameleon"
	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/game"
)

// Modos de juego. Los nombres coinciden con los del Lobby Manager.
const (
	ModeClassic   = "CLASSIC"   // 5 rondas de tríos y cuartetos (internal/game)
	ModeChameleon = "CHAMELEON" // repartidor automático, reunir 8 colores (internal/chameleon)
)

// playerLimits es la cantidad de jugadores que admite cada modo.
var playerLimits = map[string][2]int{
	ModeClassic:   {game.NumPlayers, game.NumPlayers},
	ModeChameleon: {chameleon.MinPlayers, chameleon.MaxPlayers},
}

func normalizeMode(mode string) string {
	if mode == "" {
		return ModeClassic // registros y peticiones anteriores a los modos
	}
	return mode
}

// outcome es el resultado de una operación del motor: los eventos a difundir
// y las señales que necesita el servidor para el ciclo de vida.
type outcome struct {
	events     []any
	roundEnded bool
	gameEnded  bool
}

func (o outcome) empty() bool { return len(o.events) == 0 }

// finalResult es el resultado común que se envía al Lobby.
type finalResult struct {
	Winners   []string
	EndReason string
	Standings any
}

// actionError es un rechazo de una acción del jugador, con código estable.
type actionError struct {
	Code    string
	Message string
}

func (e *actionError) Error() string { return e.Code + ": " + e.Message }

// engine es lo que el servidor necesita de un modo de juego. Cada modo tiene
// un adaptador; el servidor (WebSocket, Redis, ticks y callback al Lobby) es
// el mismo para todos.
type engine interface {
	Mode() string
	Players() []string
	Start() (outcome, error)
	StartNextRound() (outcome, error)
	// Apply decodifica y aplica una acción JSON del jugador. Los rechazos son *actionError.
	Apply(player string, raw []byte) (outcome, error)
	Tick() outcome
	SetConnected(player string, connected bool) (outcome, error)
	MarkAllDisconnected() outcome
	View(player string) (any, error)
	Status() string
	Round() int
	HandSize(player string) int
	Final() *finalResult
	Snapshot() ([]byte, error)
}

func newEngine(mode string, ids []string, cfg Config) (engine, error) {
	switch mode {
	case ModeClassic:
		pids := make([]game.PlayerID, len(ids))
		for i, id := range ids {
			pids[i] = game.PlayerID(id)
		}
		m, err := game.NewMatch(pids, game.Config{})
		if err != nil {
			return nil, err
		}
		return classicEngine{m}, nil
	case ModeChameleon:
		m, err := chameleon.New(ids, cfg.Chameleon)
		if err != nil {
			return nil, err
		}
		return chameleonEngine{m}, nil
	}
	return nil, fmt.Errorf("modo de juego desconocido %q", mode)
}

func restoreEngine(mode string, snapshot []byte, cfg Config) (engine, error) {
	switch mode {
	case ModeClassic:
		m, err := game.RestoreMatch(snapshot, game.Config{})
		if err != nil {
			return nil, err
		}
		return classicEngine{m}, nil
	case ModeChameleon:
		restoreCfg := chameleon.Config{Rand: cfg.Chameleon.Rand, Clock: cfg.Chameleon.Clock}
		m, err := chameleon.Restore(snapshot, restoreCfg)
		if err != nil {
			return nil, err
		}
		return chameleonEngine{m}, nil
	}
	return nil, fmt.Errorf("modo de juego desconocido %q", mode)
}

// decodeAction decodifica exactamente un objeto JSON con campos conocidos.
func decodeAction(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &actionError{Code: "invalid_action", Message: "mensaje JSON inválido"}
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return &actionError{Code: "invalid_action", Message: "se esperaba una sola acción por mensaje"}
	}
	return nil
}

// --- modo clásico ------------------------------------------------------------

type classicEngine struct{ m *game.Match }

func classicOutcome(events []game.Event) outcome {
	out := outcome{events: make([]any, len(events))}
	for i, e := range events {
		out.events[i] = e
		out.roundEnded = out.roundEnded || e.Type == game.EvRoundEnded
		out.gameEnded = out.gameEnded || e.Type == game.EvGameEnded
	}
	return out
}

func (e classicEngine) Mode() string { return ModeClassic }

func (e classicEngine) Players() []string {
	ids := e.m.Players()
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

func (e classicEngine) Start() (outcome, error) {
	evs, err := e.m.Start()
	return classicOutcome(evs), err
}

func (e classicEngine) StartNextRound() (outcome, error) {
	evs, err := e.m.StartNextRound()
	return classicOutcome(evs), err
}

func (e classicEngine) Apply(player string, raw []byte) (outcome, error) {
	var action game.Action
	if err := decodeAction(raw, &action); err != nil {
		return outcome{}, err
	}
	action.Player = game.PlayerID(player) // el jugador sale del token, nunca del mensaje
	evs, err := e.m.Apply(action)
	if err != nil {
		code := string(game.CodeOf(err))
		if code == "" {
			code = "invalid_action"
		}
		message := err.Error()
		var gameErr *game.Error
		if errors.As(err, &gameErr) {
			message = gameErr.Message
		}
		return outcome{}, &actionError{Code: code, Message: message}
	}
	return classicOutcome(evs), nil
}

func (e classicEngine) Tick() outcome { return classicOutcome(e.m.Tick()) }

func (e classicEngine) SetConnected(player string, connected bool) (outcome, error) {
	evs, err := e.m.SetConnected(game.PlayerID(player), connected)
	return classicOutcome(evs), err
}

func (e classicEngine) MarkAllDisconnected() outcome {
	return classicOutcome(e.m.MarkAllDisconnected())
}

func (e classicEngine) View(player string) (any, error) { return e.m.ViewFor(game.PlayerID(player)) }

func (e classicEngine) Status() string {
	switch e.m.Phase() {
	case game.PhaseWaiting:
		return statusWaiting
	case game.PhaseRoundOver:
		return statusRoundOver
	case game.PhaseGameOver:
		return statusFinished
	}
	return statusPlaying
}

func (e classicEngine) Round() int { return e.m.Round() }

func (e classicEngine) HandSize(player string) int {
	view, err := e.m.ViewFor(game.PlayerID(player))
	if err != nil {
		return 0
	}
	for _, summary := range view.Players {
		if string(summary.ID) == player {
			return summary.CardCount
		}
	}
	return 0
}

func (e classicEngine) Final() *finalResult {
	f := e.m.Final()
	if f == nil {
		return nil
	}
	winners := make([]string, len(f.Winners))
	for i, w := range f.Winners {
		winners[i] = string(w)
	}
	return &finalResult{Winners: winners, EndReason: "completed", Standings: f.Standings}
}

func (e classicEngine) Snapshot() ([]byte, error) { return e.m.Snapshot() }

// --- modo camaleón -----------------------------------------------------------

type chameleonEngine struct{ m *chameleon.Match }

func chameleonOutcome(events []chameleon.Event) outcome {
	out := outcome{events: make([]any, len(events))}
	for i, e := range events {
		out.events[i] = e
		out.gameEnded = out.gameEnded || e.Type == chameleon.EvGameEnded
	}
	return out
}

func (e chameleonEngine) Mode() string      { return ModeChameleon }
func (e chameleonEngine) Players() []string { return e.m.Players() }

func (e chameleonEngine) Start() (outcome, error) {
	evs, err := e.m.Start()
	return chameleonOutcome(evs), err
}

// StartNextRound no aplica: el modo camaleón es una sola ronda.
func (e chameleonEngine) StartNextRound() (outcome, error) {
	return outcome{}, errors.New("el modo camaleón no tiene rondas")
}

func (e chameleonEngine) Apply(player string, raw []byte) (outcome, error) {
	var action chameleon.Action
	if err := decodeAction(raw, &action); err != nil {
		return outcome{}, err
	}
	action.Player = player
	evs, err := e.m.Apply(action)
	if err != nil {
		var chErr *chameleon.Error
		if errors.As(err, &chErr) {
			return outcome{}, &actionError{Code: string(chErr.Code), Message: chErr.Message}
		}
		return outcome{}, &actionError{Code: "invalid_action", Message: err.Error()}
	}
	return chameleonOutcome(evs), nil
}

func (e chameleonEngine) Tick() outcome { return chameleonOutcome(e.m.Tick()) }

func (e chameleonEngine) SetConnected(player string, connected bool) (outcome, error) {
	evs, err := e.m.SetConnected(player, connected)
	return chameleonOutcome(evs), err
}

func (e chameleonEngine) MarkAllDisconnected() outcome {
	return chameleonOutcome(e.m.MarkAllDisconnected())
}

func (e chameleonEngine) View(player string) (any, error) { return e.m.ViewFor(player) }

func (e chameleonEngine) Status() string {
	switch e.m.Phase() {
	case chameleon.PhaseWaiting:
		return statusWaiting
	case chameleon.PhaseGameOver:
		return statusFinished
	}
	return statusPlaying
}

func (e chameleonEngine) Round() int {
	if e.m.Phase() == chameleon.PhaseWaiting {
		return 0
	}
	return 1
}

func (e chameleonEngine) HandSize(player string) int { return e.m.HandSize(player) }

func (e chameleonEngine) Final() *finalResult {
	f := e.m.Final()
	if f == nil {
		return nil
	}
	return &finalResult{Winners: f.Winners, EndReason: string(f.Reason), Standings: f.Standings}
}

func (e chameleonEngine) Snapshot() ([]byte, error) { return e.m.Snapshot() }
