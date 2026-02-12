# Changelog

All notable changes to this project will be documented in this file.
The format is based on Keep a Changelog, and this project adheres to Semantic Versioning.

## [1.5.0] - 2026-02-11
### Added
- All three GAL16V8 operating modes: Simple, Complex, and Registered.
- GAL22V10 registered output support with per-OLMC mode selection.
- `.D` extension for registered (clocked D flip-flop) outputs.
- `.OE` extension for programmable output enable equations.
- Global `AR` (async reset) and `SP` (sync preset) signals for GAL22V10.
- Device mnemonic support to force GAL16V8 mode: `g16v8as` (Simple), `g16v8ma` (Complex), `g16v8ms` (Registered).
- Auto-detection of GAL16V8 mode from equations when no mnemonic is specified.
- Feedback negation flip (`needs_flip`) for GAL22V10 registered active-high outputs.
- Product term sorting to match WinCUPL output ordering.
- Eight new blackbox test cases compiled with WinCUPL covering all modes.

### Changed
- Regenerated all existing reference JED files to match updated product term ordering.
- Updated README with device modes, extensions, and global signals documentation.

## [1.4.1] - 2026-02-10
### Removed
- Removed device name mapping in `burn` command; the JED header device name is now passed directly to minipro.

## [1.4.0] - 2026-02-10
### Changed
- All example PLD files now compile and pass tests.
- Committed to Quine-McCluskey as the sole minimization algorithm for product terms.
- Renamed example files from digicoolthings.com for brevity (e.g., `MECB_32K_RAM_32K_ROM`).
- Regenerated all `.jed` fixtures using Quine-McCluskey minimization.
- Updated README to reflect blackbox testing methodology.

## [1.3.0] - 2026-02-07
### Changed
- JED headers now preserve the normalized device name (e.g., `g22v10`) instead of mapping to minipro device strings.

### Known Issues
Golden tests still report mismatches or missing fixtures for some 22V10 examples; these need regeneration or updated baselines:
- `22V10_6502_16io`: fuse mismatch at 2212.
- `22V10_Addr_Complex`: missing `examples/addr_complex.jed`.
- `22V10_Addr_Small`: missing `examples/addr_small.jed`.
- `22V10_Addr_Isolate`: fuse mismatch at 2204.

## [1.2.0] - 2026-02-07
### Added
- `-v` flag to print the embedded VERSION at runtime.
- `burn` now accepts `.pld` inputs, builds a temporary `.jed`, then burns it.

### Changed
- Embedded example fixtures moved to `examples/` for package layout cleanup.

## [1.1.0] - 2026-02-07
### Added
- New `burn` subcommand to program JED files via `minipro`.
- Auto-detection of target device from JED headers with a `-p/--device` override.

### Changed
- JED header `Device` now uses the minipro device name when known (e.g., `ATF22V10C`).
- Regenerated example JED fixtures to match the current compiler output.

## [1.0.2] - 2026-02-07
### Added
- Another release to test the GitHub Actions pipeline.

## [1.0.1] - 2026-02-07
### Added
- Release to test the GitHub Actions pipeline.

## [1.0.0] - 2026-02-07
### Added
- Initial release.
