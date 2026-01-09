---
sidebar_position: 2
---

# Quick Start

Get SLOP MCP running with your first MCPs in under 5 minutes.

## Step 1: Add Your First MCP

Let's add the `everything` MCP - a great test MCP with various tools:

```bash
slop-mcp mcp add everything npx -y @anthropic/everything-mcp
```

This creates a `.slop-mcp.kdl` file in your current directory:

```kdl
mcp "everything" {
    transport "stdio"
    command "npx"
    args "-y" "@anthropic/everything-mcp"
}
```

## Step 2: Verify It's Connected

```bash
slop-mcp mcp list
```

Output:
```
MCPs:
  everything (stdio) - connected, 12 tools
```

## Step 3: Use It in Claude Code

With the SLOP MCP plugin active in Claude Code, you can now:

```
User: What tools are available for echoing?

Claude: Let me search for echo tools.

> search_tools query="echo"

Found 2 tools from everything:
- echo: Echo back the input
- echo_delayed: Echo back after a delay

User: Echo "Hello SLOP!"

Claude: > execute_tool mcp_name="everything" tool_name="echo"
         parameters={"message": "Hello SLOP!"}

Result: Hello SLOP!
```

## Step 4: Add More MCPs

Add as many as you need:

```bash
# Math calculations
slop-mcp mcp add math-mcp npx @anthropic/math-mcp

# File system access
slop-mcp mcp add filesystem npx @anthropic/filesystem-mcp /path/to/allowed/dir

# A streamable HTTP MCP
slop-mcp mcp add figma -t streamable https://mcp.figma.com/mcp

# Your own MCP
slop-mcp mcp add my-mcp python ./my_mcp_server.py
```

## Step 5: Authenticate (If Needed)

For MCPs that require OAuth:

```bash
slop-mcp mcp auth login figma
```

This opens your browser for authentication. Once complete, the MCP automatically reconnects with your new credentials.

## That's It!

You now have multiple MCPs connected through a single interface. Claude only sees 6 SLOP tools regardless of how many MCPs you add.

## Common Commands

```bash
# List all MCPs and their status
slop-mcp mcp list

# Remove an MCP
slop-mcp mcp remove math-mcp

# Check auth status
slop-mcp mcp auth status figma

# View full metadata
slop-mcp mcp metadata
```

## Next Steps

- [Configuration](/docs/getting-started/configuration) - Learn about scopes and KDL syntax
- [Context Efficiency](/docs/concepts/context-efficiency) - Understand how SLOP saves context
- [Examples](/docs/examples/math-calculations) - See real-world usage patterns
