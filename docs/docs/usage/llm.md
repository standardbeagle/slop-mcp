---
sidebar_position: 1
---

# `llm()` — calling language models from SLOP

The `llm()` builtin lets SLOP scripts call language models inline, without
leaving the script. It runs a full agentic tool-use loop, feeds results back
to the model when it issues tool calls, and returns the final assistant text
as a SLOP string. Three provider backends are supported — OpenAI-compatible
HTTP endpoints, GitHub Copilot, and ACP (Agent Client Protocol) subprocesses
— with automatic fall-through when a backend is unavailable.

---

## Quick start

**1. Configure a provider in KDL:**

```kdl
// ~/.config/slop-mcp/config.kdl
llm {
    chain "openai"

    provider "openai" {
        base_url      "https://api.openai.com/v1"
        api_key_env   "OPENAI_API_KEY"
        default_model "gpt-4o-mini"
    }
}
```

**2. Call `llm()` from a SLOP script:**

```python
result = llm("Summarize this in one sentence: " + text)
emit(result)
```

**3. Run via slop-mcp:**

```bash
OPENAI_API_KEY=sk-... slop-mcp run script.slop
```

---

## Function reference

### `llm(prompt, ...kwargs)`

Calls a language model and returns the final assistant response as a string.

**Positional argument:**

| Argument | Type   | Description |
|----------|--------|-------------|
| `prompt` | string | The user message. Omit when supplying `messages` directly. |

**Keyword arguments:**

| Kwarg         | Type              | Default        | Description |
|---------------|-------------------|----------------|-------------|
| `agent`       | string            | chain default  | Named agent from config. If omitted, uses a synthetic default agent with no system prompt, chain backend, and `max_iters` 5. |
| `system`      | string            | `""`           | System prompt. Overrides the agent's configured system prompt for this call. |
| `model`       | string            | agent default  | Model name sent to the backend. Overrides the agent's `model` field. |
| `tools`       | list of strings   | agent default  | Tool whitelist for this call. Overrides the agent's `tools` list. |
| `temperature` | number            | agent default  | Sampling temperature (0–2). |
| `max_tokens`  | integer           | agent default  | Hard token limit for a single completion. |
| `max_iters`   | integer           | 5              | Maximum tool-use iterations. Overrides the agent's `max_iters`. |
| `messages`    | list of maps      | `[{role:"user", content: prompt}]` | Full message list for multi-turn conversations. When present, `prompt` is ignored. Each map must have `"role"` and `"content"` string keys. |

**Return value:** `string` — the final assistant content.

**On error:** returns `ErrorValue` (catchable with `try`/`catch`). Errors include:
- Provider unavailable (all chain members failed).
- Unknown `agent` name (lists registered agents).
- `max_iters` exceeded.
- `max_tokens` stop reason from the model.

---

### `llm_last()`

Returns metadata from the most recent `llm()` call in the current SLOP run.
Returns `null` if no `llm()` call has been made yet.

**Return value:** map with these keys:

| Key          | Type    | Description |
|--------------|---------|-------------|
| `model`      | string  | Model name as reported by the backend. |
| `provider`   | string  | Backend that served the call (`"openai"`, `"copilot"`, `"acp"`). |
| `iterations` | integer | Number of tool-use iterations completed (minimum 1). |
| `stop_reason`| string  | Final stop reason: `"end_turn"`, `"tool_use"`, `"max_tokens"`, or `"stop_sequence"`. |
| `usage`      | map     | Token usage: `{"input_tokens": N, "output_tokens": N}`. Accumulated across all iterations. |

`llm_last()` reflects the completed call even when `llm()` returned an
`ErrorValue` (e.g. `max_iters` exceeded). It is per-`run_slop` invocation —
state does not persist between separate tool calls.

---

## Configuration reference

### The `llm {}` block

```kdl
llm {
    chain "acp" "copilot" "openai"

    provider "openai" {
        base_url      "https://api.openai.com/v1"
        api_key_env   "OPENAI_API_KEY"
        default_model "gpt-4o-mini"
    }

    provider "copilot" {
        default_model "gpt-4o"
    }

    provider "acp" {
        command "my-agent" "--stdio"
    }

    tool_timeout 60
}
```

| Field          | Type            | Description |
|----------------|-----------------|-------------|
| `chain`        | list of strings | Ordered fall-through chain. Provider names match the `provider "name"` keys. |
| `tool_timeout` | integer (seconds)| Per-tool-call timeout during the agentic loop. Default: 60. |

#### Provider: `openai`

Connects to any OpenAI-compatible chat-completions endpoint.

| Field           | Required | Description |
|-----------------|----------|-------------|
| `base_url`      | yes      | Base URL, e.g. `"https://api.openai.com/v1"` or a local server. |
| `api_key_env`   | recommended | Name of the environment variable holding the API key. Prefer over `api_key`. |
| `api_key`       | alternative | Literal API key. Avoid committing to version control. |
| `default_model` | yes      | Model used when no agent or per-call `model` kwarg specifies one. |

The provider is considered available when an API key (via `api_key_env` or
`api_key`) resolves to a non-empty string at call time.

#### Provider: `copilot`

Connects to `api.githubcopilot.com` using OAuth tokens managed by `auth_mcp`.

| Field           | Required | Description |
|-----------------|----------|-------------|
| `default_model` | yes      | Model used when none is specified. |

The Copilot provider uses the OAuth token from slop-mcp's token store (same
storage as `auth_mcp login copilot`). The provider is available only when a
valid token is present. **v1 note:** the integration ID sent is `vscode-chat`
(VS Code's Copilot extension value); confirm this remains valid against the
Copilot API at your release date. If Copilot is not yet authenticated, run
`auth_mcp login copilot` through the `auth_mcp` meta-tool before invoking
SLOP scripts that need it.

#### Provider: `acp`

Connects to an Agent Client Protocol subprocess over stdio JSON-RPC.

| Field     | Required | Description |
|-----------|----------|-------------|
| `command` | yes (v1) | Subprocess argv, e.g. `"my-agent" "--stdio"`. |
| `url`     | no       | Placeholder for future socket transport (not active in v1). |

The provider is available when `command` is set. The subprocess is spawned
lazily on the first call; `Available()` does not start it. The ACP wire format
follows the April 2026 draft and may change when the spec stabilizes.

### Three-tier config merge

Config files are loaded and merged in priority order:

| File | Scope | Notes |
|------|-------|-------|
| `~/.config/slop-mcp/config.kdl` | User | Baseline; personal cross-project defaults. |
| `.slop-mcp.kdl` | Project | Shared; committed to the repo. |
| `.slop-mcp.local.kdl` | Local | Personal overrides; gitignored. |

Later layers win. For `llm {}` specifically: providers are merged by name
(later layer replaces the same-named provider wholesale); `chain` is only
replaced when the later layer specifies a non-empty chain; `tool_timeout`
follows last-writer-wins.

---

### The `agent {}` block

Define named agents for reuse across scripts:

```kdl
agent "reviewer" {
    backend     "copilot"
    model       "gpt-4o"
    system      "You are a strict code reviewer. Be concise."
    tools       "search_tools" "execute_tool"
    temperature 0.2
    max_iters   5
}

agent "summarizer" {
    backend    "openai"
    model      "gpt-4o-mini"
    system     "Summarize concisely."
    max_iters  1
}
```

| Field         | Type              | Description |
|---------------|-------------------|-------------|
| `backend`     | string            | Provider name (`"openai"`, `"copilot"`, `"acp"`). If omitted or the named provider is unavailable, the fall-through chain is used. |
| `model`       | string            | Model override for this agent. Overrides the provider's `default_model`. |
| `system`      | string            | System prompt injected on every call. |
| `tools`       | list of strings   | Tool whitelist. `nil` / omitted means all available tools. |
| `temperature` | number            | Sampling temperature. |
| `max_tokens`  | integer           | Per-completion token limit. |
| `max_iters`   | integer           | Maximum tool-use iterations (default 5). |

Agents are merged by name across config tiers; a later layer's definition
fully replaces an earlier one with the same name.

---

## Fall-through chain semantics

The chain defines the order in which providers are probed when no specific
backend is bound, or when the bound backend is unavailable.

Resolution per call:

1. If the call specifies `agent: "name"` and that agent has a `backend` set,
   check that backend's `Available()` first.
2. If available, use it. Done.
3. Otherwise (no agent, no backend, or backend unavailable), walk the `chain`
   in declared order.
4. First provider where `Available()` returns true wins.
5. If all providers in the chain are unavailable, `llm()` returns an
   `ErrorValue` whose message lists each provider's unavailability reason, e.g.:
   `no llm backend available: acp: no command configured; copilot: no oauth token; openai: OPENAI_API_KEY unset`

Availability is re-evaluated per call. An OAuth token refreshing mid-session
or a new env var picking up will cause the next call to succeed without
restarting slop-mcp.

---

## Tool-use loop

When an agent has access to tools, the model may call them. The loop:

1. Send the current message list to the model.
2. If `stop_reason == "tool_use"`, execute each tool call sequentially against
   slop-mcp's registered MCP tools and SLOP builtins.
3. Append the assistant turn (with tool calls) and tool results as messages.
4. Repeat until `stop_reason == "end_turn"`, `max_iters` is exhausted, or the
   context is cancelled.

**Tool surface:** the agent's `tools` whitelist (if set) is intersected with
tools currently registered in slop-mcp. An empty whitelist means all available
tools are offered. Whitelist entries that don't resolve at call time are
silently dropped (a disconnected MCP won't break unrelated agents).

**Tool errors:** when a tool runs but reports an error (MCP `isError` flag),
the error text is fed back to the model as a tool-role message marked as
`is_error`. The model decides whether to retry, use an alternative, or stop.
Infrastructure failures (tool not found, timeout) abort the loop immediately.

**Tool calls are sequential** in v1. Parallel execution is a v2 goal.

**`llm()` from `slop-mcp run` or `monitor`** uses a `NoopHost` — no MCP tools
are available to the model in those contexts. Only the `serve` path (via
`run_slop` meta-tool) and custom-tool invocations provide a live tool surface.
If the model attempts a tool call through `NoopHost`, the loop aborts with an
infrastructure error.

---

## Examples

### Default agent (chain-resolved)

```python
result = llm("What is the capital of France?")
emit(result)
```

No agent named, no backend configured — walks the chain and uses the first
available provider.

---

### Named agent

```python
review = llm("Review this function:\n" + source_code, agent: "reviewer")
emit(review)
```

---

### Per-call kwarg override

```python
// Override max_tokens for a tight summary budget
brief = llm("Summarize in one sentence.", agent: "summarizer", max_tokens: 80)
emit(brief)
```

---

### Multi-turn `messages` kwarg

```python
answer = llm("",
    messages: [
        {"role": "user",      "content": "What is X?"},
        {"role": "assistant", "content": "X is Y because Z."},
        {"role": "user",      "content": "Can you give an example?"},
    ],
    agent: "summarizer")
emit(answer)
```

When `messages` is supplied, `prompt` is ignored.

---

### Inspecting `llm_last()`

```python
result = llm("Explain async/await.", agent: "reviewer")
meta   = llm_last()

emit(format("model: {}, provider: {}, iters: {}",
    meta["model"],
    meta["provider"],
    meta["iterations"]))

usage = meta["usage"]
emit(format("tokens in/out: {}/{}",
    usage["input_tokens"],
    usage["output_tokens"]))
```

---

### Error handling

```python
result = try(llm("Do something.", agent: "reviewer"))
if is_error(result) {
    emit("LLM call failed: " + error_message(result))
} else {
    emit(result)
}
```

Common error messages:
- `no llm backend available: ...` — all chain members unavailable.
- `unknown agent "name" (registered: ...)` — typo in agent name.
- `max_iters exceeded (5)` — tool-use loop hit the iteration budget.

---

## Notes and limitations (v1)

- **Streaming:** the builtin emits MCP progress notifications during `run_slop`
  execution (token deltas, tool-call announcements, iteration boundaries).
  In v1, only OpenAI natively streams. Copilot and ACP synthesize a single
  final delta before `Done`. True token-streaming through the SLOP runtime
  as an iterator is deferred to v2.

- **ACP wire format:** follows the April 2026 draft. If the ACP spec changes
  before stabilization, the provider implementation will need updating. The
  ACP provider is thin so churn is contained.

- **Copilot integration ID:** the provider sends `Copilot-Integration-Id: vscode-chat`
  (VS Code's value). Verify this header against current Copilot API docs
  before shipping to users.

- **Sequential tool calls:** within a single iteration, tool calls execute
  one at a time. Parallel execution is a v2 goal.

- **No MCP tools in `run` / `monitor`:** `llm()` from `slop-mcp run` or the
  monitor subcommand runs with `NoopHost` — no MCP tools are exposed to the
  model. Use the `run_slop` meta-tool via an agent (serve mode) for agentic
  tool-use workflows.

- **Copilot auth:** authenticate first with `auth_mcp login copilot` through
  the `auth_mcp` meta-tool. The Copilot provider reads from slop-mcp's OAuth
  token store; no separate configuration step is needed beyond that.

- **API key security:** API keys never appear in MCP tool responses or SLOP
  error messages. Errors name the provider and reason category only.
