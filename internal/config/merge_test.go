package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMerge_NilConfigs tests merge behavior with nil inputs.
func TestMerge_NilConfigs(t *testing.T) {
	tests := []struct {
		name     string
		user     *Config
		project  *Config
		expected int
	}{
		{
			name:     "both nil",
			user:     nil,
			project:  nil,
			expected: 0,
		},
		{
			name:     "user nil",
			user:     nil,
			project:  configWithMCPs(map[string]MCPConfig{"proj": {Name: "proj", Type: "stdio"}}),
			expected: 1,
		},
		{
			name:     "project nil",
			user:     configWithMCPs(map[string]MCPConfig{"user": {Name: "user", Type: "stdio"}}),
			project:  nil,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Merge(tt.user, tt.project)
			require.NotNil(t, result)
			assert.Len(t, result.MCPs, tt.expected)
		})
	}
}

// TestMerge_UserOnlyConfig tests that user config loads correctly when no project config exists.
func TestMerge_UserOnlyConfig(t *testing.T) {
	user := configWithMCPs(map[string]MCPConfig{
		"user-mcp-1": {
			Name:    "user-mcp-1",
			Type:    "stdio",
			Command: "user-server",
			Source:  SourceUser,
		},
		"user-mcp-2": {
			Name: "user-mcp-2",
			Type: "sse",
			URL:  "https://user.example.com/sse",
			Env: map[string]string{
				"API_KEY": "user-secret",
			},
			Source: SourceUser,
		},
	})

	result := Merge(user, nil)

	require.NotNil(t, result)
	assert.Len(t, result.MCPs, 2)

	// Verify user MCPs are present
	mcp1, ok := result.MCPs["user-mcp-1"]
	require.True(t, ok)
	assert.Equal(t, "user-server", mcp1.Command)
	assert.Equal(t, SourceUser, mcp1.Source)

	mcp2, ok := result.MCPs["user-mcp-2"]
	require.True(t, ok)
	assert.Equal(t, "https://user.example.com/sse", mcp2.URL)
	assert.Equal(t, "user-secret", mcp2.Env["API_KEY"])
}

// TestMerge_ProjectOverridesUser tests that project config takes precedence for MCPs with same name.
func TestMerge_ProjectOverridesUser(t *testing.T) {
	user := configWithMCPs(map[string]MCPConfig{
		"shared-mcp": {
			Name:    "shared-mcp",
			Type:    "stdio",
			Command: "user-command",
			Args:    []string{"--user"},
			Source:  SourceUser,
		},
		"user-only": {
			Name:    "user-only",
			Type:    "stdio",
			Command: "user-only-cmd",
			Source:  SourceUser,
		},
	})

	project := configWithMCPs(map[string]MCPConfig{
		"shared-mcp": {
			Name:    "shared-mcp",
			Type:    "sse",
			URL:     "https://project.example.com/sse",
			Source:  SourceProject,
		},
		"project-only": {
			Name:    "project-only",
			Type:    "http",
			URL:     "https://project-only.example.com",
			Source:  SourceProject,
		},
	})

	result := Merge(user, project)

	require.NotNil(t, result)
	assert.Len(t, result.MCPs, 3)

	// Project overrides user for shared-mcp
	sharedMCP, ok := result.MCPs["shared-mcp"]
	require.True(t, ok)
	assert.Equal(t, "sse", sharedMCP.Type)
	assert.Equal(t, "https://project.example.com/sse", sharedMCP.URL)
	assert.Equal(t, "", sharedMCP.Command, "user command should be overwritten")
	assert.Equal(t, SourceProject, sharedMCP.Source)

	// User-only MCP preserved
	userOnly, ok := result.MCPs["user-only"]
	require.True(t, ok)
	assert.Equal(t, "user-only-cmd", userOnly.Command)
	assert.Equal(t, SourceUser, userOnly.Source)

	// Project-only MCP added
	projectOnly, ok := result.MCPs["project-only"]
	require.True(t, ok)
	assert.Equal(t, "https://project-only.example.com", projectOnly.URL)
	assert.Equal(t, SourceProject, projectOnly.Source)
}

// TestMerge_DifferentNamesCombine tests that MCPs with different names are all included.
func TestMerge_DifferentNamesCombine(t *testing.T) {
	user := configWithMCPs(map[string]MCPConfig{
		"a": {Name: "a", Type: "stdio", Command: "a-cmd"},
		"b": {Name: "b", Type: "stdio", Command: "b-cmd"},
	})

	project := configWithMCPs(map[string]MCPConfig{
		"c": {Name: "c", Type: "stdio", Command: "c-cmd"},
		"d": {Name: "d", Type: "stdio", Command: "d-cmd"},
	})

	result := Merge(user, project)

	require.NotNil(t, result)
	assert.Len(t, result.MCPs, 4)

	for _, name := range []string{"a", "b", "c", "d"} {
		_, ok := result.MCPs[name]
		assert.True(t, ok, "MCP %s should exist", name)
	}
}

// TestMerge_PreservesAllFields tests that all MCP config fields are preserved during merge.
func TestMerge_PreservesAllFields(t *testing.T) {
	user := configWithMCPs(map[string]MCPConfig{
		"complete": {
			Name:    "complete",
			Type:    "http",
			Command: "backup-cmd",
			Args:    []string{"-v", "--debug"},
			Env: map[string]string{
				"API_KEY": "secret",
				"DEBUG":   "true",
			},
			URL: "https://api.example.com",
			Headers: map[string]string{
				"Authorization": "Bearer token",
				"X-Custom":      "value",
			},
			Source: SourceUser,
		},
	})

	result := Merge(user, nil)

	mcp, ok := result.MCPs["complete"]
	require.True(t, ok)

	assert.Equal(t, "complete", mcp.Name)
	assert.Equal(t, "http", mcp.Type)
	assert.Equal(t, "backup-cmd", mcp.Command)
	assert.Equal(t, []string{"-v", "--debug"}, mcp.Args)
	assert.Equal(t, "https://api.example.com", mcp.URL)
	assert.Equal(t, "secret", mcp.Env["API_KEY"])
	assert.Equal(t, "true", mcp.Env["DEBUG"])
	assert.Equal(t, "Bearer token", mcp.Headers["Authorization"])
	assert.Equal(t, "value", mcp.Headers["X-Custom"])
	assert.Equal(t, SourceUser, mcp.Source)
}

// TestMerge_EmptyConfigs tests merge with empty configs.
func TestMerge_EmptyConfigs(t *testing.T) {
	user := NewConfig()
	project := NewConfig()

	result := Merge(user, project)

	require.NotNil(t, result)
	assert.Len(t, result.MCPs, 0)
}

// TestThreeTierMerge_LocalOverridesProjectOverridesUser tests the three-tier merge pattern.
// Note: The Merge function only does two-tier, but this pattern is used in run.go.
func TestThreeTierMerge_LocalOverridesProjectOverridesUser(t *testing.T) {
	// Simulate three-tier merge as done in cmd/slop-mcp/run.go
	user := configWithMCPs(map[string]MCPConfig{
		"shared": {Name: "shared", Type: "stdio", Command: "user-cmd", Source: SourceUser},
		"user-only": {Name: "user-only", Type: "stdio", Command: "user-only-cmd", Source: SourceUser},
	})

	project := configWithMCPs(map[string]MCPConfig{
		"shared": {Name: "shared", Type: "sse", URL: "https://project.example.com", Source: SourceProject},
		"project-only": {Name: "project-only", Type: "http", URL: "https://proj.example.com", Source: SourceProject},
	})

	local := configWithMCPs(map[string]MCPConfig{
		"shared": {Name: "shared", Type: "http", URL: "https://local.example.com", Source: SourceLocal},
		"local-only": {Name: "local-only", Type: "stdio", Command: "local-cmd", Source: SourceLocal},
	})

	// Apply three-tier merge: user -> project -> local
	merged := NewConfig()

	// Layer 1: user
	for name, mcp := range user.MCPs {
		merged.MCPs[name] = mcp
	}
	// Layer 2: project overrides
	for name, mcp := range project.MCPs {
		merged.MCPs[name] = mcp
	}
	// Layer 3: local overrides
	for name, mcp := range local.MCPs {
		merged.MCPs[name] = mcp
	}

	assert.Len(t, merged.MCPs, 4)

	// shared should have local config (highest priority)
	sharedMCP := merged.MCPs["shared"]
	assert.Equal(t, "http", sharedMCP.Type)
	assert.Equal(t, "https://local.example.com", sharedMCP.URL)
	assert.Equal(t, SourceLocal, sharedMCP.Source)

	// user-only should be preserved
	userOnly := merged.MCPs["user-only"]
	assert.Equal(t, "user-only-cmd", userOnly.Command)
	assert.Equal(t, SourceUser, userOnly.Source)

	// project-only should be preserved
	projectOnly := merged.MCPs["project-only"]
	assert.Equal(t, "https://proj.example.com", projectOnly.URL)
	assert.Equal(t, SourceProject, projectOnly.Source)

	// local-only should be present
	localOnly := merged.MCPs["local-only"]
	assert.Equal(t, "local-cmd", localOnly.Command)
	assert.Equal(t, SourceLocal, localOnly.Source)
}

// TestLoadUserConfig_FileNotExist tests that LoadUserConfig returns empty config when file doesn't exist.
func TestLoadUserConfig_FromTempDir(t *testing.T) {
	// Create a temp config directory
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "slop-mcp")
	require.NoError(t, os.MkdirAll(configDir, 0755))

	// Set XDG_CONFIG_HOME to our temp directory
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", oldXDG)

	// Test with no config file
	cfg, err := LoadUserConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Len(t, cfg.MCPs, 0)

	// Create a config file
	configPath := filepath.Join(configDir, UserConfigFile)
	kdlContent := `mcp "user-server" {
    type "stdio"
    command "user-cmd"
}`
	require.NoError(t, os.WriteFile(configPath, []byte(kdlContent), 0644))

	// Now load should return the MCP
	cfg, err = LoadUserConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Len(t, cfg.MCPs, 1)

	mcp, ok := cfg.MCPs["user-server"]
	require.True(t, ok)
	assert.Equal(t, "user-cmd", mcp.Command)
	assert.Equal(t, SourceUser, mcp.Source)
}

// TestLoadProjectConfig_FromTempDir tests LoadProjectConfig with temp directory.
func TestLoadProjectConfig_FromTempDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with no config file
	cfg, err := LoadProjectConfig(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Len(t, cfg.MCPs, 0)

	// Create project config
	configPath := filepath.Join(tmpDir, ProjectConfigFile)
	kdlContent := `mcp "project-server" {
    type "sse"
    url "https://project.example.com/sse"
}`
	require.NoError(t, os.WriteFile(configPath, []byte(kdlContent), 0644))

	// Now load should return the MCP
	cfg, err = LoadProjectConfig(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Len(t, cfg.MCPs, 1)

	mcp, ok := cfg.MCPs["project-server"]
	require.True(t, ok)
	assert.Equal(t, "https://project.example.com/sse", mcp.URL)
	assert.Equal(t, SourceProject, mcp.Source)
}

// TestLoadLocalConfig_FromTempDir tests LoadLocalConfig with temp directory.
func TestLoadLocalConfig_FromTempDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with no config file
	cfg, err := LoadLocalConfig(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Len(t, cfg.MCPs, 0)

	// Create local config
	configPath := filepath.Join(tmpDir, LocalConfigFile)
	kdlContent := `mcp "local-server" {
    type "http"
    url "https://local.example.com/mcp"
    env {
        SECRET "local-secret"
    }
}`
	require.NoError(t, os.WriteFile(configPath, []byte(kdlContent), 0644))

	// Now load should return the MCP
	cfg, err = LoadLocalConfig(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Len(t, cfg.MCPs, 1)

	mcp, ok := cfg.MCPs["local-server"]
	require.True(t, ok)
	assert.Equal(t, "https://local.example.com/mcp", mcp.URL)
	assert.Equal(t, "local-secret", mcp.Env["SECRET"])
	assert.Equal(t, SourceLocal, mcp.Source)
}

// TestLoad_MergesUserAndProject tests the Load function which merges user and project configs.
func TestLoad_MergesUserAndProject(t *testing.T) {
	// Set up temp directories
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config", "slop-mcp")
	projectDir := filepath.Join(tmpDir, "project")
	require.NoError(t, os.MkdirAll(configDir, 0755))
	require.NoError(t, os.MkdirAll(projectDir, 0755))

	// Set XDG_CONFIG_HOME
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "config"))
	defer os.Setenv("XDG_CONFIG_HOME", oldXDG)

	// Create user config
	userConfigPath := filepath.Join(configDir, UserConfigFile)
	userKDL := `mcp "user-mcp" {
    type "stdio"
    command "user-cmd"
}

mcp "shared-mcp" {
    type "stdio"
    command "user-shared-cmd"
}`
	require.NoError(t, os.WriteFile(userConfigPath, []byte(userKDL), 0644))

	// Create project config
	projectConfigPath := filepath.Join(projectDir, ProjectConfigFile)
	projectKDL := `mcp "project-mcp" {
    type "sse"
    url "https://project.example.com"
}

mcp "shared-mcp" {
    type "http"
    url "https://project-shared.example.com"
}`
	require.NoError(t, os.WriteFile(projectConfigPath, []byte(projectKDL), 0644))

	// Load merged config
	cfg, err := Load(projectDir)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Len(t, cfg.MCPs, 3)

	// User-only MCP
	userMCP, ok := cfg.MCPs["user-mcp"]
	require.True(t, ok)
	assert.Equal(t, "user-cmd", userMCP.Command)

	// Project-only MCP
	projectMCP, ok := cfg.MCPs["project-mcp"]
	require.True(t, ok)
	assert.Equal(t, "https://project.example.com", projectMCP.URL)

	// Shared MCP should have project config (project overrides user)
	sharedMCP, ok := cfg.MCPs["shared-mcp"]
	require.True(t, ok)
	assert.Equal(t, "http", sharedMCP.Type)
	assert.Equal(t, "https://project-shared.example.com", sharedMCP.URL)
}

// TestLoadClaudeDesktopConfig_ValidJSON tests loading Claude Desktop config.
func TestLoadClaudeDesktopConfig_ValidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create Claude Desktop config structure
	var configPath string
	switch {
	case os.Getenv("APPDATA") != "":
		// Windows-like
		configPath = filepath.Join(tmpDir, "Claude", "claude_desktop_config.json")
	default:
		// Linux/macOS
		configPath = filepath.Join(tmpDir, "Claude", "claude_desktop_config.json")
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))

	// Note: This test doesn't actually test LoadClaudeDesktopConfig because
	// it uses hardcoded paths. We test the JSON parsing logic instead.
	jsonContent := `{
    "mcpServers": {
        "filesystem": {
            "command": "npx",
            "args": ["-y", "@anthropic/mcp-filesystem", "/tmp"],
            "type": "stdio"
        },
        "github": {
            "url": "https://mcp.github.com/sse",
            "type": "sse",
            "headers": {
                "Authorization": "Bearer gh-token"
            }
        }
    }
}`
	require.NoError(t, os.WriteFile(configPath, []byte(jsonContent), 0644))

	// Test JSON parsing directly since LoadClaudeDesktopConfig uses hardcoded paths
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var jsonCfg JSONConfig
	require.NoError(t, json.Unmarshal(data, &jsonCfg))

	// Verify filesystem MCP
	fs, ok := jsonCfg.MCPServers["filesystem"]
	require.True(t, ok)
	assert.Equal(t, "npx", fs.Command)
	assert.Equal(t, []string{"-y", "@anthropic/mcp-filesystem", "/tmp"}, fs.Args)
	assert.Equal(t, "stdio", fs.Type)

	// Verify github MCP
	gh, ok := jsonCfg.MCPServers["github"]
	require.True(t, ok)
	assert.Equal(t, "https://mcp.github.com/sse", gh.URL)
	assert.Equal(t, "sse", gh.Type)
	assert.Equal(t, "Bearer gh-token", gh.Headers["Authorization"])
}

// TestLoadClaudeCodeConfig_Structure tests the Claude Code config structure parsing.
func TestLoadClaudeCodeConfig_Structure(t *testing.T) {
	// Test ClaudeCodeSettings JSON parsing
	jsonContent := `{
    "mcpServers": {
        "code-mcp": {
            "command": "code-mcp-server",
            "args": ["--verbose"],
            "env": {
                "API_KEY": "code-key"
            }
        },
        "remote-mcp": {
            "url": "https://remote.example.com/mcp",
            "headers": {
                "X-Code-Header": "code-value"
            }
        }
    }
}`

	var settings ClaudeCodeSettings
	require.NoError(t, json.Unmarshal([]byte(jsonContent), &settings))

	assert.Len(t, settings.MCPServers, 2)

	// Verify code-mcp
	codeMCP, ok := settings.MCPServers["code-mcp"]
	require.True(t, ok)
	assert.Equal(t, "code-mcp-server", codeMCP.Command)
	assert.Equal(t, []string{"--verbose"}, codeMCP.Args)
	assert.Equal(t, "code-key", codeMCP.Env["API_KEY"])

	// Verify remote-mcp
	remoteMCP, ok := settings.MCPServers["remote-mcp"]
	require.True(t, ok)
	assert.Equal(t, "https://remote.example.com/mcp", remoteMCP.URL)
	assert.Equal(t, "code-value", remoteMCP.Headers["X-Code-Header"])
}

// TestClaudeCodePluginStructure tests the Claude Code plugin structure parsing.
func TestClaudeCodePluginStructure(t *testing.T) {
	jsonContent := `{
    "version": 1,
    "plugins": {
        "my-plugin": [
            {
                "scope": "user",
                "installPath": "/home/user/.claude/plugins/my-plugin",
                "version": "1.0.0"
            },
            {
                "scope": "project",
                "installPath": "/project/.claude/plugins/my-plugin",
                "version": "1.0.0"
            }
        ],
        "another-plugin": [
            {
                "scope": "user",
                "installPath": "/home/user/.claude/plugins/another-plugin",
                "version": "2.0.0"
            }
        ]
    }
}`

	var installed ClaudeCodeInstalledPlugins
	require.NoError(t, json.Unmarshal([]byte(jsonContent), &installed))

	assert.Equal(t, 1, installed.Version)
	assert.Len(t, installed.Plugins, 2)

	// Verify my-plugin instances
	myPlugin := installed.Plugins["my-plugin"]
	assert.Len(t, myPlugin, 2)
	assert.Equal(t, "user", myPlugin[0].Scope)
	assert.Equal(t, "project", myPlugin[1].Scope)

	// Verify another-plugin
	anotherPlugin := installed.Plugins["another-plugin"]
	assert.Len(t, anotherPlugin, 1)
	assert.Equal(t, "user", anotherPlugin[0].Scope)
	assert.Equal(t, "2.0.0", anotherPlugin[0].Version)
}

// TestConfigPathForScope tests getting config paths for different scopes.
func TestConfigPathForScope(t *testing.T) {
	projectDir := "/project/dir"

	tests := []struct {
		scope    Scope
		expected string
	}{
		{ScopeLocal, filepath.Join(projectDir, LocalConfigFile)},
		{ScopeProject, filepath.Join(projectDir, ProjectConfigFile)},
	}

	for _, tt := range tests {
		t.Run(tt.scope.String(), func(t *testing.T) {
			path := ConfigPathForScope(tt.scope, projectDir)
			assert.Equal(t, tt.expected, path)
		})
	}
}

// TestScope_String tests Scope string representation.
func TestScope_String(t *testing.T) {
	tests := []struct {
		scope    Scope
		expected string
	}{
		{ScopeLocal, "local"},
		{ScopeProject, "project"},
		{ScopeUser, "user"},
		{Scope(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.scope.String())
		})
	}
}

// TestParseScope tests parsing scope strings.
func TestParseScope(t *testing.T) {
	tests := []struct {
		input    string
		expected Scope
	}{
		{"local", ScopeLocal},
		{"project", ScopeProject},
		{"user", ScopeUser},
		{"unknown", ScopeProject}, // default
		{"", ScopeProject},        // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, ParseScope(tt.input))
		})
	}
}

// TestSource_String tests Source string representation.
func TestSource_String(t *testing.T) {
	tests := []struct {
		source   Source
		expected string
	}{
		{SourceUser, "user"},
		{SourceProject, "project"},
		{SourceLocal, "local"},
		{SourceRuntime, "runtime"},
		{Source(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.source.String())
		})
	}
}

// TestParseJSONConfig tests parsing JSON MCP config.
func TestParseJSONConfig(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected *MCPConfig
		hasError bool
	}{
		{
			name: "stdio with command",
			json: `{"type": "stdio", "command": "server", "args": ["-v"]}`,
			expected: &MCPConfig{
				Type:    "stdio",
				Command: "server",
				Args:    []string{"-v"},
			},
		},
		{
			name: "http with url and headers",
			json: `{"type": "http", "url": "https://api.example.com", "headers": {"Auth": "token"}}`,
			expected: &MCPConfig{
				Type: "http",
				URL:  "https://api.example.com",
				Headers: map[string]string{
					"Auth": "token",
				},
			},
		},
		{
			name: "with env vars",
			json: `{"type": "stdio", "command": "server", "env": {"KEY": "value"}}`,
			expected: &MCPConfig{
				Type:    "stdio",
				Command: "server",
				Env: map[string]string{
					"KEY": "value",
				},
			},
		},
		{
			name:     "invalid json",
			json:     `{"type": "stdio", invalid}`,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseJSONConfig(tt.json)
			if tt.hasError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected.Type, cfg.Type)
			assert.Equal(t, tt.expected.Command, cfg.Command)
			assert.Equal(t, tt.expected.URL, cfg.URL)
			assert.Equal(t, tt.expected.Args, cfg.Args)
			assert.Equal(t, tt.expected.Env, cfg.Env)
			assert.Equal(t, tt.expected.Headers, cfg.Headers)
		})
	}
}

// TestMCPConfig_ToJSON tests MCP config JSON serialization.
func TestMCPConfig_ToJSON(t *testing.T) {
	cfg := &MCPConfig{
		Name:    "test",
		Type:    "stdio",
		Command: "server",
		Args:    []string{"-v"},
		Env: map[string]string{
			"KEY": "value",
		},
	}

	jsonStr := cfg.ToJSON()
	assert.Contains(t, jsonStr, `"type": "stdio"`)
	assert.Contains(t, jsonStr, `"command": "server"`)
	assert.Contains(t, jsonStr, `"args"`)
	assert.Contains(t, jsonStr, `"-v"`)
	assert.Contains(t, jsonStr, `"env"`)
	assert.Contains(t, jsonStr, `"KEY": "value"`)
}

// TestGetMCP_FromKDLFile tests GetMCP from KDL config file.
func TestGetMCP_FromKDLFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")

	kdlContent := `mcp "server1" {
    type "stdio"
    command "cmd1"
}

mcp "server2" {
    type "sse"
    url "https://example.com"
}`
	require.NoError(t, os.WriteFile(configPath, []byte(kdlContent), 0644))

	// Get existing MCP
	mcp, err := GetMCP(configPath, "server1")
	require.NoError(t, err)
	require.NotNil(t, mcp)
	assert.Equal(t, "server1", mcp.Name)
	assert.Equal(t, "cmd1", mcp.Command)

	// Get another existing MCP
	mcp, err = GetMCP(configPath, "server2")
	require.NoError(t, err)
	require.NotNil(t, mcp)
	assert.Equal(t, "server2", mcp.Name)
	assert.Equal(t, "https://example.com", mcp.URL)

	// Get non-existing MCP
	mcp, err = GetMCP(configPath, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, mcp)
}

// TestGetMCP_FromJSONFile tests GetMCP from JSON config file.
func TestGetMCP_FromJSONFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	jsonContent := `{
    "mcpServers": {
        "json-server": {
            "type": "stdio",
            "command": "json-cmd",
            "args": ["--json"]
        }
    }
}`
	require.NoError(t, os.WriteFile(configPath, []byte(jsonContent), 0644))

	// Get existing MCP from JSON
	mcp, err := GetMCP(configPath, "json-server")
	require.NoError(t, err)
	require.NotNil(t, mcp)
	assert.Equal(t, "json-server", mcp.Name)
	assert.Equal(t, "json-cmd", mcp.Command)
	assert.Equal(t, []string{"--json"}, mcp.Args)

	// Get non-existing MCP
	mcp, err = GetMCP(configPath, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, mcp)
}

// TestAddMCPToFile tests adding an MCP to a config file.
func TestAddMCPToFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")

	// Add first MCP to new file
	err := AddMCPToFile(configPath, "first", "first-cmd", []string{"-v"})
	require.NoError(t, err)

	// Verify file contents
	cfg, err := loadConfigFile(configPath, SourceProject)
	require.NoError(t, err)
	assert.Len(t, cfg.MCPs, 1)
	assert.Equal(t, "first-cmd", cfg.MCPs["first"].Command)

	// Add second MCP
	err = AddMCPToFile(configPath, "second", "second-cmd", nil)
	require.NoError(t, err)

	// Verify both MCPs exist
	cfg, err = loadConfigFile(configPath, SourceProject)
	require.NoError(t, err)
	assert.Len(t, cfg.MCPs, 2)
	assert.Equal(t, "first-cmd", cfg.MCPs["first"].Command)
	assert.Equal(t, "second-cmd", cfg.MCPs["second"].Command)
}

// TestAddMCPConfigToFile tests adding a full MCP config to a file.
func TestAddMCPConfigToFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")

	mcp := MCPConfig{
		Name:    "full-config",
		Type:    "http",
		URL:     "https://example.com",
		Headers: map[string]string{"Auth": "token"},
		Env:     map[string]string{"KEY": "value"},
	}

	err := AddMCPConfigToFile(configPath, mcp)
	require.NoError(t, err)

	cfg, err := loadConfigFile(configPath, SourceProject)
	require.NoError(t, err)
	assert.Len(t, cfg.MCPs, 1)

	loaded := cfg.MCPs["full-config"]
	assert.Equal(t, "http", loaded.Type)
	assert.Equal(t, "https://example.com", loaded.URL)
	assert.Equal(t, "token", loaded.Headers["Auth"])
	assert.Equal(t, "value", loaded.Env["KEY"])
}

// TestRemoveMCPFromFile tests removing an MCP from a config file.
func TestRemoveMCPFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")

	// Create initial config with two MCPs
	cfg := NewConfig()
	cfg.MCPs["keep"] = MCPConfig{Name: "keep", Type: "stdio", Command: "keep-cmd"}
	cfg.MCPs["remove"] = MCPConfig{Name: "remove", Type: "stdio", Command: "remove-cmd"}
	require.NoError(t, WriteConfigFile(configPath, cfg))

	// Remove one MCP
	err := RemoveMCPFromFile(configPath, "remove")
	require.NoError(t, err)

	// Verify only one remains
	cfg, err = loadConfigFile(configPath, SourceProject)
	require.NoError(t, err)
	assert.Len(t, cfg.MCPs, 1)
	assert.Contains(t, cfg.MCPs, "keep")
	assert.NotContains(t, cfg.MCPs, "remove")
}

// TestWriteConfigFile tests writing config to file.
func TestWriteConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub", "dir")
	configPath := filepath.Join(subDir, "config.kdl")

	cfg := NewConfig()
	cfg.MCPs["test"] = MCPConfig{
		Name:    "test",
		Type:    "stdio",
		Command: "test-cmd",
	}

	// WriteConfigFile should create directories if needed
	err := WriteConfigFile(configPath, cfg)
	require.NoError(t, err)

	// Verify file exists and is readable
	loaded, err := loadConfigFile(configPath, SourceProject)
	require.NoError(t, err)
	assert.Len(t, loaded.MCPs, 1)
	assert.Equal(t, "test-cmd", loaded.MCPs["test"].Command)
}

// TestConfigPaths tests getting all config paths.
func TestConfigPaths(t *testing.T) {
	projectDir := "/project/dir"
	paths := ConfigPaths(projectDir)

	assert.NotEmpty(t, paths["user"])
	assert.Equal(t, filepath.Join(projectDir, ProjectConfigFile), paths["project"])
	assert.Equal(t, filepath.Join(projectDir, LocalConfigFile), paths["local"])
	assert.NotEmpty(t, paths["claude_desktop"])
	assert.NotEmpty(t, paths["claude_code"])
	assert.NotEmpty(t, paths["claude_code_plugins"])
}

// TestNewConfig tests creating a new config.
func TestNewConfig(t *testing.T) {
	cfg := NewConfig()
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.MCPs)
	assert.Len(t, cfg.MCPs, 0)
}

// Helper function to create a config with MCPs.
func configWithMCPs(mcps map[string]MCPConfig) *Config {
	return &Config{MCPs: mcps}
}
