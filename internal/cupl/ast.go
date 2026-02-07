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
	Line int
	LHS  string
	Expr Expr
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

type ExprConst struct{ Value bool }

func (ExprConst) isExpr() {}

type ExprFieldRange struct {
	Field string
	Lo    uint64
	Hi    uint64
}

func (ExprFieldRange) isExpr() {}
