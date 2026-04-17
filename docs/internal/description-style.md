# Meta-Tool Description Style Guide

## Why

Tool descriptions are sent to agents verbatim on every request.
Shorter descriptions reduce token cost, reduce context noise, and improve
agent parse speed. Every word in a description competes with tool output
for context budget.

## Rules

- Drop articles: a, an, the.
- Drop filler: just, really, basically, actually, simply, "you can",
  "this allows you to", "use this to".
- Drop hedging and pleasantries.
- Keep technical terms exact: tool names, field names, enum values,
  error text, JSON field paths.
- Fragments OK. Pattern: `[thing] [action] [reason]. [next step].`
- Target ≤80 chars per param description where possible.
- Don't sacrifice correctness for brevity.

## Preserve Always

- Code blocks verbatim — SLOP examples must be valid SLOP.
- Enum values listed explicitly with exact spelling.
- Required/optional context where operationally needed
  (e.g. "required for login/logout/status").
- Error message strings shown verbatim.

## Examples

### Tool description

Before:
> Search and explore all registered MCP tools by name and description.
> Results are paginated (default: 20, max: 100). Use offset for subsequent
> pages. Response includes total count and has_more flag.

After:
> Fuzzy search tools across connected MCPs. Ranked results. Paginated
> (default: 20, max: 100). Use offset for next page. Response includes
> total and has_more.

---

Before:
> Authenticate with MCP servers using OAuth. Actions: login (initiate OAuth
> flow), logout (remove token), status (check auth status), list (show all
> authenticated MCPs). Returns text status.

After:
> OAuth for MCP servers. Actions: login (start flow), logout (drop token),
> status (check), list (all authenticated). Returns text.

---

Before:
> Get metadata for connected MCP servers. Returns tool names and descriptions
> by default (compact). Use verbose=true for full schemas, or specify both
> mcp_name and tool_name to get schema for a specific tool.

After:
> Metadata for connected MCPs. Compact by default (names+descriptions).
> verbose=true for full schemas. mcp_name+tool_name for single-tool schema.

### Param description

Before:
> Where to save: memory (default, runtime only), user
> (~/.config/slop-mcp/config.kdl), or project (.slop-mcp.kdl)

After:
> Persistence: memory (default, runtime only), user
> (~/.config/slop-mcp/config.kdl), project (.slop-mcp.kdl)

---

Before:
> Mark MCP as dynamic (always re-fetch tool list, never cache)

After:
> Always re-fetch tool list, skip cache

## Applying Changes

Edit `internal/server/tools.go` (tool-level `Description:`) and
`internal/server/schemas.go` / `internal/server/agnt_watch.go`
(param-level `"description":` in JSON schemas).

Run `go test -short ./...` and `go build -tags mcp_go_client_oauth ./...`
after edits to confirm no regressions.
