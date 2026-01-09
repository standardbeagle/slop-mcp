# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.1] - 2026-01-09

### Changed

- README now leads with value proposition: "Install many MCPs without killing your context"
- Added documentation links to Quick Start, full docs, KDL config, and CLI reference

## [0.3.0] - 2026-01-09

### Added

- HTTP headers support for SSE and streamable transports in KDL config
- Custom headers now work with `headers { }` block in MCP config

### Fixed

- Environment variables now properly inherited when custom env vars are set for command MCPs
- Previously, setting custom env vars would replace the entire environment instead of extending it

## [0.2.1] - 2026-01-08

### Fixed

- Enable OAuth support in release builds by adding `-tags mcp_go_client_oauth` to build command

## [0.2.0] - 2026-01-08

### Added

- `mcp metadata` command to dump full MCP metadata

### Fixed

- Use manual JSON schemas for Claude Code MCP compatibility

## [0.1.1] - 2026-01-08

### Fixed

- Don't download binaries in PyPI job
- Migrate module to github.com/standardbeagle

### Added

- CI release workflow for PyPI and npm publishing

[0.3.1]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.3.1
[0.3.0]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.3.0
[0.2.1]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.2.1
[0.2.0]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.2.0
[0.1.1]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.1.1
