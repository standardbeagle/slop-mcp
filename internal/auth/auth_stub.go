//go:build !mcp_go_client_oauth

// Package auth handles OAuth authentication for MCP servers.
// This is the stub version when OAuth support is not compiled in.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TokenStore manages OAuth tokens for MCP servers.
type TokenStore struct {
	path string
	mu   sync.RWMutex
}

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
	Tokens  map[string]*MCPToken `json:"tokens"`
}

// NewTokenStore creates a new token store.
func NewTokenStore() *TokenStore {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = os.Getenv("HOME")
	}
	return &TokenStore{
		path: filepath.Join(configDir, "slop-mcp", "auth.json"),
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
	s.mu.RLock()
	defer s.mu.RUnlock()

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

// Save writes tokens to disk.
func (s *TokenStore) Save(tf *TokenFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0600)
}

// GetToken retrieves a token for an MCP server.
func (s *TokenStore) GetToken(serverName string) (*MCPToken, error) {
	tf, err := s.Load()
	if err != nil {
		return nil, err
	}
	return tf.Tokens[serverName], nil
}

// SetToken stores a token for an MCP server.
func (s *TokenStore) SetToken(token *MCPToken) error {
	tf, err := s.Load()
	if err != nil {
		return err
	}
	tf.Tokens[token.ServerName] = token
	return s.Save(tf)
}

// DeleteToken removes a token for an MCP server.
func (s *TokenStore) DeleteToken(serverName string) error {
	tf, err := s.Load()
	if err != nil {
		return err
	}
	delete(tf.Tokens, serverName)
	return s.Save(tf)
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

// IsExpired checks if a token is expired.
func (t *MCPToken) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return false
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
	Token       *MCPToken
	AuthURL     string
	CallbackURL string
}

// DiscoverAndAuth is a stub that returns an error when OAuth is not compiled in.
func (f *OAuthFlow) DiscoverAndAuth(ctx context.Context) (*AuthResult, error) {
	return nil, fmt.Errorf("OAuth support not compiled in. Rebuild with: go build -tags mcp_go_client_oauth")
}

// RefreshToken is a stub that returns an error when OAuth is not compiled in.
func RefreshToken(ctx context.Context, token *MCPToken, tokenEndpoint string) (*MCPToken, error) {
	return nil, fmt.Errorf("OAuth support not compiled in. Rebuild with: go build -tags mcp_go_client_oauth")
}
