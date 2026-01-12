// +build integration

package server_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestMCPs returns the MCP configuration for testing.
// Set USE_MOCK_MCP=1 to use the fast mock MCP server.
// Otherwise uses the real @modelcontextprotocol/server-everything.
func getTestMCPs(t *testing.T) []config.MCPConfig {
	if os.Getenv("USE_MOCK_MCP") == "1" {
		// Use mock MCP for fast, deterministic tests
		mockPath := getMockMCPPath(t)
		return []config.MCPConfig{
			{
				Name:    "everything",
				Type:    "stdio",
				Command: mockPath,
			},
		}
	}

	// Use real everything MCP (slower, requires npx)
	return []config.MCPConfig{
		{
			Name:    "everything",
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-everything"},
		},
	}
}

// getMockMCPPath returns the path to the mock MCP binary.
func getMockMCPPath(t *testing.T) string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not get caller info")
	}
	testDir := filepath.Dir(filename)
	return filepath.Join(testDir, "testdata", "mock-mcp", "mock-mcp")
}

func setupServer(t *testing.T, mcps []config.MCPConfig) *server.Server {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)

	s, err := server.New(ctx, mcps)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	return s
}

func TestServer_SearchTools(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Call search_tools
	result, err := s.CallTool(ctx, "search_tools", map[string]any{
		"query": "echo",
	})
	require.NoError(t, err)

	// Parse result
	data, err := json.Marshal(result)
	require.NoError(t, err)

	var searchResult struct {
		Tools []struct {
			Name    string `json:"name"`
			MCPName string `json:"mcp_name"`
		} `json:"tools"`
		Total int `json:"total"`
	}
	err = json.Unmarshal(data, &searchResult)
	require.NoError(t, err)

	assert.Greater(t, searchResult.Total, 0, "should find echo tool")

	found := false
	for _, tool := range searchResult.Tools {
		if tool.Name == "echo" && tool.MCPName == "everything" {
			found = true
			break
		}
	}
	assert.True(t, found, "should find echo tool in everything MCP")
}

func TestServer_ExecuteTool_Echo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Execute echo tool
	result, err := s.CallTool(ctx, "execute_tool", map[string]any{
		"mcp_name":  "everything",
		"tool_name": "echo",
		"parameters": map[string]any{
			"message": "Hello from integration test!",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	resultStr := formatResult(result)
	assert.Contains(t, resultStr, "Hello from integration test!")
}

func TestServer_ExecuteTool_Add(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Execute add tool
	result, err := s.CallTool(ctx, "execute_tool", map[string]any{
		"mcp_name":  "everything",
		"tool_name": "add",
		"parameters": map[string]any{
			"a": 10,
			"b": 25,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	resultStr := formatResult(result)
	assert.Contains(t, resultStr, "35")
}

func TestServer_RunSlop_Simple(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Execute simple SLOP script that calls echo
	result, err := s.CallTool(ctx, "run_slop", map[string]any{
		"script": `
result = everything.echo(message: "SLOP test message")
emit(result)
`,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Check that the result contains our message
	data, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(data), "SLOP test message")
}

func TestServer_RunSlop_AddNumbers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Execute SLOP script that adds numbers
	result, err := s.CallTool(ctx, "run_slop", map[string]any{
		"script": `
sum = everything.add(a: 100, b: 200)
emit(sum)
`,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(data), "300")
}

func TestServer_RunSlop_ChainCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Chain: add(5, 10) -> echo the result
	result, err := s.CallTool(ctx, "run_slop", map[string]any{
		"script": `
# First add two numbers
sum = everything.add(a: 5, b: 10)

# Then echo the result
message = "The sum is: " + str(sum)
echoed = everything.echo(message: message)

emit(sum: sum, echoed: echoed)
`,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	t.Logf("Chain result: %s", string(data))
	assert.Contains(t, string(data), "15")
}

func TestServer_RunSlop_Loop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Loop: call echo multiple times on items in a list
	result, err := s.CallTool(ctx, "run_slop", map[string]any{
		"script": `
# Echo numbers in a list
messages = []
numbers = [1, 2, 3, 4, 5]

for num in numbers with limit(10):
    msg = "Number: " + str(num)
    result = everything.echo(message: msg)
    messages = messages + [result]

emit(messages: messages, count: len(numbers))
`,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	t.Logf("Loop result: %s", string(data))
	// Verify we got all 5 echo responses
	assert.Contains(t, string(data), "Number: 5")
	assert.Contains(t, string(data), "count")
}

func TestServer_RunSlop_CombineOutputs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Combine outputs from multiple tool calls
	result, err := s.CallTool(ctx, "run_slop", map[string]any{
		"script": `
# Make multiple independent echo calls
msg1 = everything.echo(message: "first")
msg2 = everything.echo(message: "second")
msg3 = everything.echo(message: "third")

# Combine results into a list
results = [msg1, msg2, msg3]

# Also do an add to show we can mix tool types
sum = everything.add(a: 10, b: 20)

emit(results: results, sum: sum, count: len(results))
`,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	t.Logf("Combine result: %s", string(data))
	// Should have all three echo messages
	assert.Contains(t, string(data), "first")
	assert.Contains(t, string(data), "second")
	assert.Contains(t, string(data), "third")
}

func TestServer_RunSlop_ConditionalToolCall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Conditional tool calls based on results
	result, err := s.CallTool(ctx, "run_slop", map[string]any{
		"script": `
# Get initial message
msg = everything.echo(message: "test condition")

# Check if result contains expected text
if "test" in msg:
    # Call another tool
    second = everything.echo(message: "condition met!")
    status = "matched"
else:
    status = "no match"

emit(msg: msg, second: second, status: status)
`,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	t.Logf("Conditional result: %s", string(data))
	assert.Contains(t, string(data), "condition met")
	assert.Contains(t, string(data), "matched")
}

func TestServer_RunSlop_WithFunctions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Define and use helper functions
	result, err := s.CallTool(ctx, "run_slop", map[string]any{
		"script": `
# Define a helper function that uses the MCP
def echo_items(items):
    results = []
    for item in items with limit(20):
        msg = "Item: " + str(item)
        result = everything.echo(message: msg)
        results = results + [result]
    return results

# Use the function
words = ["apple", "banana", "cherry"]
result = echo_items(words)

emit(results: result, count: len(words))
`,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	t.Logf("Function result: %s", string(data))
	// Should have echo results for all items
	assert.Contains(t, string(data), "apple")
	assert.Contains(t, string(data), "banana")
	assert.Contains(t, string(data), "cherry")
}

func TestServer_ManageMCPs_List(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// List MCPs
	result, err := s.CallTool(ctx, "manage_mcps", map[string]any{
		"action": "list",
	})
	require.NoError(t, err)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	t.Logf("List result: %s", string(data))

	var listResult struct {
		MCPs []struct {
			Name      string `json:"name"`
			Connected bool   `json:"connected"`
			ToolCount int    `json:"tool_count"`
		} `json:"mcps"`
	}
	err = json.Unmarshal(data, &listResult)
	require.NoError(t, err)

	require.Len(t, listResult.MCPs, 1)
	assert.Equal(t, "everything", listResult.MCPs[0].Name)
	assert.True(t, listResult.MCPs[0].Connected)
}

func TestServer_GetMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Get metadata for all MCPs
	result, err := s.CallTool(ctx, "get_metadata", map[string]any{})
	require.NoError(t, err)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	t.Logf("Metadata result: %s", string(data))

	var metaResult struct {
		Metadata []struct {
			Name  string `json:"name"`
			State string `json:"state"`
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"metadata"`
		Total int `json:"total"`
	}
	err = json.Unmarshal(data, &metaResult)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, metaResult.Total, 1, "should have at least 1 MCP")

	// Find everything MCP
	found := false
	for _, mcp := range metaResult.Metadata {
		if mcp.Name == "everything" {
			found = true
			assert.Equal(t, "connected", mcp.State)
			assert.NotEmpty(t, mcp.Tools, "should have tools")
			break
		}
	}
	assert.True(t, found, "should find everything MCP in metadata")
}

func TestServer_GetMetadata_FilterByMCP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Get metadata filtered by MCP name
	result, err := s.CallTool(ctx, "get_metadata", map[string]any{
		"mcp_name": "everything",
	})
	require.NoError(t, err)

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var metaResult struct {
		Metadata []struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Total int `json:"total"`
	}
	err = json.Unmarshal(data, &metaResult)
	require.NoError(t, err)

	assert.Equal(t, 1, metaResult.Total, "should return exactly 1 MCP")
	assert.Equal(t, "everything", metaResult.Metadata[0].Name)
}

func TestServer_GetMetadata_FilterByTool(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Get metadata filtered by tool name
	result, err := s.CallTool(ctx, "get_metadata", map[string]any{
		"tool_name": "echo",
	})
	require.NoError(t, err)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	t.Logf("Tool filter result: %s", string(data))

	var metaResult struct {
		Metadata []struct {
			Name  string `json:"name"`
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
			Prompts   []struct{} `json:"prompts"`
			Resources []struct{} `json:"resources"`
		} `json:"metadata"`
		Total int `json:"total"`
	}
	err = json.Unmarshal(data, &metaResult)
	require.NoError(t, err)

	// Should find at least one MCP with the echo tool
	assert.GreaterOrEqual(t, metaResult.Total, 1, "should find MCPs with echo tool")

	// Check that the returned tools only include "echo"
	for _, mcp := range metaResult.Metadata {
		for _, tool := range mcp.Tools {
			assert.Equal(t, "echo", tool.Name, "should only return echo tool")
		}
		// Prompts and resources should be cleared when filtering by tool
		assert.Empty(t, mcp.Prompts, "prompts should be cleared when filtering by tool")
		assert.Empty(t, mcp.Resources, "resources should be cleared when filtering by tool")
	}
}

func TestServer_GetMetadata_FilterByBoth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Get metadata filtered by both MCP and tool name
	result, err := s.CallTool(ctx, "get_metadata", map[string]any{
		"mcp_name":  "everything",
		"tool_name": "echo",
	})
	require.NoError(t, err)

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var metaResult struct {
		Metadata []struct {
			Name  string `json:"name"`
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"metadata"`
		Total int `json:"total"`
	}
	err = json.Unmarshal(data, &metaResult)
	require.NoError(t, err)

	assert.Equal(t, 1, metaResult.Total, "should return exactly 1 MCP")
	assert.Equal(t, "everything", metaResult.Metadata[0].Name)
	assert.Len(t, metaResult.Metadata[0].Tools, 1, "should return exactly 1 tool")
	assert.Equal(t, "echo", metaResult.Metadata[0].Tools[0].Name)
}

func TestServer_GetMetadata_NoMatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Get metadata with non-existent tool name
	result, err := s.CallTool(ctx, "get_metadata", map[string]any{
		"tool_name": "nonexistent_tool_xyz",
	})
	require.NoError(t, err)

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var metaResult struct {
		Metadata []struct{} `json:"metadata"`
		Total    int        `json:"total"`
	}
	err = json.Unmarshal(data, &metaResult)
	require.NoError(t, err)

	assert.Equal(t, 0, metaResult.Total, "should return 0 MCPs for non-existent tool")
}

func TestServer_ExecuteTool_InvalidParams_ShowsSuggestions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Execute echo tool with wrong parameter name (typo: "mesage" instead of "message")
	_, err := s.CallTool(ctx, "execute_tool", map[string]any{
		"mcp_name":  "everything",
		"tool_name": "echo",
		"parameters": map[string]any{
			"mesage": "Test typo", // intentional typo
		},
	})

	// Should get an error
	require.Error(t, err)

	// The error should contain helpful information about parameters
	errStr := err.Error()
	t.Logf("Error message: %s", errStr)

	// Check that the error contains parameter-related information
	// Note: The exact format depends on how the MCP server reports the error
	// and whether our enhancement kicks in
	assert.True(t,
		contains(errStr, "parameter") ||
			contains(errStr, "Expected") ||
			contains(errStr, "message") ||
			contains(errStr, "Invalid"),
		"error should contain parameter-related information")
}

func TestServer_ManageMCPs_RegisterUnregister(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Start with no MCPs
	s := setupServer(t, nil)
	ctx := context.Background()

	// Register everything MCP at runtime
	result, err := s.CallTool(ctx, "manage_mcps", map[string]any{
		"action":  "register",
		"name":    "everything",
		"type":    "command",
		"command": "npx",
		"args":    []string{"-y", "@modelcontextprotocol/server-everything"},
	})
	require.NoError(t, err)
	t.Logf("Register result: %v", result)

	// Verify it's registered
	result, err = s.CallTool(ctx, "manage_mcps", map[string]any{
		"action": "list",
	})
	require.NoError(t, err)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(data), "everything")

	// Use the newly registered MCP
	result, err = s.CallTool(ctx, "execute_tool", map[string]any{
		"mcp_name":  "everything",
		"tool_name": "echo",
		"parameters": map[string]any{
			"message": "Dynamic registration works!",
		},
	})
	require.NoError(t, err)
	assert.Contains(t, formatResult(result), "Dynamic registration works!")

	// Unregister
	result, err = s.CallTool(ctx, "manage_mcps", map[string]any{
		"action": "unregister",
		"name":   "everything",
	})
	require.NoError(t, err)

	// Verify it's gone
	result, err = s.CallTool(ctx, "manage_mcps", map[string]any{
		"action": "list",
	})
	require.NoError(t, err)

	data, err = json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"name":"everything"`)
}

// Helper function to format results for assertions
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
		data, _ := json.Marshal(v)
		return string(data)
	case []any:
		var parts []string
		for _, item := range v {
			if s := formatResult(item); s != "" {
				parts = append(parts, s)
			}
		}
		return join(parts, " ")
	}

	data, _ := json.Marshal(result)
	return string(data)
}

func join(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}

func TestServer_RunSlop_FromFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	// Test echo script from file
	result, err := s.CallTool(ctx, "run_slop", map[string]any{
		"file_path": "testdata/echo_test.slop",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	t.Logf("Echo from file: %s", string(data))
	assert.Contains(t, string(data), "Hello from SLOP file")
}

func TestServer_RunSlop_ChainFromFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	result, err := s.CallTool(ctx, "run_slop", map[string]any{
		"file_path": "testdata/chain_test.slop",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	t.Logf("Chain from file: %s", string(data))
	assert.Contains(t, string(data), "100") // final result
	assert.Contains(t, string(data), "30")  // first result
}

func TestServer_RunSlop_LoopFromFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	result, err := s.CallTool(ctx, "run_slop", map[string]any{
		"file_path": "testdata/loop_test.slop",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	t.Logf("Loop from file: %s", string(data))
	assert.Contains(t, string(data), "Number 10") // last echo message
}

func TestServer_RunSlop_CombineFromFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := setupServer(t, getTestMCPs(t))
	ctx := context.Background()

	result, err := s.CallTool(ctx, "run_slop", map[string]any{
		"file_path": "testdata/combine_test.slop",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	t.Logf("Combine from file: %s", string(data))
	assert.Contains(t, string(data), "first message")
	assert.Contains(t, string(data), "second message")
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
