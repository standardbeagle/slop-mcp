package server

import (
	"context"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/overrides"
	"github.com/standardbeagle/slop-mcp/internal/registry"
	"github.com/standardbeagle/slop/pkg/slop"
	"github.com/stretchr/testify/require"
)

// markerMCPConfig returns an MCP config whose command creates markerPath when
// spawned. If any code path eagerly connects this MCP, the marker appears.
func markerMCPConfig(t *testing.T, name string) (config.MCPConfig, string) {
	t.Helper()
	if goruntime.GOOS == "windows" {
		t.Skip("marker test requires /bin/sh")
	}
	markerPath := filepath.Join(t.TempDir(), "spawned")
	return config.MCPConfig{
		Name:    name,
		Type:    "stdio",
		Command: "/bin/sh",
		Args:    []string{"-c", "touch " + markerPath + "; cat >/dev/null"},
	}, markerPath
}

// TestRunSlop_PureScript_DoesNotConnectMCPs is the core regression test for
// the eager fan-out: a script that never references an MCP must not spawn its
// subprocess.
func TestRunSlop_PureScript_DoesNotConnectMCPs(t *testing.T) {
	s := mockServer(nil)
	cfg, markerPath := markerMCPConfig(t, "marker-mcp")
	s.registry.SetConfigured(cfg)

	_, out, err := s.handleRunSlop(context.Background(), &mcp.CallToolRequest{}, RunSlopInput{
		Script: "1 + 2",
	})
	require.NoError(t, err)
	require.EqualValues(t, 3, extractSlopValue(out.Result))

	_, statErr := os.Stat(markerPath)
	require.True(t, os.IsNotExist(statErr),
		"run_slop of a pure script must not spawn subprocesses for configured MCPs")
	require.Equal(t, registry.StateConfigured, s.registry.GetState("marker-mcp"))
}

// TestExecuteCustomTool_PureBody_DoesNotConnectMCPs mirrors the run_slop
// regression test for the custom tool execution path.
func TestExecuteCustomTool_PureBody_DoesNotConnectMCPs(t *testing.T) {
	s := mockServer(nil)
	cfg, markerPath := markerMCPConfig(t, "marker-mcp")
	s.registry.SetConfigured(cfg)

	ct := overrides.CustomTool{
		Description: "adds numbers",
		InputSchema: map[string]any{"type": "object"},
		Body:        "2 + 3",
	}
	result, err := s.executeCustomTool(context.Background(), ct, map[string]any{})
	require.NoError(t, err)
	require.EqualValues(t, 5, extractSlopValue(result))

	_, statErr := os.Stat(markerPath)
	require.True(t, os.IsNotExist(statErr),
		"custom tool with a pure body must not spawn subprocesses for configured MCPs")
}

// TestRunSlop_MCPCallError_NamesMCP: when a script calls a tool on an MCP that
// cannot connect, the error must name the MCP instead of surfacing later as a
// baffling 'unknown service' failure.
func TestRunSlop_MCPCallError_NamesMCP(t *testing.T) {
	s := mockServer(nil)
	s.registry.SetConfigured(config.MCPConfig{
		Name:    "badmcp",
		Type:    "stdio",
		Command: "/nonexistent-command-slop-test",
	})

	_, _, err := s.handleRunSlop(context.Background(), &mcp.CallToolRequest{}, RunSlopInput{
		Script: "badmcp.ping()",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "badmcp")
}

func TestBuildMCPArguments(t *testing.T) {
	t.Run("kwargs pass through by name", func(t *testing.T) {
		args := buildMCPArguments(nil, map[string]slop.Value{
			"query": slop.NewStringValue("hello"),
			"limit": slop.NewIntValue(5),
		})
		require.Equal(t, "hello", args["query"])
		require.Equal(t, int64(5), args["limit"])
		require.NotContains(t, args, "input")
	})

	t.Run("single positional arg also exposed as input", func(t *testing.T) {
		args := buildMCPArguments([]slop.Value{slop.NewStringValue("x")}, nil)
		require.Equal(t, "x", args["arg0"])
		require.Equal(t, "x", args["input"])
	})

	t.Run("multiple positional args become arg0..argN without input", func(t *testing.T) {
		args := buildMCPArguments([]slop.Value{
			slop.NewIntValue(1), slop.NewIntValue(2),
		}, nil)
		require.Equal(t, int64(1), args["arg0"])
		require.Equal(t, int64(2), args["arg1"])
		require.NotContains(t, args, "input")
	})

	t.Run("positional arg with kwargs does not add input", func(t *testing.T) {
		args := buildMCPArguments(
			[]slop.Value{slop.NewStringValue("x")},
			map[string]slop.Value{"k": slop.NewBoolValue(true)},
		)
		require.Equal(t, "x", args["arg0"])
		require.Equal(t, true, args["k"])
		require.NotContains(t, args, "input")
	})
}

// TestRunSlop_CachedMCPServiceReachable: MCPs whose tools were warm-loaded from
// cache (StateCached, no live connection) must still be addressable from SLOP
// scripts. Before the AllConfigs fix only connected MCPs were registered as
// services, so scripts hit 'unknown service' for cache-warm MCPs.
func TestRunSlop_CachedMCPServiceReachable(t *testing.T) {
	s := mockServer(nil)
	s.registry.AddToolsForTesting("cachedmcp", []registry.ToolInfo{
		{Name: "get_data", Description: "d", MCPName: "cachedmcp"},
	})
	s.registry.MarkCachedForTesting("cachedmcp")

	_, _, err := s.handleRunSlop(context.Background(), &mcp.CallToolRequest{}, RunSlopInput{
		Script: "cachedmcp.get_data()",
	})
	// The lazy connect fails (no real command behind the test config), but the
	// error must come from the MCP forwarding path -- proving the service was
	// registered -- and name the MCP, not report an unknown identifier.
	require.Error(t, err)
	require.Contains(t, err.Error(), "cachedmcp")
	require.Contains(t, err.Error(), "get_data")
	require.NotContains(t, err.Error(), "unknown service")
}

func TestContentItemToSlopValue_Resources(t *testing.T) {
	t.Run("embedded resource becomes structured map", func(t *testing.T) {
		v := contentItemToSlopValue(&mcp.EmbeddedResource{
			Resource: &mcp.ResourceContents{
				URI:      "file:///tmp/x.txt",
				MIMEType: "text/plain",
				Text:     "hello",
			},
		})
		got, isMap := slop.ValueToGo(v).(map[string]any)
		require.True(t, isMap, "embedded resource must convert to a map, not a pointer string")
		require.Equal(t, "resource", got["type"])
		require.Equal(t, "file:///tmp/x.txt", got["uri"])
		require.Equal(t, "text/plain", got["mimeType"])
		require.Equal(t, "hello", got["text"])
	})

	t.Run("embedded resource blob is base64", func(t *testing.T) {
		v := contentItemToSlopValue(&mcp.EmbeddedResource{
			Resource: &mcp.ResourceContents{
				URI:  "file:///tmp/x.bin",
				Blob: []byte{0x01, 0x02},
			},
		})
		got := slop.ValueToGo(v).(map[string]any)
		require.Equal(t, "AQI=", got["blob"])
	})

	t.Run("resource link becomes structured map", func(t *testing.T) {
		v := contentItemToSlopValue(&mcp.ResourceLink{
			URI:  "file:///tmp/y.txt",
			Name: "y",
		})
		got, isMap := slop.ValueToGo(v).(map[string]any)
		require.True(t, isMap)
		require.Equal(t, "resourceLink", got["type"])
		require.Equal(t, "file:///tmp/y.txt", got["uri"])
		require.Equal(t, "y", got["name"])
	})
}

func TestResultToSlopValue(t *testing.T) {
	t.Run("structured content preferred", func(t *testing.T) {
		v := resultToSlopValue(&mcp.CallToolResult{
			StructuredContent: map[string]any{"ok": true},
			Content:           []mcp.Content{&mcp.TextContent{Text: "ignored"}},
		})
		got, isMap := slop.ValueToGo(v).(map[string]any)
		require.True(t, isMap)
		require.Equal(t, true, got["ok"])
	})

	t.Run("single JSON text content parsed", func(t *testing.T) {
		v := resultToSlopValue(&mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `{"count": 3}`}},
		})
		got, isMap := slop.ValueToGo(v).(map[string]any)
		require.True(t, isMap)
		require.Equal(t, float64(3), got["count"])
	})

	t.Run("plain text stays a string", func(t *testing.T) {
		v := resultToSlopValue(&mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "hello world"}},
		})
		require.Equal(t, "hello world", slop.ValueToGo(v))
	})

	t.Run("empty content is null", func(t *testing.T) {
		v := resultToSlopValue(&mcp.CallToolResult{})
		require.Nil(t, slop.ValueToGo(v))
	})

	t.Run("multiple content items become a list", func(t *testing.T) {
		v := resultToSlopValue(&mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "a"},
				&mcp.TextContent{Text: "b"},
			},
		})
		got, isList := slop.ValueToGo(v).([]any)
		require.True(t, isList)
		require.Equal(t, []any{"a", "b"}, got)
	})
}
