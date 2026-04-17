package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/overrides"
	"github.com/standardbeagle/slop-mcp/internal/registry"
	"github.com/stretchr/testify/require"
)

// jsonStr marshals v to a JSON string for assertions.
func jsonStr(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// newCustomizeTestServer creates a minimal server with a mock MCP tool wired in.
func newCustomizeTestServer(t *testing.T) *Server {
	t.Helper()
	tools := []registry.ToolInfo{
		{
			Name:        "tool_one",
			Description: "Upstream description for tool_one",
			MCPName:     "mock",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"description": "search query"},
				},
			},
		},
	}
	s := mockServer(nil)
	// Register under "mock" key so upstreamToolInfo can find it.
	s.registry.AddToolsForTesting("mock", tools)
	return s
}

func TestCustomizeTools_NilStore(t *testing.T) {
	s := newCustomizeTestServer(t)
	// overrideStore is nil by default in mockServer
	_, _, err := s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{
		Action: "set_override", MCP: "mock", Tool: "tool_one", Description: "x",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "override store not initialized")
}

func TestCustomizeTools_UnknownAction(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.overrideStore = store

	_, _, err = s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{Action: "bogus"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown action")
}

func TestCustomizeTools_NotYetImplementedActions(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.overrideStore = store

	notImpl := []string{"define_custom", "remove_custom", "list_custom", "export", "import"}
	for _, action := range notImpl {
		_, _, err := s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{Action: action})
		require.Error(t, err, "action %s should return error", action)
		require.Contains(t, err.Error(), "not yet implemented", "action %s: %v", action, err)
	}
}

func TestCustomizeTools_SetOverride_Persists(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.overrideStore = store

	in := CustomizeToolsInput{
		Action:      "set_override",
		MCP:         "mock",
		Tool:        "tool_one",
		Description: "SHORT",
		Scope:       "user",
	}
	_, out, err := s.handleCustomizeTools(context.Background(), nil, in)
	require.NoError(t, err)

	outStr := jsonStr(out)
	if !strings.Contains(outStr, `"affected":1`) {
		t.Errorf("expected affected:1 in output: %s", outStr)
	}
	if !strings.Contains(outStr, `"ok":true`) {
		t.Errorf("expected ok:true in output: %s", outStr)
	}

	got, ok := store.GetOverride("mock.tool_one")
	require.True(t, ok, "override should be persisted")
	require.Equal(t, "SHORT", got.Description)
	require.NotEmpty(t, got.SourceHash, "SourceHash should be computed")
}

func TestCustomizeTools_SetOverride_MissingFields(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.overrideStore = store

	cases := []CustomizeToolsInput{
		{Action: "set_override", Tool: "tool_one", Description: "x"},          // missing mcp
		{Action: "set_override", MCP: "mock", Description: "x"},               // missing tool
		{Action: "set_override", MCP: "mock", Tool: "tool_one"},                // missing description
	}
	for _, in := range cases {
		_, _, err := s.handleCustomizeTools(context.Background(), nil, in)
		require.Error(t, err)
	}
}

func TestCustomizeTools_RemoveOverride_AllScopes(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.overrideStore = store

	// Pre-populate
	require.NoError(t, store.SetOverride(overrides.ScopeUser, "mock.tool_one", overrides.OverrideEntry{Description: "x"}))

	in := CustomizeToolsInput{Action: "remove_override", MCP: "mock", Tool: "tool_one"}
	_, out, err := s.handleCustomizeTools(context.Background(), nil, in)
	require.NoError(t, err)
	require.Equal(t, 1, out.Affected)

	_, ok := store.GetOverride("mock.tool_one")
	require.False(t, ok, "override should be gone")
}

func TestCustomizeTools_RemoveOverride_MissingFields(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.overrideStore = store

	_, _, err = s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{
		Action: "remove_override", Tool: "tool_one",
	})
	require.Error(t, err)
}

func TestCustomizeTools_ListOverrides_Basic(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.overrideStore = store

	require.NoError(t, store.SetOverride(overrides.ScopeUser, "mock.tool_one", overrides.OverrideEntry{
		Description: "override desc",
		SourceHash:  "abc",
	}))

	in := CustomizeToolsInput{Action: "list_overrides"}
	_, out, err := s.handleCustomizeTools(context.Background(), nil, in)
	require.NoError(t, err)
	require.Equal(t, 1, out.Affected)
	require.Len(t, out.Entries, 1)
	require.Equal(t, "mock.tool_one", out.Entries[0].Key)
	require.Equal(t, "override desc", out.Entries[0].Description)
}

func TestCustomizeTools_ListOverrides_StaleOnly(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.overrideStore = store

	require.NoError(t, store.SetOverride(overrides.ScopeUser, "mock.tool_one", overrides.OverrideEntry{
		Description: "override",
		SourceHash:  "forced_stale_hash",
	}))

	in := CustomizeToolsInput{Action: "list_overrides", StaleOnly: true}
	_, out, err := s.handleCustomizeTools(context.Background(), nil, in)
	require.NoError(t, err)

	outStr := jsonStr(out)
	if !strings.Contains(outStr, `"stale":true`) {
		t.Errorf("expected stale entry: %s", outStr)
	}
	require.Equal(t, 1, out.Affected)
}

func TestCustomizeTools_ListOverrides_FilterByMCP(t *testing.T) {
	s := mockServer(nil)
	s.registry.AddToolsForTesting("mcp-x", []registry.ToolInfo{
		{Name: "tool_a", Description: "a", MCPName: "mcp-x"},
	})
	s.registry.AddToolsForTesting("mcp-y", []registry.ToolInfo{
		{Name: "tool_b", Description: "b", MCPName: "mcp-y"},
	})
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.overrideStore = store

	require.NoError(t, store.SetOverride(overrides.ScopeUser, "mcp-x.tool_a", overrides.OverrideEntry{Description: "ov-a"}))
	require.NoError(t, store.SetOverride(overrides.ScopeUser, "mcp-y.tool_b", overrides.OverrideEntry{Description: "ov-b"}))

	_, out, err := s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{
		Action: "list_overrides", MCP: "mcp-x",
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.Affected)
	require.Equal(t, "mcp-x.tool_a", out.Entries[0].Key)
}

func TestCustomizeTools_WrapperUnmarshal(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.overrideStore = store

	args, _ := json.Marshal(map[string]any{
		"action": "list_overrides",
	})
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Arguments: args},
	}
	result, err := s.wrapCustomizeTools(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError, "expected non-error result")
}
