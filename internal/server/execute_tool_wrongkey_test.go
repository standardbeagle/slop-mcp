package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// extractErrorText pulls the text content out of a CallToolResult for assertions.
func extractErrorText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, result)
	require.True(t, result.IsError, "expected IsError=true, got %+v", result)
	require.NotEmpty(t, result.Content)
	tc, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "expected TextContent")
	return tc.Text
}

// TestWrapExecuteTool_WrongKey_Arguments asserts that passing the common typo
// `arguments` instead of `parameters` yields an error that names the correct
// key, rather than silently running the tool with an empty parameter map.
func TestWrapExecuteTool_WrongKey_Arguments(t *testing.T) {
	s := mockServer(nil)
	ctx := context.Background()

	raw := json.RawMessage(`{
		"mcp_name": "some-mcp",
		"tool_name": "some_tool",
		"arguments": {"k": "v"}
	}`)

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: raw}}
	result, err := s.wrapExecuteTool(ctx, req)

	assert.NoError(t, err, "wrapper returns protocol err=nil and uses IsError result")
	text := extractErrorText(t, result)
	assert.Contains(t, strings.ToLower(text), "parameters",
		"error must name the correct field 'parameters'. got: %s", text)
	assert.Contains(t, strings.ToLower(text), "arguments",
		"error must reference the wrong field 'arguments' that was sent. got: %s", text)
}

// TestWrapExecuteTool_WrongKey_Args same hint for `args` alias.
func TestWrapExecuteTool_WrongKey_Args(t *testing.T) {
	s := mockServer(nil)
	ctx := context.Background()

	raw := json.RawMessage(`{
		"mcp_name": "some-mcp",
		"tool_name": "some_tool",
		"args": {"k": "v"}
	}`)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: raw}}
	result, err := s.wrapExecuteTool(ctx, req)

	assert.NoError(t, err)
	text := extractErrorText(t, result)
	assert.Contains(t, strings.ToLower(text), "parameters")
	assert.Contains(t, strings.ToLower(text), "args")
}

// TestWrapExecuteTool_WrongKey_Input same hint for `input` alias.
func TestWrapExecuteTool_WrongKey_Input(t *testing.T) {
	s := mockServer(nil)
	ctx := context.Background()

	raw := json.RawMessage(`{
		"mcp_name": "some-mcp",
		"tool_name": "some_tool",
		"input": {"k": "v"}
	}`)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: raw}}
	result, err := s.wrapExecuteTool(ctx, req)

	assert.NoError(t, err)
	text := extractErrorText(t, result)
	assert.Contains(t, strings.ToLower(text), "parameters")
	assert.Contains(t, strings.ToLower(text), "input")
}

// TestWrapExecuteTool_BothProvided_PreferParameters confirms that when both
// `parameters` and `arguments` are present, `parameters` is treated as
// authoritative. The call proceeds (registry miss is fine; we just want to
// confirm no rejection on the typo-coexists path).
func TestWrapExecuteTool_BothProvided_PreferParameters(t *testing.T) {
	s := mockServer(nil)
	ctx := context.Background()

	raw := json.RawMessage(`{
		"mcp_name": "nonexistent-mcp",
		"tool_name": "some_tool",
		"parameters": {"real": "v"},
		"arguments": {"ignored": "v"}
	}`)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: raw}}
	result, err := s.wrapExecuteTool(ctx, req)
	assert.NoError(t, err)
	require.NotNil(t, result)
	// Should fall through to registry path; error message must NOT be the
	// wrong-key hint. A registry-level error is acceptable here.
	text := ""
	if result.IsError && len(result.Content) > 0 {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok {
			text = tc.Text
		}
	}
	assert.NotContains(t, strings.ToLower(text), "did you mean",
		"should not emit wrong-key hint when parameters is present")
}

// TestWrapExecuteTool_EmptyArgumentsValue no hint when the wrong key is
// present but empty/null -- nothing to rescue, fall through to normal path.
func TestWrapExecuteTool_EmptyArgumentsValue(t *testing.T) {
	s := mockServer(nil)
	ctx := context.Background()

	for _, body := range []string{
		`{"mcp_name":"m","tool_name":"t","arguments":null}`,
		`{"mcp_name":"m","tool_name":"t","arguments":{}}`,
	} {
		raw := json.RawMessage(body)
		req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: raw}}
		result, err := s.wrapExecuteTool(ctx, req)
		assert.NoError(t, err)
		text := ""
		if result != nil && result.IsError && len(result.Content) > 0 {
			if tc, ok := result.Content[0].(*mcp.TextContent); ok {
				text = tc.Text
			}
		}
		assert.NotContains(t, strings.ToLower(text), "did you mean",
			"empty wrong-key value must not trigger hint. body=%s", body)
	}
}

func TestToolWrappersReturnErrorForMissingParams(t *testing.T) {
	s := mockServer(nil)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	tests := []struct {
		name string
		call func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error)
	}{
		{name: "search_tools", call: s.wrapSearchTools},
		{name: "execute_tool", call: s.wrapExecuteTool},
		{name: "run_slop", call: s.wrapRunSlop},
		{name: "manage_mcps", call: s.wrapManageMCPs},
		{name: "auth_mcp", call: s.wrapAuthMCP},
		{name: "get_metadata", call: s.wrapGetMetadata},
		{name: "slop_reference", call: s.wrapSlopReference},
		{name: "slop_help", call: s.wrapSlopHelp},
		{name: "agnt_watch", call: s.wrapAgntWatch},
		{name: "customize_tools", call: s.wrapCustomizeTools},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.call(ctx, req)
			require.NoError(t, err)
			text := extractErrorText(t, result)
			assert.Contains(t, text, "missing params")
		})
	}
}

func TestCallToolArgumentsBlankDefaultsToEmptyObject(t *testing.T) {
	got, err := callToolArguments(&mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(" \n\t ")},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(got))
}
