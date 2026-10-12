package game

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const snapshotVersion = 1
const maxSnapshotSize = 1 << 20

type snapshotPlayer struct {
	ID        PlayerID `json:"id"`
	Hand      []Card   `json:"hand"`
	LaidDown  bool     `json:"laid_down"`
	Connected bool     `json:"connected"`
	Total     int      `json:"total"`
	RoundsWon int      `json:"rounds_won"`
	LastRound int      `json:"last_round"`
}

type snapshotState struct {
	Version          int              `json:"version"`
	TurnDuration     time.Duration    `json:"turn_duration"`
	Players          []snapshotPlayer `json:"players"`
	Round            int              `json:"round"`
	Phase            Phase            `json:"phase"`
	Turn             int              `json:"turn"`
	FirstTurn        bool             `json:"first_turn"`
	Hidden           []Card           `json:"hidden"`
	Discard          []Card           `json:"discard"`
	DiscardAvailable bool             `json:"discard_available"`
	LastDiscarder    int              `json:"last_discarder"`
	Banks            []*DepositBank   `json:"banks"`
	TurnStarted      time.Time        `json:"turn_started"`
	Results          []RoundResult    `json:"results"`
	Final            *FinalResult     `json:"final,omitempty"`
	Sequence         uint64           `json:"sequence"`
}

// Snapshot returns a versioned representation of the complete authoritative
// match state. Randomness is reseeded after restore; the current shuffled deck
// is part of the snapshot, so the active round is unaffected.
func (m *Match) Snapshot() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := snapshotState{
		Version:          snapshotVersion,
		TurnDuration:     m.g.turnDuration,
		Round:            m.g.round,
		Phase:            m.g.phase,
		Turn:             m.g.turn,
		FirstTurn:        m.g.firstTurn,
		Hidden:           m.g.hidden,
		Discard:          m.g.discard,
		DiscardAvailable: m.g.discardAvailable,
		LastDiscarder:    m.g.lastDiscarder,
		Banks:            m.g.banks,
		TurnStarted:      m.g.turnStarted,
		Results:          m.g.results,
		Final:            m.g.final,
		Sequence:         m.g.seq,
		Players:          make([]snapshotPlayer, len(m.g.players)),
	}
	for i, player := range m.g.players {
		state.Players[i] = snapshotPlayer{
			ID: player.id, Hand: player.hand, LaidDown: player.laidDown,
			Connected: player.connected, Total: player.total,
			RoundsWon: player.roundsWon, LastRound: player.lastRound,
		}
	}
	data, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal game snapshot: %w", err)
	}
	return data, nil
}

// RestoreMatch rebuilds a Match from Snapshot output. A new random stream is
// used for future rounds, while the saved game state and current turn deadline
// are preserved.
func RestoreMatch(data []byte, cfg Config) (*Match, error) {
	if len(data) == 0 || len(data) > maxSnapshotSize {
		return nil, errors.New("game snapshot is empty or exceeds the size limit")
	}
	var state snapshotState
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return nil, fmt.Errorf("decode game snapshot: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("decode game snapshot: trailing JSON value")
		}
		return nil, fmt.Errorf("decode game snapshot trailer: %w", err)
	}
	if state.Version != snapshotVersion {
		return nil, fmt.Errorf("unsupported game snapshot version %d", state.Version)
	}
	if state.TurnDuration <= 0 || len(state.Players) != NumPlayers {
		return nil, errors.New("invalid game snapshot: duration or player count")
	}
	if state.Round < 0 || state.Round > TotalRounds || state.Turn < 0 || state.Turn >= NumPlayers {
		return nil, errors.New("invalid game snapshot: round or turn")
	}
	if !validPhase(state.Phase) {
		return nil, fmt.Errorf("invalid game snapshot phase %q", state.Phase)
	}

	ids := make([]PlayerID, NumPlayers)
	for i, player := range state.Players {
		ids[i] = player.ID
	}
	cfg.TurnDuration = state.TurnDuration
	match, err := NewMatch(ids, cfg)
	if err != nil {
		return nil, fmt.Errorf("restore game snapshot players: %w", err)
	}

	g := match.g
	g.round = state.Round
	g.phase = state.Phase
	g.turn = state.Turn
	g.firstTurn = state.FirstTurn
	g.hidden = append([]Card(nil), state.Hidden...)
	g.discard = append([]Card(nil), state.Discard...)
	g.discardAvailable = state.DiscardAvailable
	g.lastDiscarder = state.LastDiscarder
	g.banks = make([]*DepositBank, len(state.Banks))
	for i, bank := range state.Banks {
		if bank == nil {
			return nil, errors.New("invalid game snapshot: nil deposit bank")
		}
		copy := bank.clone()
		g.banks[i] = &copy
	}
	g.turnStarted = state.TurnStarted
	g.results = make([]RoundResult, len(state.Results))
	for i, result := range state.Results {
		g.results[i] = result.clone()
	}
	g.final = state.Final.clone()
	g.seq = state.Sequence
	g.pending = nil
	for i, player := range state.Players {
		g.players[i].hand = append([]Card(nil), player.Hand...)
		g.players[i].laidDown = player.LaidDown
		g.players[i].connected = player.Connected
		g.players[i].total = player.Total
		g.players[i].roundsWon = player.RoundsWon
		g.players[i].lastRound = player.LastRound
	}
	if err := g.validateSnapshot(); err != nil {
		return nil, fmt.Errorf("invalid game snapshot: %w", err)
	}
	return match, nil
}

func validPhase(phase Phase) bool {
	switch phase {
	case PhaseWaiting, PhaseAwaitingDraw, PhaseAwaitingDiscard, PhaseRoundOver, PhaseGameOver:
		return true
	default:
		return false
	}
}

func (g *Game) validateSnapshot() error {
	if g.phase == PhaseWaiting && g.round != 0 {
		return errors.New("waiting game has started round")
	}
	if g.phase == PhaseWaiting && len(g.results) != 0 {
		return errors.New("waiting game has round results")
	}
	if g.phase != PhaseWaiting && g.round == 0 {
		return errors.New("started game has no round")
	}
	if g.phase == PhaseGameOver && (g.final == nil || g.round != TotalRounds) {
		return errors.New("finished game is missing final result")
	}
	if g.phase != PhaseGameOver && g.final != nil {
		return errors.New("unfinished game has final result")
	}
	if len(g.results) != g.round && g.phase != PhaseWaiting {
		expected := g.round - 1
		if g.phase == PhaseRoundOver || g.phase == PhaseGameOver {
			expected = g.round
		}
		if len(g.results) != expected {
			return errors.New("round results do not match game phase")
		}
	}
	if g.lastDiscarder < -1 || g.lastDiscarder >= NumPlayers {
		return errors.New("invalid last discarder")
	}

	if g.phase == PhaseWaiting {
		return nil
	}
	cards := make([]bool, DeckSize)
	addCards := func(hand []Card) error {
		for _, card := range hand {
			if card.ID < 0 || int(card.ID) >= DeckSize {
				return fmt.Errorf("card id %d out of range", card.ID)
			}
			expected := NewDeck()[int(card.ID)]
			if expected != card {
				return fmt.Errorf("card %d has invalid attributes", card.ID)
			}
			if cards[card.ID] {
				return fmt.Errorf("card %d appears more than once", card.ID)
			}
			cards[card.ID] = true
		}
		return nil
	}
	for _, player := range g.players {
		if err := addCards(player.hand); err != nil {
			return err
		}
		if player.total < 0 || player.roundsWon < 0 || player.roundsWon > g.round {
			return errors.New("invalid player statistics")
		}
	}
	if err := addCards(g.hidden); err != nil {
		return err
	}
	if err := addCards(g.discard); err != nil {
		return err
	}
	for _, bank := range g.banks {
		for _, card := range bank.Cards {
			if err := addCards([]Card{card}); err != nil {
				return err
			}
		}
	}
	for _, present := range cards {
		if !present {
			return errors.New("snapshot does not contain all deck cards")
		}
	}
	return nil
}
