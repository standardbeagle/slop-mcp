package server

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/logging"
	"github.com/standardbeagle/slop-mcp/internal/overrides"
	"github.com/standardbeagle/slop-mcp/internal/registry"
	"github.com/stretchr/testify/require"
)

// newStaleTestServer builds a server with one mock tool, an override store,
// and the MCP marked cached so get_metadata serves tools from the index.
func newStaleTestServer(t *testing.T) *Server {
	t.Helper()
	s := mockServer(nil)
	s.logger = logging.Default()
	s.registry.AddToolsForTesting("mock", []registry.ToolInfo{
		{
			Name:        "tool_one",
			Description: "Upstream description for tool_one",
			MCPName:     "mock",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "search query"},
				},
			},
		},
	})
	s.registry.MarkCachedForTesting("mock")

	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)
	return s
}

// setOverride runs the set_override action and returns the stored hash.
func setOverride(t *testing.T, s *Server, desc string) string {
	t.Helper()
	_, out, err := s.handleCustomizeTools(context.Background(), nil, CustomizeToolsInput{
		Action: "set_override", MCP: "mock", Tool: "tool_one", Description: desc, Scope: "user",
	})
	require.NoError(t, err)
	require.True(t, out.OK)
	require.Len(t, out.Entries, 1)
	return out.Entries[0].Hash
}

// TestSetOverride_NotStaleAfterIndexRebuild is a regression test:
// buildIndex swaps the override description into the index, so stale
// detection must hash the preserved upstream description, not the override.
// Previously the override became permanently Stale after the index rebuild.
func TestSetOverride_NotStaleAfterIndexRebuild(t *testing.T) {
	s := newStaleTestServer(t)
	ctx := context.Background()

	hash1 := setOverride(t, s, "SHORT override")

	// list_overrides after the rebuild must not report stale.
	_, out, err := s.handleCustomizeTools(ctx, nil, CustomizeToolsInput{Action: "list_overrides"})
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)
	require.False(t, out.Entries[0].Stale, "override must not be stale right after set_override")

	// search_tools must show the override description without a stale flag.
	_, sOut, err := s.handleSearchTools(ctx, &mcp.CallToolRequest{}, SearchToolsInput{Query: "tool_one"})
	require.NoError(t, err)
	require.NotEmpty(t, sOut.Tools)
	found := false
	for _, tool := range sOut.Tools {
		if tool.MCPName == "mock" && tool.Name == "tool_one" {
			found = true
			require.Equal(t, "SHORT override", tool.Description)
			require.False(t, tool.Stale, "search_tools must not flag a fresh override stale")
		}
	}
	require.True(t, found)

	// Re-saving the override must hash the same upstream baseline, not the
	// previous override description (which would corrupt the baseline).
	hash2 := setOverride(t, s, "SHORT override v2")
	require.Equal(t, hash1, hash2, "re-save must preserve the upstream hash baseline")
}

// TestGetMetadata_StaleConsistency_CompactVsVerbose is a regression test:
// the compact path used to strip InputSchema before computing the stale hash,
// producing a different hash (nil params) than the verbose path.
func TestGetMetadata_StaleConsistency_CompactVsVerbose(t *testing.T) {
	s := newStaleTestServer(t)
	ctx := context.Background()

	setOverride(t, s, "SHORT override")

	// Query without an mcp_name filter: filtering triggers a lazy-connect,
	// which would flip this fake cached MCP into error state.
	staleOf := func(verbose bool) (stale bool, desc string) {
		t.Helper()
		_, out, err := s.handleGetMetadata(ctx, &mcp.CallToolRequest{}, GetMetadataInput{
			Verbose: verbose,
		})
		require.NoError(t, err)
		for _, m := range out.Metadata {
			if m.Name != "mock" {
				continue
			}
			require.Len(t, m.Tools, 1)
			return m.Tools[0].Stale, m.Tools[0].Description
		}
		t.Fatal("mock MCP not found in metadata")
		return false, ""
	}

	compactStale, compactDesc := staleOf(false)
	verboseStale, verboseDesc := staleOf(true)

	require.False(t, compactStale, "fresh override must not be stale in compact mode")
	require.False(t, verboseStale, "fresh override must not be stale in verbose mode")
	require.Equal(t, "SHORT override", compactDesc)
	require.Equal(t, "SHORT override", verboseDesc)

	// Force a genuinely stale override and verify both modes agree on stale=true.
	require.NoError(t, s.overrideStore.SetOverride(overrides.ScopeUser, "mock.tool_one", overrides.OverrideEntry{
		Description: "SHORT override",
		SourceHash:  "deadbeefdeadbeef",
	}))
	s.rebuildOverrideIndex()

	compactStale, _ = staleOf(false)
	verboseStale, _ = staleOf(true)
	require.True(t, compactStale, "compact mode must detect stale override")
	require.True(t, verboseStale, "verbose mode must detect stale override")
}

// TestGetMetadata_ParamMerge_DoesNotPolluteIndex is a regression test:
// merging override param descriptions used to mutate the InputSchema map
// shared with the immutable index snapshot for cached MCPs.
func TestGetMetadata_ParamMerge_DoesNotPolluteIndex(t *testing.T) {
	s := newStaleTestServer(t)
	ctx := context.Background()

	_, _, err := s.handleCustomizeTools(ctx, nil, CustomizeToolsInput{
		Action: "set_override", MCP: "mock", Tool: "tool_one",
		Description: "SHORT override",
		Params:      map[string]string{"query": "OVERRIDDEN param desc"},
		Scope:       "user",
	})
	require.NoError(t, err)

	// Verbose get_metadata merges param descriptions into the returned schema.
	// No mcp_name filter: filtering triggers a lazy-connect that would flip
	// this fake cached MCP into error state.
	_, out, err := s.handleGetMetadata(ctx, &mcp.CallToolRequest{}, GetMetadataInput{
		Verbose: true,
	})
	require.NoError(t, err)
	var merged map[string]any
	for _, m := range out.Metadata {
		if m.Name == "mock" {
			require.Len(t, m.Tools, 1)
			merged = m.Tools[0].InputSchema
		}
	}
	require.NotNil(t, merged)
	mergedProps := merged["properties"].(map[string]any)
	mergedQuery := mergedProps["query"].(map[string]any)
	require.Equal(t, "OVERRIDDEN param desc", mergedQuery["description"])

	// The index snapshot must still hold the upstream param description.
	indexTools := s.registry.SearchTools("tool_one", "mock")
	require.NotEmpty(t, indexTools)
	for _, tool := range indexTools {
		if tool.MCPName != "mock" || tool.Name != "tool_one" {
			continue
		}
		props := tool.InputSchema["properties"].(map[string]any)
		query := props["query"].(map[string]any)
		require.Equal(t, "search query", query["description"],
			"param merge must not mutate the shared index schema")
	}

	// And stale detection (which hashes upstream params) still reports fresh.
	_, lo, err := s.handleCustomizeTools(ctx, nil, CustomizeToolsInput{Action: "list_overrides"})
	require.NoError(t, err)
	require.Len(t, lo.Entries, 1)
	require.False(t, lo.Entries[0].Stale, "param merge must not corrupt stale detection")
}
