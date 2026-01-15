# Configuration Reference

This document describes the configuration options for slop-mcp.

## Configuration Files

slop-mcp uses KDL configuration with three-tier scoping:

| Scope | File | Purpose |
|-------|------|---------|
| User | `~/.config/slop-mcp/config.kdl` | Cross-project defaults |
| Project | `.slop-mcp.kdl` | Git-tracked project config |
| Local | `.slop-mcp.local.kdl` | Git-ignored secrets |

Configuration is merged in priority order: Local > Project > User. Later values override earlier ones.

## MCP Configuration

### Basic Structure

```kdl
mcp "name" {
    type "stdio"         // Transport type: stdio, sse, http, streamable
    command "executable" // For stdio: the command to run
    args "arg1" "arg2"   // Command arguments
    url "https://..."    // For sse/http: the endpoint URL
    timeout "30s"        // Connection timeout (optional)
    env {                // Environment variables (optional)
        VAR "value"
    }
    headers {            // HTTP headers (optional, for sse/http)
        Authorization "Bearer token"
    }
}
```

### Transport Types

| Type | Description | Required Fields |
|------|-------------|-----------------|
| `stdio` | Subprocess via stdin/stdout | `command` |
| `command` | Alias for `stdio` | `command` |
| `sse` | Server-Sent Events | `url` |
| `http` | HTTP Streamable | `url` |
| `streamable` | Alias for `http` | `url` |

## Connection Timeout

Connection timeout controls how long slop-mcp waits when connecting to an MCP server. This is useful for slow-starting servers or network connections.

### Configuration Priority

Timeouts are resolved in the following priority order:

1. **Per-MCP config** (highest priority) - Set in the MCP block
2. **Environment variable** - `SLOP_MCP_TIMEOUT`
3. **Default** - 30 seconds

### Per-MCP Timeout

Configure timeout for a specific MCP server:

```kdl
mcp "slow-server" {
    type "stdio"
    command "slow-mcp-server"
    timeout "60s"  // Wait up to 60 seconds for this MCP
}

mcp "fast-server" {
    type "http"
    url "https://fast.example.com/mcp"
    timeout "10s"  // Shorter timeout for fast server
}
```

### Global Timeout via Environment Variable

Set a global default timeout using the `SLOP_MCP_TIMEOUT` environment variable:

```bash
# Set global timeout to 2 minutes
export SLOP_MCP_TIMEOUT="2m"
slop-mcp serve
```

This affects all MCPs that don't have a per-MCP timeout configured.

### Timeout Format

Timeouts use Go duration format:

| Format | Description |
|--------|-------------|
| `30s` | 30 seconds |
| `2m` | 2 minutes |
| `1m30s` | 1 minute and 30 seconds |
| `5000ms` | 5000 milliseconds (5 seconds) |
| `1h` | 1 hour |

### Examples

**Slow-starting local server:**

```kdl
mcp "heavy-model" {
    type "stdio"
    command "python" "-m" "heavy_model_server"
    timeout "2m"  // Model loading takes time
}
```

**Remote server with network latency:**

```kdl
mcp "remote-api" {
    type "sse"
    url "https://slow-region.example.com/sse"
    timeout "45s"
}
```

**Mix of fast and slow servers:**

```bash
# Set a moderate global default
export SLOP_MCP_TIMEOUT="45s"
```

```kdl
// Uses global default (45s)
mcp "standard-server" {
    type "stdio"
    command "standard-mcp"
}

// Override with longer timeout
mcp "slow-server" {
    type "stdio"
    command "slow-mcp"
    timeout "2m"
}

// Override with shorter timeout
mcp "fast-server" {
    type "http"
    url "https://fast.example.com/mcp"
    timeout "10s"
}
```

### Default Timeout

If no timeout is configured (neither per-MCP nor environment variable), the default is **30 seconds**.

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SLOP_MCP_TIMEOUT` | Global connection timeout | `30s` |
| `XDG_CONFIG_HOME` | User config directory | `~/.config` |

## Complete Configuration Example

```kdl
// Local filesystem access
mcp "filesystem" {
    type "stdio"
    command "npx"
    args "-y" "@anthropic/mcp-filesystem" "/home/user/projects"
}

// Remote API with authentication
mcp "github" {
    type "sse"
    url "https://mcp.github.com/sse"
    timeout "30s"
    // OAuth handled via auth_mcp tool
}

// Slow-starting Python server
mcp "ml-model" {
    type "stdio"
    command "python"
    args "-m" "ml_model_server"
    timeout "2m"
    env {
        MODEL_PATH "/models/large-model"
        CUDA_VISIBLE_DEVICES "0"
    }
}

// HTTP API with custom headers
mcp "private-api" {
    type "http"
    url "https://api.internal.example.com/mcp"
    timeout "15s"
    headers {
        X-API-Key "secret-key"
        X-Client-ID "slop-mcp"
    }
}
```
