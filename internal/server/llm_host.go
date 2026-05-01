package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/llm"
)

// LLMHost adapts the slop-mcp registry to llm.ToolHost.
// It exposes all indexed MCP tools to the agentic loop and routes
// tool calls back through the registry's execute path.
type LLMHost struct {
	server *Server
}

// NewLLMHost returns a new LLMHost backed by the given Server.
func NewLLMHost(s *Server) *LLMHost { return &LLMHost{server: s} }

// List enumerates all indexed MCP tools, returning them as ToolSpec values.
// If allow is non-empty only the named tools are included (O(1) lookup).
// The qualified name is "mcpName.toolName".
func (h *LLMHost) List(ctx context.Context, allow []string) []llm.ToolSpec {
	all := h.server.registry.SearchTools("", "")

	allowSet := make(map[string]struct{}, len(allow))
	for _, n := range allow {
		allowSet[n] = struct{}{}
	}

	out := make([]llm.ToolSpec, 0, len(all))
	for _, t := range all {
		qualName := qualifiedName(t.MCPName, t.Name)
		if len(allowSet) > 0 {
			if _, ok := allowSet[qualName]; !ok {
				continue
			}
		}
		schema, _ := json.Marshal(t.InputSchema)
		if len(schema) == 0 || string(schema) == "null" {
			schema = json.RawMessage(`{}`)
		}
		out = append(out, llm.ToolSpec{
			Name:        qualName,
			Description: t.Description,
			Schema:      schema,
		})
	}
	return out
}

// Execute dispatches a tool call by qualified name ("mcpName.toolName").
// Returns the three-state contract defined by llm.ToolHost:
//   - (text, false, nil): success
//   - ("", false, err):   infrastructure failure (unknown tool, transport error)
//   - (errText, true, nil): tool ran and reported failure (MCP isError bit)
func (h *LLMHost) Execute(ctx context.Context, name string, args json.RawMessage) (string, bool, error) {
	toolTimeout := 60
	if h.server.config != nil && h.server.config.LLM != nil && h.server.config.LLM.ToolTimeoutSec > 0 {
		toolTimeout = h.server.config.LLM.ToolTimeoutSec
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(toolTimeout)*time.Second)
	defer cancel()

	mcpName, toolName, err := parseQualifiedName(name)
	if err != nil {
		return "", false, err
	}

	// Unmarshal args JSON into map[string]any for registry.ExecuteToolRaw.
	var params map[string]any
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &params); err != nil {
			return "", false, fmt.Errorf("invalid args for tool %q: %w", name, err)
		}
	}

	result, err := h.server.registry.ExecuteToolRaw(ctx, mcpName, toolName, params)
	if err != nil {
		return "", false, err
	}

	text := extractText(result)
	if result.IsError {
		return text, true, nil
	}
	return text, false, nil
}

// qualifiedName joins mcpName and toolName with ".".
func qualifiedName(mcpName, toolName string) string {
	return mcpName + "." + toolName
}

// parseQualifiedName splits "mcpName.toolName". The first dot is the separator;
// tool names can contain dots themselves (unlikely but safe).
func parseQualifiedName(name string) (mcpName, toolName string, err error) {
	idx := strings.Index(name, ".")
	if idx < 1 || idx == len(name)-1 {
		return "", "", fmt.Errorf("tool name %q must be in \"mcp.tool\" form", name)
	}
	return name[:idx], name[idx+1:], nil
}

// extractText joins all text content items from a CallToolResult.
func extractText(result *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}
