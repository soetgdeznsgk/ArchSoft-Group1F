package game

import (
	"cmp"
	"math/rand/v2"
	"slices"
	"time"
)

const (
	// NumPlayers es la cantidad exacta de jugadores por partida.
	NumPlayers = 4
	// DefaultTurnDuration es el límite de tiempo por turno (RF-17).
	DefaultTurnDuration = 30 * time.Second
)

// PlayerID identifica a un jugador; lo asigna el Lobby Manager.
type PlayerID string

// Phase es el estado de la máquina de estados de la partida.
type Phase string

const (
	PhaseWaiting         Phase = "waiting"          // creada, sin iniciar
	PhaseAwaitingDraw    Phase = "awaiting_draw"    // el jugador en turno puede bajar, depositar y debe tomar carta
	PhaseAwaitingDiscard Phase = "awaiting_discard" // el jugador en turno ya tomó y debe botar
	PhaseRoundOver       Phase = "round_over"       // ronda cerrada, falta StartNextRound
	PhaseGameOver        Phase = "game_over"        // partida terminada
)

// Source es el banco desde el que se toma una carta.
type Source string

const (
	SourceHidden  Source = "hidden"  // banco de cartas ocultas
	SourceDiscard Source = "discard" // banco de cartas vistas (solo la última botada)
)

// Clock abstrae el reloj para poder probar los límites de tiempo.
type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// Config parametriza una partida. Los valores cero usan los predeterminados.
type Config struct {
	TurnDuration time.Duration // 30 s por defecto
	Rand         *rand.Rand    // fuente de azar (barajado y jugador inicial); aleatoria por defecto
	Clock        Clock         // reloj del sistema por defecto
}

// DepositBank es una jugada bajada sobre la mesa que acepta depósitos (RF-28, RF-29).
type DepositBank struct {
	ID     int      `json:"id"`
	Owner  PlayerID `json:"owner"`
	Figure Figure   `json:"figure"`
	Cards  []Card   `json:"cards"`
}

func (b *DepositBank) accepts(c Card) bool { return c.Joker || c.Figure == b.Figure }

func (b *DepositBank) clone() DepositBank {
	cp := *b
	cp.Cards = slices.Clone(b.Cards)
	return cp
}

// EndReason indica cómo terminó una ronda.
type EndReason string

const (
	EndPlayerWentOut   EndReason = "player_went_out"       // un jugador quedó sin cartas (RF-30)
	EndHiddenExhausted EndReason = "hidden_bank_exhausted" // se agotó el banco de ocultas (RF-31)
)

// RoundResult es el cierre de una ronda (RF-32).
type RoundResult struct {
	Round  int              `json:"round"`
	Winner PlayerID         `json:"winner,omitempty"` // vacío si no hubo ganador
	Reason EndReason        `json:"reason"`
	Points map[PlayerID]int `json:"points"`
}

func (r RoundResult) clone() RoundResult {
	pts := make(map[PlayerID]int, len(r.Points))
	for k, v := range r.Points {
		pts[k] = v
	}
	r.Points = pts
	return r
}

// Standing es la posición final de un jugador.
type Standing struct {
	Player          PlayerID `json:"player"`
	Points          int      `json:"points"`
	RoundsWon       int      `json:"rounds_won"`
	LastRoundPoints int      `json:"last_round_points"`
	Rank            int      `json:"rank"` // 1 = ganador; empates comparten posición
}

// FinalResult es el resultado de la partida (RF-35).
type FinalResult struct {
	Standings []Standing `json:"standings"`
	Winners   []PlayerID `json:"winners"` // más de uno solo si persiste el empate
}

func (f *FinalResult) clone() *FinalResult {
	if f == nil {
		return nil
	}
	return &FinalResult{Standings: slices.Clone(f.Standings), Winners: slices.Clone(f.Winners)}
}

type player struct {
	id        PlayerID
	hand      []Card
	laidDown  bool
	connected bool
	total     int // puntos negativos acumulados
	roundsWon int
	lastRound int // puntos de la última ronda jugada
}

// Game es el estado autoritativo de una partida. No es seguro para uso
// concurrente: use Match.
type Game struct {
	turnDuration time.Duration
	rng          *rand.Rand
	clock        Clock

	players []*player // en orden de asiento; el turno avanza hacia la derecha (índice + 1)
	seat    map[PlayerID]int

	round            int
	phase            Phase
	turn             int
	firstTurn        bool // turno inicial de la ronda: solo intercambio (RF-15)
	hidden           []Card
	discard          []Card // la última botada está al final
	discardAvailable bool   // la última botada puede tomarse o robarse
	lastDiscarder    int    // asiento de quien botó la última carta; -1 si ninguno
	banks            []*DepositBank
	turnStarted      time.Time

	results []RoundResult
	final   *FinalResult

	seq     uint64
	pending []Event
}

// New crea una partida para exactamente 4 jugadores. El orden de ids es el
// orden de los asientos en la mesa.
func New(ids []PlayerID, cfg Config) (*Game, error) {
	if len(ids) != NumPlayers {
		return nil, newErrorf(CodeInvalidPlayers,
			"la partida requiere exactamente %d jugadores (hay %d)", NumPlayers, len(ids))
	}
	seat := make(map[PlayerID]int, NumPlayers)
	players := make([]*player, 0, NumPlayers)
	for i, id := range ids {
		if id == "" {
			return nil, newError(CodeInvalidPlayers, "el identificador del jugador no puede estar vacío")
		}
		if _, dup := seat[id]; dup {
			return nil, newErrorf(CodeInvalidPlayers, "jugador repetido: %s", id)
		}
		seat[id] = i
		players = append(players, &player{id: id, connected: true})
	}
	if cfg.TurnDuration <= 0 {
		cfg.TurnDuration = DefaultTurnDuration
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	if cfg.Clock == nil {
		cfg.Clock = systemClock{}
	}
	return &Game{
		turnDuration:  cfg.TurnDuration,
		rng:           cfg.Rand,
		clock:         cfg.Clock,
		players:       players,
		seat:          seat,
		phase:         PhaseWaiting,
		lastDiscarder: -1,
	}, nil
}

// Start inicia la ronda 1 con un jugador inicial al azar (RF-12, RF-14).
func (g *Game) Start() ([]Event, error) {
	if g.phase != PhaseWaiting {
		return nil, ErrGameAlreadyStarted
	}
	g.round = 1
	g.startRound(g.rng.IntN(NumPlayers))
	g.runAutoTurns()
	return g.flush(), nil
}

// StartNextRound inicia la siguiente ronda tras un cierre. La inicia el
// ganador de la ronda anterior o, si no hubo, un jugador al azar (RF-14).
func (g *Game) StartNextRound() ([]Event, error) {
	switch g.phase {
	case PhaseRoundOver:
	case PhaseWaiting:
		return nil, ErrGameNotStarted
	case PhaseGameOver:
		return nil, ErrGameOver
	default:
		return nil, ErrRoundStillInPlay
	}
	last := g.results[len(g.results)-1]
	var starter int
	if last.Winner != "" {
		starter = g.seat[last.Winner]
	} else {
		starter = g.rng.IntN(NumPlayers)
	}
	g.round++
	g.startRound(starter)
	g.runAutoTurns()
	return g.flush(), nil
}

// startRound baraja las 60 cartas, reparte y prepara los bancos (RF-12, RF-13).
func (g *Game) startRound(starter int) {
	spec, _ := Spec(g.round)
	deck := NewDeck()
	g.rng.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

	for _, p := range g.players {
		p.hand = make([]Card, 0, spec.CardsPerPlayer+2)
		p.laidDown = false
	}
	pos := 0
	for k := 0; k < spec.CardsPerPlayer; k++ {
		for i := 0; i < NumPlayers; i++ {
			p := g.players[(starter+i)%NumPlayers]
			p.hand = append(p.hand, deck[pos])
			pos++
		}
	}
	g.hidden = slices.Clone(deck[pos:])
	g.discard = nil
	g.discardAvailable = false
	g.lastDiscarder = -1
	g.banks = nil
	g.turn = starter
	g.phase = PhaseAwaitingDraw
	g.firstTurn = true
	g.turnStarted = g.clock.Now()

	g.emit(Event{Type: EvRoundStarted, Player: g.players[starter].id, Spec: &spec})
	g.emit(Event{Type: EvTurnStarted, Player: g.players[starter].id})
}

// endRound cierra la ronda y calcula el puntaje (RF-30, RF-31, RF-32).
// winner es el asiento del ganador o -1 si no hubo.
func (g *Game) endRound(winner int, reason EndReason) {
	res := RoundResult{Round: g.round, Reason: reason, Points: make(map[PlayerID]int, NumPlayers)}
	for i, p := range g.players {
		pts := 0
		if i != winner {
			pts = HandPenalty(p.hand)
		}
		p.total += pts
		p.lastRound = pts
		res.Points[p.id] = pts
	}
	if winner >= 0 {
		w := g.players[winner]
		w.roundsWon++
		res.Winner = w.id
	}
	g.results = append(g.results, res)
	evRes := res.clone()
	g.emit(Event{Type: EvRoundEnded, Player: res.Winner, Reason: string(reason), Result: &evRes})

	if g.round == TotalRounds {
		g.phase = PhaseGameOver
		g.final = g.computeFinal()
		g.emit(Event{Type: EvGameEnded, Final: g.final.clone()})
		return
	}
	g.phase = PhaseRoundOver
}

// computeFinal ordena por menos puntos, luego más rondas ganadas y luego
// menos puntos en la última ronda (propuesta 14.1). Si persiste el empate,
// los jugadores comparten la posición.
func (g *Game) computeFinal() *FinalResult {
	st := make([]Standing, NumPlayers)
	for i, p := range g.players {
		st[i] = Standing{Player: p.id, Points: p.total, RoundsWon: p.roundsWon, LastRoundPoints: p.lastRound}
	}
	compare := func(a, b Standing) int {
		if c := cmp.Compare(a.Points, b.Points); c != 0 {
			return c
		}
		if c := cmp.Compare(b.RoundsWon, a.RoundsWon); c != 0 {
			return c
		}
		return cmp.Compare(a.LastRoundPoints, b.LastRoundPoints)
	}
	slices.SortStableFunc(st, compare)
	res := &FinalResult{Standings: st}
	for i := range st {
		if i > 0 && compare(st[i-1], st[i]) == 0 {
			st[i].Rank = st[i-1].Rank
		} else {
			st[i].Rank = i + 1
		}
		if st[i].Rank == 1 {
			res.Winners = append(res.Winners, st[i].Player)
		}
	}
	return res
}

func (g *Game) emit(e Event) {
	g.seq++
	e.Seq = g.seq
	e.Round = g.round
	g.pending = append(g.pending, e)
}

func (g *Game) flush() []Event {
	evs := g.pending
	g.pending = nil
	return evs
}

func (g *Game) inPlay() bool {
	return g.phase == PhaseAwaitingDraw || g.phase == PhaseAwaitingDiscard
}

func (g *Game) popHidden() (Card, bool) {
	n := len(g.hidden)
	if n == 0 {
		return Card{}, false
	}
	c := g.hidden[n-1]
	g.hidden = g.hidden[:n-1]
	return c, true
}

func (g *Game) popDiscard() Card {
	n := len(g.discard)
	c := g.discard[n-1]
	g.discard = g.discard[:n-1]
	return c
}

// Round devuelve la ronda actual (0 antes de iniciar).
func (g *Game) Round() int { return g.round }

// Phase devuelve la fase actual.
func (g *Game) Phase() Phase { return g.phase }

// CurrentPlayer devuelve el jugador en turno ("" si no hay ronda en juego).
func (g *Game) CurrentPlayer() PlayerID {
	if !g.inPlay() {
		return ""
	}
	return g.players[g.turn].id
}

// Players devuelve los jugadores en orden de asiento.
func (g *Game) Players() []PlayerID {
	ids := make([]PlayerID, len(g.players))
	for i, p := range g.players {
		ids[i] = p.id
	}
	return ids
}

// Results devuelve una copia de los resultados de las rondas cerradas.
func (g *Game) Results() []RoundResult {
	out := make([]RoundResult, len(g.results))
	for i, r := range g.results {
		out[i] = r.clone()
	}
	return out
}

// Final devuelve el resultado de la partida (nil si no ha terminado).
func (g *Game) Final() *FinalResult { return g.final.clone() }
