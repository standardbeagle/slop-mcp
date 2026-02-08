package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigHash_Deterministic(t *testing.T) {
	cfg := config.MCPConfig{
		Type:    "stdio",
		Command: "npx",
		Args:    []string{"-y", "@anthropic/mcp-filesystem"},
		Env:     map[string]string{"KEY": "val"},
	}

	hash1 := ConfigHash(cfg)
	hash2 := ConfigHash(cfg)
	assert.Equal(t, hash1, hash2, "same config should produce same hash")
	assert.Len(t, hash1, 16, "hash should be 16 hex chars")
}

func TestConfigHash_ExcludesVolatileFields(t *testing.T) {
	base := config.MCPConfig{
		Type:    "stdio",
		Command: "server",
	}

	// Changing Name, Timeout, MaxRetries, Source, Dynamic should NOT change hash
	withVolatile := config.MCPConfig{
		Name:       "different-name",
		Type:       "stdio",
		Command:    "server",
		Timeout:    "60s",
		MaxRetries: 10,
		Dynamic:    true,
		Source:     config.SourceUser,
	}

	assert.Equal(t, ConfigHash(base), ConfigHash(withVolatile),
		"volatile fields should not affect hash")
}

func TestConfigHash_IdentityFieldsChange(t *testing.T) {
	base := config.MCPConfig{
		Type:    "stdio",
		Command: "server",
	}

	tests := []struct {
		name string
		cfg  config.MCPConfig
	}{
		{
			name: "different type",
			cfg:  config.MCPConfig{Type: "sse", Command: "server"},
		},
		{
			name: "different command",
			cfg:  config.MCPConfig{Type: "stdio", Command: "other-server"},
		},
		{
			name: "different args",
			cfg:  config.MCPConfig{Type: "stdio", Command: "server", Args: []string{"--flag"}},
		},
		{
			name: "different URL",
			cfg:  config.MCPConfig{Type: "stdio", Command: "server", URL: "http://localhost"},
		},
		{
			name: "different env",
			cfg:  config.MCPConfig{Type: "stdio", Command: "server", Env: map[string]string{"K": "V"}},
		},
		{
			name: "different headers",
			cfg:  config.MCPConfig{Type: "stdio", Command: "server", Headers: map[string]string{"Auth": "Bearer x"}},
		},
	}

	baseHash := ConfigHash(base)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEqual(t, baseHash, ConfigHash(tt.cfg),
				"changing identity field should change hash")
		})
	}
}

func TestConfigHash_MapOrderIndependent(t *testing.T) {
	cfg1 := config.MCPConfig{
		Type:    "stdio",
		Command: "server",
		Env:     map[string]string{"A": "1", "B": "2", "C": "3"},
	}
	cfg2 := config.MCPConfig{
		Type:    "stdio",
		Command: "server",
		Env:     map[string]string{"C": "3", "A": "1", "B": "2"},
	}

	assert.Equal(t, ConfigHash(cfg1), ConfigHash(cfg2),
		"map order should not affect hash")
}

func TestStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache", "tools.json")
	store := NewStoreWithPath(path)

	tools := []CachedToolInfo{
		{Name: "read_file", Description: "Read a file", MCPName: "filesystem"},
		{Name: "write_file", Description: "Write a file", MCPName: "filesystem"},
	}

	entry := &CacheEntry{
		ConfigHash:    "abc123def456abcd",
		ServerName:    "filesystem",
		ServerVersion: "1.0.0",
		Tools:         tools,
	}

	// Save
	err := store.SetEntry("filesystem", entry)
	require.NoError(t, err)

	// Load
	loaded, err := store.GetEntry("filesystem")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, entry.ConfigHash, loaded.ConfigHash)
	assert.Equal(t, entry.ServerName, loaded.ServerName)
	assert.Equal(t, entry.ServerVersion, loaded.ServerVersion)
	assert.Len(t, loaded.Tools, 2)
	assert.Equal(t, "read_file", loaded.Tools[0].Name)
}

func TestStore_LoadMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent", "tools.json")
	store := NewStoreWithPath(path)

	cf, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, CacheSchemaVersion, cf.Version)
	assert.Empty(t, cf.Entries)
}

func TestStore_LoadCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")

	// Write corrupt JSON
	err := os.WriteFile(path, []byte("not valid json{{{"), 0644)
	require.NoError(t, err)

	store := NewStoreWithPath(path)
	cf, err := store.Load()
	require.NoError(t, err, "corrupt cache should return empty, not error")
	assert.Empty(t, cf.Entries)
}

func TestStore_LoadVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")

	// Write cache with different version
	data := `{"version": 999, "entries": {"test": {"config_hash": "abc"}}}`
	err := os.WriteFile(path, []byte(data), 0644)
	require.NoError(t, err)

	store := NewStoreWithPath(path)
	cf, err := store.Load()
	require.NoError(t, err, "version mismatch should return empty, not error")
	assert.Empty(t, cf.Entries)
}

func TestStore_IsValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")
	store := NewStoreWithPath(path)

	cfg := config.MCPConfig{
		Type:    "stdio",
		Command: "server",
	}

	// Set entry with matching hash
	entry := &CacheEntry{
		ConfigHash: ConfigHash(cfg),
	}
	err := store.SetEntry("test", entry)
	require.NoError(t, err)

	// Valid: hash matches
	assert.True(t, store.IsValid("test", cfg))

	// Invalid: config changed
	cfgChanged := config.MCPConfig{
		Type:    "stdio",
		Command: "other-server",
	}
	assert.False(t, store.IsValid("test", cfgChanged))

	// Invalid: no entry
	assert.False(t, store.IsValid("nonexistent", cfg))
}

func TestStore_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")
	store := NewStoreWithPath(path)

	// Save first entry
	err := store.SetEntry("first", &CacheEntry{ConfigHash: "aaa"})
	require.NoError(t, err)

	// Verify file exists and no temp file remains
	_, err = os.Stat(path)
	require.NoError(t, err, "cache file should exist")

	_, err = os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(err), "temp file should be cleaned up")

	// Save second entry (should preserve first)
	err = store.SetEntry("second", &CacheEntry{ConfigHash: "bbb"})
	require.NoError(t, err)

	cf, err := store.Load()
	require.NoError(t, err)
	assert.Len(t, cf.Entries, 2)
	assert.Equal(t, "aaa", cf.Entries["first"].ConfigHash)
	assert.Equal(t, "bbb", cf.Entries["second"].ConfigHash)
}

func TestStore_MultipleEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")
	store := NewStoreWithPath(path)

	// Set multiple entries
	for _, name := range []string{"mcp1", "mcp2", "mcp3"} {
		err := store.SetEntry(name, &CacheEntry{
			ConfigHash:    ConfigHash(config.MCPConfig{Command: name}),
			ServerName:    name,
			ServerVersion: "1.0.0",
			Tools: []CachedToolInfo{
				{Name: name + "_tool", MCPName: name},
			},
		})
		require.NoError(t, err)
	}

	// Verify all entries exist
	cf, err := store.Load()
	require.NoError(t, err)
	assert.Len(t, cf.Entries, 3)

	for _, name := range []string{"mcp1", "mcp2", "mcp3"} {
		entry, ok := cf.Entries[name]
		require.True(t, ok, "entry %s should exist", name)
		assert.Equal(t, name, entry.ServerName)
		assert.Len(t, entry.Tools, 1)
	}
}
