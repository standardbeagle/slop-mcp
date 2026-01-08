//go:build mcp_go_client_oauth

// Package auth handles OAuth authentication for MCP servers.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

// TokenStore manages OAuth tokens for MCP servers.
type TokenStore struct {
	path string
	mu   sync.RWMutex
}

// MCPToken represents stored OAuth tokens for an MCP server.
type MCPToken struct {
	ServerName   string    `json:"server_name"`
	ServerURL    string    `json:"server_url"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret,omitempty"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Scope        string    `json:"scope,omitempty"`
}

// TokenFile is the structure of the auth token file.
type TokenFile struct {
	Version int                  `json:"version"`
	Tokens  map[string]*MCPToken `json:"tokens"` // keyed by server name
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

	// Ensure directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return err
	}

	// Write with restricted permissions
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
	Token       *MCPToken
	AuthURL     string // If non-empty, user needs to visit this URL
	CallbackURL string // The callback URL for the flow
}

// DiscoverAndAuth discovers OAuth configuration and initiates the auth flow.
func (f *OAuthFlow) DiscoverAndAuth(ctx context.Context) (*AuthResult, error) {
	// Step 1: Try to get protected resource metadata
	prm, err := oauthex.GetProtectedResourceMetadataFromID(ctx, f.ServerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get protected resource metadata: %w", err)
	}

	if len(prm.AuthorizationServers) == 0 {
		return nil, fmt.Errorf("no authorization servers found in metadata")
	}

	// Step 2: Get authorization server metadata
	authServerURL := prm.AuthorizationServers[0]
	asm, err := oauthex.GetAuthServerMeta(ctx, authServerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get auth server metadata: %w", err)
	}

	// Step 3: Dynamic client registration if supported
	var clientID, clientSecret string
	if asm.RegistrationEndpoint != "" {
		regResp, err := f.registerClient(ctx, asm.RegistrationEndpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to register client: %w", err)
		}
		clientID = regResp.ClientID
		clientSecret = regResp.ClientSecret
	} else {
		return nil, fmt.Errorf("server does not support dynamic client registration; manual registration required")
	}

	// Step 4: Start local callback server
	callbackURL, codeChan, err := f.startCallbackServer()
	if err != nil {
		return nil, fmt.Errorf("failed to start callback server: %w", err)
	}

	// Step 5: Generate PKCE verifier and challenge
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, fmt.Errorf("failed to generate PKCE: %w", err)
	}

	// Step 6: Build authorization URL
	state := generateState()
	authURL := buildAuthURL(asm.AuthorizationEndpoint, clientID, callbackURL, f.ServerURL, state, challenge, prm.ScopesSupported)

	// Step 7: Open browser
	if err := openBrowser(authURL); err != nil {
		fmt.Printf("Please open this URL in your browser:\n%s\n", authURL)
	}

	// Step 8: Wait for callback
	fmt.Println("Waiting for authorization callback...")
	select {
	case code := <-codeChan:
		if code.err != nil {
			return nil, code.err
		}
		if code.state != state {
			return nil, fmt.Errorf("state mismatch")
		}

		// Step 9: Exchange code for token
		token, err := f.exchangeCode(ctx, asm.TokenEndpoint, clientID, clientSecret, code.code, callbackURL, verifier, f.ServerURL)
		if err != nil {
			return nil, fmt.Errorf("failed to exchange code: %w", err)
		}

		mcpToken := &MCPToken{
			ServerName:   f.ServerName,
			ServerURL:    f.ServerURL,
			ClientID:     clientID,
			ClientSecret: clientSecret,
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			TokenType:    token.TokenType,
			ExpiresAt:    token.Expiry,
		}

		// Save token
		if err := f.Store.SetToken(mcpToken); err != nil {
			return nil, fmt.Errorf("failed to save token: %w", err)
		}

		return &AuthResult{Token: mcpToken}, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("authorization timeout")
	}
}

func (f *OAuthFlow) registerClient(ctx context.Context, endpoint string) (*oauthex.ClientRegistrationResponse, error) {
	meta := &oauthex.ClientRegistrationMetadata{
		RedirectURIs:            []string{"http://localhost:0/callback"}, // Will be updated
		ClientName:              "slop-mcp",
		TokenEndpointAuthMethod: "none", // Public client
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
	}
	return oauthex.RegisterClient(ctx, endpoint, meta, nil)
}

type callbackResult struct {
	code  string
	state string
	err   error
}

func (f *OAuthFlow) startCallbackServer() (string, <-chan callbackResult, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}

	port := listener.Addr().(*net.TCPAddr).Port
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	codeChan := make(chan callbackResult, 1)

	server := &http.Server{}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}

		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		errParam := r.URL.Query().Get("error")

		if errParam != "" {
			errDesc := r.URL.Query().Get("error_description")
			codeChan <- callbackResult{err: fmt.Errorf("%s: %s", errParam, errDesc)}
		} else {
			codeChan <- callbackResult{code: code, state: state}
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Authorization Complete</title></head>
<body>
<h1>Authorization Complete</h1>
<p>You can close this window and return to the terminal.</p>
<script>window.close();</script>
</body>
</html>`)

		go func() {
			time.Sleep(100 * time.Millisecond)
			server.Shutdown(context.Background())
		}()
	})

	go server.Serve(listener)
	return callbackURL, codeChan, nil
}

func (f *OAuthFlow) exchangeCode(ctx context.Context, tokenEndpoint, clientID, clientSecret, code, redirectURI, verifier, resource string) (*oauth2.Token, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {verifier},
		"resource":      {resource},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if clientSecret != "" {
		req.SetBasicAuth(clientID, clientSecret)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	token := &oauth2.Token{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		RefreshToken: tokenResp.RefreshToken,
	}
	if tokenResp.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}
	return token, nil
}

func generatePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)

	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func buildAuthURL(endpoint, clientID, redirectURI, resource, state, challenge string, scopes []string) string {
	v := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"resource":              {resource},
	}
	if len(scopes) > 0 {
		v.Set("scope", strings.Join(scopes, " "))
	}
	return endpoint + "?" + v.Encode()
}

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		return fmt.Errorf("unsupported platform")
	}

	return exec.Command(cmd, args...).Start()
}

// RefreshToken refreshes an expired token.
func RefreshToken(ctx context.Context, token *MCPToken, tokenEndpoint string) (*MCPToken, error) {
	if token.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {token.RefreshToken},
		"client_id":     {token.ClientID},
		"resource":      {token.ServerURL},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if token.ClientSecret != "" {
		req.SetBasicAuth(token.ClientID, token.ClientSecret)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed with status %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	newToken := &MCPToken{
		ServerName:   token.ServerName,
		ServerURL:    token.ServerURL,
		ClientID:     token.ClientID,
		ClientSecret: token.ClientSecret,
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}
	if tokenResp.RefreshToken != "" {
		newToken.RefreshToken = tokenResp.RefreshToken
	} else {
		newToken.RefreshToken = token.RefreshToken
	}
	return newToken, nil
}
