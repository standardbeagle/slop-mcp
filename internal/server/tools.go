package server

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTools registers all server tools.
func (s *Server) registerTools() {
	// 1. search_tools - Search registered MCP tools
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "search_tools",
		Description: "Search and explore all registered MCP tools by name and description",
	}, s.handleSearchTools)

	// 2. execute_tool - Execute an MCP tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "execute_tool",
		Description: "Execute a tool on a specific MCP server. Pass through the tool name and parameters directly.",
	}, s.handleExecuteTool)

	// 3. run_slop - Execute a SLOP script
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "run_slop",
		Description: "Execute a SLOP script with access to all registered MCPs. Provide either an inline script or a file path.",
	}, s.handleRunSlop)

	// 4. manage_mcps - Register, unregister, or list MCP servers
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "manage_mcps",
		Description: "Manage MCP server connections: register new servers, unregister existing ones, or list all registered servers",
	}, s.handleManageMCPs)
}
