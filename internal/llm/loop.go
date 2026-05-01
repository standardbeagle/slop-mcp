package llm

import (
	"context"
	"fmt"
)

// LoopResult captures everything the builtin needs to surface back to
// SLOP and to populate llm_last().
type LoopResult struct {
	Final      *Response
	Iterations int
	Provider   string
	Usage      Usage
}

type MaxItersExceededError struct {
	Iters int
}

func (e *MaxItersExceededError) Error() string {
	return fmt.Sprintf("max_iters exceeded (%d)", e.Iters)
}

// Run executes the agentic tool-use loop against a single provider.
//
// Caller responsibilities:
//   - Resolve the provider via Registry first.
//   - Pre-populate req.Messages with the user input. The loop appends
//     assistant and tool messages to req.Messages in place across
//     iterations — callers should not reuse the same Request after
//     a Run call without resetting Messages.
//
// Run sets req.Tools by calling host.List(ctx, def.Tools).
func Run(
	ctx context.Context,
	prov Provider,
	host ToolHost,
	def *AgentDef,
	req *Request,
	onEvent func(StreamEvent),
) (*LoopResult, error) {
	maxIters := def.MaxIters
	if maxIters <= 0 {
		maxIters = defaultMaxIters
	}

	req.Tools = host.List(ctx, def.Tools)

	out := &LoopResult{Provider: prov.Name()}

	for i := 0; i < maxIters; i++ {
		resp, err := prov.Sample(ctx, req, onEvent)
		if err != nil {
			return out, err
		}
		out.Iterations++
		out.Final = resp
		out.Usage.InputTokens += resp.Usage.InputTokens
		out.Usage.OutputTokens += resp.Usage.OutputTokens

		if resp.StopReason != StopReasonToolUse {
			return out, nil
		}

		// Append assistant turn carrying the tool calls.
		req.Messages = append(req.Messages, Message{
			Role:      RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute each tool call sequentially, append results.
		for _, tc := range resp.ToolCalls {
			result, isErr, execErr := host.Execute(ctx, tc.Name, tc.Arguments)
			if execErr != nil {
				// Infrastructure failure — abort.
				return out, fmt.Errorf("tool %q execute: %w", tc.Name, execErr)
			}
			req.Messages = append(req.Messages, Message{
				Role:       RoleTool,
				Name:       tc.Name,
				ToolCallID: tc.ID,
				Content:    result,
				IsError:    isErr,
			})
		}
	}

	return out, &MaxItersExceededError{Iters: maxIters}
}
