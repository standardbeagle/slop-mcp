---
sidebar_position: 3
---

# Configuration

SLOP MCP uses KDL (KDL Document Language) for configuration files. This guide covers everything you need to know about configuring your MCPs.

## Configuration Scopes

SLOP MCP supports three configuration scopes, loaded in order of precedence:

| Scope | File | Purpose |
|-------|------|---------|
| **Local** | `.slop-mcp.local.kdl` | Personal overrides, gitignored |
| **Project** | `.slop-mcp.kdl` | Shared project config, committed |
| **User** | `~/.config/slop-mcp/config.kdl` | Global user defaults |

### Using Scopes

```bash
# Add to project config (default)
slop-mcp mcp add my-mcp npx my-mcp

# Add to local config (gitignored)
slop-mcp mcp add -l my-secret-mcp npx secret-mcp

# Add to user config (global)
slop-mcp mcp add -u my-global-mcp npx global-mcp

# Or use --scope explicitly
slop-mcp mcp add --scope=local my-mcp npx my-mcp
```

## KDL Syntax

### Basic MCP Definition

```kdl
mcp "my-mcp" {
    transport "stdio"
    command "npx"
    args "my-mcp-package"
}
```

### Transport Types

#### stdio (default)
For command-line MCPs that communicate via stdin/stdout:

```kdl
mcp "filesystem" {
    transport "stdio"
    command "npx"
    args "@anthropic/filesystem-mcp" "/allowed/path"
}
```

#### streamable (HTTP)
For HTTP-based MCPs with streaming support:

```kdl
mcp "figma" {
    transport "streamable"
    url "https://mcp.figma.com/mcp"
}
```

#### sse (Server-Sent Events)
For MCPs using SSE transport:

```kdl
mcp "my-sse-mcp" {
    transport "sse"
    url "https://example.com/mcp/sse"
}
```

### Environment Variables

Pass environment variables to stdio MCPs:

```kdl
mcp "my-mcp" {
    transport "stdio"
    command "node"
    args "server.js"
    env {
        API_KEY "secret-key"
        DEBUG "true"
    }
}
```

### Multiple Arguments

```kdl
mcp "filesystem" {
    transport "stdio"
    command "npx"
    args "@anthropic/filesystem-mcp" "/path/one" "/path/two"
}
```

## Full Example

Here's a complete `.slop-mcp.kdl` with multiple MCPs:

```kdl
// Math and calculation tools
mcp "math-mcp" {
    transport "stdio"
    command "npx"
    args "@anthropic/math-mcp"
}

// File system access (restricted paths)
mcp "filesystem" {
    transport "stdio"
    command "npx"
    args "@anthropic/filesystem-mcp" "./src" "./docs"
}

// Figma design integration
mcp "figma" {
    transport "streamable"
    url "https://mcp.figma.com/mcp"
}

// Dart project management
mcp "dart" {
    transport "streamable"
    url "https://mcp.dartai.com/mcp"
}

// Custom Python MCP
mcp "my-analyzer" {
    transport "stdio"
    command "python"
    args "-m" "my_analyzer.server"
    env {
        PYTHONPATH "/path/to/modules"
    }
}
```

## Configuration Precedence

When the same MCP is defined in multiple scopes, local takes precedence:

1. `.slop-mcp.local.kdl` (highest priority)
2. `.slop-mcp.kdl`
3. `~/.config/slop-mcp/config.kdl` (lowest priority)

This lets you override project settings locally without modifying committed files.

## Viewing Effective Configuration

```bash
# See all loaded MCPs and their sources
slop-mcp mcp list

# Output shows source scope
MCPs:
  math-mcp (stdio) from project - connected, 5 tools
  my-secret (stdio) from local - connected, 3 tools
  global-util (stdio) from user - connected, 2 tools
```

## Tips

### Gitignore Local Config

Add to your `.gitignore`:

```
.slop-mcp.local.kdl
```

### Validate Config

```bash
# Attempt to load config and show any errors
slop-mcp mcp list
```

### Environment Variable Expansion

Environment variables in KDL values are expanded:

```kdl
mcp "my-mcp" {
    transport "stdio"
    command "node"
    args "server.js"
    env {
        API_KEY "${MY_API_KEY}"  // Expands from environment
    }
}
```

## Next Steps

- [Context Efficiency](/docs/concepts/context-efficiency) - How SLOP optimizes context usage
- [Skills](/docs/concepts/skills) - Create reusable MCP workflows
- [CLI Reference](/docs/reference/cli) - Full command documentation
