# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Philosophy

This project must respect the cost to clients of output length and always use progressive disclosure, providing the exact correct information in one shot.

## Project Overview

slop-mcp is an MCP orchestrator that aggregates multiple MCP servers while exposing only 6 meta-tools to agents. This solves context window bloat: instead of 50 MCPs × 20 tools = 1000 tool definitions, agents see just 6 tools regardless of scale.

## Build & Development Commands

```bash
# Build
make build              # Build slop-mcp and memory-cli to ./build/
make install            # Build and install to ~/.local/bin

# Test
make test               # Unit tests (fast, no network)
make test-mock          # Integration tests with mock MCP (fast)
make test-integration   # Integration tests with real MCPs (slow, requires npx)

# Lint & Format
make lint               # go vet + golangci-lint
make fmt                # Format code
make tidy               # Tidy go.mod
```

### Running a Single Test
```bash
go test -v -run TestHandleSearchTools ./internal/server/...
```

### End-to-End Integration Tests

Tests slop-mcp with real AI agent CLIs (Claude, Gemini, Copilot):

```bash
# Run all integration tests (requires API keys configured)
./scripts/integration-test.sh

# Test specific agent
./scripts/integration-test.sh claude
./scripts/integration-test.sh gemini
./scripts/integration-test.sh copilot

# Via pre-commit (manual stage)
pre-commit run integration-test --hook-stage manual

# Verbose output
SLOP_TEST_VERBOSE=1 ./scripts/integration-test.sh
```

**Requirements:**
- Built binary (`make build` first)
- API keys configured for each CLI
- CLIs installed: `claude`, `gemini`, `copilot`

### Build Requirements
- Go 1.24.0+
- Build tags: `-tags mcp_go_client_oauth` (required for OAuth support)

## Architecture

### Package Structure

```
cmd/
├── slop-mcp/           # Main server binary (serve, run, mcp subcommands)
└── memory-cli/         # Persistent memory tool for agent sessions

internal/
├── server/             # MCP server, tool handlers, JSON schemas
├── registry/           # MCP connections, state tracking, tool search index
├── config/             # KDL config loading with three-tier scoping
├── cli/                # Local CLI tool registry (executed without MCP overhead)
└── auth/               # OAuth token storage and automatic refresh
```

### The 6 Meta-Tools

| Tool | Handler |
|------|---------|
| `search_tools` | Fuzzy search across all MCP tools (ranked by relevance, paginated) |
| `execute_tool` | Execute any tool on any connected MCP |
| `get_metadata` | Fetch full metadata (compact by default, verbose=true for schemas) |
| `run_slop` | Execute SLOP scripts with multi-MCP access |
| `manage_mcps` | Register/unregister MCPs at runtime |
| `auth_mcp` | OAuth authentication (login, logout, status, list) |

### Key Design Patterns

**Registry with RWMutex**: `internal/registry/registry.go` uses `sync.RWMutex` for concurrent tool searches while allowing exclusive writes for connection changes.

**MCP State Machine**: MCPs track state through: `configured` → `connecting` → `connected` (or `error`/`needs_auth`). State persists for disconnected MCPs to enable reconnection.

**Ranked Tool Search**: `internal/registry/index.go` scores matches (exact name=1000, MCP match=800, prefix=300, etc.) and supports normalized fuzzy matching (ignores case, underscores, hyphens).

**Rich Error Messages**: Custom error types (`MCPNotFoundError`, `ToolNotFoundError`, `InvalidParameterError`) include available options, similar suggestions, and parameter schemas.

**Three-Tier Config**: Configs merge in priority order: Local (`.slop-mcp.local.kdl`, git-ignored) > Project (`.slop-mcp.kdl`) > User (`~/.config/slop-mcp/config.kdl`).

### Transport Types

```go
// internal/registry/registry.go Connect()
"command"/"stdio" → mcp.CommandTransport (subprocess)
"sse"             → mcp.SSEClientTransport (Server-Sent Events)
"http"/"streamable" → mcp.StreamableClientTransport
```

## Testing Patterns

Integration tests use build tag `integration` and can run against:
- **Mock MCP**: `USE_MOCK_MCP=1` enables fast deterministic tests via `internal/server/testdata/mock-mcp`
- **Real MCPs**: Requires `npx` for `@modelcontextprotocol/server-everything`

Unit tests in `internal/server/handlers_test.go` use `registry.AddToolsForTesting()` to bypass connection flow.

## Configuration Format

Uses KDL (not YAML/JSON). Example:

```kdl
mcp "filesystem" {
    command "npx" "-y" "@anthropic/mcp-filesystem"
    args "/path/to/dir"
}

mcp "github" {
    transport "sse"
    url "https://mcp.github.com/sse"
}
```

## Pre-Commit Hooks

Configured in `.pre-commit-config.yaml`:
- golangci-lint on `.go` files
- `go test -short ./...` and `go build ./...` before commit

## Release Process

### Version Locations

Versions are tracked in multiple places that must stay in sync:

| Location | Purpose | Updated By |
|----------|---------|------------|
| `internal/server/server.go` | `serverVersion` constant for MCP server info | Release script |
| `CHANGELOG.md` | Version history with `## [X.Y.Z]` headers | Manual (before release) |
| `cmd/slop-mcp/main.go` | `Version` var (default "dev") | CI via `-ldflags` at build |
| `npm/package.json` | npm package version (default "0.0.0") | CI at publish time |
| `pyproject.toml` | PyPI package version (default "0.0.0") | CI at publish time |

### Creating a Release

Use the release script to ensure version sync and automate the release:

```bash
./scripts/release.sh 0.10.0
```

The script will:
1. Validate semver format
2. Update `serverVersion` in `internal/server/server.go`
3. Verify CHANGELOG.md has an entry for the version
4. Run build and tests
5. Commit version updates (if any)
6. Create and push git tag `vX.Y.Z`
7. Create GitHub release (triggers CI)

### CI/CD Pipeline

On release publish (`.github/workflows/release.yml`):
1. Builds Go binaries for linux/darwin/windows × amd64/arm64
2. Injects version via `-ldflags="-X main.Version=$TAG"`
3. Uploads binaries to GitHub release
4. Publishes to PyPI (updates `pyproject.toml` version)
5. Publishes to npm (updates `package.json` version)

### Manual Release Checklist

If not using the script:

1. Update `CHANGELOG.md` with new version section
2. Update `serverVersion` in `internal/server/server.go`
3. Commit: `git commit -m "chore: prepare release vX.Y.Z"`
4. Tag: `git tag vX.Y.Z`
5. Push: `git push origin main && git push origin vX.Y.Z`
6. Create GitHub release from the tag

## MCP Protocol Compliance

### Stdout Reservation

**Critical**: stdout is reserved exclusively for MCP JSON-RPC protocol. Never write to stdout during serve mode:

- Use `s.logger.Warn/Info/Debug()` for diagnostics (writes to stderr)
- Return errors via `errorResult()` for tool-level errors (`isError: true`)
- Return `(nil, err)` only for malformed requests (JSON-RPC protocol error)
- User-facing messages (e.g., OAuth prompts) must use `fmt.Fprintf(os.Stderr, ...)`

### Error Handling

Tool handlers should return MCP-compliant responses:

```go
// Tool-level error (tool ran but failed) - preferred for business logic errors
return errorResult(fmt.Errorf("invalid parameters: %w", err)), nil

// Protocol-level error (request malformed) - for JSON-RPC layer issues
return nil, err
```
