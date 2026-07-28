package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// execute_tool forwards raw JSON to preserve integer precision, which used to
// mean it was also the one dispatch path where no schema was ever consulted:
// a mistyped parameter cost a round-trip and came back in the target server's
// own words. These assert the wrapper answers from the schema first.
func preflightServer() *Server {
	return mockServer([]registry.ToolInfo{{
		Name:    "task_update",
		MCPName: "test-mcp",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":      map[string]any{"type": "string"},
				"version": map[string]any{"type": "integer"},
			},
			"required": []any{"id", "version"},
		},
	}})
}

func callExecuteTool(t *testing.T, s *Server, params string) *mcp.CallToolResult {
	t.Helper()
	raw := json.RawMessage(`{"mcp_name":"test-mcp","tool_name":"task_update","parameters":` + params + `}`)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: raw}}
	result, err := s.wrapExecuteTool(context.Background(), req)
	require.NoError(t, err, "wrapper reports failures via IsError, not a protocol error")
	require.NotNil(t, result)
	return result
}

func TestWrapExecuteTool_PreflightRejectsMissingRequired(t *testing.T) {
	result := callExecuteTool(t, preflightServer(), `{"id":"01A"}`)

	text := extractErrorText(t, result)
	assert.Contains(t, text, "version", "must name the missing required parameter. got: %s", text)
	assert.NotContains(t, strings.ToLower(text), "lazy-connect",
		"must be answered from the schema, not by dialing the MCP. got: %s", text)
}

func TestWrapExecuteTool_PreflightNamesTheCorrectKey(t *testing.T) {
	result := callExecuteTool(t, preflightServer(), `{"taskId":"01A","version":3}`)

	text := extractErrorText(t, result)
	assert.Contains(t, text, "did you mean 'id'", "must map taskId -> id. got: %s", text)
}

func TestWrapExecuteTool_PreflightPassesAValidCallThrough(t *testing.T) {
	// A schema-valid call gets no preflight verdict, so it reaches dispatch and
	// fails there instead — the mock registry has no live connection. The point
	// is that the failure is a transport one, not a parameter one.
	result := callExecuteTool(t, preflightServer(), `{"id":"01A","version":3}`)

	text := extractErrorText(t, result)
	assert.NotContains(t, text, "Expected parameters",
		"a schema-valid call must not be rejected by preflight. got: %s", text)
}
