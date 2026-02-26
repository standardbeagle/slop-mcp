# slop-mcp

[![npm version](https://img.shields.io/npm/v/@standardbeagle/slop-mcp)](https://www.npmjs.com/package/@standardbeagle/slop-mcp)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

MCP orchestrator that aggregates multiple Model Context Protocol servers behind 8 meta-tools. Connect any number of MCPs without bloating your agent's context window.

```
Without slop-mcp:  50 MCPs x 20 tools = 1,000 tool definitions in context
With slop-mcp:     50 MCPs x 20 tools = 8 tool definitions in context
```

## Install

```bash
npx @standardbeagle/slop-mcp serve
```

Or install globally:

```bash
npm install -g @standardbeagle/slop-mcp
```

Also available via [PyPI](https://pypi.org/project/slop-mcp/) (`uvx slop-mcp`), [Go](https://pkg.go.dev/github.com/standardbeagle/slop-mcp), and [binary releases](https://github.com/standardbeagle/slop-mcp/releases).

## Configure

Add to Claude Desktop:

```json
{
  "mcpServers": {
    "slop": {
      "command": "npx",
      "args": ["-y", "@standardbeagle/slop-mcp", "serve"]
    }
  }
}
```

Add to Claude Code:

```bash
claude mcp add slop-mcp -- npx -y @standardbeagle/slop-mcp serve
```

**Windows users:** Claude Code on Windows may corrupt the scoped package name. Use `cmd /c` to wrap the npx call:

```json
{
  "mcpServers": {
    "slop": {
      "command": "cmd",
      "args": ["/c", "npx", "-y", "@standardbeagle/slop-mcp@latest", "serve"]
    }
  }
}
```

Define MCP servers in `.slop-mcp.kdl`:

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

## The 8 Meta-Tools

| Tool | Purpose |
|------|---------|
| `search_tools` | Fuzzy search across all connected MCP tools |
| `execute_tool` | Run any tool on any connected MCP |
| `get_metadata` | Inspect tool schemas and MCP capabilities |
| `run_slop` | Execute multi-tool scripts without round-trips |
| `manage_mcps` | Add or remove MCP servers at runtime |
| `auth_mcp` | OAuth authentication for MCPs that need it |
| `slop_reference` | Browse SLOP built-in functions |
| `slop_help` | Get detailed help for a SLOP function |

## Features

- **Progressive discovery** — agents find tools via `search_tools`, not by loading everything upfront
- **SLOP scripting** — chain tool calls and process results in a single `run_slop` call
- **Lazy connections** — MCP servers connect asynchronously with tool metadata caching
- **Persistent memory** — disk-backed `mem_save`/`mem_load`/`mem_search` across sessions
- **Three-tier config** — project-local > project > user config merging with KDL
- **OAuth support** — browser-based auth for MCPs like Figma, GitHub, Linear
- **All transports** — stdio, SSE, and streamable HTTP

## Documentation

**[standardbeagle.github.io/slop-mcp](https://standardbeagle.github.io/slop-mcp/)**

## License

MIT
