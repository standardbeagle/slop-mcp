package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	kdl "github.com/sblinch/kdl-go"
)

const (
	ProjectConfigFile = ".slop-mcp.kdl"
	LocalConfigFile   = ".slop-mcp.local.kdl"
	UserConfigDir     = "slop-mcp"
	UserConfigFile    = "config.kdl"
)

// KDLConfig is the raw KDL structure for unmarshaling.
type KDLConfig struct {
	MCPs []KDLMCPConfig `kdl:"mcp,multiple"`
}

// KDLMCPConfig represents an MCP node in KDL.
type KDLMCPConfig struct {
	Name    string            `kdl:",arg"`
	Type    string            `kdl:"type"`
	Command string            `kdl:"command"`
	Args    []string          `kdl:"args"`
	Env     map[string]string `kdl:"env"`
	URL     string            `kdl:"url"`
	Headers map[string]string `kdl:"headers"`
	Timeout string            `kdl:"timeout"`
	Dynamic bool              `kdl:"dynamic"`
}

// UserConfigPath returns the path to the user config file.
func UserConfigPath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, UserConfigDir, UserConfigFile)
}

// LocalConfigPath returns the path to the local config file (project-specific, not shared).
func LocalConfigPath(dir string) string {
	return filepath.Join(dir, LocalConfigFile)
}

// ProjectConfigPath returns the path to the project config file.
func ProjectConfigPath(dir string) string {
	return filepath.Join(dir, ProjectConfigFile)
}

// ConfigPathForScope returns the config path for a given scope.
func ConfigPathForScope(scope Scope, projectDir string) string {
	switch scope {
	case ScopeLocal:
		return LocalConfigPath(projectDir)
	case ScopeProject:
		return ProjectConfigPath(projectDir)
	case ScopeUser:
		return UserConfigPath()
	default:
		return ProjectConfigPath(projectDir)
	}
}

// ClaudeDesktopConfigPath returns the path to Claude Desktop's config file.
func ClaudeDesktopConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "Claude", "claude_desktop_config.json")
	default: // linux
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			configDir = filepath.Join(home, ".config")
		}
		return filepath.Join(configDir, "Claude", "claude_desktop_config.json")
	}
}

// ClaudeCodeConfigPath returns the path to Claude Code's main config file.
func ClaudeCodeConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude.json")
}

// ClaudeCodePluginsPath returns the path to Claude Code's plugins directory.
func ClaudeCodePluginsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "plugins")
}

// ClaudeCodeSettings represents the structure of ~/.claude.json.
type ClaudeCodeSettings struct {
	MCPServers map[string]JSONMCPConfig `json:"mcpServers"`
}

// ClaudeCodeInstalledPlugins represents installed_plugins.json structure.
type ClaudeCodeInstalledPlugins struct {
	Version int                                    `json:"version"`
	Plugins map[string][]ClaudeCodePluginInstance `json:"plugins"`
}

// ClaudeCodePluginInstance represents a single plugin installation.
type ClaudeCodePluginInstance struct {
	Scope       string `json:"scope"`
	InstallPath string `json:"installPath"`
	Version     string `json:"version"`
}

// LoadClaudeCodeConfig loads MCP servers from Claude Code's user config and plugins.
func LoadClaudeCodeConfig() (*Config, error) {
	cfg := NewConfig()

	// Load from ~/.claude.json mcpServers
	mainPath := ClaudeCodeConfigPath()
	if mainPath != "" {
		if err := loadClaudeCodeMainConfig(mainPath, cfg); err != nil {
			return nil, err
		}
	}

	// Load from user-scoped plugin .mcp.json files
	pluginsPath := ClaudeCodePluginsPath()
	if pluginsPath != "" {
		if err := loadClaudeCodePluginMCPs(pluginsPath, cfg); err != nil {
			// Non-fatal - just skip plugins if we can't read them
		}
	}

	return cfg, nil
}

func loadClaudeCodeMainConfig(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var settings ClaudeCodeSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return err
	}

	for name, mcp := range settings.MCPServers {
		mcpType := mcp.Type
		if mcpType == "" && mcp.Command != "" {
			mcpType = "stdio"
		}
		if mcpType == "" && mcp.URL != "" {
			mcpType = "http"
		}
		cfg.MCPs[name] = MCPConfig{
			Name:    name,
			Type:    mcpType,
			Command: mcp.Command,
			Args:    mcp.Args,
			Env:     mcp.Env,
			URL:     mcp.URL,
			Headers: mcp.Headers,
			Timeout: mcp.Timeout,
			Dynamic: mcp.Dynamic,
		}
	}

	return nil
}

func loadClaudeCodePluginMCPs(pluginsPath string, cfg *Config) error {
	installedPath := filepath.Join(pluginsPath, "installed_plugins.json")
	data, err := os.ReadFile(installedPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var installed ClaudeCodeInstalledPlugins
	if err := json.Unmarshal(data, &installed); err != nil {
		return err
	}

	// Track which install paths we've already processed (plugins can have multiple instances)
	processed := make(map[string]bool)

	for _, instances := range installed.Plugins {
		for _, inst := range instances {
			// Only load user-scoped plugins
			if inst.Scope != "user" {
				continue
			}

			// Skip if we've already processed this path
			if processed[inst.InstallPath] {
				continue
			}
			processed[inst.InstallPath] = true

			// Load .mcp.json from this plugin
			mcpPath := filepath.Join(inst.InstallPath, ".mcp.json")
			mcpData, err := os.ReadFile(mcpPath)
			if err != nil {
				continue // Skip plugins without .mcp.json
			}

			var pluginMCP struct {
				MCPServers map[string]JSONMCPConfig `json:"mcpServers"`
			}
			if err := json.Unmarshal(mcpData, &pluginMCP); err != nil {
				continue
			}

			for name, mcp := range pluginMCP.MCPServers {
				// Don't overwrite MCPs from main config
				if _, exists := cfg.MCPs[name]; exists {
					continue
				}

				mcpType := mcp.Type
				if mcpType == "" && mcp.Command != "" {
					mcpType = "stdio"
				}
				if mcpType == "" && mcp.URL != "" {
					mcpType = "http"
				}
				cfg.MCPs[name] = MCPConfig{
					Name:    name,
					Type:    mcpType,
					Command: mcp.Command,
					Args:    mcp.Args,
					Env:     mcp.Env,
					URL:     mcp.URL,
					Headers: mcp.Headers,
					Timeout: mcp.Timeout,
					Dynamic: mcp.Dynamic,
				}
			}
		}
	}

	return nil
}

// LoadClaudeDesktopConfig loads MCP servers from Claude Desktop config.
func LoadClaudeDesktopConfig() (*Config, error) {
	path := ClaudeDesktopConfigPath()
	if path == "" {
		return NewConfig(), nil
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewConfig(), nil
	}
	if err != nil {
		return nil, err
	}

	var jsonCfg JSONConfig
	if err := json.Unmarshal(data, &jsonCfg); err != nil {
		return nil, err
	}

	cfg := NewConfig()
	for name, mcp := range jsonCfg.MCPServers {
		cfg.MCPs[name] = MCPConfig{
			Name:    name,
			Type:    mcp.Type,
			Command: mcp.Command,
			Args:    mcp.Args,
			Env:     mcp.Env,
			URL:     mcp.URL,
			Headers: mcp.Headers,
			Timeout: mcp.Timeout,
			Dynamic: mcp.Dynamic,
		}
	}

	return cfg, nil
}

// LoadUserConfig loads configuration from the user config file.
func LoadUserConfig() (*Config, error) {
	path := UserConfigPath()
	if path == "" {
		return NewConfig(), nil
	}
	return loadConfigFile(path, SourceUser)
}

// LoadProjectConfig loads configuration from the project config file.
func LoadProjectConfig(dir string) (*Config, error) {
	path := filepath.Join(dir, ProjectConfigFile)
	return loadConfigFile(path, SourceProject)
}

// LoadLocalConfig loads configuration from the local config file.
func LoadLocalConfig(dir string) (*Config, error) {
	path := filepath.Join(dir, LocalConfigFile)
	return loadConfigFile(path, SourceLocal)
}

// GetMCP returns a specific MCP config from a file.
// Handles both KDL and JSON (Claude Desktop) configs.
func GetMCP(path, name string) (*MCPConfig, error) {
	// Try loading as KDL first
	cfg, err := loadConfigFile(path, SourceProject)
	if err == nil {
		if mcp, ok := cfg.MCPs[name]; ok {
			mcp.Name = name
			return &mcp, nil
		}
	}

	// Try loading as JSON (Claude Desktop format)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}

	var jsonCfg JSONConfig
	if err := json.Unmarshal(data, &jsonCfg); err != nil {
		return nil, nil
	}

	if jmcp, ok := jsonCfg.MCPServers[name]; ok {
		return &MCPConfig{
			Name:    name,
			Type:    jmcp.Type,
			Command: jmcp.Command,
			Args:    jmcp.Args,
			Env:     jmcp.Env,
			URL:     jmcp.URL,
			Headers: jmcp.Headers,
			Timeout: jmcp.Timeout,
			Dynamic: jmcp.Dynamic,
		}, nil
	}

	return nil, nil
}

// ConfigPaths returns all relevant config file paths.
func ConfigPaths(projectDir string) map[string]string {
	return map[string]string{
		"user":                UserConfigPath(),
		"project":             ProjectConfigPath(projectDir),
		"local":               LocalConfigPath(projectDir),
		"claude_desktop":      ClaudeDesktopConfigPath(),
		"claude_code":         ClaudeCodeConfigPath(),
		"claude_code_plugins": ClaudeCodePluginsPath(),
	}
}

func loadConfigFile(path string, source Source) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewConfig(), nil
	}
	if err != nil {
		return nil, err
	}

	return ParseKDLConfig(string(data), source)
}

// ParseKDLConfig parses KDL configuration data.
func ParseKDLConfig(data string, source Source) (*Config, error) {
	var kdlCfg KDLConfig
	if err := kdl.Unmarshal([]byte(data), &kdlCfg); err != nil {
		return nil, err
	}

	cfg := NewConfig()
	for _, m := range kdlCfg.MCPs {
		cfg.MCPs[m.Name] = MCPConfig{
			Name:    m.Name,
			Type:    m.Type,
			Command: m.Command,
			Args:    m.Args,
			Env:     m.Env,
			URL:     m.URL,
			Headers: m.Headers,
			Timeout: m.Timeout,
			Dynamic: m.Dynamic,
			Source:  source,
		}
	}

	return cfg, nil
}

// AddMCPToFile adds an MCP configuration to a KDL file.
func AddMCPToFile(path, name, command string, args []string) error {
	return AddMCPConfigToFile(path, MCPConfig{
		Name:    name,
		Type:    "stdio",
		Command: command,
		Args:    args,
	})
}

// AddMCPConfigToFile adds a full MCP configuration to a KDL file.
func AddMCPConfigToFile(path string, mcp MCPConfig) error {
	// Load existing config or create new
	cfg, err := loadConfigFile(path, SourceProject)
	if err != nil {
		return err
	}

	// Set default type if not specified
	if mcp.Type == "" {
		mcp.Type = "stdio"
	}

	// Add the new MCP
	cfg.MCPs[mcp.Name] = mcp

	// Write back to file
	return WriteConfigFile(path, cfg)
}

// RemoveMCPFromFile removes an MCP configuration from a KDL file.
func RemoveMCPFromFile(path, name string) error {
	cfg, err := loadConfigFile(path, SourceProject)
	if err != nil {
		return err
	}

	delete(cfg.MCPs, name)
	return WriteConfigFile(path, cfg)
}

// WriteConfigFile writes a config to a KDL file.
func WriteConfigFile(path string, cfg *Config) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Build KDL content
	var content string
	for _, mcp := range cfg.MCPs {
		content += formatMCPBlock(mcp)
	}

	return os.WriteFile(path, []byte(content), 0644)
}

func formatMCPBlock(mcp MCPConfig) string {
	result := "mcp \"" + mcp.Name + "\" {\n"

	if mcp.Type != "" {
		result += "    type \"" + mcp.Type + "\"\n"
	} else {
		result += "    type \"command\"\n"
	}

	if mcp.Command != "" {
		result += "    command \"" + mcp.Command + "\"\n"
	}

	if len(mcp.Args) > 0 {
		result += "    args"
		for _, arg := range mcp.Args {
			result += " \"" + arg + "\""
		}
		result += "\n"
	}

	if mcp.URL != "" {
		result += "    url \"" + mcp.URL + "\"\n"
	}

	if mcp.Timeout != "" {
		result += "    timeout \"" + mcp.Timeout + "\"\n"
	}

	if mcp.Dynamic {
		result += "    dynamic true\n"
	}

	if len(mcp.Env) > 0 {
		result += "    env {\n"
		for k, v := range mcp.Env {
			result += "        " + k + " \"" + v + "\"\n"
		}
		result += "    }\n"
	}

	if len(mcp.Headers) > 0 {
		result += "    headers {\n"
		for k, v := range mcp.Headers {
			result += "        " + k + " \"" + v + "\"\n"
		}
		result += "    }\n"
	}

	result += "}\n\n"
	return result
}
