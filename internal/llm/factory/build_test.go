package factory

import (
	"context"
	"strings"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/config"
)

func makeCfg() *config.Config {
	return &config.Config{
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
}

func TestBuild_RegistersProvidersAndAgents(t *testing.T) {
	t.Setenv("FOO_KEY", "test-key")
	r, err := Build(makeCfg(), BuildDeps{})
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
}

func TestBuild_NoBackendWhenKeyMissing(t *testing.T) {
	// FOO_KEY intentionally unset for this test. t.Setenv elsewhere uses
	// per-test cleanup so envs do not leak across tests.
	r, err := Build(makeCfg(), BuildDeps{})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r.Resolve(context.Background(), "summarizer")
	if err == nil {
		t.Fatal("expected NoBackendError when key env unset")
	}
}

func TestBuild_UnknownProviderNameErrors(t *testing.T) {
	cfg := &config.Config{
		LLM: &config.LLMConfig{
			Chain: []string{"opeani"},
			Providers: map[string]*config.LLMProviderConfig{
				"opeani": {DefaultModel: "m"}, // typo of "openai"
			},
		},
	}
	_, err := Build(cfg, BuildDeps{})
	if err == nil {
		t.Fatal("Build should reject unknown provider names")
	}
	if !strings.Contains(err.Error(), `"opeani"`) {
		t.Fatalf("error should mention the unrecognized name, got %q", err.Error())
	}
}
