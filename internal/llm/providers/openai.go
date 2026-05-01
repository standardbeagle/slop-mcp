// Package providers contains concrete LLM provider implementations.
package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

// OpenAIConfig configures an OpenAI-compatible HTTP provider.
// The same struct is reused by the Copilot provider (Task 10) which
// overrides AuthHeader, Available, and UnavailableReason while keeping
// the identical wire format.
type OpenAIConfig struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
	Name         string              // override; default "openai"
	HTTPClient   *http.Client        // optional; default http.DefaultClient
	AuthHeader   func(*http.Request) // optional; default Bearer APIKey

	// Available overrides the availability check. Default is APIKey != "".
	// Copilot supplies a TokenStore-backed closure so OAuth refresh state
	// drives availability instead of a static API key field.
	Available func(context.Context) bool

	// UnavailableReason returns the explanation when Available reports
	// false. Default is "<name>: api key empty". Empty when available.
	UnavailableReason func() string
}

// OpenAI is an OpenAI-compatible chat-completions provider.
// When Sample is called with a non-nil onEvent, it uses SSE streaming.
// Tool-call streaming is not supported in v1: if finish_reason=="tool_calls"
// arrives mid-stream, the stream is aborted and a second blocking request is
// made to retrieve the structured tool_calls reliably. This costs two HTTP
// round-trips but avoids the complexity of reassembling tool-call deltas.
type OpenAI struct {
	cfg OpenAIConfig
}

// NewOpenAI constructs an OpenAI provider with sensible defaults applied.
func NewOpenAI(cfg OpenAIConfig) *OpenAI {
	if cfg.Name == "" {
		cfg.Name = "openai"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.AuthHeader == nil {
		cfg.AuthHeader = func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		}
	}
	if cfg.Available == nil {
		cfg.Available = func(_ context.Context) bool { return cfg.APIKey != "" }
	}
	if cfg.UnavailableReason == nil {
		name := cfg.Name
		cfg.UnavailableReason = func() string {
			if cfg.APIKey != "" {
				return ""
			}
			return name + ": api key empty"
		}
	}
	return &OpenAI{cfg: cfg}
}

func (o *OpenAI) Name() string { return o.cfg.Name }

func (o *OpenAI) Available(ctx context.Context) bool {
	return o.cfg.Available(ctx)
}

func (o *OpenAI) UnavailableReason() string {
	return o.cfg.UnavailableReason()
}

// chatReq is the OpenAI chat/completions request body.
type chatReq struct {
	Model       string     `json:"model"`
	Messages    []chatMsg  `json:"messages"`
	Tools       []chatTool `json:"tools,omitempty"`
	Temperature *float64   `json:"temperature,omitempty"`
	MaxTokens   *int       `json:"max_tokens,omitempty"`
	Stop        []string   `json:"stop,omitempty"`
}

type chatMsg struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

type chatTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Role      string         `json:"role"`
			Content   *string        `json:"content"`
			ToolCalls []chatToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

// Sample issues a chat-completions request. If onEvent is non-nil, streams
// SSE deltas via onEvent and fires a final Done event. If onEvent is nil,
// uses the blocking (non-streaming) path.
func (o *OpenAI) Sample(ctx context.Context, req *llm.Request, onEvent func(llm.StreamEvent)) (*llm.Response, error) {
	if onEvent != nil {
		return o.sampleStreaming(ctx, req, onEvent)
	}
	return o.sampleBlocking(ctx, req)
}

// sampleBlocking issues a single blocking chat-completions request.
func (o *OpenAI) sampleBlocking(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	model := req.Model
	if model == "" {
		model = o.cfg.DefaultModel
	}

	body, err := json.Marshal(buildChatReq(model, req))
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	url := strings.TrimRight(o.cfg.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	o.cfg.AuthHeader(httpReq)

	httpResp, err := o.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", o.cfg.Name, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode/100 != 2 {
		buf, _ := io.ReadAll(io.LimitReader(httpResp.Body, 2048))
		snippet := strings.TrimSpace(string(buf))
		if len(snippet) > 512 {
			snippet = snippet[:512] + "…"
		}
		return nil, fmt.Errorf("%s: http %d: %s", o.cfg.Name, httpResp.StatusCode, snippet)
	}

	var raw chatResp
	if err := json.NewDecoder(httpResp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(raw.Choices) == 0 {
		return nil, fmt.Errorf("%s: empty choices", o.cfg.Name)
	}

	c := raw.Choices[0]
	out := &llm.Response{
		Model: raw.Model,
		Usage: llm.Usage{InputTokens: raw.Usage.PromptTokens, OutputTokens: raw.Usage.CompletionTokens},
	}
	if c.Message.Content != nil {
		out.Content = *c.Message.Content
	}
	for _, tc := range c.Message.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		})
	}
	out.StopReason = mapFinishReason(c.FinishReason)
	return out, nil
}

// sseChunk is the per-chunk shape from an OpenAI SSE stream.
type sseChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// sampleStreaming requests stream:true, parses SSE, dispatches deltas, and
// falls back to sampleBlocking if tool_calls are detected mid-stream (v1
// limitation — see struct-level comment).
func (o *OpenAI) sampleStreaming(ctx context.Context, req *llm.Request, onEvent func(llm.StreamEvent)) (*llm.Response, error) {
	model := req.Model
	if model == "" {
		model = o.cfg.DefaultModel
	}

	type streamReq struct {
		chatReq
		Stream bool `json:"stream"`
	}
	body, err := json.Marshal(streamReq{chatReq: buildChatReq(model, req), Stream: true})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	url := strings.TrimRight(o.cfg.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	o.cfg.AuthHeader(httpReq)

	httpResp, err := o.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", o.cfg.Name, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode/100 != 2 {
		buf, _ := io.ReadAll(io.LimitReader(httpResp.Body, 2048))
		snippet := strings.TrimSpace(string(buf))
		if len(snippet) > 512 {
			snippet = snippet[:512] + "…"
		}
		return nil, fmt.Errorf("%s: http %d: %s", o.cfg.Name, httpResp.StatusCode, snippet)
	}

	out := &llm.Response{}
	scanner := bufio.NewReader(httpResp.Body)
	toolCallsDetected := false

	for {
		line, err := scanner.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "data: [DONE]" {
			break
		}
		if strings.HasPrefix(line, "data: ") {
			payload := line[len("data: "):]
			var chunk sseChunk
			if jsonErr := json.Unmarshal([]byte(payload), &chunk); jsonErr != nil {
				// tolerate malformed chunks
				if err != nil {
					break
				}
				continue
			}
			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]
				if delta := choice.Delta.Content; delta != "" {
					out.Content += delta
					onEvent(llm.StreamEvent{Delta: delta})
				}
				if choice.FinishReason != nil {
					fr := *choice.FinishReason
					if fr == "tool_calls" {
						toolCallsDetected = true
						break
					}
					out.StopReason = mapFinishReason(fr)
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("%s: read stream: %w", o.cfg.Name, err)
		}
	}

	// Tool-call streaming: fall back to a blocking refetch to get structured
	// tool_calls. This costs two HTTP round-trips but avoids the complexity
	// of reassembling tool-call argument deltas (v1 known limitation).
	if toolCallsDetected {
		return o.sampleBlocking(ctx, req)
	}

	onEvent(llm.StreamEvent{Done: true, Final: out})
	return out, nil
}

// mapFinishReason converts an OpenAI finish_reason string to a StopReason.
func mapFinishReason(fr string) llm.StopReason {
	switch fr {
	case "tool_calls":
		return llm.StopReasonToolUse
	case "length":
		return llm.StopReasonMaxTokens
	case "stop":
		return llm.StopReasonEndTurn
	default:
		return llm.StopReason(fr)
	}
}

func buildChatReq(model string, in *llm.Request) chatReq {
	msgs := make([]chatMsg, 0, len(in.Messages)+1)
	if in.System != "" {
		msgs = append(msgs, chatMsg{Role: "system", Content: in.System})
	}
	for _, m := range in.Messages {
		out := chatMsg{Role: string(m.Role), Content: m.Content, ToolCallID: m.ToolCallID, Name: m.Name}
		for _, tc := range m.ToolCalls {
			ctc := chatToolCall{ID: tc.ID, Type: "function"}
			ctc.Function.Name = tc.Name
			ctc.Function.Arguments = string(tc.Arguments)
			out.ToolCalls = append(out.ToolCalls, ctc)
		}
		msgs = append(msgs, out)
	}
	tools := make([]chatTool, 0, len(in.Tools))
	for _, t := range in.Tools {
		ct := chatTool{Type: "function"}
		ct.Function.Name = t.Name
		ct.Function.Description = t.Description
		ct.Function.Parameters = t.Schema
		tools = append(tools, ct)
	}
	return chatReq{
		Model: model, Messages: msgs, Tools: tools,
		Temperature: in.Temperature, MaxTokens: in.MaxTokens, Stop: in.Stop,
	}
}
