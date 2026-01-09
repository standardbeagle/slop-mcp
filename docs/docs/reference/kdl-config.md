---
sidebar_position: 3
---

# KDL Configuration Reference

SLOP MCP uses [KDL (KDL Document Language)](https://kdl.dev/) for configuration files.

## File Locations

| File | Scope | Purpose |
|------|-------|---------|
| `.slop-mcp.local.kdl` | Local | Personal overrides, gitignored |
| `.slop-mcp.kdl` | Project | Shared config, committed |
| `~/.config/slop-mcp/config.kdl` | User | Global defaults |

## MCP Definition

### Basic Structure

```kdl
mcp "<name>" {
    transport "<type>"
    // ... transport-specific options
}
```

### Transport Types

#### stdio (default)

For command-line MCPs:

```kdl
mcp "my-mcp" {
    transport "stdio"
    command "<executable>"
    args "<arg1>" "<arg2>" ...
    env {
        KEY "value"
    }
}
```

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `transport` | string | No | "stdio" (default) |
| `command` | string | Yes | Executable command |
| `args` | strings | No | Command arguments |
| `env` | block | No | Environment variables (merged with system env) |

Example:

```kdl
mcp "filesystem" {
    transport "stdio"
    command "npx"
    args "@anthropic/filesystem-mcp" "./src" "./docs"
}

mcp "custom" {
    command "python"
    args "-m" "my_server"
    env {
        API_KEY "${MY_API_KEY}"
        DEBUG "true"
    }
}
```

#### streamable (HTTP)

For HTTP MCPs with streaming:

```kdl
mcp "my-mcp" {
    transport "streamable"
    url "<endpoint>"
    headers {
        KEY "value"
    }
}
```

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `transport` | string | Yes | "streamable" or "http" |
| `url` | string | Yes | HTTP endpoint URL |
| `headers` | block | No | HTTP headers |

Example:

```kdl
mcp "figma" {
    transport "streamable"
    url "https://mcp.figma.com/mcp"
}

mcp "custom-api" {
    transport "http"
    url "https://api.example.com/mcp"
    headers {
        X-API-Key "${API_KEY}"
    }
}
```

#### sse (Server-Sent Events)

For SSE-based MCPs:

```kdl
mcp "my-mcp" {
    transport "sse"
    url "<endpoint>"
    headers {
        KEY "value"
    }
}
```

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `transport` | string | Yes | "sse" |
| `url` | string | Yes | SSE endpoint URL |
| `headers` | block | No | HTTP headers |

## Environment Variables

### Inline Expansion

Environment variables are expanded in values:

```kdl
mcp "my-mcp" {
    command "node"
    args "server.js"
    env {
        API_KEY "${MY_API_KEY}"
        HOME_DIR "${HOME}/app"
    }
}
```

### Shell Expansion

For complex expansions:

```kdl
mcp "my-mcp" {
    command "sh"
    args "-c" "MY_VAR=$(cat /path/to/file) exec node server.js"
}
```

## Comments

KDL supports two comment styles:

```kdl
// Single-line comment

/*
 * Multi-line
 * comment
 */

mcp "example" {
    transport "stdio"
    command "npx"
    // This MCP is for development only
    args "dev-mcp"
}
```

## Complete Example

```kdl
// ===========================================
// SLOP MCP Configuration
// ===========================================

// -----------------------------
// Development Tools
// -----------------------------

mcp "filesystem" {
    transport "stdio"
    command "npx"
    args "@anthropic/filesystem-mcp" "./src" "./tests" "./docs"
}

mcp "git" {
    transport "stdio"
    command "npx"
    args "@anthropic/git-mcp"
}

// -----------------------------
// Math & Computation
// -----------------------------

mcp "math-mcp" {
    transport "stdio"
    command "npx"
    args "@andylbrummer/math-mcp"
}

// -----------------------------
// Cloud Services (OAuth)
// -----------------------------

mcp "figma" {
    transport "streamable"
    url "https://mcp.figma.com/mcp"
}

mcp "linear" {
    transport "streamable"
    url "https://mcp.linear.app/mcp"
}

mcp "github" {
    transport "streamable"
    url "https://mcp.github.com/mcp"
}

// -----------------------------
// Custom MCPs
// -----------------------------

mcp "my-analyzer" {
    transport "stdio"
    command "python"
    args "-m" "analyzer.server"
    env {
        PYTHONPATH "${HOME}/projects/analyzer"
        LOG_LEVEL "info"
    }
}

mcp "internal-api" {
    transport "http"
    url "https://internal.company.com/mcp"
    headers {
        Authorization "Bearer ${INTERNAL_TOKEN}"
        X-Team "engineering"
    }
}
```

## Validation

Check your configuration:

```bash
slop-mcp mcp list
```

If there are syntax errors, you'll see:

```
Error loading config: .slop-mcp.kdl:15:3 - unexpected token
```

## Tips

### 1. Use Quotes for Values with Spaces

```kdl
mcp "my-mcp" {
    args "path with spaces" "another arg"
}
```

### 2. Escape Special Characters

```kdl
mcp "my-mcp" {
    env {
        QUERY "SELECT * FROM \"users\""
    }
}
```

### 3. Multi-line Strings

```kdl
mcp "my-mcp" {
    env {
        LONG_VALUE r#"
            This is a
            multi-line
            value
        "#
    }
}
```

### 4. Organize with Comments

Group related MCPs with comment headers for clarity.

### 5. Keep Secrets in Local Config

```kdl
// .slop-mcp.local.kdl (gitignored)
mcp "secret-mcp" {
    env {
        API_KEY "actual-secret-key"
    }
}
```

## Migration from JSON

If you have JSON MCP configs, convert them:

```bash
# From Claude Desktop config
slop-mcp mcp add-from-claude-desktop

# From JSON string
slop-mcp mcp add-json my-mcp '{"command": "npx", "args": ["my-mcp"]}'
```
