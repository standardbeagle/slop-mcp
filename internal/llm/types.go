// Package llm defines the LLM provider abstraction used by the SLOP llm() builtin.
package llm

import "encoding/json"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type StopReason string

const (
	StopReasonEndTurn      StopReason = "end_turn"
	StopReasonToolUse      StopReason = "tool_use"
	StopReasonMaxTokens    StopReason = "max_tokens"
	StopReasonStopSequence StopReason = "stop_sequence"
)

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
}

type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
	IsError    bool       `json:"is_error,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Request struct {
	Messages    []Message
	System      string
	Model       string
	Tools       []ToolSpec
	Temperature *float64
	MaxTokens   *int
	Stop        []string
}

type Response struct {
	Content    string
	ToolCalls  []ToolCall
	StopReason StopReason
	Usage      Usage
	Model      string
}

type StreamEvent struct {
	Delta    string
	ToolCall *ToolCall
	Done     bool
	Final    *Response
}
