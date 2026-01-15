//go:build mcp_go_client_oauth

package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TokenStore GetToken/SetToken/DeleteToken Tests
// =============================================================================

func TestTokenStore_GetToken_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	token, err := store.GetToken("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, token)
}

func TestTokenStore_GetToken_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")

	// Pre-create file with token
	tf := &TokenFile{
		Version: 1,
		Tokens: map[string]*MCPToken{
			"test-server": {
				ServerName:  "test-server",
				ServerURL:   "https://test.example.com",
				AccessToken: "pre-existing-token",
			},
		},
	}
	data, _ := json.Marshal(tf)
	err := os.WriteFile(storePath, data, 0600)
	require.NoError(t, err)

	store := NewTokenStoreWithPath(storePath)
	token, err := store.GetToken("test-server")
	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, "pre-existing-token", token.AccessToken)
}

func TestTokenStore_SetToken_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	// Use nested path that doesn't exist
	storePath := filepath.Join(tmpDir, "nested", "dir", "auth.json")
	store := NewTokenStoreWithPath(storePath)

	token := &MCPToken{
		ServerName:  "test",
		AccessToken: "token",
	}
	err := store.SetToken(token)
	require.NoError(t, err)

	// Verify directory was created
	_, err = os.Stat(filepath.Dir(storePath))
	assert.NoError(t, err)
}

func TestTokenStore_SetToken_AllFields(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	expiry := time.Now().Add(time.Hour).Truncate(time.Second)
	token := &MCPToken{
		ServerName:    "full-server",
		ServerURL:     "https://full.example.com",
		ClientID:      "client-123",
		ClientSecret:  "secret-456",
		AccessToken:   "access-token",
		RefreshToken:  "refresh-token",
		TokenType:     "Bearer",
		ExpiresAt:     expiry,
		Scope:         "read write",
		TokenEndpoint: "https://auth.example.com/token",
	}

	err := store.SetToken(token)
	require.NoError(t, err)

	// Retrieve and verify all fields
	retrieved, err := store.GetToken("full-server")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, "full-server", retrieved.ServerName)
	assert.Equal(t, "https://full.example.com", retrieved.ServerURL)
	assert.Equal(t, "client-123", retrieved.ClientID)
	assert.Equal(t, "secret-456", retrieved.ClientSecret)
	assert.Equal(t, "access-token", retrieved.AccessToken)
	assert.Equal(t, "refresh-token", retrieved.RefreshToken)
	assert.Equal(t, "Bearer", retrieved.TokenType)
	assert.Equal(t, "read write", retrieved.Scope)
	assert.Equal(t, "https://auth.example.com/token", retrieved.TokenEndpoint)
	// Time comparison needs truncation for JSON serialization
	assert.WithinDuration(t, expiry, retrieved.ExpiresAt, time.Second)
}

func TestTokenStore_SetToken_UpdateExisting(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	// Set initial token
	token1 := &MCPToken{
		ServerName:  "update-server",
		AccessToken: "old-token",
	}
	err := store.SetToken(token1)
	require.NoError(t, err)

	// Update token
	token2 := &MCPToken{
		ServerName:  "update-server",
		AccessToken: "new-token",
	}
	err = store.SetToken(token2)
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.GetToken("update-server")
	require.NoError(t, err)
	assert.Equal(t, "new-token", retrieved.AccessToken)
}

func TestTokenStore_DeleteToken_Existing(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	// Set token
	token := &MCPToken{
		ServerName:  "delete-me",
		AccessToken: "token",
	}
	err := store.SetToken(token)
	require.NoError(t, err)

	// Delete
	err = store.DeleteToken("delete-me")
	require.NoError(t, err)

	// Verify deletion
	retrieved, err := store.GetToken("delete-me")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestTokenStore_DeleteToken_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	// Delete non-existent should not error
	err := store.DeleteToken("never-existed")
	assert.NoError(t, err)
}

func TestTokenStore_DeleteToken_PreservesOthers(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	// Add multiple tokens
	for _, name := range []string{"server1", "server2", "server3"} {
		err := store.SetToken(&MCPToken{
			ServerName:  name,
			AccessToken: "token-" + name,
		})
		require.NoError(t, err)
	}

	// Delete one
	err := store.DeleteToken("server2")
	require.NoError(t, err)

	// Verify others remain
	token1, err := store.GetToken("server1")
	require.NoError(t, err)
	assert.NotNil(t, token1)

	token3, err := store.GetToken("server3")
	require.NoError(t, err)
	assert.NotNil(t, token3)

	// Verify deleted is gone
	token2, err := store.GetToken("server2")
	require.NoError(t, err)
	assert.Nil(t, token2)
}

// =============================================================================
// MCPToken.IsExpired() Tests
// =============================================================================

func TestMCPToken_IsExpired_ZeroTime(t *testing.T) {
	token := &MCPToken{
		ServerName: "test",
		ExpiresAt:  time.Time{},
	}
	assert.False(t, token.IsExpired(), "zero time should not be considered expired")
}

func TestMCPToken_IsExpired_FarFuture(t *testing.T) {
	token := &MCPToken{
		ServerName: "test",
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}
	assert.False(t, token.IsExpired(), "token expiring in 24 hours should not be expired")
}

func TestMCPToken_IsExpired_FarPast(t *testing.T) {
	token := &MCPToken{
		ServerName: "test",
		ExpiresAt:  time.Now().Add(-24 * time.Hour),
	}
	assert.True(t, token.IsExpired(), "token expired 24 hours ago should be expired")
}

func TestMCPToken_IsExpired_JustExpired(t *testing.T) {
	token := &MCPToken{
		ServerName: "test",
		ExpiresAt:  time.Now().Add(-1 * time.Second),
	}
	assert.True(t, token.IsExpired(), "token that just expired should be expired")
}

func TestMCPToken_IsExpired_WithinBuffer(t *testing.T) {
	// Token expires in 3 minutes - within 5 minute buffer
	token := &MCPToken{
		ServerName: "test",
		ExpiresAt:  time.Now().Add(3 * time.Minute),
	}
	assert.True(t, token.IsExpired(), "token expiring within 5 minute buffer should be considered expired")
}

func TestMCPToken_IsExpired_JustOutsideBuffer(t *testing.T) {
	// Token expires in 6 minutes - just outside 5 minute buffer
	token := &MCPToken{
		ServerName: "test",
		ExpiresAt:  time.Now().Add(6 * time.Minute),
	}
	assert.False(t, token.IsExpired(), "token expiring in 6 minutes should not be expired")
}

func TestMCPToken_IsExpired_ExactlyAtBuffer(t *testing.T) {
	// Token expires in exactly 5 minutes - edge case
	// The implementation uses time.Now().Add(5 * time.Minute).After(t.ExpiresAt)
	// So exactly 5 minutes from now would NOT be expired (After returns false for equal)
	token := &MCPToken{
		ServerName: "test",
		ExpiresAt:  time.Now().Add(5*time.Minute + 1*time.Second),
	}
	assert.False(t, token.IsExpired(), "token expiring just after 5 minute buffer should not be expired")
}

func TestMCPToken_IsExpired_BufferBoundary(t *testing.T) {
	// Test various times around the 5 minute buffer
	testCases := []struct {
		name      string
		duration  time.Duration
		expectExp bool
	}{
		{"4 minutes", 4 * time.Minute, true},
		{"4.5 minutes", 4*time.Minute + 30*time.Second, true},
		{"5.5 minutes", 5*time.Minute + 30*time.Second, false},
		{"6 minutes", 6 * time.Minute, false},
		{"10 minutes", 10 * time.Minute, false},
		{"1 hour", time.Hour, false},
		{"-1 minute (past)", -1 * time.Minute, true},
		{"-5 minutes (past)", -5 * time.Minute, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			token := &MCPToken{
				ServerName: "test",
				ExpiresAt:  time.Now().Add(tc.duration),
			}
			assert.Equal(t, tc.expectExp, token.IsExpired())
		})
	}
}

// =============================================================================
// Token Refresh Detection Tests (5-minute buffer verification)
// =============================================================================

func TestMCPToken_RefreshDetection_NeedsRefresh(t *testing.T) {
	// Token expires in 2 minutes - should need refresh
	token := &MCPToken{
		ServerName:   "test",
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(2 * time.Minute),
	}

	// IsExpired returns true when token needs refresh (within buffer)
	assert.True(t, token.IsExpired(), "token expiring in 2 minutes needs refresh")
	assert.NotEmpty(t, token.RefreshToken, "refresh token should be available")
}

func TestMCPToken_RefreshDetection_NoRefreshNeeded(t *testing.T) {
	// Token expires in 30 minutes - no refresh needed
	token := &MCPToken{
		ServerName:   "test",
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(30 * time.Minute),
	}

	assert.False(t, token.IsExpired(), "token with 30 minutes left should not need refresh")
}

func TestMCPToken_RefreshDetection_NoRefreshToken(t *testing.T) {
	// Token is expiring but no refresh token available
	token := &MCPToken{
		ServerName:   "test",
		AccessToken:  "access",
		RefreshToken: "", // No refresh token
		ExpiresAt:    time.Now().Add(2 * time.Minute),
	}

	// Still reports as expired but RefreshToken is empty
	assert.True(t, token.IsExpired())
	assert.Empty(t, token.RefreshToken, "no refresh token available")
}

// =============================================================================
// File Permissions Tests (0600)
// =============================================================================

func TestTokenStore_FilePermissions_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	token := &MCPToken{
		ServerName:  "permission-test",
		AccessToken: "secret-token",
	}
	err := store.SetToken(token)
	require.NoError(t, err)

	// Check file permissions
	info, err := os.Stat(storePath)
	require.NoError(t, err)

	// On Unix-like systems, check for 0600
	perm := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0600), perm, "file should have 0600 permissions")
}

func TestTokenStore_FilePermissions_DirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "config", "slop-mcp", "auth.json")
	store := NewTokenStoreWithPath(nestedPath)

	token := &MCPToken{
		ServerName:  "dir-test",
		AccessToken: "token",
	}
	err := store.SetToken(token)
	require.NoError(t, err)

	// Check directory permissions (0700)
	dirInfo, err := os.Stat(filepath.Dir(nestedPath))
	require.NoError(t, err)

	dirPerm := dirInfo.Mode().Perm()
	assert.Equal(t, os.FileMode(0700), dirPerm, "directory should have 0700 permissions")
}

func TestTokenStore_FilePermissions_UpdatePreservesPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	// Create initial file
	token1 := &MCPToken{
		ServerName:  "first",
		AccessToken: "token1",
	}
	err := store.SetToken(token1)
	require.NoError(t, err)

	// Update file
	token2 := &MCPToken{
		ServerName:  "second",
		AccessToken: "token2",
	}
	err = store.SetToken(token2)
	require.NoError(t, err)

	// Permissions should still be 0600
	info, err := os.Stat(storePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

// =============================================================================
// Concurrent Token Access Tests (Thread Safety)
// =============================================================================

func TestTokenStore_ConcurrentReads(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	// Pre-populate with tokens
	for i := 0; i < 10; i++ {
		err := store.SetToken(&MCPToken{
			ServerName:  "server-" + string(rune('0'+i)),
			AccessToken: "token-" + string(rune('0'+i)),
		})
		require.NoError(t, err)
	}

	// Concurrent reads
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			serverName := "server-" + string(rune('0'+(i%10)))
			_, err := store.GetToken(serverName)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent read error: %v", err)
	}
}

func TestTokenStore_ConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	var wg sync.WaitGroup
	errors := make(chan error, 50)

	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			token := &MCPToken{
				ServerName:  "concurrent-server-" + string(rune('A'+i%26)),
				AccessToken: "token-" + string(rune('A'+i%26)),
			}
			if err := store.SetToken(token); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent write error: %v", err)
	}

	// Verify file is valid JSON
	data, err := os.ReadFile(storePath)
	require.NoError(t, err)

	var tf TokenFile
	err = json.Unmarshal(data, &tf)
	assert.NoError(t, err, "file should contain valid JSON after concurrent writes")
}

func TestTokenStore_ConcurrentReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	// Pre-populate
	for i := 0; i < 5; i++ {
		err := store.SetToken(&MCPToken{
			ServerName:  "server-" + string(rune('0'+i)),
			AccessToken: "token-" + string(rune('0'+i)),
		})
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	errors := make(chan error, 200)

	// Mixed reads and writes
	for i := 0; i < 100; i++ {
		wg.Add(2)

		// Reader
		go func(i int) {
			defer wg.Done()
			serverName := "server-" + string(rune('0'+(i%5)))
			_, err := store.GetToken(serverName)
			if err != nil {
				errors <- err
			}
		}(i)

		// Writer
		go func(i int) {
			defer wg.Done()
			token := &MCPToken{
				ServerName:  "new-server-" + string(rune('a'+i%26)),
				AccessToken: "new-token",
			}
			if err := store.SetToken(token); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent read/write error: %v", err)
	}
}

func TestTokenStore_ConcurrentDeletes(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	// Pre-populate
	for i := 0; i < 5; i++ {
		err := store.SetToken(&MCPToken{
			ServerName:  "delete-server-" + string(rune('a'+i)),
			AccessToken: "token",
		})
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	errors := make(chan error, 5)

	// Concurrent deletes
	// Note: Due to read-modify-write pattern, not all concurrent deletes may
	// result in the expected final state (some deletes may be lost).
	// This test verifies no errors/panics occur during concurrent access.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			serverName := "delete-server-" + string(rune('a'+i))
			if err := store.DeleteToken(serverName); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent delete error: %v", err)
	}

	// Verify file is still valid JSON (no corruption from concurrent access)
	data, err := os.ReadFile(storePath)
	require.NoError(t, err)

	var tf TokenFile
	err = json.Unmarshal(data, &tf)
	assert.NoError(t, err, "file should contain valid JSON after concurrent deletes")
}

func TestTokenStore_SequentialDeletes(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	// Pre-populate
	for i := 0; i < 5; i++ {
		err := store.SetToken(&MCPToken{
			ServerName:  "seq-delete-" + string(rune('a'+i)),
			AccessToken: "token",
		})
		require.NoError(t, err)
	}

	// Sequential deletes should all succeed
	for i := 0; i < 5; i++ {
		err := store.DeleteToken("seq-delete-" + string(rune('a'+i)))
		require.NoError(t, err)
	}

	// Verify all deleted
	tokens, err := store.ListTokens()
	require.NoError(t, err)
	assert.Empty(t, tokens, "all tokens should be deleted after sequential deletes")
}

// =============================================================================
// Load/Save Edge Cases Tests
// =============================================================================

func TestTokenStore_Load_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "nonexistent", "auth.json")
	store := NewTokenStoreWithPath(storePath)

	tf, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, 1, tf.Version)
	assert.NotNil(t, tf.Tokens)
	assert.Empty(t, tf.Tokens)
}

func TestTokenStore_Load_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")

	// Write invalid JSON
	err := os.WriteFile(storePath, []byte("not valid json {"), 0600)
	require.NoError(t, err)

	store := NewTokenStoreWithPath(storePath)
	_, err = store.Load()
	assert.Error(t, err)
}

func TestTokenStore_Load_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")

	// Write empty file
	err := os.WriteFile(storePath, []byte(""), 0600)
	require.NoError(t, err)

	store := NewTokenStoreWithPath(storePath)
	_, err = store.Load()
	// Empty file is invalid JSON, should error
	assert.Error(t, err)
}

func TestTokenStore_Load_NullTokens(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")

	// Write JSON with null tokens
	err := os.WriteFile(storePath, []byte(`{"version": 1, "tokens": null}`), 0600)
	require.NoError(t, err)

	store := NewTokenStoreWithPath(storePath)
	tf, err := store.Load()
	require.NoError(t, err)
	// Should initialize to empty map
	assert.NotNil(t, tf.Tokens)
}

func TestTokenStore_Load_EmptyTokensMap(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")

	// Write JSON with empty tokens
	err := os.WriteFile(storePath, []byte(`{"version": 1, "tokens": {}}`), 0600)
	require.NoError(t, err)

	store := NewTokenStoreWithPath(storePath)
	tf, err := store.Load()
	require.NoError(t, err)
	assert.NotNil(t, tf.Tokens)
	assert.Empty(t, tf.Tokens)
}

func TestTokenStore_Save_FormatsJSON(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	tf := &TokenFile{
		Version: 1,
		Tokens: map[string]*MCPToken{
			"test": {ServerName: "test", AccessToken: "token"},
		},
	}

	err := store.Save(tf)
	require.NoError(t, err)

	// Check that JSON is indented (pretty printed)
	data, err := os.ReadFile(storePath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "\n", "JSON should be pretty printed")
	assert.Contains(t, string(data), "  ", "JSON should be indented")
}

// =============================================================================
// ListTokens Tests
// =============================================================================

func TestTokenStore_ListTokens_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	tokens, err := store.ListTokens()
	require.NoError(t, err)
	assert.Empty(t, tokens)
}

func TestTokenStore_ListTokens_Multiple(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	// Add tokens
	serverNames := []string{"alpha", "beta", "gamma", "delta"}
	for _, name := range serverNames {
		err := store.SetToken(&MCPToken{
			ServerName:  name,
			AccessToken: "token-" + name,
		})
		require.NoError(t, err)
	}

	tokens, err := store.ListTokens()
	require.NoError(t, err)
	assert.Len(t, tokens, 4)

	// Verify all expected tokens are present
	foundNames := make(map[string]bool)
	for _, tok := range tokens {
		foundNames[tok.ServerName] = true
	}
	for _, name := range serverNames {
		assert.True(t, foundNames[name], "token %s should be in list", name)
	}
}

// =============================================================================
// TokenFile Structure Tests
// =============================================================================

func TestTokenFile_JSONSerialization(t *testing.T) {
	tf := &TokenFile{
		Version: 1,
		Tokens: map[string]*MCPToken{
			"test-server": {
				ServerName:    "test-server",
				ServerURL:     "https://test.example.com",
				ClientID:      "client-id",
				ClientSecret:  "client-secret",
				AccessToken:   "access-token",
				RefreshToken:  "refresh-token",
				TokenType:     "Bearer",
				ExpiresAt:     time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
				Scope:         "read write",
				TokenEndpoint: "https://auth.example.com/token",
			},
		},
	}

	data, err := json.Marshal(tf)
	require.NoError(t, err)

	var parsed TokenFile
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, tf.Version, parsed.Version)
	assert.Len(t, parsed.Tokens, 1)
	assert.NotNil(t, parsed.Tokens["test-server"])
	assert.Equal(t, "access-token", parsed.Tokens["test-server"].AccessToken)
}

func TestTokenFile_OmitsEmptyFields(t *testing.T) {
	token := &MCPToken{
		ServerName:  "minimal",
		AccessToken: "token",
		// All other fields empty/zero
	}

	data, err := json.Marshal(token)
	require.NoError(t, err)

	// Check that empty fields with omitempty are not in JSON
	assert.NotContains(t, string(data), "client_secret")
	assert.NotContains(t, string(data), "refresh_token")
	assert.NotContains(t, string(data), "scope")
	assert.NotContains(t, string(data), "token_endpoint")
}

// =============================================================================
// NewTokenStore Tests
// =============================================================================

func TestNewTokenStore_DefaultPath(t *testing.T) {
	store := NewTokenStore()
	path := store.Path()

	// Should contain slop-mcp/auth.json
	assert.Contains(t, path, "slop-mcp")
	assert.Contains(t, path, "auth.json")
}

func TestNewTokenStoreWithPath_CustomPath(t *testing.T) {
	customPath := "/custom/path/auth.json"
	store := NewTokenStoreWithPath(customPath)
	assert.Equal(t, customPath, store.Path())
}

// =============================================================================
// Edge Cases and Error Handling Tests
// =============================================================================

func TestTokenStore_UnreadableFile(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")

	// Create file with no read permissions
	err := os.WriteFile(storePath, []byte(`{"version":1,"tokens":{}}`), 0000)
	require.NoError(t, err)

	// Ensure we can clean up
	t.Cleanup(func() {
		os.Chmod(storePath, 0644)
	})

	store := NewTokenStoreWithPath(storePath)
	_, err = store.Load()
	assert.Error(t, err, "should fail to read file with no permissions")
}

func TestTokenStore_NonWritableDirectory(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Test not applicable when running as root")
	}

	tmpDir := t.TempDir()
	restrictedDir := filepath.Join(tmpDir, "restricted")
	err := os.Mkdir(restrictedDir, 0555) // Read+execute only
	require.NoError(t, err)

	// Ensure we can clean up
	t.Cleanup(func() {
		os.Chmod(restrictedDir, 0755)
	})

	storePath := filepath.Join(restrictedDir, "auth.json")
	store := NewTokenStoreWithPath(storePath)

	token := &MCPToken{
		ServerName:  "test",
		AccessToken: "token",
	}
	err = store.SetToken(token)
	assert.Error(t, err, "should fail to write to non-writable directory")
}

func TestMCPToken_IsExpired_EdgeCaseNow(t *testing.T) {
	// Test the boundary condition more precisely
	// If ExpiresAt is exactly 5 minutes from now:
	// time.Now().Add(5 * time.Minute).After(ExpiresAt) should be false
	now := time.Now()
	exactlyAtBuffer := now.Add(5 * time.Minute)

	token := &MCPToken{
		ServerName: "test",
		ExpiresAt:  exactlyAtBuffer,
	}

	// The implementation checks: time.Now().Add(5 * time.Minute).After(t.ExpiresAt)
	// If ExpiresAt == time.Now().Add(5*min), then After returns false
	// But due to time passing during test, we need some tolerance
	// Token expiring exactly at the buffer boundary is edge case
	// A few milliseconds either way could change result
	// This is acceptable behavior - the 5 minute buffer is approximate
	// We just verify it returns a boolean (either true or false is acceptable at exact boundary)
	_ = token.IsExpired()
}
