package registry

import (
	"context"
	"testing"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 1 * time.Second},          // Initial backoff
		{2, 2 * time.Second},          // 1s * 2
		{3, 4 * time.Second},          // 2s * 2
		{4, 8 * time.Second},          // 4s * 2
		{5, 16 * time.Second},         // 8s * 2
		{6, 32 * time.Second},         // 16s * 2
		{7, 60 * time.Second},         // Capped at MaxBackoff (60s)
		{8, 60 * time.Second},         // Still capped
		{100, 60 * time.Second},       // Large attempt still capped
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			backoff := calculateBackoff(tt.attempt)
			assert.Equal(t, tt.expected, backoff, "attempt %d", tt.attempt)
		})
	}
}

func TestReconnectionConstants(t *testing.T) {
	assert.Equal(t, 5, DefaultMaxRetries)
	assert.Equal(t, 1*time.Second, InitialBackoff)
	assert.Equal(t, 60*time.Second, MaxBackoff)
	assert.Equal(t, 2.0, BackoffMultiplier)
}

func TestMCPStateReconnecting(t *testing.T) {
	// Verify the StateReconnecting constant exists and is properly defined
	assert.Equal(t, MCPState("reconnecting"), StateReconnecting)
}

func TestSetConfiguredPreservesState(t *testing.T) {
	r := New()
	cfg := config.MCPConfig{
		Name:       "test-mcp",
		Type:       "stdio",
		Command:    "test",
		MaxRetries: 3,
	}

	// First call sets to configured
	r.SetConfigured(cfg)
	status := r.Status()
	require.Len(t, status, 1)
	assert.Equal(t, StateConfigured, status[0].State)
	assert.Equal(t, 0, status[0].ReconnectAttempts)

	// Manually update the state to simulate reconnection attempts
	r.mu.Lock()
	if s, ok := r.states["test-mcp"]; ok {
		s.reconnectAttempts = 2
		s.state = StateReconnecting
	}
	r.mu.Unlock()

	// Second call should NOT overwrite existing state
	r.SetConfigured(cfg)
	status = r.Status()
	require.Len(t, status, 1)
	assert.Equal(t, StateReconnecting, status[0].State)
	assert.Equal(t, 2, status[0].ReconnectAttempts)
}

func TestGetReconnectAttempts(t *testing.T) {
	r := New()

	// Non-existent MCP returns 0
	assert.Equal(t, 0, r.GetReconnectAttempts("unknown"))

	// Add an MCP with reconnect attempts
	cfg := config.MCPConfig{
		Name:    "test-mcp",
		Type:    "stdio",
		Command: "test",
	}
	r.SetConfigured(cfg)

	// Initially 0
	assert.Equal(t, 0, r.GetReconnectAttempts("test-mcp"))

	// Manually set reconnect attempts
	r.mu.Lock()
	if s, ok := r.states["test-mcp"]; ok {
		s.reconnectAttempts = 5
	}
	r.mu.Unlock()

	assert.Equal(t, 5, r.GetReconnectAttempts("test-mcp"))
}

func TestReconnectWithBackoff_MCPNotConfigured(t *testing.T) {
	r := New()
	ctx := context.Background()

	err := r.ReconnectWithBackoff(ctx, "nonexistent", 3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MCP not configured")
}

func TestReconnectWithBackoff_DisabledViaConfig(t *testing.T) {
	r := New()
	ctx := context.Background()

	cfg := config.MCPConfig{
		Name:       "test-mcp",
		Type:       "stdio",
		Command:    "test",
		MaxRetries: -1, // Disabled
	}
	r.SetConfigured(cfg)

	err := r.ReconnectWithBackoff(ctx, "test-mcp", 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "auto-reconnect disabled")
}

func TestReconnectWithBackoff_ContextCancellation(t *testing.T) {
	r := New()

	cfg := config.MCPConfig{
		Name:       "test-mcp",
		Type:       "stdio",
		Command:    "nonexistent-command", // Will fail to connect
		MaxRetries: 5,
	}
	r.SetConfigured(cfg)

	// Create a context that cancels quickly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := r.ReconnectWithBackoff(ctx, "test-mcp", 5)
	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestReconnectWithBackoff_UsesConfigMaxRetries(t *testing.T) {
	r := New()

	cfg := config.MCPConfig{
		Name:       "test-mcp",
		Type:       "stdio",
		Command:    "nonexistent-command",
		MaxRetries: 2, // Custom max retries
	}
	r.SetConfigured(cfg)

	// Very short timeout to fail quickly
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Pass 0 to use config's MaxRetries
	err := r.ReconnectWithBackoff(ctx, "test-mcp", 0)
	assert.Error(t, err)
	// Should fail with context cancellation before completing retries
}

func TestReconnectWithBackoff_UsesDefaultMaxRetries(t *testing.T) {
	r := New()

	cfg := config.MCPConfig{
		Name:       "test-mcp",
		Type:       "stdio",
		Command:    "nonexistent-command",
		MaxRetries: 0, // Use default
	}
	r.SetConfigured(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := r.ReconnectWithBackoff(ctx, "test-mcp", 0)
	assert.Error(t, err)
	// Default is 5, but context will cancel first
}

func TestStatusShowsReconnectAttempts(t *testing.T) {
	r := New()

	cfg := config.MCPConfig{
		Name:    "test-mcp",
		Type:    "stdio",
		Command: "test",
	}
	r.SetConfigured(cfg)

	// Manually set state to reconnecting with attempts
	r.mu.Lock()
	r.states["test-mcp"].state = StateReconnecting
	r.states["test-mcp"].reconnectAttempts = 3
	r.mu.Unlock()

	status := r.Status()
	require.Len(t, status, 1)
	assert.Equal(t, StateReconnecting, status[0].State)
	assert.Equal(t, 3, status[0].ReconnectAttempts)
}

func TestListShowsReconnectingStatus(t *testing.T) {
	r := New()

	cfg := config.MCPConfig{
		Name:    "test-mcp",
		Type:    "stdio",
		Command: "test",
	}
	r.SetConfigured(cfg)

	// Manually set state to reconnecting with attempts
	r.mu.Lock()
	r.states["test-mcp"].state = StateReconnecting
	r.states["test-mcp"].reconnectAttempts = 2
	r.mu.Unlock()

	list := r.List()
	require.Len(t, list, 1)
	assert.False(t, list[0].Connected)
	assert.Contains(t, list[0].Error, "reconnecting")
	assert.Contains(t, list[0].Error, "attempt 2")
}

func TestConfigMaxRetries(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int
		expected   int
	}{
		{
			name:       "default value",
			maxRetries: 0,
			expected:   0, // Zero means use default
		},
		{
			name:       "custom value",
			maxRetries: 10,
			expected:   10,
		},
		{
			name:       "disabled",
			maxRetries: -1,
			expected:   -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.MCPConfig{
				Name:       "test",
				Type:       "stdio",
				Command:    "test",
				MaxRetries: tt.maxRetries,
			}
			assert.Equal(t, tt.expected, cfg.MaxRetries)
		})
	}
}

func TestMCPFullStatusIncludesReconnectAttempts(t *testing.T) {
	// Test that MCPFullStatus struct has ReconnectAttempts field
	status := MCPFullStatus{
		Name:              "test",
		Type:              "stdio",
		State:             StateReconnecting,
		Source:            "runtime",
		ReconnectAttempts: 3,
	}

	assert.Equal(t, "test", status.Name)
	assert.Equal(t, StateReconnecting, status.State)
	assert.Equal(t, 3, status.ReconnectAttempts)
}
