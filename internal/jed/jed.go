package jed

import (
	"fmt"
	"strings"

	"github.com/pborges/cupl/internal/gal"
)

type Config struct {
	SecurityBit bool
	Header      []string
}

// MakeJEDEC generates a JEDEC string for the given GAL.
func MakeJEDEC(cfg Config, g *gal.GAL) string {
	var buf strings.Builder
	buf.WriteByte(0x02)
	buf.WriteByte('\n')
	for _, line := range cfg.Header {
		buf.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			buf.WriteByte('\n')
		}
	}
	buf.WriteString("*F0\n")
	if cfg.SecurityBit {
		buf.WriteString("*G1\n")
	} else {
		buf.WriteString("*G0\n")
	}
	fmt.Fprintf(&buf, "*QF%d\n", g.Chip.TotalSize())

	fb := newFuseBuilder(&buf)
	rowLen := g.Chip.NumCols()
	for row := 0; row < len(g.Fuses); row += rowLen {
		chunk := g.Fuses[row : row+rowLen]
		if anyTrue(chunk) {
			fb.add(chunk)
		} else {
			fb.skip(chunk)
		}
	}

	if g.Chip != gal.ChipGAL22V10 {
		fb.add(g.Xor)
	} else {
		// interleave XOR and AC1
		for i := 0; i < len(g.Xor) && i < len(g.AC1); i++ {
			fb.addBit(g.Xor[i])
			fb.addBit(g.AC1[i])
		}
		fb.endLine()
	}

	fb.add(g.Sig)

	if g.Chip == gal.ChipGAL16V8 {
		fb.add(g.AC1)
		fb.add(g.PT)
		fb.add([]bool{g.Syn})
		fb.add([]bool{g.AC0})
	}

	fb.checksum()
	buf.WriteString("*\n")
	buf.WriteByte(0x03)
	fmt.Fprintf(&buf, "%04x\n", fileChecksum([]byte(buf.String())))
	return buf.String()
}

func anyTrue(bits []bool) bool {
	for _, b := range bits {
		if b {
			return true
		}
	}
	return false
}

type fuseBuilder struct {
	buf      *strings.Builder
	cs       checkSummer
	idx      int
	openLine bool
}

func newFuseBuilder(buf *strings.Builder) *fuseBuilder {
	return &fuseBuilder{buf: buf}
}

func (f *fuseBuilder) add(bits []bool) {
	f.startLine()
	for _, b := range bits {
		f.addBit(b)
	}
	f.endLine()
}

func (f *fuseBuilder) skip(bits []bool) {
	for range bits {
		f.cs.add(false)
		f.idx++
	}
}

func (f *fuseBuilder) addBit(b bool) {
	f.startLine()
	f.buf.WriteByte(byte('0' + boolToInt(b)))
	f.cs.add(b)
	f.idx++
}

func (f *fuseBuilder) startLine() {
	if f.openLine {
		return
	}
	fmt.Fprintf(f.buf, "*L%05d ", f.idx)
	f.openLine = true
}

func (f *fuseBuilder) endLine() {
	if f.openLine {
		f.buf.WriteByte('\n')
		f.openLine = false
	}
}

func (f *fuseBuilder) checksum() {
	f.endLine()
	fmt.Fprintf(f.buf, "*C%04x\n", f.cs.get())
}

type checkSummer struct {
	bitNum uint8
	byte   uint8
	sum    uint16
}

func (c *checkSummer) add(bit bool) {
	if bit {
		c.byte |= 1 << c.bitNum
	}
	c.bitNum++
	if c.bitNum == 8 {
		c.sum = c.sum + uint16(c.byte)
		c.byte = 0
		c.bitNum = 0
	}
}

func (c *checkSummer) get() uint16 {
	return c.sum + uint16(c.byte)
}

func boolToInt(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func fileChecksum(data []byte) uint16 {
	var sum uint16
	for _, b := range data {
		sum += uint16(b)
	}
	return sum
}
