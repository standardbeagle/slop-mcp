---
sidebar_position: 3
---

# Multi-MCP Orchestration

This is where SLOP MCP truly shines: orchestrating **17+ MCPs** without context bloat.

## The "17 MCPs and Claude Don't Care" Setup

Here's a real-world configuration with 17 MCPs:

```kdl
// .slop-mcp.kdl - Production setup

// Development Tools
mcp "filesystem" {
    transport "stdio"
    command "npx"
    args "@anthropic/filesystem-mcp" "./src" "./docs" "./tests"
}

mcp "git" {
    transport "stdio"
    command "npx"
    args "@anthropic/git-mcp"
}

mcp "github" {
    transport "streamable"
    url "https://mcp.github.com/mcp"
}

// Search & Code Intelligence
mcp "lci" {
    transport "stdio"
    command "lci-mcp"
}

mcp "brave-search" {
    transport "stdio"
    command "npx"
    args "@anthropic/mcp-brave-search"
}

// Design & Product
mcp "figma" {
    transport "streamable"
    url "https://mcp.figma.com/mcp"
}

mcp "linear" {
    transport "streamable"
    url "https://mcp.linear.app/mcp"
}

// Project Management
mcp "dart" {
    transport "streamable"
    url "https://mcp.dartai.com/mcp"
}

mcp "notion" {
    transport "streamable"
    url "https://mcp.notion.com/mcp"
}

// Communication
mcp "slack" {
    transport "streamable"
    url "https://mcp.slack.com/mcp"
}

mcp "email" {
    transport "stdio"
    command "npx"
    args "@anthropic/email-mcp"
}

// Cloud Infrastructure
mcp "aws" {
    transport "stdio"
    command "python"
    args "-m" "aws_mcp"
}

mcp "docker" {
    transport "stdio"
    command "npx"
    args "@anthropic/docker-mcp"
}

mcp "kubernetes" {
    transport "stdio"
    command "kubectl-mcp"
}

// Monitoring
mcp "datadog" {
    transport "streamable"
    url "https://mcp.datadoghq.com/mcp"
}

mcp "pagerduty" {
    transport "streamable"
    url "https://mcp.pagerduty.com/mcp"
}

// Visualization
mcp "banana" {
    transport "stdio"
    command "npx"
    args "@anthropic/mcp-banana"
}
```

## Context Impact

### Traditional MCP Setup

```
17 MCPs × ~25 tools each × ~75 tokens/tool = 31,875 tokens

That's 16% of Claude's context window GONE
before you've said anything!
```

### With SLOP MCP

```
8 SLOP meta-tools × ~65 tokens = ~520 tokens

That's 0.3% of context - virtually nothing!
```

**Savings: ~31,355 tokens (98.4%)**

## Orchestration Patterns

### Pattern 1: Cross-MCP Workflows

Create an issue, link to Figma, notify Slack:

```
User: Create a bug report for the login issue, link the Figma mockup,
      and notify the team

Claude: I'll orchestrate this across multiple MCPs.

> search_tools query="create issue"
Found: linear:create_issue, github:create_issue

> execute_tool mcp_name="linear" tool_name="create_issue"
  parameters={
    "title": "Login button not responding",
    "description": "Users report...",
    "priority": "high"
  }
Result: { "id": "LIN-123", "url": "https://linear.app/..." }

> search_tools query="search design file"
> execute_tool mcp_name="figma" tool_name="get_file"
  parameters={"file_key": "abc123"}
Result: { "url": "https://figma.com/file/..." }

> execute_tool mcp_name="linear" tool_name="add_attachment"
  parameters={
    "issue_id": "LIN-123",
    "url": "https://figma.com/file/...",
    "title": "Login mockup"
  }

> execute_tool mcp_name="slack" tool_name="post_message"
  parameters={
    "channel": "#bugs",
    "text": "New high-priority bug: LIN-123 - Login button issue"
  }

Done! Created LIN-123, linked Figma mockup, notified #bugs channel.
```

**Context used: Only the 4 specific tools needed, not 425 total tools.**

### Pattern 2: Changelog to Infographic

Fetch release commits → Categorize → Generate chart → Post to Slack:

```
User: Create a visual release summary for v3.0.0

Claude: I'll categorize commits, chart them, and post the summary.

> execute_tool mcp_name="github" tool_name="compare_commits"
  parameters={
    "repo": "org/app",
    "base": "v2.9.0",
    "head": "v3.0.0"
  }
Result: {
  "commits": [
    {"message": "feat: add SSO login"},
    {"message": "feat: webhook retry logic"},
    {"message": "fix: timeout on /api/users"},
    {"message": "fix: null check in billing"},
    {"message": "chore: update dependencies"},
    {"message": "docs: API reference"}
  ]
}

(Claude categorizes: 2 feat, 2 fix, 1 chore, 1 docs)

> execute_tool mcp_name="banana" tool_name="generate_chart"
  parameters={
    "type": "bar",
    "title": "v3.0.0 Release Breakdown",
    "labels": ["feat", "fix", "chore", "docs"],
    "data": [2, 2, 1, 1]
  }
Result: { "image_url": "https://banana.dev/charts/xyz789.png" }

> execute_tool mcp_name="slack" tool_name="post_message"
  parameters={
    "channel": "#releases",
    "text": "v3.0.0 Released (6 commits)\n• 2 features: SSO login, webhook retries\n• 2 fixes: API timeout, billing null check\n• 1 chore, 1 docs",
    "attachments": [{"image_url": "https://banana.dev/charts/xyz789.png"}]
  }

Done! Release chart generated and posted to #releases.
```

### Pattern 3: Incident Response

Detect issue → Check logs → Create ticket → Page on-call:

```
User: Something's wrong with the API - response times are spiking

Claude: Let me investigate and escalate.

> execute_tool mcp_name="datadog" tool_name="query_metrics"
  parameters={
    "query": "avg:api.response_time{service:main}",
    "timeframe": "1h"
  }
Result: { "avg": 2500, "p99": 8000, "normal_avg": 200 }

> execute_tool mcp_name="datadog" tool_name="search_logs"
  parameters={
    "query": "service:main status:error",
    "limit": 50
  }
Result: [{ "message": "Connection pool exhausted", ... }]

> execute_tool mcp_name="linear" tool_name="create_issue"
  parameters={
    "title": "API Incident: Connection pool exhausted",
    "priority": "urgent",
    "labels": ["incident", "api"]
  }
Result: { "id": "INC-789" }

> execute_tool mcp_name="pagerduty" tool_name="create_incident"
  parameters={
    "title": "API Response Time Critical",
    "urgency": "high",
    "service": "api-main"
  }

Incident INC-789 created. On-call engineer paged.
Root cause: Connection pool exhausted.
```

## SLOP Scripts for Complex Orchestration

For repeatable multi-MCP workflows, use SLOP scripts:

```python
// scripts/deploy-and-notify.slop
version = store_get("VERSION")

// Build and push
docker.build(context: ".", tag: format("myapp:{}", version))
docker.push(image: format("myapp:{}", version))

// Deploy and wait
kubernetes.apply(
    manifest: "./k8s/deployment.yaml",
    set: {"image.tag": version}
)
kubernetes.wait(
    resource: "deployment/myapp",
    condition: "available"
)

// Notify team
slack.post_message(
    channel: "#deployments",
    text: format("Deployed myapp:{} to production", version)
)

// Create release
github.create_release(
    tag: format("v{}", version),
    generate_notes: true
)

emit(status: "deployed", version: version)
```

Run via `run_slop`:
```
> run_slop script='store_set("VERSION", "1.2.3")'
> run_slop file_path="scripts/deploy-and-notify.slop"
```

## Best Practices

### 1. Authenticate OAuth MCPs Early

```bash
# Do this once per session/day
slop-mcp mcp auth login figma
slop-mcp mcp auth login linear
slop-mcp mcp auth login slack
```

### 2. Use Specific Searches

```bash
# Good: finds exactly what you need
search_tools query="create linear issue"

# Less good: returns too many results
search_tools query="create"
```

### 3. Create Skills for Common Workflows

If you do the same multi-MCP operation often, make it a skill.

### 4. Scope MCPs Appropriately

- **Project scope**: MCPs specific to this codebase
- **User scope**: Your personal tools (email, calendar)
- **Local scope**: Secrets, API keys

### 5. Monitor What's Loaded

```
> get_metadata

{
  "connected_mcps": 17,
  "total_tools": 425,
  "tools_loaded_this_session": 12,
  "context_saved": "98.8%"
}
```

## Summary

With SLOP MCP, you can:

- Connect **unlimited MCPs** with constant context overhead
- **Orchestrate** across MCPs with intelligent tool discovery
- **Script** complex multi-MCP workflows
- **Scale** your AI capabilities without hitting context limits

**Install 17 MCPs. Or 170. Claude won't even notice.**
