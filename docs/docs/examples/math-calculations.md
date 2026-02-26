---
sidebar_position: 1
---

# Cross-MCP Workflows

This example shows the core SLOP pattern: using `execute_tool` to orchestrate multiple MCPs in a single workflow.

## The execute_tool Pattern

Every MCP tool is invoked the same way:

```
execute_tool mcp_name="<mcp>" tool_name="<tool>" parameters={...}
```

This uniform interface is what makes multi-MCP orchestration possible.

## Example: Figma Design System to Project Docs

**Goal**: Extract a component library from Figma, write design tokens to the project, and open a PR with the updates.

**Setup:**
```bash
slop-mcp mcp add figma -t streamable https://mcp.figma.com/mcp
slop-mcp mcp add filesystem npx @anthropic/mcp-filesystem
slop-mcp mcp add github -t streamable https://mcp.github.com/mcp
```

**Workflow:**
```
User: Extract the design tokens from our Figma component library
      and create a PR with the updated docs.

Claude: I'll orchestrate this across three MCPs.

Step 1: Extract the component library from Figma

> execute_tool mcp_name="figma" tool_name="get_file"
  parameters={"file_key": "abc123XYZ"}

Result: {
  "document": {
    "children": [
      {"name": "Colors/Primary", "fills": [{"color": {"r": 0.2, "g": 0.4, "b": 1.0}}]},
      {"name": "Colors/Secondary", "fills": [{"color": {"r": 0.1, "g": 0.8, "b": 0.5}}]},
      {"name": "Typography/Heading", "style": {"fontSize": 24, "fontFamily": "Inter"}}
    ]
  }
}

Step 2: Write design tokens to the project

> execute_tool mcp_name="filesystem" tool_name="write_file"
  parameters={
    "path": "docs/design-tokens.json",
    "content": "{\"colors\":{\"primary\":\"#3366FF\",\"secondary\":\"#1ACC80\"},\"typography\":{\"heading\":{\"size\":24,\"family\":\"Inter\"}}}"
  }

Step 3: Create a PR with the updates

> execute_tool mcp_name="github" tool_name="create_pull_request"
  parameters={
    "repo": "org/frontend",
    "title": "Update design tokens from Figma",
    "head": "design-tokens-update",
    "body": "Synced colors and typography from Figma component library."
  }
Result: { "url": "https://github.com/org/frontend/pull/42" }

Done! Design tokens extracted from Figma, written to docs/,
and PR #42 opened for review.
```

## Example: Email Triage to Jira

**Goal**: Scan emails flagged for action, create Jira tickets, and notify the intake channel.

**Setup:**
```bash
slop-mcp mcp add gmail npx @anthropic/mcp-gmail
slop-mcp mcp add jira -t streamable https://mcp.atlassian.com/jira/mcp
slop-mcp mcp add slack -t streamable https://mcp.slack.com/mcp
```

**Workflow:**
```
User: Triage my action-needed emails into Jira tickets.

Claude: I'll scan your inbox, create tickets, and notify #intake.

Step 1: Search for flagged emails

> execute_tool mcp_name="gmail" tool_name="search_messages"
  parameters={"query": "label:action-needed is:unread", "max_results": 10}

Result: [
  {"subject": "Deploy approval needed for v2.3", "from": "ci@company.com", "snippet": "Production deploy awaiting sign-off..."},
  {"subject": "Customer escalation: API timeouts", "from": "support@company.com", "snippet": "Acme Corp reporting 5s response times..."},
  {"subject": "Security audit findings", "from": "security@company.com", "snippet": "3 medium-severity items found..."}
]

Step 2: Create Jira issues from each email

> execute_tool mcp_name="jira" tool_name="create_issue"
  parameters={
    "project": "OPS",
    "summary": "Deploy approval needed for v2.3",
    "description": "Production deploy awaiting sign-off (from CI pipeline)",
    "issuetype": "Task",
    "labels": ["deploy"]
  }
Result: { "key": "OPS-201" }

> execute_tool mcp_name="jira" tool_name="create_issue"
  parameters={
    "project": "OPS",
    "summary": "Customer escalation: API timeouts",
    "description": "Acme Corp reporting 5s response times",
    "issuetype": "Bug",
    "labels": ["escalation", "performance"]
  }
Result: { "key": "OPS-202" }

> execute_tool mcp_name="jira" tool_name="create_issue"
  parameters={
    "project": "OPS",
    "summary": "Security audit findings",
    "description": "3 medium-severity items found in latest audit",
    "issuetype": "Task",
    "labels": ["security"]
  }
Result: { "key": "OPS-203" }

Step 3: Notify the intake channel

> execute_tool mcp_name="slack" tool_name="post_message"
  parameters={
    "channel": "#intake",
    "text": "Triaged 3 emails into Jira:\n• OPS-201: Deploy approval (Task)\n• OPS-202: API timeouts (Bug)\n• OPS-203: Security audit (Task)"
  }

Done! 3 emails triaged into Jira tickets and summary posted to #intake.
```

## Example: Release Changelog to Visual Summary

**Goal**: Categorize commits since the last release, generate a chart of changes by type, and post the summary.

**Setup:**
```bash
slop-mcp mcp add github -t streamable https://mcp.github.com/mcp
slop-mcp mcp add banana npx @anthropic/mcp-banana
slop-mcp mcp add slack -t streamable https://mcp.slack.com/mcp
```

**Workflow:**
```
User: Summarize the release changelog since v2.2.0 and post to Slack.

Claude: I'll fetch commits, chart them by type, and post the summary.

Step 1: Fetch commits since last tag

> execute_tool mcp_name="github" tool_name="compare_commits"
  parameters={
    "repo": "org/app",
    "base": "v2.2.0",
    "head": "main"
  }

Result: {
  "commits": [
    {"message": "feat: add SSO login"},
    {"message": "fix: resolve timeout on /api/users"},
    {"message": "feat: webhook retry logic"},
    {"message": "chore: update dependencies"},
    {"message": "fix: null check in billing"},
    {"message": "docs: update API reference"},
    {"message": "feat: rate limiting middleware"},
    {"message": "fix: race condition in queue"}
  ]
}

(Claude categorizes: 3 feat, 3 fix, 1 chore, 1 docs)

Step 2: Generate a bar chart of changes by category

> execute_tool mcp_name="banana" tool_name="generate_chart"
  parameters={
    "type": "bar",
    "title": "v2.3.0 Release Breakdown",
    "labels": ["feat", "fix", "chore", "docs"],
    "data": [3, 3, 1, 1]
  }
Result: { "image_url": "https://banana.dev/charts/abc123.png" }

Step 3: Post release summary to Slack

> execute_tool mcp_name="slack" tool_name="post_message"
  parameters={
    "channel": "#releases",
    "text": "v2.3.0 Release Summary (8 commits)\n• 3 features: SSO login, webhook retries, rate limiting\n• 3 fixes: API timeout, billing null check, queue race\n• 1 chore, 1 docs update",
    "attachments": [{"image_url": "https://banana.dev/charts/abc123.png"}]
  }

Done! Release changelog charted and posted to #releases.
```

## The Power of Uniform Access

Notice the pattern in every example:

1. **Same interface** — `execute_tool` works identically for every MCP
2. **Chain results** — output from one MCP feeds into another
3. **Mix freely** — design tools + project management + chat in one workflow
4. **Minimal context** — only the tools you use get loaded

This is what SLOP enables: **treating all your MCPs as one unified toolkit**.

## Context Efficiency

In the Figma design system example above:

| Approach | Context Used |
|----------|-------------|
| Traditional (3 MCPs, ~50 tools) | ~3,750 tokens |
| SLOP (8 meta-tools + tools actually used) | ~500 tokens |

**87% context savings** — and you still had access to all 50 tools.

## Next Steps

- [SLOP Script Patterns](/docs/examples/mcp-templates) — Automate multi-MCP workflows with `run_slop`
- [Multi-MCP Orchestration](/docs/examples/multi-mcp-orchestration) — Scale to 17+ MCPs
