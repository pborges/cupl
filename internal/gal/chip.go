package gal

import (
	"fmt"
	"strings"
)

type Chip int

const (
	ChipUnknown Chip = iota
	ChipGAL16V8
	ChipGAL22V10
)

type chipData struct {
	name      string
	numPins   int
	numRows   int
	numCols   int
	totalSize int
	minOLMC   int
	maxOLMC   int
	olmcMap   []int
}

var (
	chip16v8 = chipData{
		name:      "GAL16V8",
		numPins:   20,
		numRows:   64,
		numCols:   32,
		totalSize: 2194,
		minOLMC:   12,
		maxOLMC:   19,
		olmcMap:   []int{56, 48, 40, 32, 24, 16, 8, 0},
	}
	chip22v10 = chipData{
		name:      "GAL22V10",
		numPins:   24,
		numRows:   132,
		numCols:   44,
		totalSize: 5892,
		minOLMC:   14,
		maxOLMC:   23,
		olmcMap:   []int{122, 111, 98, 83, 66, 49, 34, 21, 10, 1},
	}
)

var olmcSize22v10 = []int{9, 11, 13, 15, 17, 17, 15, 13, 11, 9}

func ParseChip(name string) (Chip, error) {
	n := normalizeDevice(name)
	switch {
	case strings.Contains(n, "16V8"):
		return ChipGAL16V8, nil
	case strings.Contains(n, "22V10"):
		return ChipGAL22V10, nil
	default:
		return ChipUnknown, fmt.Errorf("unsupported device: %s", name)
	}
}

func normalizeDevice(name string) string {
	// Accept CUPL-style names like g16v8as, g22v10.
	// Normalize to GALxxVx for internal use.
	var buf []rune
	for _, r := range name {
		if r >= 'A' && r <= 'Z' {
			buf = append(buf, r)
			continue
		}
		if r >= 'a' && r <= 'z' {
			buf = append(buf, r-('a'-'A'))
			continue
		}
		if r >= '0' && r <= '9' {
			buf = append(buf, r)
		}
	}
	upper := string(buf)
	if len(upper) >= 5 && upper[0] == 'G' {
		upper = "GAL" + upper[1:]
	}
	return upper
}

func (c Chip) data() chipData {
	switch c {
	case ChipGAL16V8:
		return chip16v8
	case ChipGAL22V10:
		return chip22v10
	default:
		return chipData{}
	}
}

func (c Chip) Name() string    { return c.data().name }
func (c Chip) NumPins() int    { return c.data().numPins }
func (c Chip) NumRows() int    { return c.data().numRows }
func (c Chip) NumCols() int    { return c.data().numCols }
func (c Chip) TotalSize() int  { return c.data().totalSize }
func (c Chip) MinOLMCPin() int { return c.data().minOLMC }
func (c Chip) MaxOLMCPin() int { return c.data().maxOLMC }
func (c Chip) NumOLMCs() int   { return c.data().maxOLMC - c.data().minOLMC + 1 }
func (c Chip) PinToOLMC(pin int) (int, bool) {
	d := c.data()
	if pin < d.minOLMC || pin > d.maxOLMC {
		return 0, false
	}
	return pin - d.minOLMC, true
}

func (c Chip) NumRowsForOLMC(olmc int) int {
	if c == ChipGAL22V10 {
		return olmcSize22v10[olmc]
	}
	return 8
}

// Bounds define the usable row range for an OLMC's terms.
type Bounds struct {
	StartRow  int
	MaxRows   int
	RowOffset int
}

func (c Chip) BoundsForOLMC(olmc int) Bounds {
	return Bounds{
		StartRow:  c.data().olmcMap[olmc],
		MaxRows:   c.NumRowsForOLMC(olmc),
		RowOffset: 0,
	}
}
