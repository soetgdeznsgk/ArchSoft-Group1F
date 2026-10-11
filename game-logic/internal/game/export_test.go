package game

import "fmt"

// CheckInvariants verifica la consistencia interna del estado. Solo existe en
// las pruebas; el paquete externo game_test la usa en las simulaciones.
func (g *Game) CheckInvariants() error {
	if g.phase == PhaseWaiting {
		return nil
	}
	// 1. Conservación: las 60 cartas están exactamente una vez en manos,
	//    banco de ocultas, banco de vistas o bancos de depósito.
	seen := make(map[CardID]int, DeckSize)
	count := func(cards []Card) {
		for _, c := range cards {
			seen[c.ID]++
		}
	}
	for _, p := range g.players {
		count(p.hand)
	}
	count(g.hidden)
	count(g.discard)
	for _, b := range g.banks {
		count(b.Cards)
	}
	if len(seen) != DeckSize {
		return fmt.Errorf("hay %d cartas distintas en juego, se esperaban %d", len(seen), DeckSize)
	}
	for id, n := range seen {
		if n != 1 || id < 0 || int(id) >= DeckSize {
			return fmt.Errorf("la carta %d aparece %d veces", id, n)
		}
	}
	// 2. Cada banco de depósito tiene una figura definida y solo cartas compatibles.
	for _, b := range g.banks {
		normals := 0
		for _, c := range b.Cards {
			if !c.Joker {
				normals++
				if c.Figure != b.Figure {
					return fmt.Errorf("banco %d (F%d) contiene %v", b.ID, b.Figure, c)
				}
			}
		}
		if normals == 0 || len(b.Cards) < 3 {
			return fmt.Errorf("banco %d inválido: %v", b.ID, b.Cards)
		}
	}
	// 3. Banco de vistas coherente.
	if g.discardAvailable && (len(g.discard) == 0 || g.lastDiscarder < 0) {
		return fmt.Errorf("banco de vistas disponible sin carta o sin autor")
	}
	if g.inPlay() && g.phase == PhaseAwaitingDiscard && g.discardAvailable {
		return fmt.Errorf("robo abierto después de que el jugador en turno tomó carta")
	}
	// 4. Puntajes acumulados = suma de los resultados de ronda.
	if len(g.results) > g.round {
		return fmt.Errorf("%d resultados para la ronda %d", len(g.results), g.round)
	}
	for _, p := range g.players {
		sum, won := 0, 0
		for _, r := range g.results {
			sum += r.Points[p.id]
			if r.Winner == p.id {
				won++
				if r.Points[p.id] != 0 {
					return fmt.Errorf("el ganador %s tiene puntos en la ronda %d", p.id, r.Round)
				}
			}
		}
		if sum != p.total || won != p.roundsWon {
			return fmt.Errorf("estadísticas de %s inconsistentes", p.id)
		}
	}
	// 5. En juego, nadie queda con la mano vacía (ganaría la ronda).
	if g.inPlay() {
		for _, p := range g.players {
			if len(p.hand) == 0 {
				return fmt.Errorf("%s tiene la mano vacía con la ronda en juego", p.id)
			}
		}
	}
	return nil
}

// CheckInvariants verifica la consistencia de la partida bajo el árbitro.
func (m *Match) CheckInvariants() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.g.CheckInvariants()
}
