// Package factory wires LLM config into a ready-to-use Registry.
// It imports both internal/llm and internal/llm/providers; neither
// of those packages may import factory (would create a cycle).
package factory

import (
	"net/http"
	"os"

	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/llm"
	"github.com/standardbeagle/slop-mcp/internal/llm/providers"
)

// BuildDeps lets callers inject collaborators that the registry itself
// should not own — primarily for testing or for sharing OAuth state with
// auth_mcp.
type BuildDeps struct {
	HTTPClient        *http.Client
	CopilotTokenStore CopilotTokenStore // optional; nil disables copilot
}

// CopilotTokenStore is the minimal hook the copilot provider needs.
// auth_mcp implements this in the real wire-up.
type CopilotTokenStore interface {
	Token() (string, error)
}

// Build constructs a fully wired Registry from parsed config and dependencies.
func Build(cfg *config.Config, deps BuildDeps) (*llm.Registry, error) {
	r := llm.NewRegistry()
	if cfg.LLM != nil {
		r.SetChain(cfg.LLM.Chain)
		for name, pc := range cfg.LLM.Providers {
			p, err := buildProvider(name, pc, deps)
			if err != nil {
				return nil, err
			}
			if p != nil {
				r.RegisterProvider(p)
			}
		}
	}
	for name, ac := range cfg.Agents {
		r.RegisterAgent(&llm.AgentDef{
			Name:        name,
			Backend:     ac.Backend,
			Model:       ac.Model,
			System:      ac.System,
			Tools:       ac.Tools,
			Temperature: ac.Temperature,
			MaxTokens:   ac.MaxTokens,
			MaxIters:    ac.MaxIters,
		})
	}
	return r, nil
}

func buildProvider(name string, pc *config.LLMProviderConfig, deps BuildDeps) (llm.Provider, error) {
	switch name {
	case "openai":
		return providers.NewOpenAI(providers.OpenAIConfig{
			BaseURL:      pc.BaseURL,
			APIKey:       resolveKey(pc),
			DefaultModel: pc.DefaultModel,
			HTTPClient:   deps.HTTPClient,
		}), nil
	case "copilot":
		if deps.CopilotTokenStore == nil {
			return nil, nil // skip when auth not wired
		}
		return providers.NewCopilot(providers.CopilotConfig{
			DefaultModel: pc.DefaultModel,
			TokenStore:   deps.CopilotTokenStore,
			HTTPClient:   deps.HTTPClient,
		}), nil
	case "acp":
		return providers.NewACP(providers.ACPConfig{
			Command: pc.Command,
			URL:     pc.URL,
		}), nil
	default:
		return nil, nil // unknown name — caller may add later
	}
}

func resolveKey(pc *config.LLMProviderConfig) string {
	if pc.APIKey != "" {
		return pc.APIKey
	}
	if pc.APIKeyEnv != "" {
		return os.Getenv(pc.APIKeyEnv)
	}
	return ""
}
