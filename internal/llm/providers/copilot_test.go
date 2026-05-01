package providers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

type fakeTokenStore struct {
	tok string
	err error
}

func (f *fakeTokenStore) Token() (string, error) { return f.tok, f.err }

func TestCopilot_InjectsOAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer copilot-oauth-tok") {
			t.Errorf("auth header: %q", got)
		}
		if r.Header.Get("Editor-Version") == "" {
			t.Error("missing Editor-Version header")
		}
		if r.Header.Get("Copilot-Integration-Id") == "" {
			t.Error("missing Copilot-Integration-Id header")
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	p := NewCopilot(CopilotConfig{
		BaseURL:      srv.URL,
		DefaultModel: "gpt-4o",
		TokenStore:   &fakeTokenStore{tok: "copilot-oauth-tok"},
	})
	resp, err := p.Sample(context.Background(), &llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "ok" {
		t.Fatalf("want ok, got %q", resp.Content)
	}
}

func TestCopilot_AvailableRequiresToken(t *testing.T) {
	p := NewCopilot(CopilotConfig{TokenStore: &fakeTokenStore{tok: ""}})
	if p.Available(context.Background()) {
		t.Fatal("must be unavailable when token empty")
	}
	p2 := NewCopilot(CopilotConfig{TokenStore: &fakeTokenStore{tok: "x"}})
	if !p2.Available(context.Background()) {
		t.Fatal("must be available with token")
	}
}

func TestCopilot_UnavailableReason(t *testing.T) {
	p := NewCopilot(CopilotConfig{TokenStore: &fakeTokenStore{tok: ""}})
	reason := p.UnavailableReason()
	if reason == "" {
		t.Fatal("UnavailableReason should be non-empty when token missing")
	}
}

func TestCopilot_NameIsCopilot(t *testing.T) {
	p := NewCopilot(CopilotConfig{TokenStore: &fakeTokenStore{tok: "x"}})
	if p.Name() != "copilot" {
		t.Fatalf("want copilot, got %q", p.Name())
	}
}
