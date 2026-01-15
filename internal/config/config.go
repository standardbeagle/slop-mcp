package config

import "encoding/json"

// Config represents the merged configuration from user and project sources.
type Config struct {
	MCPs map[string]MCPConfig
}

// MCPConfig represents a single MCP server configuration.
type MCPConfig struct {
	Name                string            `json:"name,omitempty"`
	Type                string            `json:"type"` // "stdio", "sse", "http", "streamable"
	Command             string            `json:"command,omitempty"`
	Args                []string          `json:"args,omitempty"`
	Env                 map[string]string `json:"env,omitempty"`
	URL                 string            `json:"url,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	Timeout             string            `json:"timeout,omitempty"`              // Connection timeout (e.g., "30s", "1m")
	MaxRetries          int               `json:"max_retries,omitempty"`          // Max auto-reconnect retries (default 5, 0 = disabled)
	HealthCheckInterval string            `json:"health_check_interval,omitempty"` // Background health check interval (e.g., "30s", "1m"); 0 = disabled
	Source              Source            `json:"-"`
}

// Scope indicates where the config should be stored.
type Scope int

const (
	ScopeLocal   Scope = iota // ~/.claude.json style, project-specific but not shared
	ScopeProject              // .slop-mcp.kdl in project root, shared via git
	ScopeUser                 // ~/.config/slop-mcp/config.kdl, personal cross-project
)

func (s Scope) String() string {
	switch s {
	case ScopeLocal:
		return "local"
	case ScopeProject:
		return "project"
	case ScopeUser:
		return "user"
	default:
		return "unknown"
	}
}

// ParseScope parses a scope string.
func ParseScope(s string) Scope {
	switch s {
	case "local":
		return ScopeLocal
	case "project":
		return ScopeProject
	case "user":
		return ScopeUser
	default:
		return ScopeProject // default
	}
}

// Source indicates where the config came from.
type Source int

const (
	SourceUser Source = iota
	SourceProject
	SourceLocal
	SourceRuntime // Dynamically registered at runtime
)

func (s Source) String() string {
	switch s {
	case SourceUser:
		return "user"
	case SourceProject:
		return "project"
	case SourceLocal:
		return "local"
	case SourceRuntime:
		return "runtime"
	default:
		return "unknown"
	}
}

// NewConfig creates an empty config.
func NewConfig() *Config {
	return &Config{
		MCPs: make(map[string]MCPConfig),
	}
}

// JSONConfig represents the JSON format for MCP configs (Claude Desktop compatible).
type JSONConfig struct {
	MCPServers map[string]JSONMCPConfig `json:"mcpServers"`
}

// JSONMCPConfig represents a single MCP in JSON format.
type JSONMCPConfig struct {
	Type       string            `json:"type"`
	Command    string            `json:"command,omitempty"`
	Args       []string          `json:"args,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	URL        string            `json:"url,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Timeout    string            `json:"timeout,omitempty"`
	MaxRetries int               `json:"max_retries,omitempty"`
}

// ParseJSONConfig parses a JSON MCP config string.
func ParseJSONConfig(data string) (*MCPConfig, error) {
	var cfg JSONMCPConfig
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		return nil, err
	}

	return &MCPConfig{
		Type:       cfg.Type,
		Command:    cfg.Command,
		Args:       cfg.Args,
		Env:        cfg.Env,
		URL:        cfg.URL,
		Headers:    cfg.Headers,
		Timeout:    cfg.Timeout,
		MaxRetries: cfg.MaxRetries,
	}, nil
}

// ToJSON converts an MCPConfig to JSON string.
func (c *MCPConfig) ToJSON() string {
	data, _ := json.MarshalIndent(c, "", "  ")
	return string(data)
}
