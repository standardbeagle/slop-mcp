package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

func TestOpenAI_SimpleCompletion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer test-key") {
			t.Errorf("missing auth header: %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"hello"`) {
			t.Errorf("body missing prompt: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {"role": "assistant", "content": "world"},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 5, "completion_tokens": 1},
			"model": "gpt-test"
		}`))
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{
		BaseURL:      srv.URL,
		APIKey:       "test-key",
		DefaultModel: "gpt-test",
	})

	resp, err := p.Sample(context.Background(), &llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "world" {
		t.Fatalf("want world, got %q", resp.Content)
	}
	if resp.StopReason != llm.StopReasonEndTurn {
		t.Fatalf("want end_turn, got %q", resp.StopReason)
	}
}

func TestOpenAI_ToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": null,
					"tool_calls": [{
						"id": "call_1",
						"type": "function",
						"function": {"name": "echo", "arguments": "{\"v\":\"hi\"}"}
					}]
				},
				"finish_reason": "tool_calls"
			}],
			"model": "gpt-test"
		}`))
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL, APIKey: "k", DefaultModel: "gpt-test"})
	resp, err := p.Sample(context.Background(), &llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "use echo"}},
		Tools:    []llm.ToolSpec{{Name: "echo", Schema: json.RawMessage(`{"type":"object"}`)}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != llm.StopReasonToolUse {
		t.Fatalf("want tool_use, got %q", resp.StopReason)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "echo" {
		t.Fatalf("unexpected tool calls: %+v", resp.ToolCalls)
	}
}

func TestOpenAI_AvailableRequiresKey(t *testing.T) {
	noKey := NewOpenAI(OpenAIConfig{BaseURL: "http://x", DefaultModel: "m"})
	if noKey.Available(context.Background()) {
		t.Fatal("provider must report unavailable when API key empty")
	}
	withKey := NewOpenAI(OpenAIConfig{BaseURL: "http://x", APIKey: "k", DefaultModel: "m"})
	if !withKey.Available(context.Background()) {
		t.Fatal("provider must report available with key set")
	}
}

func TestOpenAI_UnavailableReason(t *testing.T) {
	p := NewOpenAI(OpenAIConfig{BaseURL: "http://x", DefaultModel: "m"})
	if reason := p.UnavailableReason(); reason == "" {
		t.Fatal("UnavailableReason should return non-empty when API key missing")
	}
	p2 := NewOpenAI(OpenAIConfig{BaseURL: "http://x", APIKey: "k", DefaultModel: "m"})
	if reason := p2.UnavailableReason(); reason != "" {
		t.Fatalf("UnavailableReason should be empty when available, got %q", reason)
	}
}

func TestOpenAI_Streaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}, \"finish_reason\":\"stop\"}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL, APIKey: "k", DefaultModel: "m"})
	var collected string
	resp, err := p.Sample(context.Background(), &llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	}, func(ev llm.StreamEvent) {
		collected += ev.Delta
	})
	if err != nil {
		t.Fatal(err)
	}
	if collected != "hello" {
		t.Fatalf("collected deltas: %q", collected)
	}
	if resp.Content != "hello" {
		t.Fatalf("final content: %q", resp.Content)
	}
}
