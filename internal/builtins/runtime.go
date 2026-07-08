package builtins

import (
	"sync"

	"github.com/standardbeagle/slop/pkg/slop"
)

// newRuntimeMu serializes runtime construction: slop v0.3.0's NewRuntime
// writes a package-level pipeline hook (builtin.SetPipelineFuncCaller), so
// constructing runtimes concurrently is a data race.
var newRuntimeMu sync.Mutex

// NewRuntime constructs a SLOP runtime, serialized against concurrent
// construction elsewhere in the process. Always use this instead of calling
// slop.NewRuntime directly.
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
