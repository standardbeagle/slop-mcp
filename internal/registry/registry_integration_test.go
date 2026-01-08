// +build integration

package registry_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getMockMCPPath returns the path to the mock MCP binary
func getMockMCPPath(t *testing.T) string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not get caller info")
	}
	// From internal/registry/, go to internal/server/testdata/mock-mcp/
	return filepath.Join(filepath.Dir(filename), "..", "server", "testdata", "mock-mcp", "mock-mcp")
}

// getTestMCPs returns the MCP configs for testing.
// When USE_MOCK_MCP=1 is set, uses the mock MCP server for fast tests.
func getTestMCPs(t *testing.T) map[string]config.MCPConfig {
	if os.Getenv("USE_MOCK_MCP") == "1" {
		mockPath := getMockMCPPath(t)
		return map[string]config.MCPConfig{
			"everything": {
				Name:    "everything",
				Type:    "stdio",
				Command: mockPath,
			},
			// Mock MCP can simulate memory MCP for basic tests
			"memory": {
				Name:    "memory",
				Type:    "stdio",
				Command: mockPath,
			},
		}
	}

	// Use real MCPs (slower, requires npx)
	return map[string]config.MCPConfig{
		"everything": {
			Name:    "everything",
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-everything"},
		},
		"memory": {
			Name:    "memory",
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-memory"},
		},
		"filesystem": {
			Name:    "filesystem",
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
		},
	}
}

func TestRegistry_ConnectEverything(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reg := registry.New()
	defer reg.Close()

	err := reg.Connect(ctx, getTestMCPs(t)["everything"])
	require.NoError(t, err, "failed to connect to everything MCP")

	// Verify connection
	list := reg.List()
	require.Len(t, list, 1)
	assert.Equal(t, "everything", list[0].Name)
	assert.True(t, list[0].Connected)
	assert.Greater(t, list[0].ToolCount, 0, "everything MCP should have tools")
}

func TestRegistry_ConnectMultipleMCPs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	reg := registry.New()
	defer reg.Close()

	// Connect everything and memory MCPs
	err := reg.Connect(ctx, getTestMCPs(t)["everything"])
	require.NoError(t, err, "failed to connect to everything MCP")

	err = reg.Connect(ctx, getTestMCPs(t)["memory"])
	require.NoError(t, err, "failed to connect to memory MCP")

	// Verify both connected
	list := reg.List()
	require.Len(t, list, 2)

	names := make(map[string]bool)
	for _, mcp := range list {
		names[mcp.Name] = mcp.Connected
	}
	assert.True(t, names["everything"])
	assert.True(t, names["memory"])
}

func TestRegistry_SearchTools(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reg := registry.New()
	defer reg.Close()

	err := reg.Connect(ctx, getTestMCPs(t)["everything"])
	require.NoError(t, err)

	// Search for echo tool
	tools := reg.SearchTools("echo", "")
	require.NotEmpty(t, tools, "should find echo tool")

	found := false
	for _, tool := range tools {
		if tool.Name == "echo" {
			found = true
			assert.Equal(t, "everything", tool.MCPName)
			break
		}
	}
	assert.True(t, found, "should find echo tool in everything MCP")

	// Search for add tool
	tools = reg.SearchTools("add", "")
	found = false
	for _, tool := range tools {
		if tool.Name == "add" {
			found = true
			break
		}
	}
	assert.True(t, found, "should find add tool")

	// Search filtered by MCP name
	tools = reg.SearchTools("", "everything")
	assert.NotEmpty(t, tools, "should find tools in everything MCP")
	for _, tool := range tools {
		assert.Equal(t, "everything", tool.MCPName)
	}
}

func TestRegistry_ExecuteEchoTool(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reg := registry.New()
	defer reg.Close()

	err := reg.Connect(ctx, getTestMCPs(t)["everything"])
	require.NoError(t, err)

	// Execute echo tool
	result, err := reg.ExecuteTool(ctx, "everything", "echo", map[string]any{
		"message": "Hello, MCP!",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Result should contain the echoed message
	resultStr := formatResult(result)
	assert.Contains(t, resultStr, "Hello, MCP!")
}

func TestRegistry_ExecuteAddTool(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reg := registry.New()
	defer reg.Close()

	err := reg.Connect(ctx, getTestMCPs(t)["everything"])
	require.NoError(t, err)

	// Execute add tool
	result, err := reg.ExecuteTool(ctx, "everything", "add", map[string]any{
		"a": 5,
		"b": 3,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Result should contain 8
	resultStr := formatResult(result)
	assert.Contains(t, resultStr, "8")
}

func TestRegistry_ExecuteToolChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reg := registry.New()
	defer reg.Close()

	err := reg.Connect(ctx, getTestMCPs(t)["everything"])
	require.NoError(t, err)

	// Chain: add(2, 3) -> echo result
	addResult, err := reg.ExecuteTool(ctx, "everything", "add", map[string]any{
		"a": 2,
		"b": 3,
	})
	require.NoError(t, err)

	// Echo the result
	echoResult, err := reg.ExecuteTool(ctx, "everything", "echo", map[string]any{
		"message": formatResult(addResult),
	})
	require.NoError(t, err)
	assert.Contains(t, formatResult(echoResult), "5")
}

func TestRegistry_ExecuteToolLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	reg := registry.New()
	defer reg.Close()

	err := reg.Connect(ctx, getTestMCPs(t)["everything"])
	require.NoError(t, err)

	// Loop: add numbers 1+2+3+4+5 = 15
	sum := 0
	for i := 1; i <= 5; i++ {
		result, err := reg.ExecuteTool(ctx, "everything", "add", map[string]any{
			"a": sum,
			"b": i,
		})
		require.NoError(t, err)

		// Parse result to get new sum
		// Result format: "The sum of X and Y is Z."
		// We need to extract Z (the last number)
		resultStr := formatResult(result)
		words := strings.Fields(resultStr)
		// Find the last number in the result
		for j := len(words) - 1; j >= 0; j-- {
			if n := parseNumber(words[j]); n > 0 || words[j] == "0" || strings.HasPrefix(words[j], "0") {
				sum = n
				break
			}
		}
	}

	assert.Equal(t, 15, sum, "sum of 1+2+3+4+5 should be 15")
}

func TestRegistry_MemoryMCP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("USE_MOCK_MCP") == "1" {
		t.Skip("skipping memory MCP test in mock mode (mock doesn't simulate memory tools)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reg := registry.New()
	defer reg.Close()

	err := reg.Connect(ctx, getTestMCPs(t)["memory"])
	require.NoError(t, err)

	// Search for memory tools
	tools := reg.SearchTools("", "memory")
	require.NotEmpty(t, tools, "memory MCP should have tools")

	// Log available tools for debugging
	t.Logf("Memory MCP tools: %v", toolNames(tools))
}

func TestRegistry_CombineMultipleMCPs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	reg := registry.New()
	defer reg.Close()

	// Connect both MCPs
	err := reg.Connect(ctx, getTestMCPs(t)["everything"])
	require.NoError(t, err)

	err = reg.Connect(ctx, getTestMCPs(t)["memory"])
	require.NoError(t, err)

	// Execute tool from everything
	echoResult, err := reg.ExecuteTool(ctx, "everything", "echo", map[string]any{
		"message": "test-entity",
	})
	require.NoError(t, err)
	assert.NotNil(t, echoResult)

	// Search for tools across all MCPs
	allTools := reg.SearchTools("", "")
	everythingTools := reg.SearchTools("", "everything")
	memoryTools := reg.SearchTools("", "memory")

	assert.Equal(t, len(allTools), len(everythingTools)+len(memoryTools),
		"all tools should be sum of individual MCP tools")
}

func TestRegistry_DisconnectMCP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reg := registry.New()
	defer reg.Close()

	err := reg.Connect(ctx, getTestMCPs(t)["everything"])
	require.NoError(t, err)

	// Verify connected
	list := reg.List()
	require.Len(t, list, 1)

	// Disconnect
	err = reg.Disconnect("everything")
	require.NoError(t, err)

	// Verify disconnected
	list = reg.List()
	assert.Len(t, list, 0)

	// Tools should no longer be searchable
	tools := reg.SearchTools("echo", "")
	assert.Empty(t, tools)
}

func TestRegistry_ExecuteNonexistentTool(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reg := registry.New()
	defer reg.Close()

	err := reg.Connect(ctx, getTestMCPs(t)["everything"])
	require.NoError(t, err)

	// Try to execute non-existent tool
	_, err = reg.ExecuteTool(ctx, "everything", "nonexistent_tool", nil)
	assert.Error(t, err)
}

func TestRegistry_ExecuteOnNonexistentMCP(t *testing.T) {
	ctx := context.Background()

	reg := registry.New()
	defer reg.Close()

	// Try to execute on non-existent MCP
	_, err := reg.ExecuteTool(ctx, "nonexistent", "echo", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// Helper functions

func formatResult(result any) string {
	if result == nil {
		return ""
	}

	switch v := result.(type) {
	case string:
		return v
	case map[string]any:
		if content, ok := v["content"]; ok {
			return formatResult(content)
		}
		if text, ok := v["text"]; ok {
			return formatResult(text)
		}
	case []any:
		var parts []string
		for _, item := range v {
			parts = append(parts, formatResult(item))
		}
		return strings.Join(parts, " ")
	}

	return ""
}

func parseNumber(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else if n > 0 {
			break
		}
	}
	return n
}

func toolNames(tools []registry.ToolInfo) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}
