package registry

import (
	"context"
	"testing"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthStatusConstants(t *testing.T) {
	assert.Equal(t, HealthStatus("unknown"), HealthStatusUnknown)
	assert.Equal(t, HealthStatus("healthy"), HealthStatusHealthy)
	assert.Equal(t, HealthStatus("unhealthy"), HealthStatusUnhealthy)
}

func TestHealthCheckConstants(t *testing.T) {
	assert.Equal(t, 5*time.Second, DefaultHealthCheckTimeout)
	assert.Equal(t, time.Duration(0), time.Duration(DefaultHealthCheckInterval))
}

func TestHealthCheckResult(t *testing.T) {
	result := HealthCheckResult{
		Name:         "test-mcp",
		Status:       HealthStatusHealthy,
		ResponseTime: "1.5ms",
		CheckedAt:    time.Now(),
	}

	assert.Equal(t, "test-mcp", result.Name)
	assert.Equal(t, HealthStatusHealthy, result.Status)
	assert.Equal(t, "1.5ms", result.ResponseTime)
	assert.Empty(t, result.Error)
}

func TestHealthCheckResultUnhealthy(t *testing.T) {
	result := HealthCheckResult{
		Name:      "test-mcp",
		Status:    HealthStatusUnhealthy,
		Error:     "connection refused",
		CheckedAt: time.Now(),
	}

	assert.Equal(t, HealthStatusUnhealthy, result.Status)
	assert.Equal(t, "connection refused", result.Error)
}

func TestMCPFullStatusWithHealthInfo(t *testing.T) {
	status := MCPFullStatus{
		Name:            "test-mcp",
		Type:            "stdio",
		State:           StateConnected,
		ToolCount:       5,
		Source:          "runtime",
		HealthStatus:    HealthStatusHealthy,
		LastHealthCheck: time.Now().Format(time.RFC3339),
	}

	assert.Equal(t, HealthStatusHealthy, status.HealthStatus)
	assert.NotEmpty(t, status.LastHealthCheck)
	assert.Empty(t, status.HealthError)
}

func TestMCPFullStatusWithHealthError(t *testing.T) {
	status := MCPFullStatus{
		Name:            "test-mcp",
		Type:            "sse",
		State:           StateConnected,
		HealthStatus:    HealthStatusUnhealthy,
		LastHealthCheck: time.Now().Format(time.RFC3339),
		HealthError:     "timeout",
	}

	assert.Equal(t, HealthStatusUnhealthy, status.HealthStatus)
	assert.Equal(t, "timeout", status.HealthError)
}

func TestMcpStateHealthFields(t *testing.T) {
	state := mcpState{
		config: config.MCPConfig{
			Name: "test-mcp",
			Type: "stdio",
		},
		state:           StateConnected,
		healthStatus:    HealthStatusHealthy,
		lastHealthCheck: time.Now(),
		healthError:     nil,
	}

	assert.Equal(t, HealthStatusHealthy, state.healthStatus)
	assert.False(t, state.lastHealthCheck.IsZero())
	assert.Nil(t, state.healthError)
}

func TestRegistryGetHealthStatus_NotConfigured(t *testing.T) {
	r := New()

	status, lastCheck, err := r.GetHealthStatus("nonexistent")
	assert.Equal(t, HealthStatusUnknown, status)
	assert.True(t, lastCheck.IsZero())
	assert.Nil(t, err)
}

func TestRegistryGetHealthStatus_ConfiguredNotChecked(t *testing.T) {
	r := New()
	cfg := config.MCPConfig{
		Name:    "test-mcp",
		Type:    "stdio",
		Command: "test",
	}
	r.SetConfigured(cfg)

	status, lastCheck, err := r.GetHealthStatus("test-mcp")
	// No healthStatus set yet means empty string (returned as-is, Status() method defaults it to Unknown)
	assert.Equal(t, HealthStatus(""), status)
	assert.True(t, lastCheck.IsZero())
	assert.Nil(t, err)
}

func TestRegistryUpdateHealthState(t *testing.T) {
	r := New()
	cfg := config.MCPConfig{
		Name:    "test-mcp",
		Type:    "stdio",
		Command: "test",
	}
	r.SetConfigured(cfg)

	// Update health state
	checkTime := time.Now()
	r.updateHealthState("test-mcp", HealthStatusHealthy, checkTime, nil)

	status, lastCheck, err := r.GetHealthStatus("test-mcp")
	assert.Equal(t, HealthStatusHealthy, status)
	assert.Equal(t, checkTime.Unix(), lastCheck.Unix())
	assert.Nil(t, err)
}

func TestRegistryUpdateHealthState_Unhealthy(t *testing.T) {
	r := New()
	cfg := config.MCPConfig{
		Name:    "test-mcp",
		Type:    "stdio",
		Command: "test",
	}
	r.SetConfigured(cfg)

	// Update health state with error
	checkTime := time.Now()
	testErr := assert.AnError
	r.updateHealthState("test-mcp", HealthStatusUnhealthy, checkTime, testErr)

	status, lastCheck, err := r.GetHealthStatus("test-mcp")
	assert.Equal(t, HealthStatusUnhealthy, status)
	assert.Equal(t, checkTime.Unix(), lastCheck.Unix())
	assert.Equal(t, testErr, err)
}

func TestRegistryUpdateHealthState_NonexistentMCP(t *testing.T) {
	r := New()

	// Should not panic when updating nonexistent MCP
	r.updateHealthState("nonexistent", HealthStatusHealthy, time.Now(), nil)

	status, _, _ := r.GetHealthStatus("nonexistent")
	assert.Equal(t, HealthStatusUnknown, status)
}

func TestRegistryStatusIncludesHealthInfo(t *testing.T) {
	r := New()
	cfg := config.MCPConfig{
		Name:    "test-mcp",
		Type:    "stdio",
		Command: "test",
	}
	r.SetConfigured(cfg)

	// Update health state
	checkTime := time.Now()
	r.updateHealthState("test-mcp", HealthStatusHealthy, checkTime, nil)

	status := r.Status()
	require.Len(t, status, 1)
	assert.Equal(t, HealthStatusHealthy, status[0].HealthStatus)
	assert.NotEmpty(t, status[0].LastHealthCheck)
	assert.Empty(t, status[0].HealthError)
}

func TestRegistryStatusIncludesHealthError(t *testing.T) {
	r := New()
	cfg := config.MCPConfig{
		Name:    "test-mcp",
		Type:    "stdio",
		Command: "test",
	}
	r.SetConfigured(cfg)

	// Update health state with error
	r.mu.Lock()
	r.states["test-mcp"].healthStatus = HealthStatusUnhealthy
	r.states["test-mcp"].lastHealthCheck = time.Now()
	r.states["test-mcp"].healthError = assert.AnError
	r.mu.Unlock()

	status := r.Status()
	require.Len(t, status, 1)
	assert.Equal(t, HealthStatusUnhealthy, status[0].HealthStatus)
	assert.NotEmpty(t, status[0].LastHealthCheck)
	assert.NotEmpty(t, status[0].HealthError)
}

func TestRegistryStatusDefaultsToUnknown(t *testing.T) {
	r := New()
	cfg := config.MCPConfig{
		Name:    "test-mcp",
		Type:    "stdio",
		Command: "test",
	}
	r.SetConfigured(cfg)

	// No health check performed yet
	status := r.Status()
	require.Len(t, status, 1)
	assert.Equal(t, HealthStatusUnknown, status[0].HealthStatus)
	assert.Empty(t, status[0].LastHealthCheck)
}

func TestRegistryHealthCheck_NoConnectedMCPs(t *testing.T) {
	r := New()
	ctx := context.Background()

	// No MCPs connected
	results := r.HealthCheck(ctx, "")
	assert.Nil(t, results)
}

func TestRegistryHealthCheck_SpecificMCPNotConnected(t *testing.T) {
	r := New()
	ctx := context.Background()

	cfg := config.MCPConfig{
		Name:    "test-mcp",
		Type:    "stdio",
		Command: "test",
	}
	r.SetConfigured(cfg)

	// MCP configured but not connected
	results := r.HealthCheck(ctx, "test-mcp")
	assert.Nil(t, results) // Not in connections map
}

func TestRegistryHealthCheck_ContextCancellation(t *testing.T) {
	r := New()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should return immediately without hanging
	results := r.HealthCheck(ctx, "")
	assert.Nil(t, results)
}

func TestStartBackgroundHealthCheck_Disabled(t *testing.T) {
	r := New()

	// Empty string disables
	err := r.StartBackgroundHealthCheck("")
	assert.NoError(t, err)
	assert.Nil(t, r.healthCheckCancel)

	// "0" disables
	err = r.StartBackgroundHealthCheck("0")
	assert.NoError(t, err)
	assert.Nil(t, r.healthCheckCancel)
}

func TestStartBackgroundHealthCheck_InvalidInterval(t *testing.T) {
	r := New()

	err := r.StartBackgroundHealthCheck("invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid health check interval")
}

func TestStartBackgroundHealthCheck_NegativeInterval(t *testing.T) {
	r := New()

	err := r.StartBackgroundHealthCheck("-5s")
	assert.NoError(t, err) // Treated as disabled
	assert.Nil(t, r.healthCheckCancel)
}

func TestStartBackgroundHealthCheck_ValidInterval(t *testing.T) {
	r := New()

	err := r.StartBackgroundHealthCheck("100ms")
	assert.NoError(t, err)

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	r.mu.Lock()
	hasCancel := r.healthCheckCancel != nil
	r.mu.Unlock()
	assert.True(t, hasCancel)

	// Clean up
	r.StopBackgroundHealthCheck()
}

func TestStopBackgroundHealthCheck(t *testing.T) {
	r := New()

	// Start background health check
	err := r.StartBackgroundHealthCheck("100ms")
	require.NoError(t, err)

	// Stop it
	r.StopBackgroundHealthCheck()

	r.mu.Lock()
	hasCancel := r.healthCheckCancel != nil
	r.mu.Unlock()
	assert.False(t, hasCancel)
}

func TestStopBackgroundHealthCheck_NotStarted(t *testing.T) {
	r := New()

	// Should not panic when stopping without starting
	r.StopBackgroundHealthCheck()
	assert.Nil(t, r.healthCheckCancel)
}

func TestStartBackgroundHealthCheck_ReplacesExisting(t *testing.T) {
	r := New()

	// Start first background health check
	err := r.StartBackgroundHealthCheck("100ms")
	require.NoError(t, err)

	// Give it a moment
	time.Sleep(50 * time.Millisecond)

	r.mu.Lock()
	firstCancel := r.healthCheckCancel
	r.mu.Unlock()
	require.NotNil(t, firstCancel)

	// Start second one - should replace first
	err = r.StartBackgroundHealthCheck("200ms")
	require.NoError(t, err)

	r.mu.Lock()
	secondCancel := r.healthCheckCancel
	r.mu.Unlock()
	require.NotNil(t, secondCancel)

	// Cancel functions should be different (first was replaced)
	// Note: We can't directly compare cancel functions, but we can verify the second one works
	r.StopBackgroundHealthCheck()

	r.mu.Lock()
	hasCancel := r.healthCheckCancel != nil
	r.mu.Unlock()
	assert.False(t, hasCancel)
}

func TestRegistryCloseStopsBackgroundHealthCheck(t *testing.T) {
	r := New()

	// Start background health check
	err := r.StartBackgroundHealthCheck("100ms")
	require.NoError(t, err)

	// Close registry
	err = r.Close()
	assert.NoError(t, err)

	r.mu.Lock()
	hasCancel := r.healthCheckCancel != nil
	r.mu.Unlock()
	assert.False(t, hasCancel)
}

func TestConfigHealthCheckInterval(t *testing.T) {
	cfg := config.MCPConfig{
		Name:                "test",
		Type:                "stdio",
		Command:             "test",
		HealthCheckInterval: "30s",
	}

	assert.Equal(t, "30s", cfg.HealthCheckInterval)
}

func TestHealthCheckTimeout(t *testing.T) {
	// Verify the default timeout is 5 seconds as per acceptance criteria
	assert.Equal(t, 5*time.Second, DefaultHealthCheckTimeout)
}
