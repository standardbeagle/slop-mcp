# LLM Builtin for SLOP — Design

**Date:** 2026-04-30
**Status:** Draft (pending user review)
**Owner:** andybrummer@standardbeagle.com

## Summary

Add an `llm()` builtin to the SLOP runtime in slop-mcp. SLOP scripts call language models without leaving the script. Backed by three pluggable providers in fall-through order: ACP (subprocess agent), GitHub Copilot, and any OpenAI-compatible HTTP endpoint. Named agents defined in KDL config bind a backend, model, system prompt, tool whitelist, and loop budget. The builtin runs a full agentic tool-use loop against slop-mcp's existing tool surface (registered MCPs + SLOP builtins), emits MCP progress notifications during streaming, and falls through to the next provider when one is unavailable.

MCP sampling was considered and dropped: insufficient client support across the agents that matter (most popular MCP clients do not implement the sampling capability) and the per-request user-consent flow is too heavy for an in-script call.

## Goals

- Single SLOP function `llm(prompt, ...kwargs)` returning final assistant text.
- Three backend providers behind one interface.
- Capability-based fall-through chain (no manual probing in user scripts).
- Named agents in KDL config; per-call kwargs override agent defaults.
- Agentic tool-use loop with `max_iters` budget.
- Streaming surfaced as MCP progress notifications during `run_slop` execution.
- KDL config + env-var fallback for keys/URLs.
- Reuse existing `auth_mcp` OAuth flow for Copilot tokens.

## Non-goals (v1)

- ACP-out (slop-mcp serving as an ACP agent): deferred to milestone 2.
- Parallel tool calls within a single iteration: sequential v1.
- Cross-call provider availability caching: re-evaluate per call.
- Streaming through the SLOP runtime as a generator/iterator: builtin stays blocking; streaming is observed only as progress notifications.
- Cost limits, budget enforcement, multi-tenant quotas.
- Embeddings, image input/output, audio.

## Architecture

### Package layout

```
internal/llm/
├── provider.go          # Provider interface, Request/Response/Message types
├── registry.go          # ProviderRegistry, AgentRegistry, Resolve()
├── chain.go             # Fall-through resolution
├── loop.go              # Agentic tool-use loop
└── providers/
    ├── acp.go           # ACP client (subprocess agent)
    ├── copilot.go       # GitHub Copilot (OpenAI-compatible w/ Copilot auth)
    └── openai.go        # OpenAI-compatible HTTP

internal/builtins/llm.go # rt.RegisterBuiltin("llm", ...) + llm_last() helper
internal/config/         # extend KDL: llm {} and agent {} blocks
```

### Wire-up

Both `cmd/slop-mcp/serve` and the `run` subcommand construct the same
`llm.Registry` at startup. The `llm` builtin closure captures the registry.
No active-session injection needed — none of the providers depend on the
MCP client session.

The `llm` builtin reaches into slop-mcp's existing tool surface to execute
tools requested by the model: it calls the same internal path used by
`execute_tool` for MCP tools, and the SLOP runtime directly for builtins.
This keeps the tool surface identical to what scripts already see.

## Provider interface (concept)

Each provider implements:

- `Name()` — stable identifier (`"acp"`, `"copilot"`, `"openai"`).
- `Available(ctx)` — capability check. ACP: subprocess spawned and handshake
  succeeded. Copilot: OAuth token present and unexpired. OpenAI: API key
  resolved.
- `Sample(ctx, req, onEvent)` — issue a request, return final response.
  `onEvent` is optional; nil means blocking, no streaming. Providers without
  native streaming synthesize one final delta event before completion — no
  fake token-by-token reconstruction.

The request carries messages (with role, content, tool calls, tool results),
optional system prompt, model name, tool specs (name + description +
JSON Schema), temperature, max_tokens, stop sequences. The response carries
final assistant content, any tool calls, stop reason, usage, and the actual
model used.

Exact Go signatures and types live in code; this document covers behavior.

## Fall-through chain

Configured in KDL as `chain "acp" "copilot" "openai"`.

- Triggered when an agent's backend is unset, OR when its bound backend's
  `Available()` returns false at call time.
- Walk in order; first `Available()=true` provider wins.
- Decision is per-call only. Capability state can change mid-session
  (e.g., an OAuth token refreshes, an ACP subprocess respawns). Caching
  across calls would mask these transitions.
- All three unavailable: return `ErrorValue` listing each provider's failure
  reason ("openai: OPENAI_API_KEY unset", …). SLOP `try`/`catch` recoverable.

## Agent registry + KDL config

```kdl
llm {
    chain "acp" "copilot" "openai"

    provider "openai" {
        base_url "https://api.openai.com/v1"
        api_key_env "OPENAI_API_KEY"
        default_model "gpt-4o-mini"
    }
    provider "copilot" {
        default_model "gpt-4o"
    }
    provider "acp" {
        command "zed-agent" "--stdio"
    }

    tool_timeout 60     // per-tool-call timeout in seconds
}

agent "reviewer" {
    backend "copilot"
    model "gpt-4o"
    system "You are a strict code reviewer..."
    tools "search_tools" "execute_tool"
    temperature 0.2
    max_iters 5
}

agent "summarizer" {
    backend "openai"
    model "gpt-4o-mini"
    system "Summarize concisely."
    max_iters 1
}
```

Agents are looked up by name. An agent with no `backend` runs the chain.
An agent with a `backend` runs that backend if available; otherwise falls
through to the chain.

### Three-tier merge

Existing pattern. User (`~/.config/slop-mcp/config.kdl`) → project
(`.slop-mcp.kdl`) → local (`.slop-mcp.local.kdl`). Agents merge by name —
later layer fully replaces earlier definition with the same name.

## llm() behavior

```
llm(prompt, agent: "name", system: "...", model: "...", tools: [...],
    temperature: 0.5, max_tokens: 1024, max_iters: 5,
    messages: [...])  // overrides prompt for multi-turn input
```

- Returns the final assistant content as a SLOP string.
- Resolution: if `agent` kwarg given, load definition; else use a synthetic
  default agent (no system prompt, chain backend, max_iters=5). Per-call
  kwargs override agent fields.
- `messages` kwarg, when present, replaces the auto-built `[user: prompt]`
  list. `prompt` may then be omitted.
- On stop reason `max_tokens` or `max_iters` exhausted: return `ErrorValue`
  with the reason. `try`/`catch` recoverable.
- Side-channel companion `llm_last()`: returns a map with `usage`, `model`,
  `iterations`, `stop_reason`, `provider` for the most recent `llm()` call
  in the current SLOP run. Avoids bloating the primary return shape.

## Tool-use loop

- Per iteration: send request → if `stop_reason == "tool_use"`, execute each
  tool call against slop-mcp's registry (MCP tools) or the SLOP runtime
  (builtins), append tool results as tool-role messages, loop.
- Tool surface for a given call: the agent's `tools` whitelist intersected
  with currently available tools. An empty whitelist means all available.
- Per-tool-call timeout from `llm.tool_timeout` (default 60s). Tool errors
  are fed back to the model as tool messages with an `is_error` flag in the
  payload — the model decides recovery vs. abort.
- Halt conditions: `stop_reason == "end_turn"`, iteration count reaches
  `max_iters`, context cancelled, provider returned a hard error.
- Tool calls execute sequentially in v1.
- Tool whitelist references that don't resolve at config load: warn and
  drop, do not fail. Otherwise an unrelated MCP being disconnected blocks
  valid agents.

## Progress notifications

In serve mode, the builtin is invoked from inside the `run_slop` tool
handler, which has a progress token under MCP's standard mechanism. The
builtin emits notifications:

- Token deltas batched (~50ms or 200 chars, whichever first).
- Tool-call announcements (`"calling X"`).
- Iteration boundaries.

In `run` subcommand, progress goes to stderr instead of MCP notifications.

Providers without native streaming emit one final delta — never simulate
token-by-token output.

## Error handling

- Provider errors wrap the underlying error with provider name and a retry
  hint flag. Network/5xx errors auto-retry once with exponential backoff.
- Auth errors (401/403) skip retry, mark provider unavailable for the
  remainder of the current call, fall through to the next.
- Agent not found: `ErrorValue` listing registered agents and similarity
  suggestions, reusing the existing pattern from `MCPNotFoundError` and
  friends.
- Tool execution timeout or error: surfaced to the model as a tool message
  with `is_error: true`. The loop continues unless the model issues
  `end_turn` in response.

## Security & secrets

- API keys never appear in MCP tool responses or SLOP error messages
  passed back through `run_slop`. Errors mention provider names and
  reason categories only.
- KDL `api_key` literals are supported but `api_key_env` is the
  recommended idiom; the example config uses env-var form.
- Copilot OAuth tokens stored via the existing `auth_mcp` token store; no
  new secret-storage code path.

## Testing

- Unit tests per provider with HTTP fakes (OpenAI), a fake ACP subprocess
  (round-trip JSON-RPC over a pipe), and a stub Copilot HTTP layer.
- Tool-loop tests: a scripted mock provider emits a deterministic sequence
  of `tool_use` and `end_turn` responses. Verify whitelist enforcement,
  `max_iters` halt, error feed-back, sequential ordering.
- Chain tests: providers with controlled `Available()` flags. Assert
  resolution order, skip-on-unavailable, error aggregation when all three
  are unavailable.
- Integration: real OpenAI-compatible endpoint (llama.cpp / ollama) behind
  the existing `integration` build tag plus `LLM_INTEGRATION_TEST=1`.
- The `llm()` builtin is tested through the actual SLOP runtime with a
  fake provider injected — no mocking at the SLOP/builtin boundary.

## Observability

- Structured logs (stderr only — stdout is reserved for MCP JSON-RPC):
  provider chosen, model, iteration count, tool-call names, latency,
  tokens (if reported), final stop reason.
- Logs at INFO for the high-level `llm()` summary line per call; DEBUG for
  per-iteration and per-tool-call detail.

## Milestones

1. **v1 (this spec):** providers (openai, copilot, acp-client),
   registry + KDL, builtin, tool-use loop, progress notifications, fall-
   through chain, tests.
2. **v2 (separate spec):** `slop-mcp acp-serve` subcommand. Listens on
   stdio or socket per ACP. Routes incoming requests through the same
   `Registry.Resolve` + loop. Reuses providers, agents, tool surface.

## Open questions

- ACP wire details (transport options beyond stdio, agent-discovery
  semantics, capability negotiation): addressed in v2 spec when
  ACP-out is designed.
- Copilot endpoint exact path and required headers: confirm against
  current Copilot API docs at implementation time. Provider impl is
  thin (HTTP + auth header), so churn is contained.
- ACP client minimal surface: which methods of the protocol are
  required for v1 (`prompt`, capability negotiation) vs. deferrable
  (cancellation, multi-message threading). Pin during impl after
  re-reading current ACP spec.

## File touch list (v1, expected)

- `internal/llm/` — new package (provider.go, registry.go, chain.go,
  loop.go, providers/*.go)
- `internal/builtins/llm.go` — new file, `RegisterLLM(rt)` entry point
- `internal/builtins/slop_reference.go` — add "llm" category entries
- `internal/config/config.go` (or sibling) — extend KDL parser for
  `llm {}` and `agent {}` blocks
- `cmd/slop-mcp/serve.go` and the `run` subcommand — construct registry,
  inject builtin into runtime
- `internal/server/server.go` — pass progress-emitter into builtin context
  during `run_slop` handler
- `go.mod` — Copilot: no official Go SDK exists; talk to Copilot's
  OpenAI-compatible chat endpoint directly over HTTP using the OAuth
  token from `auth_mcp`'s store (functionally an `openai` provider with
  Copilot-specific auth header injection). ACP: no Go SDK; implement
  minimal stdio/JSON-RPC client in-tree against the public ACP spec.
- `docs/` — usage page for `llm()` and `agent` config; `slop_reference`
  entries cover function signature

## Sketch — minimal usage

```slop
// Default agent, chain backend
result = llm("Summarize this PR diff: " + diff)

// Named agent
review = llm("Review this code:\n" + src, agent: "reviewer")
meta = llm_last()
log("reviewer used " + meta["model"] + " in " + meta["iterations"] + " iters")

// Per-call override
brief = llm("Summarize.", agent: "summarizer", max_tokens: 256)

// Multi-turn
answer = llm("",
    messages: [
        {role: "user", content: "What is X?"},
        {role: "assistant", content: "X is Y."},
        {role: "user", content: "Why?"},
    ],
    agent: "summarizer")
```
