package server

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTools registers all server tools with manually crafted schemas.
// This avoids the Go SDK's auto-generated schemas which use patterns like
// "type": ["null", "object"] that Claude Code's strict validator rejects.
func (s *Server) registerTools() {
	// 1. search_tools - Search registered MCP tools
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:         "search_tools",
			Description:  "Search and explore all registered MCP tools by name and description. Results are paginated (default: 20, max: 100). Use offset for subsequent pages. Response includes total count and has_more flag.",
			InputSchema:  searchToolsInputSchema,
			OutputSchema: searchToolsOutputSchema,
		},
		s.wrapSearchTools,
	)

	// 2. execute_tool - Execute an MCP tool
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:         "execute_tool",
			Description:  "Execute a tool on a specific MCP server. Pass through the tool name and parameters directly.",
			InputSchema:  executeToolInputSchema,
			OutputSchema: executeToolOutputSchema,
		},
		s.wrapExecuteTool,
	)

	// 3. run_slop - Execute a SLOP script
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:         "run_slop",
			Description:  "Execute a SLOP script with access to all registered MCPs. Provide either an inline script or a file path.",
			InputSchema:  runSlopInputSchema,
			OutputSchema: runSlopOutputSchema,
		},
		s.wrapRunSlop,
	)

	// 4. manage_mcps - Register, unregister, or list MCP servers
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:         "manage_mcps",
			Description:  "Manage MCP server connections: register new servers, unregister existing ones, or list all registered servers",
			InputSchema:  manageMCPsInputSchema,
			OutputSchema: manageMCPsOutputSchema,
		},
		s.wrapManageMCPs,
	)

	// 5. auth_mcp - Authenticate with MCP servers using OAuth
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:         "auth_mcp",
			Description:  "Authenticate with MCP servers using OAuth. Actions: login (initiate OAuth flow), logout (remove token), status (check auth status), list (show all authenticated MCPs)",
			InputSchema:  authMCPInputSchema,
			OutputSchema: authMCPOutputSchema,
		},
		s.wrapAuthMCP,
	)

	// 6. get_metadata - Get full metadata for all MCPs
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:         "get_metadata",
			Description:  "Get metadata for connected MCP servers. Returns tool names and descriptions by default (compact). Use verbose=true for full schemas, or specify both mcp_name and tool_name to get schema for a specific tool.",
			InputSchema:  getMetadataInputSchema,
			OutputSchema: getMetadataOutputSchema,
		},
		s.wrapGetMetadata,
	)

	// 7. slop_reference - Search SLOP language built-in functions
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:         "slop_reference",
			Description:  "Search SLOP built-in functions. Compact output (name+signature) by default. Use verbose=true for full details, list_categories=true for category counts.",
			InputSchema:  slopReferenceInputSchema,
			OutputSchema: slopReferenceOutputSchema,
		},
		s.wrapSlopReference,
	)

	// 8. slop_help - Get detailed help for a specific SLOP function
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:         "slop_help",
			Description:  "Get full details for a specific SLOP function by name.",
			InputSchema:  slopHelpInputSchema,
			OutputSchema: slopHelpOutputSchema,
		},
		s.wrapSlopHelp,
	)
}

// Wrapper handlers that parse JSON manually and call the typed handlers.

func (s *Server) wrapSearchTools(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input SearchToolsInput
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return nil, err
	}

	_, output, err := s.handleSearchTools(ctx, req, input)
	if err != nil {
		return errorResult(err), nil
	}

	return toCallToolResult(output)
}

func (s *Server) wrapExecuteTool(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input ExecuteToolInput
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return nil, err
	}

	_, output, err := s.handleExecuteTool(ctx, req, input)
	if err != nil {
		return errorResult(err), nil
	}

	return toCallToolResult(output)
}

func (s *Server) wrapRunSlop(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input RunSlopInput
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return nil, err
	}

	_, output, err := s.handleRunSlop(ctx, req, input)
	if err != nil {
		return errorResult(err), nil
	}

	return toCallToolResult(output)
}

func (s *Server) wrapManageMCPs(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input ManageMCPsInput
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return nil, err
	}

	_, output, err := s.handleManageMCPs(ctx, req, input)
	if err != nil {
		return errorResult(err), nil
	}

	return toCallToolResult(output)
}

func (s *Server) wrapAuthMCP(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input AuthMCPInput
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return nil, err
	}

	_, output, err := s.handleAuthMCP(ctx, req, input)
	if err != nil {
		return errorResult(err), nil
	}

	return toCallToolResult(output)
}

func (s *Server) wrapGetMetadata(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input GetMetadataInput
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return nil, err
	}

	_, output, err := s.handleGetMetadata(ctx, req, input)
	if err != nil {
		return errorResult(err), nil
	}

	return toCallToolResult(output)
}

func (s *Server) wrapSlopReference(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input SlopReferenceInput
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return nil, err
	}

	_, output, err := s.handleSlopReference(ctx, req, input)
	if err != nil {
		return errorResult(err), nil
	}

	return toCallToolResult(output)
}

func (s *Server) wrapSlopHelp(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input SlopHelpInput
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return nil, err
	}

	_, output, err := s.handleSlopHelp(ctx, req, input)
	if err != nil {
		return errorResult(err), nil
	}

	return toCallToolResult(output)
}

// errorResult creates an error CallToolResult.
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}

// toCallToolResult converts any output to a CallToolResult with JSON text content.
func toCallToolResult(output any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(output)
	if err != nil {
		return errorResult(err), nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil
}
