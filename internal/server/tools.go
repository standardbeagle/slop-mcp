package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/cli"
)

// registerTools registers all server tools with manually crafted schemas.
// This avoids the Go SDK's auto-generated schemas which use patterns like
// "type": ["null", "object"] that Claude Code's strict validator rejects.
func (s *Server) registerTools() {
	// 1. search_tools - Search registered MCP tools
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:        "search_tools",
			Description: "Fuzzy search tools across connected MCPs. Ranked results. Paginated (default: 20, max: 100). Use offset for next page. Response includes total and has_more.",
			InputSchema: searchToolsInputSchema,
		},
		s.wrapSearchTools,
	)

	// 2. execute_tool - Execute an MCP tool
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:        "execute_tool",
			Description: "Execute tool on MCP server. Passes parameters through. Returns underlying MCP response as-is.",
			InputSchema: executeToolInputSchema,
		},
		s.wrapExecuteTool,
	)

	// 3. run_slop - Execute a SLOP script
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name: "run_slop",
			Description: `Execute SLOP script with access to all registered MCPs. Inline script or file path. Returns final expression value as text.

Call MCP tools as mcp_name.tool_name(param: value). Example patterns:

Chain results between tools:
  data = api.fetch(id: 42)
  summary = ai.summarize(text: data["content"])
  emit(summary)

Loop and collect:
  results = []
  for id in [1, 2, 3]:
      results = results + [api.get(id: id)]
  emit(items: results, count: len(results))

Transform with builtins:
  repos = github.search(query: "mcp")
  names = map(repos, |r| r["name"])
  emit(join(names, "\n"))

Pipe for chaining transforms (left value becomes first arg):
  [1, 2, 3, 4, 5] | filter(|x| x > 2) | map(|x| x * 10)
  data | json_stringify()

Session memory persists across run_slop calls (thread-safe):
  store_set("key", value)
  prev = store_get("key")

Persistent memory survives restarts (disk-backed):
  mem_save("bank", "key", value, description: "what this stores")
  data = mem_load("bank", "key")
  entries = mem_list("bank")
  matches = mem_search("query")

Use recipe parameter: recipe: "list" to see available templates, recipe: "<name>" to load one.
Use slop_reference to browse built-in functions (map, filter, reduce, json_parse, regex_match, etc.).`,
			InputSchema: runSlopInputSchema,
		},
		s.wrapRunSlop,
	)

	// 4. manage_mcps - Register, unregister, or list MCP servers
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:        "manage_mcps",
			Description: "Manage MCP connections. Actions: register, unregister, reconnect, list, status. Returns text.",
			InputSchema: manageMCPsInputSchema,
		},
		s.wrapManageMCPs,
	)

	// 5. auth_mcp - Authenticate with MCP servers using OAuth
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:        "auth_mcp",
			Description: "OAuth for MCP servers. Actions: login (start flow), logout (drop token), status (check), list (all authenticated). Returns text.",
			InputSchema: authMCPInputSchema,
		},
		s.wrapAuthMCP,
	)

	// 6. get_metadata - Get full metadata for all MCPs
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:        "get_metadata",
			Description: "Metadata for connected MCPs. Compact by default (names+descriptions). verbose=true for full schemas. mcp_name+tool_name for single-tool schema.",
			InputSchema: getMetadataInputSchema,
		},
		s.wrapGetMetadata,
	)

	// 7. slop_reference - Search SLOP language built-in functions
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:        "slop_reference",
			Description: "Search SLOP built-in functions. Compact output (name+signature) by default. verbose=true for full details. list_categories=true for category counts.",
			InputSchema: slopReferenceInputSchema,
		},
		s.wrapSlopReference,
	)

	// 8. slop_help - Get detailed help for a specific SLOP function
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:        "slop_help",
			Description: "Full details for SLOP function by name. Returns formatted text.",
			InputSchema: slopHelpInputSchema,
		},
		s.wrapSlopHelp,
	)

	// 9. agnt_watch - Build a shell command to stream agnt daemon events.
	// Pairs with Claude Code's Monitor tool: take the returned `command`
	// and run it as a persistent monitor source.
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:        "agnt_watch",
			Description: "Build shell command streaming agnt daemon events via `agnt monitor`. Returns `command` for Claude Code Monitor tool or any shell runner. Filters: target (errors/interactions/process/all), proxy_id, process_id, severity, format.",
			InputSchema: agntWatchInputSchema,
		},
		s.wrapAgntWatch,
	)

	// 10. customize_tools - Override tool descriptions and define custom tools.
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:        "customize_tools",
			Description: "Override tool descriptions, define custom tools. Actions: set_override, remove_override, list_overrides, define_custom, remove_custom, list_custom, export, import.",
			InputSchema: customizeToolsInputSchema,
		},
		s.wrapCustomizeTools,
	)
}

// Wrapper handlers that parse JSON manually and call the typed handlers.

func (s *Server) wrapSearchTools(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input SearchToolsInput
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return errorResult(fmt.Errorf("invalid parameters: %w", err)), nil
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
		return errorResult(fmt.Errorf("invalid parameters: %w", err)), nil
	}

	if input.MCPName == "" {
		return errorResult(fmt.Errorf("mcp_name is required")), nil
	}
	if input.ToolName == "" {
		return errorResult(fmt.Errorf("tool_name is required")), nil
	}

	// Handle CLI tools (mcp_name is "cli" or tool_name has cli_ prefix)
	if input.MCPName == "cli" || cli.IsCLITool(input.ToolName) {
		_, output, err := s.handleExecuteTool(ctx, req, input)
		if err != nil {
			return errorResult(err), nil
		}
		return toCallToolResult(output)
	}

	// For MCP tools, pass through the raw response from the underlying MCP
	result, err := s.registry.ExecuteToolRaw(ctx, input.MCPName, input.ToolName, input.Parameters)
	if err != nil {
		return errorResult(err), nil
	}

	return result, nil
}

func (s *Server) wrapRunSlop(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input RunSlopInput
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return errorResult(fmt.Errorf("invalid parameters: %w", err)), nil
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
		return errorResult(fmt.Errorf("invalid parameters: %w", err)), nil
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
		return errorResult(fmt.Errorf("invalid parameters: %w", err)), nil
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
		return errorResult(fmt.Errorf("invalid parameters: %w", err)), nil
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
		return errorResult(fmt.Errorf("invalid parameters: %w", err)), nil
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
		return errorResult(fmt.Errorf("invalid parameters: %w", err)), nil
	}

	_, output, err := s.handleSlopHelp(ctx, req, input)
	if err != nil {
		return errorResult(err), nil
	}

	return toCallToolResult(output)
}

func (s *Server) wrapAgntWatch(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input AgntWatchInput
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return errorResult(fmt.Errorf("invalid parameters: %w", err)), nil
	}

	// Protect against clients probing the internal override field.
	input.AgntBinary = ""

	_, output, err := s.handleAgntWatch(ctx, req, input)
	if err != nil {
		return errorResult(err), nil
	}

	return toCallToolResult(output)
}

func (s *Server) wrapCustomizeTools(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input CustomizeToolsInput
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return errorResult(fmt.Errorf("invalid parameters: %w", err)), nil
	}

	_, output, err := s.handleCustomizeTools(ctx, req, input)
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
