package server

import (
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/stretchr/testify/assert"
)

// TestIsCLIRoute pins the execute_tool routing contract: mcp_name "cli" always
// routes to the CLI registry; the legacy cli_ tool-name prefix only does so
// when mcp_name is not a registered MCP. An explicitly addressed real MCP must
// win even if it exposes cli_-prefixed tools.
func TestIsCLIRoute(t *testing.T) {
	s := mockServer(nil)
	s.registry.SetConfigured(config.MCPConfig{Name: "real-mcp", Type: "stdio", Command: "x"})

	assert.True(t, s.isCLIRoute("cli", "jq"), "explicit cli mcp_name routes to CLI")
	assert.True(t, s.isCLIRoute("cli", "cli_jq"), "explicit cli mcp_name with prefix routes to CLI")
	assert.True(t, s.isCLIRoute("unknown-mcp", "cli_jq"), "cli_ prefix with unknown mcp_name routes to CLI")

	assert.False(t, s.isCLIRoute("real-mcp", "cli_jq"),
		"explicit registered MCP must not be hijacked by the cli_ prefix")
	assert.False(t, s.isCLIRoute("real-mcp", "some_tool"))
	assert.False(t, s.isCLIRoute("unknown-mcp", "some_tool"))
}
