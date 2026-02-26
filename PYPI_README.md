# slop-mcp

[![PyPI version](https://img.shields.io/pypi/v/slop-mcp)](https://pypi.org/project/slop-mcp/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

MCP orchestrator that aggregates multiple Model Context Protocol servers behind 8 meta-tools. Connect any number of MCPs without bloating your agent's context window.

```
Without slop-mcp:  50 MCPs x 20 tools = 1,000 tool definitions in context
With slop-mcp:     50 MCPs x 20 tools = 8 tool definitions in context
```

## Install

```bash
uvx slop-mcp serve
```

Or install globally:

```bash
pip install slop-mcp
```

Also available via [npm](https://www.npmjs.com/package/@standardbeagle/slop-mcp) (`npx @standardbeagle/slop-mcp`), [Go](https://pkg.go.dev/github.com/standardbeagle/slop-mcp), and [binary releases](https://github.com/standardbeagle/slop-mcp/releases).

## Configure

Add to Claude Desktop:

```json
{
  "mcpServers": {
    "slop": {
      "command": "uvx",
      "args": ["slop-mcp", "serve"]
    }
  }
}
```

Add to Claude Code:

```bash
claude mcp add slop-mcp -- uvx slop-mcp serve
```

**Windows users:** Use `cmd /c` to wrap the command if path handling corrupts the args:

```json
{
  "mcpServers": {
    "slop": {
      "command": "cmd",
      "args": ["/c", "uvx", "slop-mcp", "serve"]
    }
  }
}
```

Define MCP servers in `.slop-mcp.kdl`:

```kdl
mcp "github" {
    transport "sse"
    url "https://mcp.github.com/sse"
}

mcp "jira" {
    transport "streamable"
    url "https://mcp.atlassian.com/v2/mcp"
}

mcp "lci" {
    command "lci" "mcp"
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

## Examples

### Cross-MCP orchestration with SLOP

Chain tools across multiple MCPs in a single `run_slop` call — intermediate results stay out of the agent's context:

```python
# Create Jira tasks from unread emails matching a filter
emails = gmail.search_messages(query: "label:action-needed is:unread")
for email in emails {
    jira.create_issue(
        project: "OPS",
        summary: email["subject"],
        description: format("From: {}\n\n{}", email["from"], email["snippet"]),
        issue_type: "Task"
    )
    gmail.modify_message(id: email["id"], remove_labels: ["UNREAD"])
}
emit(created: len(emails))
# Agent sees only: {"created": 4}
```

```python
# Index codebase structure into persistent memory for future sessions
results = lci.search(query: "public API endpoints")
endpoints = results
    | map(|r| {"path": r["file"], "name": r["symbol"], "kind": r["kind"]})
    | filter(|r| r["kind"] == "function")
mem_save("project", "api_endpoints", endpoints,
    description: "Public API endpoint inventory")
emit(indexed: len(endpoints))
```

```python
# Generate a visual report from code analysis
stats = lci.search(query: "struct")
by_package = stats
    | map(|r| r["file"] | split("/") | first())
    | group_by(|pkg| pkg)
chart_data = by_package
    | items()
    | map(|pair| {"label": pair[0], "value": len(pair[1])})
    | sorted(|a, b| b["value"] - a["value"])
banana.create_chart(type: "bar", title: "Structs by Package", data: chart_data)
```

## Features

- **Progressive discovery** — agents find tools via `search_tools`, not by loading everything upfront
- **SLOP scripting** — chain tool calls across MCPs and process results in a single `run_slop` call
- **Lazy connections** — MCP servers connect asynchronously with tool metadata caching
- **Persistent memory** — disk-backed `mem_save`/`mem_load`/`mem_search` across sessions
- **Three-tier config** — project-local > project > user config merging with KDL
- **OAuth support** — browser-based auth for MCPs like Figma, GitHub, Linear, Jira
- **All transports** — stdio, SSE, and streamable HTTP

## Documentation

**[standardbeagle.github.io/slop-mcp](https://standardbeagle.github.io/slop-mcp/)**

## License

MIT
