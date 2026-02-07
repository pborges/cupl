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
