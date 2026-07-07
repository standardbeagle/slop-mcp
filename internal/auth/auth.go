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
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

// wellKnownURL derives a metadata discovery URL by inserting the given
// well-known path prefix ahead of the base URL's existing path, per the
// "well-known URI" conventions of RFC 9728 (protected resource) and RFC 8414
// (authorization server). For example, a base of "https://host/mcp" with
// prefix "/.well-known/oauth-protected-resource" yields
// "https://host/.well-known/oauth-protected-resource/mcp".
// Query and fragment components of the base URL are dropped: well-known
// metadata URLs are derived from the identifier's authority and path only.
func wellKnownURL(baseURL, wellKnownPath string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	joined := path.Join(wellKnownPath, u.Path)
	// path.Join strips trailing slashes; preserve one if the base path had it.
	if strings.HasSuffix(u.Path, "/") && !strings.HasSuffix(joined, "/") {
		joined += "/"
	}
	u.Path = joined
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	u.RawFragment = ""
	return u.String(), nil
}

// authServerMetaURLs returns candidate metadata discovery URLs for an issuer,
// in priority order:
//  1. RFC 8414 path-insert form for oauth-authorization-server
//  2. path-insert form for openid-configuration
//  3. OIDC Discovery path-appended form (<issuer>/.well-known/openid-configuration),
//     the standard location for path-bearing issuers such as Keycloak realms.
//
// When the issuer has no path, the appended form equals the insert form and is
// deduplicated.
func authServerMetaURLs(issuer string) ([]string, error) {
	var urls []string
	for _, wk := range []string{"/.well-known/oauth-authorization-server", "/.well-known/openid-configuration"} {
		u, err := wellKnownURL(issuer, wk)
		if err != nil {
			return nil, err
		}
		urls = append(urls, u)
	}

	// Path-appended OIDC form.
	u, err := url.Parse(issuer)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, "/.well-known/openid-configuration")
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	u.RawFragment = ""
	appended := u.String()

	for _, existing := range urls {
		if existing == appended {
			return urls, nil
		}
	}
	return append(urls, appended), nil
}

// DiscoverAndAuth discovers OAuth configuration and initiates the auth flow.
func (f *OAuthFlow) DiscoverAndAuth(ctx context.Context) (*AuthResult, error) {
	// Step 1: Try to get protected resource metadata.
	// Derive the RFC 9728 well-known metadata URL from the resource ID, then
	// validate the returned metadata's resource matches f.ServerURL.
	prmURL, err := wellKnownURL(f.ServerURL, "/.well-known/oauth-protected-resource")
	if err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}
	prm, err := oauthex.GetProtectedResourceMetadata(ctx, prmURL, f.ServerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get protected resource metadata: %w", err)
	}
	if prm == nil {
		return nil, fmt.Errorf("no protected resource metadata found at %s", prmURL)
	}

	if len(prm.AuthorizationServers) == 0 {
		return nil, fmt.Errorf("no authorization servers found in metadata")
	}

	// Step 2: Get authorization server metadata.
	// Derive well-known metadata URLs from the issuer (auth server) URL; see
	// authServerMetaURLs for the candidate order. GetAuthServerMeta returns
	// (nil, nil) on a 4xx response, so a nil result means "not found here"
	// rather than a hard error.
	authServerURL := prm.AuthorizationServers[0]
	asmURLs, err := authServerMetaURLs(authServerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid authorization server URL: %w", err)
	}
	var asm *oauthex.AuthServerMeta
	var asmErrs []string
	for _, asmURL := range asmURLs {
		asm, err = oauthex.GetAuthServerMeta(ctx, asmURL, authServerURL, nil)
		if err != nil {
			asmErrs = append(asmErrs, fmt.Sprintf("%s: %v", asmURL, err))
			asm = nil
			continue
		}
		if asm != nil {
			break
		}
		asmErrs = append(asmErrs, fmt.Sprintf("%s: not found", asmURL))
	}
	if asm == nil {
		return nil, fmt.Errorf("failed to get auth server metadata for %s: %s", authServerURL, strings.Join(asmErrs, "; "))
	}

	if asm.RegistrationEndpoint == "" {
		return nil, fmt.Errorf("server does not support dynamic client registration; manual registration required")
	}

	// Step 3: Start local callback server first so the actual redirect URI
	// (with the real bound port) can be used during client registration.
	callbackURL, codeChan, shutdown, err := f.startCallbackServer()
	if err != nil {
		return nil, fmt.Errorf("failed to start callback server: %w", err)
	}
	defer shutdown()

	// Step 4: Dynamic client registration with the actual redirect URI.
	regResp, err := f.registerClient(ctx, asm.RegistrationEndpoint, callbackURL)
	if err != nil {
		return nil, fmt.Errorf("failed to register client: %w", err)
	}
	clientID := regResp.ClientID
	clientSecret := regResp.ClientSecret

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
		// stderr: stdout is reserved for MCP JSON-RPC protocol
		fmt.Fprintf(os.Stderr, "Please open this URL in your browser:\n%s\n", authURL)
	}

	// Step 8: Wait for callback
	// stderr: stdout is reserved for MCP JSON-RPC protocol
	fmt.Fprintln(os.Stderr, "Waiting for authorization callback...")
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
			ServerName:    f.ServerName,
			ServerURL:     f.ServerURL,
			ClientID:      clientID,
			ClientSecret:  clientSecret,
			AccessToken:   token.AccessToken,
			RefreshToken:  token.RefreshToken,
			TokenType:     token.TokenType,
			ExpiresAt:     token.Expiry,
			TokenEndpoint: asm.TokenEndpoint, // Store for refresh token flow
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

func (f *OAuthFlow) registerClient(ctx context.Context, endpoint, redirectURI string) (*oauthex.ClientRegistrationResponse, error) {
	meta := &oauthex.ClientRegistrationMetadata{
		RedirectURIs:            []string{redirectURI},
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

func (f *OAuthFlow) startCallbackServer() (string, <-chan callbackResult, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, nil, err
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

		var res callbackResult
		if errParam != "" {
			errDesc := r.URL.Query().Get("error_description")
			res = callbackResult{err: fmt.Errorf("%s: %s", errParam, errDesc)}
		} else {
			res = callbackResult{code: code, state: state}
		}

		// Non-blocking send: a second callback (e.g. browser retry/refresh)
		// must not block the handler forever while shutdown waits on it.
		select {
		case codeChan <- res:
		default:
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Already Processed</title></head>
<body>
<h1>Authorization Already Processed</h1>
<p>This authorization response was already handled. You can close this window.</p>
</body>
</html>`)
			return
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
			_ = server.Shutdown(context.Background())
		}()
	})

	go func() { _ = server.Serve(listener) }()
	shutdown := func() { _ = server.Shutdown(context.Background()) }
	return callbackURL, codeChan, shutdown, nil
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
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is catastrophic; fall back to time-based state
		return base64.RawURLEncoding.EncodeToString([]byte(time.Now().String()))
	}
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
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	newToken := &MCPToken{
		ServerName:    token.ServerName,
		ServerURL:     token.ServerURL,
		ClientID:      token.ClientID,
		ClientSecret:  token.ClientSecret,
		AccessToken:   tokenResp.AccessToken,
		TokenType:     tokenResp.TokenType,
		Scope:         token.Scope,         // Preserve scope unless the response narrows it
		TokenEndpoint: token.TokenEndpoint, // Preserve token endpoint
	}
	if tokenResp.Scope != "" {
		newToken.Scope = tokenResp.Scope
	}
	// No expires_in means no known expiry: leave ExpiresAt as the zero time,
	// which IsExpired treats as "never expires" (matches exchangeCode).
	if tokenResp.ExpiresIn > 0 {
		newToken.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}
	if tokenResp.RefreshToken != "" {
		newToken.RefreshToken = tokenResp.RefreshToken
	} else {
		newToken.RefreshToken = token.RefreshToken
	}
	return newToken, nil
}
