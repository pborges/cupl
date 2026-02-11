package cupl

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

func Parse(src []byte) (Content, error) {
	text := stripComments(string(src))
	stmts := splitStatements(text)
	c := Content{
		Meta:      make(map[string]string),
		Pins:      make(map[int]PinDef),
		Fields:    make(map[string]Field),
		Equations: nil,
	}
	lineOffsets := lineOffsets(text)
	for _, st := range stmts {
		if strings.TrimSpace(st.text) == "" {
			continue
		}
		line := lineOfOffset(lineOffsets, st.offset)
		if err := parseStatement(&c, st.text, line); err != nil {
			return c, err
		}
	}
	return c, nil
}

func parseStatement(c *Content, stmt string, line int) error {
	s := strings.TrimSpace(stmt)
	if s == "" {
		return nil
	}
	upper := strings.ToUpper(s)

	// Header/meta directives
	for _, key := range []string{"NAME", "PARTNO", "REVISION", "DATE", "DESIGNER", "COMPANY", "LOCATION", "ASSEMBLY", "DEVICE"} {
		if strings.HasPrefix(upper, key+" ") || strings.EqualFold(s, key) {
			val := strings.TrimSpace(s[len(key):])
			if key == "DEVICE" {
				c.Device = strings.TrimSpace(val)
			} else {
				c.Meta[strings.Title(strings.ToLower(key))] = strings.TrimSpace(val)
			}
			return nil
		}
	}

	if strings.HasPrefix(upper, "PIN ") {
		return parsePin(c, s, line)
	}
	if strings.HasPrefix(upper, "FIELD ") {
		return parseField(c, s, line)
	}

	// APPEND keyword
	if strings.HasPrefix(upper, "APPEND ") {
		inner := strings.TrimSpace(s[7:])
		return parseEquation(c, inner, line, true)
	}

	// TABLE syntax
	if strings.HasPrefix(upper, "TABLE ") {
		return parseTable(c, s, line)
	}

	// CONDITION syntax
	if strings.HasPrefix(upper, "CONDITION ") || strings.HasPrefix(upper, "CONDITION{") {
		return parseCondition(c, s, line)
	}

	// Equation
	return parseEquation(c, s, line, false)
}

func parsePin(c *Content, stmt string, line int) error {
	s := strings.TrimSpace(stmt)
	s = strings.TrimPrefix(s, "Pin")
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("line %d: invalid pin assignment", line)
		}
		lhs := strings.TrimSpace(parts[0])
		rhs := strings.TrimSpace(parts[1])
		pins, err := parseIntList(lhs)
		if err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		bits, err := parseIdentRange(rhs)
		if err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if len(pins) != len(bits) {
			return fmt.Errorf("line %d: pin list length %d != signal list length %d", line, len(pins), len(bits))
		}
		for i, pin := range pins {
			name := bits[i]
			c.Pins[pin] = PinDef{Name: name, ActiveLow: false}
		}
		return nil
	}
	// single pin
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("line %d: invalid pin assignment", line)
	}
	pinStr := strings.TrimSpace(parts[0])
	pinNum, err := strconv.Atoi(pinStr)
	if err != nil {
		return fmt.Errorf("line %d: invalid pin number", line)
	}
	val := strings.TrimSpace(parts[1])
	activeLow := false
	if strings.HasPrefix(val, "!") {
		activeLow = true
		val = strings.TrimSpace(strings.TrimPrefix(val, "!"))
	}
	if val == "" {
		return fmt.Errorf("line %d: invalid pin name", line)
	}
	c.Pins[pinNum] = PinDef{Name: val, ActiveLow: activeLow}
	return nil
}

func parseField(c *Content, stmt string, line int) error {
	parts := strings.SplitN(strings.TrimSpace(stmt)[5:], "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("line %d: invalid field", line)
	}
	name := strings.TrimSpace(parts[0])
	bits, err := parseIdentRange(parts[1])
	if err != nil {
		return fmt.Errorf("line %d: %w", line, err)
	}
	field := Field{Name: name}
	for _, b := range bits {
		bit := FieldBit{Name: b}
		if prefix, num, ok := splitIdentNumber(b); ok {
			_ = prefix
			bit.BitNumber = num
			bit.HasNumber = true
		}
		field.Bits = append(field.Bits, bit)
	}
	c.Fields[name] = field
	return nil
}

func parseEquation(c *Content, stmt string, line int, isAppend bool) error {
	parts := strings.SplitN(stmt, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("line %d: invalid equation", line)
	}
	lhs := strings.TrimSpace(parts[0])
	rhs := strings.TrimSpace(parts[1])
	if lhs == "" || rhs == "" {
		return fmt.Errorf("line %d: invalid equation", line)
	}

	// Handle bracket LHS: [Y0..3] = expr  →  expand to per-bit equations
	if strings.HasPrefix(lhs, "[") {
		lhsIdents, err := parseIdentRange(lhs)
		if err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		// Parse RHS with bracket-set awareness
		rhsIdents := parseBracketSetRHS(rhs)
		if rhsIdents != nil && len(rhsIdents) == len(lhsIdents) {
			// Simple case: [Y0..3] = [A0..3]  (direct assignment)
			for i, lhsName := range lhsIdents {
				c.Equations = append(c.Equations, Equation{
					Line:   line,
					LHS:    lhsName,
					Expr:   rhsIdents[i],
					Append: isAppend,
				})
			}
			return nil
		}
		// Try to parse as a per-bit expression by creating a temporary field
		// and using set desugar
		// Create synthetic field for LHS
		tmpFieldName := "__set_lhs__"
		tmpField := Field{Name: tmpFieldName}
		for _, name := range lhsIdents {
			bit := FieldBit{Name: name}
			if prefix, num, ok := splitIdentNumber(name); ok {
				_ = prefix
				bit.BitNumber = num
				bit.HasNumber = true
			}
			tmpField.Bits = append(tmpField.Bits, bit)
		}

		lex := newLexer(rhs)
		p := exprParser{lex: lex}
		expr, err := p.parseExpr()
		if err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if tok := lex.peek(); tok.kind != tokEOF {
			return fmt.Errorf("line %d: unexpected token %q", line, tok.text)
		}

		// Expand to per-bit with the temporary field context
		fieldsWithTmp := make(map[string]Field)
		for k, v := range c.Fields {
			fieldsWithTmp[k] = v
		}
		fieldsWithTmp[tmpFieldName] = tmpField

		width := len(lhsIdents)
		bitExprs := exprToBitExprs(expr, width, fieldsWithTmp)
		for i, be := range bitExprs {
			c.Equations = append(c.Equations, Equation{
				Line:   line,
				LHS:    lhsIdents[i],
				Expr:   be,
				Append: isAppend,
			})
		}
		return nil
	}

	lex := newLexer(rhs)
	p := exprParser{lex: lex}
	expr, err := p.parseExpr()
	if err != nil {
		return fmt.Errorf("line %d: %w", line, err)
	}
	if tok := lex.peek(); tok.kind != tokEOF {
		return fmt.Errorf("line %d: unexpected token %q", line, tok.text)
	}
	c.Equations = append(c.Equations, Equation{Line: line, LHS: lhs, Expr: expr, Append: isAppend})
	return nil
}

// parseBracketSetRHS tries to parse RHS as a simple bracket set [A0..3]
// Returns nil if it's not a simple bracket set
func parseBracketSetRHS(rhs string) []Expr {
	rhs = strings.TrimSpace(rhs)
	if !strings.HasPrefix(rhs, "[") || !strings.HasSuffix(rhs, "]") {
		return nil
	}
	idents, err := parseIdentRange(rhs)
	if err != nil {
		return nil
	}
	exprs := make([]Expr, len(idents))
	for i, name := range idents {
		exprs[i] = ExprIdent{Name: name}
	}
	return exprs
}

func parseTable(c *Content, stmt string, line int) error {
	// TABLE <inputField> => <outputField> { val => val; ... }
	s := strings.TrimSpace(stmt)
	s = strings.TrimSpace(s[5:]) // strip "TABLE"

	braceIdx := strings.Index(s, "{")
	if braceIdx < 0 {
		return fmt.Errorf("line %d: TABLE missing {", line)
	}
	header := strings.TrimSpace(s[:braceIdx])
	body := s[braceIdx+1:]
	if idx := strings.LastIndex(body, "}"); idx >= 0 {
		body = body[:idx]
	}

	headerParts := strings.SplitN(header, "=>", 2)
	if len(headerParts) != 2 {
		return fmt.Errorf("line %d: TABLE missing =>", line)
	}
	inputFieldName := strings.TrimSpace(headerParts[0])
	outputFieldName := strings.TrimSpace(headerParts[1])

	// Parse rows
	rows := strings.Split(body, ";")
	// Track which outputs have been seen for APPEND
	seen := map[string]bool{}

	for _, row := range rows {
		row = strings.TrimSpace(row)
		if row == "" {
			continue
		}
		rowParts := strings.SplitN(row, "=>", 2)
		if len(rowParts) != 2 {
			return fmt.Errorf("line %d: TABLE invalid row %q", line, row)
		}
		inStr := strings.TrimSpace(rowParts[0])
		outStr := strings.TrimSpace(rowParts[1])

		inVal, inMask, err := parseNumberWithMask(inStr)
		if err != nil {
			return fmt.Errorf("line %d: TABLE input: %w", line, err)
		}
		outVal, _, err := parseNumberWithMask(outStr)
		if err != nil {
			return fmt.Errorf("line %d: TABLE output: %w", line, err)
		}

		// For each output bit that is set, append an equation:
		// outputBit = inputField:inVal (with mask)
		inputExpr := ExprFieldEquality{Field: inputFieldName, Value: inVal, Mask: inMask}

		// We need to know the output field's bits to figure out which output pins correspond to which bits
		// Desugar: for each output bit position where outVal has a 1, emit an APPEND equation for that bit's pin name
		c.Equations = append(c.Equations, Equation{
			Line:   line,
			LHS:    "__TABLE__" + outputFieldName,
			Expr:   inputExpr,
			Append: true,
		})
		// Store as a special marker — we'll desugar in a second pass
		// Actually, let's desugar inline: we need the field info though, which may not be set yet.
		// Let's just store raw table rows and desugar after all statements are parsed.
		// No — fields are parsed before equations in PLD files. Let's do it now.
		// Remove the marker we just added
		c.Equations = c.Equations[:len(c.Equations)-1]

		// Look up the output field
		outField, ok := c.Fields[outputFieldName]
		if !ok {
			return fmt.Errorf("line %d: TABLE unknown field %q", line, outputFieldName)
		}

		width := len(outField.Bits)
		for i := 0; i < width; i++ {
			bit := outField.Bits[i]
			bitPos := width - 1 - i // MSB first
			if (outVal>>bitPos)&1 == 1 {
				isAppend := seen[bit.Name]
				c.Equations = append(c.Equations, Equation{
					Line:   line,
					LHS:    bit.Name,
					Expr:   inputExpr,
					Append: isAppend,
				})
				seen[bit.Name] = true
			}
		}
	}
	return nil
}

func parseCondition(c *Content, stmt string, line int) error {
	// CONDITION { IF <expr> OUT <var>; ... DEFAULT OUT <var>; }
	s := strings.TrimSpace(stmt)
	// Strip "CONDITION"
	s = strings.TrimSpace(s[9:])
	if strings.HasPrefix(s, "{") {
		s = s[1:]
	}
	if idx := strings.LastIndex(s, "}"); idx >= 0 {
		s = s[:idx]
	}

	clauses := strings.Split(s, ";")
	seen := map[string]bool{}
	// First pass: collect all IF condition expressions for DEFAULT handling
	var allConditions []Expr
	type defaultClause struct {
		vars []string
	}
	type ifClause struct {
		expr Expr
		vars []string
	}
	var defaults []defaultClause
	var ifs []ifClause
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		upper := strings.ToUpper(clause)
		if strings.HasPrefix(upper, "DEFAULT") {
			rest := strings.TrimSpace(clause[7:])
			if !strings.HasPrefix(strings.ToUpper(rest), "OUT ") {
				return fmt.Errorf("line %d: CONDITION DEFAULT missing OUT", line)
			}
			rest = strings.TrimSpace(rest[4:])
			vars := strings.Split(rest, ",")
			var trimmed []string
			for _, v := range vars {
				v = strings.TrimSpace(v)
				if v != "" {
					trimmed = append(trimmed, v)
				}
			}
			defaults = append(defaults, defaultClause{vars: trimmed})
		} else if strings.HasPrefix(upper, "IF ") {
			rest := strings.TrimSpace(clause[3:])
			restUpper := strings.ToUpper(rest)
			outIdx := strings.Index(restUpper, " OUT ")
			if outIdx < 0 {
				return fmt.Errorf("line %d: CONDITION IF missing OUT", line)
			}
			exprStr := strings.TrimSpace(rest[:outIdx])
			varsStr := strings.TrimSpace(rest[outIdx+5:])
			lex := newLexer(exprStr)
			p := exprParser{lex: lex}
			expr, err := p.parseExpr()
			if err != nil {
				return fmt.Errorf("line %d: CONDITION expr: %w", line, err)
			}
			allConditions = append(allConditions, expr)
			vars := strings.Split(varsStr, ",")
			var trimmed []string
			for _, v := range vars {
				v = strings.TrimSpace(v)
				if v != "" {
					trimmed = append(trimmed, v)
				}
			}
			ifs = append(ifs, ifClause{expr: expr, vars: trimmed})
		} else {
			return fmt.Errorf("line %d: CONDITION unexpected clause %q", line, clause)
		}
	}
	// Emit IF equations
	for _, ic := range ifs {
		for _, v := range ic.vars {
			isAppend := seen[v]
			c.Equations = append(c.Equations, Equation{
				Line:   line,
				LHS:    v,
				Expr:   ic.expr,
				Append: isAppend,
			})
			seen[v] = true
		}
	}
	// Emit DEFAULT equations: DEFAULT = AND of NOT(each IF condition)
	if len(defaults) > 0 {
		var defaultExpr Expr
		for i, cond := range allConditions {
			negCond := ExprNot{X: cond}
			if i == 0 {
				defaultExpr = negCond
			} else {
				defaultExpr = ExprAnd{A: defaultExpr, B: negCond}
			}
		}
		if defaultExpr == nil {
			defaultExpr = ExprConst{Value: true}
		}
		for _, dc := range defaults {
			for _, v := range dc.vars {
				isAppend := seen[v]
				c.Equations = append(c.Equations, Equation{
					Line:   line,
					LHS:    v,
					Expr:   defaultExpr,
					Append: isAppend,
				})
				seen[v] = true
			}
		}
	}
	return nil
}

func parseIntList(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("expected [..] list")
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if inner == "" {
		return nil, fmt.Errorf("empty list")
	}
	parts := strings.Split(inner, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, fmt.Errorf("invalid integer %q", p)
		}
		if strings.Contains(p, "..") {
			rangeParts := strings.Split(p, "..")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid integer %q", p)
			}
			startStr := strings.TrimSpace(rangeParts[0])
			endStr := strings.TrimSpace(rangeParts[1])
			if startStr == "" || endStr == "" {
				return nil, fmt.Errorf("invalid integer %q", p)
			}
			start, err := strconv.Atoi(startStr)
			if err != nil {
				return nil, fmt.Errorf("invalid integer %q", p)
			}
			end, err := strconv.Atoi(endStr)
			if err != nil {
				return nil, fmt.Errorf("invalid integer %q", p)
			}
			if start <= end {
				for i := start; i <= end; i++ {
					out = append(out, i)
				}
			} else {
				for i := start; i >= end; i-- {
					out = append(out, i)
				}
			}
			continue
		}
		v, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q", p)
		}
		out = append(out, v)
	}
	return out, nil
}

func parseIdentRange(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("expected [..] range")
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])

	// Check for comma-separated list (no "..")
	if !strings.Contains(inner, "..") && strings.Contains(inner, ",") {
		parts := strings.Split(inner, ",")
		var out []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				return nil, fmt.Errorf("empty identifier in list")
			}
			out = append(out, p)
		}
		return out, nil
	}

	parts := strings.Split(inner, "..")
	if len(parts) != 2 {
		return nil, fmt.Errorf("expected name..name range")
	}
	start := strings.TrimSpace(parts[0])
	end := strings.TrimSpace(parts[1])
	p1, n1, ok1 := splitIdentNumber(start)
	if !ok1 {
		return nil, fmt.Errorf("range start %q must have numeric suffix", start)
	}

	p2, n2, ok2 := splitIdentNumber(end)
	if !ok2 || (ok2 && p2 == "") {
		// End has no prefix — it's just a number (e.g., [A0..3])
		// Try parsing end as a plain number and inherit prefix from start
		num, err := strconv.Atoi(end)
		if err != nil {
			return nil, fmt.Errorf("range end %q must have numeric suffix or be a number", end)
		}
		p2 = p1
		n2 = num
		ok2 = true
	}

	if p1 != p2 {
		return nil, fmt.Errorf("range must use same prefix with numeric suffix")
	}
	var out []string
	if n1 <= n2 {
		for i := n1; i <= n2; i++ {
			out = append(out, fmt.Sprintf("%s%d", p1, i))
		}
	} else {
		for i := n1; i >= n2; i-- {
			out = append(out, fmt.Sprintf("%s%d", p1, i))
		}
	}
	return out, nil
}

func splitIdentNumber(s string) (string, int, bool) {
	var prefix []rune
	var digits []rune
	for i, r := range s {
		if unicode.IsDigit(r) {
			digits = append(digits, []rune(s[i:])...)
			break
		}
		prefix = append(prefix, r)
	}
	if len(digits) == 0 {
		return "", 0, false
	}
	v, err := strconv.Atoi(string(digits))
	if err != nil {
		return "", 0, false
	}
	return string(prefix), v, true
}

// Lexer for expressions

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokNumber
	tokNot
	tokAnd
	tokOr
	tokXor
	tokLParen
	tokRParen
	tokColon
	tokLBrack
	tokRBrack
	tokDotDot
	tokComma
	tokArrow // =>
)

type token struct {
	kind tokenKind
	text string
}

type lexer struct {
	s string
	i int
}

func newLexer(s string) *lexer { return &lexer{s: s} }

func (l *lexer) peek() token {
	pos := l.i
	tok := l.next()
	l.i = pos
	return tok
}

func (l *lexer) next() token {
	for l.i < len(l.s) && unicode.IsSpace(rune(l.s[l.i])) {
		l.i++
	}
	if l.i >= len(l.s) {
		return token{kind: tokEOF}
	}
	ch := l.s[l.i]
	switch ch {
	case '!':
		l.i++
		return token{kind: tokNot, text: "!"}
	case '&':
		l.i++
		return token{kind: tokAnd, text: "&"}
	case '|':
		l.i++
		return token{kind: tokOr, text: "|"}
	case '#':
		l.i++
		return token{kind: tokOr, text: "#"}
	case '$':
		l.i++
		return token{kind: tokXor, text: "$"}
	case '(':
		l.i++
		return token{kind: tokLParen, text: "("}
	case ')':
		l.i++
		return token{kind: tokRParen, text: ")"}
	case ':':
		l.i++
		return token{kind: tokColon, text: ":"}
	case '[':
		l.i++
		return token{kind: tokLBrack, text: "["}
	case ']':
		l.i++
		return token{kind: tokRBrack, text: "]"}
	case ',':
		l.i++
		return token{kind: tokComma, text: ","}
	case '.':
		if l.i+1 < len(l.s) && l.s[l.i+1] == '.' {
			l.i += 2
			return token{kind: tokDotDot, text: ".."}
		}
	case '=':
		if l.i+1 < len(l.s) && l.s[l.i+1] == '>' {
			l.i += 2
			return token{kind: tokArrow, text: "=>"}
		}
	}

	if ch == '\'' {
		// Parse '<base>'<digits> fully
		if l.i+2 < len(l.s) {
			base := l.s[l.i+1]
			if l.i+2 < len(l.s) && l.s[l.i+2] == '\'' {
				// '<base>'<digits...>
				l.i += 3 // skip '<base>'
				start := l.i
				for l.i < len(l.s) && isBaseDigit(l.s[l.i], base) {
					l.i++
				}
				digits := l.s[start:l.i]
				return token{kind: tokNumber, text: fmt.Sprintf("'%c'%s", base, digits)}
			}
		}
	}

	if isIdentStart(ch) {
		start := l.i
		l.i++
		for l.i < len(l.s) && isIdentPart(l.s[l.i]) {
			l.i++
		}
		return token{kind: tokIdent, text: l.s[start:l.i]}
	}
	if isNumberStart(ch) {
		start := l.i
		l.i++
		for l.i < len(l.s) && isNumberPart(l.s[l.i]) {
			l.i++
		}
		return token{kind: tokNumber, text: l.s[start:l.i]}
	}

	l.i++
	return token{kind: tokEOF}
}

func isBaseDigit(b byte, base byte) bool {
	switch base {
	case 'b', 'B':
		return b == '0' || b == '1' || b == 'X' || b == 'x'
	case 'h', 'H':
		return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'F') || (b >= 'a' && b <= 'f') || b == 'X' || b == 'x'
	case 'o', 'O':
		return (b >= '0' && b <= '7') || b == 'X' || b == 'x'
	case 'd', 'D':
		return (b >= '0' && b <= '9') || b == 'X' || b == 'x'
	default:
		return false
	}
}

func isIdentStart(b byte) bool {
	return unicode.IsLetter(rune(b)) || b == '_'
}

func isIdentPart(b byte) bool {
	return isIdentStart(b) || unicode.IsDigit(rune(b))
}

func isNumberStart(b byte) bool {
	return unicode.IsDigit(rune(b))
}

func isNumberPart(b byte) bool {
	return unicode.IsDigit(rune(b)) || (b >= 'A' && b <= 'F') || (b >= 'a' && b <= 'f') || b == 'x' || b == 'X'
}

// Expression parser

type exprParser struct {
	lex *lexer
}

// Precedence (lowest to highest): XOR < OR < AND < NOT
func (p *exprParser) parseExpr() (Expr, error) { return p.parseXor() }

func (p *exprParser) parseXor() (Expr, error) {
	left, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	for {
		tok := p.lex.peek()
		if tok.kind != tokXor {
			break
		}
		p.lex.next()
		right, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		left = ExprXor{A: left, B: right}
	}
	return left, nil
}

func (p *exprParser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		tok := p.lex.peek()
		if tok.kind != tokOr {
			break
		}
		p.lex.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = ExprOr{A: left, B: right}
	}
	return left, nil
}

func (p *exprParser) parseAnd() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		tok := p.lex.peek()
		if tok.kind != tokAnd {
			break
		}
		p.lex.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = ExprAnd{A: left, B: right}
	}
	return left, nil
}

func (p *exprParser) parseUnary() (Expr, error) {
	tok := p.lex.peek()
	if tok.kind == tokNot {
		p.lex.next()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return ExprNot{X: x}, nil
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() (Expr, error) {
	tok := p.lex.next()
	switch tok.kind {
	case tokIdent:
		// Check for field operations: ident:value or ident:[range]
		if p.lex.peek().kind == tokColon {
			p.lex.next()
			next := p.lex.peek()
			if next.kind == tokLBrack {
				// field:[lo..hi] range
				p.lex.next()
				loTok := p.lex.next()
				if loTok.kind != tokNumber && loTok.kind != tokIdent {
					return nil, fmt.Errorf("expected number in range")
				}
				if p.lex.next().kind != tokDotDot {
					return nil, fmt.Errorf("expected .. in range")
				}
				hiTok := p.lex.next()
				if hiTok.kind != tokNumber && hiTok.kind != tokIdent {
					return nil, fmt.Errorf("expected number in range")
				}
				if p.lex.next().kind != tokRBrack {
					return nil, fmt.Errorf("expected ] in range")
				}
				lo, err := parseNumber(loTok.text)
				if err != nil {
					return nil, err
				}
				hi, err := parseNumber(hiTok.text)
				if err != nil {
					return nil, err
				}
				return ExprFieldRange{Field: tok.text, Lo: lo, Hi: hi}, nil
			}
			if next.kind == tokNumber {
				// field:value — field equality
				valTok := p.lex.next()
				val, mask, err := parseNumberWithMask(valTok.text)
				if err != nil {
					return nil, err
				}
				return ExprFieldEquality{Field: tok.text, Value: val, Mask: mask}, nil
			}
			return nil, fmt.Errorf("expected [ or number after :")
		}
		return ExprIdent{Name: tok.text}, nil

	case tokNumber:
		v, _, err := parseNumberWithMask(tok.text)
		if err != nil {
			return nil, err
		}
		return ExprConst{Value: v != 0}, nil

	case tokLParen:
		x, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.lex.next().kind != tokRParen {
			return nil, fmt.Errorf("expected )")
		}
		return x, nil

	case tokLBrack:
		// [ident..ident] set expression or reduction operator
		return p.parseBracketExpr()

	default:
		return nil, fmt.Errorf("unexpected token %q", tok.text)
	}
}

// parseBracketExpr parses [A3..0] or [a, b, c] with optional :op reduction
func (p *exprParser) parseBracketExpr() (Expr, error) {
	// We've already consumed the '['
	// Collect tokens until ']'
	var idents []string

	firstTok := p.lex.next()
	if firstTok.kind != tokIdent && firstTok.kind != tokNumber {
		return nil, fmt.Errorf("expected identifier in bracket expression")
	}

	next := p.lex.peek()
	if next.kind == tokDotDot {
		// Range: [A3..A0] or [A3..0]
		p.lex.next() // consume ..
		endTok := p.lex.next()
		if endTok.kind != tokIdent && endTok.kind != tokNumber {
			return nil, fmt.Errorf("expected identifier or number after ..")
		}
		if p.lex.next().kind != tokRBrack {
			return nil, fmt.Errorf("expected ] in bracket expression")
		}

		// Build ident list from range
		startStr := firstTok.text
		endStr := endTok.text
		p1, n1, ok1 := splitIdentNumber(startStr)
		if !ok1 {
			return nil, fmt.Errorf("range start %q must have numeric suffix", startStr)
		}
		p2, n2, ok2 := splitIdentNumber(endStr)
		if !ok2 || (ok2 && p2 == "") {
			num, err := strconv.Atoi(endStr)
			if err != nil {
				return nil, fmt.Errorf("range end %q must have numeric suffix or be a number", endStr)
			}
			p2 = p1
			n2 = num
		}
		if p1 != p2 {
			return nil, fmt.Errorf("range must use same prefix")
		}
		if n1 >= n2 {
			for i := n1; i >= n2; i-- {
				idents = append(idents, fmt.Sprintf("%s%d", p1, i))
			}
		} else {
			for i := n1; i <= n2; i++ {
				idents = append(idents, fmt.Sprintf("%s%d", p1, i))
			}
		}
	} else if next.kind == tokComma {
		// Comma-separated list: [a, b, c]
		idents = append(idents, firstTok.text)
		for p.lex.peek().kind == tokComma {
			p.lex.next() // consume ,
			idTok := p.lex.next()
			if idTok.kind != tokIdent && idTok.kind != tokNumber {
				return nil, fmt.Errorf("expected identifier in list")
			}
			idents = append(idents, idTok.text)
		}
		if p.lex.next().kind != tokRBrack {
			return nil, fmt.Errorf("expected ] in bracket expression")
		}
	} else if next.kind == tokRBrack {
		// Single element [A]
		p.lex.next()
		idents = append(idents, firstTok.text)
	} else {
		return nil, fmt.Errorf("expected .., comma, or ] in bracket expression")
	}

	// Check for reduction operator :& :# :$
	if p.lex.peek().kind == tokColon {
		p.lex.next() // consume :
		opTok := p.lex.next()
		switch opTok.kind {
		case tokAnd:
			return reduceIdents(idents, func(a, b Expr) Expr { return ExprAnd{A: a, B: b} }), nil
		case tokOr:
			return reduceIdents(idents, func(a, b Expr) Expr { return ExprOr{A: a, B: b} }), nil
		case tokXor:
			return reduceIdents(idents, func(a, b Expr) Expr { return ExprXor{A: a, B: b} }), nil
		default:
			return nil, fmt.Errorf("expected &, #, or $ after : for reduction")
		}
	}

	if len(idents) == 1 {
		return ExprIdent{Name: idents[0]}, nil
	}
	return ExprIdentList{Names: idents}, nil
}

func reduceIdents(idents []string, op func(a, b Expr) Expr) Expr {
	if len(idents) == 0 {
		return ExprConst{Value: false}
	}
	result := Expr(ExprIdent{Name: idents[0]})
	for i := 1; i < len(idents); i++ {
		result = op(result, ExprIdent{Name: idents[i]})
	}
	return result
}

func parseNumber(s string) (uint64, error) {
	v, _, err := parseNumberWithMask(s)
	return v, err
}

// parseNumberWithMask parses a number, returning value and care-mask.
// For numbers with don't-care X digits, the corresponding mask bits are 0.
func parseNumberWithMask(s string) (uint64, uint64, error) {
	// Check for CUPL base prefix: 'b'digits, 'h'digits, 'o'digits, 'd'digits
	if len(s) >= 3 && s[0] == '\'' {
		quoteIdx := strings.Index(s[1:], "'")
		if quoteIdx >= 0 {
			base := s[1 : 1+quoteIdx]
			digits := s[1+quoteIdx+1:]
			return parseBasedNumber(strings.ToLower(base), digits)
		}
	}

	// Default: hex
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}

	// Check for don't-cares
	if strings.ContainsAny(s, "Xx") {
		return parseBasedNumber("h", s)
	}

	v, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid number %q", s)
	}
	// All bits are care bits
	width := len(s) * 4
	mask := uint64(1<<width) - 1
	if mask == 0 {
		mask = ^uint64(0)
	}
	return v, mask, nil
}

func parseBasedNumber(base string, digits string) (uint64, uint64, error) {
	var bitsPerDigit int
	var digitMax byte
	switch base {
	case "b":
		bitsPerDigit = 1
		digitMax = '1'
	case "h":
		bitsPerDigit = 4
		digitMax = 0 // handled specially
	case "o":
		bitsPerDigit = 3
		digitMax = '7'
	case "d":
		// Decimal: no don't-care support, parse directly
		v, err := strconv.ParseUint(digits, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid decimal number %q", digits)
		}
		return v, ^uint64(0), nil
	default:
		return 0, 0, fmt.Errorf("unknown base %q", base)
	}

	var value, mask uint64
	for _, ch := range []byte(digits) {
		if ch == '_' {
			continue // allow underscores as separators
		}
		if ch == 'X' || ch == 'x' {
			value <<= uint(bitsPerDigit)
			mask <<= uint(bitsPerDigit)
			// don't-care: mask bits stay 0
			continue
		}
		var digitVal uint64
		if base == "h" {
			switch {
			case ch >= '0' && ch <= '9':
				digitVal = uint64(ch - '0')
			case ch >= 'A' && ch <= 'F':
				digitVal = uint64(ch-'A') + 10
			case ch >= 'a' && ch <= 'f':
				digitVal = uint64(ch-'a') + 10
			default:
				return 0, 0, fmt.Errorf("invalid hex digit %c", ch)
			}
		} else {
			if ch < '0' || ch > digitMax {
				return 0, 0, fmt.Errorf("invalid digit %c for base %s", ch, base)
			}
			digitVal = uint64(ch - '0')
		}
		value = (value << uint(bitsPerDigit)) | digitVal
		mask = (mask << uint(bitsPerDigit)) | ((1 << uint(bitsPerDigit)) - 1)
	}
	return value, mask, nil
}

// Helpers

func stripComments(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				if s[i] == '\n' {
					out.WriteByte('\n')
				}
				i++
			}
			if i+1 < len(s) {
				i += 2
			}
			continue
		}
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '/' {
			i += 2
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

type statement struct {
	text   string
	offset int
}

func splitStatements(s string) []statement {
	var stmts []statement
	var buf strings.Builder
	start := 0
	depth := 0
	for i, r := range s {
		if r == '{' {
			depth++
			buf.WriteRune(r)
			continue
		}
		if r == '}' {
			depth--
			buf.WriteRune(r)
			continue
		}
		if r == ';' && depth <= 0 {
			stmts = append(stmts, statement{text: buf.String(), offset: start})
			buf.Reset()
			start = i + 1
			continue
		}
		buf.WriteRune(r)
	}
	if buf.Len() > 0 {
		stmts = append(stmts, statement{text: buf.String(), offset: start})
	}
	return stmts
}

func lineOffsets(s string) []int {
	offs := []int{0}
	for i, r := range s {
		if r == '\n' {
			offs = append(offs, i+1)
		}
	}
	return offs
}

func lineOfOffset(lines []int, off int) int {
	line := 1
	for i := 0; i < len(lines); i++ {
		if lines[i] > off {
			return line
		}
		line++
	}
	return line
}
