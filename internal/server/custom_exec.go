package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/standardbeagle/slop-mcp/internal/builtins"
	"github.com/standardbeagle/slop-mcp/internal/overrides"
	"github.com/standardbeagle/slop/pkg/slop"
)

// ErrCustomToolRecursion is returned when custom tool calls exceed the max depth.
var ErrCustomToolRecursion = errors.New("custom tool recursion depth exceeded (max 16)")

type customDepthCtxKey struct{}

func withCustomDepth(ctx context.Context) context.Context {
	return context.WithValue(ctx, customDepthCtxKey{}, customDepth(ctx)+1)
}

func customDepth(ctx context.Context) int {
	if v, ok := ctx.Value(customDepthCtxKey{}).(int); ok {
		return v
	}
	return 0
}

// validateArgsAgainstSchema performs minimal validation: required fields present
// and top-level type checks where schema.type is provided.
func validateArgsAgainstSchema(args map[string]any, schema map[string]any) error {
	if schema == nil {
		return nil
	}
	// Check required fields
	if req, ok := schema["required"]; ok {
		switch rv := req.(type) {
		case []any:
			for _, r := range rv {
				if name, ok := r.(string); ok {
					if _, present := args[name]; !present {
						return fmt.Errorf("required parameter %q is missing", name)
					}
				}
			}
		case []string:
			for _, name := range rv {
				if _, present := args[name]; !present {
					return fmt.Errorf("required parameter %q is missing", name)
				}
			}
		}
	}
	// Check top-level property types
	props, hasProp := schema["properties"].(map[string]any)
	if !hasProp {
		return nil
	}
	for name, val := range args {
		propDef, ok := props[name].(map[string]any)
		if !ok {
			continue
		}
		expectedType, ok := propDef["type"].(string)
		if !ok {
			continue
		}
		if err := checkType(name, val, expectedType); err != nil {
			return err
		}
	}
	return nil
}

func checkType(name string, val any, expectedType string) error {
	var ok bool
	switch expectedType {
	case "string":
		_, ok = val.(string)
	case "number":
		switch val.(type) {
		case int, int32, int64, float32, float64:
			ok = true
		}
	case "integer":
		switch v := val.(type) {
		case int, int32, int64:
			ok = true
		case float64:
			// Accept whole numbers from JSON (which unmarshals integers as float64)
			ok = v == math.Trunc(v) && !math.IsInf(v, 0)
		}
	case "boolean":
		_, ok = val.(bool)
	case "array":
		_, ok = val.([]any)
	case "object":
		_, ok = val.(map[string]any)
	default:
		return fmt.Errorf("parameter %q has unsupported schema type %q", name, expectedType)
	}
	if !ok {
		return fmt.Errorf("parameter %q must be of type %s", name, expectedType)
	}
	return nil
}

// executeCustomTool runs a custom tool's SLOP body with validated, bound args.
func (s *Server) executeCustomTool(ctx context.Context, ct overrides.CustomTool, args map[string]any) (any, error) {
	if customDepth(ctx) >= 16 {
		return nil, ErrCustomToolRecursion
	}
	ctx = withCustomDepth(ctx)

	if err := validateArgsAgainstSchema(args, ct.InputSchema); err != nil {
		return nil, fmt.Errorf("args: %w", err)
	}

	execCtx, cancel := context.WithTimeout(ctx, defaultSlopExecutionTimeout)
	defer cancel()

	// Hold the process-wide SLOP construct+execute lock across construction and
	// execution (see builtins.slopExecMu). Released exactly once when execution
	// truly ends, via executeSlopWithContext's worker.
	builtins.LockSlopExec()
	relOnce := sync.OnceFunc(builtins.UnlockSlopExec)
	// Release the exec lock if we panic before handing the runtime to
	// executeSlopWithContext's worker (which otherwise owns the release);
	// otherwise a construction/binding panic would deadlock all execution.
	handedOff := false
	defer func() {
		if !handedOff {
			relOnce()
		}
	}()

	// SLOP runtime with lazy, registry-backed MCP services (see newSlopRuntime).
	rt := s.newSlopRuntime(execCtx)
	defer rt.Close()

	// Bind `args` (full params map) and shorthand per-key bindings for non-reserved names.
	globals := rt.Context().Globals
	globals.Set("args", slop.GoToValue(args))
	for k, v := range args {
		if !builtins.IsReservedBuiltin(k) {
			globals.Set(k, slop.GoToValue(v))
		}
	}

	handedOff = true
	result, err := executeSlopWithContext(execCtx, rt, ct.Body, relOnce)
	if err != nil {
		return nil, parseSlopError(ct.Body, err)
	}

	return valueToAny(slop.ValueToGo(result)), nil
}
