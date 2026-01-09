package registry

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/auth"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ConnectionTimeout is the default timeout for connecting to an MCP server.
const ConnectionTimeout = 30 * time.Second

// ToolInfo represents a tool with its source MCP.
type ToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	MCPName     string         `json:"mcp_name"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// PromptInfo represents a prompt from an MCP.
type PromptInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Arguments   []ArgumentInfo `json:"arguments,omitempty"`
}

// ArgumentInfo represents a prompt argument.
type ArgumentInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ResourceInfo represents a resource from an MCP.
type ResourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
}

// ResourceTemplateInfo represents a resource template from an MCP.
type ResourceTemplateInfo struct {
	URITemplate string `json:"uri_template"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
}

// MCPMetadata contains all metadata for an MCP server.
type MCPMetadata struct {
	Name              string                 `json:"name"`
	Type              string                 `json:"type"`
	State             MCPState               `json:"state"`
	Source            string                 `json:"source"`
	Tools             []ToolInfo             `json:"tools,omitempty"`
	Prompts           []PromptInfo           `json:"prompts,omitempty"`
	Resources         []ResourceInfo         `json:"resources,omitempty"`
	ResourceTemplates []ResourceTemplateInfo `json:"resource_templates,omitempty"`
	Error             string                 `json:"error,omitempty"`
}

// MCPStatus represents the status of a registered MCP.
type MCPStatus struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Connected bool   `json:"connected"`
	ToolCount int    `json:"tool_count"`
	Source    string `json:"source"`
	Error     string `json:"error,omitempty"`
}

// MCPState represents the connection state of an MCP.
type MCPState string

const (
	StateConfigured   MCPState = "configured"
	StateConnecting   MCPState = "connecting"
	StateConnected    MCPState = "connected"
	StateDisconnected MCPState = "disconnected"
	StateError        MCPState = "error"
	StateNeedsAuth    MCPState = "needs_auth"
)

// MCPFullStatus includes state and error information for all MCPs.
type MCPFullStatus struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	State     MCPState `json:"state"`
	ToolCount int      `json:"tool_count,omitempty"`
	Source    string   `json:"source"`
	Error     string   `json:"error,omitempty"`
}

// mcpState tracks the state of a configured MCP.
type mcpState struct {
	config config.MCPConfig
	state  MCPState
	err    error
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
	states      map[string]*mcpState // tracks all configured MCPs and their states
	toolIndex   *ToolIndex
	mu          sync.RWMutex
}

// New creates a new Registry.
func New() *Registry {
	return &Registry{
		connections: make(map[string]*mcpConnection),
		states:      make(map[string]*mcpState),
		toolIndex:   NewToolIndex(),
	}
}

// SetConfigured registers an MCP as configured but not yet connected.
func (r *Registry) SetConfigured(cfg config.MCPConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Only set if not already tracked (don't overwrite existing state)
	if _, exists := r.states[cfg.Name]; !exists {
		r.states[cfg.Name] = &mcpState{
			config: cfg,
			state:  StateConfigured,
		}
	}
}

// Status returns the full status of all configured MCPs.
func (r *Registry) Status() []MCPFullStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]MCPFullStatus, 0, len(r.states))
	for name, state := range r.states {
		status := MCPFullStatus{
			Name:   name,
			Type:   state.config.Type,
			State:  state.state,
			Source: state.config.Source.String(),
		}

		// Add tool count if connected
		if state.state == StateConnected {
			status.ToolCount = r.toolIndex.CountForMCP(name)
		}

		// Add error message if in error state
		if state.state == StateError && state.err != nil {
			status.Error = state.err.Error()
		}

		result = append(result, status)
	}

	return result
}

// Connect connects to an MCP server and registers it.
func (r *Registry) Connect(ctx context.Context, cfg config.MCPConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Track state: set to connecting
	r.states[cfg.Name] = &mcpState{
		config: cfg,
		state:  StateConnecting,
	}

	// Helper to set error state
	setError := func(err error, state MCPState) error {
		r.states[cfg.Name] = &mcpState{
			config: cfg,
			state:  state,
			err:    err,
		}
		return err
	}

	// Create MCP client
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "slop-mcp",
		Version: "0.1.0",
	}, nil)

	// Create transport based on type
	var transport mcp.Transport
	switch cfg.Type {
	case "command", "stdio", "":
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
		sseTransport := &mcp.SSEClientTransport{
			Endpoint: cfg.URL,
		}
		// Check for stored auth token
		if token := r.getStoredToken(cfg.Name); token != nil && !token.IsExpired() {
			sseTransport.HTTPClient = &http.Client{
				Transport: &authTransport{
					base:  http.DefaultTransport,
					token: token.AccessToken,
				},
			}
		}
		transport = sseTransport

	case "http", "streamable":
		streamTransport := &mcp.StreamableClientTransport{
			Endpoint: cfg.URL,
		}
		// Check for stored auth token
		if token := r.getStoredToken(cfg.Name); token != nil && !token.IsExpired() {
			streamTransport.HTTPClient = &http.Client{
				Transport: &authTransport{
					base:  http.DefaultTransport,
					token: token.AccessToken,
				},
			}
		}
		transport = streamTransport

	default:
		err := fmt.Errorf("unknown MCP transport type: %s", cfg.Type)
		return setError(err, StateError)
	}

	// Use a timeout for connection to avoid hanging on slow/unresponsive MCPs
	connectCtx, cancel := context.WithTimeout(ctx, ConnectionTimeout)
	defer cancel()

	// Connect
	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		// Check if error indicates authentication required
		errStr := err.Error()
		if strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized") ||
			strings.Contains(errStr, "Unauthorized") || strings.Contains(errStr, "authentication") {
			return setError(fmt.Errorf("authentication required: %w", err), StateNeedsAuth)
		}
		return setError(fmt.Errorf("failed to connect to MCP %s: %w", cfg.Name, err), StateError)
	}

	r.connections[cfg.Name] = &mcpConnection{
		session:   session,
		transport: transport,
		config:    cfg,
	}

	// Update state to connected
	r.states[cfg.Name] = &mcpState{
		config: cfg,
		state:  StateConnected,
	}

	// Index tools from this MCP with a fresh timeout
	indexCtx, indexCancel := context.WithTimeout(ctx, ConnectionTimeout)
	defer indexCancel()
	if err := r.indexTools(indexCtx, cfg.Name, session); err != nil {
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

	// Close the session - ignore termination signals since they're expected
	// when killing a subprocess-based MCP
	if err := conn.session.Close(); err != nil {
		// Log but don't fail - the MCP is still being disconnected
		fmt.Printf("Warning: error closing %s: %v\n", name, err)
	}

	// Update state to disconnected (preserve config for potential reconnect)
	if state, exists := r.states[name]; exists {
		r.states[name] = &mcpState{
			config: state.config,
			state:  StateDisconnected,
		}
	}

	delete(r.connections, name)
	r.toolIndex.Remove(name)

	return nil
}

// Reconnect disconnects and reconnects an MCP server.
// This is useful after OAuth authentication to establish a connection with new credentials.
func (r *Registry) Reconnect(ctx context.Context, name string) error {
	// Get the config from state (under read lock first)
	r.mu.RLock()
	state, exists := r.states[name]
	if !exists {
		r.mu.RUnlock()
		return fmt.Errorf("MCP not configured: %s", name)
	}
	cfg := state.config
	r.mu.RUnlock()

	// Disconnect if currently connected (ignore errors - might not be connected)
	_ = r.Disconnect(name)

	// Reconnect with the stored config (will pick up new auth token)
	return r.Connect(ctx, cfg)
}

// List returns all configured MCPs (including disconnected ones).
func (r *Registry) List() []MCPStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]MCPStatus, 0, len(r.states))
	for name, state := range r.states {
		status := MCPStatus{
			Name:      name,
			Type:      state.config.Type,
			Connected: state.state == StateConnected,
			Source:    state.config.Source.String(),
		}

		// Add tool count if connected
		if status.Connected {
			status.ToolCount = r.toolIndex.CountForMCP(name)
		}

		// Add error message if in error state
		if state.err != nil {
			status.Error = state.err.Error()
		} else if state.state == StateNeedsAuth {
			status.Error = "authentication required"
		} else if state.state == StateDisconnected {
			status.Error = "disconnected"
		} else if state.state == StateConfigured {
			status.Error = "not connected (configured but never attempted)"
		} else if state.state == StateConnecting {
			status.Error = "connecting..."
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

// GetMetadata returns full metadata for all connected MCPs.
func (r *Registry) GetMetadata(ctx context.Context) []MCPMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]MCPMetadata, 0, len(r.states))

	for name, state := range r.states {
		metadata := MCPMetadata{
			Name:   name,
			Type:   state.config.Type,
			State:  state.state,
			Source: state.config.Source.String(),
		}

		// If connected, fetch full metadata
		if conn, ok := r.connections[name]; ok && state.state == StateConnected {
			// Fetch tools with full schema
			if toolsResult, err := conn.session.ListTools(ctx, nil); err == nil {
				metadata.Tools = make([]ToolInfo, 0, len(toolsResult.Tools))
				for _, tool := range toolsResult.Tools {
					var inputSchema map[string]any
					if schema, ok := tool.InputSchema.(map[string]any); ok {
						inputSchema = schema
					}
					metadata.Tools = append(metadata.Tools, ToolInfo{
						Name:        tool.Name,
						Description: tool.Description,
						MCPName:     name,
						InputSchema: inputSchema,
					})
				}
			}

			// Fetch prompts
			if promptsResult, err := conn.session.ListPrompts(ctx, nil); err == nil {
				metadata.Prompts = make([]PromptInfo, 0, len(promptsResult.Prompts))
				for _, prompt := range promptsResult.Prompts {
					promptInfo := PromptInfo{
						Name:        prompt.Name,
						Description: prompt.Description,
					}
					if prompt.Arguments != nil {
						promptInfo.Arguments = make([]ArgumentInfo, 0, len(prompt.Arguments))
						for _, arg := range prompt.Arguments {
							promptInfo.Arguments = append(promptInfo.Arguments, ArgumentInfo{
								Name:        arg.Name,
								Description: arg.Description,
								Required:    arg.Required,
							})
						}
					}
					metadata.Prompts = append(metadata.Prompts, promptInfo)
				}
			}

			// Fetch resources
			if resourcesResult, err := conn.session.ListResources(ctx, nil); err == nil {
				metadata.Resources = make([]ResourceInfo, 0, len(resourcesResult.Resources))
				for _, resource := range resourcesResult.Resources {
					metadata.Resources = append(metadata.Resources, ResourceInfo{
						URI:         resource.URI,
						Name:        resource.Name,
						Description: resource.Description,
						MIMEType:    resource.MIMEType,
					})
				}
			}

			// Fetch resource templates
			if templatesResult, err := conn.session.ListResourceTemplates(ctx, nil); err == nil {
				metadata.ResourceTemplates = make([]ResourceTemplateInfo, 0, len(templatesResult.ResourceTemplates))
				for _, template := range templatesResult.ResourceTemplates {
					metadata.ResourceTemplates = append(metadata.ResourceTemplates, ResourceTemplateInfo{
						URITemplate: template.URITemplate,
						Name:        template.Name,
						Description: template.Description,
						MIMEType:    template.MIMEType,
					})
				}
			}
		}

		// Add error message if in error state
		if state.state == StateError && state.err != nil {
			metadata.Error = state.err.Error()
		}

		result = append(result, metadata)
	}

	return result
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

// getStoredToken retrieves a stored OAuth token for an MCP server.
func (r *Registry) getStoredToken(serverName string) *auth.MCPToken {
	store := auth.NewTokenStore()
	token, err := store.GetToken(serverName)
	if err != nil {
		return nil
	}
	return token
}

// authTransport is an http.RoundTripper that adds Authorization headers.
type authTransport struct {
	base  http.RoundTripper
	token string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request to avoid modifying original
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(r)
}
