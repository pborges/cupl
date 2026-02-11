package testutil

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type JEDEC struct {
	QF    int
	G     int
	Fuses []bool
	Csum  uint16
}

func ParseJEDEC(data []byte) (JEDEC, error) {
	var j JEDEC
	s := string(data)
	// remove STX/ETX if present
	s = strings.TrimPrefix(s, "\x02")
	if idx := strings.Index(s, "\x03"); idx >= 0 {
		s = s[:idx]
	}
	scanner := bufio.NewScanner(strings.NewReader(s))
	fuses := map[int]bool{}
	maxIndex := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "*QF") {
			v := strings.TrimPrefix(line, "*QF")
			qf, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				return j, err
			}
			j.QF = qf
			continue
		}
		if strings.HasPrefix(line, "*G") {
			v := strings.TrimPrefix(line, "*G")
			g, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				return j, err
			}
			j.G = g
			continue
		}
		if strings.HasPrefix(line, "*C") {
			v := strings.TrimPrefix(line, "*C")
			cs, err := strconv.ParseUint(strings.TrimSpace(v), 16, 16)
			if err != nil {
				return j, err
			}
			j.Csum = uint16(cs)
			continue
		}
		if strings.HasPrefix(line, "*L") {
			parts := strings.SplitN(line[2:], " ", 2)
			if len(parts) != 2 {
				return j, fmt.Errorf("invalid L line: %q", line)
			}
			off, err := strconv.Atoi(parts[0])
			if err != nil {
				return j, err
			}
			bits := strings.TrimSpace(parts[1])
			for i, ch := range bits {
				idx := off + i
				if ch == '1' {
					fuses[idx] = true
				} else if ch == '0' {
					fuses[idx] = false
				} else {
					return j, fmt.Errorf("invalid bit %q", ch)
				}
				if idx > maxIndex {
					maxIndex = idx
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return j, err
	}
	if j.QF == 0 {
		j.QF = maxIndex + 1
	}
	j.Fuses = make([]bool, j.QF)
	for i := 0; i < j.QF; i++ {
		if v, ok := fuses[i]; ok {
			j.Fuses[i] = v
		} else {
			j.Fuses[i] = false
		}
	}
	return j, nil
}

func FuseChecksum(bits []bool) uint16 {
	var (
		bitNum  uint8
		byteVal uint8
		sum     uint16
	)
	for _, bit := range bits {
		if bit {
			byteVal |= 1 << bitNum
		}
		bitNum++
		if bitNum == 8 {
			sum += uint16(byteVal)
			byteVal = 0
			bitNum = 0
		}
	}
	return sum + uint16(byteVal)
}

// FuseSectionName16V8 returns the section name for a given fuse index on a GAL16V8.
// JED layout: Logic(2048) + XOR(8) + SIG(64) + AC1(8) + PT(64) + SYN(1) + AC0(1) = 2194
func FuseSectionName16V8(idx int) string {
	switch {
	case idx < 2048:
		row := idx / 32
		col := idx % 32
		olmcNames := []string{"OLMC7(pin19)", "OLMC6(pin18)", "OLMC5(pin17)", "OLMC4(pin16)", "OLMC3(pin15)", "OLMC2(pin14)", "OLMC1(pin13)", "OLMC0(pin12)"}
		olmcIdx := row / 8
		olmcRow := row % 8
		name := "?"
		if olmcIdx < len(olmcNames) {
			name = olmcNames[olmcIdx]
		}
		return fmt.Sprintf("Logic %s row%d col%d", name, olmcRow, col)
	case idx < 2056:
		return fmt.Sprintf("XOR[%d]", idx-2048)
	case idx < 2120:
		return fmt.Sprintf("SIG[%d]", idx-2056)
	case idx < 2128:
		return fmt.Sprintf("AC1[%d]", idx-2120)
	case idx < 2192:
		return fmt.Sprintf("PT[%d]", idx-2128)
	case idx == 2192:
		return "SYN"
	case idx == 2193:
		return "AC0"
	default:
		return fmt.Sprintf("unknown(%d)", idx)
	}
}

// FuseSectionName22V10 returns the section name for a given fuse index on a GAL22V10.
func FuseSectionName22V10(idx int) string {
	switch {
	case idx < 5808:
		row := idx / 44
		col := idx % 44
		// OLMC boundaries
		olmcStarts := []int{0, 1, 10, 21, 34, 49, 66, 83, 98, 111, 122}
		olmcSizes := []int{1, 9, 11, 13, 15, 17, 17, 15, 13, 11, 9}
		olmcPins := []string{"AR", "pin23", "pin22", "pin21", "pin20", "pin19", "pin18", "pin17", "pin16", "pin15", "pin14"}
		for i := len(olmcStarts) - 1; i >= 0; i-- {
			if row >= olmcStarts[i] {
				localRow := row - olmcStarts[i]
				return fmt.Sprintf("Logic OLMC(%s) row%d/%d col%d", olmcPins[i], localRow, olmcSizes[i], col)
			}
		}
		return fmt.Sprintf("Logic row%d col%d", row, col)
	case idx < 5818:
		return fmt.Sprintf("XOR[%d]", idx-5808)
	case idx < 5828:
		return fmt.Sprintf("AC1[%d]", idx-5818)
	case idx < 5892:
		return fmt.Sprintf("SIG[%d]", idx-5828)
	default:
		return fmt.Sprintf("unknown(%d)", idx)
	}
}

// CompareJEDEC compares two parsed JEDEC structs and returns a human-readable diff.
// qf is used to pick the right chip section names.
func CompareJEDEC(got, want JEDEC) string {
	if got.QF != want.QF {
		return fmt.Sprintf("QF mismatch: got %d want %d", got.QF, want.QF)
	}
	if len(got.Fuses) != len(want.Fuses) {
		return fmt.Sprintf("fuse length mismatch: got %d want %d", len(got.Fuses), len(want.Fuses))
	}

	sectionName := func(idx int) string {
		switch got.QF {
		case 2194:
			return FuseSectionName16V8(idx)
		case 5892:
			return FuseSectionName22V10(idx)
		default:
			return fmt.Sprintf("fuse[%d]", idx)
		}
	}

	var buf bytes.Buffer
	mismatches := 0
	for i := range got.Fuses {
		if got.Fuses[i] != want.Fuses[i] {
			mismatches++
			gotVal := '0'
			wantVal := '0'
			if got.Fuses[i] {
				gotVal = '1'
			}
			if want.Fuses[i] {
				wantVal = '1'
			}
			fmt.Fprintf(&buf, "  fuse[%d] %s: got=%c want=%c\n", i, sectionName(i), gotVal, wantVal)
			if mismatches >= 40 {
				fmt.Fprintf(&buf, "  ... (%d+ mismatches, truncated)\n", mismatches)
				break
			}
		}
	}
	if mismatches == 0 {
		return ""
	}
	return fmt.Sprintf("%d fuse mismatches:\n%s", mismatches, buf.String())
}

func NormalizeJEDEC(data []byte) ([]byte, error) {
	j, err := ParseJEDEC(data)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "*QF%d\n", j.QF)
	fmt.Fprintf(&buf, "*G%d\n", j.G)
	for i, b := range j.Fuses {
		if i%32 == 0 {
			fmt.Fprintf(&buf, "*L%05d ", i)
		}
		if b {
			buf.WriteByte('1')
		} else {
			buf.WriteByte('0')
		}
		if i%32 == 31 {
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes(), nil
}
