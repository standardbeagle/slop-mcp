---
sidebar_position: 1
---

# Installation

SLOP MCP can be installed via pip, npm, or as a standalone binary.

## Recommended: pip

```bash
pip install slop-mcp
```

This installs the `slop-mcp` binary globally.

## Alternative: npm/npx

```bash
# Run directly without installing
npx slop-mcp

# Or install globally
npm install -g slop-mcp
```

## Binary Download

Download pre-built binaries from the [GitHub Releases](https://github.com/standardbeagle/slop-mcp/releases) page:

| Platform | Architecture | Download |
|----------|-------------|----------|
| Linux | x64 | `slop-mcp-linux-amd64` |
| Linux | ARM64 | `slop-mcp-linux-arm64` |
| macOS | x64 (Intel) | `slop-mcp-darwin-amd64` |
| macOS | ARM64 (Apple Silicon) | `slop-mcp-darwin-arm64` |
| Windows | x64 | `slop-mcp-windows-amd64.exe` |

```bash
# Example: Linux x64
curl -L https://github.com/standardbeagle/slop-mcp/releases/latest/download/slop-mcp-linux-amd64 -o slop-mcp
chmod +x slop-mcp
sudo mv slop-mcp /usr/local/bin/
```

## Build from Source

```bash
git clone https://github.com/standardbeagle/slop-mcp.git
cd slop-mcp
make build
# Binary at ./build/slop-mcp
```

## Verify Installation

```bash
slop-mcp --version
# slop-mcp v0.3.0
```

## Claude Code Integration

To use SLOP MCP with Claude Code, add it to your MCP configuration:

### Using the Marketplace (Recommended)

Install the `slop-mcp` plugin from the [standardbeagle-tools marketplace](https://github.com/standardbeagle/standardbeagle-tools):

```bash
claude mcp install standardbeagle/slop-mcp
```

### Manual Configuration

Add to your Claude Code settings (`~/.claude/settings.json`):

```json
{
  "mcpServers": {
    "slop-mcp": {
      "command": "slop-mcp",
      "args": ["serve"]
    }
  }
}
```

Or if using npx:

```json
{
  "mcpServers": {
    "slop-mcp": {
      "command": "npx",
      "args": ["slop-mcp", "serve"]
    }
  }
}
```

## Next Steps

- [Quick Start Guide](/docs/getting-started/quick-start) - Add your first MCP
- [Configuration](/docs/getting-started/configuration) - Learn about KDL config files
