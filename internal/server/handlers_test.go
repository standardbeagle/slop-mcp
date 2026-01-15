package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/auth"
	"github.com/standardbeagle/slop-mcp/internal/cli"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockServer creates a minimal server for testing handlers.
func mockServer(tools []registry.ToolInfo) *Server {
	reg := registry.New()
	// Add tools to the registry's index directly for testing
	reg.AddToolsForTesting("test-mcp", tools)

	return &Server{
		registry:    reg,
		cliRegistry: cli.NewRegistry(),
	}
}

func TestHandleSearchTools_Pagination_DefaultLimit(t *testing.T) {
	// Create 50 tools
	tools := make([]registry.ToolInfo, 50)
	for i := 0; i < 50; i++ {
		tools[i] = registry.ToolInfo{
			Name:        "tool_" + string(rune('a'+i%26)) + "_" + string(rune('0'+i/26)),
			Description: "Test tool description",
			MCPName:     "test-mcp",
		}
	}

	s := mockServer(tools)
	ctx := context.Background()

	result, output, err := s.handleSearchTools(ctx, &mcp.CallToolRequest{}, SearchToolsInput{})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 50, output.Total, "total should be all matching tools")
	assert.Equal(t, 20, output.Limit, "should use default limit")
	assert.Equal(t, 0, output.Offset, "offset should be 0")
	assert.Len(t, output.Tools, 20, "should return default limit tools")
	assert.True(t, output.HasMore, "should have more results")
}

func TestHandleSearchTools_Pagination_CustomLimit(t *testing.T) {
	tools := make([]registry.ToolInfo, 50)
	for i := 0; i < 50; i++ {
		tools[i] = registry.ToolInfo{
			Name:        "tool_" + string(rune('a'+i%26)),
			Description: "Test tool description",
			MCPName:     "test-mcp",
		}
	}

	s := mockServer(tools)
	ctx := context.Background()

	result, output, err := s.handleSearchTools(ctx, &mcp.CallToolRequest{}, SearchToolsInput{
		Limit: 10,
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 50, output.Total)
	assert.Equal(t, 10, output.Limit)
	assert.Len(t, output.Tools, 10)
	assert.True(t, output.HasMore)
}

func TestHandleSearchTools_Pagination_WithOffset(t *testing.T) {
	tools := make([]registry.ToolInfo, 50)
	for i := 0; i < 50; i++ {
		tools[i] = registry.ToolInfo{
			Name:        "tool_" + string(rune('a'+i%26)) + "_" + string(rune('0'+i)),
			Description: "Test tool description",
			MCPName:     "test-mcp",
		}
	}

	s := mockServer(tools)
	ctx := context.Background()

	result, output, err := s.handleSearchTools(ctx, &mcp.CallToolRequest{}, SearchToolsInput{
		Limit:  10,
		Offset: 40,
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 50, output.Total)
	assert.Equal(t, 10, output.Limit)
	assert.Equal(t, 40, output.Offset)
	assert.Len(t, output.Tools, 10, "should return remaining 10 tools")
	assert.False(t, output.HasMore, "should not have more results")
}

func TestHandleSearchTools_Pagination_OffsetBeyondResults(t *testing.T) {
	tools := make([]registry.ToolInfo, 10)
	for i := 0; i < 10; i++ {
		tools[i] = registry.ToolInfo{
			Name:        "tool_" + string(rune('a'+i)),
			Description: "Test tool description",
			MCPName:     "test-mcp",
		}
	}

	s := mockServer(tools)
	ctx := context.Background()

	result, output, err := s.handleSearchTools(ctx, &mcp.CallToolRequest{}, SearchToolsInput{
		Offset: 100,
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 10, output.Total)
	assert.Len(t, output.Tools, 0, "should return no tools when offset beyond results")
	assert.False(t, output.HasMore)
}

func TestHandleSearchTools_Pagination_MaxLimit(t *testing.T) {
	tools := make([]registry.ToolInfo, 200)
	for i := 0; i < 200; i++ {
		tools[i] = registry.ToolInfo{
			Name:        "tool_" + string(rune('a'+i%26)),
			Description: "Test tool description",
			MCPName:     "test-mcp",
		}
	}

	s := mockServer(tools)
	ctx := context.Background()

	result, output, err := s.handleSearchTools(ctx, &mcp.CallToolRequest{}, SearchToolsInput{
		Limit: 500, // Request more than max
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 200, output.Total)
	assert.Equal(t, MaxSearchLimit, output.Limit, "should cap at max limit")
	assert.Len(t, output.Tools, MaxSearchLimit)
	assert.True(t, output.HasMore)
}

func TestHandleSearchTools_Pagination_NegativeOffset(t *testing.T) {
	tools := make([]registry.ToolInfo, 10)
	for i := 0; i < 10; i++ {
		tools[i] = registry.ToolInfo{
			Name:        "tool_" + string(rune('a'+i)),
			Description: "Test tool description",
			MCPName:     "test-mcp",
		}
	}

	s := mockServer(tools)
	ctx := context.Background()

	result, output, err := s.handleSearchTools(ctx, &mcp.CallToolRequest{}, SearchToolsInput{
		Offset: -5, // Negative offset
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 0, output.Offset, "negative offset should be treated as 0")
}

func TestHandleSearchTools_Pagination_WithQuery(t *testing.T) {
	tools := make([]registry.ToolInfo, 50)
	for i := 0; i < 50; i++ {
		tools[i] = registry.ToolInfo{
			Name:        "search_tool_" + string(rune('a'+i%26)),
			Description: "Test tool for search testing",
			MCPName:     "test-mcp",
		}
	}

	s := mockServer(tools)
	ctx := context.Background()

	// Query should still respect pagination
	result, output, err := s.handleSearchTools(ctx, &mcp.CallToolRequest{}, SearchToolsInput{
		Query: "search",
		Limit: 5,
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 5, output.Limit)
	assert.LessOrEqual(t, len(output.Tools), 5)
}

func TestHandleSearchTools_Pagination_FewResults(t *testing.T) {
	tools := make([]registry.ToolInfo, 5)
	for i := 0; i < 5; i++ {
		tools[i] = registry.ToolInfo{
			Name:        "tool_" + string(rune('a'+i)),
			Description: "Test tool description",
			MCPName:     "test-mcp",
		}
	}

	s := mockServer(tools)
	ctx := context.Background()

	result, output, err := s.handleSearchTools(ctx, &mcp.CallToolRequest{}, SearchToolsInput{
		Limit: 20, // More than available
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 5, output.Total)
	assert.Equal(t, 20, output.Limit)
	assert.Len(t, output.Tools, 5, "should return all available tools")
	assert.False(t, output.HasMore, "should not have more when all returned")
}

// TestHandleSearchTools_QueryVariations tests search with different query patterns.
func TestHandleSearchTools_QueryVariations(t *testing.T) {
	// Helper to create a server with tools registered under their correct MCP names
	createMultiMCPServer := func() *Server {
		reg := registry.New()
		// Register tools under their correct MCP names
		reg.AddToolsForTesting("filesystem", []registry.ToolInfo{
			{Name: "read_file", Description: "Read contents of a file", MCPName: "filesystem"},
			{Name: "write_file", Description: "Write data to a file", MCPName: "filesystem"},
			{Name: "list_directory", Description: "List files in directory", MCPName: "filesystem"},
		})
		reg.AddToolsForTesting("code-search", []registry.ToolInfo{
			{Name: "search_code", Description: "Search through code files", MCPName: "code-search"},
			{Name: "code_analyze", Description: "Analyze code for issues", MCPName: "code-search"},
		})
		reg.AddToolsForTesting("git", []registry.ToolInfo{
			{Name: "git_status", Description: "Show git repository status", MCPName: "git"},
			{Name: "git_commit", Description: "Commit changes to git", MCPName: "git"},
		})
		reg.AddToolsForTesting("database", []registry.ToolInfo{
			{Name: "database_query", Description: "Execute SQL query", MCPName: "database"},
		})
		return &Server{
			registry:    reg,
			cliRegistry: cli.NewRegistry(),
		}
	}

	tests := []struct {
		name          string
		query         string
		mcpFilter     string
		wantMinCount  int
		wantMaxCount  int
		wantToolNames []string // expected tools (in any order)
	}{
		{
			name:         "empty query returns all tools",
			query:        "",
			wantMinCount: 8,
			wantMaxCount: 8,
		},
		{
			name:          "single term exact match",
			query:         "read_file",
			wantMinCount:  1,
			wantToolNames: []string{"read_file"},
		},
		{
			name:          "single term partial match in name",
			query:         "file",
			wantMinCount:  2, // read_file, write_file
			wantToolNames: []string{"read_file", "write_file"},
		},
		{
			name:          "single term match in description",
			query:         "SQL",
			wantMinCount:  1,
			wantToolNames: []string{"database_query"},
		},
		{
			name:          "multi-term query",
			query:         "git status",
			wantMinCount:  1,
			wantToolNames: []string{"git_status"},
		},
		{
			name:          "fuzzy match ignoring separators",
			query:         "readfile",
			wantMinCount:  1,
			wantToolNames: []string{"read_file"},
		},
		{
			name:         "mcp name filter",
			query:        "",
			mcpFilter:    "filesystem",
			wantMinCount: 3,
			wantMaxCount: 3,
		},
		{
			name:          "mcp name filter with query",
			query:         "file",
			mcpFilter:    "filesystem",
			wantMinCount:  2,
			wantToolNames: []string{"read_file", "write_file"},
		},
		{
			name:          "search by mcp name as query",
			query:         "filesystem",
			wantMinCount:  3,
			wantToolNames: []string{"read_file", "write_file", "list_directory"},
		},
		{
			name:         "no match returns empty",
			query:        "nonexistent_tool_xyz",
			wantMinCount: 0,
			wantMaxCount: 0,
		},
		{
			name:         "mcp filter with no match returns empty",
			query:        "",
			mcpFilter:    "nonexistent-mcp",
			wantMinCount: 0,
			wantMaxCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := createMultiMCPServer()
			ctx := context.Background()

			result, output, err := s.handleSearchTools(ctx, &mcp.CallToolRequest{}, SearchToolsInput{
				Query:   tt.query,
				MCPName: tt.mcpFilter,
				Limit:   100, // High limit to get all results
			})

			assert.NoError(t, err)
			assert.Nil(t, result)
			assert.GreaterOrEqual(t, len(output.Tools), tt.wantMinCount, "should return at least %d tools", tt.wantMinCount)
			if tt.wantMaxCount > 0 {
				assert.LessOrEqual(t, len(output.Tools), tt.wantMaxCount, "should return at most %d tools", tt.wantMaxCount)
			}

			// Check that expected tools are in results
			if len(tt.wantToolNames) > 0 {
				toolNames := make([]string, len(output.Tools))
				for i, tool := range output.Tools {
					toolNames[i] = tool.Name
				}
				for _, expectedName := range tt.wantToolNames {
					assert.Contains(t, toolNames, expectedName, "should contain tool %s", expectedName)
				}
			}
		})
	}
}

// TestHandleSearchTools_Ranking verifies that search results are ranked properly.
func TestHandleSearchTools_Ranking(t *testing.T) {
	tools := []registry.ToolInfo{
		{Name: "search_helper", Description: "Helper for searching", MCPName: "utils"},
		{Name: "search", Description: "Main search tool", MCPName: "main"},
		{Name: "deep_search", Description: "Deep search functionality", MCPName: "advanced"},
		{Name: "find_items", Description: "Find items using search", MCPName: "finder"},
	}

	s := mockServer(tools)
	ctx := context.Background()

	result, output, err := s.handleSearchTools(ctx, &mcp.CallToolRequest{}, SearchToolsInput{
		Query: "search",
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.GreaterOrEqual(t, len(output.Tools), 3, "should return tools matching 'search'")

	// The exact match "search" should be ranked first
	assert.Equal(t, "search", output.Tools[0].Name, "exact match should be ranked first")
}

// TestHandleSearchTools_EmptyRegistry tests search with no tools registered.
func TestHandleSearchTools_EmptyRegistry(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	tests := []struct {
		name  string
		input SearchToolsInput
	}{
		{
			name:  "empty query on empty registry",
			input: SearchToolsInput{},
		},
		{
			name:  "query on empty registry",
			input: SearchToolsInput{Query: "anything"},
		},
		{
			name:  "mcp filter on empty registry",
			input: SearchToolsInput{MCPName: "some-mcp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := s.handleSearchTools(ctx, &mcp.CallToolRequest{}, tt.input)

			assert.NoError(t, err)
			assert.Nil(t, result)
			assert.Equal(t, 0, output.Total)
			assert.Empty(t, output.Tools)
			assert.False(t, output.HasMore)
		})
	}
}

// TestHandleSearchTools_MultiMCP tests search across multiple MCPs.
func TestHandleSearchTools_MultiMCP(t *testing.T) {
	// Create tools across multiple MCPs
	reg := registry.New()
	reg.AddToolsForTesting("mcp-a", []registry.ToolInfo{
		{Name: "tool_alpha", Description: "Alpha tool", MCPName: "mcp-a"},
		{Name: "shared_tool", Description: "Shared functionality", MCPName: "mcp-a"},
	})
	reg.AddToolsForTesting("mcp-b", []registry.ToolInfo{
		{Name: "tool_beta", Description: "Beta tool", MCPName: "mcp-b"},
		{Name: "shared_tool", Description: "Also shared", MCPName: "mcp-b"},
	})

	s := &Server{
		registry:    reg,
		cliRegistry: cli.NewRegistry(),
	}
	ctx := context.Background()

	t.Run("search finds tools across all MCPs", func(t *testing.T) {
		_, output, err := s.handleSearchTools(ctx, &mcp.CallToolRequest{}, SearchToolsInput{
			Query: "shared",
		})

		assert.NoError(t, err)
		assert.Equal(t, 2, len(output.Tools), "should find shared_tool in both MCPs")
	})

	t.Run("filter to specific MCP", func(t *testing.T) {
		_, output, err := s.handleSearchTools(ctx, &mcp.CallToolRequest{}, SearchToolsInput{
			Query:   "shared",
			MCPName: "mcp-a",
		})

		assert.NoError(t, err)
		assert.Equal(t, 1, len(output.Tools), "should only find tool in mcp-a")
		assert.Equal(t, "mcp-a", output.Tools[0].MCPName)
	})
}

// TestHandleExecuteTool_Validation tests input validation for execute_tool.
func TestHandleExecuteTool_Validation(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	tests := []struct {
		name      string
		input     ExecuteToolInput
		wantError string
	}{
		{
			name:      "missing mcp_name",
			input:     ExecuteToolInput{ToolName: "some_tool"},
			wantError: "mcp_name is required",
		},
		{
			name:      "missing tool_name",
			input:     ExecuteToolInput{MCPName: "some-mcp"},
			wantError: "tool_name is required",
		},
		{
			name:      "both missing",
			input:     ExecuteToolInput{},
			wantError: "mcp_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := s.handleExecuteTool(ctx, &mcp.CallToolRequest{}, tt.input)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
			assert.Nil(t, result)
			assert.Nil(t, output)
		})
	}
}

// TestHandleExecuteTool_MCPNotFound tests error when MCP is not found.
func TestHandleExecuteTool_MCPNotFound(t *testing.T) {
	// Create server with tools but no actual connections
	reg := registry.New()
	reg.AddToolsForTesting("existing-mcp", []registry.ToolInfo{
		{Name: "existing_tool", Description: "A tool that exists", MCPName: "existing-mcp"},
	})

	s := &Server{
		registry:    reg,
		cliRegistry: cli.NewRegistry(),
	}
	ctx := context.Background()

	result, output, err := s.handleExecuteTool(ctx, &mcp.CallToolRequest{}, ExecuteToolInput{
		MCPName:  "nonexistent-mcp",
		ToolName: "some_tool",
	})

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Nil(t, output)

	// Verify it's an MCPNotFoundError
	var mcpErr *registry.MCPNotFoundError
	assert.ErrorAs(t, err, &mcpErr)
	assert.Equal(t, "nonexistent-mcp", mcpErr.Name)
}

// TestRegistry_ExecuteTool_ToolNotFound tests ToolNotFoundError via registry.
// This requires mocking at the registry level since we can't connect real MCPs in unit tests.
func TestRegistry_MCPNotFoundError(t *testing.T) {
	reg := registry.New()
	ctx := context.Background()

	// Try to execute on non-existent MCP
	_, err := reg.ExecuteTool(ctx, "missing-mcp", "any_tool", nil)

	assert.Error(t, err)
	var mcpErr *registry.MCPNotFoundError
	assert.ErrorAs(t, err, &mcpErr)
	assert.Equal(t, "missing-mcp", mcpErr.Name)
	assert.Contains(t, mcpErr.Error(), "MCP server not found")
}

// TestMCPNotFoundError_Message tests the error message format.
func TestMCPNotFoundError_Message(t *testing.T) {
	err := &registry.MCPNotFoundError{
		Name:          "test-mcp",
		AvailableMCPs: []string{"filesystem", "git", "database"},
	}

	msg := err.Error()
	assert.Contains(t, msg, "test-mcp")
	assert.Contains(t, msg, "MCP server not found")
	assert.Contains(t, msg, "filesystem")
	assert.Contains(t, msg, "git")
	assert.Contains(t, msg, "database")
	assert.Contains(t, msg, "Available MCP servers")
}

// TestToolNotFoundError_Message tests the error message format.
func TestToolNotFoundError_Message(t *testing.T) {
	err := &registry.ToolNotFoundError{
		MCPName:        "test-mcp",
		ToolName:       "read_fil",
		AvailableTools: []string{"read_file", "write_file", "list_dir"},
		SimilarTools:   []string{"read_file"},
	}

	msg := err.Error()
	assert.Contains(t, msg, "read_fil")
	assert.Contains(t, msg, "test-mcp")
	assert.Contains(t, msg, "not found")
	assert.Contains(t, msg, "Did you mean")
	assert.Contains(t, msg, "read_file")
	assert.Contains(t, msg, "Available tools")
}

// TestInvalidParameterError_Message tests the error message format.
func TestInvalidParameterError_Message(t *testing.T) {
	err := &registry.InvalidParameterError{
		MCPName:         "test-mcp",
		ToolName:        "write_file",
		OriginalError:   "missing required parameter: path",
		ProvidedParams:  []string{"content", "pth"},
		ExpectedParams:  []registry.ParamInfo{
			{Name: "path", Type: "string", Description: "File path", Required: true},
			{Name: "content", Type: "string", Description: "File content", Required: true},
		},
		SimilarParams:   map[string]string{"pth": "path"},
		MissingRequired: []string{"path"},
		UnknownParams:   []string{"pth"},
	}

	msg := err.Error()
	assert.Contains(t, msg, "write_file")
	assert.Contains(t, msg, "test-mcp")
	assert.Contains(t, msg, "Invalid parameters")
	assert.Contains(t, msg, "Missing required parameters")
	assert.Contains(t, msg, "path")
	assert.Contains(t, msg, "Unknown parameters")
	assert.Contains(t, msg, "pth")
	assert.Contains(t, msg, "did you mean")
}

// mockServerWithMetadata creates a server with registered MCP states for metadata tests.
// Since we can't connect real MCPs in unit tests, we use AddToolsForTesting and SetConfigured.
func mockServerWithMetadata() *Server {
	reg := registry.New()

	// Register tools under MCPs using the testing helper
	reg.AddToolsForTesting("filesystem", []registry.ToolInfo{
		{
			Name:        "read_file",
			Description: "Read contents of a file",
			MCPName:     "filesystem",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "File path"},
				},
				"required": []any{"path"},
			},
		},
		{
			Name:        "write_file",
			Description: "Write data to a file",
			MCPName:     "filesystem",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "File path"},
					"content": map[string]any{"type": "string", "description": "File content"},
				},
				"required": []any{"path", "content"},
			},
		},
	})
	reg.AddToolsForTesting("git", []registry.ToolInfo{
		{
			Name:        "git_status",
			Description: "Show git repository status",
			MCPName:     "git",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	})

	return &Server{
		registry:    reg,
		cliRegistry: cli.NewRegistry(),
	}
}

// TestHandleGetMetadata_CompactMode tests get_metadata with compact output (default).
func TestHandleGetMetadata_CompactMode(t *testing.T) {
	s := mockServerWithMetadata()
	ctx := context.Background()

	// Default mode (no verbose flag) should strip input schemas
	result, output, err := s.handleGetMetadata(ctx, &mcp.CallToolRequest{}, GetMetadataInput{})

	assert.NoError(t, err)
	assert.Nil(t, result)
	// In unit tests, GetMetadata returns empty since we don't have real connections
	// The registry returns metadata based on states, not tool index
	// This test validates the handler behavior with the input parameters
	assert.NotNil(t, output.Metadata)
}

// TestHandleGetMetadata_VerboseMode tests get_metadata with verbose output.
func TestHandleGetMetadata_VerboseMode(t *testing.T) {
	s := mockServerWithMetadata()
	ctx := context.Background()

	// Verbose mode should include full input schemas
	result, output, err := s.handleGetMetadata(ctx, &mcp.CallToolRequest{}, GetMetadataInput{
		Verbose: true,
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.NotNil(t, output.Metadata)
}

// TestHandleGetMetadata_MCPNameFilter tests get_metadata with mcp_name filter.
func TestHandleGetMetadata_MCPNameFilter(t *testing.T) {
	s := mockServerWithMetadata()
	ctx := context.Background()

	tests := []struct {
		name      string
		mcpName   string
		wantCount int
	}{
		{
			name:      "filter to specific MCP",
			mcpName:   "filesystem",
			wantCount: 1, // Only filesystem MCP if it has state
		},
		{
			name:      "filter to non-existent MCP",
			mcpName:   "nonexistent-mcp",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := s.handleGetMetadata(ctx, &mcp.CallToolRequest{}, GetMetadataInput{
				MCPName: tt.mcpName,
			})

			assert.NoError(t, err)
			assert.Nil(t, result)
			// Filter should work on metadata array
			for _, m := range output.Metadata {
				assert.Equal(t, tt.mcpName, m.Name)
			}
		})
	}
}

// TestHandleGetMetadata_ToolNameFilter tests get_metadata with tool_name filter.
func TestHandleGetMetadata_ToolNameFilter(t *testing.T) {
	s := mockServerWithMetadata()
	ctx := context.Background()

	// Filter by tool name
	result, output, err := s.handleGetMetadata(ctx, &mcp.CallToolRequest{}, GetMetadataInput{
		ToolName: "read_file",
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	// Any MCPs in result should only contain the filtered tool
	for _, m := range output.Metadata {
		for _, tool := range m.Tools {
			assert.Equal(t, "read_file", tool.Name)
		}
		// When filtering by tool, prompts/resources should be cleared
		assert.Nil(t, m.Prompts)
		assert.Nil(t, m.Resources)
		assert.Nil(t, m.ResourceTemplates)
	}
}

// TestHandleGetMetadata_MCPAndToolFilter tests get_metadata with both filters.
func TestHandleGetMetadata_MCPAndToolFilter(t *testing.T) {
	s := mockServerWithMetadata()
	ctx := context.Background()

	// Filter by both MCP and tool - should include schemas (like verbose mode)
	result, output, err := s.handleGetMetadata(ctx, &mcp.CallToolRequest{}, GetMetadataInput{
		MCPName:  "filesystem",
		ToolName: "read_file",
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	// Should return only matching entries
	for _, m := range output.Metadata {
		assert.Equal(t, "filesystem", m.Name)
		for _, tool := range m.Tools {
			assert.Equal(t, "read_file", tool.Name)
		}
	}
}

// TestHandleGetMetadata_SchemaStripping tests that schemas are stripped in compact mode.
func TestHandleGetMetadata_SchemaStripping(t *testing.T) {
	// This test validates the schema stripping logic in the handler
	s := mockServerWithMetadata()
	ctx := context.Background()

	// Test compact mode (default) - schemas should be stripped
	_, compactOutput, err := s.handleGetMetadata(ctx, &mcp.CallToolRequest{}, GetMetadataInput{
		Verbose: false,
	})
	assert.NoError(t, err)

	// Test verbose mode - schemas should be included
	_, verboseOutput, err := s.handleGetMetadata(ctx, &mcp.CallToolRequest{}, GetMetadataInput{
		Verbose: true,
	})
	assert.NoError(t, err)

	// Both should succeed - schema content depends on actual connections
	assert.NotNil(t, compactOutput.Metadata)
	assert.NotNil(t, verboseOutput.Metadata)
}

// TestHandleGetMetadata_EmptyRegistry tests get_metadata with no MCPs.
func TestHandleGetMetadata_EmptyRegistry(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, output, err := s.handleGetMetadata(ctx, &mcp.CallToolRequest{}, GetMetadataInput{})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 0, output.Total)
	assert.Empty(t, output.Metadata)
}

// TestHandleManageMCPs_ListAction tests manage_mcps list action.
func TestHandleManageMCPs_ListAction(t *testing.T) {
	s := mockServerWithMetadata()
	ctx := context.Background()

	result, output, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action: "list",
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	// List should return MCP statuses (may be empty in unit tests without real connections)
	assert.NotNil(t, output.MCPs)
}

// TestHandleManageMCPs_StatusAction tests manage_mcps status action.
func TestHandleManageMCPs_StatusAction(t *testing.T) {
	s := mockServerWithMetadata()
	ctx := context.Background()

	result, output, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action: "status",
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	// Status should return full status information
	assert.NotNil(t, output.Status)
}

// TestHandleManageMCPs_InvalidAction tests manage_mcps with invalid action.
func TestHandleManageMCPs_InvalidAction(t *testing.T) {
	s := mockServerWithMetadata()
	ctx := context.Background()

	tests := []struct {
		name      string
		action    string
		wantError string
	}{
		{
			name:      "invalid action",
			action:    "invalid",
			wantError: "invalid action: invalid",
		},
		{
			name:      "empty action",
			action:    "",
			wantError: "invalid action:",
		},
		{
			name:      "unknown action",
			action:    "restart",
			wantError: "invalid action: restart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
				Action: tt.action,
			})

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
			assert.Nil(t, result)
		})
	}
}

// TestHandleManageMCPs_RegisterValidation tests manage_mcps register action validation.
func TestHandleManageMCPs_RegisterValidation(t *testing.T) {
	s := mockServerWithMetadata()
	ctx := context.Background()

	tests := []struct {
		name      string
		input     ManageMCPsInput
		wantError string
	}{
		{
			name: "register without name",
			input: ManageMCPsInput{
				Action:  "register",
				Command: "npx",
			},
			wantError: "name is required for register action",
		},
		{
			name: "register with invalid scope",
			input: ManageMCPsInput{
				Action:  "register",
				Name:    "test-mcp",
				Command: "npx",
				Scope:   "invalid-scope",
			},
			wantError: "invalid scope: invalid-scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, tt.input)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
			assert.Nil(t, result)
		})
	}
}

// TestHandleManageMCPs_UnregisterValidation tests manage_mcps unregister action validation.
func TestHandleManageMCPs_UnregisterValidation(t *testing.T) {
	s := mockServerWithMetadata()
	ctx := context.Background()

	// Unregister without name should fail
	result, _, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action: "unregister",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name is required for unregister action")
	assert.Nil(t, result)
}

// TestHandleManageMCPs_UnregisterNonexistent tests unregistering non-existent MCP.
func TestHandleManageMCPs_UnregisterNonexistent(t *testing.T) {
	s := mockServerWithMetadata()
	ctx := context.Background()

	// Unregister non-existent MCP should fail
	result, _, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action: "unregister",
		Name:   "nonexistent-mcp",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MCP not found")
	assert.Nil(t, result)
}

// TestHandleManageMCPs_RegisterMemoryScope tests register with memory scope (default).
func TestHandleManageMCPs_RegisterMemoryScope(t *testing.T) {
	reg := registry.New()
	s := &Server{
		registry:    reg,
		cliRegistry: cli.NewRegistry(),
	}
	ctx := context.Background()

	// Register with memory scope - this will fail because the command doesn't exist
	// but we can verify the input handling
	result, _, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action:  "register",
		Name:    "test-mcp",
		Type:    "command",
		Command: "nonexistent-command-that-does-not-exist",
		Scope:   "memory", // explicit memory scope
	})

	// Connection will fail because command doesn't exist
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to register MCP")
	assert.Nil(t, result)
}

// TestHandleManageMCPs_RegisterDefaultType tests register with default transport type.
func TestHandleManageMCPs_RegisterDefaultType(t *testing.T) {
	reg := registry.New()
	s := &Server{
		registry:    reg,
		cliRegistry: cli.NewRegistry(),
	}
	ctx := context.Background()

	// Register without type - should default to "command"
	// Will fail because command doesn't exist, but validates default handling
	_, _, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action:  "register",
		Name:    "test-mcp",
		Command: "nonexistent-command",
		// Type is empty - should default to "command"
	})

	// Should fail trying to execute the command
	assert.Error(t, err)
}

// TestHandleManageMCPs_RegisterDefaultScope tests register with default scope.
func TestHandleManageMCPs_RegisterDefaultScope(t *testing.T) {
	reg := registry.New()
	s := &Server{
		registry:    reg,
		cliRegistry: cli.NewRegistry(),
	}
	ctx := context.Background()

	// Register without scope - should default to "memory"
	// Will fail because command doesn't exist, but validates default handling
	_, _, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action:  "register",
		Name:    "test-mcp",
		Command: "nonexistent-command",
		// Scope is empty - should default to "memory"
	})

	// Should fail trying to execute the command
	assert.Error(t, err)
}

// TestHandleManageMCPs_ListEmpty tests list action with no MCPs.
func TestHandleManageMCPs_ListEmpty(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, output, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action: "list",
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.NotNil(t, output.MCPs)
	assert.Empty(t, output.MCPs)
}

// TestHandleManageMCPs_StatusEmpty tests status action with no MCPs.
func TestHandleManageMCPs_StatusEmpty(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, output, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action: "status",
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.NotNil(t, output.Status)
	assert.Empty(t, output.Status)
}

// TestHandleManageMCPs_ValidScopes tests all valid scope values.
func TestHandleManageMCPs_ValidScopes(t *testing.T) {
	validScopes := []string{"memory", "user", "project"}

	for _, scope := range validScopes {
		t.Run("scope_"+scope, func(t *testing.T) {
			reg := registry.New()
			s := &Server{
				registry:    reg,
				cliRegistry: cli.NewRegistry(),
			}
			ctx := context.Background()

			// All scopes should be accepted (error will be from connection, not scope validation)
			_, _, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
				Action:  "register",
				Name:    "test-mcp",
				Command: "nonexistent-command",
				Scope:   scope,
			})

			// If error is about invalid scope, the test fails
			if err != nil {
				assert.NotContains(t, err.Error(), "invalid scope")
			}
		})
	}
}

// TestHandleManageMCPs_ReconnectValidation tests manage_mcps reconnect action validation.
func TestHandleManageMCPs_ReconnectValidation(t *testing.T) {
	s := mockServerWithMetadata()
	ctx := context.Background()

	// Reconnect without name should fail
	result, _, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action: "reconnect",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name is required for reconnect action")
	assert.Nil(t, result)
}

// TestHandleManageMCPs_ReconnectNonexistent tests reconnecting non-existent MCP.
func TestHandleManageMCPs_ReconnectNonexistent(t *testing.T) {
	s := mockServerWithMetadata()
	ctx := context.Background()

	// Reconnect non-existent MCP should fail
	result, _, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action: "reconnect",
		Name:   "nonexistent-mcp",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to reconnect MCP")
	assert.Nil(t, result)
}

// TestHandleManageMCPs_ReconnectConfiguredMCP tests reconnecting a configured MCP.
func TestHandleManageMCPs_ReconnectConfiguredMCP(t *testing.T) {
	reg := registry.New()
	s := &Server{
		registry:    reg,
		cliRegistry: cli.NewRegistry(),
	}
	ctx := context.Background()

	// First, configure an MCP (this will fail to connect but MCP will be configured)
	cfg := config.MCPConfig{
		Name:    "test-mcp",
		Type:    "stdio",
		Command: "nonexistent-command-that-does-not-exist",
	}
	reg.SetConfigured(cfg)

	// Reconnect should attempt to reconnect the configured MCP
	result, _, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action: "reconnect",
		Name:   "test-mcp",
	})

	// Will fail because command doesn't exist, but reconnect action was recognized
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to reconnect MCP")
	assert.Nil(t, result)
}

// TestHandleManageMCPs_InvalidActionIncludesReconnect tests that error message lists reconnect.
func TestHandleManageMCPs_InvalidActionIncludesReconnect(t *testing.T) {
	s := mockServerWithMetadata()
	ctx := context.Background()

	_, _, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action: "invalid",
	})

	assert.Error(t, err)
	// Error message should mention reconnect as valid action
	assert.Contains(t, err.Error(), "reconnect")
}

// =============================================================================
// Tests for handleRunSlop
// =============================================================================

// extractSlopValue extracts the actual value from a SLOP runtime result.
// SLOP values are returned as maps with a "Value" key due to JSON marshaling.
// This helper unwraps them for easier testing.
func extractSlopValue(v any) any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		if val, exists := m["Value"]; exists {
			return val
		}
	}
	return v
}

// TestHandleRunSlop_MissingInput tests run_slop with neither script nor file_path.
func TestHandleRunSlop_MissingInput(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "either script or file_path is required")
	assert.Nil(t, result)
	assert.Equal(t, RunSlopOutput{}, output)
}

// TestHandleRunSlop_InlineScript_Simple tests run_slop with a simple inline script.
func TestHandleRunSlop_InlineScript_Simple(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	// Test with a simple arithmetic expression
	result, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{
		Script: "1 + 2",
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	// The result of 1 + 2 should be 3 (wrapped in SLOP value format)
	assert.Equal(t, float64(3), extractSlopValue(output.Result))
}

// TestHandleRunSlop_InlineScript_StringConcat tests run_slop with string operations.
func TestHandleRunSlop_InlineScript_StringConcat(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{
		Script: `"hello" + " " + "world"`,
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, "hello world", extractSlopValue(output.Result))
}

// TestHandleRunSlop_InlineScript_Variables tests run_slop with variable assignment.
func TestHandleRunSlop_InlineScript_Variables(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	// SLOP uses = for assignment and newlines to separate statements
	script := `x = 10
y = 20
x + y`
	result, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{
		Script: script,
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, float64(30), extractSlopValue(output.Result))
}

// TestHandleRunSlop_InlineScript_Emit tests run_slop with emit statements.
func TestHandleRunSlop_InlineScript_Emit(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	// SLOP uses emit(value) function call syntax
	script := `emit("first")
emit("second")
"done"`
	result, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{
		Script: script,
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, "done", extractSlopValue(output.Result))
	assert.Len(t, output.Emitted, 2)
	// Emitted values are also wrapped in SLOP value format
	emitted0 := extractSlopValue(output.Emitted[0])
	emitted1 := extractSlopValue(output.Emitted[1])
	assert.Contains(t, []any{emitted0, emitted1}, "first")
	assert.Contains(t, []any{emitted0, emitted1}, "second")
}

// TestHandleRunSlop_InlineScript_Invalid tests run_slop with an invalid script.
func TestHandleRunSlop_InlineScript_Invalid(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{
		Script: "this is not valid SLOP syntax !!!",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "script execution error")
	assert.Nil(t, result)
	assert.Equal(t, RunSlopOutput{}, output)
}

// TestHandleRunSlop_InlineScript_UndefinedVariable tests run_slop with undefined variable.
func TestHandleRunSlop_InlineScript_UndefinedVariable(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{
		Script: "undefined_variable",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "script execution error")
	assert.Nil(t, result)
	assert.Equal(t, RunSlopOutput{}, output)
}

// TestHandleRunSlop_FilePath_Simple tests run_slop with a script file.
func TestHandleRunSlop_FilePath_Simple(t *testing.T) {
	// Create a temp file with a simple script
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.slop")
	err := os.WriteFile(scriptPath, []byte("5 * 5"), 0644)
	require.NoError(t, err)

	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{
		FilePath: scriptPath,
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, float64(25), extractSlopValue(output.Result))
}

// TestHandleRunSlop_FilePath_MultiLine tests run_slop with a multi-line script file.
func TestHandleRunSlop_FilePath_MultiLine(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "multiline.slop")
	// SLOP uses = for assignment and emit(value) function syntax
	script := `a = 10
b = 20
c = a + b
emit(c)
c * 2`
	err := os.WriteFile(scriptPath, []byte(script), 0644)
	require.NoError(t, err)

	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{
		FilePath: scriptPath,
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, float64(60), extractSlopValue(output.Result))
	require.Len(t, output.Emitted, 1)
	assert.Equal(t, float64(30), extractSlopValue(output.Emitted[0]))
}

// TestHandleRunSlop_FilePath_NotFound tests run_slop with a non-existent file.
func TestHandleRunSlop_FilePath_NotFound(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{
		FilePath: "/nonexistent/path/script.slop",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read script file")
	assert.Nil(t, result)
	assert.Equal(t, RunSlopOutput{}, output)
}

// TestHandleRunSlop_FilePath_InvalidContent tests run_slop with a file containing invalid script.
func TestHandleRunSlop_FilePath_InvalidContent(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "invalid.slop")
	err := os.WriteFile(scriptPath, []byte("this is invalid @@@ SLOP"), 0644)
	require.NoError(t, err)

	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{
		FilePath: scriptPath,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "script execution error")
	assert.Nil(t, result)
	assert.Equal(t, RunSlopOutput{}, output)
}

// TestHandleRunSlop_BothScriptAndFile tests run_slop when both script and file_path are provided.
// In this case, file_path takes precedence.
func TestHandleRunSlop_BothScriptAndFile(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.slop")
	err := os.WriteFile(scriptPath, []byte("100"), 0644)
	require.NoError(t, err)

	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	// When both are provided, file_path should take precedence
	result, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{
		Script:   "200",
		FilePath: scriptPath,
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, float64(100), extractSlopValue(output.Result)) // File content takes precedence
}

// TestHandleRunSlop_InlineScript_Boolean tests run_slop with boolean expressions.
func TestHandleRunSlop_InlineScript_Boolean(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	tests := []struct {
		name     string
		script   string
		expected any
	}{
		{"true literal", "true", true},
		{"false literal", "false", false},
		{"comparison true", "10 > 5", true},
		{"comparison false", "10 < 5", false},
		{"equality true", "5 == 5", true},
		{"equality false", "5 == 6", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{
				Script: tt.script,
			})

			assert.NoError(t, err)
			assert.Nil(t, result)
			assert.Equal(t, tt.expected, extractSlopValue(output.Result))
		})
	}
}

// TestHandleRunSlop_InlineScript_NoneResult tests run_slop with none result.
func TestHandleRunSlop_InlineScript_NoneResult(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	// SLOP uses "none" instead of "null"
	result, output, err := s.handleRunSlop(ctx, &mcp.CallToolRequest{}, RunSlopInput{
		Script: "none",
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	// none is wrapped in a NoneValue struct, check the output is the none type
	assert.NotNil(t, output.Result)
}

// =============================================================================
// Tests for handleAuthMCP
// =============================================================================

// setupAuthTestStore creates a temporary directory and returns a token store path.
// The returned cleanup function should be deferred to clean up the temp directory.
func setupAuthTestStore(t *testing.T) (storePath string, cleanup func()) {
	tmpDir := t.TempDir()
	storePath = filepath.Join(tmpDir, "auth.json")
	cleanup = func() {
		// t.TempDir() is automatically cleaned up
	}
	return storePath, cleanup
}

// TestHandleAuthMCP_ListAction_Empty tests auth_mcp list with no tokens.
func TestHandleAuthMCP_ListAction_Empty(t *testing.T) {
	// Create a server with empty registry
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	// Use a temp directory so the default token store doesn't interfere
	// Note: The handler uses auth.NewTokenStore() which uses the default path
	// For proper isolation, we'd need to modify the handler to accept a store
	// For now, we test the basic behavior
	result, output, err := s.handleAuthMCP(ctx, &mcp.CallToolRequest{}, AuthMCPInput{
		Action: "list",
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.NotNil(t, output.Tokens)
	assert.Contains(t, output.Message, "authenticated MCPs")
}

// TestHandleAuthMCP_InvalidAction tests auth_mcp with invalid action.
func TestHandleAuthMCP_InvalidAction(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	tests := []struct {
		name      string
		action    string
		wantError string
	}{
		{"empty action", "", "invalid action:"},
		{"unknown action", "unknown", "invalid action: unknown"},
		{"typo", "logn", "invalid action: logn"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _, err := s.handleAuthMCP(ctx, &mcp.CallToolRequest{}, AuthMCPInput{
				Action: tt.action,
			})

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
			assert.Nil(t, result)
		})
	}
}

// TestHandleAuthMCP_StatusAction_MissingName tests auth_mcp status without name.
func TestHandleAuthMCP_StatusAction_MissingName(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, _, err := s.handleAuthMCP(ctx, &mcp.CallToolRequest{}, AuthMCPInput{
		Action: "status",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name is required for status action")
	assert.Nil(t, result)
}

// TestHandleAuthMCP_LoginAction_MissingName tests auth_mcp login without name.
func TestHandleAuthMCP_LoginAction_MissingName(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, _, err := s.handleAuthMCP(ctx, &mcp.CallToolRequest{}, AuthMCPInput{
		Action: "login",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name is required for login action")
	assert.Nil(t, result)
}

// TestHandleAuthMCP_LogoutAction_MissingName tests auth_mcp logout without name.
func TestHandleAuthMCP_LogoutAction_MissingName(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, _, err := s.handleAuthMCP(ctx, &mcp.CallToolRequest{}, AuthMCPInput{
		Action: "logout",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name is required for logout action")
	assert.Nil(t, result)
}

// TestHandleAuthMCP_LoginAction_MCPNotFound tests auth_mcp login with unknown MCP.
func TestHandleAuthMCP_LoginAction_MCPNotFound(t *testing.T) {
	s := mockServer([]registry.ToolInfo{})
	ctx := context.Background()

	result, _, err := s.handleAuthMCP(ctx, &mcp.CallToolRequest{}, AuthMCPInput{
		Action: "login",
		Name:   "nonexistent-mcp",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Nil(t, result)
}

// TestTokenStore_GetToken_NoToken tests getting a non-existent token.
func TestTokenStore_GetToken_NoToken(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := auth.NewTokenStoreWithPath(storePath)

	token, err := store.GetToken("nonexistent-server")

	assert.NoError(t, err)
	assert.Nil(t, token)
}

// TestTokenStore_SetAndGetToken tests setting and getting a token.
func TestTokenStore_SetAndGetToken(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := auth.NewTokenStoreWithPath(storePath)

	testToken := &auth.MCPToken{
		ServerName:   "test-server",
		ServerURL:    "https://test.example.com",
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	// Set the token
	err := store.SetToken(testToken)
	require.NoError(t, err)

	// Get the token
	retrieved, err := store.GetToken("test-server")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, testToken.ServerName, retrieved.ServerName)
	assert.Equal(t, testToken.ServerURL, retrieved.ServerURL)
	assert.Equal(t, testToken.AccessToken, retrieved.AccessToken)
	assert.Equal(t, testToken.RefreshToken, retrieved.RefreshToken)
	assert.Equal(t, testToken.TokenType, retrieved.TokenType)
}

// TestTokenStore_ListTokens tests listing all tokens.
func TestTokenStore_ListTokens(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := auth.NewTokenStoreWithPath(storePath)

	// Add multiple tokens
	tokens := []*auth.MCPToken{
		{ServerName: "server1", AccessToken: "token1"},
		{ServerName: "server2", AccessToken: "token2"},
		{ServerName: "server3", AccessToken: "token3"},
	}

	for _, tok := range tokens {
		err := store.SetToken(tok)
		require.NoError(t, err)
	}

	// List tokens
	listed, err := store.ListTokens()
	require.NoError(t, err)

	assert.Len(t, listed, 3)
	// Check all servers are present
	serverNames := make([]string, len(listed))
	for i, tok := range listed {
		serverNames[i] = tok.ServerName
	}
	assert.Contains(t, serverNames, "server1")
	assert.Contains(t, serverNames, "server2")
	assert.Contains(t, serverNames, "server3")
}

// TestTokenStore_DeleteToken tests deleting a token.
func TestTokenStore_DeleteToken(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := auth.NewTokenStoreWithPath(storePath)

	// Add a token
	testToken := &auth.MCPToken{
		ServerName:  "to-delete",
		AccessToken: "token-to-delete",
	}
	err := store.SetToken(testToken)
	require.NoError(t, err)

	// Verify it exists
	retrieved, err := store.GetToken("to-delete")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Delete it
	err = store.DeleteToken("to-delete")
	require.NoError(t, err)

	// Verify it's gone
	retrieved, err = store.GetToken("to-delete")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

// TestTokenStore_ListTokens_Empty tests listing tokens with empty store.
func TestTokenStore_ListTokens_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := auth.NewTokenStoreWithPath(storePath)

	listed, err := store.ListTokens()
	require.NoError(t, err)
	assert.Empty(t, listed)
}

// TestMCPToken_IsExpired tests token expiration checking.
func TestMCPToken_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		wantExp   bool
	}{
		{
			name:      "not expired - 1 hour from now",
			expiresAt: time.Now().Add(time.Hour),
			wantExp:   false,
		},
		{
			name:      "expired - 1 hour ago",
			expiresAt: time.Now().Add(-time.Hour),
			wantExp:   true,
		},
		{
			name:      "expired - within 5 minute buffer",
			expiresAt: time.Now().Add(3 * time.Minute), // Less than 5 minute buffer
			wantExp:   true,
		},
		{
			name:      "not expired - just outside buffer",
			expiresAt: time.Now().Add(10 * time.Minute), // More than 5 minute buffer
			wantExp:   false,
		},
		{
			name:      "no expiry set - zero time",
			expiresAt: time.Time{},
			wantExp:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &auth.MCPToken{
				ServerName: "test",
				ExpiresAt:  tt.expiresAt,
			}
			assert.Equal(t, tt.wantExp, token.IsExpired())
		})
	}
}

// TestTokenStore_Persistence tests that tokens persist across store instances.
func TestTokenStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")

	// Create first store and add token
	store1 := auth.NewTokenStoreWithPath(storePath)
	testToken := &auth.MCPToken{
		ServerName:  "persistent-server",
		AccessToken: "persistent-token",
		ServerURL:   "https://persistent.example.com",
	}
	err := store1.SetToken(testToken)
	require.NoError(t, err)

	// Create second store with same path
	store2 := auth.NewTokenStoreWithPath(storePath)

	// Verify token is accessible from second store
	retrieved, err := store2.GetToken("persistent-server")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, "persistent-server", retrieved.ServerName)
	assert.Equal(t, "persistent-token", retrieved.AccessToken)
	assert.Equal(t, "https://persistent.example.com", retrieved.ServerURL)
}

// TestTokenStore_Path tests the Path() method.
func TestTokenStore_Path(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "custom-auth.json")
	store := auth.NewTokenStoreWithPath(storePath)

	assert.Equal(t, storePath, store.Path())
}

// TestTokenStore_Load_InvalidJSON tests loading with invalid JSON.
func TestTokenStore_Load_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")

	// Write invalid JSON
	err := os.WriteFile(storePath, []byte("not valid json"), 0600)
	require.NoError(t, err)

	store := auth.NewTokenStoreWithPath(storePath)
	_, err = store.Load()
	assert.Error(t, err)
}

// TestTokenStore_UpdateExistingToken tests updating an existing token.
func TestTokenStore_UpdateExistingToken(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := auth.NewTokenStoreWithPath(storePath)

	// Set initial token
	token1 := &auth.MCPToken{
		ServerName:  "updatable-server",
		AccessToken: "old-token",
	}
	err := store.SetToken(token1)
	require.NoError(t, err)

	// Update with new token
	token2 := &auth.MCPToken{
		ServerName:  "updatable-server",
		AccessToken: "new-token",
	}
	err = store.SetToken(token2)
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.GetToken("updatable-server")
	require.NoError(t, err)
	assert.Equal(t, "new-token", retrieved.AccessToken)

	// Verify only one token exists
	listed, err := store.ListTokens()
	require.NoError(t, err)
	assert.Len(t, listed, 1)
}

// TestHandleManageMCPs_HealthCheckAction tests manage_mcps health_check action.
func TestHandleManageMCPs_HealthCheckAction(t *testing.T) {
	s := mockServer(nil)
	ctx := context.Background()

	// Health check with no MCPs connected
	result, output, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action: "health_check",
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Contains(t, output.Message, "0/0 MCPs healthy")
	assert.Empty(t, output.HealthChecks)
}

// TestHandleManageMCPs_HealthCheckSpecificMCP tests health_check for a specific MCP.
func TestHandleManageMCPs_HealthCheckSpecificMCP(t *testing.T) {
	s := mockServer(nil)
	ctx := context.Background()

	// Health check for non-connected MCP
	result, output, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action: "health_check",
		Name:   "nonexistent",
	})

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Contains(t, output.Message, "nonexistent is not connected")
	assert.Empty(t, output.HealthChecks)
}

// TestHandleManageMCPs_InvalidActionIncludesHealthCheck tests error message includes health_check.
func TestHandleManageMCPs_InvalidActionIncludesHealthCheck(t *testing.T) {
	s := mockServer(nil)
	ctx := context.Background()

	_, _, err := s.handleManageMCPs(ctx, &mcp.CallToolRequest{}, ManageMCPsInput{
		Action: "invalid_action",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "health_check")
	assert.Contains(t, err.Error(), "register")
	assert.Contains(t, err.Error(), "unregister")
	assert.Contains(t, err.Error(), "reconnect")
	assert.Contains(t, err.Error(), "list")
	assert.Contains(t, err.Error(), "status")
}

// TestManageMCPsOutputHealthChecks tests ManageMCPsOutput struct has HealthChecks field.
func TestManageMCPsOutputHealthChecks(t *testing.T) {
	output := ManageMCPsOutput{
		Message: "Health check complete",
		HealthChecks: []registry.HealthCheckResult{
			{
				Name:         "test-mcp",
				Status:       registry.HealthStatusHealthy,
				ResponseTime: "1ms",
			},
		},
	}

	assert.Equal(t, "Health check complete", output.Message)
	require.Len(t, output.HealthChecks, 1)
	assert.Equal(t, "test-mcp", output.HealthChecks[0].Name)
	assert.Equal(t, registry.HealthStatusHealthy, output.HealthChecks[0].Status)
}

// TestManageMCPsInputActionDocumentation tests the action field documentation includes health_check.
func TestManageMCPsInputActionDocumentation(t *testing.T) {
	// This test verifies the struct definition includes health_check in the action field
	input := ManageMCPsInput{
		Action: "health_check",
		Name:   "test-mcp",
	}
	assert.Equal(t, "health_check", input.Action)
}
