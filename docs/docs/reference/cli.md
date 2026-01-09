---
sidebar_position: 1
---

# CLI Reference

Complete reference for the `slop-mcp` command-line interface.

## Global Options

```bash
slop-mcp [command] [options]

Options:
  --version, -v    Show version
  --help, -h       Show help
```

## Commands

### serve

Start the SLOP MCP server (for use with Claude Code):

```bash
slop-mcp serve [options]

Options:
  --config, -c     Path to config file
  --port, -p       Port for HTTP transport (default: stdio)
```

### mcp

Manage MCP servers.

#### mcp add

Add an MCP to configuration:

```bash
slop-mcp mcp add <name> <command> [args...]

Options:
  -l, --local      Add to local config (.slop-mcp.local.kdl)
  -p, --project    Add to project config (.slop-mcp.kdl) [default]
  -u, --user       Add to user config (~/.config/slop-mcp/config.kdl)
  -s, --scope      Set scope: local, project, or user

  -t, --transport  Transport type: stdio (default), sse, http, streamable
  --url            URL for HTTP transports
  --env            Environment variables (KEY=VALUE)

Examples:
  # Add stdio MCP
  slop-mcp mcp add math-mcp npx @anthropic/math-mcp

  # Add HTTP MCP
  slop-mcp mcp add figma -t streamable https://mcp.figma.com/mcp

  # Add to user config
  slop-mcp mcp add -u my-global npx my-global-mcp

  # With environment variables
  slop-mcp mcp add my-mcp node server.js --env API_KEY=secret
```

#### mcp add-json

Add an MCP from a JSON configuration:

```bash
slop-mcp mcp add-json <name> '<json>'

Options:
  -l, --local      Add to local config
  -p, --project    Add to project config [default]
  -u, --user       Add to user config

Example:
  slop-mcp mcp add-json my-mcp '{"command": "npx", "args": ["my-mcp"]}'
```

#### mcp remove

Remove an MCP from configuration:

```bash
slop-mcp mcp remove <name>

Options:
  -l, --local      Remove from local config
  -p, --project    Remove from project config [default]
  -u, --user       Remove from user config
  -a, --all        Remove from all configs
```

#### mcp list

List all configured MCPs:

```bash
slop-mcp mcp list [options]

Options:
  --json           Output as JSON
  --verbose        Show full configuration

Output:
  MCPs:
    math-mcp (stdio) from project - connected, 8 tools
    figma (streamable) from user - needs auth
    my-local (stdio) from local - connected, 3 tools
```

#### mcp metadata

Show full metadata for all MCPs:

```bash
slop-mcp mcp metadata [options]

Options:
  --mcp <name>     Filter to specific MCP
  --json           Output as JSON
  -o, --output     Write to file

Example:
  slop-mcp mcp metadata --mcp figma --json > figma-tools.json
```

### mcp auth

Manage OAuth authentication.

#### mcp auth login

Authenticate with an MCP:

```bash
slop-mcp mcp auth login <name> [options]

Options:
  --force          Force re-authentication
  --no-browser     Print URL instead of opening browser

Example:
  slop-mcp mcp auth login figma
  # Opens browser, completes OAuth, reconnects automatically
```

#### mcp auth logout

Remove authentication:

```bash
slop-mcp mcp auth logout <name>
```

#### mcp auth status

Check authentication status:

```bash
slop-mcp mcp auth status <name>

Output:
  figma authentication status:
    Authenticated: Yes
    Expires: 2024-01-15T10:30:00Z
    Has refresh token: Yes
```

#### mcp auth list

List all authenticated MCPs:

```bash
slop-mcp mcp auth list

Output:
  Authenticated MCPs:
    figma - expires 2024-01-15
    linear - expires 2024-01-20
    dart - expires 2024-01-18
```

### skill

Manage skills.

#### skill add

Create a new skill:

```bash
slop-mcp skill add <name> [options]

Options:
  --mcp            Target MCP
  --tool           Tool to invoke
  --param          Parameter definition (can repeat)
  --description    Skill description

Example:
  slop-mcp skill add calculate \
    --mcp math-mcp \
    --tool calculate \
    --param "expression:Math expression to evaluate" \
    --description "Evaluate mathematical expressions"
```

#### skill list

List available skills:

```bash
slop-mcp skill list [options]

Options:
  --json           Output as JSON
```

#### skill remove

Remove a skill:

```bash
slop-mcp skill remove <name>
```

### run

Execute a SLOP script:

```bash
slop-mcp run <script> [params...]

Options:
  --dry-run        Show what would be executed
  --verbose        Show detailed output

Example:
  slop-mcp run scripts/deploy.slop VERSION=1.2.3
```

### version

Show version information:

```bash
slop-mcp version

Output:
  slop-mcp v0.3.0
  Built with OAuth support
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `SLOP_CONFIG` | Path to config file |
| `SLOP_LOG_LEVEL` | Log level (debug, info, warn, error) |
| `BROWSER` | Browser to use for OAuth |
| `NO_COLOR` | Disable colored output |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Configuration error |
| 3 | Authentication error |
| 4 | MCP connection error |

## Configuration Files

SLOP MCP looks for configuration in this order:

1. `.slop-mcp.local.kdl` (local, gitignored)
2. `.slop-mcp.kdl` (project)
3. `~/.config/slop-mcp/config.kdl` (user)

All configs are merged, with local taking precedence.
