# CLAUDE.md

此檔示 Claude Code (claude.ai/code) 於此倉行事之準。

## Project Philosophy

本案重客戶 token 成本；必用 progressive disclosure，一答即給精確所需。

## Project Overview

slop-mcp，MCP orchestrator 也；聚多 MCP servers，而僅示 agents 8 meta-tools。解 context window bloat：非 50 MCPs × 20 tools = 1000 tool definitions，無論規模，agents 但見 8 tools。

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

以真 AI agent CLIs（Claude, Gemini, Copilot）驗 slop-mcp：

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
- Go 1.25.0+ (required by go-sdk v1.6.1)
- Build tags: `-tags mcp_go_client_oauth`（OAuth 必需）

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

### The 8 Meta-Tools

| Tool | Handler |
|------|---------|
| `search_tools` | 跨 MCP tools fuzzy search，ranked relevance，paginated |
| `execute_tool` | 於任 connected MCP 執任 tool |
| `get_metadata` | 取 full metadata；默 compact，`verbose=true` 得 schemas |
| `run_slop` | 執 SLOP scripts，可訪多 MCP |
| `manage_mcps` | runtime register/unregister MCPs |
| `auth_mcp` | OAuth authentication (`login`, `logout`, `status`, `list`) |
| `slop_reference` | 依 name/category 搜 SLOP built-in functions |
| `slop_help` | 取某 SLOP function full details |

### Key Design Patterns

**Registry with RWMutex**：`internal/registry/registry.go` 用 `sync.RWMutex`；tool search 並行讀，connection changes 獨占寫。

**MCP State Machine**：MCPs 狀態：`configured` → `connecting` → `connected`（或 `error`/`needs_auth`）。disconnected MCPs 仍存狀態，以便 reconnection。

**Ranked Tool Search**：`internal/registry/index.go` 計分（exact name=1000, MCP match=800, prefix=300, etc.），支援 normalized fuzzy matching（忽 case, underscores, hyphens）。

**Rich Error Messages**：custom error types (`MCPNotFoundError`, `ToolNotFoundError`, `InvalidParameterError`) 含 available options、similar suggestions、parameter schemas。

**Three-Tier Config**：Configs 依優先序 merge：Local (`.slop-mcp.local.kdl`, git-ignored) > Project (`.slop-mcp.kdl`) > User (`~/.config/slop-mcp/config.kdl`)。

### Transport Types

```go
// internal/registry/registry.go Connect()
"command"/"stdio" → mcp.CommandTransport (subprocess)
"sse"             → mcp.SSEClientTransport (Server-Sent Events)
"http"/"streamable" → mcp.StreamableClientTransport
```

## Testing Patterns

Integration tests 用 build tag `integration`，可對：
- **Mock MCP**：`USE_MOCK_MCP=1` 啟 fast deterministic tests via `internal/server/testdata/mock-mcp`
- **Real MCPs**：需 `npx` for `@modelcontextprotocol/server-everything`

Unit tests 於 `internal/server/handlers_test.go` 用 `registry.AddToolsForTesting()` 繞 connection flow。

## Configuration Format

用 KDL，非 YAML/JSON。例：

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

見 `.pre-commit-config.yaml`：
- golangci-lint on `.go` files
- `go test -short ./...` and `go build ./...` before commit

## Release Process

### Version Locations

版本散列多處，須同步：

| Location | Purpose | Updated By |
|----------|---------|------------|
| `internal/server/server.go` | `serverVersion` constant for MCP server info | Release script |
| `CHANGELOG.md` | Version history with `## [X.Y.Z]` headers | Manual (before release) |
| `cmd/slop-mcp/main.go` | `Version` var (default "dev") | CI via `-ldflags` at build |
| `npm/package.json` | npm package version (default "0.0.0") | CI at publish time |
| `pyproject.toml` | PyPI package version (default "0.0.0") | CI at publish time |

### Creating a Release

用 release script 保 version sync 並自動 release：

```bash
./scripts/release.sh 0.10.0
```

Script 將：
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

不用 script 則：

1. Update `CHANGELOG.md` with new version section
2. Update `serverVersion` in `internal/server/server.go`
3. Commit: `git commit -m "chore: prepare release vX.Y.Z"`
4. Tag: `git tag vX.Y.Z`
5. Push: `git push origin main && git push origin vX.Y.Z`
6. Create GitHub release from the tag

## MCP Protocol Compliance

### Stdout Reservation

**Critical**：stdout 唯供 MCP JSON-RPC protocol。serve mode 勿寫 stdout：

- 用 `s.logger.Warn/Info/Debug()` 診斷（寫 stderr）
- tool-level errors 經 `errorResult()` 回（`isError: true`）
- 僅 malformed requests 回 `(nil, err)`（JSON-RPC protocol error）
- User-facing messages（如 OAuth prompts）須用 `fmt.Fprintf(os.Stderr, ...)`

### Error Handling

Tool handlers 宜回 MCP-compliant responses：

```go
// Tool-level error (tool ran but failed) - preferred for business logic errors
return errorResult(fmt.Errorf("invalid parameters: %w", err)), nil

// Protocol-level error (request malformed) - for JSON-RPC layer issues
return nil, err
```