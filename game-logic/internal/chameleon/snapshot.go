package chameleon

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"
)

const (
	snapshotVersion = 1
	maxSnapshotSize = 1 << 20
)

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

type snapshotConfig struct {
	GameDuration    time.Duration `json:"game_duration"`
	SpawnMin        time.Duration `json:"spawn_min"`
	SpawnMax        time.Duration `json:"spawn_max"`
	CardLifetime    time.Duration `json:"card_lifetime"`               // vida máxima (nombre anterior a la vida aleatoria)
	CardLifetimeMin time.Duration `json:"card_lifetime_min,omitempty"` // ausente en snapshots viejos: vida fija
	MaxBoardCards   int           `json:"max_board_cards"`
	ChameleonChance float64       `json:"chameleon_chance"`
}

type snapshotPlayer struct {
	ID        string `json:"id"`
	Hand      []Card `json:"hand"`
	Connected bool   `json:"connected"`
}

type snapshotState struct {
	Version    int              `json:"version"`
	Mode       string           `json:"mode"`
	Config     snapshotConfig   `json:"config"`
	Players    []snapshotPlayer `json:"players"`
	Phase      Phase            `json:"phase"`
	Board      []BoardCard      `json:"board"`
	NextCardID int              `json:"next_card_id"`
	NextSpawn  time.Time        `json:"next_spawn"`
	StartedAt  time.Time        `json:"started_at"`
	EndsAt     time.Time        `json:"ends_at"`
	Final      *FinalResult     `json:"final,omitempty"`
	Sequence   uint64           `json:"sequence"`
}

// Snapshot serializa el estado completo de la partida (JSON versionado).
// Los instantes son absolutos: tras un reinicio, el reloj de la partida sigue
// corriendo y las cartas vencidas se retiran en el siguiente tick.
func (m *Match) Snapshot() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := snapshotState{
		Version: snapshotVersion, Mode: Mode,
		Config: snapshotConfig{
			GameDuration: m.cfg.GameDuration, SpawnMin: m.cfg.SpawnMin, SpawnMax: m.cfg.SpawnMax,
			CardLifetime: m.cfg.CardLifetimeMax, CardLifetimeMin: m.cfg.CardLifetimeMin,
			MaxBoardCards: m.cfg.MaxBoardCards, ChameleonChance: m.cfg.ChameleonChance,
		},
		Phase: m.phase, Board: slices.Clone(m.board), NextCardID: m.nextCardID,
		NextSpawn: m.nextSpawn, StartedAt: m.startedAt, EndsAt: m.endsAt,
		Final: m.final.clone(), Sequence: m.seq,
	}
	for _, p := range m.players {
		s.Players = append(s.Players, snapshotPlayer{ID: p.id, Hand: slices.Clone(p.hand), Connected: p.connected})
	}
	return json.Marshal(s)
}

// Restore reconstruye una partida desde Snapshot. cfg solo aporta Rand y
// Clock; la configuración del repartidor viene del snapshot.
func Restore(data []byte, cfg Config) (*Match, error) {
	if len(data) == 0 || len(data) > maxSnapshotSize {
		return nil, errors.New("snapshot vacío o demasiado grande")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var s snapshotState
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("decodificar snapshot: %w", err)
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("el snapshot contiene datos adicionales")
	}
	if s.Version != snapshotVersion || s.Mode != Mode {
		return nil, fmt.Errorf("snapshot no soportado (versión %d, modo %q)", s.Version, s.Mode)
	}
	ids := make([]string, len(s.Players))
	for i, p := range s.Players {
		ids[i] = p.ID
	}
	cfg.GameDuration, cfg.SpawnMin, cfg.SpawnMax = s.Config.GameDuration, s.Config.SpawnMin, s.Config.SpawnMax
	cfg.CardLifetimeMax, cfg.CardLifetimeMin = s.Config.CardLifetime, s.Config.CardLifetimeMin
	if cfg.CardLifetimeMin == 0 {
		cfg.CardLifetimeMin = cfg.CardLifetimeMax // snapshot anterior: la partida sigue con vida fija
	}
	cfg.MaxBoardCards = s.Config.MaxBoardCards
	cfg.ChameleonChance = s.Config.ChameleonChance
	if cfg.ChameleonChance == 0 {
		cfg.ChameleonChance = NoChameleons
	}
	m, err := New(ids, cfg)
	if err != nil {
		return nil, err
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	for i, p := range s.Players {
		m.players[i].hand = slices.Clone(p.Hand)
		m.players[i].connected = p.Connected
	}
	m.phase, m.board, m.nextCardID = s.Phase, slices.Clone(s.Board), s.NextCardID
	m.nextSpawn, m.startedAt, m.endsAt = s.NextSpawn, s.StartedAt, s.EndsAt
	m.final, m.seq = s.Final.clone(), s.Sequence
	return m, nil
}

func (s *snapshotState) validate() error {
	switch s.Phase {
	case PhaseWaiting, PhasePlaying, PhaseGameOver:
	default:
		return fmt.Errorf("fase inválida %q", s.Phase)
	}
	if (s.Phase == PhaseGameOver) != (s.Final != nil) {
		return errors.New("resultado final inconsistente con la fase")
	}
	seen := map[int]bool{}
	check := func(c Card) error {
		if c.ID < 0 || c.ID >= s.NextCardID || seen[c.ID] {
			return fmt.Errorf("carta %d inválida o repetida", c.ID)
		}
		if !validColor(c.Color) && !c.IsChameleon() {
			return fmt.Errorf("color inválido %q", c.Color)
		}
		seen[c.ID] = true
		return nil
	}
	for _, p := range s.Players {
		colors := map[Color]bool{}
		for _, c := range p.Hand {
			if err := check(c); err != nil {
				return err
			}
			if c.IsChameleon() || colors[c.Color] {
				return fmt.Errorf("mano inválida para %s", p.ID)
			}
			colors[c.Color] = true
		}
		if len(p.Hand) == NumColors && s.Phase != PhaseGameOver {
			return fmt.Errorf("%s tiene los 8 colores con la partida en juego", p.ID)
		}
	}
	for _, b := range s.Board {
		if err := check(b.Card); err != nil {
			return err
		}
	}
	if s.Phase == PhaseWaiting && (len(s.Board) != 0 || s.NextCardID != 0) {
		return errors.New("partida en espera con cartas repartidas")
	}
	return nil
}
