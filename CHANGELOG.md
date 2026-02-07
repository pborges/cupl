# Changelog

All notable changes to this project will be documented in this file.
The format is based on Keep a Changelog, and this project adheres to Semantic Versioning.

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
