---
sidebar_position: 2
---

# Tools Reference

SLOP MCP exposes 6 meta-tools that provide access to all connected MCPs.

## search_tools

Search for tools across all connected MCPs.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query` | string | No | Search query (fuzzy matched) |
| `mcp_name` | string | No | Filter to specific MCP |

### Response

Returns matching tools with their schemas:

```json
{
  "tools": [
    {
      "name": "calculate",
      "description": "Evaluate mathematical expressions",
      "mcp_name": "math-mcp",
      "input_schema": { ... }
    }
  ],
  "total": 1
}
```

### Examples

```bash
# Search all MCPs
search_tools query="file upload"

# Search specific MCP
search_tools mcp_name="github" query="issue"

# List all tools from an MCP
search_tools mcp_name="math-mcp"
```

---

## execute_tool

Execute a tool on a specific MCP.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `mcp_name` | string | Yes | Target MCP name |
| `tool_name` | string | Yes | Tool to execute |
| `parameters` | object | No | Tool parameters |

### Response

Returns the tool's response:

```json
{
  "content": [
    {
      "type": "text",
      "text": "Result: 42"
    }
  ]
}
```

### Examples

```bash
# Simple execution
execute_tool mcp_name="math-mcp" tool_name="calculate" \
  parameters={"expression": "2 + 2"}

# With complex parameters
execute_tool mcp_name="github" tool_name="create_issue" \
  parameters={
    "repo": "owner/repo",
    "title": "Bug report",
    "body": "Description here",
    "labels": ["bug", "priority-high"]
  }
```

---

## manage_mcps

Manage MCP server connections.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `action` | string | Yes | Action: register, unregister, list, status |
| `name` | string | Conditional | MCP name (required for register/unregister) |
| `type` | string | No | Transport type (for register) |
| `command` | string | No | Command (for stdio) |
| `args` | array | No | Command arguments |
| `url` | string | No | URL (for http/sse/streamable) |
| `env` | object | No | Environment variables |
| `headers` | object | No | HTTP headers |

### Actions

#### register

Register a new MCP:

```bash
manage_mcps action="register" name="my-mcp" \
  type="stdio" command="npx" args=["my-mcp-package"]

manage_mcps action="register" name="figma" \
  type="streamable" url="https://mcp.figma.com/mcp"
```

#### unregister

Remove an MCP:

```bash
manage_mcps action="unregister" name="my-mcp"
```

#### list

List all MCPs:

```bash
manage_mcps action="list"
```

Response:

```json
{
  "mcps": [
    {
      "name": "math-mcp",
      "type": "stdio",
      "connected": true,
      "tool_count": 8
    }
  ]
}
```

#### status

Get detailed status:

```bash
manage_mcps action="status" name="figma"
```

---

## auth_mcp

Manage OAuth authentication for MCPs.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `action` | string | Yes | Action: login, logout, status, list |
| `name` | string | Conditional | MCP name (required for login/logout/status) |

### Actions

#### login

Initiate OAuth flow:

```bash
auth_mcp action="login" name="figma"
```

Opens browser for authentication. After completion:

```json
{
  "message": "Successfully authenticated with figma - connection re-established with new credentials",
  "status": {
    "server_name": "figma",
    "is_authenticated": true,
    "expires_at": "2024-01-15T10:30:00Z",
    "has_refresh_token": true
  }
}
```

#### logout

Remove authentication:

```bash
auth_mcp action="logout" name="figma"
```

#### status

Check auth status:

```bash
auth_mcp action="status" name="figma"
```

#### list

List authenticated MCPs:

```bash
auth_mcp action="list"
```

---

## get_metadata

Get full metadata for connected MCPs.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `mcp_name` | string | No | Filter to specific MCP |
| `file_path` | string | No | Write output to file |

### Response

Returns comprehensive metadata:

```json
{
  "metadata": [
    {
      "name": "math-mcp",
      "type": "stdio",
      "state": "connected",
      "tools": [
        {
          "name": "calculate",
          "description": "...",
          "input_schema": { ... }
        }
      ],
      "prompts": [],
      "resources": [],
      "resource_templates": []
    }
  ]
}
```

### Examples

```bash
# Get all metadata
get_metadata

# Get specific MCP
get_metadata mcp_name="github"

# Write to file
get_metadata file_path="metadata.json"
```

---

## run_slop

Execute a SLOP script.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `script` | string | Conditional | Inline SLOP script |
| `file_path` | string | Conditional | Path to .slop file |

One of `script` or `file_path` is required.

### Examples

#### Inline Script

```bash
run_slop script='
@call math-mcp.calculate {
  expression: "100 * 1.15"
}
'
```

#### File Script

```bash
run_slop file_path="scripts/deploy.slop"
```

### SLOP Script Syntax

```slop
// Variables
@let tax_rate = 0.08

// MCP calls
@call math-mcp.calculate {
  expression: "100 * (1 + $tax_rate)"
}

// Conditionals
@if result > 100 {
  @call slack.post_message {
    channel: "#alerts"
    text: "High value: $result"
  }
}

// Loops
@each item in $items {
  @call processor.process {
    data: $item
  }
}
```

---

## Error Handling

All tools return errors in a consistent format:

```json
{
  "error": {
    "code": "MCP_NOT_FOUND",
    "message": "MCP 'unknown' not found",
    "details": {
      "available_mcps": ["math-mcp", "github", "figma"]
    }
  }
}
```

### Error Codes

| Code | Description |
|------|-------------|
| `MCP_NOT_FOUND` | MCP not registered |
| `TOOL_NOT_FOUND` | Tool not found in MCP |
| `AUTH_REQUIRED` | MCP requires authentication |
| `AUTH_EXPIRED` | Token expired, re-auth needed |
| `CONNECTION_FAILED` | Failed to connect to MCP |
| `EXECUTION_FAILED` | Tool execution failed |
| `INVALID_PARAMS` | Invalid parameters provided |
