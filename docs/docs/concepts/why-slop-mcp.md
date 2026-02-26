---
sidebar_position: 0
---

# Why SLOP MCP

## The Problem

When AI agents work with multiple MCP servers, two fundamental issues emerge:

1. **Context window overload** — every tool from every MCP loads its schema upfront. Install 10 MCPs with 20 tools each and you've burned 15,000+ tokens before the conversation starts.

2. **Intermediate result duplication** — when an agent calls tool A, gets a result, then passes it to tool B, the full result transits through the agent's context window. Chain 5 operations and you've duplicated data 5 times in context.

Anthropic's engineering team describes this problem and its solution in [Code Execution with MCP](https://www.anthropic.com/engineering/code-execution-with-mcp): instead of routing every intermediate value through the model, execute code *within* the MCP layer so data flows directly between tools.

## How slop-mcp Implements This

slop-mcp takes a complementary approach to the patterns described in the article:

### Progressive Discovery

Instead of loading all tool schemas upfront, agents discover tools on demand via `search_tools`. Only matching tools enter the context — the same lazy-loading pattern the article describes for filesystem exploration.

```
Traditional: 393 tool schemas loaded at startup → 30,000 tokens
SLOP MCP:    8 meta-tool schemas loaded         → 500 tokens
             + tools loaded only when searched
```

### Code Execution via SLOP

The `run_slop` tool implements *code execution within the MCP layer* — exactly what the Anthropic article proposes. A SLOP script can:

- Call multiple MCP tools in sequence
- Transform and filter intermediate results
- Loop over collections
- Persist data across sessions

All without intermediate results re-entering the agent's context:

```python
# 1 run_slop call replaces 6 agent round-trips
issues = github.list_issues(repo: "org/app", state: "open")
critical = issues | filter(|i| "critical" in i["labels"])
for issue in critical {
    linear.create_issue(title: issue["title"], priority: 1)
    slack.post_message(channel: "#incidents", text: issue["title"])
}
emit(synced: len(critical))
# Agent sees only: {"synced": 3}
```

### Intermediate Results Stay Out of Context

When a SLOP script runs, data flows between MCP tools inside the SLOP runtime. The agent only sees the final `emit()` output. A script that processes 50 GitHub issues, creates 10 Linear tickets, and sends 10 Slack messages returns a single summary object to the agent.

## Why a Purpose-Built Language?

The Anthropic article discusses embedding general-purpose languages (Python, TypeScript) for code execution. SLOP takes a different approach — a purpose-built language designed specifically for MCP orchestration:

| Concern | General-purpose language | SLOP |
|---|---|---|
| **Security** | Requires container/VM sandboxing | Sandboxed by design — no fs, network, or shell access |
| **MCP integration** | Needs SDK setup, imports, boilerplate | Native `mcp.tool(param: value)` syntax |
| **Data pipelines** | Verbose chaining with variables | `\|` operator composes transforms fluently |
| **Dependencies** | npm install / pip install / runtime setup | Zero — compiled into the slop-mcp binary |
| **Error feedback** | Stack traces, unstructured | Structured JSON with line/column/source |
| **Built-in functions** | Import libraries for transforms | 150+ built-ins for strings, lists, maps, JSON, regex, crypto |

SLOP scripts are intentionally constrained. They can't access the filesystem, make HTTP requests, or run shell commands. The *only* way to interact with the outside world is through MCP tool calls — which means the MCP server remains in full control of what's accessible.

## Further Reading

- [SLOP Language Reference](/docs/reference/slop-language) — full syntax and built-in functions
- [Context Efficiency](/docs/concepts/context-efficiency) — how progressive discovery saves tokens
- [Code Execution with MCP](https://www.anthropic.com/engineering/code-execution-with-mcp) — Anthropic's engineering article on the pattern
