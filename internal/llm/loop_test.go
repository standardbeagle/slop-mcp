package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
	if mie.Iters != 2 {
		t.Fatalf("want Iters=2, got %d", mie.Iters)
	}
}

func TestLoop_PropagatesProviderError(t *testing.T) {
	prov := &scriptedProvider{
		failures: []error{nil, errors.New("network exploded")},
		answers: []*Response{
			{StopReason: StopReasonToolUse, ToolCalls: []ToolCall{{ID: "t", Name: "echo"}}, Model: "scripted"},
		},
	}
	host := &stubTools{specs: []ToolSpec{{Name: "echo"}}, results: map[string]string{"echo": "ok"}}
	def := &AgentDef{Name: "x", MaxIters: 5}

	out, err := Run(context.Background(), prov, host, def, &Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, nil)
	if err == nil || err.Error() != "network exploded" {
		t.Fatalf("want network error propagation, got %v", err)
	}
	if out.Iterations != 1 {
		t.Fatalf("want 1 completed iteration before failure, got %d", out.Iterations)
	}
}

func TestLoop_AbortsOnInfrastructureToolError(t *testing.T) {
	prov := &scriptedProvider{
		answers: []*Response{
			{StopReason: StopReasonToolUse, ToolCalls: []ToolCall{{ID: "t", Name: "missing"}}, Model: "scripted"},
		},
	}
	host := &stubTools{specs: []ToolSpec{{Name: "echo"}}} // "missing" not registered → infrastructure error
	def := &AgentDef{Name: "x", MaxIters: 5}

	_, err := Run(context.Background(), prov, host, def, &Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, nil)
	if err == nil {
		t.Fatal("want infrastructure error abort")
	}
	if !strings.Contains(err.Error(), `tool "missing" execute`) {
		t.Fatalf("error should wrap tool name, got %q", err.Error())
	}
}

func TestLoop_FeedsBackIsErrorAndContinues(t *testing.T) {
	prov := &scriptedProvider{
		answers: []*Response{
			{StopReason: StopReasonToolUse, ToolCalls: []ToolCall{{ID: "t", Name: "broken"}}, Model: "scripted"},
			{Content: "recovered", StopReason: StopReasonEndTurn, Model: "scripted"},
		},
	}
	host := &stubTools{
		specs: []ToolSpec{{Name: "broken"}},
		errs:  map[string]error{"broken": errors.New("tool said no")},
	}
	def := &AgentDef{Name: "x", MaxIters: 5}

	req := &Request{Messages: []Message{{Role: RoleUser, Content: "try broken"}}}
	out, err := Run(context.Background(), prov, host, def, req, nil)
	if err != nil {
		t.Fatalf("want successful continuation, got %v", err)
	}
	if out.Final.Content != "recovered" {
		t.Fatalf("want final content 'recovered', got %q", out.Final.Content)
	}
	// Verify the tool message in req.Messages carries IsError=true.
	var sawErrToolMsg bool
	for _, m := range req.Messages {
		if m.Role == RoleTool && m.IsError {
			sawErrToolMsg = true
			break
		}
	}
	if !sawErrToolMsg {
		t.Fatal("expected RoleTool message with IsError=true in conversation")
	}
}

func TestLoop_ExecutesAllToolCallsInTurn(t *testing.T) {
	prov := &scriptedProvider{
		answers: []*Response{
			{
				StopReason: StopReasonToolUse,
				ToolCalls: []ToolCall{
					{ID: "1", Name: "a"},
					{ID: "2", Name: "b"},
					{ID: "3", Name: "c"},
				},
				Model: "scripted",
			},
			{Content: "done", StopReason: StopReasonEndTurn, Model: "scripted"},
		},
	}
	host := &stubTools{
		specs:   []ToolSpec{{Name: "a"}, {Name: "b"}, {Name: "c"}},
		results: map[string]string{"a": "ra", "b": "rb", "c": "rc"},
	}
	def := &AgentDef{Name: "x", MaxIters: 5}

	req := &Request{Messages: []Message{{Role: RoleUser, Content: "all three"}}}
	_, err := Run(context.Background(), prov, host, def, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a", "b", "c"} {
		if _, called := host.calls[n]; !called {
			t.Errorf("tool %q not executed", n)
		}
	}
	// Tool-result messages should appear in the same order they were called.
	var toolNamesInOrder []string
	for _, m := range req.Messages {
		if m.Role == RoleTool {
			toolNamesInOrder = append(toolNamesInOrder, m.Name)
		}
	}
	want := []string{"a", "b", "c"}
	if len(toolNamesInOrder) != 3 ||
		toolNamesInOrder[0] != want[0] ||
		toolNamesInOrder[1] != want[1] ||
		toolNamesInOrder[2] != want[2] {
		t.Fatalf("tool message order mismatch: got %v want %v", toolNamesInOrder, want)
	}
}
