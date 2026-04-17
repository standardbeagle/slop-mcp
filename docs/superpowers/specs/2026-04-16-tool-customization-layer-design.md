# Tool Customization Layer — Design

**Date:** 2026-04-16
**Status:** Draft, awaiting user review
**Author:** Brainstormed with Claude

## 1. Goals

Three capabilities delivered by a single new subsystem:

1. **Override** tool descriptions and per-parameter descriptions for any managed MCP tool, so agents can shrink verbose vendor documentation (the original motivating example: a Figma MCP with long schemas).
2. **Define custom tools** that are agent-authored, SLOP-backed, and surfaced to clients through the same search/execute path as native tools.
3. **Scoped and shareable** — user, project, and local tiers with import/export for packaging and distributing customizations across teams and machines.

### Non-Goals

- Tool renaming. Names pass through from upstream unchanged to keep agent learning stable.
- LLM automation inside slop-mcp. The agent drives all rewrites; slop-mcp stores and serves them.
- Replacing the existing KDL configuration. Customizations live in memory banks, not KDL.

## 2. Data Model

Customizations are stored in two reserved memory banks. The prefix `_slop.` is blocked from user `mem_save` writes; mutations go through a new meta-tool.

### `_slop.overrides`

Keyed by `<mcp>.<tool>`:

```json
{
  "description": "compressed override text",
  "params": { "query": "search str", "limit": "max rows" },
  "source_hash": "a3f9c12b8d7e4520",
  "scope": "project",
  "updated_at": "2026-04-16T10:30:00Z"
}
```

### `_slop.tools`

Keyed by custom tool name:

```json
{
  "description": "add multiple subtasks at once",
  "inputSchema": { "type": "object", "properties": {}, "required": [] },
  "body": "for t in $args.tasks { execute_tool(\"dart.create_task\", t) }",
  "depends_on": [
    { "mcp": "dart", "tool": "create_task", "hash": "9b1f..." }
  ],
  "scope": "user",
  "updated_at": "..."
}
```

### Hashing

`source_hash` is `SHA-256(description + "\n" + canonical_json(param_desc_map))`, truncated to the first 16 hex characters. Canonical JSON sorts keys and omits extra whitespace. The same rule applies per-entry in a custom tool's `depends_on` list so stale macros can be detected.

## 3. Storage Scopes

Scope applies only to reserved `_slop.*` banks. User banks (existing `mem_save`) are unchanged and remain at the user root.

| Scope   | Path                                      | Commit? |
|---------|-------------------------------------------|---------|
| user    | `~/.config/slop-mcp/memory/_slop/`        | no — machine-local |
| project | `<repo>/.slop-mcp/memory/_slop/`          | yes — shared via git |
| local   | `<repo>/.slop-mcp/memory.local/_slop/`    | no — gitignored |

The repo root is detected by walking up to `.git` or a marker file (`.slop-mcp.kdl`). If the process is not inside a repo, project and local scopes return an error on write.

### Read semantics

- Load all three scopes, merge per key with precedence Local > Project > User.
- The merged entry carries a `scope` field indicating which tier won.
- Base values outside `_slop.*` are read only from the existing user memory root.

### Write semantics

- The new meta-tool takes a `scope` argument (default `user`).
- Writes land in that scope's root using the existing atomic-write pattern.
- Deletes remove only from the specified scope.

The `.slop-mcp/memory/` and `.slop-mcp/memory.local/` directories are auto-created on first write. If the project scope is used and a `.gitignore` exists, `memory.local/` is appended to it.

## 4. The `customize_tools` Meta-Tool

A single new meta-tool. Total meta-tool count rises from 8 to 9. Actions dispatch via a required `action` field, mirroring the pattern in `manage_mcps`.

| Action            | Required args                                   | Optional args                                                      |
|-------------------|-------------------------------------------------|--------------------------------------------------------------------|
| `set_override`    | `mcp`, `tool`, `description`                    | `params` (map), `scope` (default `user`)                           |
| `remove_override` | `mcp`, `tool`                                   | `scope` (default all)                                              |
| `list_overrides`  | —                                               | `mcp`, `scope`, `stale_only` (bool)                                |
| `define_custom`   | `name`, `description`, `inputSchema`, `body`    | `scope`                                                            |
| `remove_custom`   | `name`                                          | `scope`                                                            |
| `list_custom`     | —                                               | `scope`, `stale_only`                                              |
| `export`          | —                                               | `mcp`, `keys` (glob), `include_custom` (default true), `scope`     |
| `import`          | `data` (JSON blob)                              | `scope` (dest, default `user`), `overwrite` (default false)        |

### Response shape

Always structured JSON: `{ok: true, action, affected: N, entries: [...]}` on success or `{ok: false, error: {...}}` on failure. List actions return full entries; mutating actions return summaries (keys + hashes, no bodies).

### Hashing behavior

`set_override` computes `source_hash` server-side by fetching the current upstream description and params. If the upstream MCP is not connected, the existing lazy-connect flow (cached-MCP) triggers. If the upstream is unreachable, the override is not saved and an error is returned.

`list_overrides` with `stale_only: true` recomputes upstream hashes and returns only entries whose stored hash differs, with both override and upstream text included so the agent can reconcile.

## 5. Integration With Existing Meta-Tools

### `search_tools`

- Results pass through the override layer. `description` is the override if present, otherwise the upstream value.
- The ranking index is rebuilt on override change (the existing rebuild path, triggered by a new event).
- Custom tools are indexed alongside native tools, with `mcp: "_custom"` as the marker.
- Stale overrides get `stale: true` and `stale_hint` on the result. The full `stale_source` payload is only returned by `get_metadata` to keep search responses lean.

### `get_metadata`

- Returns a merged view: `description` is the override if present; `inputSchema.properties[*].description` is merged from `override.params`.
- When an override applies: adds `override_scope`, `override_hash`, `stale` (bool).
- When stale: adds `stale_source: {description, params}` with upstream originals.
- For custom tools: adds `custom: true`; `body` is omitted unless `verbose: true`.

### `execute_tool`

1. If the requested `tool` name matches a custom tool (after scope merge), route to the SLOP engine. Args are validated against `inputSchema` first, then bound per Section 8.
2. Otherwise native MCP path, unchanged.
3. Custom tools that call themselves directly or transitively are bounded by a depth guard (default 16). Overflow returns a recursion error.

### `manage_mcps`

Gains a `list_stale_overrides` subaction as an ergonomic shortcut; it delegates to the same code path as `customize_tools list_overrides stale_only:true`.

No changes to `run_slop`, `auth_mcp`, `slop_reference`, or `slop_help`.

## 6. Import / Export

### Pack format

```json
{
  "schema_version": 1,
  "exported_at": "2026-04-16T10:30:00Z",
  "source": "slop-mcp v0.14.0",
  "selector": { "mcp": "figma" },
  "overrides": [
    {
      "key": "figma.get_file",
      "description": "...",
      "params": {},
      "source_hash": "a3f9c12b8d7e4520"
    }
  ],
  "custom_tools": [
    {
      "name": "figma_quick_extract",
      "description": "...",
      "inputSchema": {},
      "body": "...",
      "depends_on": [{ "mcp": "figma", "tool": "get_file", "hash": "..." }]
    }
  ]
}
```

`scope` and `updated_at` are not in the pack. The recipient picks the scope on import; timestamps are local telemetry.

### Selector precedence

An explicit `keys` glob overrides the `mcp` filter. `mcp: "figma"` expands to `keys: ["figma.*"]` for overrides plus any custom tool whose `depends_on` references figma.

### Import validation

- Unknown `schema_version` is rejected with a forward-compat hint.
- `source_hash` is recomputed on import against the current upstream; mismatches are flagged in the response (`imported_stale: []`) but still stored.
- If `overwrite: false` and a key exists at the destination scope, the collision is collected, skipped, and reported.
- Custom tool name collisions follow the same rule.
- Atomic per-bank: all writes for one bank succeed or none (write tmpfile, rename).

### Wire format

JSON string passed as the `data` argument for `import`. `export` returns the same JSON as its tool result. Packs are file-agnostic — saving or loading files is the agent's job via existing filesystem tools.

## 7. Slop-MCP's Own Tool Documentation

The feature's motivating style is applied to slop-mcp's own meta-tool descriptions too — dogfooding the pattern.

- Rewrite descriptions in `internal/server/schemas.go` and tool definitions in `internal/server/tools.go`.
- Use caveman-full style: drop articles and filler, fragments permitted, technical terms exact, examples preserved.
- Keep `inputSchema` parameter descriptions concise but complete (enum values listed, types precise).
- Before: *"Search for tools across all connected MCP servers using fuzzy matching. Returns ranked results with relevance scores. Use this to discover what capabilities are available before calling execute_tool."*
- After: *"Fuzzy search tools across connected MCPs. Returns ranked results. Discover capabilities before execute_tool."*

A new `docs/internal/description-style.md` captures the rules as a developer reference so future tool additions stay consistent. This is not user-facing.

## 8. Custom Tool Execution

### Validation pipeline

1. Resolve the custom tool by name across scopes (Local > Project > User).
2. Validate args against the stored `inputSchema` using the existing schema validator (same path as native tool param checks in `registry.go`).
3. Build the binding context:
   - `$args` holds the full args map.
   - For each top-level arg key, if the name does not collide with a SLOP builtin, bind `$<key>` as a shorthand.
   - The builtin collision list lives in a new `internal/builtins/reserved.go`.
4. Pass to the run_slop engine with a recursion guard (depth counter threaded through context).

### Runtime context

- All existing SLOP builtins are available: `execute_tool`, `mem_*`, `store_*`, HTTP, loops.
- No sandbox — full access, per the agent-driven model.
- `execute_tool` call chains are tracked via a context-keyed counter; depth greater than 16 returns `ErrCustomToolRecursion`.

### Output

- The last expression value of the body is the tool result.
- Converted via `slop.ValueToGo` to a JSON-serializable form.
- SLOP parser and runtime errors are returned as MCP tool errors with the `parseSlopError` JSON shape (the v0.11.0 structured-error pattern).

### Concurrency

- Custom tools are reentrant; parallel calls to the same tool run in independent contexts.
- Shared state flows through `store_*` (session) or `mem_*` (persistent).

### Performance

- Bodies are parsed once per process; ASTs are cached by `(name, scope, body_hash)`.
- The cache entry for a name is invalidated on `define_custom` / `remove_custom`.

## 9. Concurrency Model

Mutexes guard in-memory state only. I/O and external calls happen without locks held.

### Override writes

1. Fetch upstream tool metadata — no lock held (may hit network).
2. Compute hash — pure CPU, no lock.
3. Marshal the entry — no lock.
4. Acquire a brief mutex on the in-memory bank map: copy the entry in, mark the bank dirty, release.
5. A background per-bank flusher picks up the dirty mark, takes a brief mutex to snapshot the map, releases, then writes `<bank>.json.tmp` and renames. No lock is held during the write.

### Per-bank flusher

- One goroutine per loaded bank, spawned on first write.
- An unbuffered channel signals "dirty"; bursts are coalesced (drain before snapshot).
- Atomic rename is the OS-level sync primitive — the file on disk is always coherent.
- On shutdown, pending writes flush.

### Ranking index

- `atomic.Pointer[ToolIndex]` on the registry.
- Rebuild takes a brief read lock to snapshot the tool set, releases, then builds a new index lock-free.
- The pointer is swapped atomically. Readers load once per query and use the snapshot for the duration — no contention.

### Upstream hash staleness check

- `list_overrides stale_only: true` fans out reads across connected MCPs; no locks held during those reads.
- Per-MCP reads use the existing cached tool list unless `refresh: true`.

### Race behavior

- Two concurrent `set_override` on the same key: last in-memory write wins; the flusher persists the final state. No lost entries (RMW is atomic in-memory).
- A concurrent read during a write sees either the pre- or post-state atomically — no torn reads.

## 10. Security and Edge Cases

### Reserved bank protection

- `_slop.*` prefix blocked in `mem_save`, `mem_delete`, `mem_clear`; error: `"bank '_slop.overrides' reserved; use customize_tools"`.
- `mem_list`, `mem_search`, `mem_info`, `mem_load` remain read-accessible (introspection is fine).
- Prefix check in `internal/builtins/memory.go` at entry of each mutating op.
- memory-cli mirrors the check.

### Schema validation

- `define_custom` validates the supplied `inputSchema` is a well-formed JSON Schema (the draft-07 subset already supported).
- `$ref` values pointing outside self are rejected (no remote refs).
- Body size limit: 64 KB, configurable via `SLOP_MAX_CUSTOM_BODY`.

### Name collision rules

- Custom tool names must not collide with existing meta-tool names.
- Custom tool names must not match `<mcp>.<tool>` for a real connected MCP tool.
- Regex: `^[a-z][a-z0-9_]{0,63}$`.

### Scope precedence edges

- If a key exists at multiple scopes, Local wins at read time, but `list_overrides` with no scope filter returns all entries flagged with their scope so the agent can reconcile.
- Deleting at scope X never touches other scopes.

### Failure modes

- Upstream MCP disconnected at override-set time: error, override not saved (hash required).
- Import pack referencing an uninstalled MCP: stored but marked `missing_deps: ["figma"]` in list responses.

## 11. Testing

### Unit tests (new package `internal/overrides/`)

- Hash determinism and canonical JSON ordering.
- Scope merge precedence (Local > Project > User).
- Reserved bank prefix rejection in `mem_save`.
- Custom tool `inputSchema` validation against valid and invalid cases.
- Name collision rules.
- Import/export round-trip preserves entries and recomputes hashes.
- Per-bank flusher coalescing, atomic rename, and shutdown flush of pending writes.

### Integration tests (`internal/server/` with mock-mcp)

- All eight `customize_tools` actions via MCP JSON-RPC.
- `search_tools` reflects the override description.
- `get_metadata` returns the override and `stale: true` when upstream diverges.
- `execute_tool` routes a custom tool through the SLOP engine with args validated.
- Recursion depth guard fires.
- Custom tool calling a native tool end-to-end.

### Concurrency tests

- Parallel `set_override` on the same key: final state deterministic, no lost entries.
- Flusher under load: 1,000 rapid writes coalesce to a reasonable file-write count.
- Index snapshot readers do not block writers.

## 12. Migration and Rollout

- Feature is additive; no existing data to migrate.
- Audit confirmed no existing user bank starts with `_slop.`, so the new reserved-prefix check is safe.
- Bump `serverVersion` in `internal/server/server.go` to 0.14.0.
- CHANGELOG entry covering overrides, custom tools, import/export, and dogfooded description compression.
- New docs page at `docs/docs/concepts/customization.md` with a Figma example.
- Single PR delivering slop-mcp's own-tool wenyan rewrite and the new feature together.
- No feature flag; reserved banks are simply empty until first use.
