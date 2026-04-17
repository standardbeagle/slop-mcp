//go:build integration

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/overrides"
	"github.com/standardbeagle/slop-mcp/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newE2EServer creates a server with a single mock MCP tool pre-registered.
// Uses AddToolsForTesting so no external processes are needed.
func newE2EServer(t *testing.T, userRoot string) *Server {
	t.Helper()
	tools := []registry.ToolInfo{
		{
			Name:        "greet",
			Description: "Greets the user by name",
			MCPName:     "mybot",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "person to greet"},
				},
			},
		},
	}
	s := mockServer(nil)
	s.registry.AddToolsForTesting("mybot", tools)

	store, err := overrides.OpenStore(overrides.StoreOptions{UserRoot: userRoot})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	s.SetOverrideStoreForTesting(store)

	return s
}

func TestE2E_CustomizeTools_FullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	userRoot := t.TempDir()
	s := newE2EServer(t, userRoot)

	var storedHash string
	var exportPackJSON string

	t.Run("set_override", func(t *testing.T) {
		_, out, err := s.handleCustomizeTools(ctx, nil, CustomizeToolsInput{
			Action:      "set_override",
			MCP:         "mybot",
			Tool:        "greet",
			Description: "Custom: says hello with extra flair",
			Scope:       "user",
		})
		require.NoError(t, err)
		assert.True(t, out.OK)
		assert.Equal(t, 1, out.Affected)
		require.Len(t, out.Entries, 1)
		assert.Equal(t, "mybot.greet", out.Entries[0].Key)
		assert.NotEmpty(t, out.Entries[0].Hash)
		storedHash = out.Entries[0].Hash
	})

	t.Run("search_uses_override", func(t *testing.T) {
		_, searchOut, err := s.handleSearchTools(ctx, nil, SearchToolsInput{
			Query:   "greet",
			MCPName: "mybot",
		})
		require.NoError(t, err)
		require.NotEmpty(t, searchOut.Tools)
		found := false
		for _, tool := range searchOut.Tools {
			if tool.MCPName == "mybot" && tool.Name == "greet" {
				found = true
				assert.Equal(t, "Custom: says hello with extra flair", tool.Description,
					"search result should reflect override description")
			}
		}
		assert.True(t, found, "greet tool not found in search results")
	})

	t.Run("export_import_roundtrip", func(t *testing.T) {
		// Export from the server's store (MCP selector = "mybot").
		_, exportOut, err := s.handleCustomizeTools(ctx, nil, CustomizeToolsInput{
			Action: "export",
			MCP:    "mybot",
		})
		require.NoError(t, err)
		assert.True(t, exportOut.OK)
		require.NotNil(t, exportOut.Pack)
		found := false
		for _, po := range exportOut.Pack.Overrides {
			if po.Key == "mybot.greet" {
				found = true
				break
			}
		}
		assert.True(t, found, "pack should contain the mybot.greet override")

		packBytes, err := json.Marshal(exportOut.Pack)
		require.NoError(t, err)
		exportPackJSON = string(packBytes)

		// Import into a fresh server with its own temp store.
		s2 := newE2EServer(t, t.TempDir())
		_, importOut, err := s2.handleCustomizeTools(ctx, nil, CustomizeToolsInput{
			Action:    "import",
			Data:      exportPackJSON,
			Overwrite: true,
			Scope:     "user",
		})
		require.NoError(t, err)
		assert.True(t, importOut.OK)
		assert.GreaterOrEqual(t, importOut.Affected, 1)
		require.NotNil(t, importOut.ImportReport)
		assert.GreaterOrEqual(t, importOut.ImportReport.ImportedOverrides, 1)

		// Verify the override is present in s2's store.
		e, ok := s2.overrideStore.GetOverride("mybot.greet")
		assert.True(t, ok, "override should be present after import")
		assert.Equal(t, "Custom: says hello with extra flair", e.Description)
	})

	t.Run("define_and_execute_custom", func(t *testing.T) {
		_, defOut, err := s.handleCustomizeTools(ctx, nil, CustomizeToolsInput{
			Action:      "define_custom",
			Name:        "echo",
			Description: "Returns 'echo: <message>'",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string"},
				},
			},
			Body:  `"echo: " + args["message"]`,
			Scope: "user",
		})
		require.NoError(t, err)
		assert.True(t, defOut.OK)
		assert.Equal(t, 1, defOut.Affected)

		// Execute the custom tool.
		_, execResult, err := s.handleExecuteTool(ctx, nil, ExecuteToolInput{
			MCPName:    "_custom",
			ToolName:   "echo",
			Parameters: map[string]any{"message": "hi"},
		})
		require.NoError(t, err, "execute_tool for _custom/echo should not error")
		// execResult is the raw Go value returned by executeCustomTool.
		// The body emits "echo: hi" — it may come back as a string or a slice.
		resultStr := strings.ToLower(fmt.Sprintf("%v", execResult))
		assert.Contains(t, resultStr, "echo", "custom tool result should contain 'echo'")
		assert.Contains(t, resultStr, "hi", "custom tool result should contain the input")
	})

	t.Run("stale_detection", func(t *testing.T) {
		// Mutate the stored hash to simulate the upstream tool changing.
		mutated := overrides.OverrideEntry{
			Description: "Custom: says hello with extra flair",
			SourceHash:  "0000000000000000", // deliberately wrong hash
		}
		err := s.overrideStore.SetOverride(overrides.ScopeUser, "mybot.greet", mutated)
		require.NoError(t, err)

		_, listOut, err := s.handleCustomizeTools(ctx, nil, CustomizeToolsInput{
			Action:    "list_overrides",
			StaleOnly: true,
		})
		require.NoError(t, err)
		assert.True(t, listOut.OK)
		require.NotEmpty(t, listOut.Entries, "stale entry should appear in stale_only list")
		assert.True(t, listOut.Entries[0].Stale, "entry should be marked stale")
		assert.NotNil(t, listOut.Entries[0].StaleSource, "stale_source should be populated")
		assert.Equal(t, "mybot.greet", listOut.Entries[0].Key)
		_ = storedHash // referenced to avoid unused-var; used for context above
	})

	t.Run("remove_override", func(t *testing.T) {
		// Re-establish a valid override first.
		_, _, err := s.handleCustomizeTools(ctx, nil, CustomizeToolsInput{
			Action:      "set_override",
			MCP:         "mybot",
			Tool:        "greet",
			Description: "Custom again",
			Scope:       "user",
		})
		require.NoError(t, err)

		// Now remove it.
		_, rmOut, err := s.handleCustomizeTools(ctx, nil, CustomizeToolsInput{
			Action: "remove_override",
			MCP:    "mybot",
			Tool:   "greet",
			Scope:  "user",
		})
		require.NoError(t, err)
		assert.True(t, rmOut.OK)
		assert.GreaterOrEqual(t, rmOut.Affected, 1)

		// Search should now return the original upstream description.
		_, searchOut, err := s.handleSearchTools(ctx, nil, SearchToolsInput{
			Query:   "greet",
			MCPName: "mybot",
		})
		require.NoError(t, err)
		for _, tool := range searchOut.Tools {
			if tool.MCPName == "mybot" && tool.Name == "greet" {
				assert.Equal(t, "Greets the user by name", tool.Description,
					"after removing override, upstream description should be restored")
			}
		}
	})
}
