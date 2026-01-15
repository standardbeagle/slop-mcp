# Memory CLI Tool Specification

## Overview

`memory-cli` is a standalone CLI tool that provides structured file-based memory banks for AI agent sessions. It enables persistent key-value storage that survives across sessions, with support for querying, searching, and metadata tracking.

## Design Goals

1. **Simple API**: Basic CRUD operations with intuitive commands
2. **Structured Data**: JSON storage with optional schema validation
3. **Scoped Storage**: User-level and project-level memory banks
4. **Query Support**: jq-compatible filtering for complex queries
5. **Lock-free**: Atomic file operations, no external dependencies
6. **Fast**: Minimal overhead, suitable for frequent agent calls

## Use Cases

### Agent Session Memory
```bash
# Store conversation context
memory-cli write session context '{"topic": "refactoring", "files": ["main.go"]}'

# Retrieve context in new session
memory-cli read session context
```

### Project Notes
```bash
# Add project-specific knowledge
memory-cli write project decisions '{"auth": "JWT", "db": "PostgreSQL"}'

# Query specific decisions
memory-cli query project decisions '.auth'
```

### Cross-Session Task Tracking
```bash
# Track recurring task state
memory-cli write tasks daily-standup '{"last_run": "2024-01-15", "items": []}'

# Update with new items
memory-cli update tasks daily-standup '.items += ["Review PRs"]'
```

## Storage Architecture

### File Locations

| Scope | Path | Purpose |
|-------|------|---------|
| User | `~/.config/slop-mcp/memory/<bank>.json` | Cross-project memories |
| Project | `.slop-mcp/memory/<bank>.json` | Project-specific memories |

### File Format

Each bank is a JSON file with this structure:

```json
{
  "_meta": {
    "version": 1,
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T14:45:00Z",
    "schema": null
  },
  "entries": {
    "context": {
      "value": {"topic": "refactoring", "files": ["main.go"]},
      "created_at": "2024-01-15T10:30:00Z",
      "updated_at": "2024-01-15T14:45:00Z",
      "ttl": null
    }
  }
}
```

### Bank Naming

- Names: lowercase alphanumeric with hyphens/underscores
- Pattern: `^[a-z][a-z0-9_-]*$`
- Max length: 64 characters
- Reserved: `_system`, `_meta`, `_index`

## CLI Interface

### Commands

#### `read` - Read memory entries

```bash
memory-cli read <bank> [key] [flags]

# Read entire bank
memory-cli read session

# Read specific key
memory-cli read session context

# Read with default value
memory-cli read session missing --default '{"empty": true}'
```

**Flags:**
- `--scope <user|project>`: Storage scope (default: auto-detect)
- `--default <json>`: Default value if key doesn't exist
- `--format <json|value>`: Output format (default: json)

**Output:**
```json
{
  "value": {"topic": "refactoring"},
  "metadata": {
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T14:45:00Z"
  }
}
```

#### `write` - Write memory entries

```bash
memory-cli write <bank> <key> <value> [flags]

# Write JSON value
memory-cli write session context '{"topic": "testing"}'

# Write from stdin
echo '{"data": "value"}' | memory-cli write session data --stdin

# Write with TTL (auto-expire)
memory-cli write cache token '{"jwt": "..."}' --ttl 3600
```

**Flags:**
- `--scope <user|project>`: Storage scope (default: project)
- `--stdin`: Read value from stdin
- `--ttl <seconds>`: Time-to-live for auto-expiration
- `--create-only`: Fail if key already exists
- `--update-only`: Fail if key doesn't exist

**Output:**
```json
{
  "success": true,
  "key": "context",
  "bank": "session",
  "operation": "create"
}
```

#### `update` - Update existing entries with jq

```bash
memory-cli update <bank> <key> <jq-filter> [flags]

# Append to array
memory-cli update tasks items '.items += ["new task"]'

# Increment counter
memory-cli update stats visits '.count += 1'

# Set nested field
memory-cli update config settings '.theme = "dark"'
```

**Flags:**
- `--scope <user|project>`: Storage scope
- `--create`: Create entry if doesn't exist (with null as initial)

#### `delete` - Delete entries or banks

```bash
memory-cli delete <bank> [key] [flags]

# Delete specific key
memory-cli delete session context

# Delete entire bank
memory-cli delete session --bank

# Delete with confirmation
memory-cli delete session --bank --confirm
```

**Flags:**
- `--scope <user|project>`: Storage scope
- `--bank`: Delete entire bank, not just key
- `--confirm`: Skip confirmation prompt

#### `list` - List banks or keys

```bash
memory-cli list [bank] [flags]

# List all banks
memory-cli list

# List keys in bank
memory-cli list session

# List with metadata
memory-cli list session --verbose
```

**Flags:**
- `--scope <user|project|all>`: Which scope(s) to list
- `--verbose`: Include metadata (timestamps, sizes)
- `--format <json|table>`: Output format

**Output (table):**
```
BANK      KEYS  UPDATED              SCOPE
session   3     2024-01-15 14:45:00  project
config    5     2024-01-14 09:00:00  user
cache     12    2024-01-15 15:00:00  project
```

**Output (json):**
```json
{
  "banks": [
    {
      "name": "session",
      "scope": "project",
      "key_count": 3,
      "updated_at": "2024-01-15T14:45:00Z",
      "size_bytes": 1234
    }
  ]
}
```

#### `query` - Query with jq filters

```bash
memory-cli query <bank> [key] <filter> [flags]

# Query specific key
memory-cli query session context '.topic'

# Query entire bank
memory-cli query session '.entries | keys'

# Complex filter
memory-cli query tasks items '.items[] | select(.done == false)'
```

**Flags:**
- `--scope <user|project>`: Storage scope
- `--raw`: Output raw strings (like jq -r)

#### `search` - Search across banks

```bash
memory-cli search <pattern> [flags]

# Search by key name
memory-cli search "context"

# Search in values (regex)
memory-cli search --value "refactor.*main"

# Search with scope filter
memory-cli search "config" --scope user
```

**Flags:**
- `--scope <user|project|all>`: Scope to search
- `--value`: Search in values, not just keys
- `--format <json|table>`: Output format

#### `export` / `import` - Backup and restore

```bash
# Export bank to file
memory-cli export session > session-backup.json

# Import bank from file
memory-cli import session < session-backup.json

# Export all banks
memory-cli export --all > all-memories.json
```

#### `gc` - Garbage collection

```bash
# Remove expired entries (TTL)
memory-cli gc

# Remove entries older than 30 days
memory-cli gc --older-than 30d

# Dry run
memory-cli gc --dry-run
```

#### `repair` - Repair corrupted banks

```bash
# Check and repair all banks
memory-cli repair

# Repair specific bank
memory-cli repair session

# Check only (no modifications)
memory-cli repair --check-only
```

## Limits and Constraints

| Limit | Default | Description |
|-------|---------|-------------|
| Max value size | 1 MB | Maximum size of a single value |
| Max bank size | 10 MB | Maximum size of a bank file |
| Max key length | 256 chars | Maximum key name length |
| Max keys per bank | 10,000 | Maximum entries in one bank |

### Scope Auto-Detection

When `--scope` is not specified:
1. **For reads**: Check project scope first, then user scope
2. **For writes**: Use project scope if `.slop-mcp/` exists, else user scope
3. **For deletes**: Must match the scope where the entry exists

## MCP Tool Integration

When exposed through slop-mcp, memory-cli provides these tools:

### `memory_read`
```json
{
  "name": "memory_read",
  "description": "Read from a memory bank",
  "inputSchema": {
    "type": "object",
    "properties": {
      "bank": {"type": "string", "description": "Memory bank name"},
      "key": {"type": "string", "description": "Key to read (optional, reads all if omitted)"},
      "scope": {"type": "string", "enum": ["user", "project"]},
      "default": {"description": "Default value if key not found"}
    },
    "required": ["bank"]
  }
}
```

### `memory_write`
```json
{
  "name": "memory_write",
  "description": "Write to a memory bank",
  "inputSchema": {
    "type": "object",
    "properties": {
      "bank": {"type": "string"},
      "key": {"type": "string"},
      "value": {"description": "Value to store (any JSON)"},
      "scope": {"type": "string", "enum": ["user", "project"]},
      "ttl": {"type": "integer", "description": "TTL in seconds"}
    },
    "required": ["bank", "key", "value"]
  }
}
```

### `memory_update`
```json
{
  "name": "memory_update",
  "description": "Update memory entry with jq filter",
  "inputSchema": {
    "type": "object",
    "properties": {
      "bank": {"type": "string"},
      "key": {"type": "string"},
      "filter": {"type": "string", "description": "jq filter to apply"},
      "scope": {"type": "string", "enum": ["user", "project"]}
    },
    "required": ["bank", "key", "filter"]
  }
}
```

### `memory_delete`
```json
{
  "name": "memory_delete",
  "description": "Delete memory entry or bank",
  "inputSchema": {
    "type": "object",
    "properties": {
      "bank": {"type": "string"},
      "key": {"type": "string", "description": "Key to delete (omit to delete entire bank)"},
      "scope": {"type": "string", "enum": ["user", "project"]}
    },
    "required": ["bank"]
  }
}
```

### `memory_list`
```json
{
  "name": "memory_list",
  "description": "List memory banks or keys",
  "inputSchema": {
    "type": "object",
    "properties": {
      "bank": {"type": "string", "description": "Bank to list keys from (omit to list banks)"},
      "scope": {"type": "string", "enum": ["user", "project", "all"]}
    }
  }
}
```

### `memory_query`
```json
{
  "name": "memory_query",
  "description": "Query memory with jq filter",
  "inputSchema": {
    "type": "object",
    "properties": {
      "bank": {"type": "string"},
      "key": {"type": "string"},
      "filter": {"type": "string", "description": "jq filter expression"},
      "scope": {"type": "string", "enum": ["user", "project"]}
    },
    "required": ["bank", "filter"]
  }
}
```

### `memory_search`
```json
{
  "name": "memory_search",
  "description": "Search across memory banks",
  "inputSchema": {
    "type": "object",
    "properties": {
      "pattern": {"type": "string", "description": "Search pattern (regex)"},
      "search_values": {"type": "boolean", "description": "Search in values too"},
      "scope": {"type": "string", "enum": ["user", "project", "all"]}
    },
    "required": ["pattern"]
  }
}
```

## Implementation Details

### Concurrency

Memory-cli uses atomic file operations for thread safety:

1. **Read**: Direct file read (no locking needed)
2. **Write**: Write to temp file, then atomic rename
3. **Update**: Read → modify → write to temp → rename

For high-concurrency scenarios, use file-based CAS:
```go
// Pseudo-code for compare-and-swap
func update(bank, key string, fn func(old any) any) error {
    for retries := 0; retries < 3; retries++ {
        data, modTime := read(bank)
        newData := fn(data)
        if err := writeIfUnmodified(bank, newData, modTime); err == ErrModified {
            continue // Retry
        }
        return err
    }
    return ErrTooManyRetries
}
```

### Error Handling

All errors fail fast with structured JSON:

```json
{
  "error": {
    "code": "BANK_NOT_FOUND",
    "message": "Memory bank 'session' does not exist",
    "bank": "session",
    "scope": "project"
  }
}
```

Error codes:
- `BANK_NOT_FOUND`: Bank doesn't exist
- `KEY_NOT_FOUND`: Key doesn't exist in bank
- `INVALID_BANK_NAME`: Bank name doesn't match pattern
- `INVALID_JSON`: Value is not valid JSON
- `PERMISSION_DENIED`: Cannot write to scope
- `CONFLICT`: Concurrent modification (CAS failure)
- `TTL_EXPIRED`: Entry has expired

### Performance

- **Index file**: Optional `_index.json` for large banks
- **Lazy loading**: Only load requested keys for large banks
- **Compression**: Optional gzip for banks > 1MB

## CLI Tool Definition (KDL)

```kdl
cli "memory" {
    description "Structured file-based memory banks for agent sessions"
    command "memory-cli"

    arg "command" {
        description "Subcommand: read, write, update, delete, list, query, search"
        required true
        position 0
        enum "read" "write" "update" "delete" "list" "query" "search" "export" "import" "gc"
    }

    arg "bank" {
        description "Memory bank name"
        required false
        position 1
    }

    arg "key" {
        description "Key within bank"
        required false
        position 2
    }

    arg "value" {
        description "Value to write (JSON) or jq filter for query/update"
        required false
        position 3
    }

    flag "scope" {
        long "--scope"
        type "string"
        description "Storage scope"
        enum "user" "project" "all"
        default "project"
    }

    flag "default" {
        long "--default"
        type "string"
        description "Default value if key not found"
    }

    flag "ttl" {
        long "--ttl"
        type "number"
        description "Time-to-live in seconds"
    }

    flag "stdin" {
        long "--stdin"
        description "Read value from stdin"
    }

    flag "raw" {
        short "-r"
        long "--raw"
        description "Raw string output (no JSON encoding)"
    }

    flag "verbose" {
        short "-v"
        long "--verbose"
        description "Include metadata in output"
    }

    flag "format" {
        long "--format"
        type "string"
        description "Output format"
        enum "json" "table" "value"
        default "json"
    }

    stdin {
        description "Value to write (when --stdin is used)"
        format "json"
    }

    stdout {
        format "json"
    }

    timeout 30000
}
```

## Version History

- v1.0.0: Initial specification
