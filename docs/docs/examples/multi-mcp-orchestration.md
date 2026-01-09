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

// Math & Data
mcp "math-mcp" {
    transport "stdio"
    command "npx"
    args "@andylbrummer/math-mcp"
}

mcp "statistics" {
    transport "stdio"
    command "python"
    args "-m" "stats_mcp"
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

// Database
mcp "postgres" {
    transport "stdio"
    command "npx"
    args "@anthropic/postgres-mcp"
    env {
        DATABASE_URL "${DATABASE_URL}"
    }
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
6 SLOP meta-tools × ~65 tokens = 390 tokens

That's 0.2% of context - virtually nothing!
```

**Savings: 31,485 tokens (98.8%)**

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

### Pattern 2: Data Pipeline

Query database → Calculate statistics → Generate report → Email:

```
User: Send me a weekly sales summary

Claude: I'll pull data, analyze it, and email the report.

> execute_tool mcp_name="postgres" tool_name="query"
  parameters={
    "sql": "SELECT * FROM sales WHERE date >= NOW() - INTERVAL '7 days'"
  }
Result: [{ "product": "Widget", "revenue": 5000 }, ...]

> execute_tool mcp_name="statistics" tool_name="summarize"
  parameters={
    "data": [...],
    "metrics": ["sum", "mean", "growth_rate"]
  }
Result: { "total": 127500, "average": 18214, "growth": 0.12 }

> execute_tool mcp_name="email" tool_name="send"
  parameters={
    "to": "team@company.com",
    "subject": "Weekly Sales Summary",
    "body": "Total: $127,500 (+12% WoW)..."
  }

Sent! Weekly summary: $127,500 total revenue, 12% growth.
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

```slop
// scripts/deploy-and-notify.slop
@name "deploy-and-notify"
@description "Deploy to Kubernetes and notify team"

// Build
@call docker.build {
  context: "."
  tag: "$VERSION"
}

// Push
@call docker.push {
  image: "myapp:$VERSION"
}

// Deploy
@call kubernetes.apply {
  manifest: "./k8s/deployment.yaml"
  set: { "image.tag": "$VERSION" }
}

// Wait for rollout
@call kubernetes.wait {
  resource: "deployment/myapp"
  condition: "available"
}

// Notify
@call slack.post_message {
  channel: "#deployments"
  text: "Deployed myapp:$VERSION to production"
}

// Create release notes
@call github.create_release {
  tag: "v$VERSION"
  generate_notes: true
}
```

Run with:
```bash
slop-mcp run scripts/deploy-and-notify.slop VERSION=1.2.3
```

Or in Claude Code:
```
> run_slop file_path="scripts/deploy-and-notify.slop"
  parameters={"VERSION": "1.2.3"}
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
