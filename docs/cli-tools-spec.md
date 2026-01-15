# CLI Tools Metadata Specification

## Overview

This specification defines the KDL format for describing standalone CLI tools that can be called through slop-mcp's `execute_tool` interface. CLI tools are different from MCP servers - they are regular executables that don't speak the MCP protocol but can be wrapped and exposed as MCP tools.

## Design Goals

1. **KDL-native**: Use KDL format consistent with existing slop-mcp config
2. **Schema-compatible**: Generate valid MCP tool schemas for LLM consumption
3. **Minimal boilerplate**: Simple tools should have simple definitions
4. **Full control**: Complex tools can specify detailed argument handling
5. **Safe by default**: Sandboxing and resource limits built-in

## File Locations

CLI tool definitions are loaded from:

1. **User scope**: `~/.config/slop-mcp/cli/*.kdl`
2. **Project scope**: `.slop-mcp/cli/*.kdl`
3. **Inline**: Within main config files using `cli` blocks

Files are merged in order (user < project), with later definitions overriding earlier ones.

## Basic Syntax

### Minimal Definition

```kdl
cli "jq" {
    description "Command-line JSON processor"
    command "jq"
}
```

This creates a tool that:
- Accepts a single `args` parameter (string or array)
- Passes args directly to the command
- Returns stdout as the result

### Full Definition

```kdl
cli "jq" {
    description "Process JSON with jq filters"
    command "jq"

    // Positional arguments (ordered by position)
    arg "filter" {
        description "jq filter expression"
        required true
        position 0
        type "string"
    }

    arg "file" {
        description "Input file (optional, uses stdin if omitted)"
        required false
        position 1
        type "string"
    }

    // Named flags
    flag "raw-output" {
        short "-r"
        long "--raw-output"
        description "Output raw strings, not JSON"
        type "boolean"
    }

    flag "compact" {
        short "-c"
        long "--compact-output"
        description "Compact output"
        type "boolean"
    }

    flag "slurp" {
        short "-s"
        long "--slurp"
        description "Read entire input into array"
        type "boolean"
    }

    flag "null-input" {
        short "-n"
        long "--null-input"
        description "Don't read any input"
        type "boolean"
    }

    // Input/output handling
    stdin {
        description "JSON input to process"
        type "string"
        format "json"
    }

    stdout {
        type "string"
        format "auto"  // "json", "text", "auto"
    }

    stderr {
        capture true  // Include in error responses
    }

    // Execution options
    timeout 30000  // milliseconds
    workdir "."    // Working directory (default: current)

    // Environment
    env {
        LC_ALL "C"
    }
}
```

## Schema Elements

### `cli` Node

The root node for a CLI tool definition.

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| (name) | string | yes | - | Tool name (first arg to `cli`) |
| description | string | yes | - | Tool description for LLM |
| command | string | yes | - | Executable path or name |

### `arg` Node (Positional Arguments)

Defines positional arguments passed to the command.

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| (name) | string | yes | - | Argument name (for schema) |
| description | string | no | - | Argument description |
| required | boolean | no | false | Whether argument is required |
| position | integer | no | auto | Position in argument list (0-indexed) |
| type | string | no | "string" | "string", "number", "boolean", "array" |
| default | any | no | - | Default value if not provided |
| enum | array | no | - | Allowed values |

### `flag` Node (Named Flags)

Defines command-line flags/options.

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| (name) | string | yes | - | Flag name (for schema) |
| short | string | no | - | Short form (e.g., "-r") |
| long | string | no | - | Long form (e.g., "--raw-output") |
| description | string | no | - | Flag description |
| type | string | no | "boolean" | "boolean", "string", "number", "array" |
| default | any | no | - | Default value |
| enum | array | no | - | Allowed values (for string/array) |
| separator | string | no | " " | For array: how to join values |
| repeat | boolean | no | false | For array: repeat flag per value |

### `stdin` Node

Defines standard input handling.

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| description | string | no | - | Description of expected input |
| type | string | no | "string" | Input type for schema |
| format | string | no | "text" | "json", "text", "binary" |
| required | boolean | no | false | Whether stdin is required |

### `stdout` Node

Defines standard output handling.

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| type | string | no | "string" | Output type |
| format | string | no | "auto" | "json", "text", "auto" |
| trim | boolean | no | true | Trim whitespace |
| encoding | string | no | "utf8" | "utf8", "base64" (for binary output) |

### `stderr` Node

Defines standard error handling.

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| capture | boolean | no | true | Include stderr in response |
| fail_on_output | boolean | no | false | Treat stderr output as failure |

### Execution Options

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| timeout | integer | no | 30000 | Timeout in milliseconds |
| workdir | string | no | "." | Working directory |
| env | map | no | {} | Environment variables |
| expand_env | boolean | no | false | Expand $VAR in env values |
| shell | boolean | no | false | Run through shell (security risk!) |
| allow_failure | boolean | no | false | Don't fail on non-zero exit |

**Security Note**: `shell: false` is the default and strongly recommended. When `shell: false`, all arguments are passed directly to the executable without shell interpretation, preventing command injection attacks.

## Generated MCP Schema

A CLI tool definition generates an MCP tool with this schema pattern:

```json
{
  "name": "cli_jq",
  "description": "Process JSON with jq filters",
  "inputSchema": {
    "type": "object",
    "properties": {
      "filter": {
        "type": "string",
        "description": "jq filter expression"
      },
      "file": {
        "type": "string",
        "description": "Input file (optional, uses stdin if omitted)"
      },
      "raw_output": {
        "type": "boolean",
        "description": "Output raw strings, not JSON"
      },
      "compact": {
        "type": "boolean",
        "description": "Compact output"
      },
      "stdin": {
        "type": "string",
        "description": "JSON input to process"
      }
    },
    "required": ["filter"]
  }
}
```

## Naming Conventions

- Tool names are prefixed with `cli_` to distinguish from MCP tools
- Flag names are converted: `raw-output` → `raw_output`
- Argument names use snake_case

## Example Definitions

### ripgrep (rg)

```kdl
cli "rg" {
    description "Search files for patterns using ripgrep"
    command "rg"

    arg "pattern" {
        description "Search pattern (regex)"
        required true
        position 0
    }

    arg "path" {
        description "File or directory to search"
        required false
        position 1
        default "."
    }

    flag "ignore-case" {
        short "-i"
        long "--ignore-case"
        description "Case insensitive search"
    }

    flag "word-regexp" {
        short "-w"
        long "--word-regexp"
        description "Match whole words only"
    }

    flag "context" {
        short "-C"
        long "--context"
        type "number"
        description "Lines of context around matches"
    }

    flag "glob" {
        short "-g"
        long "--glob"
        type "array"
        repeat true
        description "Include/exclude files matching glob"
    }

    flag "type" {
        short "-t"
        long "--type"
        type "array"
        repeat true
        description "File types to search"
        enum "js" "ts" "py" "go" "rs" "json" "md" "yaml"
    }

    flag "json" {
        long "--json"
        description "Output in JSON format"
    }

    stdout {
        format "auto"
    }

    timeout 60000
}
```

### curl

```kdl
cli "curl" {
    description "Make HTTP requests"
    command "curl"

    arg "url" {
        description "URL to request"
        required true
        position 0
    }

    flag "method" {
        short "-X"
        long "--request"
        type "string"
        description "HTTP method"
        enum "GET" "POST" "PUT" "DELETE" "PATCH" "HEAD"
        default "GET"
    }

    flag "header" {
        short "-H"
        long "--header"
        type "array"
        repeat true
        description "HTTP headers"
    }

    flag "data" {
        short "-d"
        long "--data"
        type "string"
        description "Request body"
    }

    flag "output" {
        short "-o"
        long "--output"
        type "string"
        description "Write output to file"
    }

    flag "silent" {
        short "-s"
        long "--silent"
        description "Silent mode"
        default true
    }

    flag "show-error" {
        short "-S"
        long "--show-error"
        description "Show errors even in silent mode"
        default true
    }

    flag "location" {
        short "-L"
        long "--location"
        description "Follow redirects"
        default true
    }

    timeout 120000
}
```

### sqlite3

```kdl
cli "sqlite3" {
    description "SQLite database CLI"
    command "sqlite3"

    arg "database" {
        description "Database file path"
        required true
        position 0
    }

    arg "sql" {
        description "SQL query to execute"
        required false
        position 1
    }

    flag "header" {
        long "-header"
        description "Show column headers"
    }

    flag "json" {
        long "-json"
        description "Output as JSON"
    }

    flag "csv" {
        long "-csv"
        description "Output as CSV"
    }

    flag "readonly" {
        long "-readonly"
        description "Open database in read-only mode"
    }

    stdin {
        description "SQL commands to execute"
        type "string"
    }

    stdout {
        format "auto"
    }

    timeout 60000
}
```

## Security Considerations

### Sandboxing

CLI tools run with these default restrictions:
- No network access (unless explicitly allowed)
- Limited filesystem access (current directory + temp)
- Resource limits (CPU, memory, file descriptors)

Override with:

```kdl
cli "tool" {
    // ...
    sandbox {
        network true       // Allow network access
        filesystem "full"  // "none", "cwd", "home", "full"
        resources {
            cpu_seconds 60
            memory_mb 512
            open_files 100
        }
    }
}
```

### Path Resolution

Commands are resolved:
1. Absolute path (if specified)
2. PATH environment variable
3. Well-known locations for common tools

Commands NOT in PATH must be specified with absolute path.

## Implementation Notes

### Transport Type

CLI tools use a virtual "cli" transport type in slop-mcp. They appear alongside regular MCP servers but:
- Don't maintain persistent connections
- Execute per-call
- Don't support resources or prompts

### Discovery

CLI tools are discovered at startup:
1. Scan configured directories for `*.kdl` files
2. Parse and validate definitions
3. Generate MCP tool schemas
4. Register with the tool index

### Execution Flow

1. Receive `execute_tool` call with `cli_<name>` tool
2. Map MCP parameters to CLI arguments
3. Build command line
4. Execute with resource limits
5. Capture stdout/stderr
6. Return formatted response

## Version History

- v1.0.0: Initial specification
