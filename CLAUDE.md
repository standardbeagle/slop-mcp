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
