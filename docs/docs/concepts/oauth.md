---
sidebar_position: 3
---

# OAuth Authentication

Many MCPs require authentication to access their services. SLOP MCP provides seamless OAuth support with automatic reconnection.

## Supported MCPs

OAuth works with any MCP that implements the MCP OAuth specification, including:

- **Figma** - Design file access
- **Linear** - Issue tracking
- **Dart** - Project management
- **Notion** - Workspace access
- **Slack** - Channel messaging
- **GitHub** (OAuth apps) - Repository access

## Quick Start

### Login

```bash
slop-mcp mcp auth login figma
```

This will:
1. Open your default browser
2. Redirect to the MCP's OAuth consent page
3. After you authorize, capture the token
4. **Automatically reconnect** the MCP with new credentials

### Check Status

```bash
slop-mcp mcp auth status figma
```

Output:
```
Figma authentication status:
  Authenticated: Yes
  Expires: 2024-01-15T10:30:00Z
  Has refresh token: Yes
```

### Logout

```bash
slop-mcp mcp auth logout figma
```

## Using in Claude Code

With the SLOP MCP plugin, authentication is available via the `auth_mcp` tool:

```
User: I need to access my Figma designs

Claude: Let me authenticate with Figma.

> auth_mcp action="login" name="figma"

Opening browser for OAuth authentication...
Successfully authenticated with figma - connection re-established with new credentials

Now I can access your Figma files. What would you like to do?
```

## Auto-Reconnection

**New in v0.3.0**: After successful authentication, SLOP MCP automatically:

1. Stores the OAuth token securely
2. Disconnects the existing MCP connection
3. Reconnects with the new credentials
4. Re-indexes available tools

No manual reconnection needed!

```
Before v0.3.0:
1. auth login figma
2. (manually) mcp remove figma
3. (manually) mcp add figma ...
4. Now it works

After v0.3.0:
1. auth login figma
2. It just works ✓
```

## Token Storage

OAuth tokens are stored securely in your system's credential store:

| Platform | Storage Location |
|----------|-----------------|
| macOS | Keychain |
| Linux | Secret Service (libsecret) |
| Windows | Credential Manager |

Fallback: `~/.config/slop-mcp/tokens.json` (encrypted)

## Token Refresh

SLOP MCP automatically refreshes tokens when they expire:

1. Before making an MCP call, check token expiry
2. If expired and refresh token exists, refresh automatically
3. If refresh fails, prompt for re-authentication

You don't need to manage token lifecycle manually.

## Configuration for OAuth MCPs

OAuth MCPs must use HTTP transport (streamable or SSE):

```kdl
mcp "figma" {
    transport "streamable"
    url "https://mcp.figma.com/mcp"
}

mcp "linear" {
    transport "sse"
    url "https://mcp.linear.app/sse"
}
```

:::warning
OAuth does not work with `stdio` transport MCPs. The MCP must expose an HTTP endpoint.
:::

## Troubleshooting

### "OAuth flow failed"

1. Check your internet connection
2. Verify the MCP URL is correct
3. Ensure the MCP supports OAuth

```bash
# Verify MCP is reachable
curl https://mcp.figma.com/.well-known/oauth-authorization-server
```

### "Token expired" errors

Force a refresh:

```bash
slop-mcp mcp auth login figma --force
```

### Browser doesn't open

Set your browser manually:

```bash
export BROWSER=/usr/bin/firefox
slop-mcp mcp auth login figma
```

Or use the URL directly:

```bash
slop-mcp mcp auth login figma --no-browser
# Prints URL to copy/paste
```

### "Connection failed after auth"

Check that the MCP is properly configured:

```bash
slop-mcp mcp list
# Should show: figma (streamable) - connected
```

If disconnected, try:

```bash
slop-mcp mcp remove figma
slop-mcp mcp add figma -t streamable https://mcp.figma.com/mcp
slop-mcp mcp auth login figma
```

## Security Best Practices

1. **Don't share tokens** - Tokens are personal credentials
2. **Use MCP-specific scopes** - Only authorize needed permissions
3. **Rotate periodically** - Logout and re-login monthly
4. **Audit access** - Check MCP provider's connected apps page

## API Reference

### auth_mcp Tool

```json
{
  "action": "login" | "logout" | "status" | "list",
  "name": "mcp-name"  // Required for login/logout/status
}
```

### CLI Commands

```bash
# Authenticate
slop-mcp mcp auth login <name>
slop-mcp mcp auth login <name> --force
slop-mcp mcp auth login <name> --no-browser

# Check status
slop-mcp mcp auth status <name>

# Remove token
slop-mcp mcp auth logout <name>

# List all authenticated MCPs
slop-mcp mcp auth list
```
