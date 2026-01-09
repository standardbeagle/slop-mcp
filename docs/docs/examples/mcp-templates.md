---
sidebar_position: 2
---

# MCP Templates & Computation Extraction

This guide shows how to extract reusable computation patterns from MCPs, turning complex tool calls into simple, repeatable templates.

## The Template Pattern

MCPs often have powerful tools with many parameters. Instead of remembering every parameter, create templates that encode common patterns.

### Example: The Everything MCP's Add Tool

The [everything-mcp](https://github.com/anthropics/everything-mcp) has a simple `add` tool:

```json
{
  "name": "add",
  "description": "Add two numbers",
  "inputSchema": {
    "type": "object",
    "properties": {
      "a": { "type": "number" },
      "b": { "type": "number" }
    },
    "required": ["a", "b"]
  }
}
```

### Direct Usage

```
> execute_tool mcp_name="everything" tool_name="add"
  parameters={"a": 5, "b": 3}
Result: 8
```

### Templated Usage

Create a template that builds on `add`:

```kdl
// templates/sum-list.kdl
template "sum-list" {
    description "Sum a list of numbers using repeated addition"
    mcp "everything"

    // Uses the add tool iteratively
    implementation {
        reduce tool="add" over="$numbers" {
            accumulator "a"
            current "b"
            initial 0
        }
    }
}
```

Now:
```
/sum-list numbers=[1, 2, 3, 4, 5]
Result: 15
```

## Extracting Patterns from Complex MCPs

### Pattern 1: Parameter Defaults

Many MCP tools have optional parameters with sensible defaults. Encode these:

```kdl
// Original tool has 12 parameters
// Template encodes your common use case

template "quick-deploy" {
    mcp "kubernetes"
    tool "apply"

    defaults {
        namespace "production"
        dry_run false
        wait true
        timeout "5m"
        // ... 8 more defaults
    }

    params {
        manifest { required true }
        // Only expose what varies
    }
}
```

### Pattern 2: Chained Computations

Extract multi-step workflows:

```kdl
template "tax-calculation" {
    description "Calculate price with tax and format as currency"

    steps {
        step "calculate" {
            mcp "math-mcp"
            tool "calculate"
            expression "$price * (1 + $tax_rate)"
            output "total"
        }

        step "format" {
            mcp "formatter"
            tool "currency"
            params {
                value "$total"
                currency "$currency"
                locale "$locale"
            }
        }
    }

    params {
        price { required true }
        tax_rate { default 0.08 }
        currency { default "USD" }
        locale { default "en-US" }
    }
}
```

Usage:
```
/tax-calculation price=99.99
Result: $107.99
```

### Pattern 3: Conditional Logic

Handle different scenarios:

```kdl
template "smart-search" {
    description "Search using the best available MCP"

    conditions {
        when "$query contains 'code'" {
            mcp "github"
            tool "search_code"
        }
        when "$query contains 'file'" {
            mcp "filesystem"
            tool "search"
        }
        default {
            mcp "web-search"
            tool "search"
        }
    }
}
```

### Pattern 4: Data Transformation

Transform data between MCP calls:

```kdl
template "analyze-and-report" {
    steps {
        step "fetch" {
            mcp "database"
            tool "query"
            params { sql "$query" }
            output "raw_data"
        }

        step "transform" {
            transform "$raw_data" {
                select ["name", "value", "date"]
                filter "value > 100"
                sort "date desc"
                limit 10
            }
            output "filtered_data"
        }

        step "visualize" {
            mcp "charts"
            tool "bar_chart"
            params {
                data "$filtered_data"
                x "name"
                y "value"
            }
        }
    }
}
```

## Real-World Template Examples

### Invoice Calculator

```kdl
template "invoice-total" {
    description "Calculate invoice with line items, tax, and discount"
    mcp "math-mcp"

    steps {
        step "subtotal" {
            tool "calculate"
            // Sum all line items
            expression "sum($line_items.map(item => item.qty * item.price))"
            output "subtotal"
        }

        step "after-discount" {
            tool "calculate"
            expression "$subtotal * (1 - $discount)"
            output "discounted"
        }

        step "with-tax" {
            tool "calculate"
            expression "$discounted * (1 + $tax_rate)"
            output "total"
        }
    }

    params {
        line_items { required true }
        discount { default 0 }
        tax_rate { default 0.08 }
    }

    returns {
        subtotal "$subtotal"
        discount_amount "$subtotal - $discounted"
        tax_amount "$total - $discounted"
        total "$total"
    }
}
```

### Image Processing Pipeline

```kdl
template "optimize-image" {
    description "Resize, compress, and upload image"

    steps {
        step "resize" {
            mcp "image-mcp"
            tool "resize"
            params {
                input "$source"
                width "$width"
                maintain_aspect true
            }
            output "resized_path"
        }

        step "compress" {
            mcp "image-mcp"
            tool "compress"
            params {
                input "$resized_path"
                quality 85
                format "webp"
            }
            output "compressed_path"
        }

        step "upload" {
            mcp "s3"
            tool "upload"
            params {
                file "$compressed_path"
                bucket "$bucket"
                key "images/$filename.webp"
            }
            output "url"
        }
    }

    params {
        source { required true }
        width { default 1200 }
        bucket { default "my-images" }
        filename { required true }
    }
}
```

## Benefits of Templates

| Aspect | Direct Tool Calls | Templates |
|--------|------------------|-----------|
| Context usage | Full schema each time | Once defined, minimal |
| Error prone | Easy to forget params | Defaults prevent errors |
| Reusability | Copy/paste | Invoke by name |
| Maintenance | Update everywhere | Update template once |
| Discovery | Search all tools | Browse templates |

## Creating Templates

### Via CLI

```bash
slop-mcp template create my-template \
  --mcp math-mcp \
  --tool calculate \
  --param "x:First number" \
  --param "y:Second number" \
  --expression "x * y + 100"
```

### Via Config File

Add to `templates/` directory in your project.

### Via SLOP Script

```slop
@template "my-template" {
  @use math-mcp.calculate
  @param x: number
  @param y: number
  @compute x * y + 100
}
```

## Summary

Templates let you:

1. **Encode domain knowledge** - Capture your common patterns
2. **Simplify complex tools** - Hide unnecessary parameters
3. **Chain computations** - Multi-step workflows in one call
4. **Reduce errors** - Sensible defaults prevent mistakes
5. **Save context** - Define once, invoke with minimal tokens

Combined with SLOP MCP's lazy loading, templates make your MCP toolkit both powerful and efficient.
