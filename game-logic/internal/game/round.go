package game

import (
	"fmt"
	"strings"
)

// MeldKind es el tipo de jugada. Su valor coincide con la cantidad de cartas.
type MeldKind int

const (
	Trio    MeldKind = 3
	Quartet MeldKind = 4
)

// Size devuelve la cantidad exacta de cartas de la jugada.
func (k MeldKind) Size() int { return int(k) }

// MaxJokers devuelve el máximo de comodines permitido (RF-27): toda jugada
// necesita al menos una carta normal que defina la figura.
func (k MeldKind) MaxJokers() int { return int(k) - 1 }

func (k MeldKind) String() string {
	switch k {
	case Trio:
		return "trío"
	case Quartet:
		return "cuarteto"
	}
	return fmt.Sprintf("jugada(%d)", int(k))
}

func kindForSize(n int) (MeldKind, bool) {
	switch n {
	case 3:
		return Trio, true
	case 4:
		return Quartet, true
	}
	return 0, false
}

// TotalRounds es la cantidad de rondas de una partida.
const TotalRounds = 5

// RoundSpec describe una ronda: cartas repartidas y jugadas objetivo (RF-13).
type RoundSpec struct {
	Number         int        `json:"number"`
	CardsPerPlayer int        `json:"cards_per_player"`
	Melds          []MeldKind `json:"melds"`
}

// Describe devuelve la jugada objetivo en texto, p. ej. "un cuarteto y un trío".
func (s RoundSpec) Describe() string {
	parts := make([]string, len(s.Melds))
	for i, k := range s.Melds {
		parts[i] = "un " + k.String()
	}
	return strings.Join(parts, " y ")
}

var roundSpecs = [TotalRounds]RoundSpec{
	{Number: 1, CardsPerPlayer: 3, Melds: []MeldKind{Trio}},
	{Number: 2, CardsPerPlayer: 4, Melds: []MeldKind{Quartet}},
	{Number: 3, CardsPerPlayer: 6, Melds: []MeldKind{Trio, Trio}},
	{Number: 4, CardsPerPlayer: 7, Melds: []MeldKind{Quartet, Trio}},
	{Number: 5, CardsPerPlayer: 8, Melds: []MeldKind{Quartet, Quartet}},
}

// Spec devuelve una copia de la especificación de la ronda (1..5).
func Spec(round int) (RoundSpec, bool) {
	if round < 1 || round > TotalRounds {
		return RoundSpec{}, false
	}
	s := roundSpecs[round-1]
	s.Melds = append([]MeldKind(nil), s.Melds...)
	return s, true
}
