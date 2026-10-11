// Comando simulate juega partidas completas con bots para verificar la lógica
// sin red ni persistencia (prueba de humo y demostración de la API).
//
//	go run ./cmd/simulate -games 200 -seed 7 -disconnect 0.02
package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"time"

	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/bot"
	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/game"
)

func main() {
	games := flag.Int("games", 100, "cantidad de partidas a simular")
	seed := flag.Uint64("seed", 1, "semilla para reproducir resultados")
	disconnect := flag.Float64("disconnect", 0, "probabilidad de desconexión antes de cada turno")
	verbose := flag.Bool("v", false, "imprime el resultado de cada partida")
	flag.Parse()

	players := []game.PlayerID{"ana", "beto", "caro", "dani"}
	var total bot.Stats
	wins := map[game.PlayerID]int{}
	start := time.Now()

	for i := 0; i < *games; i++ {
		s := *seed + uint64(i)
		m, err := game.NewMatch(players, game.Config{Rand: rand.New(rand.NewPCG(s, s^0x9e3779b97f4a7c15))})
		if err != nil {
			fail(err)
		}
		st, err := bot.PlayGame(m, rand.New(rand.NewPCG(s, 42)), bot.Options{DisconnectRate: *disconnect})
		if err != nil {
			fail(fmt.Errorf("partida %d (semilla %d): %w", i, s, err))
		}
		total.Actions += st.Actions
		total.Steals += st.Steals
		total.RoundsWithWinner += st.RoundsWithWinner
		total.RoundsWithoutWinner += st.RoundsWithoutWinner
		for _, w := range st.Final.Winners {
			wins[w]++
		}
		if *verbose {
			fmt.Printf("partida %3d ganadores=%v posiciones=%+v\n", i, st.Final.Winners, st.Final.Standings)
		}
	}

	elapsed := time.Since(start)
	fmt.Printf("partidas:              %d\n", *games)
	fmt.Printf("acciones aplicadas:    %d\n", total.Actions)
	fmt.Printf("robos:                 %d\n", total.Steals)
	fmt.Printf("rondas con ganador:    %d\n", total.RoundsWithWinner)
	fmt.Printf("rondas sin ganador:    %d\n", total.RoundsWithoutWinner)
	fmt.Printf("victorias por jugador: %v\n", wins)
	if total.Actions > 0 {
		fmt.Printf("tiempo total:          %v (%.1f µs por acción, incluye bots)\n",
			elapsed, float64(elapsed.Microseconds())/float64(total.Actions))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
