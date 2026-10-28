package chameleon

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

var players = []string{"ana", "beto", "caro"}

func newTest(t *testing.T, cfg Config) (*Match, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	cfg.Clock = clk
	if cfg.Rand == nil {
		cfg.Rand = rand.New(rand.NewPCG(1, 2))
	}
	m, err := New(players, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return m, clk
}

func started(t *testing.T, cfg Config) (*Match, *fakeClock) {
	t.Helper()
	m, clk := newTest(t, cfg)
	if _, err := m.Start(); err != nil {
		t.Fatal(err)
	}
	return m, clk
}

// place pone una carta en la mesa a mano (prueba de caja blanca).
func place(m *Match, color Color) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.cfg.Clock.Now()
	id := m.nextCardID
	m.nextCardID++
	m.board = append(m.board, BoardCard{Card: Card{ID: id, Color: color}, X: 0.5, Y: 0.5,
		SpawnedAt: now, ExpiresAt: now.Add(m.cfg.CardLifetimeMax)})
	return id
}

func give(m *Match, seat int, colors ...Color) {
	for _, c := range colors {
		id := place(m, c)
		if _, err := m.Take(m.players[seat].id, id); err != nil {
			panic(err)
		}
	}
}

func handColors(m *Match, seat int) []Color {
	var out []Color
	for _, c := range m.players[seat].hand {
		out = append(out, c.Color)
	}
	return out
}

func find(evs []Event, typ EventType) *Event {
	for i := range evs {
		if evs[i].Type == typ {
			return &evs[i]
		}
	}
	return nil
}

func expectCode(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if got := CodeOf(err); got != code {
		t.Fatalf("se esperaba %q, se obtuvo %q (%v)", code, got, err)
	}
}

func TestNewValidatesPlayers(t *testing.T) {
	for _, ids := range [][]string{{"a"}, {"a", "b", "c", "d", "e"}, {"a", "a"}, {"a", ""}} {
		_, err := New(ids, Config{})
		expectCode(t, err, CodeInvalidPlayers)
	}
	for _, n := range []int{2, 3, 4} {
		if _, err := New([]string{"a", "b", "c", "d"}[:n], Config{}); err != nil {
			t.Fatalf("%d jugadores: %v", n, err)
		}
	}
	if _, err := New([]string{"a", "b"}, Config{SpawnMin: 2 * time.Second, SpawnMax: time.Second}); err == nil {
		t.Fatal("SpawnMax < SpawnMin debería fallar")
	}
	m, _ := New([]string{"a", "b"}, Config{})
	if m.cfg.GameDuration != DefaultGameDuration || m.cfg.ChameleonChance != DefaultChameleonChance ||
		m.cfg.MaxBoardCards != DefaultMaxBoardCards || m.Phase() != PhaseWaiting {
		t.Fatal("valores por defecto no aplicados")
	}
}

func TestStartAndActionsBeforeStart(t *testing.T) {
	m, _ := newTest(t, Config{})
	_, err := m.Take("ana", 0)
	expectCode(t, err, CodeNotStarted)
	evs, err := m.Start()
	if err != nil || find(evs, EvGameStarted) == nil || m.Phase() != PhasePlaying {
		t.Fatalf("inicio: %v %v", evs, err)
	}
	_, err = m.Start()
	expectCode(t, err, CodeAlreadyStarted)
	_, err = m.Take("zoe", 0)
	expectCode(t, err, CodeUnknownPlayer)
	_, err = m.Apply(Action{Type: "volar", Player: "ana"})
	expectCode(t, err, CodeInvalidAction)
}

// Regla: tomar un color nuevo lo agrega a la mano.
func TestTakeNewColor(t *testing.T) {
	m, _ := started(t, Config{})
	id := place(m, Red)
	evs, err := m.Apply(Action{Type: ActTake, Player: "beto", Card: id})
	if err != nil {
		t.Fatal(err)
	}
	e := find(evs, EvCardTaken)
	if e == nil || e.Player != "beto" || e.Card.ID != id || e.Card.Color != Red {
		t.Fatalf("evento: %+v", e)
	}
	if !slices.Equal(handColors(m, 1), []Color{Red}) || len(m.board) != 0 {
		t.Fatal("la carta debe pasar de la mesa a la mano")
	}
	_, err = m.Take("ana", id)
	expectCode(t, err, CodeCardNotAvailable)
}

// Regla: tomar un color repetido hace perder ambas cartas.
func TestRepeatedColorLosesBoth(t *testing.T) {
	m, _ := started(t, Config{})
	give(m, 0, Red, Blue)
	first := m.players[0].hand[0]
	id := place(m, Red)
	evs, err := m.Take("ana", id)
	if err != nil {
		t.Fatal(err)
	}
	e := find(evs, EvColorRepeated)
	if e == nil || len(e.Lost) != 2 || e.Lost[0].ID != first.ID || e.Lost[1].ID != id {
		t.Fatalf("evento: %+v", e)
	}
	if !slices.Equal(handColors(m, 0), []Color{Blue}) {
		t.Fatalf("mano: %v", handColors(m, 0))
	}
	// Puede volver a conseguir el rojo.
	give(m, 0, Red)
	if !slices.Equal(handColors(m, 0), []Color{Blue, Red}) {
		t.Fatalf("mano: %v", handColors(m, 0))
	}
}

// Regla: el camaleón hace perder todas las cartas.
func TestChameleonLosesEverything(t *testing.T) {
	m, _ := started(t, Config{})
	give(m, 2, Red, Green, Pink)
	id := place(m, ChameleonColor)
	evs, err := m.Take("caro", id)
	if err != nil {
		t.Fatal(err)
	}
	e := find(evs, EvChameleonTaken)
	if e == nil || len(e.Lost) != 3 || !e.Card.IsChameleon() {
		t.Fatalf("evento: %+v", e)
	}
	if len(m.players[2].hand) != 0 {
		t.Fatal("debe quedar sin cartas")
	}
	// Con la mano vacía el camaleón no hace nada más.
	evs, _ = m.Take("caro", place(m, ChameleonColor))
	if e := find(evs, EvChameleonTaken); e == nil || len(e.Lost) != 0 {
		t.Fatalf("evento: %+v", e)
	}
}

// Regla: quien reúne los 8 colores gana de inmediato.
func TestEightColorsWins(t *testing.T) {
	m, _ := started(t, Config{})
	give(m, 1, Green, Blue)
	give(m, 0, Colors[:7]...)
	if m.Phase() != PhasePlaying {
		t.Fatal("con 7 colores aún no gana")
	}
	evs, err := m.Take("ana", place(m, Colors[7]))
	if err != nil {
		t.Fatal(err)
	}
	ge := find(evs, EvGameEnded)
	if ge == nil || ge.Final.Reason != EndAllColors || !slices.Equal(ge.Final.Winners, []string{"ana"}) {
		t.Fatalf("fin: %+v", ge)
	}
	f := m.Final()
	want := []Standing{{"ana", 8, 1}, {"beto", 2, 2}, {"caro", 0, 3}}
	if !slices.Equal(f.Standings, want) {
		t.Fatalf("posiciones: %+v", f.Standings)
	}
	if m.Phase() != PhaseGameOver || len(m.board) != 0 {
		t.Fatal("la partida termina y la mesa se vacía")
	}
	_, err = m.Take("beto", 0)
	expectCode(t, err, CodeGameOver)
	if m.Tick() != nil {
		t.Fatal("Tick no actúa al terminar")
	}
}

// Fin por tiempo: ganan quienes tengan más colores; empates comparten.
func TestTimeUp(t *testing.T) {
	m, clk := started(t, Config{GameDuration: time.Minute})
	give(m, 0, Red, Blue)
	give(m, 1, Green, Pink)
	give(m, 2, Cyan)
	clk.Advance(time.Minute)
	_, err := m.Take("ana", place(m, Yellow))
	expectCode(t, err, CodeGameOver)
	evs := m.Tick()
	ge := find(evs, EvGameEnded)
	if ge == nil || ge.Final.Reason != EndTimeUp || !slices.Equal(ge.Final.Winners, []string{"ana", "beto"}) {
		t.Fatalf("fin: %+v", ge)
	}
	if ranks := []int{ge.Final.Standings[0].Rank, ge.Final.Standings[1].Rank, ge.Final.Standings[2].Rank}; !slices.Equal(ranks, []int{1, 1, 3}) {
		t.Fatalf("posiciones: %v", ranks)
	}
}

func TestTimeUpWithNobodyHoldingCards(t *testing.T) {
	m, clk := started(t, Config{GameDuration: time.Second})
	clk.Advance(time.Second)
	ge := find(m.Tick(), EvGameEnded)
	if ge == nil || len(ge.Final.Winners) != 0 || ge.Final.Standings[0].Rank != 1 || ge.Final.Standings[2].Rank != 1 {
		t.Fatalf("fin: %+v", ge)
	}
}

// Repartidor: respeta el intervalo, el máximo de cartas y el vencimiento.
func TestDealerSpawnsExpiresAndCaps(t *testing.T) {
	m, clk := started(t, Config{SpawnMin: time.Second, SpawnMax: time.Second,
		CardLifetimeMin: 3 * time.Second, CardLifetimeMax: 3 * time.Second,
		MaxBoardCards: 2, ChameleonChance: NoChameleons})
	if evs := m.Tick(); len(evs) != 0 {
		t.Fatal("antes de SpawnMin no reparte")
	}
	clk.Advance(time.Second)
	sp := find(m.Tick(), EvCardSpawned)
	if sp == nil || sp.Card.ID != 0 || sp.Card.IsChameleon() || sp.Card.X < 0.08 || sp.Card.X > 0.92 ||
		sp.Card.Y < 0.08 || sp.Card.Y > 0.92 || !sp.Card.ExpiresAt.Equal(clk.Now().Add(3*time.Second)) {
		t.Fatalf("carta repartida: %+v", sp)
	}
	clk.Advance(time.Second)
	m.Tick()
	clk.Advance(time.Second)
	if evs := m.Tick(); find(evs, EvCardSpawned) != nil || len(m.board) != 2 {
		t.Fatal("con la mesa llena no reparte")
	}
	clk.Advance(time.Second) // la primera carta cumple 3 s
	evs := m.Tick()
	if ex := find(evs, EvCardExpired); ex == nil || ex.Card.ID != 0 {
		t.Fatalf("vencimiento: %+v", evs)
	}
	if find(evs, EvCardSpawned) == nil || len(m.board) != 2 {
		t.Fatal("tras vencer, hay lugar para otra carta")
	}
}

// Una carta vencida no se puede tomar aunque el tick no la haya retirado.
func TestExpiredCardCannotBeTaken(t *testing.T) {
	m, clk := started(t, Config{CardLifetimeMax: time.Second})
	id := place(m, Red)
	clk.Advance(time.Second)
	_, err := m.Take("ana", id)
	expectCode(t, err, CodeCardNotAvailable)
	if len(m.board) != 1 {
		t.Fatal("una acción rechazada no modifica el estado")
	}
	v, _ := m.ViewFor("ana")
	if len(v.Board) != 0 {
		t.Fatal("la vista no muestra cartas vencidas")
	}
}

func TestSpawnDistributionAndRandomInterval(t *testing.T) {
	m, clk := started(t, Config{SpawnMin: 100 * time.Millisecond, SpawnMax: 300 * time.Millisecond,
		CardLifetimeMax: time.Millisecond, MaxBoardCards: 100, GameDuration: time.Hour})
	colors := map[Color]int{}
	for i := 0; i < 20000; i++ {
		clk.Advance(50 * time.Millisecond)
		for _, e := range m.Tick() {
			if e.Type == EvCardSpawned {
				colors[e.Card.Color]++
			}
		}
	}
	total := 0
	for _, n := range colors {
		total += n
	}
	// 1000 s / 0,2 s de media ≈ 5000 cartas.
	if total < 3500 || total > 6500 {
		t.Fatalf("se repartieron %d cartas", total)
	}
	if len(colors) != NumColors+1 {
		t.Fatalf("colores vistos: %v", colors)
	}
	share := float64(colors[ChameleonColor]) / float64(total)
	if share < 0.08 || share > 0.16 {
		t.Fatalf("proporción de camaleones %.3f", share)
	}
}

// RF-25 adaptado: dos jugadores toman la misma carta; solo uno la obtiene.
func TestConcurrentTakeOnlyOneWins(t *testing.T) {
	for it := 0; it < 300; it++ {
		m, _ := started(t, Config{})
		id := place(m, Purple)
		var wins atomic.Int32
		var wg sync.WaitGroup
		gate := make(chan struct{})
		for _, p := range players {
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				<-gate
				if _, err := m.Take(p, id); err == nil {
					wins.Add(1)
				} else if !errors.Is(err, ErrCardNotAvailable) {
					t.Errorf("rechazo inesperado: %v", err)
				}
			}(p)
		}
		close(gate)
		wg.Wait()
		owners := 0
		for seat := range m.players {
			owners += len(m.players[seat].hand)
		}
		if wins.Load() != 1 || owners != 1 {
			t.Fatalf("iteración %d: %d ganaron, %d dueños", it, wins.Load(), owners)
		}
	}
}

func TestConnectionAndView(t *testing.T) {
	m, clk := started(t, Config{GameDuration: time.Minute})
	give(m, 0, Red)
	evs, err := m.SetConnected("beto", false)
	if err != nil || find(evs, EvPlayerConnection) == nil {
		t.Fatal("desconexión")
	}
	if evs, _ := m.SetConnected("beto", false); evs != nil {
		t.Fatal("repetir el estado no genera eventos")
	}
	_, err = m.SetConnected("zoe", true)
	expectCode(t, err, CodeUnknownPlayer)
	clk.Advance(10 * time.Second)
	id := place(m, Blue)
	clk.Advance(time.Second)
	v, err := m.ViewFor("caro")
	if err != nil {
		t.Fatal(err)
	}
	if v.Mode != Mode || v.You != "caro" || v.RemainingMs != 49_000 || !v.CanTake || len(v.Colors) != 8 {
		t.Fatalf("vista: %+v", v)
	}
	if len(v.Board) != 1 || v.Board[0].ID != id || v.Board[0].ExpiresInMs != 1000 {
		t.Fatalf("mesa: %+v", v.Board)
	}
	// Las manos de todos son públicas.
	if len(v.Players) != 3 || v.Players[0].Colors != 1 || v.Players[0].Cards[0].Color != Red || v.Players[1].Connected {
		t.Fatalf("jugadores: %+v", v.Players)
	}
	v.Players[0].Cards[0].Color = Pink
	if m.players[0].hand[0].Color != Red {
		t.Fatal("la vista debe ser una copia")
	}
	_, err = m.ViewFor("zoe")
	expectCode(t, err, CodeUnknownPlayer)
	evs = m.MarkAllDisconnected()
	if len(evs) != 2 || m.HandSize("ana") != 1 || m.HandSize("zoe") != 0 {
		t.Fatalf("MarkAllDisconnected: %d eventos", len(evs))
	}
}

func TestEventJSONHasModeAndNoEmptyFields(t *testing.T) {
	m, _ := started(t, Config{})
	evs, _ := m.Take("ana", place(m, Green))
	raw, _ := json.Marshal(evs[0])
	s := string(raw)
	if !strings.Contains(s, `"mode":"CHAMELEON"`) || !strings.Contains(s, `"type":"card_taken"`) ||
		!strings.Contains(s, `"color":"green"`) || strings.Contains(s, `"lost"`) || strings.Contains(s, `"final"`) {
		t.Fatalf("JSON: %s", s)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	m, clk := started(t, Config{GameDuration: time.Minute, ChameleonChance: NoChameleons})
	give(m, 0, Red, Blue)
	give(m, 2, Yellow)
	id := place(m, Pink)
	m.SetConnected("beto", false)
	data, err := m.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	r, err := Restore(data, Config{Clock: clk})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := m.ViewFor("ana")
	b, _ := r.ViewFor("ana")
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatalf("vistas distintas:\n%s\n%s", ja, jb)
	}
	if r.cfg.ChameleonChance != 0 || r.cfg.GameDuration != time.Minute || r.seq != m.seq ||
		r.cfg.CardLifetimeMin != DefaultCardLifetimeMin || r.cfg.CardLifetimeMax != DefaultCardLifetimeMax {
		t.Fatalf("config restaurada: %+v", r.cfg)
	}
	if _, err := r.Take("caro", id); err != nil {
		t.Fatal(err)
	}
	// Una partida terminada también se restaura.
	give(r, 0, Orange, Yellow, Green, Cyan, Purple, Pink)
	data, _ = r.Snapshot()
	done, err := Restore(data, Config{Clock: clk})
	if err != nil || done.Phase() != PhaseGameOver || done.Final() == nil {
		t.Fatalf("restaurar terminada: %v", err)
	}
	// Una partida en espera también.
	w, _ := newTest(t, Config{})
	data, _ = w.Snapshot()
	if r, err := Restore(data, Config{}); err != nil || r.Phase() != PhaseWaiting {
		t.Fatalf("restaurar en espera: %v", err)
	}
}

func TestRestoreRejectsInvalidSnapshots(t *testing.T) {
	m, _ := started(t, Config{})
	give(m, 0, Red)
	good, _ := m.Snapshot()
	mutate := func(f func(map[string]any)) []byte {
		var s map[string]any
		_ = json.Unmarshal(good, &s)
		f(s)
		out, _ := json.Marshal(s)
		return out
	}
	cases := map[string][]byte{
		"vacío":             nil,
		"basura":            []byte("{"),
		"campo extra":       mutate(func(s map[string]any) { s["x"] = 1 }),
		"otra versión":      mutate(func(s map[string]any) { s["version"] = 2 }),
		"otro modo":         mutate(func(s map[string]any) { s["mode"] = "CLASSIC" }),
		"fase inválida":     mutate(func(s map[string]any) { s["phase"] = "x" }),
		"final sin fin":     mutate(func(s map[string]any) { s["final"] = map[string]any{"reason": "time_up"} }),
		"un jugador":        mutate(func(s map[string]any) { s["players"] = s["players"].([]any)[:1] }),
		"carta repetida":    mutate(func(s map[string]any) { s["next_card_id"] = 0 }),
		"datos adicionales": append(append([]byte{}, good...), []byte(" {}")...),
		"color inválido": mutate(func(s map[string]any) {
			s["players"].([]any)[0].(map[string]any)["hand"].([]any)[0].(map[string]any)["color"] = "gris"
		}),
		"color duplicado en mano": mutate(func(s map[string]any) {
			hand := s["players"].([]any)[0].(map[string]any)
			hand["hand"] = append(hand["hand"].([]any), map[string]any{"id": 0, "color": "red"})
		}),
	}
	for name, data := range cases {
		if _, err := Restore(data, Config{}); err == nil {
			t.Errorf("%s: se esperaba error", name)
		}
	}
}

func TestErrorHelpers(t *testing.T) {
	if !errors.Is(newError(CodeGameOver, "x"), ErrGameOver) || errors.Is(ErrGameOver, ErrNotStarted) {
		t.Fatal("errors.Is compara por código")
	}
	if CodeOf(errors.New("x")) != "" || ErrGameOver.Error() != "game_over: la partida terminó" {
		t.Fatal("CodeOf / Error")
	}
	if (&Error{}).Is(errors.New("x")) {
		t.Fatal("Is con error ajeno")
	}
}

// Cada carta dura un tiempo aleatorio entre 0,8 y 2 s: nunca más de 2 s.
func TestCardLifetimeIsRandomAndAtMostTwoSeconds(t *testing.T) {
	m, clk := started(t, Config{SpawnMin: 10 * time.Millisecond, SpawnMax: 10 * time.Millisecond,
		MaxBoardCards: 1000, GameDuration: time.Hour})
	if m.cfg.CardLifetimeMin != 800*time.Millisecond || m.cfg.CardLifetimeMax != 2*time.Second {
		t.Fatalf("valores por defecto: %v a %v", m.cfg.CardLifetimeMin, m.cfg.CardLifetimeMax)
	}
	var shortest, longest time.Duration = time.Hour, 0
	distinct := map[time.Duration]bool{}
	for i := 0; i < 2000; i++ {
		clk.Advance(10 * time.Millisecond)
		for _, e := range m.Tick() {
			if e.Type != EvCardSpawned {
				continue
			}
			life := e.Card.ExpiresAt.Sub(e.Card.SpawnedAt)
			shortest, longest = min(shortest, life), max(longest, life)
			distinct[life] = true
		}
	}
	if shortest < 800*time.Millisecond || longest > 2*time.Second {
		t.Fatalf("vida fuera de rango: %v a %v", shortest, longest)
	}
	if len(distinct) < 100 || longest-shortest < time.Second {
		t.Fatalf("la vida no es aleatoria: %d valores distintos entre %v y %v", len(distinct), shortest, longest)
	}
	// Configuración inválida: mínimo mayor que el máximo.
	if _, err := New(players, Config{CardLifetimeMin: 3 * time.Second, CardLifetimeMax: time.Second}); err == nil {
		t.Fatal("CardLifetimeMin > CardLifetimeMax debería fallar")
	}
}

// Las partidas guardadas antes de la vida aleatoria se restauran con su vida fija.
func TestRestoreLegacySnapshotKeepsFixedLifetime(t *testing.T) {
	m, _ := started(t, Config{})
	data, _ := m.Snapshot()
	var s map[string]any
	_ = json.Unmarshal(data, &s)
	cfg := s["config"].(map[string]any)
	delete(cfg, "card_lifetime_min")
	cfg["card_lifetime"] = float64(5 * time.Second)
	legacy, _ := json.Marshal(s)
	r, err := Restore(legacy, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if r.cfg.CardLifetimeMin != 5*time.Second || r.cfg.CardLifetimeMax != 5*time.Second {
		t.Fatalf("vida restaurada: %v a %v", r.cfg.CardLifetimeMin, r.cfg.CardLifetimeMax)
	}
}
