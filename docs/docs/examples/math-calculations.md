---
sidebar_position: 1
---

# Combining MCPs with execute_tool

This example shows the core SLOP pattern: using `execute_tool` to orchestrate multiple MCPs in a single workflow.

## The execute_tool Pattern

Every MCP tool is invoked the same way:

```
execute_tool mcp_name="<mcp>" tool_name="<tool>" parameters={...}
```

This uniform interface is what makes multi-MCP orchestration possible.

## Setup: A Multi-MCP Environment

Let's set up a realistic scenario with math, data, and notification MCPs:

```bash
slop-mcp mcp add math-mcp npx @andylbrummer/math-mcp
slop-mcp mcp add postgres npx @anthropic/postgres-mcp
slop-mcp mcp add slack -t streamable https://mcp.slack.com/mcp
```

## Example: Sales Commission Calculator

**Goal**: Query sales data, calculate commissions, notify the team.

```
User: Calculate Q4 commissions for the sales team and post to Slack

Claude: I'll orchestrate this across three MCPs.

Step 1: Query sales data from Postgres

> execute_tool mcp_name="postgres" tool_name="query"
  parameters={
    "sql": "SELECT rep_name, SUM(revenue) as total FROM sales WHERE quarter='Q4' GROUP BY rep_name"
  }

Result: [
  {"rep_name": "Alice", "total": 125000},
  {"rep_name": "Bob", "total": 98000},
  {"rep_name": "Carol", "total": 156000}
]

Step 2: Calculate 8% commission for each rep

> execute_tool mcp_name="math-mcp" tool_name="calculate"
  parameters={"expression": "125000 * 0.08"}
Result: 10000

> execute_tool mcp_name="math-mcp" tool_name="calculate"
  parameters={"expression": "98000 * 0.08"}
Result: 7840

> execute_tool mcp_name="math-mcp" tool_name="calculate"
  parameters={"expression": "156000 * 0.08"}
Result: 12480

Step 3: Post summary to Slack

> execute_tool mcp_name="slack" tool_name="post_message"
  parameters={
    "channel": "#sales",
    "text": "Q4 Commissions:\n• Alice: $10,000\n• Bob: $7,840\n• Carol: $12,480\nTotal: $30,320"
  }

Done! Commissions calculated and posted to #sales.
```

## Example: Invoice with Tax Calculation

**Goal**: Fetch line items, calculate totals with tax, send via email.

```
User: Generate and send invoice #1234

Claude:

> execute_tool mcp_name="postgres" tool_name="query"
  parameters={
    "sql": "SELECT item, qty, price FROM invoice_lines WHERE invoice_id=1234"
  }

Result: [
  {"item": "Widget A", "qty": 10, "price": 25.00},
  {"item": "Widget B", "qty": 5, "price": 45.00}
]

> execute_tool mcp_name="math-mcp" tool_name="calculate"
  parameters={"expression": "(10 * 25) + (5 * 45)"}
Result: 475

> execute_tool mcp_name="math-mcp" tool_name="calculate"
  parameters={"expression": "475 * 1.08"}
Result: 513

> execute_tool mcp_name="email" tool_name="send"
  parameters={
    "to": "customer@example.com",
    "subject": "Invoice #1234",
    "body": "Subtotal: $475.00\nTax (8%): $38.00\nTotal: $513.00"
  }

Invoice sent!
```

## Example: Budget Monitoring

**Goal**: Check spending, compare to budget, alert if over.

```
User: Check if marketing is over budget this month

Claude:

> execute_tool mcp_name="postgres" tool_name="query"
  parameters={
    "sql": "SELECT SUM(amount) as spent FROM expenses WHERE dept='marketing' AND month=CURRENT_MONTH"
  }
Result: {"spent": 47500}

> execute_tool mcp_name="postgres" tool_name="query"
  parameters={
    "sql": "SELECT budget FROM budgets WHERE dept='marketing'"
  }
Result: {"budget": 45000}

> execute_tool mcp_name="math-mcp" tool_name="calculate"
  parameters={"expression": "((47500 - 45000) / 45000) * 100"}
Result: 5.56

Marketing is 5.6% over budget ($47,500 vs $45,000 budget).

> execute_tool mcp_name="slack" tool_name="post_message"
  parameters={
    "channel": "#finance-alerts",
    "text": "⚠️ Marketing is 5.6% over budget this month"
  }

Alert posted to #finance-alerts.
```

## The Power of Uniform Access

Notice the pattern in every example:

1. **Same interface** - `execute_tool` works identically for every MCP
2. **Chain results** - Output from one MCP feeds into another
3. **Mix freely** - Database + math + notifications in one workflow
4. **Minimal context** - Only the tools you use get loaded

This is what SLOP enables: **treating all your MCPs as one unified toolkit**.

## Context Efficiency

In the commission example above:

| Approach | Context Used |
|----------|-------------|
| Traditional (3 MCPs, ~50 tools) | ~3,750 tokens |
| SLOP (only tools actually used) | ~300 tokens |

**92% context savings** - and you still had access to all 50 tools.

## Next Steps

- [MCP Templates](/docs/examples/mcp-templates) - Create reusable multi-MCP patterns
- [Multi-MCP Orchestration](/docs/examples/multi-mcp-orchestration) - Scale to 17+ MCPs
