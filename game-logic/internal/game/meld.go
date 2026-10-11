package game

import (
	"slices"
)

// ValidateMeld verifica que las cartas formen una jugada válida del tipo dado
// y devuelve la figura que la define (RF-26, RF-27):
//   - exactamente kind.Size() cartas;
//   - al menos una carta normal;
//   - todas las cartas normales de la misma figura.
//
// Con tamaño fijo y al menos una normal, el límite de comodines (2 en trío,
// 3 en cuarteto) queda garantizado.
func ValidateMeld(kind MeldKind, cards []Card) (Figure, error) {
	if kind != Trio && kind != Quartet {
		return NoFigure, newError(CodeInvalidMeld, "tipo de jugada desconocido")
	}
	if len(cards) != kind.Size() {
		return NoFigure, newErrorf(CodeInvalidMeld,
			"un %s debe tener exactamente %d cartas (tiene %d)", kind, kind.Size(), len(cards))
	}
	fig := NoFigure
	for _, c := range cards {
		if c.Joker {
			continue
		}
		if fig == NoFigure {
			fig = c.Figure
		} else if c.Figure != fig {
			return NoFigure, newErrorf(CodeInvalidMeld,
				"todas las cartas normales de un %s deben ser de la misma figura", kind)
		}
	}
	if fig == NoFigure {
		return NoFigure, newErrorf(CodeInvalidMeld,
			"un %s debe tener al menos una carta normal (máximo %d comodines)", kind, kind.MaxJokers())
	}
	return fig, nil
}

// meldsMatchSpec indica si los tamaños de las jugadas enviadas coinciden,
// sin importar el orden, con las jugadas objetivo de la ronda.
func meldsMatchSpec(spec RoundSpec, sizes []int) bool {
	req := make([]int, len(spec.Melds))
	for i, k := range spec.Melds {
		req[i] = k.Size()
	}
	got := slices.Clone(sizes)
	slices.Sort(req)
	slices.Sort(got)
	return slices.Equal(req, got)
}
