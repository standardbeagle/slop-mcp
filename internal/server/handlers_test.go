package server

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/cli"
	"github.com/standardbeagle/slop-mcp/internal/registry"
	"github.com/stretchr/testify/assert"
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
