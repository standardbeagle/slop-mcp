package registry

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/cache"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRegistryWithCache returns a registry backed by a temp-dir cache store.
func newTestRegistryWithCache(t *testing.T) (*Registry, *cache.Store) {
	t.Helper()
	store := cache.NewStoreWithPath(filepath.Join(t.TempDir(), "tools.json"))
	return NewWithCache(logging.Default(), store), store
}

// installFakeConnection installs a connection/state pair directly, bypassing
// the network connect flow (session stays nil; updateCache tolerates that).
func installFakeConnection(r *Registry, cfg config.MCPConfig, toolsIndexed bool) {
	r.mu.Lock()
	r.connections[cfg.Name] = &mcpConnection{config: cfg}
	r.states[cfg.Name] = &mcpState{
		config:       cfg,
		state:        StateConnected,
		toolsIndexed: toolsIndexed,
	}
	r.mu.Unlock()
}

func TestUpdateCache_SkipsWhenToolsNotIndexed(t *testing.T) {
	r, store := newTestRegistryWithCache(t)
	cfg := config.MCPConfig{Name: "m", Type: "stdio", Command: "x"}
	installFakeConnection(r, cfg, false)

	r.updateCache("m")

	entry, err := store.GetEntry("m")
	require.NoError(t, err)
	assert.Nil(t, entry, "cache entry must not be written when tool indexing failed")
}

func TestUpdateCache_SetsCachedAt(t *testing.T) {
	r, store := newTestRegistryWithCache(t)
	cfg := config.MCPConfig{Name: "m", Type: "stdio", Command: "x"}
	installFakeConnection(r, cfg, true)
	r.AddToolsForTesting("m", []ToolInfo{{Name: "t1", Description: "d", MCPName: "m"}})

	before := time.Now()
	r.updateCache("m")

	entry, err := store.GetEntry("m")
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.False(t, entry.CachedAt.IsZero(), "CachedAt must be stamped")
	assert.False(t, entry.CachedAt.Before(before.Add(-time.Second)))
	require.Len(t, entry.Tools, 1)
	assert.Equal(t, "t1", entry.Tools[0].Name)
}

// TestEnsureConnected_RaceWithStateMutation exercises concurrent lazy connects
// against in-place mcpState mutations (health checks, status reads). Run with
// -race: it guards against field reads on shared *mcpState after RUnlock.
func TestEnsureConnected_RaceWithStateMutation(t *testing.T) {
	r, _ := newTestRegistryWithCache(t)
	cfg := config.MCPConfig{
		Name:    "race-mcp",
		Type:    "stdio",
		Command: "/nonexistent-slop-mcp-test-binary",
		Timeout: "1s",
	}
	r.SetConfigured(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.EnsureConnected(context.Background(), "race-mcp")
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			r.updateHealthState("race-mcp", HealthStatusUnhealthy, time.Now(), nil)
			_ = r.Status()
			_ = r.GetState("race-mcp")
		}
	}()
	wg.Wait()

	// Connect must have failed (bogus binary) and recorded an error state.
	assert.Equal(t, StateError, r.GetState("race-mcp"))
}

// TestEnsureConnected_WaitsForInFlightConnect verifies that a caller hitting
// StateConnecting waits on the per-MCP connect mutex for the in-flight attempt
// instead of failing immediately with a "cannot connect" error.
func TestEnsureConnected_WaitsForInFlightConnect(t *testing.T) {
	r, _ := newTestRegistryWithCache(t)
	cfg := config.MCPConfig{Name: "inflight", Type: "stdio", Command: "x"}
	r.SetConfigured(cfg)

	// Simulate an in-flight connect exactly as connectLocked does: hold the
	// per-MCP connect semaphore and set StateConnecting.
	sem := r.getConnectMu("inflight")
	sem <- struct{}{}
	r.mu.Lock()
	r.states["inflight"].state = StateConnecting
	r.mu.Unlock()

	done := make(chan error, 1)
	go func() { done <- r.EnsureConnected(context.Background(), "inflight") }()

	// Must not return while the connect is in flight.
	select {
	case err := <-done:
		t.Fatalf("EnsureConnected returned during in-flight connect: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	// Finish the in-flight connect successfully, then release the mutex.
	r.mu.Lock()
	r.states["inflight"].state = StateConnected
	r.connections["inflight"] = &mcpConnection{config: cfg}
	r.mu.Unlock()
	<-sem

	select {
	case err := <-done:
		assert.NoError(t, err, "waiter must observe the finished connect")
	case <-time.After(2 * time.Second):
		t.Fatal("EnsureConnected did not return after in-flight connect finished")
	}
}

// TestEnsureConnected_ErrorStateBackoff verifies that StateError fails fast
// within ErrorRetryInterval and permits one fresh dial after the window.
func TestEnsureConnected_ErrorStateBackoff(t *testing.T) {
	r, _ := newTestRegistryWithCache(t)
	cfg := config.MCPConfig{
		Name:    "err-mcp",
		Type:    "stdio",
		Command: "/nonexistent-slop-mcp-test-binary",
		Timeout: "1s",
	}
	r.SetConfigured(cfg)
	ctx := context.Background()

	// First lazy connect dials and fails, recording lastFailure.
	err := r.EnsureConnected(ctx, "err-mcp")
	require.Error(t, err)
	require.Equal(t, StateError, r.GetState("err-mcp"))

	// Within the backoff window: fail fast with the stored error, no re-dial.
	err = r.EnsureConnected(ctx, "err-mcp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error state")

	// Age the failure past the window: one retry attempt is allowed (it dials,
	// fails again, and re-arms the window).
	r.mu.Lock()
	r.states["err-mcp"].lastFailure = time.Now().Add(-ErrorRetryInterval - time.Second)
	r.mu.Unlock()

	err = r.EnsureConnected(ctx, "err-mcp")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "error state", "elapsed window must trigger a real dial, not fail-fast")

	// Fresh failure re-armed the window.
	err = r.EnsureConnected(ctx, "err-mcp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error state")
}

// TestExecuteToolRaw_ConcurrentFirstUse hammers the lazy-connect tool-call path
// from many goroutines (run with -race). Exactly one dial happens; the rest
// fail fast on the recorded error state without racing map iteration.
func TestExecuteToolRaw_ConcurrentFirstUse(t *testing.T) {
	r, _ := newTestRegistryWithCache(t)
	cfg := config.MCPConfig{
		Name:    "first-use",
		Type:    "stdio",
		Command: "/nonexistent-slop-mcp-test-binary",
		Timeout: "1s",
	}
	r.SetConfigured(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.ExecuteToolRaw(context.Background(), "first-use", "ping", nil)
			assert.Error(t, err)
		}()
	}
	// Concurrent state mutation and listNames consumers.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = r.Status()
			_, _ = r.ExecuteToolRaw(context.Background(), "no-such-mcp", "x", nil) // exercises listNames
		}
	}()
	wg.Wait()

	assert.Equal(t, StateError, r.GetState("first-use"))
}

// TestReconnectWithBackoff_ConcurrentSerialized verifies that concurrent
// reconnect loops are serialized by the per-MCP connect mutex, so attempt
// counts accumulate deterministically instead of interleaving.
func TestReconnectWithBackoff_ConcurrentSerialized(t *testing.T) {
	r, _ := newTestRegistryWithCache(t)
	cfg := config.MCPConfig{
		Name:    "backoff-mcp",
		Type:    "stdio",
		Command: "/nonexistent-slop-mcp-test-binary",
		Timeout: "1s",
	}
	r.SetConfigured(cfg)

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := r.ReconnectWithBackoff(ctx, "backoff-mcp", 1)
			assert.Error(t, err)
		}()
	}
	wg.Wait()

	// Serialized: second loop observes the first loop's attempt count (1)
	// and adds its own single attempt.
	assert.Equal(t, 2, r.GetReconnectAttempts("backoff-mcp"))
	assert.Equal(t, StateError, r.GetState("backoff-mcp"))
}

// TestEnsureConnected_CancelableWhileWaiting verifies that a caller blocked
// behind a long-held lifecycle semaphore (e.g. a reconnect backoff loop)
// returns when its context is cancelled instead of hanging.
func TestEnsureConnected_CancelableWhileWaiting(t *testing.T) {
	r, _ := newTestRegistryWithCache(t)
	cfg := config.MCPConfig{Name: "held", Type: "stdio", Command: "x"}
	r.SetConfigured(cfg)

	sem := r.getConnectMu("held")
	sem <- struct{}{} // simulate a long-running lifecycle holder
	defer func() { <-sem }()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.EnsureConnected(ctx, "held") }()

	select {
	case err := <-done:
		t.Fatalf("EnsureConnected returned while semaphore held: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("EnsureConnected did not honor context cancellation")
	}
}
