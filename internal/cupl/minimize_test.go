package cupl

import (
	"reflect"
	"sort"
	"testing"
)

func sortTerms(terms []Term) {
	for i := range terms {
		sort.Slice(terms[i].Lits, func(a, b int) bool {
			return terms[i].Lits[a].Name < terms[i].Lits[b].Name
		})
	}
	sort.Slice(terms, func(i, j int) bool {
		a, b := terms[i], terms[j]
		minLen := len(a.Lits)
		if len(b.Lits) < minLen {
			minLen = len(b.Lits)
		}
		for k := 0; k < minLen; k++ {
			if a.Lits[k].Name != b.Lits[k].Name {
				return a.Lits[k].Name < b.Lits[k].Name
			}
			if a.Lits[k].Neg != b.Lits[k].Neg {
				return !a.Lits[k].Neg
			}
		}
		return len(a.Lits) < len(b.Lits)
	})
}

func TestMinimizeTerms_ABnB_OR_AB(t *testing.T) {
	// A&!B # A&B → A
	terms := []Term{
		{Lits: []Literal{{Name: "A"}, {Name: "B", Neg: true}}},
		{Lits: []Literal{{Name: "A"}, {Name: "B"}}},
	}
	result := minimizeTerms(terms)
	expected := []Term{
		{Lits: []Literal{{Name: "A"}}},
	}
	sortTerms(result)
	sortTerms(expected)
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestMinimizeTerms_ManyPterms_Y0(t *testing.T) {
	// 4 minterms of A with all combos of B,C → should reduce to just A
	terms := []Term{
		{Lits: []Literal{{Name: "A"}, {Name: "B", Neg: true}, {Name: "C", Neg: true}}},
		{Lits: []Literal{{Name: "A"}, {Name: "B"}, {Name: "C", Neg: true}}},
		{Lits: []Literal{{Name: "A"}, {Name: "B", Neg: true}, {Name: "C"}}},
		{Lits: []Literal{{Name: "A"}, {Name: "B"}, {Name: "C"}}},
	}
	result := minimizeTerms(terms)
	expected := []Term{
		{Lits: []Literal{{Name: "A"}}},
	}
	sortTerms(result)
	sortTerms(expected)
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestMinimizeTerms_SingleTerm(t *testing.T) {
	terms := []Term{
		{Lits: []Literal{{Name: "A"}, {Name: "B"}}},
	}
	result := minimizeTerms(terms)
	if !reflect.DeepEqual(result, terms) {
		t.Errorf("got %v, want %v", result, terms)
	}
}

func TestMinimizeTerms_Empty(t *testing.T) {
	var terms []Term
	result := minimizeTerms(terms)
	if len(result) != 0 {
		t.Errorf("got %v, want empty", result)
	}
}

func TestMinimizeTerms_TRUE(t *testing.T) {
	// TRUE term (empty lits) should short-circuit
	terms := []Term{
		{Lits: []Literal{}},
		{Lits: []Literal{{Name: "A"}}},
	}
	result := minimizeTerms(terms)
	if !reflect.DeepEqual(result, terms) {
		t.Errorf("got %v, want %v (unchanged)", result, terms)
	}
}

func TestMinimizeTerms_Subsumption(t *testing.T) {
	// A # A&B → A (A subsumes A&B)
	terms := []Term{
		{Lits: []Literal{{Name: "A"}}},
		{Lits: []Literal{{Name: "A"}, {Name: "B"}}},
	}
	result := minimizeTerms(terms)
	expected := []Term{
		{Lits: []Literal{{Name: "A"}}},
	}
	sortTerms(result)
	sortTerms(expected)
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}
