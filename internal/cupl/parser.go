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

	if strings.HasPrefix(strings.ToUpper(s), "PIN ") {
		return parsePin(c, s, line)
	}
	if strings.HasPrefix(strings.ToUpper(s), "FIELD ") {
		return parseField(c, s, line)
	}

	// Equation
	return parseEquation(c, s, line)
}

func parsePin(c *Content, stmt string, line int) error {
	// Two forms:
	// Pin 1 = !ioaddr
	// Pin [16, 7, ...] = [a15..a3]
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
	// Use original case for name.
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

func parseEquation(c *Content, stmt string, line int) error {
	parts := strings.SplitN(stmt, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("line %d: invalid equation", line)
	}
	lhs := strings.TrimSpace(parts[0])
	rhs := strings.TrimSpace(parts[1])
	if lhs == "" || rhs == "" {
		return fmt.Errorf("line %d: invalid equation", line)
	}
	lex := newLexer(rhs)
	p := parser{lex: lex}
	expr, err := p.parseExpr()
	if err != nil {
		return fmt.Errorf("line %d: %w", line, err)
	}
	if tok := lex.peek(); tok.kind != tokEOF {
		return fmt.Errorf("line %d: unexpected token %q", line, tok.text)
	}
	c.Equations = append(c.Equations, Equation{Line: line, LHS: lhs, Expr: expr})
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
	parts := strings.Split(inner, "..")
	if len(parts) != 2 {
		return nil, fmt.Errorf("expected name..name range")
	}
	start := strings.TrimSpace(parts[0])
	end := strings.TrimSpace(parts[1])
	p1, n1, ok1 := splitIdentNumber(start)
	p2, n2, ok2 := splitIdentNumber(end)
	if !ok1 || !ok2 || p1 != p2 {
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
	tokLParen
	tokRParen
	tokColon
	tokLBrack
	tokRBrack
	tokDotDot
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
	case '.':
		if l.i+1 < len(l.s) && l.s[l.i+1] == '.' {
			l.i += 2
			return token{kind: tokDotDot, text: ".."}
		}
	}

	if ch == '\'' {
		// Only support 'b'0 or 'b'1
		if l.i+3 < len(l.s) && (l.s[l.i+1] == 'b' || l.s[l.i+1] == 'B') && l.s[l.i+2] == '\'' {
			v := l.s[l.i+3]
			l.i += 4
			return token{kind: tokNumber, text: string(v)}
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

func isIdentStart(b byte) bool {
	return unicode.IsLetter(rune(b)) || b == '_' || b == '$'
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

// Parser

type parser struct {
	lex *lexer
}

func (p *parser) parseExpr() (Expr, error) { return p.parseOr() }

func (p *parser) parseOr() (Expr, error) {
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

func (p *parser) parseAnd() (Expr, error) {
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

func (p *parser) parseUnary() (Expr, error) {
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

func (p *parser) parsePrimary() (Expr, error) {
	tok := p.lex.next()
	switch tok.kind {
	case tokIdent:
		// field range?
		if p.lex.peek().kind == tokColon {
			p.lex.next()
			if p.lex.next().kind != tokLBrack {
				return nil, fmt.Errorf("expected [ after :")
			}
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
		return ExprIdent{Name: tok.text}, nil
	case tokNumber:
		v, err := parseNumber(tok.text)
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
	default:
		return nil, fmt.Errorf("unexpected token %q", tok.text)
	}
}

func parseNumber(s string) (uint64, error) {
	base := 10
	for _, r := range s {
		if (r >= 'A' && r <= 'F') || (r >= 'a' && r <= 'f') {
			base = 16
			break
		}
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		base = 16
		s = s[2:]
	}
	v, err := strconv.ParseUint(s, base, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", s)
	}
	return v, nil
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
	for i, r := range s {
		if r == ';' {
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
	// offsets of each line start
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
