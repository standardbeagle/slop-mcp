package registry

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/auth"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/logging"
)

// DefaultConnectionTimeout is the default timeout for connecting to an MCP server.
const DefaultConnectionTimeout = 30 * time.Second

// TimeoutEnvVar is the environment variable name for global timeout configuration.
const TimeoutEnvVar = "SLOP_MCP_TIMEOUT"

// GetConnectionTimeout returns the connection timeout for an MCP.
// Priority: per-MCP config > SLOP_MCP_TIMEOUT env var > default (30s).
func GetConnectionTimeout(cfg config.MCPConfig) time.Duration {
	// 1. Per-MCP config takes highest priority
	if cfg.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Timeout); err == nil && d > 0 {
			return d
		}
	}

	// 2. Environment variable takes second priority
	if envTimeout := os.Getenv(TimeoutEnvVar); envTimeout != "" {
		if d, err := time.ParseDuration(envTimeout); err == nil && d > 0 {
			return d
		}
	}

	// 3. Default timeout
	return DefaultConnectionTimeout
}

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
	StateConfigured    MCPState = "configured"
	StateConnecting    MCPState = "connecting"
	StateConnected     MCPState = "connected"
	StateDisconnected  MCPState = "disconnected"
	StateReconnecting  MCPState = "reconnecting"
	StateError         MCPState = "error"
	StateNeedsAuth     MCPState = "needs_auth"
)

// Reconnection constants for exponential backoff.
const (
	DefaultMaxRetries    = 5              // Default max auto-reconnect retries (0 = disabled)
	InitialBackoff       = 1 * time.Second
	MaxBackoff           = 60 * time.Second
	BackoffMultiplier    = 2.0
)

// Health check constants.
const (
	DefaultHealthCheckTimeout  = 5 * time.Second  // Timeout for individual health check pings
	DefaultHealthCheckInterval = 0                 // Default is disabled (0 = no background health checks)
)

// HealthStatus represents the health check status of an MCP.
type HealthStatus string

const (
	HealthStatusUnknown   HealthStatus = "unknown"   // Never checked
	HealthStatusHealthy   HealthStatus = "healthy"   // Last ping succeeded
	HealthStatusUnhealthy HealthStatus = "unhealthy" // Last ping failed
)

// MCPFullStatus includes state and error information for all MCPs.
type MCPFullStatus struct {
	Name              string       `json:"name"`
	Type              string       `json:"type"`
	State             MCPState     `json:"state"`
	ToolCount         int          `json:"tool_count,omitempty"`
	Source            string       `json:"source"`
	Error             string       `json:"error,omitempty"`
	ReconnectAttempts int          `json:"reconnect_attempts,omitempty"`
	HealthStatus      HealthStatus `json:"health_status,omitempty"`
	LastHealthCheck   string       `json:"last_health_check,omitempty"`
	HealthError       string       `json:"health_error,omitempty"`
}

// mcpState tracks the state of a configured MCP.
type mcpState struct {
	config            config.MCPConfig
	state             MCPState
	err               error
	reconnectAttempts int          // Number of reconnection attempts since last successful connect
	lastReconnect     time.Time    // Time of last reconnection attempt
	healthStatus      HealthStatus // Last health check result
	lastHealthCheck   time.Time    // Time of last health check
	healthError       error        // Error from last health check if unhealthy
}

// mcpConnection holds an MCP client session.
type mcpConnection struct {
	session   *mcp.ClientSession
	transport mcp.Transport
	config    config.MCPConfig
}

// Registry manages multiple MCP connections.
type Registry struct {
	connections       map[string]*mcpConnection
	states            map[string]*mcpState // tracks all configured MCPs and their states
	toolIndex         *ToolIndex
	logger            logging.Logger
	mu                sync.RWMutex
	healthCheckCancel context.CancelFunc // cancels background health check goroutine
}

// New creates a new Registry.
func New() *Registry {
	return &Registry{
		connections: make(map[string]*mcpConnection),
		states:      make(map[string]*mcpState),
		toolIndex:   NewToolIndex(),
		logger:      logging.Default(),
	}
}

// NewWithLogger creates a new Registry with a custom logger.
func NewWithLogger(logger logging.Logger) *Registry {
	return &Registry{
		connections: make(map[string]*mcpConnection),
		states:      make(map[string]*mcpState),
		toolIndex:   NewToolIndex(),
		logger:      logger,
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
			Name:              name,
			Type:              state.config.Type,
			State:             state.state,
			Source:            state.config.Source.String(),
			ReconnectAttempts: state.reconnectAttempts,
		}

		// Add tool count if connected
		if state.state == StateConnected {
			status.ToolCount = r.toolIndex.CountForMCP(name)
		}

		// Add error message if in error state
		if state.state == StateError && state.err != nil {
			status.Error = state.err.Error()
		}

		// Add health check info
		if state.healthStatus != "" {
			status.HealthStatus = state.healthStatus
		} else {
			status.HealthStatus = HealthStatusUnknown
		}
		if !state.lastHealthCheck.IsZero() {
			status.LastHealthCheck = state.lastHealthCheck.Format(time.RFC3339)
		}
		if state.healthError != nil {
			status.HealthError = state.healthError.Error()
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
		Version: "0.5.0",
	}, nil)

	// Create transport based on type
	var transport mcp.Transport
	switch cfg.Type {
	case "command", "stdio", "":
		cmd := exec.Command(cfg.Command, cfg.Args...)
		if len(cfg.Env) > 0 {
			// Start with current environment, then add custom vars
			cmd.Env = os.Environ()
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
		// Apply custom headers and/or OAuth token (refreshes expired tokens automatically)
		if httpClient := r.buildHTTPClient(ctx, cfg); httpClient != nil {
			sseTransport.HTTPClient = httpClient
		}
		transport = sseTransport

	case "http", "streamable":
		streamTransport := &mcp.StreamableClientTransport{
			Endpoint: cfg.URL,
		}
		// Apply custom headers and/or OAuth token (refreshes expired tokens automatically)
		if httpClient := r.buildHTTPClient(ctx, cfg); httpClient != nil {
			streamTransport.HTTPClient = httpClient
		}
		transport = streamTransport

	default:
		err := fmt.Errorf("unknown MCP transport type: %s", cfg.Type)
		return setError(err, StateError)
	}

	// Use a timeout for connection to avoid hanging on slow/unresponsive MCPs
	timeout := GetConnectionTimeout(cfg)
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
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
	indexCtx, indexCancel := context.WithTimeout(ctx, timeout)
	defer indexCancel()
	if err := r.indexTools(indexCtx, cfg.Name, session); err != nil {
		r.logger.Warn("failed to index tools", "mcp_name", cfg.Name, "error", err)
	}

	toolCount := r.toolIndex.CountForMCP(cfg.Name)
	r.logger.Info("connected to MCP", "mcp_name", cfg.Name, "type", cfg.Type, "tool_count", toolCount)

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
		// Debug level: expected during shutdown, errors stored in state for status queries
		r.logger.Debug("error closing MCP session", "mcp_name", name, "error", err)
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

// ReconnectWithBackoff attempts to reconnect an MCP with exponential backoff.
// It will try up to maxRetries times (or cfg.MaxRetries if 0 is passed), with increasing delays.
// Returns nil on successful reconnection, or the last error if all retries failed.
// If maxRetries is 0, uses the config's MaxRetries value (defaults to DefaultMaxRetries=5).
// To disable retries entirely, set cfg.MaxRetries to -1.
func (r *Registry) ReconnectWithBackoff(ctx context.Context, name string, maxRetries int) error {
	// Get the config and current state
	r.mu.RLock()
	state, exists := r.states[name]
	if !exists {
		r.mu.RUnlock()
		return fmt.Errorf("MCP not configured: %s", name)
	}
	cfg := state.config
	currentAttempts := state.reconnectAttempts
	r.mu.RUnlock()

	// Determine max retries
	if maxRetries == 0 {
		if cfg.MaxRetries > 0 {
			maxRetries = cfg.MaxRetries
		} else if cfg.MaxRetries == 0 {
			maxRetries = DefaultMaxRetries
		} else {
			// MaxRetries < 0 means disabled
			return fmt.Errorf("auto-reconnect disabled for MCP %s", name)
		}
	}

	// Update state to reconnecting
	r.mu.Lock()
	if s, ok := r.states[name]; ok {
		s.state = StateReconnecting
	}
	r.mu.Unlock()

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Calculate backoff delay
		backoff := calculateBackoff(attempt)

		// Update state with attempt count
		r.mu.Lock()
		if s, ok := r.states[name]; ok {
			s.reconnectAttempts = currentAttempts + attempt
			s.lastReconnect = time.Now()
		}
		r.mu.Unlock()

		// Wait for backoff (respecting context cancellation)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		// Attempt reconnection
		err := r.Reconnect(ctx, name)
		if err == nil {
			// Success - reset reconnection attempts
			r.mu.Lock()
			if s, ok := r.states[name]; ok {
				s.reconnectAttempts = 0
			}
			r.mu.Unlock()
			return nil
		}

		lastErr = err
		// Debug level: transient failures during reconnection, final state stored for queries
		r.logger.Debug("reconnect attempt failed",
			"mcp_name", name,
			"attempt", attempt,
			"max_retries", maxRetries,
			"error", err)
	}

	// All retries exhausted - update state to error
	r.mu.Lock()
	if s, ok := r.states[name]; ok {
		s.state = StateError
		s.err = fmt.Errorf("reconnection failed after %d attempts: %w", maxRetries, lastErr)
	}
	r.mu.Unlock()

	return fmt.Errorf("reconnection failed after %d attempts: %w", maxRetries, lastErr)
}

// calculateBackoff returns the backoff duration for a given retry attempt.
// Uses exponential backoff: 1s, 2s, 4s, 8s, ..., up to MaxBackoff (60s).
func calculateBackoff(attempt int) time.Duration {
	backoff := InitialBackoff
	for i := 1; i < attempt; i++ {
		backoff = time.Duration(float64(backoff) * BackoffMultiplier)
		if backoff > MaxBackoff {
			return MaxBackoff
		}
	}
	return backoff
}

// GetReconnectAttempts returns the number of reconnection attempts for an MCP.
func (r *Registry) GetReconnectAttempts(name string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if state, ok := r.states[name]; ok {
		return state.reconnectAttempts
	}
	return 0
}

// HealthCheckResult contains the result of a health check for a single MCP.
type HealthCheckResult struct {
	Name         string       `json:"name"`
	Status       HealthStatus `json:"status"`
	ResponseTime string       `json:"response_time,omitempty"`
	Error        string       `json:"error,omitempty"`
	CheckedAt    time.Time    `json:"checked_at"`
}

// HealthCheck performs a health check on the specified MCP or all connected MCPs.
// If name is empty, all connected MCPs are checked concurrently.
// Health checks use the MCP ping method with a 5-second timeout.
// Results are stored in the MCP state and returned.
func (r *Registry) HealthCheck(ctx context.Context, name string) []HealthCheckResult {
	r.mu.RLock()
	var toCheck []string
	if name != "" {
		// Check specific MCP
		if _, ok := r.connections[name]; ok {
			toCheck = []string{name}
		}
	} else {
		// Check all connected MCPs
		for n := range r.connections {
			toCheck = append(toCheck, n)
		}
	}
	r.mu.RUnlock()

	if len(toCheck) == 0 {
		return nil
	}

	// Run health checks concurrently
	results := make([]HealthCheckResult, len(toCheck))
	var wg sync.WaitGroup
	wg.Add(len(toCheck))

	for i, mcpName := range toCheck {
		go func(idx int, n string) {
			defer wg.Done()
			results[idx] = r.healthCheckOne(ctx, n)
		}(i, mcpName)
	}

	wg.Wait()
	return results
}

// healthCheckOne performs a health check on a single MCP.
func (r *Registry) healthCheckOne(ctx context.Context, name string) HealthCheckResult {
	checkTime := time.Now()
	result := HealthCheckResult{
		Name:      name,
		CheckedAt: checkTime,
	}

	// Get the connection
	r.mu.RLock()
	conn, ok := r.connections[name]
	r.mu.RUnlock()

	if !ok {
		result.Status = HealthStatusUnhealthy
		result.Error = "MCP not connected"
		r.updateHealthState(name, HealthStatusUnhealthy, checkTime, fmt.Errorf("MCP not connected"))
		return result
	}

	// Create a context with health check timeout
	pingCtx, cancel := context.WithTimeout(ctx, DefaultHealthCheckTimeout)
	defer cancel()

	// Perform ping
	start := time.Now()
	err := conn.session.Ping(pingCtx, &mcp.PingParams{})
	elapsed := time.Since(start)

	result.ResponseTime = elapsed.String()

	if err != nil {
		result.Status = HealthStatusUnhealthy
		result.Error = err.Error()
		r.updateHealthState(name, HealthStatusUnhealthy, checkTime, err)
		// Debug level: health status stored in state for status queries
		r.logger.Debug("health check failed", "mcp_name", name, "error", err, "response_time", elapsed)
	} else {
		result.Status = HealthStatusHealthy
		r.updateHealthState(name, HealthStatusHealthy, checkTime, nil)
		r.logger.Debug("health check passed", "mcp_name", name, "response_time", elapsed)
	}

	return result
}

// updateHealthState updates the health check state for an MCP.
func (r *Registry) updateHealthState(name string, status HealthStatus, checkedAt time.Time, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if state, ok := r.states[name]; ok {
		state.healthStatus = status
		state.lastHealthCheck = checkedAt
		state.healthError = err
	}
}

// GetHealthStatus returns the last known health status for an MCP.
func (r *Registry) GetHealthStatus(name string) (HealthStatus, time.Time, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if state, ok := r.states[name]; ok {
		return state.healthStatus, state.lastHealthCheck, state.healthError
	}
	return HealthStatusUnknown, time.Time{}, nil
}

// StartBackgroundHealthCheck starts a background goroutine that periodically runs health checks.
// The interval is parsed from a duration string (e.g., "30s", "1m").
// Pass an empty string or "0" to disable background health checks.
// Calling this method again will stop any existing background health check and start a new one.
func (r *Registry) StartBackgroundHealthCheck(intervalStr string) error {
	// Stop any existing background health check
	r.StopBackgroundHealthCheck()

	if intervalStr == "" || intervalStr == "0" {
		return nil // Disabled
	}

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		return fmt.Errorf("invalid health check interval: %w", err)
	}

	if interval <= 0 {
		return nil // Disabled
	}

	// Create a cancellable context for the background goroutine
	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.healthCheckCancel = cancel
	r.mu.Unlock()

	go r.backgroundHealthCheckLoop(ctx, interval)
	r.logger.Info("started background health check", "interval", interval)
	return nil
}

// StopBackgroundHealthCheck stops any running background health check goroutine.
func (r *Registry) StopBackgroundHealthCheck() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.healthCheckCancel != nil {
		r.healthCheckCancel()
		r.healthCheckCancel = nil
	}
}

// backgroundHealthCheckLoop runs health checks on all connected MCPs at the specified interval.
func (r *Registry) backgroundHealthCheckLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Debug("background health check stopped")
			return
		case <-ticker.C:
			r.HealthCheck(ctx, "") // Check all MCPs
		}
	}
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
		} else if state.state == StateReconnecting {
			status.Error = fmt.Sprintf("reconnecting (attempt %d)", state.reconnectAttempts)
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

	// Normalize nil params to empty map (MCP protocol requires object, not null)
	if params == nil {
		params = make(map[string]any)
	}

	// Call tool
	result, err := conn.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: params,
	})
	if err != nil {
		// Check for tool not found errors
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "unknown") {
			availableTools := r.toolIndex.ListForMCP(mcpName)
			return nil, &ToolNotFoundError{
				MCPName:        mcpName,
				ToolName:       toolName,
				AvailableTools: availableTools,
				SimilarTools:   findSimilarTools(toolName, availableTools),
			}
		}
		// Check for protocol/validation errors
		if protocolErr := parseProtocolError(mcpName, toolName, err); protocolErr != nil {
			return nil, protocolErr
		}
		return nil, fmt.Errorf("error calling tool '%s' on '%s': %w", toolName, mcpName, err)
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

		// Check if this looks like a parameter error and enhance the message
		if isParameterError(errMsg) {
			return nil, r.createParameterError(mcpName, toolName, errMsg, params)
		}

		return nil, fmt.Errorf("tool error: %s", errMsg)
	}

	// Convert result to Go types
	return contentToAny(result), nil
}

// ExecuteToolRaw calls a tool and returns the raw MCP response without conversion.
// Use this when you need to pass through the underlying MCP's response format directly.
func (r *Registry) ExecuteToolRaw(ctx context.Context, mcpName, toolName string, params map[string]any) (*mcp.CallToolResult, error) {
	r.mu.RLock()
	conn, ok := r.connections[mcpName]
	r.mu.RUnlock()

	if !ok {
		return nil, &MCPNotFoundError{
			Name:          mcpName,
			AvailableMCPs: r.listNames(),
		}
	}

	// Normalize nil params to empty map (MCP protocol requires object, not null)
	if params == nil {
		params = make(map[string]any)
	}

	// Call tool and return raw result
	result, err := conn.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: params,
	})
	if err != nil {
		// Check for tool not found errors
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "unknown") {
			availableTools := r.toolIndex.ListForMCP(mcpName)
			return nil, &ToolNotFoundError{
				MCPName:        mcpName,
				ToolName:       toolName,
				AvailableTools: availableTools,
				SimilarTools:   findSimilarTools(toolName, availableTools),
			}
		}
		// Check for protocol/validation errors
		if protocolErr := parseProtocolError(mcpName, toolName, err); protocolErr != nil {
			return nil, protocolErr
		}
		return nil, fmt.Errorf("error calling tool '%s' on '%s': %w", toolName, mcpName, err)
	}

	return result, nil
}

// isParameterError checks if an error message indicates invalid parameters.
func isParameterError(errMsg string) bool {
	errLower := strings.ToLower(errMsg)
	paramIndicators := []string{
		"parameter",
		"argument",
		"property",
		"required",
		"missing",
		"invalid",
		"unknown",
		"unexpected",
		"schema",
		"validation",
		"type",
	}

	for _, indicator := range paramIndicators {
		if strings.Contains(errLower, indicator) {
			return true
		}
	}
	return false
}

// createParameterError creates an enhanced error with parameter suggestions.
func (r *Registry) createParameterError(mcpName, toolName, originalError string, params map[string]any) error {
	// Get the tool info to access its schema
	toolInfo := r.toolIndex.GetTool(mcpName, toolName)

	// Collect provided parameter names
	providedParams := make([]string, 0, len(params))
	for k := range params {
		providedParams = append(providedParams, k)
	}

	// If we don't have schema info, return a basic error with provided params
	if toolInfo == nil || toolInfo.InputSchema == nil {
		return &InvalidParameterError{
			MCPName:        mcpName,
			ToolName:       toolName,
			OriginalError:  originalError,
			ProvidedParams: providedParams,
		}
	}

	// Extract expected parameters from schema
	expectedParams := extractParamsFromSchema(toolInfo.InputSchema)

	// Build a set of expected param names (normalized for matching)
	expectedNormSet := make(map[string]bool)
	expectedNameSet := make(map[string]bool)
	for _, e := range expectedParams {
		expectedNormSet[normalizeParam(e.Name)] = true
		expectedNameSet[e.Name] = true
	}

	// Find unknown parameters (provided but not in schema)
	var unknownParams []string
	for _, p := range providedParams {
		pNorm := normalizeParam(p)
		if !expectedNormSet[pNorm] {
			unknownParams = append(unknownParams, p)
		}
	}

	// Find missing required parameters
	providedNormSet := make(map[string]bool)
	for _, p := range providedParams {
		providedNormSet[normalizeParam(p)] = true
	}
	var missingRequired []string
	for _, e := range expectedParams {
		if e.Required && !providedNormSet[normalizeParam(e.Name)] {
			missingRequired = append(missingRequired, e.Name)
		}
	}

	// Find similar parameters (suggestions for ALL unknown params)
	similarParams := findSimilarParams(unknownParams, expectedParams)

	return &InvalidParameterError{
		MCPName:         mcpName,
		ToolName:        toolName,
		OriginalError:   originalError,
		ProvidedParams:  providedParams,
		ExpectedParams:  expectedParams,
		SimilarParams:   similarParams,
		MissingRequired: missingRequired,
		UnknownParams:   unknownParams,
	}
}

// Close closes all MCP connections and stops background health checks.
func (r *Registry) Close() error {
	// Stop background health check first (uses its own lock)
	r.StopBackgroundHealthCheck()

	r.mu.Lock()
	defer r.mu.Unlock()

	var lastErr error
	for name, conn := range r.connections {
		if err := conn.session.Close(); err != nil {
			lastErr = err
			// Debug level: subprocess exit codes during shutdown are expected
			r.logger.Debug("error closing MCP", "mcp_name", name, "error", err)
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
		var inputSchema map[string]any
		if schema, ok := tool.InputSchema.(map[string]any); ok {
			inputSchema = schema
		}
		tools = append(tools, ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
			MCPName:     mcpName,
			InputSchema: inputSchema,
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
	SimilarTools   []string // tools with similar names
}

func (e *ToolNotFoundError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tool '%s' not found on MCP '%s'\n\n", e.ToolName, e.MCPName))

	// Show similar tools first (most helpful)
	if len(e.SimilarTools) > 0 {
		sb.WriteString("Did you mean:\n")
		for _, name := range e.SimilarTools {
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
		}
		sb.WriteString("\n")
	}

	// Show all available tools
	if len(e.AvailableTools) > 0 {
		sb.WriteString("Available tools:\n")
		for _, name := range e.AvailableTools {
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
		}
	}

	sb.WriteString("\nUse search_tools to find tools across all MCPs")
	return sb.String()
}

// findSimilarTools finds tools with names similar to the query.
func findSimilarTools(query string, available []string) []string {
	queryNorm := normalizeParam(query)
	type scored struct {
		name  string
		score int
	}
	var matches []scored

	for _, name := range available {
		nameNorm := normalizeParam(name)
		score := similarity(queryNorm, nameNorm)
		if score >= 40 { // 40% threshold for tool suggestions
			matches = append(matches, scored{name: name, score: score})
		}
	}

	// Sort by score descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	// Return top 5 matches
	result := make([]string, 0, 5)
	for i, m := range matches {
		if i >= 5 {
			break
		}
		result = append(result, m.name)
	}
	return result
}

// InvalidParameterError is returned when invalid parameters are passed to a tool.
type InvalidParameterError struct {
	MCPName           string
	ToolName          string
	OriginalError     string
	ProvidedParams    []string
	ExpectedParams    []ParamInfo
	SimilarParams     map[string]string // provided -> suggested
	MissingRequired   []string          // required params not provided
	UnknownParams     []string          // provided params not in schema
}

// ParamInfo describes a parameter in a tool's schema.
type ParamInfo struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

func (e *InvalidParameterError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Invalid parameters for tool '%s' on MCP '%s'\n", e.ToolName, e.MCPName))
	sb.WriteString(fmt.Sprintf("Error: %s\n\n", e.OriginalError))

	// Show missing required parameters first (most critical)
	if len(e.MissingRequired) > 0 {
		sb.WriteString("Missing required parameters:\n")
		for _, name := range e.MissingRequired {
			// Find the param info for description
			for _, param := range e.ExpectedParams {
				if param.Name == name {
					typeStr := ""
					if param.Type != "" {
						typeStr = fmt.Sprintf(" [%s]", param.Type)
					}
					descStr := ""
					if param.Description != "" {
						descStr = fmt.Sprintf(" - %s", param.Description)
					}
					sb.WriteString(fmt.Sprintf("  - %s%s%s\n", name, typeStr, descStr))
					break
				}
			}
		}
		sb.WriteString("\n")
	}

	// Show unknown parameters with suggestions
	if len(e.UnknownParams) > 0 {
		sb.WriteString("Unknown parameters:\n")
		for _, name := range e.UnknownParams {
			if suggestion, ok := e.SimilarParams[name]; ok {
				sb.WriteString(fmt.Sprintf("  - '%s' (did you mean '%s'?)\n", name, suggestion))
			} else {
				sb.WriteString(fmt.Sprintf("  - '%s'\n", name))
			}
		}
		sb.WriteString("\n")
	}

	// Show all expected parameters for reference
	if len(e.ExpectedParams) > 0 {
		sb.WriteString("Expected parameters:\n")
		for _, param := range e.ExpectedParams {
			reqStr := ""
			if param.Required {
				reqStr = " (required)"
			}
			typeStr := ""
			if param.Type != "" {
				typeStr = fmt.Sprintf(" [%s]", param.Type)
			}
			descStr := ""
			if param.Description != "" {
				descStr = fmt.Sprintf(" - %s", param.Description)
			}
			sb.WriteString(fmt.Sprintf("  - %s%s%s%s\n", param.Name, typeStr, reqStr, descStr))
		}
	}

	return sb.String()
}

// MCPProtocolError wraps MCP protocol-level errors with actionable messages.
type MCPProtocolError struct {
	MCPName       string
	ToolName      string
	OriginalError string
	ErrorCode     string // e.g., "invalid_type"
	Path          string // e.g., "params.arguments"
	Suggestion    string // actionable fix
}

func (e *MCPProtocolError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("MCP protocol error calling '%s' on '%s'\n\n", e.ToolName, e.MCPName))

	// Provide a human-readable interpretation
	if e.ErrorCode == "invalid_type" && strings.Contains(e.Path, "arguments") {
		sb.WriteString("Issue: Tool arguments have incorrect type\n")
	} else if e.OriginalError != "" {
		sb.WriteString(fmt.Sprintf("Details: %s\n", e.OriginalError))
	}

	if e.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("\nFix: %s\n", e.Suggestion))
	}

	return sb.String()
}

// parseProtocolError attempts to parse common MCP protocol error formats.
// Returns nil if the error doesn't match known patterns.
func parseProtocolError(mcpName, toolName string, err error) *MCPProtocolError {
	errStr := err.Error()

	// Detect Zod-style validation errors (common in TypeScript MCPs)
	// Format: [{"expected":"record","code":"invalid_type","path":["params","arguments"],"message":"..."}]
	if strings.Contains(errStr, "invalid_type") && strings.Contains(errStr, "expected") {
		protocolErr := &MCPProtocolError{
			MCPName:       mcpName,
			ToolName:      toolName,
			OriginalError: errStr,
			ErrorCode:     "invalid_type",
		}

		// Extract path
		if strings.Contains(errStr, "arguments") {
			protocolErr.Path = "params.arguments"
			protocolErr.Suggestion = "Pass an empty object {} instead of null for parameters"
		}

		// Extract expected type
		if strings.Contains(errStr, "expected") && strings.Contains(errStr, "record") {
			protocolErr.Suggestion = "The 'parameters' field must be an object {}, not null or undefined"
		}

		return protocolErr
	}

	// Detect JSON-RPC error format
	if strings.Contains(errStr, "Invalid params") || strings.Contains(errStr, "-32602") {
		return &MCPProtocolError{
			MCPName:       mcpName,
			ToolName:      toolName,
			OriginalError: errStr,
			ErrorCode:     "invalid_params",
			Suggestion:    "Check the tool's expected parameters using get_metadata",
		}
	}

	// Detect method not found
	if strings.Contains(errStr, "Method not found") || strings.Contains(errStr, "-32601") {
		return &MCPProtocolError{
			MCPName:       mcpName,
			ToolName:      toolName,
			OriginalError: errStr,
			ErrorCode:     "method_not_found",
			Suggestion:    "This tool may not exist on the MCP server. Use search_tools to find available tools.",
		}
	}

	return nil
}

// findSimilarParams finds expected parameters that are similar to provided ones.
// Uses fuzzy matching with normalized comparison (ignoring case, underscores, hyphens).
func findSimilarParams(provided []string, expected []ParamInfo) map[string]string {
	result := make(map[string]string)

	// Build a set of normalized expected param names for quick lookup
	expectedNormSet := make(map[string]bool)
	for _, e := range expected {
		expectedNormSet[normalizeParam(e.Name)] = true
	}

	for _, p := range provided {
		pNorm := normalizeParam(p)

		// Skip if this provided param matches any expected param exactly (normalized)
		if expectedNormSet[pNorm] {
			continue
		}

		bestMatch := ""
		bestScore := 0

		for _, e := range expected {
			eNorm := normalizeParam(e.Name)

			// Calculate similarity score
			score := similarity(pNorm, eNorm)
			if score > bestScore && score >= 50 { // 50% threshold for suggestions
				bestScore = score
				bestMatch = e.Name
			}
		}

		if bestMatch != "" {
			result[p] = bestMatch
		}
	}

	return result
}

// normalizeParam normalizes a parameter name for comparison.
func normalizeParam(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

// similarity returns a percentage similarity between two normalized strings.
// Uses longest common subsequence for comparison.
func similarity(a, b string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	// Check for substring match first
	if strings.Contains(a, b) || strings.Contains(b, a) {
		shorter := len(a)
		if len(b) < shorter {
			shorter = len(b)
		}
		longer := len(a)
		if len(b) > longer {
			longer = len(b)
		}
		return shorter * 100 / longer
	}

	// Use longest common subsequence
	lcs := longestCommonSubsequence(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	return lcs * 100 / maxLen
}

// longestCommonSubsequence returns the length of the LCS of two strings.
func longestCommonSubsequence(a, b string) int {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
			}
		}
	}

	return dp[m][n]
}

// extractParamsFromSchema extracts parameter info from a JSON schema.
func extractParamsFromSchema(schema map[string]any) []ParamInfo {
	if schema == nil {
		return nil
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}

	var required []string
	if reqList, ok := schema["required"].([]any); ok {
		for _, r := range reqList {
			if s, ok := r.(string); ok {
				required = append(required, s)
			}
		}
	}

	reqSet := make(map[string]bool)
	for _, r := range required {
		reqSet[r] = true
	}

	params := make([]ParamInfo, 0, len(props))
	for name, propAny := range props {
		prop, ok := propAny.(map[string]any)
		if !ok {
			continue
		}

		param := ParamInfo{
			Name:     name,
			Required: reqSet[name],
		}

		if t, ok := prop["type"].(string); ok {
			param.Type = t
		}
		if d, ok := prop["description"].(string); ok {
			param.Description = d
		}

		params = append(params, param)
	}

	return params
}

// ConnectFromConfig connects all MCPs from a config.
func (r *Registry) ConnectFromConfig(ctx context.Context, cfg *config.Config) error {
	for _, mcpCfg := range cfg.MCPs {
		if err := r.Connect(ctx, mcpCfg); err != nil {
			// Debug level: errors are stored in state for status queries
			r.logger.Debug("failed to connect to MCP", "mcp_name", mcpCfg.Name, "error", err)
		}
	}
	return nil
}

// AddToolsForTesting adds tools to the index for testing purposes.
// This bypasses the normal connection flow and directly populates the index.
func (r *Registry) AddToolsForTesting(mcpName string, tools []ToolInfo) {
	r.toolIndex.Add(mcpName, tools)
}

// getValidToken retrieves a stored token and refreshes it if expired.
// Returns the valid token or nil if unavailable/refresh failed.
func (r *Registry) getValidToken(ctx context.Context, serverName string) *auth.MCPToken {
	store := auth.NewTokenStore()
	token, err := store.GetToken(serverName)
	if err != nil || token == nil {
		return nil
	}

	// Token is valid, use it
	if !token.IsExpired() {
		return token
	}

	// Token expired - try to refresh
	if token.RefreshToken == "" || token.TokenEndpoint == "" {
		// Can't refresh without refresh token or endpoint
		return nil
	}

	// Attempt refresh with a timeout
	refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	newToken, err := auth.RefreshToken(refreshCtx, token, token.TokenEndpoint)
	if err != nil {
		// Refresh failed - token is unusable
		return nil
	}

	// Save the refreshed token
	if err := store.SetToken(newToken); err != nil {
		// Failed to save but token is still valid for this request
		return newToken
	}

	return newToken
}

// headersTransport is an http.RoundTripper that adds custom headers to requests.
type headersTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headersTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request to avoid modifying original
	r := req.Clone(req.Context())
	for k, v := range t.headers {
		r.Header.Set(k, v)
	}
	return t.base.RoundTrip(r)
}

// buildHTTPClient creates an HTTP client with custom headers and/or OAuth token.
// If the stored token is expired, it will attempt to refresh it using the refresh token.
func (r *Registry) buildHTTPClient(ctx context.Context, cfg config.MCPConfig) *http.Client {
	headers := make(map[string]string)

	// Add headers from config
	for k, v := range cfg.Headers {
		headers[k] = v
	}

	// Add OAuth token if available (overrides config Authorization header)
	// This will automatically refresh expired tokens if possible
	if token := r.getValidToken(ctx, cfg.Name); token != nil {
		headers["Authorization"] = "Bearer " + token.AccessToken
	}

	// If no custom headers, return nil to use default client
	if len(headers) == 0 {
		return nil
	}

	return &http.Client{
		Transport: &headersTransport{
			base:    http.DefaultTransport,
			headers: headers,
		},
	}
}
