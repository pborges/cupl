package cupl

type Content struct {
	Meta      map[string]string
	Device    string
	Pins      map[int]PinDef
	Fields    map[string]Field
	Equations []Equation
}

type PinDef struct {
	Name      string
	ActiveLow bool
}

type Field struct {
	Name string
	Bits []FieldBit
}

type FieldBit struct {
	Name      string
	BitNumber int
	HasNumber bool
}

type Equation struct {
	Line   int
	LHS    string
	Expr   Expr
	Append bool
}

// Expr AST

type Expr interface{ isExpr() }

type ExprIdent struct{ Name string }

func (ExprIdent) isExpr() {}

type ExprNot struct{ X Expr }

func (ExprNot) isExpr() {}

type ExprAnd struct{ A, B Expr }

func (ExprAnd) isExpr() {}

type ExprOr struct{ A, B Expr }

func (ExprOr) isExpr() {}

type ExprXor struct{ A, B Expr }

func (ExprXor) isExpr() {}

type ExprConst struct{ Value bool }

func (ExprConst) isExpr() {}

type ExprFieldRange struct {
	Field string
	Lo    uint64
	Hi    uint64
}

func (ExprFieldRange) isExpr() {}

type ExprFieldEquality struct {
	Field string
	Value uint64
	Mask  uint64 // 1=care, 0=don't-care
}

func (ExprFieldEquality) isExpr() {}

// ExprIdentList represents a bracket set like [A0..3] that expands to multiple idents.
// Used for set/bus operations.
type ExprIdentList struct {
	Names []string
}

func (ExprIdentList) isExpr() {}
