package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/builtins"
	"github.com/standardbeagle/slop-mcp/internal/cli"
	"github.com/standardbeagle/slop/pkg/slop"
)

// newSlopRuntime builds a SLOP runtime with slop-mcp built-ins and one lazy,
// registry-backed service per configured MCP. No MCP is connected up front:
// a script that never calls an MCP spawns no subprocess, and a script that
// does gets routed through the registry's shared session (EnsureConnected
// performs the lazy connect), instead of a second per-runtime subprocess.
func (s *Server) newSlopRuntime(ctx context.Context) *slop.Runtime {
	rt := builtins.NewRuntimeWithConfig(slop.Config{
		MaxIterations: 100000,
		MaxDuration:   int64(defaultSlopExecutionTimeout / time.Second),
	})

	rt.RegisterBuiltin("print", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		parts := make([]string, len(args))
		for i, arg := range args {
			parts[i] = fmt.Sprint(slop.ValueToGo(arg))
		}
		fmt.Fprintln(os.Stderr, strings.Join(parts, " "))
		return slop.NewNullValue(), nil
	})

	// Register built-in functions.
	builtins.RegisterCrypto(rt)
	builtins.RegisterSlopSearch(rt)
	builtins.RegisterJWT(rt)
	builtins.RegisterTemplate(rt)

	// Thread-safe session store (overrides SLOP's default store_*).
	if s.sessionStore != nil {
		builtins.RegisterSession(rt, s.sessionStore)
	}

	// Persistent memory functions.
	if s.memoryStore != nil {
		builtins.RegisterMemory(rt, s.memoryStore)
	}

	// One lazy forwarding service per registered MCP -- including cached,
	// configured-but-unconnected, and errored ones: keeps every MCP name in
	// scope (so hyphenated identifiers still resolve) without connecting.
	// EnsureConnected dials on first use.
	for _, cfg := range s.registry.AllConfigs() {
		rt.RegisterExternalService(cfg.Name, &registrySlopService{
			server: s,
			ctx:    ctx,
			name:   cfg.Name,
		})
	}

	// CLI tools as a service (accessible as cli.tool_name() in scripts).
	if s.cliRegistry.Count() > 0 {
		rt.RegisterExternalService("cli", cli.NewSlopService(ctx, s.cliRegistry))
	}

	return rt
}

type slopExecutionResult struct {
	value slop.Value
	err   error
}

func executeSlopWithContext(ctx context.Context, rt *slop.Runtime, script string) (slop.Value, error) {
	// The buffered channel guarantees the worker can always send and exit even
	// after we return on timeout, so no goroutine leaks.
	done := make(chan slopExecutionResult, 1)
	go func() {
		value, err := rt.Execute(script)
		done <- slopExecutionResult{value: value, err: err}
	}()

	select {
	case result := <-done:
		return result.value, result.err
	case <-ctx.Done():
		// rt.Execute is not context-aware but self-terminates via the runtime's
		// MaxDuration (set equal to this timeout in newSlopRuntime), so the
		// worker stops shortly after. Do not Close here: the caller owns the
		// runtime's lifecycle via its deferred Close, and Close only tears down
		// the (empty) MCP manager — closing it twice or mid-Eval is pointless
		// and would race the deferred call.
		return nil, fmt.Errorf("SLOP execution canceled or timed out: %w", ctx.Err())
	}
}

// registrySlopService adapts a registered MCP to the SLOP Service interface,
// forwarding tool calls through the server's registry. The registry connects
// the MCP on first use (EnsureConnected), so merely registering this service
// costs nothing.
type registrySlopService struct {
	server *Server
	ctx    context.Context
	name   string
}

// Call forwards service.method(args, kwargs) to the registry-managed MCP.
func (m *registrySlopService) Call(method string, args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	arguments := buildMCPArguments(args, kwargs)

	result, err := m.server.registry.ExecuteToolRaw(m.ctx, m.name, method, arguments)
	if err != nil {
		// Warn (not Debug): a swallowed connect failure previously surfaced
		// later as a baffling 'unknown service' error inside the script.
		if m.server.logger != nil {
			m.server.logger.Warn("slop MCP tool call failed", "mcp", m.name, "tool", method, "error", err)
		}
		return nil, fmt.Errorf("MCP %q tool %q: %w", m.name, method, err)
	}

	if result.IsError {
		var errMsg string
		for _, content := range result.Content {
			if text, ok := content.(*mcp.TextContent); ok {
				errMsg += text.Text
			}
		}
		if errMsg == "" {
			errMsg = "tool returned error"
		}
		return nil, fmt.Errorf("MCP tool %s.%s error: %s", m.name, method, errMsg)
	}

	return resultToSlopValue(result), nil
}

// buildMCPArguments converts SLOP call args/kwargs into an MCP arguments map,
// mirroring the SLOP runtime's own MCP service semantics: kwargs pass through
// by name, positional args become arg0..argN, and a single positional arg
// with no kwargs is also exposed as "input".
func buildMCPArguments(args []slop.Value, kwargs map[string]slop.Value) map[string]any {
	arguments := make(map[string]any, len(kwargs)+len(args)+1)
	for k, v := range kwargs {
		arguments[k] = slop.ValueToGo(v)
	}
	for i, arg := range args {
		arguments[fmt.Sprintf("arg%d", i)] = slop.ValueToGo(arg)
	}
	if len(args) == 1 && len(kwargs) == 0 {
		arguments["input"] = slop.ValueToGo(args[0])
	}
	return arguments
}

// resultToSlopValue converts an MCP tool result to a SLOP value with the same
// semantics as the SLOP runtime's built-in MCP service: structured content is
// preferred, single text content is JSON-parsed when possible, and multiple
// content items become a list.
func resultToSlopValue(result *mcp.CallToolResult) slop.Value {
	if result.StructuredContent != nil {
		return slop.GoToValue(normalizeJSONValue(result.StructuredContent))
	}
	if len(result.Content) == 0 {
		return slop.NewNullValue()
	}
	if len(result.Content) == 1 {
		return contentItemToSlopValue(result.Content[0])
	}
	items := make([]slop.Value, 0, len(result.Content))
	for _, content := range result.Content {
		items = append(items, contentItemToSlopValue(content))
	}
	return slop.NewListValue(items)
}

func contentItemToSlopValue(content mcp.Content) slop.Value {
	switch c := content.(type) {
	case *mcp.TextContent:
		var parsed any
		if err := json.Unmarshal([]byte(c.Text), &parsed); err == nil {
			return slop.GoToValue(parsed)
		}
		return slop.NewStringValue(c.Text)
	case *mcp.ImageContent:
		return slop.GoToValue(map[string]any{
			"type":     "image",
			"mimeType": c.MIMEType,
			"data":     base64.StdEncoding.EncodeToString(c.Data),
		})
	case *mcp.AudioContent:
		return slop.GoToValue(map[string]any{
			"type":     "audio",
			"mimeType": c.MIMEType,
			"data":     base64.StdEncoding.EncodeToString(c.Data),
		})
	case *mcp.EmbeddedResource:
		m := map[string]any{"type": "resource"}
		if c.Resource != nil {
			m["uri"] = c.Resource.URI
			if c.Resource.Text != "" {
				m["text"] = c.Resource.Text
			}
			if c.Resource.MIMEType != "" {
				m["mimeType"] = c.Resource.MIMEType
			}
			if len(c.Resource.Blob) > 0 {
				m["blob"] = base64.StdEncoding.EncodeToString(c.Resource.Blob)
			}
		}
		return slop.GoToValue(m)
	case *mcp.ResourceLink:
		m := map[string]any{
			"type": "resourceLink",
			"uri":  c.URI,
		}
		if c.Name != "" {
			m["name"] = c.Name
		}
		return slop.GoToValue(m)
	default:
		// Unknown content type: marshal through the SDK's wire format so the
		// script gets structured data instead of a Go pointer rendering.
		if data, err := json.Marshal(content); err == nil {
			var parsed any
			if json.Unmarshal(data, &parsed) == nil {
				return slop.GoToValue(parsed)
			}
		}
		return slop.NewStringValue(fmt.Sprintf("%v", content))
	}
}

// normalizeJSONValue re-normalizes structured content through JSON so that
// arbitrary Go types (structs, typed maps) become the map[string]any / []any
// shapes that slop.GoToValue understands. Values already in those shapes are
// returned unchanged.
func normalizeJSONValue(v any) any {
	switch v.(type) {
	case nil, bool, string, float64, float32, int, int32, int64, map[string]any, []any:
		return v
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return fmt.Sprintf("%v", v)
	}
	return out
}
