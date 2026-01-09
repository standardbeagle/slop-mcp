---
sidebar_position: 2
---

# Skills

Skills are the secret weapon for **zero-context MCP invocations**. Instead of tool schemas flooding your conversation, you invoke a skill with a single line.

## What Are Skills?

A skill is a pre-defined MCP operation that you can invoke by name. The skill definition (including which MCP, tool, and parameters) lives in a file - not your context window.

```
Without Skills:
┌─────────────────────────────────────────────┐
│ Context includes:                           │
│ - math.calculate schema (75 tokens)         │
│ - math.evaluate schema (80 tokens)          │
│ - Full parameter descriptions               │
└─────────────────────────────────────────────┘

With Skills:
┌─────────────────────────────────────────────┐
│ Context includes:                           │
│ - /calculate skill name (5 tokens)          │
│ That's it.                                  │
└─────────────────────────────────────────────┘
```

## Using Skills in Claude Code

With the SLOP MCP plugin from [standardbeagle-tools](https://github.com/standardbeagle/standardbeagle-tools), skills are available as slash commands:

```
User: Calculate the tax on $500 at 8.5%

Claude: /calculate expression="500 * 0.085"

Result: 42.5

The tax would be $42.50.
```

**No tool schema loaded. No search performed. Just direct invocation.**

## Creating Skills

### Quick Skills via CLI

```bash
slop-mcp skill add calculate \
  --mcp math \
  --tool calculate \
  --param "expression:The math expression to evaluate"
```

### Skills in Config Files

Create a `skills/` directory with KDL skill definitions:

```kdl
// skills/calculate.kdl
skill "calculate" {
    description "Evaluate mathematical expressions"
    mcp "math"
    tool "calculate"
    params {
        expression {
            description "The mathematical expression"
            required true
        }
    }
}
```

```kdl
// skills/create-issue.kdl
skill "create-issue" {
    description "Create a GitHub issue"
    mcp "github"
    tool "create_issue"
    params {
        repo {
            description "Repository (owner/name)"
            required true
        }
        title {
            description "Issue title"
            required true
        }
        body {
            description "Issue body"
            required false
        }
    }
}
```

## Multi-Step Skills

Skills can chain multiple MCP calls:

```kdl
// skills/deploy-report.kdl
skill "deploy-report" {
    description "Build, deploy, and notify"

    steps {
        step "build" {
            mcp "docker"
            tool "build"
            params {
                dockerfile "./Dockerfile"
                tag "$version"
            }
        }

        step "deploy" {
            mcp "kubernetes"
            tool "apply"
            params {
                manifest "./k8s/deployment.yaml"
            }
        }

        step "notify" {
            mcp "slack"
            tool "post_message"
            params {
                channel "#deployments"
                text "Deployed version $version"
            }
        }
    }
}
```

Invoke with:
```
/deploy-report version="v1.2.3"
```

## Skills from the Marketplace

The [standardbeagle-tools marketplace](https://github.com/standardbeagle/standardbeagle-tools) provides pre-built skills:

```bash
# Install the SLOP MCP plugin with skills
claude plugin install standardbeagle/slop-mcp

# Available skills include:
# /calculate - Math expressions
# /search-code - Code search across repos
# /create-pr - GitHub PR creation
# /deploy - Kubernetes deployment
# /analyze-image - Image analysis via vision MCPs
```

## Skills vs Direct Tool Calls

| Aspect | Direct Tool Call | Skill |
|--------|-----------------|-------|
| Context overhead | ~75-100 tokens/tool | ~5 tokens |
| Schema in context | Yes | No |
| Learning curve | Read full schema | Simple arguments |
| Reusability | Copy/paste params | Name + args |
| Multi-step | Manual chaining | Built-in |

## Best Practices

### 1. Create Skills for Frequent Operations

If you call `execute_tool mcp="X" tool="Y"` more than twice, make it a skill.

### 2. Use Descriptive Names

```kdl
// Good
skill "calculate-shipping-cost" { ... }

// Less good
skill "calc" { ... }
```

### 3. Document Parameters

```kdl
skill "resize-image" {
    params {
        path {
            description "Path to the image file"
            required true
        }
        width {
            description "Target width in pixels (maintains aspect ratio if height omitted)"
            required true
        }
        height {
            description "Target height in pixels (optional)"
            required false
        }
    }
}
```

### 4. Set Sensible Defaults

```kdl
skill "search-logs" {
    mcp "datadog"
    tool "search"
    params {
        query { required true }
        timeframe {
            default "1h"
            description "Time window: 15m, 1h, 24h, 7d"
        }
        limit {
            default 100
        }
    }
}
```

## Skill Discovery

List available skills:

```bash
slop-mcp skill list
```

Or in Claude Code:
```
User: What skills are available?

Claude: Let me check the available SLOP skills.

Available skills:
- /calculate - Evaluate math expressions
- /create-issue - Create GitHub issue
- /search-code - Search across repositories
- /deploy - Deploy to Kubernetes
```

## Summary

Skills transform verbose MCP operations into simple slash commands:

- **Zero context overhead** - No tool schemas loaded
- **Reusable** - Define once, use everywhere
- **Chainable** - Multi-step workflows in one command
- **Discoverable** - `/help` lists all skills

Combined with SLOP MCP's lazy tool loading, skills make it possible to have **hundreds of MCP capabilities** available without any context penalty.
