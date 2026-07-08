// Package auth handles OAuth authentication for MCP servers.
// This file contains the token store shared by both the OAuth and stub builds.
package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/atomicfile"
	"github.com/standardbeagle/slop-mcp/internal/config"
)

// TokenStore manages OAuth tokens for MCP servers.
type TokenStore struct {
	path string
}

// storeMu serializes all token file access. It is package-level (not
// per-instance) because callers construct a fresh TokenStore per operation,
// which would make a per-instance mutex ineffective.
var storeMu sync.Mutex

// MCPToken represents stored OAuth tokens for an MCP server.
type MCPToken struct {
	ServerName    string    `json:"server_name"`
	ServerURL     string    `json:"server_url"`
	ClientID      string    `json:"client_id"`
	ClientSecret  string    `json:"client_secret,omitempty"`
	AccessToken   string    `json:"access_token"`
	RefreshToken  string    `json:"refresh_token,omitempty"`
	TokenType     string    `json:"token_type"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	Scope         string    `json:"scope,omitempty"`
	TokenEndpoint string    `json:"token_endpoint,omitempty"` // For refresh token flow
}

// TokenFile is the structure of the auth token file.
type TokenFile struct {
	Version int                  `json:"version"`
	Tokens  map[string]*MCPToken `json:"tokens"` // keyed by server name
}

// NewTokenStore creates a new token store.
func NewTokenStore() *TokenStore {
	configDir := config.UserConfigDirPath()
	if configDir == "" {
		return &TokenStore{}
	}
	return &TokenStore{
		path: filepath.Join(configDir, "auth.json"),
	}
}

// NewTokenStoreWithPath creates a token store with a custom path.
func NewTokenStoreWithPath(path string) *TokenStore {
	return &TokenStore{path: path}
}

// Path returns the token store file path.
func (s *TokenStore) Path() string {
	return s.path
}

// Load reads tokens from disk.
func (s *TokenStore) Load() (*TokenFile, error) {
	storeMu.Lock()
	defer storeMu.Unlock()

	return s.loadUnlocked()
}

func (s *TokenStore) loadUnlocked() (*TokenFile, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return &TokenFile{Version: 1, Tokens: make(map[string]*MCPToken)}, nil
	}
	if err != nil {
		return nil, err
	}

	var tf TokenFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, err
	}
	if tf.Tokens == nil {
		tf.Tokens = make(map[string]*MCPToken)
	}
	return &tf, nil
}

// Save writes tokens to disk atomically (temp file + rename).
func (s *TokenStore) Save(tf *TokenFile) error {
	storeMu.Lock()
	defer storeMu.Unlock()

	return s.saveUnlocked(tf)
}

func (s *TokenStore) saveUnlocked(tf *TokenFile) error {
	// Ensure directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write with restricted permissions (unique temp file + rename).
	return atomicfile.WriteFile(s.path, data, 0600)
}

// GetToken retrieves a token for an MCP server.
func (s *TokenStore) GetToken(serverName string) (*MCPToken, error) {
	tf, err := s.Load()
	if err != nil {
		return nil, err
	}
	return tf.Tokens[serverName], nil
}

// SetToken stores a token for an MCP server. The lock is held across the
// load-modify-save cycle so concurrent updates cannot lose writes.
func (s *TokenStore) SetToken(token *MCPToken) error {
	storeMu.Lock()
	defer storeMu.Unlock()

	tf, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	tf.Tokens[token.ServerName] = token
	return s.saveUnlocked(tf)
}

// DeleteToken removes a token for an MCP server. The lock is held across the
// load-modify-save cycle so concurrent updates cannot lose writes.
func (s *TokenStore) DeleteToken(serverName string) error {
	storeMu.Lock()
	defer storeMu.Unlock()

	tf, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	delete(tf.Tokens, serverName)
	return s.saveUnlocked(tf)
}

// ListTokens returns all stored tokens.
func (s *TokenStore) ListTokens() ([]*MCPToken, error) {
	tf, err := s.Load()
	if err != nil {
		return nil, err
	}
	tokens := make([]*MCPToken, 0, len(tf.Tokens))
	for _, t := range tf.Tokens {
		tokens = append(tokens, t)
	}
	return tokens, nil
}

// IsExpired checks if a token is expired (with 5 minute buffer).
func (t *MCPToken) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return false // No expiry set
	}
	return time.Now().Add(5 * time.Minute).After(t.ExpiresAt)
}

// OAuthFlow handles the OAuth authorization flow for an MCP server.
type OAuthFlow struct {
	ServerName string
	ServerURL  string
	Store      *TokenStore
}

// AuthResult contains the result of an OAuth flow.
type AuthResult struct {
	Token *MCPToken
}
