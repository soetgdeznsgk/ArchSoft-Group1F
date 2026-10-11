package game_test

import (
	"math/rand/v2"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/bot"
	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/game"
)

var players = []game.PlayerID{"ana", "beto", "caro", "dani"}

func newMatch(t testing.TB, seed uint64) *game.Match {
	t.Helper()
	m, err := game.NewMatch(players, game.Config{Rand: rand.New(rand.NewPCG(seed, seed+1))})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// Pruebas de propiedades: partidas completas con bots, verificando las
// invariantes del estado después de cada acción.
func TestRandomGamesKeepInvariants(t *testing.T) {
	games := 300
	if testing.Short() {
		games = 40
	}
	var withWinner, withoutWinner, steals int
	for seed := uint64(0); seed < uint64(games); seed++ {
		m := newMatch(t, seed)
		rate := 0.0
		if seed%3 == 0 {
			rate = 0.05 // un tercio de las partidas con desconexiones
		}
		st, err := bot.PlayGame(m, rand.New(rand.NewPCG(seed, 99)), bot.Options{
			DisconnectRate: rate,
			After:          m.CheckInvariants,
		})
		if err != nil {
			t.Fatalf("semilla %d: %v", seed, err)
		}
		if m.Phase() != game.PhaseGameOver || len(m.Results()) != game.TotalRounds {
			t.Fatalf("semilla %d: la partida no terminó", seed)
		}
		checkFinal(t, m)
		withWinner += st.RoundsWithWinner
		withoutWinner += st.RoundsWithoutWinner
		steals += st.Steals
	}
	if withWinner == 0 || withoutWinner == 0 || steals == 0 {
		t.Fatalf("la simulación no ejercitó todos los caminos: ganador=%d sin=%d robos=%d", withWinner, withoutWinner, steals)
	}
	t.Logf("rondas con ganador=%d sin ganador=%d robos=%d", withWinner, withoutWinner, steals)
}

// checkFinal verifica RF-35 contra los resultados por ronda.
func checkFinal(t *testing.T, m *game.Match) {
	t.Helper()
	f := m.Final()
	totals := map[game.PlayerID]int{}
	won := map[game.PlayerID]int{}
	for _, r := range m.Results() {
		for p, pts := range r.Points {
			totals[p] += pts
		}
		if r.Winner != "" {
			won[r.Winner]++
		}
	}
	best := f.Standings[0]
	for _, s := range f.Standings {
		if s.Points != totals[s.Player] || s.RoundsWon != won[s.Player] {
			t.Fatalf("posiciones inconsistentes: %+v", s)
		}
		if s.Points < best.Points || (s.Points == best.Points && s.RoundsWon > best.RoundsWon) {
			t.Fatalf("%s debería ir antes que %s", s.Player, best.Player)
		}
		if (s.Rank == 1) != slices.Contains(f.Winners, s.Player) {
			t.Fatalf("ganadores %v no coinciden con posiciones", f.Winners)
		}
	}
}

// RNF-05: 10 partidas simultáneas no comparten estado.
func TestTenConcurrentMatchesAreIsolated(t *testing.T) {
	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)
	matches := make([]*game.Match, n)
	for i := 0; i < n; i++ {
		matches[i] = newMatch(t, uint64(1000+i))
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = bot.PlayGame(matches[i], rand.New(rand.NewPCG(uint64(i), 7)), bot.Options{After: matches[i].CheckInvariants})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("partida %d: %v", i, err)
		}
	}
	// Repetir la partida 0 de forma aislada produce exactamente el mismo resultado.
	solo := newMatch(t, 1000)
	if _, err := bot.PlayGame(solo, rand.New(rand.NewPCG(0, 7)), bot.Options{}); err != nil {
		t.Fatal(err)
	}
	a, b := matches[0].Final().Standings, solo.Final().Standings
	if !slices.Equal(a, b) {
		t.Fatalf("la ejecución concurrente alteró el resultado: %v vs %v", a, b)
	}
}

// RNF-01 (parte de la lógica): el procesamiento de cada acción debe ser una
// fracción mínima del presupuesto de 200 ms. Se mide el p95 con 10 partidas
// en paralelo y se exige < 5 ms para dejar margen a red y difusión.
func TestActionLatencyBudget(t *testing.T) {
	const n = 10
	var mu sync.Mutex
	var samples []time.Duration
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		m := newMatch(t, uint64(2000+i))
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			local := make([]time.Duration, 0, 4096)
			last := time.Now()
			_, err := bot.PlayGame(m, rand.New(rand.NewPCG(uint64(i), 3)), bot.Options{After: func() error {
				// Mide desde la acción anterior: incluye la decisión del bot,
				// por lo que es una cota superior del costo del motor.
				now := time.Now()
				local = append(local, now.Sub(last))
				last = now
				return nil
			}})
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			samples = append(samples, local...)
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	var sum time.Duration
	for _, s := range samples {
		sum += s
	}
	p95 := samples[len(samples)*95/100]
	// En Windows la resolución del reloj puede hacer que p50 se reporte como 0.
	t.Logf("acciones=%d media=%v p50=%v p95=%v max=%v", len(samples),
		sum/time.Duration(len(samples)), samples[len(samples)/2], p95, samples[len(samples)-1])
	if p95 > 5*time.Millisecond {
		t.Fatalf("p95 = %v supera 5 ms", p95)
	}
}

// BenchmarkExchange mide un intercambio completo (tomar + botar) en el árbitro.
func BenchmarkExchange(b *testing.B) {
	m := newMatch(b, 1)
	if _, err := m.Start(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if m.Phase() != game.PhaseAwaitingDraw {
			b.StopTimer()
			m = newMatch(b, uint64(i))
			if _, err := m.Start(); err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
		}
		p := m.CurrentPlayer()
		if _, err := m.Apply(game.Action{Type: game.ActDraw, Player: p, Source: game.SourceHidden}); err != nil {
			b.Fatal(err)
		}
		if m.Phase() != game.PhaseAwaitingDiscard {
			continue // la ronda terminó por agotarse las ocultas
		}
		v, _ := m.ViewFor(p)
		if _, err := m.Apply(game.Action{Type: game.ActDiscard, Player: p, Card: v.Hand[0].ID}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFullGame mide una partida completa de 5 rondas con bots.
func BenchmarkFullGame(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := newMatch(b, uint64(i))
		if _, err := bot.PlayGame(m, rand.New(rand.NewPCG(uint64(i), 5)), bot.Options{}); err != nil {
			b.Fatal(err)
		}
	}
}
