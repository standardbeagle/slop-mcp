---
sidebar_position: 5
---

# Tool Customization

Tool descriptions from third-party MCPs are written for human readers, not agents. A single `get_file` description may run 400 tokens of prose and caveats — multiplied across dozens of tools, that erodes the context budget fast. The customization layer lets you replace any tool's description and parameter docs with a tighter version, define entirely new composite tools backed by SLOP scripts, and share those customizations with your team.

Customization is managed through the `customize_tools` meta-tool, which exposes eight actions: `set_override`, `remove_override`, `list_overrides`, `define_custom`, `remove_custom`, `list_custom`, `export`, and `import`.

## When to Customize

- A vendor MCP ships verbose, marketing-heavy tool descriptions that burn context on every search hit.
- Parameter docs include deprecation notices, edge-case caveats, or SDK history that an agent doesn't need.
- You want project-specific shortcuts — a single `deploy_staging` tool that wraps three github + terraform calls.
- Your team has agreed on a set of description standards and wants them committed to the repo.

## Scopes

Customizations are stored at one of three scopes. Higher-priority scopes shadow lower ones; the original MCP tool is never modified.

| Scope | File | Commit to git? |
|-------|------|----------------|
| `user` | `~/.config/slop-mcp/customizations.json` | No — personal |
| `project` | `.slop-mcp-custom.kdl` in project root | Yes — shared with team |
| `local` | `.slop-mcp-custom.local.kdl` in project root | No — git-ignored |

When the same tool has overrides in multiple scopes, the order of precedence is: **local > project > user**.

## Overrides

An override replaces the description and/or parameter docs that `search_tools` and `get_metadata` return for an existing MCP tool. The underlying tool implementation is unchanged — only what the agent *sees* is different.

### Setting an override

Pass `set_override` to `customize_tools` with the MCP name, tool name, and your replacement text:

```json
{
  "action": "set_override",
  "mcp": "figma",
  "tool": "get_file",
  "description": "Fetch a Figma document by file key. Returns page tree and node metadata.",
  "params": {
    "file_key": "Figma file key (last path segment of the Figma URL)",
    "depth": "Node traversal depth (default 1; increase for deeper component trees)"
  },
  "scope": "project"
}
```

`params` is a flat map of parameter name → replacement description. Omit a parameter to leave its original description intact. Omit `scope` to default to `user`.

### Staleness detection

slop-mcp hashes the upstream tool's schema at the time an override is saved. If the MCP vendor later changes the tool's input schema, your override is flagged stale. To see only stale overrides:

```json
{
  "action": "list_overrides",
  "stale_only": true
}
```

Or use the `manage_mcps` shortcut:

```json
{
  "action": "list_stale_overrides"
}
```

A stale override still applies — the agent won't see a broken schema — but you should review it to make sure your description still matches the updated parameters.

### Removing an override

```json
{
  "action": "remove_override",
  "mcp": "figma",
  "tool": "get_file",
  "scope": "project"
}
```

Removing an override restores the original MCP tool description at that scope. If a lower-priority scope also has an override, that one becomes active.

## Custom Tools

Custom tools are new tools you define from scratch. They appear in `search_tools` results alongside real MCP tools and can be executed via `execute_tool`. Route them by passing `_custom` as the MCP name, or leave MCP name empty — slop-mcp checks the custom tool registry first.

### Defining a custom tool

A custom tool has a name, description, JSON Schema for its inputs, and a SLOP body that runs when the tool is called. The body has access to an `args` map containing the validated input values.

```json
{
  "action": "define_custom",
  "name": "figma_page_names",
  "description": "List all page names in a Figma file.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "file_key": {"type": "string", "description": "Figma file key"}
    },
    "required": ["file_key"]
  },
  "body": "doc = execute_tool(\"figma\", \"get_file\", {file_key: args[\"file_key\"]})\nmap(doc[\"pages\"], |p| p[\"name\"])",
  "scope": "project"
}
```

The tool's return value is the last evaluated expression in the body (or any value passed to `emit()`).

### SLOP body syntax quick reference

Custom tool bodies are SLOP scripts. Key rules:

- **No `$`-prefixed variables.** Variables are plain identifiers: `doc`, `pages`, `result`.
- **Map field access uses bracket notation:** `args["file_key"]`, `page["name"]`.
- **Pipe operator chains transforms:** `items | filter(|x| x["active"]) | map(|x| x["name"])`.
- **Anonymous functions use `|param|` syntax:** `map(list, |item| item["id"])`.
- **`emit()` sets named output fields:** `emit(count: len(result), names: result)`.
- **`execute_tool(mcp, tool, args_map)`** calls any connected MCP tool.

Example body calling two MCPs and returning a merged result:

```
issues = execute_tool("github", "list_issues", {repo: args["repo"], state: "open"})
names = map(issues, |i| i["title"])
emit(count: len(issues), titles: names)
```

### Arg binding and shorthand

Inside a body, all inputs arrive in the `args` map. For frequently accessed parameters you can bind them at the top of the body as plain variables:

```
file_key = args["file_key"]
depth = args["depth"]
doc = execute_tool("figma", "get_file", {file_key: file_key, depth: depth})
```

If a variable name you bind would shadow a SLOP built-in (e.g., `map`, `len`, `filter`), slop-mcp logs a warning and the binding is skipped — use `args["map"]` directly in that case.

### Body size limit

The body string is capped at **64 KB** by default. Set `SLOP_MAX_CUSTOM_BODY` in your environment to raise the limit. For logic that exceeds this, store the script as a recipe file and call it via `run_slop` with `recipe:` instead.

### Recursion depth guard

Custom tools may call other custom tools. The call stack is limited to **16 frames** (`ErrCustomToolRecursion`). Mutual recursion beyond this depth returns an error rather than a stack overflow.

### Listing and removing custom tools

```json
{ "action": "list_custom", "scope": "project" }
```

```json
{
  "action": "remove_custom",
  "name": "figma_page_names",
  "scope": "project"
}
```

## Import / Export

Customizations can be packaged into a portable JSON file — a *customization pack* — for sharing across machines or repositories.

### Exporting a pack

```json
{
  "action": "export",
  "scope": "project",
  "path": "./figma-custom-pack.json"
}
```

The exported file contains all overrides and custom tools at the specified scope, plus the schema hashes used for staleness detection.

### Importing a pack

```json
{
  "action": "import",
  "path": "./figma-custom-pack.json",
  "scope": "project"
}
```

Importing merges the pack into the target scope. Existing entries are overwritten; entries not in the pack are untouched. The schema hashes are imported as-is — if the current MCP version differs, imported overrides may immediately appear stale.

### Sharing via git

Export to a versioned file in your repository, then commit it. Teammates run `import` once after pulling. Because `project` scope writes to `.slop-mcp-custom.kdl` (which is committed), the pack file can be a convenience snapshot rather than the primary storage:

```
# One-time setup after clone
customize_tools import path: ./.slop-mcp-packs/figma.json scope: project
```

## Staleness and Upgrades

When a MCP vendor ships a schema change, overrides referencing the old schema are flagged. The hash stored at save time is compared against the current schema hash on each load. A mismatch sets the override's `stale` field to `true`.

Staleness doesn't break the tool — the override still applies and agents still see your description. But you should review the updated schema and either update the description or remove the override if the vendor's new description is acceptable.

Use `list_overrides stale_only: true` to find affected entries before upgrading an MCP version.

## Reserved Memory Banks

The `_slop.*` namespace in the persistent memory store is reserved for slop-mcp internals. Custom tool bodies may **read** from `_slop.*` banks using `mem_get`, but **writes must go through `customize_tools`** — direct `mem_save` calls to `_slop.*` keys return an error. This prevents custom tools from corrupting the customization index or staleness hashes.

## Example: Figma Compression Workflow

This walkthrough shows the full lifecycle: discovering verbose docs, trimming them, exporting, and sharing with the team.

**Step 1 — Discover the problem.**

The agent calls `search_tools` for "figma" and notices that `get_file`, `get_node`, and `get_comments` each return 300+ token descriptions. Context cost is significant when any figma tool is in scope.

**Step 2 — Set overrides for each tool.**

```json
{
  "action": "set_override",
  "mcp": "figma",
  "tool": "get_file",
  "description": "Fetch a Figma document. Returns page tree and top-level nodes.",
  "params": {
    "file_key": "Figma file key (last segment of share URL)",
    "depth": "Traversal depth (default 1)"
  },
  "scope": "project"
}
```

Repeat for `get_node` and `get_comments`.

**Step 3 — Define a composite helper.**

Rather than calling `get_file` and parsing the result every time, define a custom tool that returns just the page names:

```json
{
  "action": "define_custom",
  "name": "figma_page_names",
  "description": "Return all page names in a Figma file as a list of strings.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "file_key": {"type": "string"}
    },
    "required": ["file_key"]
  },
  "body": "doc = execute_tool(\"figma\", \"get_file\", {file_key: args[\"file_key\"]})\nmap(doc[\"pages\"], |p| p[\"name\"])",
  "scope": "project"
}
```

**Step 4 — Export and commit.**

```json
{
  "action": "export",
  "scope": "project",
  "path": "./.slop-mcp-packs/figma.json"
}
```

Commit `.slop-mcp-packs/figma.json` to the repository.

**Step 5 — Teammates import.**

After pulling, each teammate runs:

```json
{
  "action": "import",
  "path": "./.slop-mcp-packs/figma.json",
  "scope": "project"
}
```

From this point, every agent in the repo sees the compressed Figma tool descriptions and the `figma_page_names` custom tool, without any per-user setup.
