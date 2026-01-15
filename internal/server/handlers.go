package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/auth"
	"github.com/standardbeagle/slop-mcp/internal/builtins"
	"github.com/standardbeagle/slop-mcp/internal/cli"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/registry"
	"github.com/standardbeagle/slop/pkg/slop"
)

// DefaultSearchLimit is the default maximum number of tools to return from search_tools.
const DefaultSearchLimit = 20

// MaxSearchLimit is the maximum allowed limit for search_tools.
const MaxSearchLimit = 100

// SearchToolsInput is the input for the search_tools tool.
type SearchToolsInput struct {
	Query   string `json:"query,omitempty" jsonschema:"Search query for tool names and descriptions"`
	MCPName string `json:"mcp_name,omitempty" jsonschema:"Filter to a specific MCP server"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Maximum number of results to return (default: 20, max: 100)"`
	Offset  int    `json:"offset,omitempty" jsonschema:"Number of results to skip for pagination (default: 0)"`
}

// SearchToolsOutput is the output for the search_tools tool.
type SearchToolsOutput struct {
	Tools   []registry.ToolInfo `json:"tools"`
	Total   int                 `json:"total"`    // Total matching tools (before pagination)
	Limit   int                 `json:"limit"`    // Limit applied
	Offset  int                 `json:"offset"`   // Offset applied
	HasMore bool                `json:"has_more"` // True if more results available
}

func (s *Server) handleSearchTools(
	ctx context.Context,
	req *mcp.CallToolRequest,
	input SearchToolsInput,
) (*mcp.CallToolResult, SearchToolsOutput, error) {
	tools := s.registry.SearchTools(input.Query, input.MCPName)

	// Include CLI tools if not filtering by specific MCP (or filtering by "cli")
	if input.MCPName == "" || input.MCPName == "cli" {
		cliInfos := s.cliRegistry.GetToolInfos()
		for _, cliTool := range cliInfos {
			// Apply query filter if specified
			if input.Query != "" && !matchesQuery(cliTool.Name, cliTool.Description, input.Query) {
				continue
			}
			tools = append(tools, registry.ToolInfo{
				Name:        cliTool.Name,
				Description: cliTool.Description,
				MCPName:     cliTool.MCPName,
				InputSchema: cliTool.InputSchema,
			})
		}
	}

	// Calculate total before pagination
	total := len(tools)

	// Apply default and max limits
	limit := input.Limit
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	// Apply offset
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}

	// Apply pagination
	if offset >= len(tools) {
		tools = []registry.ToolInfo{}
	} else {
		tools = tools[offset:]
		if len(tools) > limit {
			tools = tools[:limit]
		}
	}

	return nil, SearchToolsOutput{
		Tools:   tools,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		HasMore: offset+len(tools) < total,
	}, nil
}

// matchesQuery checks if a tool name or description matches the search query.
func matchesQuery(name, description, query string) bool {
	query = strings.ToLower(query)
	return strings.Contains(strings.ToLower(name), query) ||
		strings.Contains(strings.ToLower(description), query)
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

	// Handle CLI tools (mcp_name is "cli" or tool_name has cli_ prefix)
	if input.MCPName == "cli" || cli.IsCLITool(input.ToolName) {
		toolName := input.ToolName
		if cli.IsCLITool(toolName) {
			toolName = cli.StripCLIPrefix(toolName)
		}

		result, err := s.cliRegistry.Execute(ctx, toolName, input.Parameters)
		if err != nil {
			return nil, nil, err
		}

		// Return structured result
		return nil, result, nil
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

	// Register crypto built-ins
	builtins.RegisterCrypto(rt)

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

	// Register CLI tools as a service (accessible as cli.tool_name() in scripts)
	if s.cliRegistry.Count() > 0 {
		cliService := cli.NewSlopService(ctx, s.cliRegistry)
		rt.RegisterExternalService("cli", cliService)
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
	Action  string            `json:"action" jsonschema:"Action to perform: register, unregister, reconnect, list, status, or health_check"`
	Name    string            `json:"name,omitempty" jsonschema:"MCP server name (required for register/unregister/reconnect, optional for health_check)"`
	Type    string            `json:"type,omitempty" jsonschema:"Transport type: command (default), sse, or streamable"`
	Command string            `json:"command,omitempty" jsonschema:"Command executable for command transport"`
	Args    []string          `json:"args,omitempty" jsonschema:"Command arguments"`
	Env     map[string]string `json:"env,omitempty" jsonschema:"Environment variables"`
	URL     string            `json:"url,omitempty" jsonschema:"Server URL for HTTP transports"`
	Headers map[string]string `json:"headers,omitempty" jsonschema:"HTTP headers for HTTP transports"`
	Scope   string            `json:"scope,omitempty" jsonschema:"Where to save: memory (default, runtime only), user (~/.config/slop-mcp/config.kdl), or project (.slop-mcp.kdl)"`
}

// ManageMCPsOutput is the output for the manage_mcps tool.
type ManageMCPsOutput struct {
	Message      string                        `json:"message,omitempty"`
	MCPs         []registry.MCPStatus          `json:"mcps,omitempty"`
	Status       []registry.MCPFullStatus      `json:"status,omitempty"`
	HealthChecks []registry.HealthCheckResult  `json:"health_checks,omitempty"`
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

		// Determine source based on scope
		scope := input.Scope
		if scope == "" {
			scope = "memory"
		}

		var source config.Source
		switch scope {
		case "memory":
			source = config.SourceRuntime
		case "user":
			source = config.SourceUser
		case "project":
			source = config.SourceProject
		default:
			return nil, ManageMCPsOutput{}, fmt.Errorf("invalid scope: %s (must be memory, user, or project)", scope)
		}

		cfg := config.MCPConfig{
			Name:    input.Name,
			Type:    transportType,
			Command: input.Command,
			Args:    input.Args,
			Env:     input.Env,
			URL:     input.URL,
			Headers: input.Headers,
			Source:  source,
		}

		// Persist to file if not memory scope
		if scope != "memory" {
			var configPath string
			if scope == "user" {
				configPath = config.UserConfigPath()
			} else {
				// Use current working directory for project scope
				cwd, err := os.Getwd()
				if err != nil {
					return nil, ManageMCPsOutput{}, fmt.Errorf("failed to get working directory: %w", err)
				}
				configPath = config.ProjectConfigPath(cwd)
			}

			if err := config.AddMCPConfigToFile(configPath, cfg); err != nil {
				return nil, ManageMCPsOutput{}, fmt.Errorf("failed to save MCP to %s: %w", configPath, err)
			}
		}

		// Also connect at runtime
		if err := s.registry.Connect(ctx, cfg); err != nil {
			return nil, ManageMCPsOutput{}, fmt.Errorf("failed to register MCP: %w", err)
		}

		msg := fmt.Sprintf("Successfully registered MCP: %s", input.Name)
		if scope != "memory" {
			msg += fmt.Sprintf(" (saved to %s scope)", scope)
		}

		return nil, ManageMCPsOutput{
			Message: msg,
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

	case "reconnect":
		if input.Name == "" {
			return nil, ManageMCPsOutput{}, fmt.Errorf("name is required for reconnect action")
		}

		if err := s.registry.Reconnect(ctx, input.Name); err != nil {
			return nil, ManageMCPsOutput{}, fmt.Errorf("failed to reconnect MCP: %w", err)
		}

		return nil, ManageMCPsOutput{
			Message: fmt.Sprintf("Successfully reconnected MCP: %s", input.Name),
		}, nil

	case "list":
		return nil, ManageMCPsOutput{
			MCPs: s.registry.List(),
		}, nil

	case "status":
		return nil, ManageMCPsOutput{
			Status: s.registry.Status(),
		}, nil

	case "health_check":
		// Perform health check on specific MCP or all connected MCPs
		results := s.registry.HealthCheck(ctx, input.Name)
		msg := ""
		if input.Name != "" {
			if len(results) > 0 {
				msg = fmt.Sprintf("Health check for %s: %s", input.Name, results[0].Status)
			} else {
				msg = fmt.Sprintf("MCP %s is not connected", input.Name)
			}
		} else {
			healthy := 0
			for _, r := range results {
				if r.Status == registry.HealthStatusHealthy {
					healthy++
				}
			}
			msg = fmt.Sprintf("Health check complete: %d/%d MCPs healthy", healthy, len(results))
		}
		return nil, ManageMCPsOutput{
			Message:      msg,
			HealthChecks: results,
		}, nil

	default:
		return nil, ManageMCPsOutput{}, fmt.Errorf("invalid action: %s (must be register, unregister, reconnect, list, status, or health_check)", input.Action)
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
	ToolName string `json:"tool_name,omitempty" jsonschema:"Filter to a specific tool by name (optional)"`
	FilePath string `json:"file_path,omitempty" jsonschema:"Path to write metadata to (optional)"`
	Verbose  bool   `json:"verbose,omitempty" jsonschema:"Include full input schemas (default: only when querying specific tool)"`
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

	// Filter by tool name if specified
	if input.ToolName != "" {
		filteredMetadata := make([]registry.MCPMetadata, 0)
		for _, m := range metadata {
			filteredTools := make([]registry.ToolInfo, 0)
			for _, tool := range m.Tools {
				if tool.Name == input.ToolName {
					filteredTools = append(filteredTools, tool)
				}
			}
			// Only include MCP if it has matching tools
			if len(filteredTools) > 0 {
				m.Tools = filteredTools
				// Clear other metadata types when filtering by tool
				m.Prompts = nil
				m.Resources = nil
				m.ResourceTemplates = nil
				filteredMetadata = append(filteredMetadata, m)
			}
		}
		metadata = filteredMetadata
	}

	// Determine if we should include full schemas:
	// - Always include if verbose=true
	// - Include if querying a specific tool (mcp_name + tool_name both specified)
	// - Otherwise strip input_schema to reduce output size
	includeSchemas := input.Verbose || (input.MCPName != "" && input.ToolName != "")

	if !includeSchemas {
		// Strip input schemas from tools to reduce output size
		for i := range metadata {
			strippedTools := make([]registry.ToolInfo, len(metadata[i].Tools))
			for j, tool := range metadata[i].Tools {
				strippedTools[j] = registry.ToolInfo{
					Name:        tool.Name,
					Description: tool.Description,
					MCPName:     tool.MCPName,
					// InputSchema intentionally omitted
				}
			}
			metadata[i].Tools = strippedTools
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
