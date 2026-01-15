package registry

import (
	"os"
	"testing"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestGetConnectionTimeout_Default(t *testing.T) {
	// Clear env var to ensure default behavior
	os.Unsetenv(TimeoutEnvVar)

	cfg := config.MCPConfig{
		Name:    "test",
		Type:    "stdio",
		Command: "test",
	}

	timeout := GetConnectionTimeout(cfg)
	assert.Equal(t, DefaultConnectionTimeout, timeout)
	assert.Equal(t, 30*time.Second, timeout)
}

func TestGetConnectionTimeout_PerMCPConfig(t *testing.T) {
	// Clear env var to isolate per-MCP test
	os.Unsetenv(TimeoutEnvVar)

	tests := []struct {
		name     string
		timeout  string
		expected time.Duration
	}{
		{
			name:     "seconds",
			timeout:  "45s",
			expected: 45 * time.Second,
		},
		{
			name:     "minutes",
			timeout:  "2m",
			expected: 2 * time.Minute,
		},
		{
			name:     "milliseconds",
			timeout:  "5000ms",
			expected: 5000 * time.Millisecond,
		},
		{
			name:     "combined",
			timeout:  "1m30s",
			expected: 90 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.MCPConfig{
				Name:    "test",
				Type:    "stdio",
				Command: "test",
				Timeout: tt.timeout,
			}

			timeout := GetConnectionTimeout(cfg)
			assert.Equal(t, tt.expected, timeout)
		})
	}
}

func TestGetConnectionTimeout_EnvVar(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected time.Duration
	}{
		{
			name:     "env var in seconds",
			envValue: "60s",
			expected: 60 * time.Second,
		},
		{
			name:     "env var in minutes",
			envValue: "5m",
			expected: 5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env var
			os.Setenv(TimeoutEnvVar, tt.envValue)
			defer os.Unsetenv(TimeoutEnvVar)

			cfg := config.MCPConfig{
				Name:    "test",
				Type:    "stdio",
				Command: "test",
				// No per-MCP timeout
			}

			timeout := GetConnectionTimeout(cfg)
			assert.Equal(t, tt.expected, timeout)
		})
	}
}

func TestGetConnectionTimeout_Priority(t *testing.T) {
	// Test that per-MCP config takes precedence over env var
	t.Run("per-MCP beats env var", func(t *testing.T) {
		os.Setenv(TimeoutEnvVar, "60s")
		defer os.Unsetenv(TimeoutEnvVar)

		cfg := config.MCPConfig{
			Name:    "test",
			Type:    "stdio",
			Command: "test",
			Timeout: "45s", // Per-MCP should win
		}

		timeout := GetConnectionTimeout(cfg)
		assert.Equal(t, 45*time.Second, timeout)
	})

	// Test that env var takes precedence over default
	t.Run("env var beats default", func(t *testing.T) {
		os.Setenv(TimeoutEnvVar, "120s")
		defer os.Unsetenv(TimeoutEnvVar)

		cfg := config.MCPConfig{
			Name:    "test",
			Type:    "stdio",
			Command: "test",
			// No per-MCP timeout
		}

		timeout := GetConnectionTimeout(cfg)
		assert.Equal(t, 120*time.Second, timeout)
	})
}

func TestGetConnectionTimeout_InvalidValues(t *testing.T) {
	tests := []struct {
		name        string
		mcpTimeout  string
		envTimeout  string
		expected    time.Duration
		description string
	}{
		{
			name:        "invalid per-MCP falls back to env",
			mcpTimeout:  "invalid",
			envTimeout:  "60s",
			expected:    60 * time.Second,
			description: "invalid per-MCP timeout should fall back to env var",
		},
		{
			name:        "invalid per-MCP falls back to default when no env",
			mcpTimeout:  "invalid",
			envTimeout:  "",
			expected:    DefaultConnectionTimeout,
			description: "invalid per-MCP timeout should fall back to default when no env var",
		},
		{
			name:        "invalid env falls back to default",
			mcpTimeout:  "",
			envTimeout:  "invalid",
			expected:    DefaultConnectionTimeout,
			description: "invalid env var should fall back to default",
		},
		{
			name:        "zero timeout falls back to env",
			mcpTimeout:  "0s",
			envTimeout:  "60s",
			expected:    60 * time.Second,
			description: "zero per-MCP timeout should fall back to env var",
		},
		{
			name:        "negative timeout falls back to env",
			mcpTimeout:  "-30s",
			envTimeout:  "60s",
			expected:    60 * time.Second,
			description: "negative per-MCP timeout should fall back to env var",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envTimeout != "" {
				os.Setenv(TimeoutEnvVar, tt.envTimeout)
				defer os.Unsetenv(TimeoutEnvVar)
			} else {
				os.Unsetenv(TimeoutEnvVar)
			}

			cfg := config.MCPConfig{
				Name:    "test",
				Type:    "stdio",
				Command: "test",
				Timeout: tt.mcpTimeout,
			}

			timeout := GetConnectionTimeout(cfg)
			assert.Equal(t, tt.expected, timeout, tt.description)
		})
	}
}

func TestTimeoutEnvVarConstant(t *testing.T) {
	// Verify the env var name is correct
	assert.Equal(t, "SLOP_MCP_TIMEOUT", TimeoutEnvVar)
}

func TestDefaultConnectionTimeoutConstant(t *testing.T) {
	// Verify the default timeout is 30 seconds
	assert.Equal(t, 30*time.Second, DefaultConnectionTimeout)
}
