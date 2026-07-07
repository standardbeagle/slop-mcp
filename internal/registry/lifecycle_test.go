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
