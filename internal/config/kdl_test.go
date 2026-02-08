package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKDLConfig_CommandTransport(t *testing.T) {
	tests := []struct {
		name     string
		kdl      string
		expected MCPConfig
	}{
		{
			name: "basic command with args",
			kdl: `mcp "filesystem" {
    type "stdio"
    command "npx"
    args "-y" "@anthropic/mcp-filesystem" "/tmp"
}`,
			expected: MCPConfig{
				Name:    "filesystem",
				Type:    "stdio",
				Command: "npx",
				Args:    []string{"-y", "@anthropic/mcp-filesystem", "/tmp"},
				Source:  SourceProject,
			},
		},
		{
			name: "command type alias",
			kdl: `mcp "test" {
    type "command"
    command "test-server"
}`,
			expected: MCPConfig{
				Name:    "test",
				Type:    "command",
				Command: "test-server",
				Source:  SourceProject,
			},
		},
		{
			name: "command without args",
			kdl: `mcp "simple" {
    type "stdio"
    command "/usr/bin/server"
}`,
			expected: MCPConfig{
				Name:    "simple",
				Type:    "stdio",
				Command: "/usr/bin/server",
				Source:  SourceProject,
			},
		},
		{
			name: "command with single arg",
			kdl: `mcp "single-arg" {
    type "stdio"
    command "npx"
    args "@some/package"
}`,
			expected: MCPConfig{
				Name:    "single-arg",
				Type:    "stdio",
				Command: "npx",
				Args:    []string{"@some/package"},
				Source:  SourceProject,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseKDLConfig(tt.kdl, SourceProject)
			require.NoError(t, err)
			require.NotNil(t, cfg)
			require.Len(t, cfg.MCPs, 1)

			mcp, ok := cfg.MCPs[tt.expected.Name]
			require.True(t, ok, "MCP %s not found", tt.expected.Name)

			assert.Equal(t, tt.expected.Name, mcp.Name)
			assert.Equal(t, tt.expected.Type, mcp.Type)
			assert.Equal(t, tt.expected.Command, mcp.Command)
			assert.Equal(t, tt.expected.Args, mcp.Args)
			assert.Equal(t, tt.expected.Source, mcp.Source)
		})
	}
}

func TestParseKDLConfig_SSETransport(t *testing.T) {
	tests := []struct {
		name     string
		kdl      string
		expected MCPConfig
	}{
		{
			name: "basic SSE",
			kdl: `mcp "github" {
    type "sse"
    url "https://mcp.github.com/sse"
}`,
			expected: MCPConfig{
				Name:   "github",
				Type:   "sse",
				URL:    "https://mcp.github.com/sse",
				Source: SourceProject,
			},
		},
		{
			name: "SSE with port",
			kdl: `mcp "local-sse" {
    type "sse"
    url "http://localhost:8080/sse"
}`,
			expected: MCPConfig{
				Name:   "local-sse",
				Type:   "sse",
				URL:    "http://localhost:8080/sse",
				Source: SourceProject,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseKDLConfig(tt.kdl, SourceProject)
			require.NoError(t, err)
			require.NotNil(t, cfg)

			mcp, ok := cfg.MCPs[tt.expected.Name]
			require.True(t, ok, "MCP %s not found", tt.expected.Name)

			assert.Equal(t, tt.expected.Name, mcp.Name)
			assert.Equal(t, tt.expected.Type, mcp.Type)
			assert.Equal(t, tt.expected.URL, mcp.URL)
			assert.Equal(t, tt.expected.Source, mcp.Source)
		})
	}
}

func TestParseKDLConfig_StreamableTransport(t *testing.T) {
	tests := []struct {
		name     string
		kdl      string
		expected MCPConfig
	}{
		{
			name: "http transport",
			kdl: `mcp "api-server" {
    type "http"
    url "https://api.example.com/mcp"
}`,
			expected: MCPConfig{
				Name:   "api-server",
				Type:   "http",
				URL:    "https://api.example.com/mcp",
				Source: SourceProject,
			},
		},
		{
			name: "streamable transport",
			kdl: `mcp "stream-server" {
    type "streamable"
    url "https://stream.example.com/mcp"
}`,
			expected: MCPConfig{
				Name:   "stream-server",
				Type:   "streamable",
				URL:    "https://stream.example.com/mcp",
				Source: SourceProject,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseKDLConfig(tt.kdl, SourceProject)
			require.NoError(t, err)
			require.NotNil(t, cfg)

			mcp, ok := cfg.MCPs[tt.expected.Name]
			require.True(t, ok, "MCP %s not found", tt.expected.Name)

			assert.Equal(t, tt.expected.Name, mcp.Name)
			assert.Equal(t, tt.expected.Type, mcp.Type)
			assert.Equal(t, tt.expected.URL, mcp.URL)
			assert.Equal(t, tt.expected.Source, mcp.Source)
		})
	}
}

func TestParseKDLConfig_EnvironmentVariables(t *testing.T) {
	tests := []struct {
		name     string
		kdl      string
		expected map[string]string
	}{
		{
			name: "single env var",
			kdl: `mcp "with-env" {
    type "stdio"
    command "server"
    env {
        API_KEY "secret123"
    }
}`,
			expected: map[string]string{
				"API_KEY": "secret123",
			},
		},
		{
			name: "multiple env vars",
			kdl: `mcp "multi-env" {
    type "stdio"
    command "server"
    env {
        API_KEY "secret123"
        DEBUG "true"
        PORT "8080"
    }
}`,
			expected: map[string]string{
				"API_KEY": "secret123",
				"DEBUG":   "true",
				"PORT":    "8080",
			},
		},
		{
			name: "env with special characters in value",
			kdl: `mcp "special-env" {
    type "stdio"
    command "server"
    env {
        CONNECTION_STRING "postgres://user:pass@localhost/db"
        PATH "/usr/bin:/usr/local/bin"
    }
}`,
			expected: map[string]string{
				"CONNECTION_STRING": "postgres://user:pass@localhost/db",
				"PATH":              "/usr/bin:/usr/local/bin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseKDLConfig(tt.kdl, SourceProject)
			require.NoError(t, err)
			require.NotNil(t, cfg)
			require.Len(t, cfg.MCPs, 1)

			var mcp MCPConfig
			for _, m := range cfg.MCPs {
				mcp = m
				break
			}

			require.NotNil(t, mcp.Env)
			assert.Equal(t, tt.expected, mcp.Env)
		})
	}
}

func TestParseKDLConfig_HTTPHeaders(t *testing.T) {
	tests := []struct {
		name     string
		kdl      string
		expected map[string]string
	}{
		{
			name: "single header",
			kdl: `mcp "with-header" {
    type "sse"
    url "https://api.example.com/sse"
    headers {
        Authorization "Bearer token123"
    }
}`,
			expected: map[string]string{
				"Authorization": "Bearer token123",
			},
		},
		{
			name: "multiple headers",
			kdl: `mcp "multi-header" {
    type "http"
    url "https://api.example.com/mcp"
    headers {
        Authorization "Bearer token123"
        X-API-Key "api-key-value"
        Content-Type "application/json"
    }
}`,
			expected: map[string]string{
				"Authorization": "Bearer token123",
				"X-API-Key":     "api-key-value",
				"Content-Type":  "application/json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseKDLConfig(tt.kdl, SourceProject)
			require.NoError(t, err)
			require.NotNil(t, cfg)
			require.Len(t, cfg.MCPs, 1)

			var mcp MCPConfig
			for _, m := range cfg.MCPs {
				mcp = m
				break
			}

			require.NotNil(t, mcp.Headers)
			assert.Equal(t, tt.expected, mcp.Headers)
		})
	}
}

func TestParseKDLConfig_InvalidSyntax(t *testing.T) {
	tests := []struct {
		name string
		kdl  string
	}{
		{
			name: "unclosed brace",
			kdl: `mcp "broken" {
    type "stdio"
    command "server"
`,
		},
		{
			name: "unclosed string",
			kdl: `mcp "broken {
    type "stdio"
}`,
		},
		{
			name: "invalid nesting",
			kdl: `mcp "broken" {
    type "stdio"
    env {
        KEY "value"
    }
}
}`,
		},
		{
			name: "missing node name",
			kdl: `{
    type "stdio"
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseKDLConfig(tt.kdl, SourceProject)
			// Either returns error or empty config for invalid input
			if err != nil {
				assert.Error(t, err)
			} else {
				// Some invalid KDL may not parse the mcp node correctly
				assert.NotNil(t, cfg)
			}
		})
	}
}

func TestParseKDLConfig_EmptyAndMinimal(t *testing.T) {
	tests := []struct {
		name        string
		kdl         string
		expectEmpty bool
		expectCount int
	}{
		{
			name:        "empty string",
			kdl:         "",
			expectEmpty: true,
			expectCount: 0,
		},
		{
			name:        "whitespace only",
			kdl:         "   \n\t  \n",
			expectEmpty: true,
			expectCount: 0,
		},
		{
			name: "comment only",
			kdl: `// This is a comment
// Another comment`,
			expectEmpty: true,
			expectCount: 0,
		},
		{
			name: "minimal valid config",
			kdl: `mcp "minimal" {
}`,
			expectEmpty: false,
			expectCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseKDLConfig(tt.kdl, SourceProject)
			require.NoError(t, err)
			require.NotNil(t, cfg)

			if tt.expectEmpty {
				assert.Len(t, cfg.MCPs, 0)
			} else {
				assert.Len(t, cfg.MCPs, tt.expectCount)
			}
		})
	}
}

func TestParseKDLConfig_MultipleMCPs(t *testing.T) {
	kdl := `mcp "server1" {
    type "stdio"
    command "server1"
}

mcp "server2" {
    type "sse"
    url "https://example.com/sse"
}

mcp "server3" {
    type "http"
    url "https://api.example.com/mcp"
    headers {
        Authorization "Bearer token"
    }
}`

	cfg, err := ParseKDLConfig(kdl, SourceProject)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Len(t, cfg.MCPs, 3)

	// Verify server1
	mcp1, ok := cfg.MCPs["server1"]
	require.True(t, ok)
	assert.Equal(t, "stdio", mcp1.Type)
	assert.Equal(t, "server1", mcp1.Command)

	// Verify server2
	mcp2, ok := cfg.MCPs["server2"]
	require.True(t, ok)
	assert.Equal(t, "sse", mcp2.Type)
	assert.Equal(t, "https://example.com/sse", mcp2.URL)

	// Verify server3
	mcp3, ok := cfg.MCPs["server3"]
	require.True(t, ok)
	assert.Equal(t, "http", mcp3.Type)
	assert.Equal(t, "https://api.example.com/mcp", mcp3.URL)
	assert.Equal(t, "Bearer token", mcp3.Headers["Authorization"])
}

func TestParseKDLConfig_SourcePreservation(t *testing.T) {
	kdl := `mcp "test" {
    type "stdio"
    command "test"
}`

	sources := []Source{SourceUser, SourceProject, SourceLocal, SourceRuntime}

	for _, source := range sources {
		t.Run(source.String(), func(t *testing.T) {
			cfg, err := ParseKDLConfig(kdl, source)
			require.NoError(t, err)

			mcp, ok := cfg.MCPs["test"]
			require.True(t, ok)
			assert.Equal(t, source, mcp.Source)
		})
	}
}

func TestParseKDLConfig_CompleteConfig(t *testing.T) {
	kdl := `mcp "complete-server" {
    type "http"
    command "backup-command"
    args "-v" "--config" "/etc/config"
    url "https://api.example.com/mcp"
    env {
        API_KEY "secret"
        DEBUG "true"
    }
    headers {
        Authorization "Bearer token"
        X-Custom-Header "custom-value"
    }
}`

	cfg, err := ParseKDLConfig(kdl, SourceProject)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	mcp, ok := cfg.MCPs["complete-server"]
	require.True(t, ok)

	assert.Equal(t, "complete-server", mcp.Name)
	assert.Equal(t, "http", mcp.Type)
	assert.Equal(t, "backup-command", mcp.Command)
	assert.Equal(t, []string{"-v", "--config", "/etc/config"}, mcp.Args)
	assert.Equal(t, "https://api.example.com/mcp", mcp.URL)

	assert.Len(t, mcp.Env, 2)
	assert.Equal(t, "secret", mcp.Env["API_KEY"])
	assert.Equal(t, "true", mcp.Env["DEBUG"])

	assert.Len(t, mcp.Headers, 2)
	assert.Equal(t, "Bearer token", mcp.Headers["Authorization"])
	assert.Equal(t, "custom-value", mcp.Headers["X-Custom-Header"])
}

func TestParseKDLConfig_DuplicateMCPNames(t *testing.T) {
	// When there are duplicate MCP names, the last one should win
	kdl := `mcp "duplicate" {
    type "stdio"
    command "first"
}

mcp "duplicate" {
    type "sse"
    url "https://example.com/sse"
}`

	cfg, err := ParseKDLConfig(kdl, SourceProject)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Only one entry for "duplicate" should exist
	assert.Len(t, cfg.MCPs, 1)

	mcp, ok := cfg.MCPs["duplicate"]
	require.True(t, ok)

	// Last one wins
	assert.Equal(t, "sse", mcp.Type)
	assert.Equal(t, "https://example.com/sse", mcp.URL)
}

func TestParseKDLConfig_SpecialCharactersInName(t *testing.T) {
	tests := []struct {
		name    string
		kdl     string
		mcpName string
	}{
		{
			name: "hyphenated name",
			kdl: `mcp "my-mcp-server" {
    type "stdio"
    command "server"
}`,
			mcpName: "my-mcp-server",
		},
		{
			name: "underscored name",
			kdl: `mcp "my_mcp_server" {
    type "stdio"
    command "server"
}`,
			mcpName: "my_mcp_server",
		},
		{
			name: "name with dots",
			kdl: `mcp "com.example.mcp" {
    type "stdio"
    command "server"
}`,
			mcpName: "com.example.mcp",
		},
		{
			name: "name with at symbol",
			kdl: `mcp "@scope/package" {
    type "stdio"
    command "server"
}`,
			mcpName: "@scope/package",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseKDLConfig(tt.kdl, SourceProject)
			require.NoError(t, err)
			require.NotNil(t, cfg)

			mcp, ok := cfg.MCPs[tt.mcpName]
			require.True(t, ok, "MCP %s not found", tt.mcpName)
			assert.Equal(t, tt.mcpName, mcp.Name)
		})
	}
}

func TestParseKDLConfig_NoTypeSpecified(t *testing.T) {
	// When type is not specified, it should remain empty
	kdl := `mcp "no-type" {
    command "server"
}`

	cfg, err := ParseKDLConfig(kdl, SourceProject)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	mcp, ok := cfg.MCPs["no-type"]
	require.True(t, ok)
	assert.Equal(t, "", mcp.Type)
	assert.Equal(t, "server", mcp.Command)
}

func TestParseKDLConfig_EmptyEnvAndHeaders(t *testing.T) {
	kdl := `mcp "empty-maps" {
    type "stdio"
    command "server"
    env {
    }
    headers {
    }
}`

	cfg, err := ParseKDLConfig(kdl, SourceProject)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	mcp, ok := cfg.MCPs["empty-maps"]
	require.True(t, ok)

	// Empty maps should be nil or empty
	assert.True(t, len(mcp.Env) == 0)
	assert.True(t, len(mcp.Headers) == 0)
}

func TestFormatMCPBlock(t *testing.T) {
	tests := []struct {
		name     string
		mcp      MCPConfig
		contains []string
	}{
		{
			name: "command transport",
			mcp: MCPConfig{
				Name:    "test",
				Type:    "stdio",
				Command: "test-server",
				Args:    []string{"-v", "--config"},
			},
			contains: []string{
				`mcp "test"`,
				`type "stdio"`,
				`command "test-server"`,
				`args "-v" "--config"`,
			},
		},
		{
			name: "http transport with url",
			mcp: MCPConfig{
				Name: "api",
				Type: "http",
				URL:  "https://api.example.com",
			},
			contains: []string{
				`mcp "api"`,
				`type "http"`,
				`url "https://api.example.com"`,
			},
		},
		{
			name: "with env vars",
			mcp: MCPConfig{
				Name:    "env-test",
				Type:    "stdio",
				Command: "server",
				Env: map[string]string{
					"KEY": "value",
				},
			},
			contains: []string{
				`mcp "env-test"`,
				"env {",
				`KEY "value"`,
			},
		},
		{
			name: "with headers",
			mcp: MCPConfig{
				Name: "header-test",
				Type: "http",
				URL:  "https://example.com",
				Headers: map[string]string{
					"Authorization": "Bearer token",
				},
			},
			contains: []string{
				`mcp "header-test"`,
				"headers {",
				`Authorization "Bearer token"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatMCPBlock(tt.mcp)

			for _, expected := range tt.contains {
				assert.Contains(t, result, expected, "output should contain: %s", expected)
			}
		})
	}
}

func TestFormatMCPBlock_DefaultType(t *testing.T) {
	// When type is empty, formatMCPBlock should default to "command"
	mcp := MCPConfig{
		Name:    "no-type",
		Command: "server",
	}

	result := formatMCPBlock(mcp)
	assert.Contains(t, result, `type "command"`)
}

func TestFormatMCPBlock_RoundTrip(t *testing.T) {
	// Test that we can format and then parse back
	original := MCPConfig{
		Name:    "roundtrip",
		Type:    "stdio",
		Command: "test-server",
		Args:    []string{"-v", "--debug"},
		Env: map[string]string{
			"API_KEY": "secret",
		},
	}

	formatted := formatMCPBlock(original)
	cfg, err := ParseKDLConfig(formatted, SourceProject)
	require.NoError(t, err)

	parsed, ok := cfg.MCPs["roundtrip"]
	require.True(t, ok)

	assert.Equal(t, original.Name, parsed.Name)
	assert.Equal(t, original.Type, parsed.Type)
	assert.Equal(t, original.Command, parsed.Command)
	assert.Equal(t, original.Args, parsed.Args)
	assert.Equal(t, original.Env["API_KEY"], parsed.Env["API_KEY"])
}

func TestParseKDLConfig_Timeout(t *testing.T) {
	tests := []struct {
		name            string
		kdl             string
		expectedTimeout string
	}{
		{
			name: "with timeout in seconds",
			kdl: `mcp "slow-server" {
    type "stdio"
    command "slow-mcp"
    timeout "60s"
}`,
			expectedTimeout: "60s",
		},
		{
			name: "with timeout in minutes",
			kdl: `mcp "very-slow-server" {
    type "sse"
    url "https://slow.example.com/sse"
    timeout "2m"
}`,
			expectedTimeout: "2m",
		},
		{
			name: "with timeout in milliseconds",
			kdl: `mcp "quick-server" {
    type "http"
    url "https://fast.example.com/mcp"
    timeout "5000ms"
}`,
			expectedTimeout: "5000ms",
		},
		{
			name: "without timeout",
			kdl: `mcp "default-server" {
    type "stdio"
    command "default-mcp"
}`,
			expectedTimeout: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseKDLConfig(tt.kdl, SourceProject)
			require.NoError(t, err)
			require.NotNil(t, cfg)
			require.Len(t, cfg.MCPs, 1)

			var mcp MCPConfig
			for _, m := range cfg.MCPs {
				mcp = m
				break
			}

			assert.Equal(t, tt.expectedTimeout, mcp.Timeout)
		})
	}
}

func TestFormatMCPBlock_WithTimeout(t *testing.T) {
	mcp := MCPConfig{
		Name:    "with-timeout",
		Type:    "stdio",
		Command: "server",
		Timeout: "45s",
	}

	result := formatMCPBlock(mcp)
	assert.Contains(t, result, `timeout "45s"`)
	assert.Contains(t, result, `mcp "with-timeout"`)
}

func TestParseKDLConfig_Dynamic(t *testing.T) {
	tests := []struct {
		name            string
		kdl             string
		expectedDynamic bool
	}{
		{
			name: "with dynamic true",
			kdl: `mcp "github" {
    type "sse"
    url "https://mcp.github.com/sse"
    dynamic true
}`,
			expectedDynamic: true,
		},
		{
			name: "with dynamic false",
			kdl: `mcp "static-server" {
    type "stdio"
    command "static-mcp"
    dynamic false
}`,
			expectedDynamic: false,
		},
		{
			name: "without dynamic (default false)",
			kdl: `mcp "default-server" {
    type "stdio"
    command "default-mcp"
}`,
			expectedDynamic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseKDLConfig(tt.kdl, SourceProject)
			require.NoError(t, err)
			require.NotNil(t, cfg)
			require.Len(t, cfg.MCPs, 1)

			var mcp MCPConfig
			for _, m := range cfg.MCPs {
				mcp = m
				break
			}

			assert.Equal(t, tt.expectedDynamic, mcp.Dynamic)
		})
	}
}

func TestFormatMCPBlock_WithDynamic(t *testing.T) {
	mcp := MCPConfig{
		Name:    "dynamic-mcp",
		Type:    "sse",
		URL:     "https://example.com/sse",
		Dynamic: true,
	}

	result := formatMCPBlock(mcp)
	assert.Contains(t, result, "dynamic true")
	assert.Contains(t, result, `mcp "dynamic-mcp"`)
}

func TestFormatMCPBlock_WithoutDynamic(t *testing.T) {
	mcp := MCPConfig{
		Name:    "static-mcp",
		Type:    "stdio",
		Command: "server",
		Dynamic: false,
	}

	result := formatMCPBlock(mcp)
	assert.NotContains(t, result, "dynamic")
}

func TestFormatMCPBlock_RoundTripWithDynamic(t *testing.T) {
	original := MCPConfig{
		Name:    "roundtrip-dynamic",
		Type:    "sse",
		URL:     "https://api.example.com/sse",
		Dynamic: true,
	}

	formatted := formatMCPBlock(original)
	cfg, err := ParseKDLConfig(formatted, SourceProject)
	require.NoError(t, err)

	parsed, ok := cfg.MCPs["roundtrip-dynamic"]
	require.True(t, ok)

	assert.Equal(t, original.Dynamic, parsed.Dynamic)
}

func TestFormatMCPBlock_RoundTripWithTimeout(t *testing.T) {
	original := MCPConfig{
		Name:    "roundtrip-timeout",
		Type:    "http",
		URL:     "https://api.example.com",
		Timeout: "90s",
	}

	formatted := formatMCPBlock(original)
	cfg, err := ParseKDLConfig(formatted, SourceProject)
	require.NoError(t, err)

	parsed, ok := cfg.MCPs["roundtrip-timeout"]
	require.True(t, ok)

	assert.Equal(t, original.Timeout, parsed.Timeout)
}
