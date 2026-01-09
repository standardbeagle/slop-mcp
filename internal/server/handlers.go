package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/standardbeagle/slop-mcp/internal/auth"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/registry"
	"github.com/standardbeagle/slop/pkg/slop"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SearchToolsInput is the input for the search_tools tool.
type SearchToolsInput struct {
	Query   string `json:"query,omitempty" jsonschema:"Search query for tool names and descriptions"`
	MCPName string `json:"mcp_name,omitempty" jsonschema:"Filter to a specific MCP server"`
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
	MCPName    string         `json:"mcp_name" jsonschema:"Target MCP server name"`
	ToolName   string         `json:"tool_name" jsonschema:"Tool to execute on the MCP server"`
	Parameters map[string]any `json:"parameters,omitempty" jsonschema:"Tool parameters to pass through"`
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
	Script   string `json:"script,omitempty" jsonschema:"Inline SLOP script to execute"`
	FilePath string `json:"file_path,omitempty" jsonschema:"Path to a .slop file to execute"`
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
		// Normalize type for SLOP runtime (slop uses "command", not "stdio")
		transportType := cfg.Type
		if transportType == "stdio" {
			transportType = "command"
		}
		slopCfg := slop.MCPConfig{
			Name:    cfg.Name,
			Type:    transportType,
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
	Action  string            `json:"action" jsonschema:"Action to perform: register, unregister, list, or status"`
	Name    string            `json:"name,omitempty" jsonschema:"MCP server name (required for register/unregister)"`
	Type    string            `json:"type,omitempty" jsonschema:"Transport type: command (default), sse, or streamable"`
	Command string            `json:"command,omitempty" jsonschema:"Command executable for command transport"`
	Args    []string          `json:"args,omitempty" jsonschema:"Command arguments"`
	Env     map[string]string `json:"env,omitempty" jsonschema:"Environment variables"`
	URL     string            `json:"url,omitempty" jsonschema:"Server URL for HTTP transports"`
	Headers map[string]string `json:"headers,omitempty" jsonschema:"HTTP headers for HTTP transports"`
}

// ManageMCPsOutput is the output for the manage_mcps tool.
type ManageMCPsOutput struct {
	Message string                   `json:"message,omitempty"`
	MCPs    []registry.MCPStatus     `json:"mcps,omitempty"`
	Status  []registry.MCPFullStatus `json:"status,omitempty"`
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

	case "status":
		return nil, ManageMCPsOutput{
			Status: s.registry.Status(),
		}, nil

	default:
		return nil, ManageMCPsOutput{}, fmt.Errorf("invalid action: %s (must be register, unregister, list, or status)", input.Action)
	}
}

// AuthMCPInput is the input for the auth_mcp tool.
type AuthMCPInput struct {
	Action string `json:"action" jsonschema:"Action to perform: login, logout, status, or list"`
	Name   string `json:"name,omitempty" jsonschema:"MCP server name (required for login/logout/status)"`
}

// AuthMCPOutput is the output for the auth_mcp tool.
type AuthMCPOutput struct {
	Message string           `json:"message,omitempty"`
	Status  *AuthStatusInfo  `json:"status,omitempty"`
	Tokens  []AuthStatusInfo `json:"tokens,omitempty"`
}

// AuthStatusInfo contains authentication status information.
type AuthStatusInfo struct {
	ServerName  string `json:"server_name"`
	ServerURL   string `json:"server_url,omitempty"`
	IsAuth      bool   `json:"is_authenticated"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	IsExpired   bool   `json:"is_expired,omitempty"`
	HasRefresh  bool   `json:"has_refresh_token,omitempty"`
}

func (s *Server) handleAuthMCP(
	ctx context.Context,
	req *mcp.CallToolRequest,
	input AuthMCPInput,
) (*mcp.CallToolResult, AuthMCPOutput, error) {
	store := auth.NewTokenStore()

	switch input.Action {
	case "login":
		if input.Name == "" {
			return nil, AuthMCPOutput{}, fmt.Errorf("name is required for login action")
		}

		// Get MCP config to find the URL
		var serverURL string
		var mcpCfg *config.MCPConfig
		configs := s.registry.GetConfigs()
		for _, cfg := range configs {
			if cfg.Name == input.Name {
				serverURL = cfg.URL
				cfgCopy := cfg
				mcpCfg = &cfgCopy
				break
			}
		}

		if serverURL == "" {
			// Check if it's configured but not connected
			foundCfg, err := s.findMCPConfig(input.Name)
			if err != nil {
				return nil, AuthMCPOutput{}, fmt.Errorf("MCP '%s' not found and no URL configured", input.Name)
			}
			serverURL = foundCfg.URL
			mcpCfg = foundCfg
		}

		if serverURL == "" {
			return nil, AuthMCPOutput{}, fmt.Errorf("MCP '%s' does not have a URL configured; OAuth requires HTTP transport", input.Name)
		}

		flow := &auth.OAuthFlow{
			ServerName: input.Name,
			ServerURL:  serverURL,
			Store:      store,
		}

		result, err := flow.DiscoverAndAuth(ctx)
		if err != nil {
			return nil, AuthMCPOutput{}, fmt.Errorf("OAuth flow failed: %w", err)
		}

		// Reconnect the MCP to use the new credentials
		reconnectMsg := ""
		if err := s.registry.Reconnect(ctx, input.Name); err != nil {
			// If reconnect fails because MCP isn't in registry, try connecting with the config
			if mcpCfg != nil {
				if connectErr := s.registry.Connect(ctx, *mcpCfg); connectErr != nil {
					reconnectMsg = fmt.Sprintf(" (connection failed: %v)", connectErr)
				} else {
					reconnectMsg = " - connection established with new credentials"
				}
			} else {
				reconnectMsg = fmt.Sprintf(" (reconnect failed: %v)", err)
			}
		} else {
			reconnectMsg = " - connection re-established with new credentials"
		}

		return nil, AuthMCPOutput{
			Message: fmt.Sprintf("Successfully authenticated with %s%s", input.Name, reconnectMsg),
			Status: &AuthStatusInfo{
				ServerName: result.Token.ServerName,
				ServerURL:  result.Token.ServerURL,
				IsAuth:     true,
				ExpiresAt:  result.Token.ExpiresAt.Format("2006-01-02T15:04:05Z"),
				HasRefresh: result.Token.RefreshToken != "",
			},
		}, nil

	case "logout":
		if input.Name == "" {
			return nil, AuthMCPOutput{}, fmt.Errorf("name is required for logout action")
		}

		if err := store.DeleteToken(input.Name); err != nil {
			return nil, AuthMCPOutput{}, fmt.Errorf("failed to remove token: %w", err)
		}

		return nil, AuthMCPOutput{
			Message: fmt.Sprintf("Logged out from %s", input.Name),
		}, nil

	case "status":
		if input.Name == "" {
			return nil, AuthMCPOutput{}, fmt.Errorf("name is required for status action")
		}

		token, err := store.GetToken(input.Name)
		if err != nil {
			return nil, AuthMCPOutput{}, fmt.Errorf("failed to get token: %w", err)
		}

		if token == nil {
			return nil, AuthMCPOutput{
				Status: &AuthStatusInfo{
					ServerName: input.Name,
					IsAuth:     false,
				},
			}, nil
		}

		return nil, AuthMCPOutput{
			Status: &AuthStatusInfo{
				ServerName: token.ServerName,
				ServerURL:  token.ServerURL,
				IsAuth:     true,
				ExpiresAt:  token.ExpiresAt.Format("2006-01-02T15:04:05Z"),
				IsExpired:  token.IsExpired(),
				HasRefresh: token.RefreshToken != "",
			},
		}, nil

	case "list":
		tokens, err := store.ListTokens()
		if err != nil {
			return nil, AuthMCPOutput{}, fmt.Errorf("failed to list tokens: %w", err)
		}

		statuses := make([]AuthStatusInfo, 0, len(tokens))
		for _, t := range tokens {
			statuses = append(statuses, AuthStatusInfo{
				ServerName: t.ServerName,
				ServerURL:  t.ServerURL,
				IsAuth:     true,
				ExpiresAt:  t.ExpiresAt.Format("2006-01-02T15:04:05Z"),
				IsExpired:  t.IsExpired(),
				HasRefresh: t.RefreshToken != "",
			})
		}

		return nil, AuthMCPOutput{
			Message: fmt.Sprintf("Found %d authenticated MCPs", len(statuses)),
			Tokens:  statuses,
		}, nil

	default:
		return nil, AuthMCPOutput{}, fmt.Errorf("invalid action: %s (must be login, logout, status, or list)", input.Action)
	}
}

// GetMetadataInput is the input for the get_metadata tool.
type GetMetadataInput struct {
	MCPName  string `json:"mcp_name,omitempty" jsonschema:"Filter to a specific MCP server (optional)"`
	FilePath string `json:"file_path,omitempty" jsonschema:"Path to write metadata to (optional)"`
}

// GetMetadataOutput is the output for the get_metadata tool.
type GetMetadataOutput struct {
	Metadata []registry.MCPMetadata `json:"metadata"`
	Total    int                    `json:"total"`
	FilePath string                 `json:"file_path,omitempty"`
}

func (s *Server) handleGetMetadata(
	ctx context.Context,
	req *mcp.CallToolRequest,
	input GetMetadataInput,
) (*mcp.CallToolResult, GetMetadataOutput, error) {
	allMetadata := s.registry.GetMetadata(ctx)

	// Filter by MCP name if specified
	metadata := allMetadata
	if input.MCPName != "" {
		metadata = make([]registry.MCPMetadata, 0)
		for _, m := range allMetadata {
			if m.Name == input.MCPName {
				metadata = append(metadata, m)
			}
		}
	}

	output := GetMetadataOutput{
		Metadata: metadata,
		Total:    len(metadata),
	}

	// Write to file if path specified
	if input.FilePath != "" {
		data, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return nil, output, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		if err := os.WriteFile(input.FilePath, data, 0644); err != nil {
			return nil, output, fmt.Errorf("failed to write metadata file: %w", err)
		}
		output.FilePath = input.FilePath
	}

	return nil, output, nil
}

// findMCPConfig looks up an MCP config by name from the loaded config.
func (s *Server) findMCPConfig(name string) (*config.MCPConfig, error) {
	if s.config == nil || s.config.MCPs == nil {
		return nil, fmt.Errorf("no config loaded")
	}
	if cfg, ok := s.config.MCPs[name]; ok {
		return &cfg, nil
	}
	return nil, fmt.Errorf("MCP '%s' not found in config", name)
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
