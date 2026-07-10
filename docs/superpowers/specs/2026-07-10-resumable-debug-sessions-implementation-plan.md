# Resumable Debug Sessions Implementation Plan

Design: [2026-07-10-resumable-debug-sessions-design.md](./2026-07-10-resumable-debug-sessions-design.md)

## Delivery strategy

Deliver in two coordinated repositories. The SLOP interpreter owns continuations, failure capture, checkpoint validation, and resume semantics. slop-mcp owns capability tokens, durable revision storage, progressive inspection, MCP schemas, and user-facing error envelopes.

Do not copy SLOP internals into slop-mcp. Do not add a ninth MCP tool.

## Phase 1: upstream SLOP checkpoint model

### 1. Define the versioned execution-state schema

In the upstream `github.com/standardbeagle/slop` repository:

- Add public serialized types for executable program nodes, continuation frames, pending service calls, pause/failure reason, breakpoints, and checkpoint revisions.
- Retain existing serialized context, scope, value, transaction, limit, emitted-value, and control-flow types.
- Separate diagnostic failure metadata from executable continuation state.
- Give the new format a new major checkpoint version.
- Add migration detection for v1 explicit-pause checkpoints.

Tests first:

- Round-trip every program node and continuation-frame variant.
- Round-trip nested function, loop, conditional, pipeline, and service-call execution states.
- Reject unknown versions, dangling frame references, invalid value types, and incompatible program/continuation pairs.

Verification:

```bash
go test ./internal/evaluator/... ./pkg/slop/...
go test -race ./internal/evaluator/... ./pkg/slop/...
```

### 2. Refactor evaluation into explicit resumable frames

- Represent partially evaluated statements and expressions as concrete frames rather than relying solely on the Go call stack.
- Maintain an explicit continuation stack in evaluator context.
- Preserve existing `Execute`, `Eval`, and non-checkpoint behavior.
- Resume a continuation without replaying already completed expressions.

TDD scenarios:

- Pause within nested arithmetic and collection expressions.
- Pause within a function called from a loop.
- Resume after condition evaluation but before branch execution.
- Resume pipelines without repeating completed transforms.
- Confirm successful service calls are not repeated.

### 3. Capture resumable failures before unwinding

- Intercept uncaught evaluator and service errors at the frame boundary.
- Serialize the full continuation before frames unwind.
- Record failure kind, source position, pending destination, and editable paths.
- Preserve the original error even when checkpoint persistence fails.

TDD scenarios:

- Undefined variable and type errors.
- Service transport, protocol, and tool-level errors.
- Errors nested inside functions, loops, arguments, and pipelines.
- Checkpoint-write failure returns both original error and checkpoint error.

### 4. Add result substitution and explicit call retry

Add public resume modes:

- `continue`: resume a deliberate pause or breakpoint.
- `accept_result`: validate a replacement value, place it at the failed call's pending destination, and continue without a service invocation.
- `retry_call`: validate the edited call frame, invoke it exactly once, and continue on success.

Tests must count service calls and prove that substitution performs zero retries and explicit retry performs exactly one.

### 5. Add breakpoint hooks

- Add line breakpoints and optional SLOP conditions.
- Check at statement boundaries before execution.
- Serialize the next continuation before reporting a breakpoint.
- Route language `pause` statements through the same checkpoint result.

### 6. Publish the upstream API

Expose public APIs that operate on typed checkpoint data rather than forcing callers to edit files:

```go
ExecuteResumable(ctx, source, options) (ExecutionResult, error)
Resume(ctx, checkpoint, options) (ExecutionResult, error)
ValidateCheckpoint(checkpoint) error
ApplyCheckpointPatch(checkpoint, operations) (*Checkpoint, error)
```

`ExecutionResult` distinguishes `completed`, `paused`, and `failed`, and carries checkpoint data when resumable.

Run full upstream formatting, unit, race, and integration suites. Release a tagged SLOP version before beginning slop-mcp integration.

## Phase 2: slop-mcp durable debug store

### 7. Upgrade the SLOP dependency

Files:

- `go.mod`
- `go.sum`
- `internal/builtins/runtime.go`

Upgrade to the tagged SLOP release. Add thin construction helpers only; do not expose upstream internal types across unrelated packages.

### 8. Add debug configuration

Create `internal/debug/config.go` with defaults and environment/config parsing for:

- Checkpoint directory.
- Token TTL, default 24 hours.
- Maximum sessions, revisions per session, total bytes, and inspect response bytes.
- Automatic error checkpoint enable/disable switch.

Reject unsafe or nonsensical values. Resolve the default beneath the slop-mcp user data directory.

### 9. Implement checkpoint-family storage

Create:

- `internal/debug/store.go`
- `internal/debug/manifest.go`
- `internal/debug/store_test.go`

Responsibilities:

- Owner-only directories and files.
- Random checkpoint-family IDs.
- Immutable revision files plus an atomically updated current manifest.
- Per-family cross-process locks using `internal/filelock`.
- Atomic writes using `internal/atomicfile`.
- Expected-revision conflict checks.
- TTL and quota enforcement.
- Lazy expiry cleanup and startup sweep.
- Abort/revoke/delete behavior.

Test concurrent processes and injected write failures. A failed new revision must leave the prior manifest and revision readable.

### 10. Implement opaque capability tokens

Create:

- `internal/debug/token.go`
- `internal/debug/token_test.go`

Use cryptographically random bearer secrets. Persist only a hash with family ID, revision, expiry, and revocation metadata. Validate with constant-time comparison.

Tests:

- Valid, expired, malformed, guessed, revoked, rotated, and stale-revision tokens.
- Token never contains a checkpoint path.
- Resume rotation invalidates the prior write capability.

### 11. Implement validated patch and inspect services

Create:

- `internal/debug/patch.go`
- `internal/debug/inspect.go`

Patch supports `test`, `add`, `replace`, and `remove`. Parse JSON pointers strictly, block metadata/token mutation, delegate executable-state validation to upstream SLOP, and commit only fully valid revisions.

Inspect defaults to compact metadata and accepts explicit paths. Enforce response-size limits and return truncation/continuation hints rather than dumping the full checkpoint.

## Phase 3: extend `run_slop`

### 12. Extend input and output types

Files:

- `internal/server/handlers.go`
- `internal/server/schemas.go`
- `internal/server/tools.go`

Add typed fields for action, debug token, revision, breakpoints, paths, patches, and resume mode. Keep omitted `action` equivalent to the current run behavior.

Update the hand-written JSON schema with enums and conditional requirements while keeping the description compact.

### 13. Split action dispatch from execution

Create `internal/server/debug_handler.go` and keep `handleRunSlop` as the compatibility entry point.

Dispatch:

- `run`: existing behavior unless `debug:true`; on any uncaught error, create a lightweight error checkpoint when enabled.
- `inspect`: authorize token and return selected views.
- `patch`: authorize, validate expected revision, apply patch, issue rotated token.
- `resume`: authorize, delegate to SLOP, persist the next state, and rotate token.
- `abort`: revoke and delete.
- `list`: return compact session metadata without secrets or paths.

Business failures remain MCP tool-level errors through `errorResult`; malformed requests remain protocol errors under existing conventions.

### 14. Add error-token envelopes

Extend structured SLOP errors with:

- `debug_token`
- `revision`
- `expires_at`
- `position`
- `failure_kind`
- `editable_paths`
- Suggested `run_slop` actions.

If automatic checkpointing fails, return the original error plus `debug_checkpoint_error`. Never replace or obscure the root error.

### 15. Integrate server lifecycle

Files:

- `internal/server/server.go`
- `internal/server/slop_runtime.go`

Initialize one debug store per server. Sweep expired sessions without delaying startup. Close any maintenance goroutine during server shutdown. Keep checkpoint operations independent of the global runtime execution lock where safe.

## Phase 4: acceptance tests and documentation

### 16. Add server-level regression tests

Cover:

- Existing `run_slop` calls remain byte/semantics compatible where documented.
- `debug:true` line breakpoint → inspect → patch variable → resume.
- Language pause → resume.
- Ordinary non-debug API error returns a debug token.
- Patch failed API result → `accept_result` → completion with zero duplicate calls.
- Edit wrong method → `retry_call` → completion with exactly one retry.
- Nested-expression failure resumes at the pending frame.
- Stale revision, expiry, abort, corrupt file, quota, and token replay failures.
- Concurrent patch/resume conflict behavior.
- No checkpoint paths or configured secrets leak through summaries.

### 17. Add mock-MCP end-to-end tests

Extend the mock MCP with deterministic failing and counted tools. Verify the complete MCP request/response flow using the `run_slop` meta-tool only.

### 18. Update user documentation

Files:

- `docs/docs/reference/tools.md`
- `docs/docs/reference/slop-language.md`
- `CHANGELOG.md` at release time

Document the progressive workflow with compact examples:

1. Receive an error and debug token.
2. Inspect only `/failure/call`.
3. Patch a result or call.
4. Resume with an explicit mode.
5. Abort when finished.

Warn that tokens expose potentially sensitive execution state and that retrying calls can repeat side effects.

## Quality gates

After each phase, run focused tests. Before completion run fresh full verification:

```bash
make fmt
make lint
make build
make test
make test-mock
go test -race -short ./...
git diff --check
```

Then run independent correctness and maintainability reviews. Fix every actionable finding and repeat the affected gates until both reviews are clean.

## Current blocker

The upstream SLOP repository is not available as an editable checkout in this workspace. Its module-cache copy is immutable and must not be modified. Phases 7–18 are blocked until Phases 1–6 are implemented and released upstream, or an editable upstream checkout is added to the workspace and a temporary `go.work`/`replace` development link is explicitly authorized.
