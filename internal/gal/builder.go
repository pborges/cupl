package gal

import "fmt"

// Active indicates output polarity.
type Active int

const (
	ActiveLow Active = iota
	ActiveHigh
)

type PinMode int

const (
	PinModeCombinatorial PinMode = iota
)

type OLMC struct {
	Active   Active
	Output   *Term
	Feedback bool
}

type Blueprint struct {
	Chip Chip
	Pins []string
	Sig  []byte
	OLMC []OLMC
}

func NewBlueprint(chip Chip) Blueprint {
	olmcs := make([]OLMC, chip.NumOLMCs())
	for i := range olmcs {
		olmcs[i] = OLMC{Active: ActiveLow}
	}
	pins := make([]string, chip.NumPins())
	for i := range pins {
		pins[i] = fmt.Sprintf("PIN%d", i+1)
	}
	return Blueprint{Chip: chip, Pins: pins, OLMC: olmcs}
}

// BuildGAL constructs a fuse map from a blueprint.
func BuildGAL(bp Blueprint) (*GAL, error) {
	gal := NewGAL(bp.Chip)
	if bp.Chip == ChipGAL16V8 {
		gal.SetSimpleMode()
	}
	setSig(gal, bp.Sig)
	setTristate(gal, bp, bp.Chip == ChipGAL22V10)
	setXors(gal, bp)
	if bp.Chip == ChipGAL22V10 {
		// Default AR/SP to false.
		if err := setARSP(gal); err != nil {
			return nil, err
		}
	}
	if err := setCoreEqns(gal, bp); err != nil {
		return nil, err
	}
	setPTs(gal)
	return gal, nil
}

func setSig(gal *GAL, sig []byte) {
	for i := 0; i < len(sig) && i < 8; i++ {
		c := sig[i]
		for j := 0; j < 8; j++ {
			gal.Sig[i*8+j] = (c<<j)&0x80 != 0
		}
	}
}

// For 22V10, combinatorial outputs are implemented as tristate with enable asserted.
func setTristate(gal *GAL, bp Blueprint, comIsTri bool) {
	olmcs := len(bp.OLMC)
	for i, olmc := range bp.OLMC {
		isTri := false
		if olmc.Output == nil {
			isTri = olmc.Feedback
		} else {
			isTri = comIsTri
		}
		if isTri {
			gal.AC1[olmcs-1-i] = true
		}
	}
}

func setXors(gal *GAL, bp Blueprint) {
	olmcs := len(bp.OLMC)
	for i, olmc := range bp.OLMC {
		if olmc.Output != nil && olmc.Active == ActiveHigh {
			gal.Xor[olmcs-1-i] = true
		}
	}
}

func setPTs(gal *GAL) {
	for i := range gal.PT {
		gal.PT[i] = true
	}
}

func setARSP(gal *GAL) error {
	// AR is row 0, SP is row 131 on GAL22V10.
	if gal.Chip != ChipGAL22V10 {
		return nil
	}
	if err := gal.AddTerm(FalseTerm(0), Bounds{StartRow: 0, MaxRows: 1, RowOffset: 0}); err != nil {
		return err
	}
	if err := gal.AddTerm(FalseTerm(0), Bounds{StartRow: 131, MaxRows: 1, RowOffset: 0}); err != nil {
		return err
	}
	return nil
}

func setCoreEqns(gal *GAL, bp Blueprint) error {
	for i, olmc := range bp.OLMC {
		bounds := gal.Chip.BoundsForOLMC(i)
		if olmc.Output != nil {
			if gal.Chip == ChipGAL22V10 {
				// Row 0 is reserved for tristate enable on 22V10 outputs.
				bounds.RowOffset = 1
			}
			if err := gal.AddTerm(*olmc.Output, bounds); err != nil {
				return err
			}
		} else {
			if err := gal.AddTerm(FalseTerm(0), bounds); err != nil {
				return err
			}
		}
	}
	return nil
}
