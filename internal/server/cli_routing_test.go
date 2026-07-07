package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/cli"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestExecuteTool_CLIFailureReturnsErrorResult(t *testing.T) {
	s := mockServer(nil)
	s.cliRegistry.Register(&cli.ToolConfig{
		Name:    "fail",
		Command: "sh",
		Args: []cli.ArgConfig{
			{Name: "flag", Position: 0, Type: "string", Default: "-c"},
			{Name: "script", Required: true, Position: 1, Type: "string"},
		},
	})

	result, out, err := s.handleExecuteTool(context.Background(), nil, ExecuteToolInput{
		MCPName:    "cli",
		ToolName:   "fail",
		Parameters: map[string]any{"script": "echo partial; echo bad >&2; exit 7"},
	})
	require.NoError(t, err)
	require.Nil(t, out)
	require.NotNil(t, result)
	require.True(t, result.IsError)

	payload := decodeTextPayload(t, result)
	assert.Equal(t, float64(7), payload["exit_code"])
	assert.Contains(t, payload["stdout"], "partial")
	assert.Contains(t, payload["stderr"], "bad")
	assert.Contains(t, payload["error"], "exit code 7")
}

func TestWrapExecuteTool_CLIFailureReturnsIsError(t *testing.T) {
	s := mockServer(nil)
	s.cliRegistry.Register(&cli.ToolConfig{
		Name:    "fail",
		Command: "sh",
		Args: []cli.ArgConfig{
			{Name: "flag", Position: 0, Type: "string", Default: "-c"},
			{Name: "script", Required: true, Position: 1, Type: "string"},
		},
	})

	rawArgs := json.RawMessage(`{
		"mcp_name": "cli",
		"tool_name": "fail",
		"parameters": {"script": "exit 9"}
	}`)
	result, err := s.wrapExecuteTool(context.Background(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Arguments: rawArgs},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError)

	payload := decodeTextPayload(t, result)
	assert.Equal(t, float64(9), payload["exit_code"])
	assert.Contains(t, payload["error"], "exit code 9")
}

func decodeTextPayload(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "expected text content, got %T", result.Content[0])

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
	return payload
}
