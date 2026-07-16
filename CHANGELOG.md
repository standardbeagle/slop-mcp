# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.14.5] - 2026-07-16

### Fixed

- **`EACCES` on spawn when the consumer sets npm `ignore-scripts=true`**: the platform binary was made executable only by the `postinstall` chmod (`install.js`), which npm skips under `ignore-scripts`, leaving it `0644`. The binary is now chmod'd to `0755` at resolve time on every run (`getBinaryPath`), so the spawn works regardless of install-script policy. The release workflow also restores the exec bit after `download-artifact` (which strips unix permissions) so the published tarball ships the binaries executable.

## [0.14.4] - 2026-07-08

### Fixed

- **`run_slop` returned internal evaluator structs**: results and emitted values were JSON-marshaled without conversion, so agents received `{"Value":3}` / `{"Elements":[…]}` instead of plain values. Output now matches the custom-tool path (native Go via `ValueToGo`); SLOP integers stay integers.
- **Dead MCP connections never recovered**: a crashed stdio subprocess or dropped socket left the MCP in `connected` forever. Transport failures on tool calls and health-check pings now demote the MCP to `error` and tear down the dead session, and the background health loop drives real exponential-backoff auto-reconnect.
- **Concurrent `run_slop` raced and misrouted pipeline callbacks**: slop v0.3.0 reads its pipeline-caller global unsynchronized during `map`/`filter`/`reduce`, so serializing only construction was insufficient. Construction *and* execution are now serialized process-wide; evaluator panics are recovered into errors instead of crashing the server.
- **Lost updates across processes**: memory banks and OAuth tokens are written by multiple slop-mcp processes (and memory-cli). A new cross-process file lock (`flock`/`LockFileEx`) now guards each load-modify-write, so concurrent writers no longer clobber each other and a rotating OAuth refresh token is not double-spent (which some providers revoke the whole token family for). `memory-cli` and the server now agree on the user memory directory and preserve entry TTLs across re-save.
- **Customizations could silently fail to persist**: `customize_tools` reported success while the override/custom-tool write was fire-and-forget; a failed disk write only surfaced at shutdown. Writes are now synchronous and their errors are returned to the caller.
- **`execute_tool` had no timeout**: a hung MCP tool blocked the calling agent indefinitely. Calls are now bounded by a generous, `SLOP_MCP_EXECUTE_TIMEOUT`-overridable deadline (any shorter client deadline still wins).
- **OAuth tokens were not refreshed mid-session**: the bearer token was baked into the transport at connect and never re-evaluated, so an expiry mid-session 401'd until reconnect. HTTP/SSE MCPs now refresh per request. `generateState` fails fast on RNG failure instead of a predictable fallback, and token-endpoint calls have a timeout.
- **CLI `run`/`monitor` ignored context**: `--timeout` and Ctrl-C did not stop a running script. Both now execute under the context, and `monitor`'s message tailer consumes only complete lines (no duplicated or truncated events).
- **Custom-tool integer parameters lost precision**: the `_custom`/CLI route decoded params through float64. It now preserves large integers via `json.Number`.
- **Duplicate `mcp` blocks in one KDL file** silently last-wins-overwrote; now a parse error.
- Config files are written `0600` (they embed API keys / auth headers); atomic writes now fsync for crash durability; `search_tools`/`get_metadata` withhold full schemas per the progressive-disclosure design; per-MCP `health_check_interval` is honored instead of collapsing to the shortest; the monitor message file is scoped per project; memory nil-map panics, unbiased crypto RNG, time-aware `jwt_verify`, session-store deep-copy, and dynamic reserved-builtin detection.

### Added

- Optional HTTP bearer-token auth (`SLOP_MCP_HTTP_TOKEN`) gating all HTTP/SSE routes, which were previously unauthenticated.
- `internal/filelock` cross-process advisory locking (unix `flock`, Windows `LockFileEx`).
- CLI tool command/`workdir` paths now expand a leading `~`.

### Fixed (earlier in this cycle)

- **Custom tools were unexecutable over the real MCP protocol**: `execute_tool` only routed `_custom` tools in the test-helper path, so real clients got `MCPNotFoundError` for tools that `search_tools` had just listed. The protocol path now routes them.
- **Overridden tools were permanently flagged stale**: the search index substitutes override descriptions, and staleness checks then hashed the override text as if it were the upstream description (re-saving corrupted the baseline, and the tool cache persisted override text as server descriptions). The index now retains the upstream description and all staleness hashes use it; compact and verbose `get_metadata` now agree.
- **Command injection**: `agnt_watch` command strings and CLI-tool shell mode passed caller-controlled values to the shell unquoted; both now strictly single-quote non-trivial arguments.
- **Path traversal in memory banks**: `mem_*` builtins accepted bank names containing path separators, escaping `~/.config/slop-mcp/memory`. Bank names are now validated everywhere (shared with memory-cli), and reserved `_slop.*` banks are rejected on read entry points too.
- **ES256/384/512 JWT signing panicked**: `signECDSA` passed a nil randomness source to `ecdsa.Sign`.
- **KDL config writing corrupted on special characters**: emitted values (command, args, env, headers) are now escaped, and env/header keys quoted.
- **CLI tool failures were swallowed in SLOP**: a failing CLI tool returned its partial stdout to scripts as success; failures now surface as errors.
- **OAuth**: auth-server metadata discovery no longer nil-panics on 4xx and falls back to `openid-configuration`; refresh responses without `expires_in` no longer mark tokens instantly expired; dynamic client registration registers the real loopback callback URI instead of `localhost:0`; token writes are atomic and serialized; `auth_mcp` timestamps are real UTC.
- **Registry**: connecting no longer holds the registry lock across network I/O (one slow MCP stalled every request); lifecycle operations are serialized per MCP; disconnect/reconnect races and a leaked session on connect-overwrite fixed; a transient tool-listing failure can no longer persist an empty tool cache; `manage_mcps unregister` removes the entry instead of leaving a phantom "disconnected" one, and a failed `register` no longer persists the broken config to the KDL file.
- **`run_slop` with both `recipe` and `file_path` silently discarded the recipe**; now rejected as mutually exclusive.
- Concurrent SLOP runtime construction was a data race (upstream slop v0.3.0 package-level hook); construction is now serialized.

### Changed

- **`run_slop` and custom tools no longer eagerly connect every registered MCP**: SLOP runtimes get one lazy registry-backed service per MCP, so scripts only connect (via the shared registry session) to servers they actually call. Previously every execution spawned and tore down one subprocess per command-transport MCP, even for pure scripts. `customize_tools` listings no longer trigger connections either: staleness for unconnected dependencies reports `stale_status: "unknown"`.
- **Upgraded `modelcontextprotocol/go-sdk` from v1.2.0 to v1.6.1**. This raises the minimum Go toolchain to 1.25.0 (the SDK's new floor). The OAuth discovery flow was migrated to the SDK's revised `oauthex` API: `GetProtectedResourceMetadataFromID` is gone, so the RFC 9728 / RFC 8414 well-known metadata URLs are now derived locally and passed to `GetProtectedResourceMetadata`/`GetAuthServerMeta` alongside the resource/issuer for validation.

## [0.14.3] - 2026-05-31

### Fixed

- **`execute_tool` silently corrupted large integers in `parameters`**: parameters were decoded into `map[string]any`, which coerces every JSON number to `float64`. Integers above 2^53 — lease tokens, snowflake IDs, nanosecond timestamps — were rounded before being forwarded to the target MCP. Tools taking string ids (reads) were unaffected, but any mutation carrying a large integer was silently mangled, making writes appear to "fail silently" through slop while working when the MCP was driven directly. Parameters are now forwarded as verbatim `json.RawMessage`, so the target MCP receives exactly what the caller sent.

## [0.14.2] - 2026-05-06

### Added

- **Wrong-key detection in `execute_tool`**: passing `arguments`, `args`, or `input` instead of the canonical `parameters` field now returns a helpful error naming both the wrong key and the correct one with an example payload, instead of silently running the tool with an empty parameter map.
- **`Notes` field on SLOP function reference**: `slop_help` and `slop_reference --verbose` now surface caveats and cross-refs. Every `mem_*` entry gained notes documenting semantics (default behaviour, glob syntax, case-insensitive match, bank scope).
- **`mem_save` multiline-map warning**: notes explicitly call out that SLOP's parser does not skip newlines between map literal pairs, so maps must stay on a single line. Examples updated to inline literals (`{"task": "review", "done": false}`) so agents copy a working pattern.

### Fixed

- **CLI executor dropped `Env` and `Stdin`**: `Execute()` rebuilt `cmd` to apply the timeout context but restored fields with `cmd.Env = cmd.Env` (self-assignment on the freshly constructed cmd). Environment variables and stdin readers configured upstream were silently lost. Saved and restored via local variables.
- **OAuth `generateState` rand.Read failure**: now falls back to a time-based string on `crypto/rand` failure instead of returning data from a partially-initialised buffer.
- **Monitor tail-loop `f.Seek` failure**: closes the file and skips the cycle instead of reading from the wrong offset.

### Changed

- **`strings.Title` replaced with a local ASCII `titleCase`** helper in template builtins (avoids deprecated stdlib symbol without pulling in `golang.org/x/text` and bumping the Go toolchain requirement).
- **Lint sweep**: cleared all remaining errcheck, staticcheck, gosimple, and unused-symbol findings across the codebase.

## [0.14.1] - 2026-04-17

### Added

- **`monitor` subcommand**: runs a SLOP script as a Claude Code Monitor event source. Each `print()` call becomes a notification line on stdout. Supports inline scripts (`-e`), script files, and `--timeout`. Includes a `changed(key, value)` builtin for delta detection across poll cycles.
- **`message` subcommand**: appends a one-line event to a shared file the running monitor tails. Lets git hooks, build scripts, CI, and file watchers push events into a live monitor session.
- **Recipe scripts**: `monitor_poll_delta` (single-tool polling with delta detection) and `monitor_multi_check` (multi-MCP polling with persistent state via `mem_save`).
- **Monitoring docs**: full concepts page with git hook templates, build wrappers, file watcher patterns, and SLOP polling examples.
- **Image-MCP customization walkthrough**: three-stage compression example in the customization docs (caveman descriptions → hardcoded-value param docs → custom SLOP wrapper with minimal schema).

### Changed

- **Recipe descriptions**: `extractDescription` now parses `#` comment headers in addition to `//`, so SLOP-comment-style recipes pick up their first-line description.
- **Marketing surface**: README, intro, and feature grid now promote tool customization (`customize_tools`) and event monitoring as top-line selling points.

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
