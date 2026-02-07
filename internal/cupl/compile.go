package cupl

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pborges/cupl/internal/gal"
)

type Symbol struct {
	Pin       int
	ActiveLow bool
}

// Compile builds a GAL fuse map from CUPL content.
func Compile(c Content) (*gal.GAL, error) {
	chip, err := gal.ParseChip(c.Device)
	if err != nil {
		return nil, err
	}
	bp := gal.NewBlueprint(chip)
	if partno := strings.TrimSpace(c.Meta["Partno"]); partno != "" {
		bp.Sig = []byte(partno)
	}

	symbols := make(map[string]Symbol)
	for pin, def := range c.Pins {
		if pin < 1 || pin > chip.NumPins() {
			return nil, fmt.Errorf("pin %d out of range for %s", pin, chip.Name())
		}
		bp.Pins[pin-1] = def.Name
		symbols[def.Name] = Symbol{Pin: pin, ActiveLow: def.ActiveLow}
	}
	// Add power pins
	symbols["VCC"] = Symbol{Pin: chip.NumPins(), ActiveLow: false}
	symbols["GND"] = Symbol{Pin: chip.NumPins() / 2, ActiveLow: false}

	type compiledEq struct {
		eq    Equation
		terms []Term
	}
	compiled := make([]compiledEq, 0, len(c.Equations))
	for _, eq := range c.Equations {
		terms, err := exprToTerms(eq.Expr, c.Fields)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", eq.Line, err)
		}
		compiled = append(compiled, compiledEq{eq: eq, terms: terms})
		// Mark feedback use based on actual terms (post range expansion).
		for _, term := range terms {
			for _, lit := range term.Lits {
				if sym, ok := symbols[lit.Name]; ok {
					if olmc, ok := chip.PinToOLMC(sym.Pin); ok {
						bp.OLMC[olmc].Feedback = true
					}
				}
			}
		}
	}

	for _, item := range compiled {
		eq := item.eq
		lhs := strings.TrimSpace(eq.LHS)
		sym, ok := symbols[lhs]
		if !ok {
			return nil, fmt.Errorf("line %d: unknown output %q", eq.Line, lhs)
		}
		olmc, ok := chip.PinToOLMC(sym.Pin)
		if !ok {
			return nil, fmt.Errorf("line %d: %q is not a valid output pin", eq.Line, lhs)
		}
		if bp.OLMC[olmc].Output != nil {
			return nil, fmt.Errorf("line %d: output %q already defined", eq.Line, lhs)
		}

		galTerms, err := mapTermsToPins(item.terms, symbols)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", eq.Line, err)
		}

		term := gal.Term{Line: eq.Line, Pins: galTerms}
		bp.OLMC[olmc].Output = &term
		if sym.ActiveLow {
			bp.OLMC[olmc].Active = gal.ActiveLow
		} else {
			bp.OLMC[olmc].Active = gal.ActiveHigh
		}
	}

	return gal.BuildGAL(bp)
}

// exprToLiterals returns all literals (variable names) referenced by expr.
func exprToLiterals(expr Expr, fields map[string]Field) ([]Literal, error) {
	switch e := expr.(type) {
	case ExprIdent:
		return []Literal{{Name: e.Name}}, nil
	case ExprNot:
		return exprToLiterals(e.X, fields)
	case ExprAnd:
		l1, err := exprToLiterals(e.A, fields)
		if err != nil {
			return nil, err
		}
		l2, err := exprToLiterals(e.B, fields)
		if err != nil {
			return nil, err
		}
		return append(l1, l2...), nil
	case ExprOr:
		l1, err := exprToLiterals(e.A, fields)
		if err != nil {
			return nil, err
		}
		l2, err := exprToLiterals(e.B, fields)
		if err != nil {
			return nil, err
		}
		return append(l1, l2...), nil
	case ExprFieldRange:
		f, ok := fields[e.Field]
		if !ok {
			return nil, fmt.Errorf("unknown field %q", e.Field)
		}
		out := make([]Literal, 0, len(f.Bits))
		for _, b := range f.Bits {
			out = append(out, Literal{Name: b.Name})
		}
		return out, nil
	case ExprConst:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported expression")
	}
}

// DNF handling

type Literal struct {
	Name string
	Neg  bool
}

type Term struct {
	Lits []Literal
}

func exprToTerms(expr Expr, fields map[string]Field) ([]Term, error) {
	nnf := toNNF(expr, false)
	return dnf(nnf, fields)
}

func toNNF(expr Expr, neg bool) Expr {
	switch e := expr.(type) {
	case ExprConst:
		if neg {
			return ExprConst{Value: !e.Value}
		}
		return e
	case ExprIdent:
		if neg {
			return ExprNot{X: e}
		}
		return e
	case ExprFieldRange:
		if neg {
			return ExprNot{X: e}
		}
		return e
	case ExprNot:
		return toNNF(e.X, !neg)
	case ExprAnd:
		if neg {
			return ExprOr{A: toNNF(e.A, true), B: toNNF(e.B, true)}
		}
		return ExprAnd{A: toNNF(e.A, false), B: toNNF(e.B, false)}
	case ExprOr:
		if neg {
			return ExprAnd{A: toNNF(e.A, true), B: toNNF(e.B, true)}
		}
		return ExprOr{A: toNNF(e.A, false), B: toNNF(e.B, false)}
	default:
		return expr
	}
}

func dnf(expr Expr, fields map[string]Field) ([]Term, error) {
	switch e := expr.(type) {
	case ExprConst:
		if e.Value {
			return []Term{{}}, nil
		}
		return nil, nil
	case ExprIdent:
		return []Term{{Lits: []Literal{{Name: e.Name}}}}, nil
	case ExprNot:
		switch inner := e.X.(type) {
		case ExprIdent:
			return []Term{{Lits: []Literal{{Name: inner.Name, Neg: true}}}}, nil
		case ExprFieldRange:
			return fieldRangeTerms(inner, fields, true)
		default:
			return nil, fmt.Errorf("unsupported negation")
		}
	case ExprFieldRange:
		return fieldRangeTerms(e, fields, false)
	case ExprAnd:
		left, err := dnf(e.A, fields)
		if err != nil {
			return nil, err
		}
		right, err := dnf(e.B, fields)
		if err != nil {
			return nil, err
		}
		return andDNF(left, right), nil
	case ExprOr:
		left, err := dnf(e.A, fields)
		if err != nil {
			return nil, err
		}
		right, err := dnf(e.B, fields)
		if err != nil {
			return nil, err
		}
		return append(left, right...), nil
	default:
		return nil, fmt.Errorf("unsupported expression")
	}
}

func andDNF(a, b []Term) []Term {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	var out []Term
	for _, ta := range a {
		for _, tb := range b {
			if t, ok := mergeTerms(ta, tb); ok {
				out = append(out, t)
			}
		}
	}
	return out
}

func mergeTerms(a, b Term) (Term, bool) {
	m := map[string]bool{}
	for _, l := range a.Lits {
		m[l.Name] = l.Neg
	}
	for _, l := range b.Lits {
		if neg, ok := m[l.Name]; ok {
			if neg != l.Neg {
				return Term{}, false
			}
			continue
		}
		m[l.Name] = l.Neg
	}
	lits := make([]Literal, 0, len(m))
	for name, neg := range m {
		lits = append(lits, Literal{Name: name, Neg: neg})
	}
	// stable order for deterministic output
	sort.Slice(lits, func(i, j int) bool { return lits[i].Name < lits[j].Name })
	return Term{Lits: lits}, true
}

func fieldRangeTerms(fr ExprFieldRange, fields map[string]Field, negated bool) ([]Term, error) {
	field, ok := fields[fr.Field]
	if !ok {
		return nil, fmt.Errorf("unknown field %q", fr.Field)
	}
	width := len(field.Bits)
	if width == 0 {
		return nil, fmt.Errorf("field %q has no bits", field.Name)
	}
	lo, hi := fr.Lo, fr.Hi
	projLo := projectValue(field, lo)
	projHi := projectValue(field, hi)
	if projLo > projHi {
		projLo, projHi = projHi, projLo
	}
	maxVal := uint64(1<<width) - 1

	var ranges [][2]uint64
	if !negated {
		ranges = append(ranges, [2]uint64{projLo, projHi})
	} else {
		if projLo > 0 {
			ranges = append(ranges, [2]uint64{0, projLo - 1})
		}
		if projHi < maxVal {
			ranges = append(ranges, [2]uint64{projHi + 1, maxVal})
		}
	}

	var out []Term
	for _, r := range ranges {
		cubes := rangeToCubes(r[0], r[1], width)
		for _, c := range cubes {
			term := Term{}
			for bit := 0; bit < width; bit++ {
				if (c.mask>>bit)&1 == 0 {
					continue
				}
				idx := width - 1 - bit // map LSB->last
				bitVal := (c.value >> bit) & 1
				lit := Literal{Name: field.Bits[idx].Name, Neg: bitVal == 0}
				term.Lits = append(term.Lits, lit)
			}
			out = append(out, term)
		}
	}
	return out, nil
}

type cube struct {
	mask  uint64
	value uint64
}

func rangeToCubes(lo, hi uint64, width int) []cube {
	if lo > hi {
		return nil
	}
	var out []cube
	for lo <= hi {
		remaining := hi - lo + 1
		blockSize := maxBlockSize(lo, remaining)
		k := uint64(0)
		for (uint64(1) << k) < blockSize {
			k++
		}
		mask := uint64(1<<width) - 1
		if k > 0 {
			mask &^= (uint64(1) << k) - 1
		}
		out = append(out, cube{mask: mask, value: lo})
		lo += blockSize
	}
	return out
}

func maxBlockSize(lo, remaining uint64) uint64 {
	if remaining == 0 {
		return 0
	}
	// Largest power of two <= remaining.
	maxPow := uint64(1)
	for (maxPow << 1) <= remaining {
		maxPow <<= 1
	}
	if lo == 0 {
		return maxPow
	}
	lsb := lo & -lo
	if lsb < maxPow {
		return lsb
	}
	return maxPow
}

func projectValue(field Field, v uint64) uint64 {
	width := len(field.Bits)
	if width == 0 {
		return 0
	}
	allNumbered := true
	for _, b := range field.Bits {
		if !b.HasNumber {
			allNumbered = false
			break
		}
	}
	if !allNumbered {
		mask := uint64(1<<width) - 1
		return v & mask
	}
	var out uint64
	for _, b := range field.Bits {
		out <<= 1
		if (v>>b.BitNumber)&1 == 1 {
			out |= 1
		}
	}
	return out
}

func mapTermsToPins(terms []Term, symbols map[string]Symbol) ([][]gal.Pin, error) {
	var out [][]gal.Pin
	for _, t := range terms {
		var row []gal.Pin
		for _, lit := range t.Lits {
			sym, ok := symbols[lit.Name]
			if !ok {
				return nil, fmt.Errorf("unknown symbol %q", lit.Name)
			}
			neg := lit.Neg
			if sym.ActiveLow {
				neg = !neg
			}
			row = append(row, gal.Pin{Pin: sym.Pin, Neg: neg})
		}
		out = append(out, row)
	}
	return out, nil
}
