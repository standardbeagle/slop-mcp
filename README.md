# slop-mcp

**Install many MCPs without killing your context.**

slop-mcp is an MCP orchestrator that lets you connect dozens of MCP servers while exposing only 5 meta-tools to your agent. No more context window bloat from loading hundreds of tool definitions upfront.

```
Without slop-mcp:  50 MCPs × 20 tools = 1000 tool definitions in context
With slop-mcp:     50 MCPs × 20 tools = 5 tool definitions in context
```

## Documentation

- **[Quick Start Guide](https://standardbeagle.github.io/slop-mcp/docs/getting-started/quick-start)** - Get running in 5 minutes
- **[Full Documentation](https://standardbeagle.github.io/slop-mcp/)** - Complete guides and reference
- **[KDL Configuration](https://standardbeagle.github.io/slop-mcp/docs/reference/kdl-config)** - Configuration reference
- **[CLI Reference](https://standardbeagle.github.io/slop-mcp/docs/reference/cli)** - Command-line options

## The Problem

As described in Anthropic's article [Code Execution with MCP](https://www.anthropic.com/engineering/code-execution-with-mcp), current MCP implementations face two critical challenges:

1. **Context Window Overload**: When agents connect to many tools, loading all tool definitions upfront consumes excessive tokens. With thousands of connected tools, agents must process hundreds of thousands of tokens before even reading user requests.

2. **Intermediate Result Duplication**: Tool outputs repeatedly flow through the model's context. Transferring large documents between services forces the same data through the model between operations, potentially doubling token consumption.

The article proposes code execution within MCP as a solution—letting agents discover tools progressively and process data within the execution environment rather than shuttling everything through context.

## How slop-mcp Addresses These Issues

slop-mcp takes a different but complementary approach: instead of code execution, it provides an **orchestration layer** that aggregates multiple MCP servers while maintaining context efficiency.

### Progressive Tool Discovery

Rather than loading all tool definitions upfront, slop-mcp exposes just 5 meta-tools:

| Tool | Purpose |
|------|---------|
| `search_tools` | Find tools across all connected MCPs by name or description |
| `execute_tool` | Execute a specific tool on a specific MCP |
| `run_slop` | Execute SLOP scripts with access to all MCPs |
| `manage_mcps` | Register/unregister MCPs at runtime |
| `auth_mcp` | Handle OAuth authentication for MCPs that require it |

This means an agent connecting to slop-mcp sees **5 tool definitions** regardless of how many MCPs are connected or how many tools they expose. The agent discovers tools on-demand via `search_tools` and executes them via `execute_tool`.

### Lazy Connection & Async Startup

MCP servers connect asynchronously in the background:

```
Server starts → Immediately ready to serve
                ↓ (background)
                MCP #1 connecting...
                MCP #2 connecting...
                MCP #N connecting...
```

The server doesn't block waiting for all MCPs to connect. Tools become available progressively as their MCPs come online.

### In-Environment Script Execution

The `run_slop` tool allows executing structured scripts that can:
- Call multiple tools across different MCPs
- Process intermediate results without sending them back through the model
- Chain operations efficiently

This keeps large intermediate data within the execution environment, addressing the token duplication problem.

### Efficient Tool Index

Tools are indexed locally when MCPs connect:
- Fuzzy search by name or description
- Filter by MCP name
- No network calls during search
- Thread-safe concurrent access

## Architecture

```
┌─────────────────────────────────────────────────────┐
│              slop-mcp Server                        │
│  ┌───────────────────────────────────────────────┐  │
│  │  5 Meta-Tools (constant context cost)         │  │
│  │  • search_tools    • execute_tool             │  │
│  │  • run_slop        • manage_mcps              │  │
│  │  • auth_mcp                                   │  │
│  └───────────────────────────────────────────────┘  │
│                          │                          │
│         ┌────────────────┼────────────────┐         │
│         ▼                ▼                ▼         │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐    │
│  │  Registry  │  │ Tool Index │  │   Auth     │    │
│  │  (async)   │  │  (local)   │  │  (OAuth)   │    │
│  └─────┬──────┘  └────────────┘  └────────────┘    │
└────────┼────────────────────────────────────────────┘
         │
    ┌────┼────┬─────────────┐
    ▼    ▼    ▼             ▼
┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐
│MCP #1│ │MCP #2│ │MCP #3│ │MCP #N│
│stdio │ │ SSE  │ │ HTTP │ │ ...  │
└──────┘ └──────┘ └──────┘ └──────┘
```

## Configuration

slop-mcp uses KDL configuration with three-tier scoping:

| Scope | File | Purpose |
|-------|------|---------|
| User | `~/.config/slop-mcp/config.kdl` | Cross-project defaults |
| Project | `.slop-mcp.kdl` | Git-tracked project config |
| Local | `.slop-mcp.local.kdl` | Git-ignored secrets |

Example configuration:

```kdl
mcp "filesystem" {
    command "npx" "-y" "@anthropic/mcp-filesystem"
    args "/path/to/allowed/dir"
}

mcp "github" {
    transport "sse"
    url "https://mcp.github.com/sse"
    // OAuth handled automatically via auth_mcp tool
}
```

Import existing configurations:

```kdl
import "claude-desktop"  // Import from Claude Desktop config
import "claude-code"     // Import from Claude Code settings
```

## Quick Start

### npm

```bash
npx @standardbeagle/slop-mcp
```

### PyPI

```bash
uvx slop-mcp
```

Or install globally:

```bash
# npm
npm install -g @standardbeagle/slop-mcp

# pip
pip install slop-mcp
```

### From Source

```bash
go install github.com/standardbeagle/slop-mcp/cmd/slop-mcp@latest
```

## Usage

### As an MCP Server (stdio)

```bash
slop-mcp serve
```

### With HTTP/SSE Transport

```bash
slop-mcp serve --transport http --port 8080
```

### Claude Desktop Configuration

Add to your Claude Desktop config:

```json
{
  "mcpServers": {
    "slop": {
      "command": "slop-mcp",
      "args": ["serve"]
    }
  }
}
```

## Comparison with Code Execution Approach

| Aspect | Code Execution (Article) | slop-mcp |
|--------|-------------------------|----------|
| Tool Discovery | Filesystem exploration | `search_tools` with fuzzy matching |
| Context Cost | Minimal (code interpreter) | Constant (5 meta-tools) |
| Data Processing | In-sandbox code | SLOP scripts via `run_slop` |
| Infrastructure | Secure sandbox required | Standard MCP servers |
| Flexibility | Full code execution | Structured tool orchestration |

Both approaches solve the same core problems. Code execution offers maximum flexibility but requires sandboxing infrastructure. slop-mcp provides a simpler deployment model while still achieving significant context efficiency gains.

## Related Projects

- [standardbeagle-tools](https://github.com/standardbeagle/standardbeagle-tools) - Claude Code plugin for slop-mcp integration

## License

MIT
