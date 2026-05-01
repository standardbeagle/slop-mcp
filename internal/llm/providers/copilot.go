package providers

import (
	"context"
	"net/http"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

// CopilotConfig configures the GitHub Copilot provider.
// Copilot exposes an OpenAI-compatible chat endpoint; this config
// supplies the Copilot-specific headers and OAuth token source.
type CopilotConfig struct {
	BaseURL       string
	DefaultModel  string
	TokenStore    interface{ Token() (string, error) }
	HTTPClient    *http.Client
	EditorVersion string
}

// Copilot wraps OpenAI with Copilot-specific auth headers injected per request.
type Copilot struct {
	inner *OpenAI
}

// NewCopilot constructs a Copilot provider that delegates to an OpenAI-compatible
// endpoint with OAuth token auth and Copilot-specific headers.
func NewCopilot(cfg CopilotConfig) *Copilot {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.githubcopilot.com"
	}
	if cfg.EditorVersion == "" {
		cfg.EditorVersion = "slop-mcp/0"
	}
	inner := NewOpenAI(OpenAIConfig{
		Name:         "copilot",
		BaseURL:      cfg.BaseURL,
		DefaultModel: cfg.DefaultModel,
		HTTPClient:   cfg.HTTPClient,
		AuthHeader: func(r *http.Request) {
			if cfg.TokenStore != nil {
				// Token error is intentionally dropped — the server will
				// reply 401 and the OpenAI provider surfaces that with the
				// bounded error body. UnavailableReason is the diagnostic
				// path, not this header injector.
				tok, _ := cfg.TokenStore.Token()
				if tok != "" {
					r.Header.Set("Authorization", "Bearer "+tok)
				}
			}
			r.Header.Set("Editor-Version", cfg.EditorVersion)
			// Integration ID is the value VS Code's Copilot extension uses;
			// confirm against current API docs at release time.
			r.Header.Set("Copilot-Integration-Id", "vscode-chat")
		},
		Available: func(_ context.Context) bool {
			if cfg.TokenStore == nil {
				return false
			}
			tok, err := cfg.TokenStore.Token()
			return err == nil && tok != ""
		},
		UnavailableReason: func() string {
			if cfg.TokenStore == nil {
				return "copilot: no token store wired"
			}
			tok, err := cfg.TokenStore.Token()
			if err != nil {
				return "copilot: token store error: " + err.Error()
			}
			if tok == "" {
				return "copilot: no oauth token"
			}
			return ""
		},
	})
	return &Copilot{inner: inner}
}

func (c *Copilot) Name() string                     { return "copilot" }
func (c *Copilot) Available(ctx context.Context) bool { return c.inner.Available(ctx) }
func (c *Copilot) UnavailableReason() string         { return c.inner.UnavailableReason() }

// Sample delegates to the inner OpenAI provider.
func (c *Copilot) Sample(ctx context.Context, req *llm.Request, on func(llm.StreamEvent)) (*llm.Response, error) {
	return c.inner.Sample(ctx, req, on)
}
