package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/slop-mcp/internal/config"
	"github.com/anthropics/slop-mcp/internal/registry"
	"github.com/anthropics/slop/pkg/slop"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SearchToolsInput is the input for the search_tools tool.
type SearchToolsInput struct {
	Query   string `json:"query,omitempty" jsonschema:"description=Search query for tool names and descriptions"`
	MCPName string `json:"mcp_name,omitempty" jsonschema:"description=Filter to a specific MCP server"`
}

// SearchToolsOutput is the output for the search_tools tool.
type SearchToolsOutput struct {
	Tools []registry.ToolInfo `json:"tools"`
	Total int                 `json:"total"`
}

func (s *Server) handleSearchTools(
	ctx context.Context,
	req *mcp.CallToolRequest,
	input SearchToolsInput,
) (*mcp.CallToolResult, SearchToolsOutput, error) {
	tools := s.registry.SearchTools(input.Query, input.MCPName)

	return nil, SearchToolsOutput{
		Tools: tools,
		Total: len(tools),
	}, nil
}

// ExecuteToolInput is the input for the execute_tool tool.
type ExecuteToolInput struct {
	MCPName    string         `json:"mcp_name" jsonschema:"required,description=Target MCP server name"`
	ToolName   string         `json:"tool_name" jsonschema:"required,description=Tool to execute on the MCP server"`
	Parameters map[string]any `json:"parameters,omitempty" jsonschema:"description=Tool parameters to pass through"`
}

func (s *Server) handleExecuteTool(
	ctx context.Context,
	req *mcp.CallToolRequest,
	input ExecuteToolInput,
) (*mcp.CallToolResult, any, error) {
	if input.MCPName == "" {
		return nil, nil, fmt.Errorf("mcp_name is required")
	}
	if input.ToolName == "" {
		return nil, nil, fmt.Errorf("tool_name is required")
	}

	result, err := s.registry.ExecuteTool(ctx, input.MCPName, input.ToolName, input.Parameters)
	if err != nil {
		return nil, nil, err
	}

	return nil, result, nil
}

// RunSlopInput is the input for the run_slop tool.
type RunSlopInput struct {
	Script   string `json:"script,omitempty" jsonschema:"description=Inline SLOP script to execute"`
	FilePath string `json:"file_path,omitempty" jsonschema:"description=Path to a .slop file to execute"`
}

// RunSlopOutput is the output for the run_slop tool.
type RunSlopOutput struct {
	Result  any   `json:"result,omitempty"`
	Emitted []any `json:"emitted,omitempty"`
}

func (s *Server) handleRunSlop(
	ctx context.Context,
	req *mcp.CallToolRequest,
	input RunSlopInput,
) (*mcp.CallToolResult, RunSlopOutput, error) {
	if input.Script == "" && input.FilePath == "" {
		return nil, RunSlopOutput{}, fmt.Errorf("either script or file_path is required")
	}

	script := input.Script
	if input.FilePath != "" {
		data, err := os.ReadFile(input.FilePath)
		if err != nil {
			return nil, RunSlopOutput{}, fmt.Errorf("failed to read script file: %w", err)
		}
		script = string(data)
	}

	// Create SLOP runtime
	rt := slop.NewRuntime()
	defer rt.Close()

	// Connect all MCP services to the slop runtime
	for _, cfg := range s.registry.GetConfigs() {
		slopCfg := slop.MCPConfig{
			Name:    cfg.Name,
			Type:    cfg.Type,
			Command: cfg.Command,
			Args:    cfg.Args,
			Env:     mapToSlice(cfg.Env),
			URL:     cfg.URL,
			Headers: cfg.Headers,
		}
		if err := rt.ConnectMCP(ctx, slopCfg); err != nil {
			fmt.Printf("Warning: failed to connect MCP %s to slop runtime: %v\n", cfg.Name, err)
		}
	}

	// Execute script
	result, err := rt.Execute(script)
	if err != nil {
		return nil, RunSlopOutput{}, fmt.Errorf("script execution error: %w", err)
	}

	// Collect emitted values
	emitted := make([]any, 0)
	for _, v := range rt.Emitted() {
		emitted = append(emitted, valueToAny(v))
	}

	return nil, RunSlopOutput{
		Result:  valueToAny(result),
		Emitted: emitted,
	}, nil
}

// ManageMCPsInput is the input for the manage_mcps tool.
type ManageMCPsInput struct {
	Action  string            `json:"action" jsonschema:"required,description=Action to perform: register, unregister, or list"`
	Name    string            `json:"name,omitempty" jsonschema:"description=MCP server name (required for register/unregister)"`
	Type    string            `json:"type,omitempty" jsonschema:"description=Transport type: command (default), sse, or streamable"`
	Command string            `json:"command,omitempty" jsonschema:"description=Command executable for command transport"`
	Args    []string          `json:"args,omitempty" jsonschema:"description=Command arguments"`
	Env     map[string]string `json:"env,omitempty" jsonschema:"description=Environment variables"`
	URL     string            `json:"url,omitempty" jsonschema:"description=Server URL for HTTP transports"`
	Headers map[string]string `json:"headers,omitempty" jsonschema:"description=HTTP headers for HTTP transports"`
}

// ManageMCPsOutput is the output for the manage_mcps tool.
type ManageMCPsOutput struct {
	Message string               `json:"message,omitempty"`
	MCPs    []registry.MCPStatus `json:"mcps,omitempty"`
}

func (s *Server) handleManageMCPs(
	ctx context.Context,
	req *mcp.CallToolRequest,
	input ManageMCPsInput,
) (*mcp.CallToolResult, ManageMCPsOutput, error) {
	switch input.Action {
	case "register":
		if input.Name == "" {
			return nil, ManageMCPsOutput{}, fmt.Errorf("name is required for register action")
		}

		transportType := input.Type
		if transportType == "" {
			transportType = "command"
		}

		cfg := config.MCPConfig{
			Name:    input.Name,
			Type:    transportType,
			Command: input.Command,
			Args:    input.Args,
			Env:     input.Env,
			URL:     input.URL,
			Headers: input.Headers,
			Source:  config.SourceRuntime,
		}

		if err := s.registry.Connect(ctx, cfg); err != nil {
			return nil, ManageMCPsOutput{}, fmt.Errorf("failed to register MCP: %w", err)
		}

		return nil, ManageMCPsOutput{
			Message: fmt.Sprintf("Successfully registered MCP: %s", input.Name),
		}, nil

	case "unregister":
		if input.Name == "" {
			return nil, ManageMCPsOutput{}, fmt.Errorf("name is required for unregister action")
		}

		if err := s.registry.Disconnect(input.Name); err != nil {
			return nil, ManageMCPsOutput{}, err
		}

		return nil, ManageMCPsOutput{
			Message: fmt.Sprintf("Unregistered MCP: %s", input.Name),
		}, nil

	case "list":
		return nil, ManageMCPsOutput{
			MCPs: s.registry.List(),
		}, nil

	default:
		return nil, ManageMCPsOutput{}, fmt.Errorf("invalid action: %s (must be register, unregister, or list)", input.Action)
	}
}

// mapToSlice converts a map to a slice of "key=value" strings.
func mapToSlice(m map[string]string) []string {
	if m == nil {
		return nil
	}
	result := make([]string, 0, len(m))
	for k, v := range m {
		result = append(result, k+"="+v)
	}
	return result
}

// valueToAny converts any value to a JSON-serializable type.
func valueToAny(v any) any {
	if v == nil {
		return nil
	}

	// If it's already a basic type, return as-is
	switch val := v.(type) {
	case bool, int, int64, float64, string:
		return val
	case []any:
		return val
	case map[string]any:
		return val
	}

	// Try to convert via JSON for complex types
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}

	var result any
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Sprintf("%v", v)
	}

	return result
}
