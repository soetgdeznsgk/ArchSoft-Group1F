package game

import (
	"sync"
	"time"
)

// Match envuelve una Game y serializa todas las operaciones con un mutex.
// Es el árbitro único de la partida (RNF-02, RNF-03): ante acciones
// simultáneas (dos robos, o un robo y la toma del jugador en turno) la que
// adquiere primero el árbitro se aplica completa y las demás se validan contra
// el estado resultante, por lo que se rechazan con un mensaje informativo.
//
// Cada Match es independiente: varias partidas pueden ejecutarse en paralelo
// sin compartir estado (RNF-05).
type Match struct {
	mu sync.Mutex
	g  *Game
}

// NewMatch crea una partida protegida para uso concurrente.
func NewMatch(ids []PlayerID, cfg Config) (*Match, error) {
	g, err := New(ids, cfg)
	if err != nil {
		return nil, err
	}
	return &Match{g: g}, nil
}

func (m *Match) Start() ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.g.Start()
}

func (m *Match) StartNextRound() ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.g.StartNextRound()
}

func (m *Match) Apply(a Action) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.g.Apply(a)
}

func (m *Match) Tick() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.g.Tick()
}

func (m *Match) SetConnected(p PlayerID, connected bool) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.g.SetConnected(p, connected)
}

func (m *Match) ViewFor(p PlayerID) (PlayerView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.g.ViewFor(p)
}

func (m *Match) Phase() Phase {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.g.Phase()
}

func (m *Match) Round() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.g.Round()
}

func (m *Match) CurrentPlayer() PlayerID {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.g.CurrentPlayer()
}

func (m *Match) Players() []PlayerID {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.g.Players()
}

func (m *Match) TurnDeadline() (time.Time, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.g.TurnDeadline()
}

func (m *Match) Results() []RoundResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.g.Results()
}

func (m *Match) Final() *FinalResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.g.Final()
}
