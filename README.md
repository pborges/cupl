# cupl

**WARNING** Built with AI assistance, may or may not work at all

A minimal, cross-platform CLI to compile WinCUPL `.pld` files into JEDEC (`.jed`) programming files for PAL/GAL devices.

WinCUPL is Windows-only and long out of date. This project aims to provide a modern replacement focused on:

- A **small, dependency-light** Go codebase
- A **clean CLI** for batch-friendly workflows
- **Deterministic output** suitable for programming PAL/GAL devices

## Status

Working MVP:

- Parses a practical CUPL subset
- Supports `g16v8` and `g22v10`
- Generates JEDEC with checksums
- Golden tests against provided samples

## Limitations

- Not full WinCUPL parity yet
- Focused on logic equations used in the sample designs
- Limited device support (GAL16V8/22V10 variants only)

## Features

- WinCUPL-style `.pld` input to JEDEC `.jed` output
- Deterministic JEDEC generation with checksums
- Device support: `g16v8`, `g22v10`
- Batch-friendly CLI (`build`, `burn`, `devices`, `version`)
- Golden tests against real-world PLD/JED examples
- Small, dependency-light Go codebase

## Non-goals (initially)

- GUI tooling
- IDE integration
- Full WinCUPL feature parity on day one

## CLI

```bash
# Compile PLD into JEDEC
cupl build path/to/design.pld -o path/to/design.jed

# Burn JEDEC to device with minipro (device auto-detected from JED header)
cupl burn path/to/design.jed

# Override minipro device name
cupl burn path/to/design.jed -p g16v8as

# Show device info or list supported devices
cupl devices
```

## Build And Test

```bash
go build ./cmd/cupl
go test ./...
```

## References

- CUPL Programmer’s Guide: https://ece-classes.usc.edu/ee459/library/documents/CUPL_Reference.pdf
- `galette` (Rust GAL assembler): https://github.com/simon-frankau/galette
- `GALasm` (older GAL assembler): https://github.com/daveho/GALasm

## Samples

Test fixtures included in `examples/`:

Example PLD/JED files courtesy of `https://digicoolthings.com/`.

- `examples/MECB_ChipSelect_6502_CPU_16V8_56K_RAM_8K_ROM.PLD` (input)
- `examples/MECB_ChipSelect_6502_CPU_16V8_56K_RAM_8K_ROM.jed` (expected output)
- `examples/MECB_ChipSelect_6502_CPU_16V8_48K_RAM_16K_ROM.PLD` (input)
- `examples/MECB_ChipSelect_6502_CPU_16V8_48K_RAM_16K_ROM.jed` (expected output)
- `examples/MECB_ChipSelect_6502_CPU_16V8_32K_RAM_32K_ROM.PLD` (input)
- `examples/MECB_ChipSelect_6502_CPU_16V8_32K_RAM_32K_ROM.jed` (expected output)
- `examples/MECB_ChipSelect_6502_CPU_16V8_CreatiVision_4K_RAM_32K_ROM_onboard.PLD` (input)
- `examples/MECB_ChipSelect_6502_CPU_16V8_CreatiVision_4K_RAM_32K_ROM_onboard.jed` (expected output)
- `examples/MECB_ChipSelect_6502_CPU_16V8_CreatiVision_4K_RAM_Expansion_ROM.PLD` (input)
- `examples/MECB_ChipSelect_6502_CPU_16V8_CreatiVision_4K_RAM_Expansion_ROM.jed` (expected output)
- `examples/MECB_CHIPSELECT_PROTOTYPE_PLD_22V10.PLD` (input)
- `examples/MECB_CHIPSELECT_PROTOTYPE_PLD_22V10.jed` (expected output)
- `examples/MECB_CHIPSELECT_PROTOTYPE_PLD_22V10_IO_0xE0-0xFF.PLD` (input)
- `examples/MECB_CHIPSELECT_PROTOTYPE_PLD_22V10_IO_0xE0-0xFF.jed` (expected output)
- `examples/MECB_CHIPSELECT_PROTOTYPE_PLD_22V10_0xA000-0xAFFF.PLD` (input)
- `examples/MECB_CHIPSELECT_PROTOTYPE_PLD_22V10_0xA000-0xAFFF.jed` (expected output)

## License

TBD
