package factory

import (
	"context"
	"os"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/config"
)

func TestBuild_RegistersProvidersAndAgents(t *testing.T) {
	t.Setenv("FOO_KEY", "test-key")
	cfg := &config.Config{
		LLM: &config.LLMConfig{
			Chain: []string{"openai"},
			Providers: map[string]*config.LLMProviderConfig{
				"openai": {BaseURL: "http://localhost:9999", APIKeyEnv: "FOO_KEY", DefaultModel: "m1"},
			},
		},
		Agents: map[string]*config.AgentConfig{
			"summarizer": {Name: "summarizer", Backend: "openai", Model: "m1", MaxIters: 1},
		},
	}

	r, err := Build(cfg, BuildDeps{})
	if err != nil {
		t.Fatal(err)
	}
	def, prov, err := r.Resolve(context.Background(), "summarizer")
	if err != nil {
		t.Fatal(err)
	}
	if def.MaxIters != 1 {
		t.Fatalf("want max_iters 1, got %d", def.MaxIters)
	}
	if prov.Name() != "openai" {
		t.Fatalf("want openai, got %q", prov.Name())
	}

	// Provider should not be available if env var unset.
	os.Unsetenv("FOO_KEY")
	r2, err := Build(cfg, BuildDeps{})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r2.Resolve(context.Background(), "summarizer")
	if err == nil {
		t.Fatal("expected NoBackendError when key env unset")
	}
}
