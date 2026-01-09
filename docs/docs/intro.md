---
sidebar_position: 1
slug: /intro
---

# Introduction to SLOP MCP

**Install 17 MCPs. Claude won't even notice.**

SLOP MCP is a Model Context Protocol (MCP) orchestration layer that lets you connect unlimited MCP servers to Claude without destroying your context window.

## The Problem

Traditional MCP setups have a fatal flaw: **every tool from every MCP floods your context**. Install 5 MCPs with 20 tools each, and you've just burned 100+ tool schemas before asking your first question.

```
Without SLOP MCP:
┌─────────────────────────────────────────────┐
│ Context Window (200k tokens)                │
├─────────────────────────────────────────────┤
│ 🔧 Tool schemas from MCP 1 (20 tools)       │
│ 🔧 Tool schemas from MCP 2 (15 tools)       │
│ 🔧 Tool schemas from MCP 3 (30 tools)       │
│ 🔧 Tool schemas from MCP 4 (25 tools)       │
│ 🔧 Tool schemas from MCP 5 (10 tools)       │
│ ─────────────────────────────────────────── │
│ 📝 Your actual conversation... (what's left)│
└─────────────────────────────────────────────┘
```

## The Solution

SLOP MCP exposes **just 6 meta-tools** regardless of how many MCPs you connect. Tool schemas are loaded on-demand when you search for them.

```
With SLOP MCP:
┌─────────────────────────────────────────────┐
│ Context Window (200k tokens)                │
├─────────────────────────────────────────────┤
│ 🔧 search_tools                             │
│ 🔧 execute_tool                             │
│ 🔧 manage_mcps                              │
│ 🔧 auth_mcp                                 │
│ 🔧 get_metadata                             │
│ 🔧 run_slop                                 │
│ ─────────────────────────────────────────── │
│ 📝 Your entire conversation fits here!      │
│                                             │
│    (Tools loaded only when you need them)   │
└─────────────────────────────────────────────┘
```

## Key Features

### 🚀 Unlimited MCPs, Constant Context

Connect 17 MCPs or 170. Your context overhead stays the same: 6 tool schemas.

### 🔍 On-Demand Tool Discovery

Use `search_tools` to find what you need. Only matching tools get loaded into context.

```bash
# Search across all MCPs
search_tools query="file upload"

# Found 3 tools from 2 MCPs
# Only these 3 schemas hit your context!
```

### ⚡ Skills = Zero Overhead

Create skills that call MCPs directly. The skill invocation is one line; SLOP handles the orchestration behind the scenes.

```bash
# In Claude Code
/calculate-tax income=50000 state=CA

# Skill calls math-mcp and tax-mcp internally
# No tool schemas in context at all!
```

### 🔐 OAuth That Actually Works

Authenticate to MCPs like Figma, Linear, or Dart with browser-based OAuth. Connection auto-reconnects after auth.

```bash
slop-mcp mcp auth login figma
# Browser opens, you authenticate
# MCP reconnects automatically with new credentials
```

### 📦 Flexible Configuration

Define MCPs at project, user, or local scope using KDL config files:

```kdl
mcp "math-mcp" {
    transport "stdio"
    command "npx"
    args "@anthropic/math-mcp"
}

mcp "figma" {
    transport "streamable"
    url "https://mcp.figma.com/mcp"
}
```

## Quick Example

Here's SLOP MCP in action with Claude Code:

```
User: Calculate 15% tip on $47.50

Claude: Let me search for calculation tools.

> search_tools query="calculate"
Found: math-mcp:calculate, math-mcp:evaluate

> execute_tool mcp_name="math-mcp" tool_name="calculate"
  parameters={"expression": "47.50 * 0.15"}
Result: 7.125

The 15% tip on $47.50 is $7.13.
```

**Only the `calculate` tool schema was loaded** - not the 50+ other tools from your connected MCPs.

## Ready to Start?

import Link from '@docusaurus/Link';

<div style={{display: 'flex', gap: '1rem', marginTop: '2rem'}}>
  <Link className="button button--primary button--lg" to="/docs/getting-started/installation">
    Install SLOP MCP
  </Link>
  <Link className="button button--secondary button--lg" to="/docs/examples/math-calculations">
    See Examples
  </Link>
</div>
