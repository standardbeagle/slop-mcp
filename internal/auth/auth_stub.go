//go:build !mcp_go_client_oauth

// Package auth handles OAuth authentication for MCP servers.
// This is the stub version when OAuth support is not compiled in.
// The TokenStore implementation is shared and lives in tokenstore.go.
package auth

import (
	"context"
	"fmt"
)

// DiscoverAndAuth is a stub that returns an error when OAuth is not compiled in.
func (f *OAuthFlow) DiscoverAndAuth(ctx context.Context) (*AuthResult, error) {
	return nil, fmt.Errorf("OAuth support not compiled in. Rebuild with: go build -tags mcp_go_client_oauth")
}

// RefreshToken is a stub that returns an error when OAuth is not compiled in.
func RefreshToken(ctx context.Context, token *MCPToken, tokenEndpoint string) (*MCPToken, error) {
	return nil, fmt.Errorf("OAuth support not compiled in. Rebuild with: go build -tags mcp_go_client_oauth")
}
