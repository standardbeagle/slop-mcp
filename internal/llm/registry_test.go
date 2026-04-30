package llm

import (
	"context"
	"testing"
)

type fakeProvider struct {
	name      string
	available bool
}

func (f *fakeProvider) Name() string                 { return f.name }
func (f *fakeProvider) Available(_ context.Context) bool { return f.available }
func (f *fakeProvider) UnavailableReason() string {
	if f.available {
		return ""
	}
	return "fake unavailable"
}
func (f *fakeProvider) Sample(_ context.Context, _ *Request, _ func(StreamEvent)) (*Response, error) {
	return &Response{Content: "ok", StopReason: StopReasonEndTurn, Model: "fake"}, nil
}

func TestRegistry_RegisterAndResolveByAgentBackend(t *testing.T) {
	r := NewRegistry()
	r.RegisterProvider(&fakeProvider{name: "openai", available: true})
	r.SetChain([]string{"openai"})
	r.RegisterAgent(&AgentDef{Name: "summarizer", Backend: "openai", Model: "gpt-4o-mini"})

	def, prov, err := r.Resolve(context.Background(), "summarizer")
	if err != nil {
		t.Fatal(err)
	}
	if def.Name != "summarizer" {
		t.Fatalf("want agent summarizer, got %q", def.Name)
	}
	if prov.Name() != "openai" {
		t.Fatalf("want provider openai, got %q", prov.Name())
	}
}

func TestRegistry_ResolveSyntheticDefault(t *testing.T) {
	r := NewRegistry()
	r.RegisterProvider(&fakeProvider{name: "openai", available: true})
	r.SetChain([]string{"openai"})

	def, prov, err := r.Resolve(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if def.MaxIters != defaultMaxIters {
		t.Fatalf("default max_iters: want %d, got %d", defaultMaxIters, def.MaxIters)
	}
	if prov.Name() != "openai" {
		t.Fatalf("want openai, got %q", prov.Name())
	}
}

func TestRegistry_AgentNotFound(t *testing.T) {
	r := NewRegistry()
	r.RegisterAgent(&AgentDef{Name: "summarizer"})
	r.RegisterAgent(&AgentDef{Name: "reviewer"})

	_, _, err := r.Resolve(context.Background(), "summarize")
	if err == nil {
		t.Fatal("want error for unknown agent")
	}
	// Error message must list candidates
	msg := err.Error()
	if !contains(msg, "summarizer") || !contains(msg, "reviewer") {
		t.Fatalf("error must list known agents, got %q", msg)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
