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
	bp.ModeHint = gal.ParseModeHint(c.Device)
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

	// Desugar set/bus operations (field-name LHS) before processing
	c.Equations = desugarSetOps(c)

	aliases := make(map[string]Expr)
	for _, eq := range c.Equations {
		info, err := parseEquationLHS(eq.LHS)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", eq.Line, err)
		}
		if info.ActiveLow {
			if _, ok := symbols[info.Name]; !ok {
				// Allow active-low on AR/SP (they're not pins)
				if !isGlobalSignal(info.Name) {
					return nil, fmt.Errorf("line %d: active-low output %q is not a defined pin", eq.Line, info.Name)
				}
			}
		}
		if _, ok := symbols[info.Name]; !ok {
			if !eq.Append && !isGlobalSignal(info.Name) && info.Extension == "" {
				aliases[info.Name] = eq.Expr
			}
		}
	}

	type compiledEq struct {
		eq         Equation
		terms      []Term
		activeLow  bool
		outputName string
		extension  string
	}
	compiled := make([]compiledEq, 0, len(c.Equations))
	for _, eq := range c.Equations {
		info, err := parseEquationLHS(eq.LHS)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", eq.Line, err)
		}

		// Handle global AR/SP signals
		if isGlobalSignal(info.Name) {
			chosenTerms, err := exprToTerms(eq.Expr, c.Fields, aliases)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", eq.Line, err)
			}
			galTerms, err := mapTermsToPins(chosenTerms, symbols)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", eq.Line, err)
			}
			term := gal.Term{Line: eq.Line, Pins: galTerms}
			switch strings.ToUpper(info.Name) {
			case "AR":
				bp.AR = &term
			case "SP":
				bp.SP = &term
			}
			continue
		}

		if _, ok := symbols[info.Name]; !ok {
			// Non-output equation: treat as alias for expressions.
			continue
		}

		// Polarity optimization: if the top-level expression is NOT, unwrap it
		// and flip polarity (compile the inner expression with inverted XOR bit).
		// This matches WinCUPL's behavior.
		compileExpr := eq.Expr
		polarityFlipped := false
		if notExpr, ok := eq.Expr.(ExprNot); ok && !eq.Append && info.Extension != "E" && info.Extension != "R" {
			compileExpr = notExpr.X
			polarityFlipped = true
		}

		chosenTerms, err := exprToTerms(compileExpr, c.Fields, aliases)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", eq.Line, err)
		}

		finalActiveLow := info.ActiveLow
		if polarityFlipped {
			finalActiveLow = !finalActiveLow
		}

		compiled = append(compiled, compiledEq{eq: eq, terms: chosenTerms, activeLow: finalActiveLow, outputName: info.Name, extension: info.Extension})
		// Mark feedback use based on actual terms (post range expansion).
		for _, term := range chosenTerms {
			for _, lit := range term.Lits {
				if sym, ok := symbols[lit.Name]; ok {
					if olmc, ok := chip.PinToOLMC(sym.Pin); ok {
						bp.OLMC[olmc].Feedback = true
					}
				}
			}
		}
	}

	// Accumulate all terms per output (including APPEND), then minimize and place.
	type olmcAccum struct {
		terms     []Term
		activeLow bool
		line      int
		lhs       string
		extension string
	}
	accum := make(map[int]*olmcAccum) // keyed by OLMC index
	oeAccum := make(map[int]*olmcAccum)

	for _, item := range compiled {
		eq := item.eq
		lhs := item.outputName
		sym, ok := symbols[lhs]
		if !ok {
			return nil, fmt.Errorf("line %d: unknown output %q", eq.Line, lhs)
		}
		olmc, ok := chip.PinToOLMC(sym.Pin)
		if !ok {
			return nil, fmt.Errorf("line %d: %q is not a valid output pin", eq.Line, lhs)
		}

		if item.extension == "E" {
			// Output enable equation — store separately
			if _, exists := oeAccum[olmc]; exists {
				return nil, fmt.Errorf("line %d: OE for %q already defined", eq.Line, lhs)
			}
			oeAccum[olmc] = &olmcAccum{
				terms: item.terms,
				line:  eq.Line,
				lhs:   lhs,
			}
			continue
		}

		if a, exists := accum[olmc]; exists {
			if !eq.Append {
				return nil, fmt.Errorf("line %d: output %q already defined", eq.Line, lhs)
			}
			a.terms = append(a.terms, item.terms...)
		} else {
			accum[olmc] = &olmcAccum{
				terms:     item.terms,
				activeLow: item.activeLow || sym.ActiveLow,
				line:      eq.Line,
				lhs:       lhs,
				extension: item.extension,
			}
		}
	}

	for olmc, a := range accum {
		// Minimize the accumulated terms for this output
		a.terms = minimizeTerms(a.terms)

		galTerms, err := mapTermsToPins(a.terms, symbols)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", a.line, err)
		}

		term := gal.Term{Line: a.line, Pins: galTerms}
		bp.OLMC[olmc].Output = &term
		if a.activeLow {
			bp.OLMC[olmc].Active = gal.ActiveLow
		} else {
			bp.OLMC[olmc].Active = gal.ActiveHigh
		}

		switch a.extension {
		case "R":
			bp.OLMC[olmc].Registered = true
		case "T":
			// Tristate data — implies OE term needed
		}
	}

	// Place OE terms
	for olmc, oe := range oeAccum {
		oe.terms = minimizeTerms(oe.terms)
		galTerms, err := mapTermsToPins(oe.terms, symbols)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", oe.line, err)
		}
		term := gal.Term{Line: oe.line, Pins: galTerms}
		bp.OLMC[olmc].OETerm = &term
	}

	// Note: AC1 handling for unused OLMCs is done in setTristate based on mode.

	// needs_flip: On GAL22V10, registered + active-high outputs have their
	// feedback taken from the register (pre-XOR gate). Since XOR=1 inverts
	// the output, the feedback value is the complement of the pin value.
	// To compensate, flip the negation for any AND array reference to such pins.
	if chip == gal.ChipGAL22V10 {
		flipPins := make(map[int]bool)
		for i, olmc := range bp.OLMC {
			if olmc.Registered && olmc.Active == gal.ActiveHigh {
				flipPins[chip.MinOLMCPin()+i] = true
			}
		}
		if len(flipPins) > 0 {
			flipTermPins := func(t *gal.Term) {
				if t == nil {
					return
				}
				for ri, row := range t.Pins {
					for ci, p := range row {
						if flipPins[p.Pin] {
							t.Pins[ri][ci].Neg = !p.Neg
						}
					}
				}
			}
			for i := range bp.OLMC {
				flipTermPins(bp.OLMC[i].Output)
				flipTermPins(bp.OLMC[i].OETerm)
			}
			flipTermPins(bp.AR)
			flipTermPins(bp.SP)
		}
	}

	return gal.BuildGAL(bp)
}

// isGlobalSignal returns true for AR and SP (global signals, not pins).
func isGlobalSignal(name string) bool {
	n := strings.ToUpper(name)
	return n == "AR" || n == "SP"
}

// desugarSetOps expands field-name LHS equations into per-bit equations.
func desugarSetOps(c Content) []Equation {
	var out []Equation
	for _, eq := range c.Equations {
		lhs := strings.TrimSpace(eq.LHS)
		// Strip ! prefix for lookup
		lhsClean := lhs
		if strings.HasPrefix(lhsClean, "!") {
			lhsClean = strings.TrimSpace(lhsClean[1:])
		}
		field, ok := c.Fields[lhsClean]
		if !ok {
			out = append(out, eq)
			continue
		}
		// LHS is a field name — expand to per-bit equations
		expanded := expandFieldExpr(eq.Expr, field, c.Fields, eq.Line, eq.Append, lhs)
		out = append(out, expanded...)
	}
	return out
}

func expandFieldExpr(expr Expr, outField Field, fields map[string]Field, line int, isAppend bool, lhs string) []Equation {
	width := len(outField.Bits)
	bitExprs := exprToBitExprs(expr, width, fields)
	var out []Equation
	for i, be := range bitExprs {
		out = append(out, Equation{
			Line:   line,
			LHS:    outField.Bits[i].Name,
			Expr:   be,
			Append: isAppend,
		})
	}
	return out
}

// exprToBitExprs breaks an expression into per-bit expressions for a field of given width.
func exprToBitExprs(expr Expr, width int, fields map[string]Field) []Expr {
	switch e := expr.(type) {
	case ExprAnd:
		leftBits := exprToBitExprs(e.A, width, fields)
		rightBits := exprToBitExprs(e.B, width, fields)
		out := make([]Expr, width)
		for i := 0; i < width; i++ {
			out[i] = ExprAnd{A: leftBits[i], B: rightBits[i]}
		}
		return out
	case ExprOr:
		leftBits := exprToBitExprs(e.A, width, fields)
		rightBits := exprToBitExprs(e.B, width, fields)
		out := make([]Expr, width)
		for i := 0; i < width; i++ {
			out[i] = ExprOr{A: leftBits[i], B: rightBits[i]}
		}
		return out
	case ExprXor:
		leftBits := exprToBitExprs(e.A, width, fields)
		rightBits := exprToBitExprs(e.B, width, fields)
		out := make([]Expr, width)
		for i := 0; i < width; i++ {
			out[i] = ExprXor{A: leftBits[i], B: rightBits[i]}
		}
		return out
	case ExprNot:
		innerBits := exprToBitExprs(e.X, width, fields)
		out := make([]Expr, width)
		for i := 0; i < width; i++ {
			out[i] = ExprNot{X: innerBits[i]}
		}
		return out
	case ExprIdent:
		// Check if this ident is a field name
		if f, ok := fields[e.Name]; ok && len(f.Bits) == width {
			out := make([]Expr, width)
			for i, b := range f.Bits {
				out[i] = ExprIdent{Name: b.Name}
			}
			return out
		}
		// Scalar: broadcast to all bits
		out := make([]Expr, width)
		for i := 0; i < width; i++ {
			out[i] = e
		}
		return out
	case ExprIdentList:
		if len(e.Names) == width {
			out := make([]Expr, width)
			for i, name := range e.Names {
				out[i] = ExprIdent{Name: name}
			}
			return out
		}
		// Width mismatch: broadcast whole expression
		out := make([]Expr, width)
		for i := 0; i < width; i++ {
			out[i] = expr
		}
		return out
	default:
		// Scalar expression: broadcast
		out := make([]Expr, width)
		for i := 0; i < width; i++ {
			out[i] = expr
		}
		return out
	}
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
	case ExprXor:
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
	case ExprFieldEquality:
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

func exprToTerms(expr Expr, fields map[string]Field, aliases map[string]Expr) ([]Term, error) {
	nnf, err := toNNF(expr, false, aliases, make(map[string]bool))
	if err != nil {
		return nil, err
	}
	terms, err := dnf(nnf, fields)
	if err != nil {
		return nil, err
	}
	return terms, nil
}

func toNNF(expr Expr, neg bool, aliases map[string]Expr, visiting map[string]bool) (Expr, error) {
	switch e := expr.(type) {
	case ExprConst:
		if neg {
			return ExprConst{Value: !e.Value}, nil
		}
		return e, nil
	case ExprIdent:
		if alias, ok := aliases[e.Name]; ok {
			if visiting[e.Name] {
				return nil, fmt.Errorf("cyclic alias %q", e.Name)
			}
			visiting[e.Name] = true
			out, err := toNNF(alias, neg, aliases, visiting)
			delete(visiting, e.Name)
			return out, err
		}
		if neg {
			return ExprNot{X: e}, nil
		}
		return e, nil
	case ExprFieldRange:
		if neg {
			return ExprNot{X: e}, nil
		}
		return e, nil
	case ExprFieldEquality:
		if neg {
			return ExprNot{X: e}, nil
		}
		return e, nil
	case ExprNot:
		return toNNF(e.X, !neg, aliases, visiting)
	case ExprAnd:
		if neg {
			left, err := toNNF(e.A, true, aliases, visiting)
			if err != nil {
				return nil, err
			}
			right, err := toNNF(e.B, true, aliases, visiting)
			if err != nil {
				return nil, err
			}
			return ExprOr{A: left, B: right}, nil
		}
		left, err := toNNF(e.A, false, aliases, visiting)
		if err != nil {
			return nil, err
		}
		right, err := toNNF(e.B, false, aliases, visiting)
		if err != nil {
			return nil, err
		}
		return ExprAnd{A: left, B: right}, nil
	case ExprOr:
		if neg {
			left, err := toNNF(e.A, true, aliases, visiting)
			if err != nil {
				return nil, err
			}
			right, err := toNNF(e.B, true, aliases, visiting)
			if err != nil {
				return nil, err
			}
			return ExprAnd{A: left, B: right}, nil
		}
		left, err := toNNF(e.A, false, aliases, visiting)
		if err != nil {
			return nil, err
		}
		right, err := toNNF(e.B, false, aliases, visiting)
		if err != nil {
			return nil, err
		}
		return ExprOr{A: left, B: right}, nil
	case ExprXor:
		// XOR(a,b) = OR(AND(a, NOT(b)), AND(NOT(a), b))
		// XNOR(a,b) = OR(AND(a, b), AND(NOT(a), NOT(b)))
		if neg {
			// XNOR
			left, err := toNNF(ExprAnd{A: e.A, B: e.B}, false, aliases, visiting)
			if err != nil {
				return nil, err
			}
			right, err := toNNF(ExprAnd{A: ExprNot{X: e.A}, B: ExprNot{X: e.B}}, false, aliases, visiting)
			if err != nil {
				return nil, err
			}
			return ExprOr{A: left, B: right}, nil
		}
		left, err := toNNF(ExprAnd{A: e.A, B: ExprNot{X: e.B}}, false, aliases, visiting)
		if err != nil {
			return nil, err
		}
		right, err := toNNF(ExprAnd{A: ExprNot{X: e.A}, B: e.B}, false, aliases, visiting)
		if err != nil {
			return nil, err
		}
		return ExprOr{A: left, B: right}, nil
	default:
		return expr, nil
	}
}

type LHSInfo struct {
	Name      string
	ActiveLow bool
	Extension string // "", "R", "T", "E"
}

func parseEquationLHS(lhs string) (LHSInfo, error) {
	lhs = strings.TrimSpace(lhs)
	if lhs == "" {
		return LHSInfo{}, fmt.Errorf("invalid equation LHS")
	}
	info := LHSInfo{}
	if strings.HasPrefix(lhs, "!") {
		info.ActiveLow = true
		lhs = strings.TrimSpace(lhs[1:])
	}
	if lhs == "" {
		return LHSInfo{}, fmt.Errorf("invalid equation LHS")
	}
	// Split on "." to extract extension
	if idx := strings.Index(lhs, "."); idx >= 0 {
		ext := strings.ToUpper(lhs[idx+1:])
		if ext == "OE" {
			ext = "E" // WinCUPL uses .OE, normalize to .E
		} else if ext == "D" {
			ext = "R" // WinCUPL uses .D for registered, normalize to .R
		}
		info.Extension = ext
		lhs = lhs[:idx]
	}
	info.Name = lhs
	return info, nil
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
		case ExprFieldEquality:
			return fieldEqualityTermsNeg(inner, fields)
		default:
			return nil, fmt.Errorf("unsupported negation of %T", inner)
		}
	case ExprFieldRange:
		return fieldRangeTerms(e, fields, false)
	case ExprFieldEquality:
		return fieldEqualityTerms(e, fields)
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
		return nil, fmt.Errorf("unsupported expression %T", expr)
	}
}

func fieldEqualityTerms(fe ExprFieldEquality, fields map[string]Field) ([]Term, error) {
	field, ok := fields[fe.Field]
	if !ok {
		return nil, fmt.Errorf("unknown field %q", fe.Field)
	}
	width := len(field.Bits)
	if width == 0 {
		return nil, fmt.Errorf("field %q has no bits", fe.Field)
	}

	// Project value and mask through the field's bit mapping
	projValue := projectValue(field, fe.Value)
	projMask := projectValue(field, fe.Mask)

	// Build a single AND term: for each care-bit, the field bit must match the value bit
	var lits []Literal
	for i := 0; i < width; i++ {
		bitPos := width - 1 - i // MSB first
		if (projMask>>bitPos)&1 == 0 {
			continue // don't-care bit
		}
		neg := (projValue>>bitPos)&1 == 0
		lits = append(lits, Literal{Name: field.Bits[i].Name, Neg: neg})
	}
	return []Term{{Lits: lits}}, nil
}

func fieldEqualityTermsNeg(fe ExprFieldEquality, fields map[string]Field) ([]Term, error) {
	field, ok := fields[fe.Field]
	if !ok {
		return nil, fmt.Errorf("unknown field %q", fe.Field)
	}
	width := len(field.Bits)
	if width == 0 {
		return nil, fmt.Errorf("field %q has no bits", fe.Field)
	}

	projValue := projectValue(field, fe.Value)
	projMask := projectValue(field, fe.Mask)

	// Negation of AND(lits) = OR of negated literals (one term per care-bit, each with that bit flipped)
	var terms []Term
	for i := 0; i < width; i++ {
		bitPos := width - 1 - i
		if (projMask>>bitPos)&1 == 0 {
			continue
		}
		// Flip this bit
		neg := (projValue>>bitPos)&1 == 1
		terms = append(terms, Term{Lits: []Literal{{Name: field.Bits[i].Name, Neg: neg}}})
	}
	return terms, nil
}

func andDNF(a, b []Term) []Term {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	var out []Term
	for _, tb := range b {
		for _, ta := range a {
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
