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
| `env` | block | No | Environment variables (merged with system env), passed literally |
| `devkey` | string | No | devkey key name(s), comma-separated. Launches the server via `devkey run` so secrets stay out of this file — see [Secrets](#secrets-use-devkey-not-env) |

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
        DEBUG "true"
        LOG_LEVEL "info"
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
        X-API-Key "literal-value-or-see-devkey"
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

### Values are literal

`env` and `headers` values are passed to the server exactly as written. There is
no `${VAR}` expansion. Writing `API_KEY "${MY_API_KEY}"` sends the server the
thirteen characters `${MY_API_KEY}`, not the value of that variable.

Expansion does exist for CLI tools declared with `tool`, where it is opt-in per
tool via `expand_env true`. It has never applied to `mcp` blocks.

### Shell Expansion

To compute a value, run the server under a shell and let the shell do it:

```kdl
mcp "my-mcp" {
    command "sh"
    args "-c" "MY_VAR=$(cat /path/to/file) exec node server.js"
}
```

### Secrets: use `devkey`, not `env`

A token written into `env` is a plaintext secret in a config file. It survives
in backups, in anything that syncs the file, and in the output of any command
that prints the config while debugging.

Instead, name the key and let the [devkey](https://github.com/standardbeagle/dev-setup)
broker inject it:

```kdl
mcp "figma-query" {
    command "figma-query"
    devkey "figma"
}
```

slop-mcp launches the server as `devkey run figma -- figma-query`. devkey puts
the values in the child's environment itself, so the secret never enters
slop-mcp's memory, its config, or its logs. Keep using `env` alongside it for
everything that is not a secret.

A server needing more than one key names them comma-separated, matching
`devkey run`:

```kdl
    devkey "figma,openrouter"
```

The key must be declared in devkey's registry and granted on this machine
(`devkey approve <key> --ttl 30m`). Grants are per-machine and time-boxed, so a
server spawned after its grant expires fails to connect until it is re-approved.

If `devkey` is not on `PATH` — common when an agent runs from a non-login shell
— set `SLOP_DEVKEY_BINARY` to its full path. Without it the connection fails
with an error naming the missing broker rather than a generic transport error.

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
        PYTHONPATH "/home/me/projects/analyzer"
        LOG_LEVEL "info"
    }
}

mcp "internal-api" {
    transport "http"
    url "https://internal.company.com/mcp"
    headers {
        Authorization "Bearer <token>"
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

### 5. Keep Secrets Out of Config Entirely

Prefer `devkey` over a literal in any file, local or not. Gitignoring a config
keeps it off the remote; it does not keep it out of backups, out of a synced
home directory, or out of a terminal that printed the file while debugging.

```kdl
mcp "secret-mcp" {
    command "secret-server"
    devkey "my-service"
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
