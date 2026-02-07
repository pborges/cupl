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

## Goals

- Parse WinCUPL-compatible `.pld` source
- Generate JEDEC `.jed` output
- Support common PAL/GAL device families
- Provide clear errors and diagnostics
- Keep dependencies minimal (prefer Go standard library)

## Non-goals (initially)

- GUI tooling
- IDE integration
- Full WinCUPL feature parity on day one

## CLI

```bash
# Compile PLD into JEDEC
cupl build path/to/design.pld -o path/to/design.jed

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

## Roadmap (Concrete)

Phase 0: Project scaffolding

- Define CLI shape and subcommands (done)
- Add `cmd/cupl` entrypoint (done)
- Decide minimal file layout for parser, device models, and JEDEC writer (done)

Phase 1: Parsing (subset) (done)

- Implement lexer for CUPL tokens used in the sample PLD (done)
- Parse `Device`, `Pin`, and logic equations into an AST (done)

Phase 2: Semantics (done for current subset)

- DNF expansion and range handling (done)
- Resolve pin names, input/output types, and default polarity (done)

Phase 3: Device + Fuse Map (done for 16V8/22V10)

- Add GAL16V8 and GAL22V10 device definitions (done)
- Map product terms to fuse locations (done)

Phase 4: JEDEC Output (done)

- Implement JEDEC writer (header, fuse data, checksums) (done)
- Golden test: compile sample PLD and compare to sample JED (done)

Phase 5: CLI + UX (done)

- `cupl build <file.pld> -o <file.jed>` (done)
- `cupl devices` (done)
- `cupl version` (done)

Phase 6: Expand Coverage

- Add more CUPL constructs as needed (e.g., `FIELD`, `TABLE`)
- Add more device types (GAL20V8, GAL22V10, etc.)
- Add additional golden tests

## License

TBD
