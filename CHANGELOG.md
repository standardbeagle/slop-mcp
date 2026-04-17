# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.14.0] - 2026-04-16

### Added

- **`customize_tools` meta-tool (9th meta-tool)**: Actions `set_override`, `remove_override`, `list_overrides`, `define_custom`, `remove_custom`, `list_custom`, `export`, `import`. Lets agents override tool descriptions and param docs, and define SLOP-backed custom tools.

- **Three storage scopes for reserved `_slop.*` memory banks**: user (`~/.config/slop-mcp/memory/_slop/`), project (`<repo>/.slop-mcp/memory/_slop/`, committable), local (`<repo>/.slop-mcp/memory.local/_slop/`, gitignored). Merge precedence: local > project > user.

- **Hash-tied staleness detection**: overrides flag `stale: true` when upstream descriptions change. `list_overrides stale_only:true` returns only mismatches; `manage_mcps list_stale_overrides` is an ergonomic shortcut.

- **Custom tool execution through the SLOP runtime**: arg validation, `$args` + scalar shorthand bindings, recursion depth guard (default 16), and body size limit (`SLOP_MAX_CUSTOM_BODY`, default 64 KB).

- **Pack-based import/export** (`schema_version: 1`) for sharing customizations across machines and teams.

### Changed

- **Meta-tool descriptions**: rewritten in caveman-style for token efficiency. See `docs/internal/description-style.md`.

- **Registry ranking index**: now uses `atomic.Pointer` for lock-free reads during rebuilds.

- **Memory access control**: `mem_save`, `mem_delete`, and `memory-cli` reject writes to banks prefixed with `_slop.`.

- **Custom tool execution**: whole-number `float64` args now pass JSON Schema `"integer"` type checks (fixes the JSON-decoded-integer edge case).

### Internal

- **New `internal/overrides/` package**: entry types, canonical JSON + SHA-256 truncated hashing, scope resolution, scope-aware store, single-slot coalescing bank flusher (no locks held over I/O), and pack import/export.

- **`registry.OverrideProvider` interface**: for injecting descriptions and custom tools into the ranking index.

## [0.13.1] - 2026-03-06

### Fixed

- **CI**: updated Go version from 1.23 to 1.24 in release workflow, fixing broken npm/PyPI binary builds.
- **CI**: added `windows/arm64` to the build matrix.

## [0.13.0] - 2026-03-01

### Changed

- **SLOP language**: upgraded to slop v0.3.0 with context-aware hyphenated identifiers.
  MCP server names with hyphens (e.g., `dart-query.search()`) now work naturally in
  SLOP scripts without needing `execute_tool` workaround.

## [0.12.2] - 2026-02-25

### Changed

- **npm and PyPI READMEs**: replaced generic examples with MCP-focused orchestration
  patterns — creating Jira tasks from email queries, indexing codebase structure into
  persistent memory via LCI, generating visual reports with Nano Banana MCP.

- **PyPI README**: now uses a dedicated `PYPI_README.md` with uvx-focused install
  instructions instead of the marketing-heavy root README.

## [0.12.1] - 2026-02-25

### Added

- **SLOP Language Reference** in docusaurus docs: full syntax documentation with pipe
  operator, MCP tool calls, TypeScript/Python comparison, sandboxing model, and recipes.

- **"Why SLOP MCP" concept page**: references Anthropic's "Code Execution with MCP"
  article and explains how slop-mcp implements the code-execution-within-MCP pattern.

### Fixed

- **Windows quick start**: added `cmd /c` wrapper for npx calls to prevent Windows path
  handling from corrupting the `@standardbeagle/slop-mcp` scoped package name.

- **Stale npm references**: fixed `npx slop-mcp` → `npx @standardbeagle/slop-mcp` and
  corrected JSON config args in installation docs.

- **Incorrect SLOP syntax in tools.md**: replaced `@call`/`@let`/`@if`/`@each` pseudo-syntax
  with actual SLOP syntax in the `run_slop` examples.

- **Tool count references**: updated "6 meta-tools" → "8 meta-tools" across all docs,
  README, and CLAUDE.md.

### Changed

- **SEO improvements**: added meta description/keywords to docusaurus config, expanded
  npm/PyPI keywords and descriptions, fixed homepage URLs.

## [0.12.0] - 2026-02-19

### Added

- **Memory metadata and discovery**: `mem_save` now accepts optional `description:` and
  `schema:` kwargs and auto-computes `size` (bytes of serialized value). Metadata is
  preserved across re-saves when kwargs are omitted (non-destructive updates).

- **`mem_info(bank, key)`**: Returns entry metadata map (description, schema, size,
  created_at, updated_at) without loading the value — zero-cost discoverability.

- **`mem_list(bank, pattern: "")`**: Lists all entries in a bank with metadata (no
  values). Optional glob `pattern` kwarg filters by key name. Output is sorted by key
  for stable results.

- **`mem_search(query, bank: "", include_values: false)`**: Case-insensitive substring
  search across key names and descriptions in all banks (or a single `bank`). Set
  `include_values: true` to also search serialized value content and include values in
  results.

- **memory-cli interoperability**: `Entry`, `KeyInfo`, and `SearchMatch` structs updated
  with `description`, `schema`, and `size` fields. `cmdWrite` auto-computes size and
  preserves metadata on update. `cmdSearch` now matches on description text in addition
  to key names.

## [0.11.0] - 2026-02-18

### Added

- **Session-level memory**: Thread-safe `store_set`/`store_get`/`store_delete`/
  `store_exists`/`store_keys` that persist across `run_slop` calls within a
  server session. Overrides SLOP's unsynchronized built-in store with
  `sync.RWMutex`-backed implementation safe for concurrent access.

- **Persistent memory**: Disk-backed `mem_save`/`mem_load`/`mem_delete`/
  `mem_keys`/`mem_banks` functions that survive server restarts. Data stored as
  JSON in `~/.config/slop-mcp/memory/<bank>.json`, compatible with memory-cli
  format. Atomic writes via temp file + rename.

- **Structured SLOP errors**: Script execution errors now return structured JSON
  with `type` ("parse"/"runtime"), `message`, and `errors` array containing
  `line`, `column`, `message`, and `source_line` for agent self-correction.

- **Embedded recipe templates**: `recipe` parameter on `run_slop` loads built-in
  `.slop` templates. Use `recipe: "list"` to see available recipes. Ships with
  `batch_collect`, `search_and_inspect`, and `transform_pipeline`.

- **Pipe syntax examples**: Updated `run_slop` tool description with pipe
  chaining, session memory, persistent memory, and recipe usage examples.

## [0.10.0] - 2026-02-14

### Added

- **Tool metadata cache and lazy loading**: MCP tool metadata is cached to
  `~/.config/slop-mcp/cache/tools.json` for faster startup. Cached MCPs
  connect lazily on first `execute_tool` or specific `get_metadata` call.
  Use `dynamic true` in MCP config to skip caching and always connect eagerly.

### Fixed

- **Documentation**: Corrected tool count from 5/6 to 8 across README and
  CLAUDE.md to reflect `slop_reference` and `slop_help` tools added in v0.8.0.
- **README**: Removed non-existent `--transport` flag from `serve` command
  examples (use `--port` instead).
- **serve --help**: Added missing local config tier to configuration
  description.

## [0.9.3] - 2026-01-24

### Fixed

- **Nil parameters handling**: Convert null/nil parameters to empty object `{}` before
  calling downstream MCPs, preventing "expected record, received null" validation errors.

- **Improved error messages**: Added `MCPProtocolError` type with human-readable messages
  and actionable fix suggestions for common MCP protocol errors (Zod validation errors,
  JSON-RPC -32602/-32601 errors).

## [0.9.2] - 2026-01-24

### Fixed

- **MCP protocol compliance**: Removed `outputSchema` from all 8 tools to fix error
  -32600 "Tool has an output schema but did not return structured content". Tools now
  return unstructured `Content` with `TextContent` as designed.

### Changed

- **execute_tool passthrough**: Now returns the underlying MCP tool's response directly
  without re-serializing. This preserves the original response format from connected MCPs.

- **Tool descriptions updated**: Each tool now documents its output format in the
  description for clarity.

## [0.9.1] - 2026-01-24

### Changed

- **Quieter operation**: Connection, shutdown, and health check errors are now
  logged at DEBUG level instead of WARN. These errors are stored in registry
  state and can be queried via `mcp status`. Only tool indexing failures remain
  at WARN level as they indicate actual operational issues.

## [0.9.0] - 2026-01-24

### Added

- **JWT built-in functions for SLOP scripts**:
  - `jwt_decode(token)`: Decode JWT without verification (inspect claims)
  - `jwt_verify(token, key, alg)`: Verify JWT signature with key
  - `jwt_sign(claims, key, alg)`: Create signed JWT token
  - `jwt_expired(token)`: Check if JWT exp claim has passed
  - Supports HS256/384/512, RS256/384/512, ES256/384/512, EdDSA

- **Go template built-in functions for SLOP scripts**:
  - `template_render(template, data)`: Render Go template with data
  - `template_render_file(path, data)`: Render template from file
  - `indent(text, spaces)`: Indent all lines by N spaces
  - `dedent(text)`: Remove common leading whitespace
  - `wrap(text, width)`: Word-wrap text at width
  - Templates support SLOP callbacks via `{{ slop "expression" }}`
  - Rich template functions: upper, lower, trim, split, join, toJson, etc.

### Fixed

- **Stdio transport stdout corruption**: Removed all `fmt.Printf` calls that wrote
  to stdout during MCP serve mode, corrupting the JSON-RPC protocol stream
- **Default to serve mode**: Running the binary without a subcommand now defaults
  to stdio MCP server mode instead of printing usage and exiting
- **MCP-compliant error responses**: Tool parameter validation errors now return
  proper `isError: true` tool results instead of JSON-RPC protocol errors
- **OAuth flow output**: Auth flow messages (browser URL, status) now write to
  stderr instead of stdout to avoid protocol interference
- **Clean signal shutdown**: SIGINT/SIGTERM no longer reports "Server error:
  context canceled" on normal shutdown

## [0.8.0] - 2026-01-15

### Added

- **SLOP language reference MCP tools**:
  - `slop_reference`: Search SLOP built-in functions with progressive disclosure
    - Compact mode (default): signature only, 10 results
    - `verbose=true`: adds category, description, example
    - `list_categories=true`: returns category counts
  - `slop_help`: Get full details for a specific function by name
  - Text output format for token efficiency

- **Crypto built-in functions for SLOP scripts**:
  - `crypto_password(length)`: Generate secure random password
  - `crypto_passphrase(words)`: Generate word-based passphrase
  - `crypto_ed25519_keygen()`: Generate Ed25519 keypair
  - `crypto_rsa_keygen(bits)`: Generate RSA keypair
  - `crypto_x25519_keygen()`: Generate X25519 key exchange keypair
  - `crypto_sha256(data)`, `crypto_sha512(data)`: Hash functions
  - `crypto_random_bytes(n)`, `crypto_random_base64(n)`: Random data generation

- **SLOP language search functions** (accessible in scripts):
  - `slop_search(query, limit)`: Search function reference
  - `slop_categories()`: List all function categories
  - `slop_help(name)`: Get function details

- **Comprehensive test coverage**:
  - Handler unit tests for all meta-tools
  - Config tests for KDL parsing and three-tier merge
  - Auth tests for OAuth flow, token refresh, expiry detection

- **Configurable timeouts**: `SLOP_MCP_TIMEOUT` env var or per-MCP `timeout` config

- **MCP reconnection**: Automatic reconnect with exponential backoff, `manage_mcps reconnect` action

- **Health checks**: `manage_mcps health_check` action to detect stale connections

- **Structured logging**: Optional JSON logging via `SLOP_MCP_LOG_FORMAT=json`

### Changed

- SLOP reference tools output formatted text instead of JSON for better readability and token efficiency

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

[0.10.0]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.10.0
[0.9.3]: https://github.com/standardbeagle/slop-mcp/releases/tag/v0.9.3
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
