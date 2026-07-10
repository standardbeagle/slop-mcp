# Resumable Debug Sessions Design

## Status

Approved design. Implementation not started.

## Goal

Make failed and deliberately paused SLOP programs editable, resumable data while preserving slop-mcp's fixed eight-tool surface. Users must be able to patch a failed API result, correct and re-execute a call, edit variables or executable program structure, and continue from the suspended point.

## Provenance and decisions

- Keep the eight meta-tools; extend `run_slop` with actions (`file:internal/server/tools.go:37`).
- Store canonical execution state in editable checkpoint files (user decision, 2026-07-10).
- Proactive breakpoints and pause controls are opt-in (user decision, 2026-07-10).
- Every uncaught runtime or API error creates a lightweight resumable checkpoint and includes a debug-session token in its error report (user decision, 2026-07-10).
- SLOP v0.3 already serializes scripts, positions, scopes, values, limits, transaction logs, emitted values, and control flow, but only checkpoints explicit top-level pauses (`file:$GOPATH/pkg/mod/github.com/standardbeagle/slop@v0.3.0/internal/evaluator/serialize.go:13`, `file:$GOPATH/pkg/mod/github.com/standardbeagle/slop@v0.3.0/internal/evaluator/checkpoint.go:241`).

## Public tool contract

`run_slop` gains an optional `action` field. Omitted action remains equivalent to `run`, preserving existing callers.

Actions:

- `run`: execute `script`, `file_path`, or `recipe`. `debug:true` enables breakpoints and deliberate pauses. Errors always return a debug token when checkpoint creation succeeds.
- `inspect`: return a bounded view of the checkpoint addressed by `debug_token`. The default view is a summary; callers request specific JSON-pointer paths for progressive disclosure.
- `patch`: apply validated patch operations to the checkpoint addressed by `debug_token` and create a new revision.
- `resume`: continue the checkpoint revision. It may complete, pause, or fail and issue a new token/revision.
- `abort`: revoke the token and delete the checkpoint family unless retention policy requires audit preservation.
- `list`: list resumable sessions using compact metadata only.

Input additions:

```json
{
  "action": "run|inspect|patch|resume|abort|list",
  "debug": false,
  "debug_token": "opaque capability",
  "revision": 3,
  "breakpoints": [
    {"line": 12},
    {"line": 20, "condition": "attempts > 2"}
  ],
  "paths": ["/failure", "/context/scopes", "/continuation"],
  "patches": [
    {"op": "replace", "path": "/failure/call/result", "value": {"type": "map", "data": {}}}
  ],
  "resume_mode": "continue|retry_call|accept_result"
}
```

Patch operations initially support `test`, `add`, `replace`, and `remove` over validated JSON pointers. `move` and `copy` are deferred because they complicate type and reference validation without enabling a core recovery flow.

## Responses and errors

Execution responses have a stable status envelope:

```json
{
  "status": "completed|paused|error|aborted",
  "result": null,
  "reason": "breakpoint|pause|error",
  "position": {"line": 12, "column": 5, "statement_index": 4},
  "debug_token": "opaque capability",
  "revision": 1,
  "expires_at": "RFC3339 timestamp",
  "failure": {
    "message": "upstream request failed",
    "kind": "service_call",
    "editable_paths": ["/failure/call", "/context/scopes", "/program"]
  }
}
```

An ordinary non-debug execution remains one-shot on success. On an uncaught error it writes the smallest complete resumable checkpoint and returns the same MCP tool-level error semantics as today, augmented with `debug_token`, revision, expiry, failure position, and suggested next actions. If checkpoint creation fails, the original error is still returned and includes a separate `debug_checkpoint_error`; failure to create debug state must never hide the program error.

## Debug tokens

Tokens are opaque bearer capabilities, not paths. A token identifies a checkpoint family, current revision, expiry, and random secret. The server stores only a hash of the secret in checkpoint metadata. Validation uses constant-time comparison.

- Default lifetime: 24 hours, configurable.
- Tokens are scoped to debug actions and one checkpoint family.
- Patch and resume require the expected revision; stale revisions return a conflict with current revision metadata.
- Resume rotates the token when it advances state. Abort revokes all outstanding tokens for the family.
- Tool responses never reveal the backing path by default. An explicit local administrative option may expose it.

## Checkpoint model

Checkpoint files are versioned JSON and remain the source of truth. Each revision is written atomically with restrictive permissions. A manifest points to the current revision only after the new file is durable.

Required state:

- Original source plus serialized executable program.
- Current source position and continuation/instruction cursor.
- Pending evaluator frames, including partially evaluated expressions.
- Scope chain and concrete serialized values.
- Service-call frame with service, method, arguments, result, and error.
- Transaction log, emitted values, limits, counters, and control-flow flags.
- Breakpoints, pause reason, failure metadata, token hash, revision, and expiry.

The checkpoint schema explicitly distinguishes executable state from diagnostic metadata so edits cannot accidentally treat messages as instructions.

## Continuation and error recovery

SLOP must checkpoint before an error unwinds evaluator frames. The continuation records what remains to be evaluated and where the failing value will be placed.

For a failed service/API call:

- `accept_result`: the user patches a concrete serialized result. Resume places it into the pending call expression and continues without invoking the service.
- `retry_call`: the user may edit service, method, arguments, or keyword arguments. Resume invokes the edited call once, records a new transaction entry, and continues if successful.
- `continue`: valid for breakpoint/manual pauses and resumes the next instruction without special failure handling.

Retries are never implicit. This avoids duplicating side effects. A call already recorded as successful cannot be re-executed without an explicit retry operation.

Program edits replace serialized nodes or source and must revalidate node types, references, and continuation compatibility. An incompatible edit is rejected with the exact invalid path. Large source rewrites may explicitly request restart from a chosen safe checkpoint; they are not silently coerced into a restart.

## Breakpoints and pausing

`debug:true` activates breakpoint checks. Initial breakpoint support is source line plus optional SLOP condition. The evaluator checks at statement boundaries and records the next continuation before returning. Existing language-level `pause` statements use the same checkpoint path and response envelope.

Breakpoint tables live in checkpoint state and are editable. Future step-in/over/out can be added as resume modes without adding MCP tools.

## Upstream SLOP changes

This cannot be implemented correctly as a slop-mcp wrapper around SLOP v0.3. The upstream public runtime needs:

1. A serializable continuation/frame representation.
2. Error interception before frame unwinding.
3. Checkpoint-on-error and breakpoint hooks.
4. Public load, validate, patch, and resume APIs operating on checkpoint data rather than only paths.
5. Resume modes for accepting a replacement result or retrying an edited call.
6. Checkpoint schema migration and compatibility validation.

slop-mcp then supplies file lifecycle, capability tokens, progressive inspection, MCP schemas, and response formatting. It must not duplicate the evaluator or import SLOP `internal` packages.

## Storage and lifecycle

- Default directory: the slop-mcp user data directory under `debug/checkpoints`; configurable for tests and deployments.
- Files and directories use owner-only permissions because state may contain credentials or API data.
- Atomic revision writes and a per-family file lock prevent lost updates across processes.
- Configurable TTL, total bytes, session count, and revisions per session bound disk use.
- Expired sessions are lazily collected and may also be swept at startup.
- `inspect` is size-bounded and path-selective; full state requires explicit paths or local file access.

## Validation and security

- Resolve all storage internally from token metadata; reject user-supplied checkpoint paths.
- Validate checkpoint version, revision, script/program consistency, serialized value types, frame references, JSON-pointer targets, and execution limits before commit.
- Redact configured secret headers and environment values from diagnostic summaries. Concrete program values remain editable but are returned only through a valid token.
- Preserve stdout exclusively for MCP JSON-RPC; debugging diagnostics use tool results or stderr logging.
- Patch failure is atomic and leaves the previous revision current.

## Testing strategy

- Backward compatibility tests for existing `run_slop` inputs and outputs.
- Red/green tests for explicit breakpoints, language pauses, automatic error tokens, inspect, patch, resume, abort, expiry, and stale revisions.
- Recovery tests for patched API results and edited/retried calls, asserting no unintended duplicate invocation.
- Nested-expression and function-frame continuation tests.
- Serialization round trips for every value, program node, frame, and checkpoint version.
- Concurrency and cross-process tests for simultaneous patch/resume.
- Path traversal, token guessing, token replay, redaction, oversized patch, corrupt checkpoint, and atomic-write failure tests.
- Race-enabled full suite plus mock-MCP integration coverage.

## Compatibility and rollout

The checkpoint schema receives a new major version because continuation frames change its meaning. Existing v1 explicit-pause checkpoints remain resumable through a migration adapter when they contain enough state; otherwise inspection explains why resume is unavailable.

The feature rolls out behind the new action fields with no additional MCP tool definitions. Automatic error checkpoints are enabled by default but bounded; an operator may disable persistence, in which case errors clearly report that no debug token could be issued.

## Deferred work

- Collaborative multi-writer editing beyond optimistic revision checks.
- Arbitrary instruction-pointer jumps.
- Time-travel reversal of external side effects.
- Graphical debugging UI.
- `move`/`copy` patch operations and advanced breakpoint types.
