package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolHost is the surface the loop uses to enumerate and execute tools.
// slop-mcp's server provides a real implementation that bridges to the
// MCP registry and SLOP runtime. Tests use a stub.
type ToolHost interface {
	// List returns specs visible to a given call. allow is the agent's
	// whitelist; nil/empty means "all available".
	List(ctx context.Context, allow []string) []ToolSpec

	// Execute runs a tool by name. result is the textual output fed back
	// to the model.
	//
	// Three return shapes:
	//   - (result, false, nil): success.
	//   - ("", false, err): infrastructure failure — the tool could not
	//     be reached at all (unknown name, transport error, timeout).
	//     The loop treats this as a hard error and aborts the iteration.
	//   - (result, true, nil): tool ran but reported failure (MCP isError
	//     bit set). The loop forwards the result to the model marked as
	//     an error so the model can decide whether to recover or stop.
	//
	// Implementations must not return (_, true, non-nil err) — the
	// "tool ran and failed" and "tool could not run" paths are mutually
	// exclusive.
	Execute(ctx context.Context, name string, args json.RawMessage) (result string, isError bool, err error)
}

// NoopHost is a ToolHost that reports no tools and fails all Execute calls.
// Used in CLI contexts (slop-mcp run, slop-mcp monitor) where no MCP registry
// is available. llm() calls succeed if the agent def has no tools; any tool
// call the model attempts will receive an infrastructure error.
type NoopHost struct{}

func (NoopHost) List(_ context.Context, _ []string) []ToolSpec { return nil }
func (NoopHost) Execute(_ context.Context, name string, _ json.RawMessage) (string, bool, error) {
	return "", false, fmt.Errorf("tools unavailable in this context: %s", name)
}
