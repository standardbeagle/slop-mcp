package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/registry"
)

// newLLMHostServer builds a minimal Server with two test tools registered.
func newLLMHostServer(t *testing.T) *Server {
	t.Helper()
	tools := []registry.ToolInfo{
		{
			Name:        "echo",
			Description: "Echo the input",
			MCPName:     "test-mcp",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string"},
				},
			},
		},
		{
			Name:        "add",
			Description: "Add two numbers",
			MCPName:     "test-mcp",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"a": map[string]any{"type": "number"},
					"b": map[string]any{"type": "number"},
				},
			},
		},
	}
	return mockServer(tools)
}

func TestLLMHost_ListReturnsToolSpecs(t *testing.T) {
	s := newLLMHostServer(t)
	host := NewLLMHost(s)

	specs := host.List(context.Background(), nil)

	if len(specs) == 0 {
		t.Fatal("expected at least one ToolSpec, got none")
	}
	for _, sp := range specs {
		if sp.Name == "" {
			t.Fatalf("spec missing name: %+v", sp)
		}
		if len(sp.Schema) == 0 {
			t.Fatalf("spec %q missing schema", sp.Name)
		}
		// Schema must be valid JSON.
		var raw map[string]any
		if err := json.Unmarshal(sp.Schema, &raw); err != nil {
			t.Fatalf("spec %q schema is not valid JSON: %v", sp.Name, err)
		}
	}
}

func TestLLMHost_ListQualifiedNames(t *testing.T) {
	s := newLLMHostServer(t)
	host := NewLLMHost(s)

	specs := host.List(context.Background(), nil)

	for _, sp := range specs {
		mcpName, toolName, err := parseQualifiedName(sp.Name)
		if err != nil {
			t.Fatalf("spec name %q is not qualified: %v", sp.Name, err)
		}
		if mcpName == "" || toolName == "" {
			t.Fatalf("parseQualifiedName(%q) returned empty parts", sp.Name)
		}
	}
}

func TestLLMHost_ListAllowFiltering(t *testing.T) {
	s := newLLMHostServer(t)
	host := NewLLMHost(s)

	all := host.List(context.Background(), nil)
	if len(all) < 2 {
		t.Skip("not enough tools to test filtering")
	}

	allow := []string{all[0].Name}
	filtered := host.List(context.Background(), allow)

	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered spec, got %d: %+v", len(filtered), filtered)
	}
	if filtered[0].Name != allow[0] {
		t.Fatalf("filter returned wrong tool: want %q, got %q", allow[0], filtered[0].Name)
	}
}

func TestLLMHost_ListEmptyAllowReturnsAll(t *testing.T) {
	s := newLLMHostServer(t)
	host := NewLLMHost(s)

	all := host.List(context.Background(), nil)
	explicit := host.List(context.Background(), []string{})

	if len(all) != len(explicit) {
		t.Fatalf("nil allow and empty allow should return same count: %d vs %d", len(all), len(explicit))
	}
}

func TestParseQualifiedName(t *testing.T) {
	cases := []struct {
		input   string
		wantMCP string
		wantTool string
		wantErr bool
	}{
		{"test-mcp.echo", "test-mcp", "echo", false},
		{"mcp.tool.with.dots", "mcp", "tool.with.dots", false},
		{"notqualified", "", "", true},
		{".tool", "", "", true},
		{"mcp.", "", "", true},
	}

	for _, tc := range cases {
		mcpName, toolName, err := parseQualifiedName(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseQualifiedName(%q): expected error, got (%q, %q, nil)", tc.input, mcpName, toolName)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseQualifiedName(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if mcpName != tc.wantMCP || toolName != tc.wantTool {
			t.Errorf("parseQualifiedName(%q): got (%q, %q), want (%q, %q)",
				tc.input, mcpName, toolName, tc.wantMCP, tc.wantTool)
		}
	}
}

func TestLLMHost_ExecuteUnknownToolReturnsError(t *testing.T) {
	s := newLLMHostServer(t)
	host := NewLLMHost(s)

	_, _, err := host.Execute(context.Background(), "notqualified", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected infrastructure error for malformed tool name, got nil")
	}
}

func TestLLMHost_ExecuteUnknownMCPReturnsError(t *testing.T) {
	s := newLLMHostServer(t)
	host := NewLLMHost(s)

	// MCP "ghost" is not registered, so this should return ("", false, err).
	result, isErr, err := host.Execute(context.Background(), "ghost.tool", json.RawMessage(`{}`))
	if err == nil {
		t.Fatalf("expected error for unknown MCP; got result=%q isErr=%v", result, isErr)
	}
	if isErr {
		t.Fatal("infrastructure failure must return isError=false")
	}
}

func TestServer_LLMRegistryConstructed(t *testing.T) {
	s, err := NewFromConfig(config.NewConfig())
	if err != nil {
		t.Fatalf("NewFromConfig returned error: %v", err)
	}
	defer s.Close()
	if s.llmRegistry == nil {
		t.Fatal("server should construct llmRegistry during init")
	}
}
