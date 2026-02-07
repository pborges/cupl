package cupl

import (
	"testing"

	"github.com/pborges/cupl/examples"
	"github.com/pborges/cupl/internal/jed"
	"github.com/pborges/cupl/internal/testutil"
)

func TestGoldenExamples(t *testing.T) {
	cases := []struct {
		name    string
		pldPath string
		jedPath string
	}{
		{
			name:    "16V8_56K_8K",
			pldPath: "MECB_ChipSelect_6502_CPU_16V8_56K_RAM_8K_ROM.PLD",
			jedPath: "MECB_ChipSelect_6502_CPU_16V8_56K_RAM_8K_ROM.jed",
		},
		{
			name:    "16V8_48K_16K",
			pldPath: "MECB_ChipSelect_6502_CPU_16V8_48K_RAM_16K_ROM.PLD",
			jedPath: "MECB_ChipSelect_6502_CPU_16V8_48K_RAM_16K_ROM.jed",
		},
		{
			name:    "16V8_32K_32K",
			pldPath: "MECB_ChipSelect_6502_CPU_16V8_32K_RAM_32K_ROM.PLD",
			jedPath: "MECB_ChipSelect_6502_CPU_16V8_32K_RAM_32K_ROM.jed",
		},
		{
			name:    "16V8_CreatiVision_Onboard",
			pldPath: "MECB_ChipSelect_6502_CPU_16V8_CreatiVision_4K_RAM_32K_ROM_onboard.PLD",
			jedPath: "MECB_ChipSelect_6502_CPU_16V8_CreatiVision_4K_RAM_32K_ROM_onboard.jed",
		},
		{
			name:    "16V8_CreatiVision_Expansion",
			pldPath: "MECB_ChipSelect_6502_CPU_16V8_CreatiVision_4K_RAM_Expansion_ROM.PLD",
			jedPath: "MECB_ChipSelect_6502_CPU_16V8_CreatiVision_4K_RAM_Expansion_ROM.jed",
		},
		{
			name:    "22V10_IO",
			pldPath: "MECB_CHIPSELECT_PROTOTYPE_PLD_22V10_IO_0xE0-0xFF.PLD",
			jedPath: "MECB_CHIPSELECT_PROTOTYPE_PLD_22V10_IO_0xE0-0xFF.jed",
		},
		{
			name:    "22V10_IO_Minimal",
			pldPath: "MECB_CHIPSELECT_PROTOTYPE_PLD_22V10.PLD",
			jedPath: "MECB_CHIPSELECT_PROTOTYPE_PLD_22V10.jed",
		},
	{
			name:    "22V10_Memory",
			pldPath: "MECB_CHIPSELECT_PROTOTYPE_PLD_22V10_0xA000-0xAFFF.PLD",
			jedPath: "MECB_CHIPSELECT_PROTOTYPE_PLD_22V10_0xA000-0xAFFF.jed",
		},
		{
			name:    "22V10_6502_16io",
			pldPath: "6502_16io_PLD_22V10.pld",
			jedPath: "6502_16io_PLD_22V10.jed",
		},
		{
			name:    "22V10_Addr_Complex",
			pldPath: "addr_complex.pld",
			jedPath: "addr_complex.jed",
		},
		{
			name:    "22V10_Addr_Small",
			pldPath: "addr_small.pld",
			jedPath: "addr_small.jed",
		},
		{
			name:    "22V10_Addr_Isolate",
			pldPath: "addr_isolate.pld",
			jedPath: "addr_isolate.jed",
		},
		{
			name:    "JCODEC",
			pldPath: "JCODEC.pld",
			jedPath: "JCODEC.jed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pld := mustRead(t, tc.pldPath)
			expected := mustRead(t, tc.jedPath)
			content, err := Parse(pld)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			g, err := Compile(content)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			gotJed := jed.MakeJEDEC(jed.Config{}, g)
			compareJEDEC(t, gotJed, expected)
		})
	}
}

func compareJEDEC(t *testing.T, gotJed string, expected []byte) {
	got, err := testutil.ParseJEDEC([]byte(gotJed))
	if err != nil {
		t.Fatalf("parse got jed: %v", err)
	}
	want, err := testutil.ParseJEDEC(expected)
	if err != nil {
		t.Fatalf("parse expected jed: %v", err)
	}
	if got.QF != want.QF {
		t.Fatalf("QF mismatch: got %d want %d", got.QF, want.QF)
	}
	if len(got.Fuses) != len(want.Fuses) {
		t.Fatalf("fuse length mismatch: got %d want %d", len(got.Fuses), len(want.Fuses))
	}
	for i := range got.Fuses {
		if got.Fuses[i] != want.Fuses[i] {
			t.Fatalf("fuse mismatch at %d", i)
		}
	}
}

func mustRead(t *testing.T, path string) []byte {
	b, err := examples.FS.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
