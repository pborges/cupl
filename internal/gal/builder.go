package gal

import "fmt"

// Active indicates output polarity.
type Active int

const (
	ActiveLow Active = iota
	ActiveHigh
)

// Mode represents the operating mode for GAL16V8.
type Mode int

const (
	ModeAuto       Mode = iota // auto-detect from blueprint
	ModeSimple                 // SYN=1, AC0=0
	ModeComplex                // SYN=1, AC0=1
	ModeRegistered             // SYN=0, AC0=1
)

type OLMC struct {
	Active     Active
	Output     *Term
	Feedback   bool
	Registered bool  // true if .R extension used
	OETerm     *Term // output enable term (complex mode / 22V10 tristate)
}

type Blueprint struct {
	Chip     Chip
	Pins     []string
	Sig      []byte
	OLMC     []OLMC
	AR       *Term // global async reset (22V10 row 0)
	SP       *Term // global sync preset (22V10 row 131)
	ModeHint Mode  // forced mode from device mnemonic (ModeAuto = auto-detect)
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

// detectMode determines the GAL16V8 operating mode from the blueprint.
func detectMode(bp Blueprint) Mode {
	if bp.ModeHint != ModeAuto {
		return bp.ModeHint
	}
	for _, olmc := range bp.OLMC {
		if olmc.Registered {
			return ModeRegistered
		}
	}
	for _, olmc := range bp.OLMC {
		if olmc.OETerm != nil {
			return ModeComplex
		}
	}
	// Check if pins 15 or 16 are used as inputs (forces complex mode).
	for _, olmc := range bp.OLMC {
		if olmc.Output != nil {
			for _, row := range olmc.Output.Pins {
				for _, pin := range row {
					if pin.Pin == 15 || pin.Pin == 16 {
						return ModeComplex
					}
				}
			}
		}
	}
	// Check if any OLMC output feeds back to another equation (complex mode).
	for _, olmc := range bp.OLMC {
		if olmc.Feedback && olmc.Output != nil {
			return ModeComplex
		}
	}
	return ModeSimple
}

// BuildGAL constructs a fuse map from a blueprint.
func BuildGAL(bp Blueprint) (*GAL, error) {
	g := NewGAL(bp.Chip)

	if bp.Chip == ChipGAL16V8 {
		mode := detectMode(bp)
		switch mode {
		case ModeSimple:
			g.SetSimpleMode()
		case ModeComplex:
			g.SetComplexMode()
		case ModeRegistered:
			g.SetRegisteredMode()
		}
	}

	setSig(g, bp.Sig)
	setTristate(g, bp)
	setXors(g, bp)

	if bp.Chip == ChipGAL22V10 {
		if err := setARSP(g, bp); err != nil {
			return nil, err
		}
	}
	if err := setCoreEqns(g, bp); err != nil {
		return nil, err
	}
	setPTs(g)
	return g, nil
}

func setSig(gal *GAL, sig []byte) {
	for i := 0; i < len(sig) && i < 8; i++ {
		c := sig[i]
		for j := 0; j < 8; j++ {
			gal.Sig[i*8+j] = (c<<j)&0x80 != 0
		}
	}
}

// setTristate configures AC1 bits for each OLMC.
// In complex/registered modes (16V8) and for 22V10, combinatorial outputs
// are implemented as tristate with OE asserted. Registered outputs get AC1=0.
func setTristate(g *GAL, bp Blueprint) {
	comIsTri := false
	if bp.Chip == ChipGAL22V10 {
		comIsTri = true
	} else if bp.Chip == ChipGAL16V8 {
		// Complex and registered modes use tristate for combinatorial outputs.
		comIsTri = g.AC0 // AC0=true in both complex and registered modes
	}

	isSimple := bp.Chip == ChipGAL16V8 && g.Syn && !g.AC0

	olmcs := len(bp.OLMC)
	for i, olmc := range bp.OLMC {
		isTri := false
		if olmc.Output == nil {
			// In simple mode, unused OLMCs are inputs (AC1=1).
			// In complex/registered modes, unused OLMCs stay AC1=0.
			if isSimple {
				isTri = true
			} else {
				isTri = olmc.Feedback
			}
		} else if olmc.Registered {
			isTri = false // registered outputs always AC1=0
		} else {
			isTri = comIsTri
		}
		if isTri {
			g.AC1[olmcs-1-i] = true
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

func setARSP(g *GAL, bp Blueprint) error {
	// AR is row 0, SP is row 131 on GAL22V10.
	if g.Chip != ChipGAL22V10 {
		return nil
	}
	if err := g.AddTermOpt(bp.AR, Bounds{StartRow: 0, MaxRows: 1, RowOffset: 0}); err != nil {
		return err
	}
	if err := g.AddTermOpt(bp.SP, Bounds{StartRow: 131, MaxRows: 1, RowOffset: 0}); err != nil {
		return err
	}
	return nil
}

func setCoreEqns(g *GAL, bp Blueprint) error {
	isComplex := bp.Chip == ChipGAL16V8 && g.Syn && g.AC0
	hasOERow := bp.Chip == ChipGAL22V10 || isComplex

	for i, olmc := range bp.OLMC {
		bounds := g.Chip.BoundsForOLMC(i)

		if hasOERow && olmc.Output != nil {
			// Row 0 is reserved for the OE/tristate term.
			if olmc.OETerm != nil {
				oeBounds := Bounds{StartRow: bounds.StartRow, MaxRows: 1, RowOffset: 0}
				if err := g.AddTerm(*olmc.OETerm, oeBounds); err != nil {
					return err
				}
			}
			// If no explicit OE term, row 0 stays all-1s (TRUE = OE always on).
			bounds.RowOffset = 1
		}

		if err := g.AddTermOpt(olmc.Output, bounds); err != nil {
			return err
		}
	}
	return nil
}
