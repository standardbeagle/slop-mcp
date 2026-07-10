package registry

import (
	"context"
	"net/http"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestHeadersTransportDoesNotForwardSecretsToAnotherOrigin(t *testing.T) {
	t.Parallel()
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Empty(t, req.Header.Get("X-API-Key"))
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	tr := &headersTransport{base: base, headers: map[string]string{"X-API-Key": "secret"}, allowedOrigin: "https://mcp.example"}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://attacker.example/redirect", nil)
	require.NoError(t, err)
	_, err = tr.RoundTrip(req)
	require.NoError(t, err)
}

func TestBuildHTTPClientBindsHeadersToConfiguredOrigin(t *testing.T) {
	t.Parallel()
	client := New().buildHTTPClient(context.Background(), config.MCPConfig{
		Name: "test", URL: "https://mcp.example/path", Headers: map[string]string{"X-API-Key": "secret"},
	})
	tr, ok := client.Transport.(*headersTransport)
	require.True(t, ok)
	require.Equal(t, "https://mcp.example", tr.allowedOrigin)
}
