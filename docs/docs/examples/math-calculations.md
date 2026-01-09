---
sidebar_position: 1
---

# Math Calculations with math-mcp

This example shows how to use SLOP MCP with [andylbrummer/math-mcp](https://github.com/andylbrummer/math-mcp) for mathematical computations.

## Setup

First, add the math-mcp to your configuration:

```bash
slop-mcp mcp add math-mcp npx @andylbrummer/math-mcp
```

Verify it's connected:

```bash
slop-mcp mcp list
# math-mcp (stdio) - connected, 8 tools
```

## Available Tools

Search for math tools:

```
> search_tools mcp_name="math-mcp"

Found 8 tools:
- calculate: Evaluate mathematical expressions
- solve_equation: Solve algebraic equations
- derivative: Calculate derivatives
- integral: Calculate integrals
- matrix_multiply: Multiply matrices
- statistics: Calculate statistical measures
- convert_units: Convert between units
- financial: Financial calculations (NPV, IRR, etc.)
```

## Basic Calculations

### Simple Arithmetic

```
User: What's 15% of $847.50?

Claude: Let me calculate that.

> execute_tool mcp_name="math-mcp" tool_name="calculate"
  parameters={"expression": "847.50 * 0.15"}

Result: 127.125

15% of $847.50 is $127.13
```

### Complex Expressions

```
User: Calculate the compound interest on $10,000 at 5% APR
      compounded monthly for 3 years

Claude: > execute_tool mcp_name="math-mcp" tool_name="calculate"
  parameters={
    "expression": "10000 * (1 + 0.05/12)^(12*3)"
  }

Result: 11614.72

After 3 years, you'd have $11,614.72
(That's $1,614.72 in interest)
```

## Solving Equations

```
User: Solve for x: 2x² + 5x - 3 = 0

Claude: > execute_tool mcp_name="math-mcp" tool_name="solve_equation"
  parameters={
    "equation": "2*x^2 + 5*x - 3 = 0",
    "variable": "x"
  }

Result: x = 0.5 or x = -3

The solutions are x = 0.5 and x = -3
```

## Calculus

### Derivatives

```
User: What's the derivative of x³ + 2x² - 5x + 1?

Claude: > execute_tool mcp_name="math-mcp" tool_name="derivative"
  parameters={
    "expression": "x^3 + 2*x^2 - 5*x + 1",
    "variable": "x"
  }

Result: 3x² + 4x - 5

The derivative is 3x² + 4x - 5
```

### Integrals

```
User: Integrate sin(x) * cos(x) dx

Claude: > execute_tool mcp_name="math-mcp" tool_name="integral"
  parameters={
    "expression": "sin(x) * cos(x)",
    "variable": "x"
  }

Result: (sin(x))² / 2 + C

The integral is ½sin²(x) + C
```

## Statistics

```
User: Calculate mean, median, and standard deviation of:
      [23, 45, 67, 89, 12, 34, 56, 78, 90, 11]

Claude: > execute_tool mcp_name="math-mcp" tool_name="statistics"
  parameters={
    "data": [23, 45, 67, 89, 12, 34, 56, 78, 90, 11],
    "measures": ["mean", "median", "std_dev"]
  }

Result: {
  "mean": 50.5,
  "median": 50.5,
  "std_dev": 29.14
}

Statistics for your data:
- Mean: 50.5
- Median: 50.5
- Standard Deviation: 29.14
```

## Financial Calculations

### Net Present Value

```
User: Calculate NPV of cash flows [-1000, 300, 400, 500, 600]
      at 10% discount rate

Claude: > execute_tool mcp_name="math-mcp" tool_name="financial"
  parameters={
    "operation": "npv",
    "cash_flows": [-1000, 300, 400, 500, 600],
    "rate": 0.10
  }

Result: 362.74

The NPV is $362.74 (positive = good investment!)
```

### Loan Payment

```
User: Monthly payment for $250,000 mortgage at 6.5% for 30 years?

Claude: > execute_tool mcp_name="math-mcp" tool_name="financial"
  parameters={
    "operation": "pmt",
    "principal": 250000,
    "rate": 0.065,
    "periods": 360
  }

Result: 1580.17

Monthly payment: $1,580.17
Total paid over 30 years: $568,861.20
Total interest: $318,861.20
```

## Unit Conversions

```
User: Convert 100 kilometers per hour to miles per hour

Claude: > execute_tool mcp_name="math-mcp" tool_name="convert_units"
  parameters={
    "value": 100,
    "from": "km/h",
    "to": "mph"
  }

Result: 62.14

100 km/h = 62.14 mph
```

## Creating a Calculation Skill

For frequently used calculations, create a skill:

```kdl
// skills/tip-calculator.kdl
skill "tip" {
    description "Calculate tip and split bill"
    mcp "math-mcp"
    tool "calculate"

    params {
        amount {
            description "Bill amount"
            required true
        }
        percent {
            description "Tip percentage"
            default 20
        }
        split {
            description "Number of people"
            default 1
        }
    }

    expression "$amount * (1 + $percent/100) / $split"
}
```

Now use it:

```
/tip amount=85.50 percent=18 split=3

Result: Each person pays $33.66 (including 18% tip)
```

## Context Efficiency

Notice that throughout these examples, only the specific tool being used was loaded into context - not all 8 math tools. This is SLOP MCP's lazy loading in action:

```
Traditional MCP: ~600 tokens (all 8 tool schemas)
SLOP MCP: ~75 tokens (just the tool you searched for)
```

## Next Steps

- [MCP Templates](/docs/examples/mcp-templates) - Extract computation patterns
- [Multi-MCP Orchestration](/docs/examples/multi-mcp-orchestration) - Combine math with other MCPs
