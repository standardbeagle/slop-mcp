package builtins

import (
	"context"
	"fmt"
	"sync"

	"github.com/standardbeagle/slop/pkg/slop"
)

// newRuntimeMu serializes runtime construction: slop v0.3.0's NewRuntime
// writes a package-level pipeline hook (builtin.SetPipelineFuncCaller), so
// constructing runtimes concurrently is a data race.
var newRuntimeMu sync.Mutex

// slopExecMu serializes SLOP construction AND execution across the whole
// process. This is required — not merely defensive — because slop v0.3.0 binds
// the pipeline function caller to a package-level global at construction
// (builtin.SetPipelineFuncCaller) and reads it *unsynchronized* while executing
// pipeline builtins (map/filter/reduce/find/zip_with/...). Consequences a
// construction-only lock cannot prevent:
//   - constructing a second runtime while the first is executing races the
//     global (write vs read), and
//   - it rebinds the global to the second runtime's evaluator, so the first
//     runtime's pipeline callbacks are misrouted into the wrong evaluator.
//
// Holding slopExecMu from just before construction until execution completes is
// the only correct fix available without patching slop. The proper fix is
// upstream: make the pipeline caller per-registry instead of a global. Until
// then, server-side run_slop / custom-tool executions are serialized.
var slopExecMu sync.Mutex

// LockSlopExec acquires the process-wide SLOP construct+execute lock. Callers
// MUST construct their runtime and run it (or parse it) before releasing, so no
// other runtime rebinds the shared pipeline-caller global in between. See
// slopExecMu for why this spans construction and execution.
func LockSlopExec() { slopExecMu.Lock() }

// UnlockSlopExec releases the process-wide SLOP construct+execute lock.
func UnlockSlopExec() { slopExecMu.Unlock() }

// NewRuntime constructs a SLOP runtime, serialized against concurrent
// construction elsewhere in the process. Always use this instead of calling
// slop.NewRuntime directly. NOTE: callers that will EXECUTE the runtime must
// hold LockSlopExec across construction and execution; the construction lock
// here alone does not prevent the pipeline-global race described on slopExecMu.
func NewRuntime() *slop.Runtime {
	newRuntimeMu.Lock()
	defer newRuntimeMu.Unlock()
	return slop.NewRuntime()
}

// NewRuntimeWithConfig constructs a SLOP runtime with execution limits,
// serialized for the same package-level hook as NewRuntime.
func NewRuntimeWithConfig(cfg slop.Config) *slop.Runtime {
	newRuntimeMu.Lock()
	defer newRuntimeMu.Unlock()
	return slop.NewRuntimeWithConfig(cfg)
}

// RunScriptContext executes a SLOP script under ctx while holding the
// process-wide exec lock, so it is safe to call even if other SLOP execution is
// happening concurrently. rt.Execute is not context-aware but self-limits via
// the runtime's MaxDuration; on ctx cancellation this returns promptly while
// the worker finishes in the background and releases the lock. Panics in the
// evaluator are converted to errors rather than crashing the process.
//
// Unlike the server's path, this acquires the exec lock itself (callers here
// construct a single runtime and are not already holding it).
func RunScriptContext(ctx context.Context, rt *slop.Runtime, script string) (slop.Value, error) {
	LockSlopExec()
	relOnce := sync.OnceFunc(UnlockSlopExec)

	type result struct {
		value slop.Value
		err   error
	}
	done := make(chan result, 1)
	go func() {
		defer relOnce()
		defer func() {
			if r := recover(); r != nil {
				done <- result{err: fmt.Errorf("slop execution panicked: %v", r)}
			}
		}()
		value, err := rt.Execute(script)
		done <- result{value: value, err: err}
	}()

	select {
	case res := <-done:
		return res.value, res.err
	case <-ctx.Done():
		return nil, fmt.Errorf("slop execution canceled or timed out: %w", ctx.Err())
	}
}
