// Package providers contains concrete LLM provider implementations.
package providers

import (
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
// Streaming is not yet implemented (Task 16); onEvent is ignored.
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

// Sample issues a blocking chat-completions request. onEvent is accepted for
// interface compliance but ignored until Task 16 adds streaming support.
func (o *OpenAI) Sample(ctx context.Context, req *llm.Request, _ func(llm.StreamEvent)) (*llm.Response, error) {
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
	switch c.FinishReason {
	case "tool_calls":
		out.StopReason = llm.StopReasonToolUse
	case "length":
		out.StopReason = llm.StopReasonMaxTokens
	case "stop":
		out.StopReason = llm.StopReasonEndTurn
	default:
		out.StopReason = llm.StopReason(c.FinishReason)
	}
	return out, nil
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
