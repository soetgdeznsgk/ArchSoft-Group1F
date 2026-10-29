// Package chameleon implementa el modo de juego "Camaleón": un repartidor
// automático deja cartas en posiciones aleatorias de la mesa y los jugadores
// compiten por tomarlas. Gana quien reúne los 8 colores sin repetir.
//
// Reglas completas: game-logic/docs/MODO_CAMALEON.md.
//
// El motor es puro (sin red ni persistencia) y autoritativo: valida cada
// acción contra el estado. Match es seguro para uso concurrente; ante dos
// jugadores que toman la misma carta, gana quien adquiere primero el mutex.
package chameleon

import (
	"cmp"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"sync"
	"time"
)

// Mode es el identificador del modo en el Lobby, en Redis y en las vistas.
const Mode = "CHAMELEON"

const (
	MinPlayers = 2
	MaxPlayers = 4
	NumColors  = 8
)

// Color de una carta. Las cartas normales tienen uno de los 8 colores; la
// carta camaleón usa el valor especial ChameleonColor.
type Color string

const (
	Red    Color = "red"
	Orange Color = "orange"
	Yellow Color = "yellow"
	Green  Color = "green"
	Cyan   Color = "cyan"
	Blue   Color = "blue"
	Purple Color = "purple"
	Pink   Color = "pink"

	ChameleonColor Color = "chameleon"
)

// Colors son los 8 colores que hay que reunir, en orden de presentación.
var Colors = []Color{Red, Orange, Yellow, Green, Cyan, Blue, Purple, Pink}

func validColor(c Color) bool { return slices.Contains(Colors, c) }

// Card es una carta. Los IDs son únicos dentro de la partida.
type Card struct {
	ID    int   `json:"id"`
	Color Color `json:"color"`
}

// IsChameleon indica si la carta es el camaleón.
func (c Card) IsChameleon() bool { return c.Color == ChameleonColor }

// BoardCard es una carta tirada sobre la mesa. X e Y están normalizadas en
// [0, 1] respecto al tamaño de la mesa; cada cliente las escala a su pantalla.
type BoardCard struct {
	Card
	X         float64   `json:"x"`
	Y         float64   `json:"y"`
	SpawnedAt time.Time `json:"spawned_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Phase es el estado de la partida.
type Phase string

const (
	PhaseWaiting  Phase = "waiting"
	PhasePlaying  Phase = "playing"
	PhaseGameOver Phase = "game_over"
)

// EndReason indica cómo terminó la partida.
type EndReason string

const (
	EndAllColors EndReason = "all_colors" // alguien reunió los 8 colores
	EndTimeUp    EndReason = "time_up"    // se acabó el tiempo de la partida
)

// Clock abstrae el reloj para probar el repartidor y los plazos.
type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// NoChameleons desactiva la carta camaleón (útil en pruebas).
const NoChameleons = -1.0

// Config parametriza el repartidor. Los valores cero usan los predeterminados.
type Config struct {
	GameDuration    time.Duration // duración máxima de la partida (3 min)
	SpawnMin        time.Duration // espera mínima entre cartas (0,8 s)
	SpawnMax        time.Duration // espera máxima entre cartas (2,5 s)
	CardLifetimeMin time.Duration // vida mínima de una carta en la mesa (0,8 s)
	CardLifetimeMax time.Duration // vida máxima de una carta en la mesa (2 s); cada carta dura un tiempo aleatorio entre ambos
	MaxBoardCards   int           // cartas simultáneas en la mesa (6)
	ChameleonChance float64       // probabilidad de que una carta sea camaleón (0,12); NoChameleons = 0
	Rand            *rand.Rand
	Clock           Clock
}

// Valores predeterminados del repartidor.
const (
	DefaultGameDuration    = 3 * time.Minute
	DefaultSpawnMin        = 800 * time.Millisecond
	DefaultSpawnMax        = 2500 * time.Millisecond
	DefaultCardLifetimeMin = 800 * time.Millisecond
	DefaultCardLifetimeMax = 2 * time.Second
	DefaultMaxBoardCards   = 6
	DefaultChameleonChance = 0.12
)

func (c Config) withDefaults() (Config, error) {
	if c.GameDuration == 0 {
		c.GameDuration = DefaultGameDuration
	}
	if c.SpawnMin == 0 {
		c.SpawnMin = DefaultSpawnMin
	}
	if c.SpawnMax == 0 {
		c.SpawnMax = max(DefaultSpawnMax, c.SpawnMin)
	}
	if c.CardLifetimeMax == 0 {
		c.CardLifetimeMax = max(DefaultCardLifetimeMax, c.CardLifetimeMin)
	}
	if c.CardLifetimeMin == 0 {
		c.CardLifetimeMin = min(DefaultCardLifetimeMin, c.CardLifetimeMax)
	}
	if c.MaxBoardCards == 0 {
		c.MaxBoardCards = DefaultMaxBoardCards
	}
	switch {
	case c.ChameleonChance == 0:
		c.ChameleonChance = DefaultChameleonChance
	case c.ChameleonChance < 0:
		c.ChameleonChance = 0
	}
	if c.GameDuration < 0 || c.SpawnMin < 0 || c.SpawnMax < c.SpawnMin ||
		c.CardLifetimeMin <= 0 || c.CardLifetimeMax < c.CardLifetimeMin ||
		c.MaxBoardCards < 0 || c.ChameleonChance > 1 {
		return c, fmt.Errorf("configuración del repartidor inválida")
	}
	if c.Rand == nil {
		c.Rand = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	if c.Clock == nil {
		c.Clock = systemClock{}
	}
	return c, nil
}

// Standing es la posición final de un jugador.
type Standing struct {
	Player string `json:"player"`
	Colors int    `json:"colors"` // colores distintos en la mano al terminar
	Rank   int    `json:"rank"`   // 1 = ganador; empates comparten posición
}

// FinalResult es el resultado de la partida.
type FinalResult struct {
	Reason    EndReason  `json:"reason"`
	Winners   []string   `json:"winners"`
	Standings []Standing `json:"standings"`
}

func (f *FinalResult) clone() *FinalResult {
	if f == nil {
		return nil
	}
	return &FinalResult{Reason: f.Reason, Winners: slices.Clone(f.Winners), Standings: slices.Clone(f.Standings)}
}

type player struct {
	id        string
	hand      []Card
	connected bool
}

func (p *player) indexOfColor(c Color) int {
	return slices.IndexFunc(p.hand, func(card Card) bool { return card.Color == c })
}

// Match es el estado autoritativo de una partida del modo Camaleón.
type Match struct {
	mu  sync.Mutex
	cfg Config

	players []*player // en orden de asiento
	seat    map[string]int

	phase      Phase
	board      []BoardCard
	nextCardID int
	nextSpawn  time.Time
	startedAt  time.Time
	endsAt     time.Time
	final      *FinalResult

	seq     uint64
	pending []Event
}

// New crea una partida de 2 a 4 jugadores. El orden de ids es el orden de los asientos.
func New(ids []string, cfg Config) (*Match, error) {
	if len(ids) < MinPlayers || len(ids) > MaxPlayers {
		return nil, newErrorf(CodeInvalidPlayers, "el modo camaleón admite de %d a %d jugadores (hay %d)",
			MinPlayers, MaxPlayers, len(ids))
	}
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, newError(CodeInvalidPlayers, err.Error())
	}
	m := &Match{cfg: cfg, seat: make(map[string]int, len(ids)), phase: PhaseWaiting}
	for i, id := range ids {
		if id == "" {
			return nil, newError(CodeInvalidPlayers, "el identificador del jugador no puede estar vacío")
		}
		if _, dup := m.seat[id]; dup {
			return nil, newErrorf(CodeInvalidPlayers, "jugador repetido: %s", id)
		}
		m.seat[id] = i
		m.players = append(m.players, &player{id: id, connected: true})
	}
	return m, nil
}

// Start inicia la partida: arranca el reloj y programa la primera carta.
func (m *Match) Start() ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.phase != PhaseWaiting {
		return nil, ErrAlreadyStarted
	}
	now := m.cfg.Clock.Now()
	m.phase = PhasePlaying
	m.startedAt = now
	m.endsAt = now.Add(m.cfg.GameDuration)
	m.nextSpawn = now.Add(m.cfg.SpawnMin)
	m.emit(Event{Type: EvGameStarted})
	return m.flush(), nil
}

// Take intenta tomar una carta de la mesa. Si dos jugadores la piden a la
// vez, la recibe el primero que procesa el árbitro; el otro recibe
// card_not_available.
func (m *Match) Take(playerID string, cardID int) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i, ok := m.seat[playerID]
	if !ok {
		return nil, ErrUnknownPlayer
	}
	switch m.phase {
	case PhaseWaiting:
		return nil, ErrNotStarted
	case PhaseGameOver:
		return nil, ErrGameOver
	}
	// Una carta vencida ya no se puede tomar aunque el tick aún no la haya
	// retirado (el tick la retira y difunde card_expired).
	now := m.cfg.Clock.Now()
	if !now.Before(m.endsAt) {
		return nil, ErrGameOver // el próximo tick cierra la partida por tiempo
	}
	idx := slices.IndexFunc(m.board, func(b BoardCard) bool { return b.ID == cardID })
	if idx < 0 || !now.Before(m.board[idx].ExpiresAt) {
		return nil, ErrCardNotAvailable
	}
	taken := m.board[idx]
	m.board = slices.Delete(m.board, idx, idx+1)
	p := m.players[i]

	switch {
	case taken.IsChameleon():
		lost := p.hand
		p.hand = nil
		m.emit(Event{Type: EvChameleonTaken, Player: p.id, Card: &taken, Lost: lost})
	case p.indexOfColor(taken.Color) >= 0:
		k := p.indexOfColor(taken.Color)
		lost := []Card{p.hand[k], taken.Card}
		p.hand = slices.Delete(p.hand, k, k+1)
		m.emit(Event{Type: EvColorRepeated, Player: p.id, Card: &taken, Lost: lost})
	default:
		p.hand = append(p.hand, taken.Card)
		m.emit(Event{Type: EvCardTaken, Player: p.id, Card: &taken})
		if len(p.hand) == NumColors {
			m.endGame(EndAllColors)
		}
	}
	return m.flush(), nil
}

// Apply despacha una acción serializada por la capa de red.
func (m *Match) Apply(a Action) ([]Event, error) {
	switch a.Type {
	case ActTake:
		return m.Take(a.Player, a.Card)
	}
	return nil, ErrInvalidAction
}

// Tick debe invocarse periódicamente (cada ~250 ms). Termina la partida si se
// acabó el tiempo, retira las cartas vencidas y reparte cuando corresponde.
func (m *Match) Tick() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.phase != PhasePlaying {
		return nil
	}
	now := m.cfg.Clock.Now()
	if !now.Before(m.endsAt) {
		m.endGame(EndTimeUp)
		return m.flush()
	}
	m.expireCards(now)
	if !now.Before(m.nextSpawn) {
		if len(m.board) < m.cfg.MaxBoardCards {
			m.spawn(now)
		}
		m.nextSpawn = now.Add(m.spawnDelay())
	}
	return m.flush()
}

func (m *Match) spawnDelay() time.Duration {
	return m.randomBetween(m.cfg.SpawnMin, m.cfg.SpawnMax)
}

// cardLifetime elige cuánto dura una carta en la mesa. Al ser aleatorio, los
// jugadores no pueden confiarse: premia reaccionar rápido.
func (m *Match) cardLifetime() time.Duration {
	return m.randomBetween(m.cfg.CardLifetimeMin, m.cfg.CardLifetimeMax)
}

func (m *Match) randomBetween(lo, hi time.Duration) time.Duration {
	if hi <= lo {
		return lo
	}
	return lo + time.Duration(m.cfg.Rand.Int64N(int64(hi-lo)+1))
}

// spawn tira una carta nueva en una posición aleatoria, intentando que no se
// encime con las que ya están en la mesa.
func (m *Match) spawn(now time.Time) {
	color := Colors[m.cfg.Rand.IntN(NumColors)]
	if m.cfg.Rand.Float64() < m.cfg.ChameleonChance {
		color = ChameleonColor
	}
	const margin, minDistance = 0.08, 0.18
	var x, y float64
	for attempt := 0; attempt < 12; attempt++ {
		x = margin + m.cfg.Rand.Float64()*(1-2*margin)
		y = margin + m.cfg.Rand.Float64()*(1-2*margin)
		if !m.crowded(x, y, minDistance) {
			break
		}
	}
	card := BoardCard{
		Card: Card{ID: m.nextCardID, Color: color}, X: round3(x), Y: round3(y),
		SpawnedAt: now, ExpiresAt: now.Add(m.cardLifetime()),
	}
	m.nextCardID++
	m.board = append(m.board, card)
	m.emit(Event{Type: EvCardSpawned, Card: &card})
}

func (m *Match) crowded(x, y, distance float64) bool {
	for _, b := range m.board {
		if math.Hypot(b.X-x, b.Y-y) < distance {
			return true
		}
	}
	return false
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

func (m *Match) expireCards(now time.Time) {
	m.board = slices.DeleteFunc(m.board, func(b BoardCard) bool {
		if now.Before(b.ExpiresAt) {
			return false
		}
		expired := b
		m.emit(Event{Type: EvCardExpired, Card: &expired})
		return true
	})
}

// endGame cierra la partida y calcula las posiciones: más colores distintos
// primero; empates comparten posición. Por tiempo, ganan quienes tengan más
// colores (nadie gana si todos tienen 0).
func (m *Match) endGame(reason EndReason) {
	st := make([]Standing, len(m.players))
	for i, p := range m.players {
		st[i] = Standing{Player: p.id, Colors: len(p.hand)}
	}
	slices.SortStableFunc(st, func(a, b Standing) int { return cmp.Compare(b.Colors, a.Colors) })
	res := &FinalResult{Reason: reason, Standings: st, Winners: []string{}}
	for i := range st {
		if i > 0 && st[i].Colors == st[i-1].Colors {
			st[i].Rank = st[i-1].Rank
		} else {
			st[i].Rank = i + 1
		}
		if st[i].Rank == 1 && st[i].Colors > 0 {
			res.Winners = append(res.Winners, st[i].Player)
		}
	}
	m.phase = PhaseGameOver
	m.board = nil
	m.final = res
	m.emit(Event{Type: EvGameEnded, Final: res.clone()})
}

// SetConnected registra conexión o desconexión. En este modo no hay turnos:
// un jugador desconectado simplemente no toma cartas.
func (m *Match) SetConnected(playerID string, connected bool) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i, ok := m.seat[playerID]
	if !ok {
		return nil, ErrUnknownPlayer
	}
	p := m.players[i]
	if p.connected == connected {
		return nil, nil
	}
	p.connected = connected
	m.emit(Event{Type: EvPlayerConnection, Player: p.id, Connected: &connected})
	return m.flush(), nil
}

// MarkAllDisconnected marca a todos como desconectados (al restaurar tras un
// reinicio del servicio, los sockets no sobreviven).
func (m *Match) MarkAllDisconnected() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.players {
		if p.connected {
			p.connected = false
			off := false
			m.emit(Event{Type: EvPlayerConnection, Player: p.id, Connected: &off})
		}
	}
	return m.flush()
}

// Phase devuelve la fase actual.
func (m *Match) Phase() Phase {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.phase
}

// Players devuelve los jugadores en orden de asiento.
func (m *Match) Players() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, len(m.players))
	for i, p := range m.players {
		ids[i] = p.id
	}
	return ids
}

// HandSize devuelve la cantidad de cartas de un jugador (0 si no existe).
func (m *Match) HandSize(playerID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i, ok := m.seat[playerID]; ok {
		return len(m.players[i].hand)
	}
	return 0
}

// Final devuelve el resultado (nil si la partida no ha terminado).
func (m *Match) Final() *FinalResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.final.clone()
}

func (m *Match) emit(e Event) {
	m.seq++
	e.Seq = m.seq
	m.pending = append(m.pending, e)
}

func (m *Match) flush() []Event {
	evs := m.pending
	m.pending = nil
	return evs
}
