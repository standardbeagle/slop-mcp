package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/standardbeagle/slop-mcp/internal/builtins"
	"github.com/standardbeagle/slop-mcp/internal/cli"
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

// reservedShorthandNames lists names that must not be bound as $<name> shorthands
// because they collide with SLOP builtins or slop-mcp injected functions.
// TODO(task-10): replace with builtins.IsReservedBuiltin once that package ships.
var reservedShorthandNames = map[string]bool{
	"args":           true,
	"mem_save":       true,
	"mem_load":       true,
	"mem_list":       true,
	"mem_search":     true,
	"mem_info":       true,
	"mem_delete":     true,
	"store_set":      true,
	"store_get":      true,
	"store_list":     true,
	"execute_tool":   true,
	"emit":           true,
	"map":            true,
	"filter":         true,
	"reduce":         true,
	"len":            true,
	"json_parse":     true,
	"json_stringify": true,
	"http_get":       true,
	"http_post":      true,
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
		switch val.(type) {
		case int, int32, int64:
			ok = true
		}
	case "boolean":
		_, ok = val.(bool)
	case "array":
		_, ok = val.([]any)
	case "object":
		_, ok = val.(map[string]any)
	default:
		ok = true // unknown type — pass through
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

	rt := slop.NewRuntime()
	defer rt.Close()

	// Register built-ins matching handleRunSlop.
	builtins.RegisterCrypto(rt)
	builtins.RegisterSlopSearch(rt)
	builtins.RegisterJWT(rt)
	builtins.RegisterTemplate(rt)
	if s.sessionStore != nil {
		builtins.RegisterSession(rt, s.sessionStore)
	}
	if s.memoryStore != nil {
		builtins.RegisterMemory(rt, s.memoryStore)
	}

	// Connect MCPs.
	for _, cfg := range s.registry.GetConfigs() {
		transportType := cfg.Type
		if transportType == "stdio" {
			transportType = "command"
		}
		slopCfg := slop.MCPConfig{
			Name:    cfg.Name,
			Type:    transportType,
			Command: cfg.Command,
			Args:    cfg.Args,
			Env:     mapToSlice(cfg.Env),
			URL:     cfg.URL,
			Headers: cfg.Headers,
		}
		if err := rt.ConnectMCP(ctx, slopCfg); err != nil {
			s.logger.Debug("custom_tool: failed to connect MCP", "mcp", cfg.Name, "err", err)
		}
	}

	// Register CLI tools.
	if s.cliRegistry.Count() > 0 {
		cliService := cli.NewSlopService(ctx, s.cliRegistry)
		rt.RegisterExternalService("cli", cliService)
	}

	// Bind `args` (full params map) and shorthand per-key bindings for non-reserved names.
	globals := rt.Context().Globals
	globals.Set("args", slop.GoToValue(args))
	for k, v := range args {
		if !reservedShorthandNames[k] {
			globals.Set(k, slop.GoToValue(v))
		}
	}

	result, err := rt.Execute(ct.Body)
	if err != nil {
		return nil, parseSlopError(ct.Body, err)
	}

	return valueToAny(slop.ValueToGo(result)), nil
}
