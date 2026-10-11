// Package bot implementa un jugador automático sencillo sobre la API pública
// de game.Match. Se usa en las pruebas de simulación y en cmd/simulate para
// ejercitar partidas completas; no forma parte de las reglas del juego.
package bot

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"

	"github.com/soetgdeznsgk/ArchSoft-Group1F/game-logic/internal/game"
)

// FindMelds busca de forma voraz las jugadas objetivo de la ronda en la mano.
// Devuelve nil si no las encuentra. No es óptimo: puede no hallar una
// combinación que sí existe.
func FindMelds(hand []game.Card, spec game.RoundSpec) [][]game.CardID {
	byFig := make(map[game.Figure][]game.CardID)
	var jokers []game.CardID
	for _, c := range hand {
		if c.Joker {
			jokers = append(jokers, c.ID)
		} else {
			byFig[c.Figure] = append(byFig[c.Figure], c.ID)
		}
	}
	kinds := slices.Clone(spec.Melds)
	slices.SortFunc(kinds, func(a, b game.MeldKind) int { return b.Size() - a.Size() })

	out := make([][]game.CardID, 0, len(kinds))
	for _, k := range kinds {
		size := k.Size()
		best, bestN := game.NoFigure, 0
		for f := game.Figure(1); f <= game.NumFigures; f++ {
			if n := len(byFig[f]); n > bestN {
				best, bestN = f, n
			}
		}
		if bestN == 0 {
			return nil
		}
		take := min(bestN, size)
		need := size - take
		if need > len(jokers) {
			return nil
		}
		meld := slices.Clone(byFig[best][:take])
		byFig[best] = byFig[best][take:]
		meld = append(meld, jokers[:need]...)
		jokers = jokers[need:]
		out = append(out, meld)
	}
	return out
}

func figureCount(hand []game.Card, f game.Figure) int {
	n := 0
	for _, c := range hand {
		if !c.Joker && c.Figure == f {
			n++
		}
	}
	return n
}

func useful(c *game.Card, hand []game.Card) bool {
	return c != nil && (c.Joker || figureCount(hand, c.Figure) >= 1)
}

// WantsSteal decide si el bot intenta robar la última carta botada.
func WantsSteal(v game.PlayerView, rng *rand.Rand) bool {
	if !v.CanSteal || v.TopDiscard == nil {
		return false
	}
	c := v.TopDiscard
	if c.Joker || figureCount(v.Hand, c.Figure) >= 2 {
		return rng.Float64() < 0.6
	}
	return rng.Float64() < 0.05
}

func fits(c game.Card, banks []game.DepositBank) (int, bool) {
	for _, b := range banks {
		if c.Joker || c.Figure == b.Figure {
			return b.ID, true
		}
	}
	return 0, false
}

// pickDiscard elige la carta a botar: nunca un comodín si hay alternativa,
// evita cartas depositables si ya bajó y prefiere la figura menos repetida.
func pickDiscard(v game.PlayerView, rng *rand.Rand) game.CardID {
	bestScore := 1 << 30
	var cands []game.CardID
	for _, c := range v.Hand {
		score := 100
		if !c.Joker {
			score = figureCount(v.Hand, c.Figure)
			if v.LaidDown {
				if _, ok := fits(c, v.Banks); ok {
					score += 50
				}
			}
		}
		if score < bestScore {
			bestScore, cands = score, cands[:0]
		}
		if score == bestScore {
			cands = append(cands, c.ID)
		}
	}
	return cands[rng.IntN(len(cands))]
}

func inPlay(p game.Phase) bool {
	return p == game.PhaseAwaitingDraw || p == game.PhaseAwaitingDiscard
}

// errStop indica que la ronda terminó durante el turno (no es un fallo).
var errStop = errors.New("ronda terminada")

// PlayTurn juega el turno completo del jugador p: baja si puede, deposita lo
// que pueda, toma y bota. after se invoca tras cada acción aplicada.
func PlayTurn(m *game.Match, p game.PlayerID, rng *rand.Rand, after func() error) error {
	apply := func(a game.Action) error {
		if _, err := m.Apply(a); err != nil {
			return fmt.Errorf("%s de %s: %w", a.Type, p, err)
		}
		if after != nil {
			if err := after(); err != nil {
				return err
			}
		}
		if !inPlay(m.Phase()) {
			return errStop
		}
		return nil
	}
	err := playTurn(m, p, rng, apply)
	if errors.Is(err, errStop) {
		return nil
	}
	return err
}

func playTurn(m *game.Match, p game.PlayerID, rng *rand.Rand, apply func(game.Action) error) error {
	v, err := m.ViewFor(p)
	if err != nil {
		return err
	}
	if v.CurrentTurn != p || v.Phase != game.PhaseAwaitingDraw {
		return fmt.Errorf("no es el turno de %s", p)
	}
	if v.CanLayDown {
		if melds := FindMelds(v.Hand, *v.Spec); melds != nil {
			if err := apply(game.Action{Type: game.ActLayDown, Player: p, Melds: melds}); err != nil {
				return err
			}
		}
	}
	for {
		if v, err = m.ViewFor(p); err != nil {
			return err
		}
		if !v.CanDeposit {
			break
		}
		deposited := false
		for _, c := range v.Hand {
			if bank, ok := fits(c, v.Banks); ok {
				if err := apply(game.Action{Type: game.ActDeposit, Player: p, Card: c.ID, Bank: bank}); err != nil {
					return err
				}
				deposited = true
				break
			}
		}
		if !deposited {
			break
		}
	}
	src := game.SourceHidden
	if v.CanTakeDiscard && useful(v.TopDiscard, v.Hand) {
		src = game.SourceDiscard
	}
	if err := apply(game.Action{Type: game.ActDraw, Player: p, Source: src}); err != nil {
		return err
	}
	if v, err = m.ViewFor(p); err != nil {
		return err
	}
	return apply(game.Action{Type: game.ActDiscard, Player: p, Card: pickDiscard(v, rng)})
}

// Options configura una partida simulada.
type Options struct {
	// DisconnectRate es la probabilidad, antes de cada turno, de desconectar
	// a un jugador al azar. Los desconectados se reconectan con probabilidad 0.3.
	DisconnectRate float64
	// After se invoca tras cada acción aplicada (útil para verificar invariantes).
	After func() error
}

// Stats resume una partida simulada.
type Stats struct {
	Actions             int
	Steals              int
	RoundsWithWinner    int
	RoundsWithoutWinner int
	Final               *game.FinalResult
}

// PlayGame juega una partida completa con 4 bots.
func PlayGame(m *game.Match, rng *rand.Rand, opt Options) (Stats, error) {
	var st Stats
	after := func() error {
		st.Actions++
		if opt.After != nil {
			return opt.After()
		}
		return nil
	}
	if _, err := m.Start(); err != nil {
		return st, err
	}
	players := m.Players()
	disconnected := map[game.PlayerID]bool{}

	for guard := 0; guard < 1_000_000; guard++ {
		switch m.Phase() {
		case game.PhaseGameOver:
			for _, r := range m.Results() {
				if r.Winner != "" {
					st.RoundsWithWinner++
				} else {
					st.RoundsWithoutWinner++
				}
			}
			st.Final = m.Final()
			return st, nil
		case game.PhaseRoundOver:
			if _, err := m.StartNextRound(); err != nil {
				return st, err
			}
			if err := after(); err != nil {
				return st, err
			}
			continue
		}

		if opt.DisconnectRate > 0 {
			for _, p := range players {
				if disconnected[p] && rng.Float64() < 0.3 {
					disconnected[p] = false
					if _, err := m.SetConnected(p, true); err != nil {
						return st, err
					}
				}
			}
			if rng.Float64() < opt.DisconnectRate {
				p := players[rng.IntN(len(players))]
				disconnected[p] = true
				if _, err := m.SetConnected(p, false); err != nil {
					return st, err
				}
			}
			if err := after(); err != nil {
				return st, err
			}
			if !inPlay(m.Phase()) {
				continue
			}
		}

		cur := m.CurrentPlayer()
		for _, p := range players {
			if p == cur || disconnected[p] {
				continue
			}
			v, err := m.ViewFor(p)
			if err != nil {
				return st, err
			}
			if WantsSteal(v, rng) {
				if _, err := m.Apply(game.Action{Type: game.ActSteal, Player: p}); err != nil {
					return st, fmt.Errorf("robo habilitado en la vista pero rechazado: %w", err)
				}
				st.Steals++
				if err := after(); err != nil {
					return st, err
				}
			}
		}
		if !inPlay(m.Phase()) {
			continue
		}
		if err := PlayTurn(m, cur, rng, after); err != nil {
			return st, err
		}
	}
	return st, errors.New("la partida no terminó")
}
