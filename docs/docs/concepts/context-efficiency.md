---
sidebar_position: 1
---

# Context Efficiency

SLOP MCP's killer feature: **connect unlimited MCPs without burning your context window**.

## The Context Problem

Every MCP tool needs a schema. A typical tool schema looks like this:

```json
{
  "name": "create_file",
  "description": "Create a new file with the specified content",
  "inputSchema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "The path where the file should be created"
      },
      "content": {
        "type": "string",
        "description": "The content to write to the file"
      }
    },
    "required": ["path", "content"]
  }
}
```

That's ~50-100 tokens per tool. With traditional MCP setups:

| MCPs | Tools/MCP | Total Tools | Context Tokens |
|------|-----------|-------------|----------------|
| 3 | 10 | 30 | ~2,000 |
| 5 | 15 | 75 | ~5,000 |
| 10 | 20 | 200 | ~15,000 |
| 17 | 25 | 425 | **~30,000** |

That's 30,000 tokens gone before you've said "hello".

## The SLOP Solution

SLOP MCP exposes exactly **6 meta-tools**, regardless of how many MCPs you connect:

```
┌────────────────────────────────────────────┐
│ SLOP MCP Meta-Tools (~400 tokens total)    │
├────────────────────────────────────────────┤
│ 1. search_tools    - Find tools by query   │
│ 2. execute_tool    - Run any MCP tool      │
│ 3. manage_mcps     - Add/remove MCPs       │
│ 4. auth_mcp        - OAuth authentication  │
│ 5. get_metadata    - Fetch MCP details     │
│ 6. run_slop        - Execute SLOP scripts  │
└────────────────────────────────────────────┘
```

**400 tokens. Always. Whether you have 3 MCPs or 300.**

## On-Demand Tool Loading

Here's the magic: tool schemas are loaded **only when you search for them**.

### Without SLOP (Eager Loading)

```
Conversation Start:
├── Load filesystem MCP (20 tools) → 1,500 tokens
├── Load math MCP (15 tools) → 1,000 tokens
├── Load database MCP (30 tools) → 2,500 tokens
└── Total overhead: 5,000 tokens

User: "What's 2 + 2?"
Claude: Uses math.calculate (but already loaded everything)
```

### With SLOP (Lazy Loading)

```
Conversation Start:
└── Load SLOP meta-tools → 400 tokens

User: "What's 2 + 2?"
Claude: search_tools query="calculate add"
├── Returns: math.calculate schema → 75 tokens
└── Total loaded: 475 tokens

Claude: execute_tool mcp="math" tool="calculate" ...
Result: 4
```

**Savings: 4,525 tokens** on a simple calculation.

## Real-World Impact

Consider a complex project setup:

```kdl
// .slop-mcp.kdl with 17 MCPs
mcp "filesystem" { ... }    // 20 tools
mcp "git" { ... }           // 15 tools
mcp "database" { ... }      // 25 tools
mcp "math" { ... }          // 10 tools
mcp "figma" { ... }         // 30 tools
mcp "linear" { ... }        // 20 tools
mcp "slack" { ... }         // 15 tools
mcp "github" { ... }        // 40 tools
mcp "aws" { ... }           // 50 tools
mcp "docker" { ... }        // 20 tools
mcp "kubernetes" { ... }    // 35 tools
mcp "terraform" { ... }     // 25 tools
mcp "datadog" { ... }       // 15 tools
mcp "pagerduty" { ... }     // 10 tools
mcp "jira" { ... }          // 25 tools
mcp "confluence" { ... }    // 20 tools
mcp "notion" { ... }        // 18 tools
// Total: 393 tools
```

| Approach | Context Overhead |
|----------|-----------------|
| Traditional | ~30,000 tokens |
| SLOP MCP | ~400 tokens |
| **Savings** | **29,600 tokens (98.7%)** |

## Search Efficiency

The `search_tools` command is designed for minimal context impact:

```bash
# Narrow search - 2-3 tool schemas returned
search_tools query="calculate percentage"

# Broader search - still only matching tools
search_tools query="file"  # Returns ~5-10 tools, not all 393
```

### Search Tips

1. **Be specific**: "calculate tax" beats "math"
2. **Use MCP filter**: `search_tools mcp_name="math" query="*"`
3. **Combine terms**: "file upload s3" narrows to exact matches

## Skills: Zero Context Overhead

Skills take this further by **completely removing tool schemas from context**:

```bash
# Traditional: tool schema in context for every call
execute_tool mcp="math" tool="calculate" params={...}

# Skill: just the invocation, SLOP handles the rest
/calculate expression="100 * 1.15"
```

The skill definition lives in a file, not your context:

```kdl
// skills/calculate.kdl
skill "calculate" {
    description "Evaluate mathematical expressions"
    mcp "math"
    tool "calculate"
    params {
        expression "$expression"
    }
}
```

See [Skills](/docs/concepts/skills) for more.

## Measuring Your Savings

SLOP tracks context efficiency:

```bash
# In Claude Code
get_metadata

# Response includes:
{
  "total_mcps": 17,
  "total_tools": 393,
  "slop_overhead_tokens": 400,
  "traditional_overhead_tokens": 29600,
  "savings_percentage": 98.7
}
```

## Best Practices

### 1. Add MCPs Freely

Don't hesitate to add MCPs. The marginal context cost is zero.

### 2. Use Specific Searches

```bash
# Good: specific
search_tools query="create github issue"

# Less good: vague
search_tools query="create"
```

### 3. Leverage Skills for Repeated Tasks

If you call the same MCP tool often, make it a skill.

### 4. Trust the Lazy Loading

Claude will search when needed. You don't have to preload anything.

## Summary

| Feature | Traditional MCP | SLOP MCP |
|---------|----------------|----------|
| 17 MCPs overhead | ~30,000 tokens | ~400 tokens |
| Adding new MCP | +2,000 tokens | +0 tokens |
| Tool loading | Eager (all upfront) | Lazy (on search) |
| Skills overhead | N/A | Zero |

**Install 17 MCPs. Claude won't even notice.**
