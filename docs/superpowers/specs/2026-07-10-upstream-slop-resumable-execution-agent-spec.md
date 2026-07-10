# Agent Spec: Add Editable, Resumable Execution to SLOP

Implement this feature in the `github.com/standardbeagle/slop` repository. Work only in the SLOP repository. Do not implement slop-mcp storage, tokens, or MCP tools.

## Objective

Make every active SLOP execution state concrete, serializable, editable, validatable, and resumable. A caller must be able to suspend at a breakpoint or uncaught error, edit the serialized state, and continue without replaying completed work.

The core recovery cases are:

1. An API/service call fails. The caller supplies a concrete replacement result and continues after the call without invoking it again.
2. An API/service call is wrong. The caller edits its service, method, or arguments, explicitly retries it once, and continues.
3. The caller edits variables, scopes, breakpoints, or executable program nodes and resumes when the edited state remains compatible with the pending continuation.
4. A breakpoint or language `pause` statement suspends execution and returns the same checkpoint/result shape used for errors.

## Existing behavior to preserve

SLOP v0.3 has:

- `Runtime.ExecuteWithCheckpoints` and `Runtime.ResumeFromCheckpoint`.
- Explicit `pause` statements.
- Serializable values, scopes, limits, transaction logs, emitted values, control flow, scripts, and top-level statement positions.
- A v1 checkpoint JSON format.

Preserve existing `Execute`, `Eval`, pause, and checkpoint behavior for compatible callers. Existing non-resumable execution must not become slower or semantically different without a clear reason and benchmark evidence.

The current evaluator resumes only at top-level statement boundaries and relies on the Go call stack for nested evaluation. That is insufficient. Do not simulate continuation by restarting the script or replaying a whole statement.

## Required semantics

### Explicit continuation state

Represent active evaluation with concrete continuation frames rather than relying only on the Go call stack. The serialized continuation must identify:

- The active statement or expression node.
- The current evaluation phase within that node.
- Completed operand/argument values.
- Remaining operands, arguments, branches, or body nodes.
- The destination that receives the next value.
- Active lexical scope and function closure references.
- Loop, function, pipeline, and control-flow state.
- A pending service call, when applicable.

Resuming must continue at the pending frame. It must not repeat completed expressions, successful service calls, emitted values, or transaction entries.

### Checkpoint on uncaught error

Capture a resumable checkpoint before an uncaught evaluator or service error unwinds its frames. Return both the original error and the checkpoint data.

If checkpoint creation fails, preserve the original error and return the checkpoint failure separately. Never replace the program error with a serialization or persistence error.

Error checkpoints must work inside:

- Nested expressions.
- Function and lambda bodies.
- Loop bodies and conditions.
- Conditional and match branches.
- Pipelines.
- Positional and keyword service-call arguments.

### Failed service-call frame

A failed service call must serialize:

- Service name.
- Method/tool name.
- Evaluated positional arguments.
- Evaluated keyword arguments.
- Failure message and structured error data when available.
- Transaction operation ID, if created.
- The pending destination for the call's result.
- Whether invoking the call may have produced external side effects.

The runtime must never retry a failed call implicitly.

### Resume modes

Implement these modes:

```go
type ResumeMode string

const (
    ResumeContinue     ResumeMode = "continue"
    ResumeAcceptResult ResumeMode = "accept_result"
    ResumeRetryCall    ResumeMode = "retry_call"
)
```

Semantics:

- `continue`: resume a breakpoint or deliberate pause at its next continuation.
- `accept_result`: require a replacement serialized value, validate it, place it in the failed call's pending destination, and continue with zero service invocations.
- `retry_call`: use the checkpoint's possibly edited service-call frame, invoke it exactly once, record the new transaction outcome, and continue on success.

Reject a resume mode that does not match the checkpoint reason. Return precise errors with the invalid field or state.

### Breakpoints

Support statement-boundary breakpoints:

```go
type Breakpoint struct {
    Line      int    `json:"line"`
    Condition string `json:"condition,omitempty"`
}
```

- A line breakpoint pauses before executing the matching statement.
- An optional condition is a SLOP expression evaluated in the current scope.
- A false condition continues normally.
- A condition error produces an error checkpoint that identifies the breakpoint condition as the failure source.
- Existing `pause` statements use the same result and checkpoint model.
- Store breakpoints in editable checkpoint state.

Step-in, step-over, step-out, arbitrary instruction jumps, and time-travel reversal are out of scope.

## Public API

Expose typed public APIs from `pkg/slop`. Exact names may follow repository conventions, but the capability must match this shape:

```go
type ExecutionStatus string

const (
    ExecutionCompleted ExecutionStatus = "completed"
    ExecutionPaused    ExecutionStatus = "paused"
    ExecutionFailed    ExecutionStatus = "failed"
)

type PauseReason string

const (
    PauseBreakpoint PauseReason = "breakpoint"
    PauseStatement  PauseReason = "pause"
    PauseError      PauseReason = "error"
)

type ExecuteOptions struct {
    Debug       bool
    Breakpoints []Breakpoint
}

type ResumeOptions struct {
    Mode              ResumeMode
    ReplacementResult *SerializedValue
}

type ExecutionResult struct {
    Status     ExecutionStatus
    Result     Value
    Checkpoint *Checkpoint
    Reason     PauseReason
    Failure    *ExecutionFailure
}

func (r *Runtime) ExecuteResumable(
    ctx context.Context,
    source string,
    options ExecuteOptions,
) (ExecutionResult, error)

func (r *Runtime) Resume(
    ctx context.Context,
    checkpoint *Checkpoint,
    options ResumeOptions,
) (ExecutionResult, error)

func ValidateCheckpoint(checkpoint *Checkpoint) error
```

Also expose typed checkpoint marshal/unmarshal and validation APIs. Callers must not need to import `internal` packages or write a checkpoint to disk before resuming it.

SLOP owns semantic checkpoint validation. External orchestrators will own file revisions, capability tokens, quotas, and JSON patch application.

## Checkpoint schema

Introduce a new major checkpoint version. The checkpoint must serialize:

- Source script and source hash.
- Executable program structure or an equivalent stable serialized representation.
- Current source position.
- Continuation stack and pending destinations.
- Scope chain, globals, closures, functions, and concrete values.
- Pending or failed service-call frame.
- Transaction log.
- Emitted values.
- Execution limits and counters.
- Control-flow state.
- Breakpoints.
- Pause/failure reason.
- Structured failure metadata.

Keep diagnostic metadata separate from executable state.

Every serialized node and frame needs a stable discriminator such as `kind`. Reject unknown discriminators, missing required fields, dangling scope/node references, invalid pending destinations, and incompatible program/continuation combinations.

Do not include bearer tokens, storage paths, TTLs, revision manifests, or slop-mcp-specific fields.

### Editing program structure

Callers may edit serialized program nodes. Validation must confirm:

- The new node is valid for its parent slot.
- Referenced scopes, functions, and nodes exist.
- The active continuation remains compatible with the edited node.
- The pending destination accepts the resumed value.

Reject incompatible edits. Do not silently restart execution or move the instruction pointer.

### v1 compatibility

Detect existing v1 explicit-pause checkpoints.

- Resume them when enough information exists to translate them safely.
- Otherwise return a typed incompatibility error that still permits inspection.
- Never guess a missing continuation or replay side effects.

## Implementation constraints

- Use TDD: observe each regression test fail before implementing its behavior.
- Keep serialization deterministic.
- Preserve exact integer values and existing concrete value semantics.
- Propagate `context.Context` cancellation through execution, resume, breakpoint conditions, and service calls.
- Do not serialize Go pointers, function closures, goroutines, channels, or opaque implementation state.
- Avoid package cycles and keep interpreter details behind the public `pkg/slop` API.
- Do not modify generated or cached dependency files.
- Do not add storage, authentication, or network dependencies.

## Required tests

### Serialization

- Round-trip every serialized value and AST/program node type.
- Round-trip every continuation-frame type.
- Round-trip nested scope and closure references.
- Deterministic JSON for equivalent state.
- Reject corrupt and incompatible state.

### Continuation correctness

- Pause and resume within nested arithmetic, list/map construction, indexing, and function arguments.
- Pause and resume within functions, lambdas, loops, conditionals, matches, and pipelines.
- Confirm completed operands are not evaluated again.
- Confirm emitted values are not duplicated.
- Confirm successful service calls are not repeated.

### Error recovery

- Capture undefined-variable, type, limit, and service errors before frame unwinding.
- Capture errors at every nesting level listed above.
- Substitute a failed call's result and complete with zero retry calls.
- Edit a failed call and retry exactly once.
- A second retry failure returns a new resumable checkpoint.
- Reject `accept_result` without a replacement value.
- Reject `retry_call` for non-call failures.
- Preserve the original error when checkpoint serialization fails.

### Breakpoints

- Pause before a line executes.
- Conditional true and false behavior.
- Conditional evaluation error.
- Multiple breakpoints and resume to the next breakpoint.
- Existing `pause` statement compatibility.

### Compatibility and quality

- Existing runtime and checkpoint tests continue to pass.
- Race-enabled evaluator and public API tests pass.
- Add focused benchmarks that compare ordinary execution before and after the frame refactor. Report material regressions.

## Acceptance criteria

The work is complete only when all conditions hold:

1. A nested failed service call returns an editable checkpoint containing its live continuation.
2. Replacing the failed result resumes the original program without another service invocation.
3. Editing and retrying the call invokes it exactly once and resumes on success.
4. Breakpoints and `pause` statements use the same typed checkpoint/result path.
5. Program, scope, and call-frame edits are validated before resume.
6. No completed side effect or emitted value is repeated during resume.
7. Existing non-resumable APIs remain compatible.
8. Public consumers can perform the workflow without importing `internal` packages.
9. Full unit, race, formatting, lint, and repository integration checks pass.
10. Public documentation explains the new checkpoint version, APIs, resume modes, and v1 compatibility.

## Verification

Use the repository's documented commands. At minimum run:

```bash
gofmt -w <changed-go-files>
go vet ./...
go test ./...
go test -race ./...
git diff --check
```

Run any configured linter and integration suite as well. Do not claim completion without fresh command output.

## Handoff

Return:

- A concise implementation summary.
- Public API and checkpoint-schema changes.
- Compatibility notes.
- Exact verification commands and results.
- Any known unsupported program constructs.
- The tagged release version or the commit SHA that downstream slop-mcp should consume.
