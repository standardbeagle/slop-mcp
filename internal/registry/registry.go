package registry

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/anthropics/slop-mcp/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolInfo represents a tool with its source MCP.
type ToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	MCPName     string         `json:"mcp_name"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// MCPStatus represents the status of a registered MCP.
type MCPStatus struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Connected bool   `json:"connected"`
	ToolCount int    `json:"tool_count"`
	Source    string `json:"source"`
}

// mcpConnection holds an MCP client session.
type mcpConnection struct {
	session   *mcp.ClientSession
	transport mcp.Transport
	config    config.MCPConfig
}

// Registry manages multiple MCP connections.
type Registry struct {
	connections map[string]*mcpConnection
	toolIndex   *ToolIndex
	mu          sync.RWMutex
}

// New creates a new Registry.
func New() *Registry {
	return &Registry{
		connections: make(map[string]*mcpConnection),
		toolIndex:   NewToolIndex(),
	}
}

// Connect connects to an MCP server and registers it.
func (r *Registry) Connect(ctx context.Context, cfg config.MCPConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create MCP client
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "slop-mcp",
		Version: "0.1.0",
	}, nil)

	// Create transport based on type
	var transport mcp.Transport
	switch cfg.Type {
	case "command", "":
		cmd := exec.Command(cfg.Command, cfg.Args...)
		if len(cfg.Env) > 0 {
			for k, v := range cfg.Env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
		}
		transport = &mcp.CommandTransport{
			Command: cmd,
		}

	case "sse":
		transport = &mcp.SSEClientTransport{
			Endpoint: cfg.URL,
		}

	case "streamable":
		transport = &mcp.StreamableClientTransport{
			Endpoint: cfg.URL,
		}

	default:
		return fmt.Errorf("unknown MCP transport type: %s", cfg.Type)
	}

	// Connect
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to MCP %s: %w", cfg.Name, err)
	}

	r.connections[cfg.Name] = &mcpConnection{
		session:   session,
		transport: transport,
		config:    cfg,
	}

	// Index tools from this MCP
	if err := r.indexTools(ctx, cfg.Name, session); err != nil {
		fmt.Printf("Warning: failed to index tools for %s: %v\n", cfg.Name, err)
	}

	return nil
}

// Disconnect disconnects an MCP server.
func (r *Registry) Disconnect(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, ok := r.connections[name]
	if !ok {
		return fmt.Errorf("MCP not found: %s", name)
	}

	if err := conn.session.Close(); err != nil {
		return err
	}

	delete(r.connections, name)
	r.toolIndex.Remove(name)

	return nil
}

// List returns all registered MCPs.
func (r *Registry) List() []MCPStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]MCPStatus, 0, len(r.connections))
	for name, conn := range r.connections {
		status := MCPStatus{
			Name:      name,
			Type:      conn.config.Type,
			Connected: true,
			ToolCount: r.toolIndex.CountForMCP(name),
			Source:    conn.config.Source.String(),
		}
		result = append(result, status)
	}

	return result
}

// HasMCP checks if an MCP is registered.
func (r *Registry) HasMCP(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.connections[name]
	return ok
}

// SearchTools searches tools by query and/or MCP name.
func (r *Registry) SearchTools(query, mcpName string) []ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.toolIndex.Search(query, mcpName)
}

// ExecuteTool executes a tool on a specific MCP.
func (r *Registry) ExecuteTool(ctx context.Context, mcpName, toolName string, params map[string]any) (any, error) {
	r.mu.RLock()
	conn, ok := r.connections[mcpName]
	r.mu.RUnlock()

	if !ok {
		return nil, &MCPNotFoundError{
			Name:          mcpName,
			AvailableMCPs: r.listNames(),
		}
	}

	// Call tool
	result, err := conn.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: params,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "unknown") {
			return nil, &ToolNotFoundError{
				MCPName:        mcpName,
				ToolName:       toolName,
				AvailableTools: r.toolIndex.ListForMCP(mcpName),
			}
		}
		return nil, err
	}

	if result.IsError {
		var errMsg string
		for _, content := range result.Content {
			if text, ok := content.(*mcp.TextContent); ok {
				errMsg += text.Text
			}
		}
		if errMsg == "" {
			errMsg = "tool returned error"
		}
		return nil, fmt.Errorf("tool error: %s", errMsg)
	}

	// Convert result to Go types
	return contentToAny(result), nil
}

// Close closes all MCP connections.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var lastErr error
	for name, conn := range r.connections {
		if err := conn.session.Close(); err != nil {
			lastErr = err
			fmt.Printf("Warning: error closing %s: %v\n", name, err)
		}
	}
	r.connections = make(map[string]*mcpConnection)
	return lastErr
}

// GetConfigs returns all MCP configs for use with slop runtime.
func (r *Registry) GetConfigs() []config.MCPConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	configs := make([]config.MCPConfig, 0, len(r.connections))
	for _, conn := range r.connections {
		configs = append(configs, conn.config)
	}
	return configs
}

func (r *Registry) listNames() []string {
	names := make([]string, 0, len(r.connections))
	for name := range r.connections {
		names = append(names, name)
	}
	return names
}

func (r *Registry) indexTools(ctx context.Context, mcpName string, session *mcp.ClientSession) error {
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return err
	}

	tools := make([]ToolInfo, 0, len(result.Tools))
	for _, tool := range result.Tools {
		tools = append(tools, ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
			MCPName:     mcpName,
		})
	}
	r.toolIndex.Add(mcpName, tools)

	return nil
}

func contentToAny(result *mcp.CallToolResult) any {
	if result.StructuredContent != nil {
		return result.StructuredContent
	}

	if len(result.Content) == 0 {
		return nil
	}

	if len(result.Content) == 1 {
		return contentItemToAny(result.Content[0])
	}

	items := make([]any, 0, len(result.Content))
	for _, c := range result.Content {
		items = append(items, contentItemToAny(c))
	}
	return items
}

func contentItemToAny(content mcp.Content) any {
	switch c := content.(type) {
	case *mcp.TextContent:
		return c.Text
	case *mcp.ImageContent:
		return map[string]any{
			"type":     "image",
			"mimeType": c.MIMEType,
			"data":     c.Data,
		}
	case *mcp.AudioContent:
		return map[string]any{
			"type":     "audio",
			"mimeType": c.MIMEType,
			"data":     c.Data,
		}
	default:
		return fmt.Sprintf("%v", content)
	}
}

// MCPNotFoundError is returned when an MCP is not found.
type MCPNotFoundError struct {
	Name          string
	AvailableMCPs []string
}

func (e *MCPNotFoundError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("MCP server not found: %s\n\n", e.Name))

	if len(e.AvailableMCPs) > 0 {
		sb.WriteString("Available MCP servers:\n")
		for _, name := range e.AvailableMCPs {
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Fix:\n")
	sb.WriteString("1. Check the MCP name spelling\n")
	sb.WriteString("2. Register the MCP with: register_mcp tool or slop-mcp mcp add\n")
	sb.WriteString("3. Add to config: .slop-mcp.kdl or ~/.config/slop-mcp/config.kdl\n")

	return sb.String()
}

// ToolNotFoundError is returned when a tool is not found on an MCP.
type ToolNotFoundError struct {
	MCPName        string
	ToolName       string
	AvailableTools []string
}

func (e *ToolNotFoundError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tool '%s' not found on MCP '%s'\n\n", e.ToolName, e.MCPName))

	if len(e.AvailableTools) > 0 {
		sb.WriteString("Available tools:\n")
		for _, name := range e.AvailableTools {
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
		}
	}

	sb.WriteString("\nUse search_tools to find available tools")
	return sb.String()
}

// ConnectFromConfig connects all MCPs from a config.
func (r *Registry) ConnectFromConfig(ctx context.Context, cfg *config.Config) error {
	for _, mcpCfg := range cfg.MCPs {
		if err := r.Connect(ctx, mcpCfg); err != nil {
			fmt.Printf("Warning: failed to connect to MCP %s: %v\n", mcpCfg.Name, err)
		}
	}
	return nil
}
