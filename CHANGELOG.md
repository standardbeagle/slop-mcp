# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.7.0] - 2026-01-15

### Added

- **CLI tools accessible from SLOP scripts**: CLI tools are now available as `cli.tool_name()` in SLOP scripts
- Memory tools can be used for persistent storage in scripts:
  ```slop
  cli.memory_write(bank: "session", key: "context", value: '{"topic": "testing"}')
  data = cli.memory_read(bank: "session", key: "context")
  ```
- Example CLI tool definitions in `examples/cli/memory-tools.kdl`
- Upgraded to slop v0.2.0 with external service API

## [0.6.1] - 2026-01-15

### Fixed

- `search_tools` now paginates results (default: 20, max: 100) to prevent large responses from filling context
- Response includes `total`, `limit`, `offset`, and `has_more` for pagination control

### Added

- CLAUDE.md documentation for Claude Code integration

## [0.6.0] - 2026-01-14

### Added

- Ranked multi-term search: queries like "lci code insight" now find `code_insight` from `lci` MCP
- Results ranked by relevance: exact match > MCP name match > prefix > term matches > fuzzy
- `get_metadata` now supports `verbose` flag for full schema output
- ToolNotFoundError now suggests similar tools using fuzzy matching
- InvalidParameterError now shows ALL issues exhaustively:
  - Missing required parameters with descriptions
  - Unknown parameters with "did you mean" suggestions
  - Full expected parameter list for reference

### Changed

- `get_metadata` default output is now compact (tool names + descriptions only)
- Full schemas only included when `verbose=true` or querying specific `mcp_name + tool_name`
- Reduces token usage significantly when browsing MCP capabilities

## [0.5.0] - 2026-01-12

### Added

- `get_metadata` tool now supports `tool_name` filter to retrieve metadata for a specific tool
- Enhanced error messages for invalid tool parameters with "Did you mean?" suggestions
- Parameter error messages now include full expected parameter list with types and descriptions

### Changed

- Tool indexing now stores input schemas for better parameter validation error messages

## [0.4.1] - 2026-01-11

### Fixed

- Version reporting now correctly shows release version instead of hardcoded value

## [0.4.0] - 2026-01-11

### Added

- Fuzzy matching for tool search - now matches tools even with typos or partial names

## [0.3.2] - 2026-01-10

### Fixed

- Automatic OAuth token refresh for expired tokens

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

[0.7.0]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.7.0
[0.6.1]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.6.1
[0.6.0]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.6.0
[0.5.0]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.5.0
[0.4.1]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.4.1
[0.4.0]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.4.0
[0.3.2]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.3.2
[0.3.1]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.3.1
[0.3.0]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.3.0
[0.2.1]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.2.1
[0.2.0]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.2.0
[0.1.1]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.1.1
