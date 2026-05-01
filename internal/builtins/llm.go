package builtins

import (
	"context"
	"fmt"

	"github.com/standardbeagle/slop-mcp/internal/llm"
	"github.com/standardbeagle/slop/pkg/slop"
)

// ProgressFn receives streaming events from the llm() builtin.
// nil means no streaming surface; the loop still completes normally.
type ProgressFn func(llm.StreamEvent)

// RegisterLLM wires the llm() and llm_last() builtins into the SLOP runtime.
// host provides tool enumeration/execution for the agentic loop.
// progressFn is optional; when non-nil, stream events are forwarded to the
// caller (e.g. as MCP progress notifications or stderr in CLI mode).
func RegisterLLM(rt *slop.Runtime, r *llm.Registry, host llm.ToolHost, progressFn ProgressFn) {
	state := &llmState{}
	rt.RegisterBuiltin("llm", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		// context.Background() is used here because BuiltinFunction has no context
		// parameter. Real cancellation is handled at the run_slop handler level which
		// wraps the entire script execution in a deadline context.
		return state.callLLM(context.Background(), r, host, progressFn, args, kwargs)
	})
	rt.RegisterBuiltin("llm_last", func(_ []slop.Value, _ map[string]slop.Value) (slop.Value, error) {
		return state.lastValue(), nil
	})
}

type llmState struct {
	last *llm.LoopResult
}

func (s *llmState) callLLM(
	ctx context.Context,
	r *llm.Registry,
	host llm.ToolHost,
	progressFn ProgressFn,
	args []slop.Value,
	kwargs map[string]slop.Value,
) (slop.Value, error) {
	prompt := ""
	if len(args) > 0 {
		if sv, ok := args[0].(*slop.StringValue); ok {
			prompt = sv.Value
		}
	}

	agentName := stringKwarg(kwargs, "agent")
	def, prov, err := r.Resolve(ctx, agentName)
	if err != nil {
		return slop.NewErrorValue(err.Error()), nil
	}

	// Shallow copy so per-call kwarg overrides do not mutate the registered AgentDef.
	defCopy := *def

	// Apply per-call overrides onto the copy.
	if v := stringKwarg(kwargs, "system"); v != "" {
		defCopy.System = v
	}
	if v := stringKwarg(kwargs, "model"); v != "" {
		defCopy.Model = v
	}
	if v, ok := intKwarg(kwargs, "max_iters"); ok {
		defCopy.MaxIters = v
	}
	if v, ok := kwargs["tools"]; ok {
		defCopy.Tools = stringListFromSLOP(v)
	}

	req := &llm.Request{
		System: defCopy.System,
		Model:  defCopy.Model,
	}
	if v, ok := kwargs["temperature"]; ok {
		if nv, ok := v.(*slop.NumberValue); ok {
			t := nv.Value
			req.Temperature = &t
		}
	}
	if v, ok := intKwarg(kwargs, "max_tokens"); ok {
		req.MaxTokens = &v
	}

	// messages kwarg replaces the single-prompt default.
	if msgs, ok := kwargs["messages"]; ok {
		converted, err := messagesFromSLOP(msgs)
		if err != nil {
			return nil, err
		}
		req.Messages = converted
	} else {
		req.Messages = []llm.Message{{Role: llm.RoleUser, Content: prompt}}
	}

	res, err := llm.Run(ctx, prov, host, &defCopy, req, progressFn)
	if err != nil {
		s.last = res
		return slop.NewErrorValue(err.Error()), nil
	}
	s.last = res
	return slop.NewStringValue(res.Final.Content), nil
}

func (s *llmState) lastValue() slop.Value {
	if s.last == nil {
		return slop.NewNullValue()
	}
	m := slop.NewMapValue()
	m.Set("model", slop.NewStringValue(s.last.Final.Model))
	m.Set("provider", slop.NewStringValue(s.last.Provider))
	m.Set("iterations", slop.NewIntValue(int64(s.last.Iterations)))
	m.Set("stop_reason", slop.NewStringValue(string(s.last.Final.StopReason)))
	usage := slop.NewMapValue()
	usage.Set("input_tokens", slop.NewIntValue(int64(s.last.Usage.InputTokens)))
	usage.Set("output_tokens", slop.NewIntValue(int64(s.last.Usage.OutputTokens)))
	m.Set("usage", usage)
	return m
}

func stringKwarg(kw map[string]slop.Value, key string) string {
	v, ok := kw[key]
	if !ok {
		return ""
	}
	sv, ok := v.(*slop.StringValue)
	if !ok {
		return ""
	}
	return sv.Value
}

func intKwarg(kw map[string]slop.Value, key string) (int, bool) {
	v, ok := kw[key]
	if !ok {
		return 0, false
	}
	if iv, ok := v.(*slop.IntValue); ok {
		return int(iv.Value), true
	}
	if nv, ok := v.(*slop.NumberValue); ok {
		return int(nv.Value), true
	}
	return 0, false
}

func messagesFromSLOP(v slop.Value) ([]llm.Message, error) {
	lv, ok := v.(*slop.ListValue)
	if !ok {
		return nil, fmt.Errorf("messages: expected list")
	}
	out := make([]llm.Message, 0, len(lv.Elements))
	for i, el := range lv.Elements {
		mv, ok := el.(*slop.MapValue)
		if !ok {
			return nil, fmt.Errorf("messages[%d]: expected map", i)
		}
		var m llm.Message
		if r, ok := mv.Get("role"); ok {
			if s, ok := r.(*slop.StringValue); ok {
				m.Role = llm.Role(s.Value)
			}
		}
		if c, ok := mv.Get("content"); ok {
			if s, ok := c.(*slop.StringValue); ok {
				m.Content = s.Value
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func stringListFromSLOP(v slop.Value) []string {
	lv, ok := v.(*slop.ListValue)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(lv.Elements))
	for _, el := range lv.Elements {
		if s, ok := el.(*slop.StringValue); ok {
			out = append(out, s.Value)
		}
	}
	return out
}
