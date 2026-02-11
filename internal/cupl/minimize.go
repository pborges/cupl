package cupl

import "sort"

// minimizeTerms applies Quine-McCluskey minimization to reduce the number
// of product terms. This finds all prime implicants, then selects a minimum
// cover using essential prime implicants followed by greedy selection.
func minimizeTerms(terms []Term) []Term {
	if len(terms) <= 1 {
		return terms
	}
	// Short-circuit if any term is TRUE (empty literals = always true)
	for _, t := range terms {
		if len(t.Lits) == 0 {
			return terms
		}
	}

	// Convert terms to implicant representation for efficient comparison
	vars, varIndex := collectVars(terms)
	if len(vars) == 0 {
		return terms
	}

	numVars := len(vars)

	// Convert input terms to implicants and expand to minterms
	inputImps := make([]implicant, len(terms))
	for i, t := range terms {
		inputImps[i] = termToImplicant(t, varIndex)
	}

	// Expand all implicants to their constituent minterms
	mintermSet := make(map[uint64]bool)
	for _, imp := range inputImps {
		expandMinterms(imp, numVars, &mintermSet)
	}
	if len(mintermSet) == 0 {
		return terms
	}

	// Convert to sorted minterm list
	minterms := make([]uint64, 0, len(mintermSet))
	for m := range mintermSet {
		minterms = append(minterms, m)
	}
	sort.Slice(minterms, func(i, j int) bool { return minterms[i] < minterms[j] })

	// Find all prime implicants via Quine-McCluskey
	primes := findPrimeImplicants(minterms, numVars)

	// Select minimum cover
	selected := minimumCover(primes, minterms, numVars)

	if len(selected) < len(terms) {
		// QM reduced term count — use QM result, sort descending
		sort.Slice(selected, func(i, j int) bool {
			if selected[i].value != selected[j].value {
				return selected[i].value > selected[j].value
			}
			return selected[i].mask > selected[j].mask
		})
		return implicantsToTerms(selected, vars)
	}

	// QM didn't reduce — keep original terms, sort ascending
	sort.Slice(inputImps, func(i, j int) bool {
		if inputImps[i].value != inputImps[j].value {
			return inputImps[i].value < inputImps[j].value
		}
		return inputImps[i].mask < inputImps[j].mask
	})
	return implicantsToTerms(inputImps, vars)
}

// implicant represents a product term using bitmasks.
// value holds the bit values for care positions; mask has 1=care, 0=don't-care.
type implicant struct {
	value uint64
	mask  uint64
}

// termToImplicant converts a Term to bitmask representation.
func termToImplicant(t Term, varIndex map[string]int) implicant {
	var value, mask uint64
	for _, l := range t.Lits {
		bit := uint64(1) << varIndex[l.Name]
		mask |= bit
		if !l.Neg {
			value |= bit
		}
	}
	return implicant{value: value, mask: mask}
}

// expandMinterms expands an implicant (which may have don't-care bits) into
// all its constituent minterms over numVars variables.
func expandMinterms(imp implicant, numVars int, out *map[uint64]bool) {
	// Collect don't-care bit positions within variable range
	var dcBits []int
	for b := 0; b < numVars; b++ {
		if imp.mask&(uint64(1)<<b) == 0 {
			dcBits = append(dcBits, b)
		}
	}

	// Base value: care bits are fixed
	base := imp.value & imp.mask

	// Enumerate all combinations of don't-care bits
	if len(dcBits) > 20 {
		// Safety: too many don't-cares, skip expansion
		(*out)[base] = true
		return
	}

	n := 1 << len(dcBits)
	for i := 0; i < n; i++ {
		m := base
		for j, bit := range dcBits {
			if i&(1<<j) != 0 {
				m |= uint64(1) << bit
			}
		}
		(*out)[m] = true
	}
}

// findPrimeImplicants implements the QM merge phase.
// Groups implicants by popcount and iteratively merges pairs that differ
// in exactly one bit, collecting all unmerged implicants as prime implicants.
func findPrimeImplicants(minterms []uint64, numVars int) []implicant {
	fullMask := uint64((1 << numVars) - 1)

	// Start with minterms as implicants (fully specified)
	current := make(map[implicant]bool)
	for _, m := range minterms {
		current[implicant{value: m & fullMask, mask: fullMask}] = true
	}

	primeSet := make(map[implicant]bool)

	for len(current) > 0 {
		merged := make(map[implicant]bool)
		used := make(map[implicant]bool)

		// Convert to slice for iteration
		impList := make([]implicant, 0, len(current))
		for imp := range current {
			impList = append(impList, imp)
		}

		// Try all pairs
		for i := 0; i < len(impList); i++ {
			for j := i + 1; j < len(impList); j++ {
				if m, ok := tryMerge(impList[i], impList[j]); ok {
					merged[m] = true
					used[impList[i]] = true
					used[impList[j]] = true
				}
			}
		}

		// Unmerged implicants are prime
		for _, imp := range impList {
			if !used[imp] {
				primeSet[imp] = true
			}
		}

		current = merged
	}

	// Convert to sorted slice for deterministic output
	primes := make([]implicant, 0, len(primeSet))
	for p := range primeSet {
		primes = append(primes, p)
	}
	sort.Slice(primes, func(i, j int) bool {
		if primes[i].mask != primes[j].mask {
			return primes[i].mask > primes[j].mask
		}
		return primes[i].value > primes[j].value
	})

	return primes
}

// tryMerge attempts to merge two implicants that have the same mask (same
// set of care variables) and differ in exactly one variable's polarity.
func tryMerge(a, b implicant) (implicant, bool) {
	if a.mask != b.mask {
		return implicant{}, false
	}
	diff := (a.value ^ b.value) & a.mask
	if diff == 0 || (diff&(diff-1)) != 0 {
		return implicant{}, false // 0 or >1 bits differ
	}
	// Exactly one bit differs — merge by removing that variable
	return implicant{
		value: a.value &^ diff,
		mask:  a.mask &^ diff,
	}, true
}

// minimumCover selects a minimum set of prime implicants that cover all minterms.
// Uses essential prime implicants first, then greedy selection.
func minimumCover(primes []implicant, minterms []uint64, numVars int) []implicant {
	if len(primes) == 0 {
		return nil
	}

	// Build coverage: which primes cover which minterms
	mintermIdx := make(map[uint64]int, len(minterms))
	for i, m := range minterms {
		mintermIdx[m] = i
	}

	// For each prime, find which minterms it covers
	type primeInfo struct {
		imp     implicant
		covers  map[int]bool // indices into minterms
	}
	pInfos := make([]primeInfo, len(primes))
	for i, p := range primes {
		pInfos[i] = primeInfo{imp: p, covers: make(map[int]bool)}
		// Expand this prime to minterms and check which are in our set
		var expanded map[uint64]bool = make(map[uint64]bool)
		expandMinterms(p, numVars, &expanded)
		for m := range expanded {
			if idx, ok := mintermIdx[m]; ok {
				pInfos[i].covers[idx] = true
			}
		}
	}

	uncovered := make([]bool, len(minterms))
	for i := range uncovered {
		uncovered[i] = true
	}
	uncoveredCount := len(minterms)

	var selected []implicant

	// Phase 1: Find essential prime implicants
	// A PI is essential if it's the only one covering some minterm.
	// Iterate minterms in order for deterministic results.
	for changed := true; changed; {
		changed = false
		for mi := 0; mi < len(minterms); mi++ {
			if !uncovered[mi] {
				continue
			}
			sole := -1
			for pi, p := range pInfos {
				if p.covers == nil {
					continue
				}
				if p.covers[mi] {
					if sole >= 0 {
						sole = -1
						break // more than one covers this minterm
					}
					sole = pi
				}
			}
			if sole >= 0 {
				// Essential PI
				selected = append(selected, pInfos[sole].imp)
				for mi2 := range pInfos[sole].covers {
					if uncovered[mi2] {
						uncovered[mi2] = false
						uncoveredCount--
					}
				}
				pInfos[sole].covers = nil
				changed = true
			}
		}
	}

	// Phase 2: Greedy cover for remaining minterms
	for uncoveredCount > 0 {
		bestPI := -1
		bestCount := 0
		for pi, p := range pInfos {
			if p.covers == nil {
				continue
			}
			count := 0
			for mi := range p.covers {
				if uncovered[mi] {
					count++
				}
			}
			if count > bestCount {
				bestCount = count
				bestPI = pi
			}
		}
		if bestPI < 0 {
			break
		}
		selected = append(selected, pInfos[bestPI].imp)
		for mi := range pInfos[bestPI].covers {
			if uncovered[mi] {
				uncovered[mi] = false
				uncoveredCount--
			}
		}
		pInfos[bestPI].covers = nil
	}

	return selected
}

// collectVars gathers sorted unique variable names and builds an index map.
func collectVars(terms []Term) ([]string, map[string]int) {
	seen := make(map[string]bool)
	for _, t := range terms {
		for _, l := range t.Lits {
			seen[l.Name] = true
		}
	}
	vars := make([]string, 0, len(seen))
	for v := range seen {
		vars = append(vars, v)
	}
	sort.Strings(vars)
	idx := make(map[string]int, len(vars))
	for i, v := range vars {
		idx[v] = i
	}
	return vars, idx
}

// implicantsToTerms converts implicants back to Terms with sorted literals.
func implicantsToTerms(imps []implicant, vars []string) []Term {
	terms := make([]Term, 0, len(imps))
	for _, imp := range imps {
		var lits []Literal
		for i, v := range vars {
			bit := uint64(1) << i
			if imp.mask&bit == 0 {
				continue // don't-care
			}
			lits = append(lits, Literal{
				Name: v,
				Neg:  imp.value&bit == 0,
			})
		}
		sort.Slice(lits, func(i, j int) bool { return lits[i].Name < lits[j].Name })
		terms = append(terms, Term{Lits: lits})
	}
	return terms
}
