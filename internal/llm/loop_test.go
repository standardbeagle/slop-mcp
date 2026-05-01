package llm

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// scriptedProvider returns pre-baked responses one per call.
type scriptedProvider struct {
	calls    int
	answers  []*Response
	failures []error
}

func (s *scriptedProvider) Name() string                     { return "scripted" }
func (s *scriptedProvider) Available(_ context.Context) bool { return true }
func (s *scriptedProvider) UnavailableReason() string        { return "" }
func (s *scriptedProvider) Sample(_ context.Context, _ *Request, _ func(StreamEvent)) (*Response, error) {
	i := s.calls
	s.calls++
	if i < len(s.failures) && s.failures[i] != nil {
		return nil, s.failures[i]
	}
	if i >= len(s.answers) {
		return nil, errors.New("scripted provider exhausted")
	}
	return s.answers[i], nil
}

func TestLoop_SingleTurnNoTools(t *testing.T) {
	prov := &scriptedProvider{
		answers: []*Response{
			{Content: "hello", StopReason: StopReasonEndTurn, Model: "scripted"},
		},
	}
	host := &stubTools{}
	def := &AgentDef{Name: "x", MaxIters: 3}

	out, err := Run(context.Background(), prov, host, def, &Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Final.Content != "hello" {
		t.Fatalf("want content hello, got %q", out.Final.Content)
	}
	if out.Iterations != 1 {
		t.Fatalf("want 1 iteration, got %d", out.Iterations)
	}
}

func TestLoop_ToolUseThenEndTurn(t *testing.T) {
	prov := &scriptedProvider{
		answers: []*Response{
			{
				StopReason: StopReasonToolUse,
				ToolCalls: []ToolCall{
					{ID: "t1", Name: "echo", Arguments: json.RawMessage(`{"v":"hi"}`)},
				},
				Model: "scripted",
			},
			{Content: "done", StopReason: StopReasonEndTurn, Model: "scripted"},
		},
	}
	host := &stubTools{
		specs:   []ToolSpec{{Name: "echo"}},
		results: map[string]string{"echo": "hi"},
	}
	def := &AgentDef{Name: "x", MaxIters: 5}

	out, err := Run(context.Background(), prov, host, def, &Request{
		Messages: []Message{{Role: RoleUser, Content: "say hi via echo"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Iterations != 2 {
		t.Fatalf("want 2 iterations, got %d", out.Iterations)
	}
	if out.Final.Content != "done" {
		t.Fatalf("want done, got %q", out.Final.Content)
	}
	if _, called := host.calls["echo"]; !called {
		t.Fatal("echo tool was not executed")
	}
}

func TestLoop_MaxItersExceeded(t *testing.T) {
	// Provider always asks for a tool call; loop must halt at MaxIters.
	prov := &scriptedProvider{
		answers: []*Response{
			{StopReason: StopReasonToolUse, ToolCalls: []ToolCall{{ID: "t", Name: "echo"}}, Model: "scripted"},
			{StopReason: StopReasonToolUse, ToolCalls: []ToolCall{{ID: "t", Name: "echo"}}, Model: "scripted"},
			{StopReason: StopReasonToolUse, ToolCalls: []ToolCall{{ID: "t", Name: "echo"}}, Model: "scripted"},
		},
	}
	host := &stubTools{specs: []ToolSpec{{Name: "echo"}}, results: map[string]string{"echo": "x"}}
	def := &AgentDef{Name: "x", MaxIters: 2}

	_, err := Run(context.Background(), prov, host, def, &Request{
		Messages: []Message{{Role: RoleUser, Content: "loop"}},
	}, nil)
	if err == nil {
		t.Fatal("want max-iters error")
	}
	var mie *MaxItersExceededError
	if !errors.As(err, &mie) {
		t.Fatalf("want MaxItersExceededError, got %T", err)
	}
}
