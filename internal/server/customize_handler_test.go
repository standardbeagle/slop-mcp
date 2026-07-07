package server

import (
	"context"
	"encoding/json"
	"os"
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
	s.SetOverrideStoreForTesting(store)

	_, _, err = s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{Action: "bogus"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown action")
}

func TestCustomizeTools_DefineCustom_Persists(t *testing.T) {
	s := newCustomizeTestServer(t)
	dir := t.TempDir()
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: dir})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

	in := CustomizeToolsInput{
		Action:      "define_custom",
		Name:        "my_tool",
		Description: "test",
		InputSchema: map[string]any{"type": "object"},
		Body:        `emit("ok")`,
		Scope:       "user",
	}
	_, _, err = s.handleCustomizeTools(context.Background(), nil, in)
	require.NoError(t, err)
	ct, ok := store.GetCustom("my_tool")
	if !ok || ct.Body != `emit("ok")` {
		t.Fatalf("not persisted: %+v", ct)
	}
}

func TestCustomizeTools_DefineCustom_RejectsInvalidName(t *testing.T) {
	s := newCustomizeTestServer(t)
	dir := t.TempDir()
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: dir})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

	cases := []string{"BadName", "1starts", "has-dash", "with space", "way_too_long_" + strings.Repeat("x", 80)}
	for _, name := range cases {
		in := CustomizeToolsInput{
			Action: "define_custom", Name: name, Description: "t",
			InputSchema: map[string]any{"type": "object"}, Body: "42",
		}
		_, _, err := s.handleCustomizeTools(context.Background(), nil, in)
		if err == nil {
			t.Errorf("expected rejection for %q", name)
		}
	}
}

func TestCustomizeTools_DefineCustom_RejectsMetaToolCollision(t *testing.T) {
	s := newCustomizeTestServer(t)
	dir := t.TempDir()
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: dir})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

	in := CustomizeToolsInput{
		Action: "define_custom", Name: "search_tools",
		Description: "x", InputSchema: map[string]any{"type": "object"}, Body: "1",
	}
	_, _, err = s.handleCustomizeTools(context.Background(), nil, in)
	if err == nil {
		t.Error("expected collision error for meta-tool name")
	}
}

func TestCustomizeTools_DefineCustom_RejectsInvalidInputSchema(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

	tests := []struct {
		name   string
		schema map[string]any
		want   string
	}{
		{
			name:   "missing type",
			schema: map[string]any{"properties": map[string]any{}},
			want:   "inputSchema.type",
		},
		{
			name:   "non object type",
			schema: map[string]any{"type": "array"},
			want:   `"object"`,
		},
		{
			name:   "property is not object",
			schema: map[string]any{"type": "object", "properties": map[string]any{"x": "bad"}},
			want:   "properties.x",
		},
		{
			name:   "property type is not string",
			schema: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": 3}}},
			want:   "properties.x.type",
		},
		{
			name:   "property type is unsupported",
			schema: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "strng"}}},
			want:   "one of string, number, integer, boolean, array, object",
		},
		{
			name:   "required entry is not string",
			schema: map[string]any{"type": "object", "required": []any{"x", 3}},
			want:   "required[1]",
		},
		{
			name:   "external ref",
			schema: map[string]any{"type": "object", "$ref": "https://example.com/schema.json"},
			want:   "external $ref",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{
				Action:      "define_custom",
				Name:        "schema_test",
				Description: "t",
				InputSchema: tt.schema,
				Body:        "42",
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestCustomizeTools_DefineCustom_ReportsShorthandCollisions(t *testing.T) {
	s := newCustomizeTestServer(t)
	dir := t.TempDir()
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: dir})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

	in := CustomizeToolsInput{
		Action: "define_custom", Name: "my_tool", Description: "t",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"mem_save": map[string]any{"type": "string"},
				"safe_key": map[string]any{"type": "string"},
			},
		},
		Body: "42",
	}
	_, out, err := s.handleCustomizeTools(context.Background(), nil, in)
	require.NoError(t, err)
	js := jsonStr(out)
	if !strings.Contains(js, "shorthand_skipped") {
		t.Errorf("expected shorthand_skipped field in response: %s", js)
	}
	if !strings.Contains(js, "mem_save") {
		t.Errorf("mem_save should be listed as shorthand_skipped: %s", js)
	}
}

func TestCustomizeTools_DefineCustom_RejectsOversizedBody(t *testing.T) {
	t.Setenv("SLOP_MAX_CUSTOM_BODY", "64")
	s := newCustomizeTestServer(t)
	dir := t.TempDir()
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: dir})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

	big := strings.Repeat("x", 100)
	in := CustomizeToolsInput{
		Action: "define_custom", Name: "big", Description: "t",
		InputSchema: map[string]any{"type": "object"}, Body: big,
	}
	_, _, err = s.handleCustomizeTools(context.Background(), nil, in)
	if err == nil {
		t.Error("expected body-size rejection")
	}
}

func TestCustomizeTools_RemoveCustom(t *testing.T) {
	s := newCustomizeTestServer(t)
	dir := t.TempDir()
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: dir})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

	_ = store.SetCustom(overrides.ScopeUser, "x", overrides.CustomTool{Body: "1"})
	in := CustomizeToolsInput{Action: "remove_custom", Name: "x"}
	_, _, err = s.handleCustomizeTools(context.Background(), nil, in)
	require.NoError(t, err)
	if _, ok := store.GetCustom("x"); ok {
		t.Error("custom tool should be removed")
	}
}

func TestCustomizeTools_ListCustom(t *testing.T) {
	s := newCustomizeTestServer(t)
	dir := t.TempDir()
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: dir})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

	_ = store.SetCustom(overrides.ScopeUser, "alpha", overrides.CustomTool{Description: "a"})
	_ = store.SetCustom(overrides.ScopeUser, "beta", overrides.CustomTool{Description: "b"})

	in := CustomizeToolsInput{Action: "list_custom"}
	_, out, err := s.handleCustomizeTools(context.Background(), nil, in)
	require.NoError(t, err)
	js := jsonStr(out)
	if !strings.Contains(js, "alpha") || !strings.Contains(js, "beta") {
		t.Errorf("expected both tools: %s", js)
	}
}

func TestCustomizeTools_SetOverride_Persists(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

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
	s.SetOverrideStoreForTesting(store)

	cases := []CustomizeToolsInput{
		{Action: "set_override", Tool: "tool_one", Description: "x"}, // missing mcp
		{Action: "set_override", MCP: "mock", Description: "x"},      // missing tool
		{Action: "set_override", MCP: "mock", Tool: "tool_one"},      // missing description
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
	s.SetOverrideStoreForTesting(store)

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
	s.SetOverrideStoreForTesting(store)

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
	s.SetOverrideStoreForTesting(store)

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
	s.SetOverrideStoreForTesting(store)

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
	s.SetOverrideStoreForTesting(store)

	require.NoError(t, store.SetOverride(overrides.ScopeUser, "mcp-x.tool_a", overrides.OverrideEntry{Description: "ov-a"}))
	require.NoError(t, store.SetOverride(overrides.ScopeUser, "mcp-y.tool_b", overrides.OverrideEntry{Description: "ov-b"}))

	_, out, err := s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{
		Action: "list_overrides", MCP: "mcp-x",
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.Affected)
	require.Equal(t, "mcp-x.tool_a", out.Entries[0].Key)
}

func TestCustomizeTools_Export_Import_RoundTrip(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

	// Set an override directly.
	require.NoError(t, store.SetOverride(overrides.ScopeUser, "mock.tool_one", overrides.OverrideEntry{
		Description: "exported desc", SourceHash: "abc",
	}))

	// Export.
	_, exportOut, err := s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{
		Action: "export",
	})
	require.NoError(t, err)
	require.True(t, exportOut.OK)
	require.Equal(t, "export", exportOut.Action)
	require.Equal(t, 1, exportOut.Affected)
	require.NotNil(t, exportOut.Pack)
	require.Len(t, exportOut.Pack.Overrides, 1)
	require.Equal(t, "mock.tool_one", exportOut.Pack.Overrides[0].Key)

	// Serialise pack to JSON (simulates wire transfer).
	packJSON, err := json.Marshal(exportOut.Pack)
	require.NoError(t, err)

	// Import into a fresh store on the same server.
	store2, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store2.Close() })
	s.SetOverrideStoreForTesting(store2)

	_, importOut, err := s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{
		Action: "import",
		Data:   string(packJSON),
	})
	require.NoError(t, err)
	require.True(t, importOut.OK)
	require.Equal(t, "import", importOut.Action)
	require.Equal(t, 1, importOut.Affected)
	require.NotNil(t, importOut.ImportReport)
	require.Equal(t, 1, importOut.ImportReport.ImportedOverrides)

	got, ok := store2.GetOverride("mock.tool_one")
	require.True(t, ok)
	require.Equal(t, "exported desc", got.Description)
}

func TestCustomizeTools_Import_MissingData(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

	_, _, err = s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{
		Action: "import",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "data is required")
}

func TestCustomizeTools_Import_BadSchemaVersion(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

	_, _, err = s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{
		Action: "import",
		Data:   `{"schema_version":999}`,
	})
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "schema")
}

func TestCustomizeTools_WrapperUnmarshal(t *testing.T) {
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

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

// newListNoConnectServer builds a server with an override store and one MCP
// configured with a marker command: if any list action eagerly connects it,
// the marker file appears.
func newListNoConnectServer(t *testing.T) (*Server, string) {
	t.Helper()
	s := newCustomizeTestServer(t)
	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

	cfg, markerPath := markerMCPConfig(t, "ghost")
	s.registry.SetConfigured(cfg)
	return s, markerPath
}

// TestCustomizeTools_ListOverrides_UnknownMCP_NoConnect: listing overrides for
// an MCP that is neither connected nor cached must not trigger a connection
// and must honestly report stale status as unknown.
func TestCustomizeTools_ListOverrides_UnknownMCP_NoConnect(t *testing.T) {
	s, markerPath := newListNoConnectServer(t)

	require.NoError(t, s.overrideStore.SetOverride(overrides.ScopeUser, "ghost.some_tool", overrides.OverrideEntry{
		Description: "custom desc",
		SourceHash:  "deadbeef",
	}))

	_, out, err := s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{Action: "list_overrides"})
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)
	require.False(t, out.Entries[0].Stale, "unknown upstream must not be reported stale")
	require.Equal(t, "unknown", out.Entries[0].StaleStatus)

	_, statErr := os.Stat(markerPath)
	require.True(t, os.IsNotExist(statErr), "list_overrides must not connect MCPs")
}

// TestCustomizeTools_ListOverrides_IndexedMCP_StillDetectsStale: stale
// detection keeps working from the index without connecting.
func TestCustomizeTools_ListOverrides_IndexedMCP_StillDetectsStale(t *testing.T) {
	s, _ := newListNoConnectServer(t)

	require.NoError(t, s.overrideStore.SetOverride(overrides.ScopeUser, "mock.tool_one", overrides.OverrideEntry{
		Description: "custom desc",
		SourceHash:  "stale-hash-that-wont-match",
	}))

	_, out, err := s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{Action: "list_overrides"})
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)
	require.True(t, out.Entries[0].Stale)
	require.Empty(t, out.Entries[0].StaleStatus)
	require.NotNil(t, out.Entries[0].StaleSource)
}

// TestCustomizeTools_ListCustom_UnknownDeps_NoConnect: listing custom tools
// with dependencies on unavailable MCPs must not connect them; the deps are
// reported as unknown instead.
func TestCustomizeTools_ListCustom_UnknownDeps_NoConnect(t *testing.T) {
	s, markerPath := newListNoConnectServer(t)

	require.NoError(t, s.overrideStore.SetCustom(overrides.ScopeUser, "combo", overrides.CustomTool{
		Description: "combined tool",
		InputSchema: map[string]any{"type": "object"},
		Body:        `ghost.some_tool()`,
		DependsOn: []overrides.Dependency{
			{MCP: "ghost", Tool: "some_tool", Hash: "deadbeef"},
		},
	}))

	_, out, err := s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{Action: "list_custom"})
	require.NoError(t, err)
	require.Len(t, out.CustomEntries, 1)
	entry := out.CustomEntries[0]
	require.False(t, entry.Stale, "unknown deps must not be reported stale")
	require.Empty(t, entry.StaleDeps)
	require.Equal(t, []string{"ghost.some_tool"}, entry.UnknownDeps)

	_, statErr := os.Stat(markerPath)
	require.True(t, os.IsNotExist(statErr), "list_custom must not connect MCPs")
}
