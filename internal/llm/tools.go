package llm

import (
	"context"
	"encoding/json"
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
