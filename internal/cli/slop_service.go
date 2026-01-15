package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/standardbeagle/slop/pkg/slop"
)

// SlopService wraps a CLI Registry to implement the slop.Service interface.
// This allows CLI tools to be called from SLOP scripts as: cli.tool_name(args...)
type SlopService struct {
	registry *Registry
	ctx      context.Context
}

// NewSlopService creates a new SLOP service that wraps CLI tools.
func NewSlopService(ctx context.Context, registry *Registry) *SlopService {
	return &SlopService{
		registry: registry,
		ctx:      ctx,
	}
}

// Call implements slop.Service interface.
// method is the CLI tool name (without cli_ prefix).
// args are positional arguments (rarely used for CLI tools).
// kwargs are named parameters matching the tool's schema.
func (s *SlopService) Call(method string, args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	// Convert kwargs to map[string]any
	params := make(map[string]any)
	for k, v := range kwargs {
		params[k] = slop.ValueToGo(v)
	}

	// Handle positional args (if the tool accepts them)
	// For memory tools, positional args could be: bank, key, value
	if len(args) > 0 {
		tool := s.registry.Get(method)
		if tool != nil {
			// Map positional args to their named positions
			for i, arg := range args {
				for _, toolArg := range tool.Args {
					if toolArg.Position == i {
						params[toolArg.Name] = slop.ValueToGo(arg)
						break
					}
				}
			}
		}
	}

	// Execute the CLI tool
	result, err := s.registry.Execute(s.ctx, method, params)
	if err != nil {
		return slop.NewErrorValue(fmt.Sprintf("CLI tool %s failed: %v", method, err)), nil
	}

	// Parse output as JSON if possible
	var parsed any
	if err := json.Unmarshal([]byte(result.Stdout), &parsed); err == nil {
		return slop.GoToValue(parsed), nil
	}

	// Return as string if not JSON
	return slop.NewStringValue(result.Stdout), nil
}
