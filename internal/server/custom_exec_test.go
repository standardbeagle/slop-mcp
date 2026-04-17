package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/overrides"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCustomTestServer(t *testing.T) (*Server, *overrides.Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: dir})
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	s := mockServer(nil)
	s.overrideStore = store
	return s, store
}

func TestExecuteTool_RoutesCustomTool(t *testing.T) {
	s, store := newCustomTestServer(t)

	err := store.SetCustom(overrides.ScopeUser, "greet", overrides.CustomTool{
		Description: "greet a person",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		},
		Body: `"hello " + args["name"]`,
	})
	require.NoError(t, err)

	ctx := context.Background()
	_, out, err := s.handleExecuteTool(ctx, nil, ExecuteToolInput{
		MCPName:    "_custom",
		ToolName:   "greet",
		Parameters: map[string]any{"name": "world"},
	})
	require.NoError(t, err)
	assert.Equal(t, "hello world", fmt.Sprintf("%v", out))
}

func TestExecuteTool_RoutesCustomTool_ShorthandBinding(t *testing.T) {
	s, store := newCustomTestServer(t)

	err := store.SetCustom(overrides.ScopeUser, "double", overrides.CustomTool{
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"x": map[string]any{"type": "number"}},
			"required":   []any{"x"},
		},
		Body: `x * 2`,
	})
	require.NoError(t, err)

	ctx := context.Background()
	_, out, err := s.handleExecuteTool(ctx, nil, ExecuteToolInput{
		MCPName:    "_custom",
		ToolName:   "double",
		Parameters: map[string]any{"x": float64(7)},
	})
	require.NoError(t, err)
	// 7 * 2 = 14
	assert.Equal(t, float64(14), out)
}

func TestExecuteTool_CustomTool_MissingRequired(t *testing.T) {
	s, store := newCustomTestServer(t)

	err := store.SetCustom(overrides.ScopeUser, "greet", overrides.CustomTool{
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"name": map[string]any{"type": "string"}},
			"required":   []any{"name"},
		},
		Body: `"hi"`,
	})
	require.NoError(t, err)

	ctx := context.Background()
	result, out, err := s.handleExecuteTool(ctx, nil, ExecuteToolInput{
		MCPName:    "_custom",
		ToolName:   "greet",
		Parameters: map[string]any{},
	})
	// Custom tool errors route through errorResult: protocol err is nil, result has IsError=true.
	require.NoError(t, err)
	assert.Nil(t, out)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	// Error text should mention the missing parameter.
	require.NotEmpty(t, result.Content)
	text := result.Content[0].(*mcp.TextContent).Text
	assert.Contains(t, text, "name")
}

func TestExecuteTool_CustomTool_RecursionGuard(t *testing.T) {
	s, _ := newCustomTestServer(t)

	ct := overrides.CustomTool{Body: `"ok"`}
	ctx := context.Background()
	// Pre-load depth to 16
	for i := 0; i < 16; i++ {
		ctx = withCustomDepth(ctx)
	}
	_, err := s.executeCustomTool(ctx, ct, map[string]any{})
	assert.ErrorIs(t, err, ErrCustomToolRecursion)
}

func TestExecuteTool_CustomTool_FallsThrough_WhenNoStore(t *testing.T) {
	// overrideStore == nil → normal MCP path → MCPNotFoundError
	s := mockServer(nil)
	ctx := context.Background()
	_, _, err := s.handleExecuteTool(ctx, nil, ExecuteToolInput{
		MCPName:  "_custom",
		ToolName: "anything",
	})
	// Should fall through to registry, which returns an error (MCP not found)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrCustomToolRecursion)
}

func TestExecuteTool_CustomTool_UnknownNameFallsThrough(t *testing.T) {
	s, _ := newCustomTestServer(t)
	// Store is present but tool "noexist" not in it → fall through to registry
	ctx := context.Background()
	_, _, err := s.handleExecuteTool(ctx, nil, ExecuteToolInput{
		MCPName:  "_custom",
		ToolName: "noexist",
	})
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrCustomToolRecursion)
}
