// Package game implementa la lógica del juego de cartas Bololó (modo de
// 5 rondas con tríos y cuartetos) como un motor puro: sin red ni persistencia.
//
// El motor es autoritativo: toda acción se valida contra el estado antes de
// aplicarse. Game NO es seguro para uso concurrente; use Match, que serializa
// las acciones y actúa como árbitro único (RNF-02, RNF-03).
package game

import "fmt"

// Figure identifica la figura de una carta normal (1..7). Los comodines usan NoFigure.
type Figure int

const (
	NoFigure       Figure = 0
	NumFigures            = 7
	CardsPerFigure        = 8
	NumJokers             = 4
	DeckSize              = NumFigures*CardsPerFigure + NumJokers // 60

	NormalPenalty = 1 // puntos negativos por carta normal en mano
	JokerPenalty  = 5 // puntos negativos por comodín en mano
)

// CardID identifica de forma única una carta del mazo (0..59). Los clientes
// referencian cartas siempre por su ID.
type CardID int

// Card es una carta del mazo. Las cartas 0..55 son normales (8 por figura) y
// las 56..59 son comodines.
type Card struct {
	ID     CardID `json:"id"`
	Figure Figure `json:"figure"`
	Joker  bool   `json:"joker"`
}

// Penalty devuelve los puntos negativos que suma la carta si queda en la mano.
func (c Card) Penalty() int {
	if c.Joker {
		return JokerPenalty
	}
	return NormalPenalty
}

func (c Card) String() string {
	if c.Joker {
		return fmt.Sprintf("comodín#%d", c.ID)
	}
	return fmt.Sprintf("F%d#%d", c.Figure, c.ID)
}

// NewDeck devuelve el mazo completo de 60 cartas, ordenado por ID.
func NewDeck() []Card {
	deck := make([]Card, 0, DeckSize)
	for f := 1; f <= NumFigures; f++ {
		for k := 0; k < CardsPerFigure; k++ {
			deck = append(deck, Card{ID: CardID(len(deck)), Figure: Figure(f)})
		}
	}
	for k := 0; k < NumJokers; k++ {
		deck = append(deck, Card{ID: CardID(len(deck)), Joker: true})
	}
	return deck
}

// HandPenalty suma los puntos negativos de un conjunto de cartas.
func HandPenalty(cards []Card) int {
	total := 0
	for _, c := range cards {
		total += c.Penalty()
	}
	return total
}

func indexOfCard(cards []Card, id CardID) int {
	for i, c := range cards {
		if c.ID == id {
			return i
		}
	}
	return -1
}
