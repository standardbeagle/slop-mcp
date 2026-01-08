# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.2.1]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.2.1
[0.2.0]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.2.0
[0.1.1]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.1.1
