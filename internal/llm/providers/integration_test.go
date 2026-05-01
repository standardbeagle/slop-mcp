//go:build integration

package providers

import (
	"context"
	"os"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

func TestIntegration_OpenAIRoundTrip(t *testing.T) {
	if os.Getenv("LLM_INTEGRATION_TEST") != "1" {
		t.Skip("set LLM_INTEGRATION_TEST=1 to run")
	}
	url := os.Getenv("LLM_TEST_URL")
	if url == "" {
		url = "http://localhost:11434/v1" // ollama default
	}
	model := os.Getenv("LLM_TEST_MODEL")
	if model == "" {
		model = "llama3.2"
	}

	p := NewOpenAI(OpenAIConfig{BaseURL: url, APIKey: "ollama", DefaultModel: model})
	resp, err := p.Sample(context.Background(), &llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Say only the word PONG."}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("response: %q (model=%s)", resp.Content, resp.Model)
	if resp.Content == "" {
		t.Fatal("empty response from real endpoint")
	}
}
