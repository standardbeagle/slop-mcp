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

	// Execute runs a tool by name. The string is the textual result fed
	// back to the model; isError reports whether the call failed (the
	// loop will mark the tool message accordingly).
	Execute(ctx context.Context, name string, args json.RawMessage) (result string, isError bool, err error)
}
