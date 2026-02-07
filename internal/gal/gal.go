package gal

import "fmt"

// Pin represents an input to a term.
type Pin struct {
	Pin int
	Neg bool
}

// Term is an OR of AND terms. Each inner slice is an AND term.
type Term struct {
	Line int
	Pins [][]Pin
}

type GAL struct {
	Chip Chip

	Fuses []bool
	Xor   []bool
	Sig   []bool
	AC1   []bool
	PT    []bool
	Syn   bool
	AC0   bool
}

func NewGAL(chip Chip) *GAL {
	logicSize := chip.NumRows() * chip.NumCols()
	olmcs := chip.NumOLMCs()
	g := &GAL{
		Chip:  chip,
		Fuses: make([]bool, logicSize),
		Xor:   make([]bool, olmcs),
		Sig:   make([]bool, 64),
		AC1:   make([]bool, olmcs),
		PT:    make([]bool, 64),
		Syn:   false,
		AC0:   false,
	}
	for i := range g.Fuses {
		g.Fuses[i] = true
	}
	return g
}

// Only simple mode for GAL16V8.
func (g *GAL) SetSimpleMode() {
	g.Syn = true
	g.AC0 = false
}

func (g *GAL) AddTerm(term Term, bounds Bounds) error {
	b := bounds
	singleRow := b.MaxRows == b.RowOffset+1
	for _, row := range term.Pins {
		if b.RowOffset == b.MaxRows {
			if singleRow {
				return fmt.Errorf("line %d: more than one product term", term.Line)
			}
			return fmt.Errorf("line %d: too many product terms (max %d)", term.Line, b.MaxRows-1)
		}
		for _, input := range row {
			if err := g.setAnd(b.StartRow+b.RowOffset, input.Pin, input.Neg); err != nil {
				return fmt.Errorf("line %d: %w", term.Line, err)
			}
		}
		b.RowOffset++
	}
	g.clearRows(b)
	return nil
}

func (g *GAL) AddTermOpt(term *Term, bounds Bounds) error {
	if term == nil {
		return g.AddTerm(FalseTerm(0), bounds)
	}
	return g.AddTerm(*term, bounds)
}

func (g *GAL) clearRows(bounds Bounds) {
	rowLen := g.Chip.NumCols()
	start := (bounds.StartRow + bounds.RowOffset) * rowLen
	end := (bounds.StartRow + bounds.MaxRows) * rowLen
	for i := start; i < end; i++ {
		g.Fuses[i] = false
	}
}

func (g *GAL) setAnd(row int, pin int, neg bool) error {
	rowLen := g.Chip.NumCols()
	col, err := g.pinToColumn(pin)
	if err != nil {
		return err
	}
	off := 0
	if neg {
		off = 1
	}
	idx := row*rowLen + col + off
	if idx < 0 || idx >= len(g.Fuses) {
		return fmt.Errorf("fuse index out of range")
	}
	g.Fuses[idx] = false
	return nil
}

// Only for GAL16V8 simple mode and GAL22V10.
func (g *GAL) pinToColumn(pin int) (int, error) {
	if pin < 1 || pin > g.Chip.NumPins() {
		return 0, fmt.Errorf("invalid pin %d", pin)
	}
	if g.Chip == ChipGAL16V8 {
		return pinToCol16Simple(pin)
	}
	if g.Chip == ChipGAL22V10 {
		return pinToCol22v10(pin)
	}
	return 0, fmt.Errorf("unsupported chip")
}

func pinToCol16Simple(pin int) (int, error) {
	// 1-based pin index into table.
	// Table adapted from galette (GAL16V8 simple mode).
	switch pin {
	case 1:
		return 2, nil
	case 2:
		return 0, nil
	case 3:
		return 4, nil
	case 4:
		return 8, nil
	case 5:
		return 12, nil
	case 6:
		return 16, nil
	case 7:
		return 20, nil
	case 8:
		return 24, nil
	case 9:
		return 28, nil
	case 10:
		return 0, fmt.Errorf("pin %d is power", pin)
	case 11:
		return 30, nil
	case 12:
		return 26, nil
	case 13:
		return 22, nil
	case 14:
		return 18, nil
	case 15:
		return 0, fmt.Errorf("pin %d is not an input in simple mode", pin)
	case 16:
		return 0, fmt.Errorf("pin %d is not an input in simple mode", pin)
	case 17:
		return 14, nil
	case 18:
		return 10, nil
	case 19:
		return 6, nil
	case 20:
		return 0, fmt.Errorf("pin %d is power", pin)
	default:
		return 0, fmt.Errorf("invalid pin %d", pin)
	}
}

func pinToCol22v10(pin int) (int, error) {
	switch pin {
	case 1:
		return 0, nil
	case 2:
		return 4, nil
	case 3:
		return 8, nil
	case 4:
		return 12, nil
	case 5:
		return 16, nil
	case 6:
		return 20, nil
	case 7:
		return 24, nil
	case 8:
		return 28, nil
	case 9:
		return 32, nil
	case 10:
		return 36, nil
	case 11:
		return 40, nil
	case 12:
		return 0, fmt.Errorf("pin %d is power", pin)
	case 13:
		return 42, nil
	case 14:
		return 38, nil
	case 15:
		return 34, nil
	case 16:
		return 30, nil
	case 17:
		return 26, nil
	case 18:
		return 22, nil
	case 19:
		return 18, nil
	case 20:
		return 14, nil
	case 21:
		return 10, nil
	case 22:
		return 6, nil
	case 23:
		return 2, nil
	case 24:
		return 0, fmt.Errorf("pin %d is power", pin)
	default:
		return 0, fmt.Errorf("invalid pin %d", pin)
	}
}

func TrueTerm(line int) Term {
	return Term{Line: line, Pins: [][]Pin{{}}}
}

func FalseTerm(line int) Term {
	return Term{Line: line, Pins: nil}
}
