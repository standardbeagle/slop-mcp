package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/overrides"
)

// CustomizeToolsInput is the input for the customize_tools tool.
type CustomizeToolsInput struct {
	Action        string            `json:"action"`
	MCP           string            `json:"mcp,omitempty"`
	Tool          string            `json:"tool,omitempty"`
	Description   string            `json:"description,omitempty"`
	Params        map[string]string `json:"params,omitempty"`
	Scope         string            `json:"scope,omitempty"`
	StaleOnly     bool              `json:"stale_only,omitempty"`
	Name          string            `json:"name,omitempty"`
	InputSchema   map[string]any    `json:"inputSchema,omitempty"`
	Body          string            `json:"body,omitempty"`
	Keys          []string          `json:"keys,omitempty"`
	IncludeCustom bool              `json:"include_custom,omitempty"`
	Data          string            `json:"data,omitempty"`
	Overwrite     bool              `json:"overwrite,omitempty"`
}

// customizeOverrideEntry is the wire format for a single override in responses.
type customizeOverrideEntry struct {
	Key         string            `json:"key"`
	Description string            `json:"description,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
	Scope       string            `json:"scope,omitempty"`
	Hash        string            `json:"hash,omitempty"`
	Stale       bool              `json:"stale,omitempty"`
	StaleSource map[string]any    `json:"stale_source,omitempty"`
}

// customizeToolsOutput is the wire format returned by customize_tools actions.
type customizeToolsOutput struct {
	OK      bool                     `json:"ok"`
	Action  string                   `json:"action"`
	Affected int                     `json:"affected"`
	Entries []customizeOverrideEntry `json:"entries"`
}

func (s *Server) handleCustomizeTools(
	ctx context.Context,
	req *mcp.CallToolRequest,
	in CustomizeToolsInput,
) (*mcp.CallToolResult, customizeToolsOutput, error) {
	if s.overrideStore == nil {
		return nil, customizeToolsOutput{}, fmt.Errorf("override store not initialized")
	}
	switch in.Action {
	case "set_override":
		return s.customizeSetOverride(ctx, in)
	case "remove_override":
		return s.customizeRemoveOverride(ctx, in)
	case "list_overrides":
		return s.customizeListOverrides(ctx, in)
	case "define_custom", "remove_custom", "list_custom", "export", "import":
		return nil, customizeToolsOutput{}, fmt.Errorf("action %q not yet implemented (tasks 13-14)", in.Action)
	default:
		return nil, customizeToolsOutput{}, fmt.Errorf("unknown action: %q", in.Action)
	}
}

func (s *Server) customizeSetOverride(ctx context.Context, in CustomizeToolsInput) (*mcp.CallToolResult, customizeToolsOutput, error) {
	if in.MCP == "" {
		return nil, customizeToolsOutput{}, fmt.Errorf("mcp is required for set_override")
	}
	if in.Tool == "" {
		return nil, customizeToolsOutput{}, fmt.Errorf("tool is required for set_override")
	}
	if in.Description == "" {
		return nil, customizeToolsOutput{}, fmt.Errorf("description is required for set_override")
	}

	scope := overrides.Scope(in.Scope)
	if scope == "" {
		scope = overrides.ScopeUser
	}

	// Try the tool lookup directly first; if not found, lazy-connect then retry.
	upstreamDesc, upstreamParams, err := s.upstreamToolInfo(in.MCP, in.Tool)
	if err != nil {
		if connErr := s.registry.EnsureConnected(ctx, in.MCP); connErr != nil {
			return nil, customizeToolsOutput{}, fmt.Errorf("could not connect to MCP %q: %w", in.MCP, connErr)
		}
		upstreamDesc, upstreamParams, err = s.upstreamToolInfo(in.MCP, in.Tool)
		if err != nil {
			return nil, customizeToolsOutput{}, err
		}
	}

	hash := overrides.ComputeHash(upstreamDesc, upstreamParams)
	entry := overrides.OverrideEntry{
		Description: in.Description,
		Params:      in.Params,
		SourceHash:  hash,
	}
	if err := s.overrideStore.SetOverride(scope, in.MCP+"."+in.Tool, entry); err != nil {
		return nil, customizeToolsOutput{}, fmt.Errorf("storing override: %w", err)
	}

	s.rebuildOverrideIndex()

	out := customizeToolsOutput{
		OK:       true,
		Action:   "set_override",
		Affected: 1,
		Entries: []customizeOverrideEntry{
			{Key: in.MCP + "." + in.Tool, Hash: hash, Scope: string(scope)},
		},
	}
	return nil, out, nil
}

func (s *Server) customizeRemoveOverride(_ context.Context, in CustomizeToolsInput) (*mcp.CallToolResult, customizeToolsOutput, error) {
	if in.MCP == "" {
		return nil, customizeToolsOutput{}, fmt.Errorf("mcp is required for remove_override")
	}
	if in.Tool == "" {
		return nil, customizeToolsOutput{}, fmt.Errorf("tool is required for remove_override")
	}

	n, err := s.overrideStore.RemoveOverride(overrides.Scope(in.Scope), in.MCP+"."+in.Tool)
	if err != nil {
		return nil, customizeToolsOutput{}, fmt.Errorf("removing override: %w", err)
	}

	s.rebuildOverrideIndex()

	entries := []customizeOverrideEntry{}
	out := customizeToolsOutput{
		OK:       true,
		Action:   "remove_override",
		Affected: n,
		Entries:  entries,
	}
	return nil, out, nil
}

func (s *Server) customizeListOverrides(ctx context.Context, in CustomizeToolsInput) (*mcp.CallToolResult, customizeToolsOutput, error) {
	all := s.overrideStore.ListOverrides()

	type flatEntry struct {
		key   string
		scope overrides.Scope
		e     overrides.OverrideEntry
	}
	var flat []flatEntry
	for scope, m := range all {
		if in.Scope != "" && string(scope) != in.Scope {
			continue
		}
		for key, e := range m {
			if in.MCP != "" && !strings.HasPrefix(key, in.MCP+".") {
				continue
			}
			flat = append(flat, flatEntry{key: key, scope: scope, e: e})
		}
	}

	entries := make([]customizeOverrideEntry, 0, len(flat))
	for _, f := range flat {
		ce := customizeOverrideEntry{
			Key:         f.key,
			Description: f.e.Description,
			Params:      f.e.Params,
			Scope:       string(f.scope),
			Hash:        f.e.SourceHash,
		}

		// Always compute stale status so callers get current information.
		stale, upstreamDesc, upstreamParams := s.isStale(ctx, f.key, f.e.SourceHash)
		ce.Stale = stale
		if stale {
			ss := map[string]any{"description": upstreamDesc}
			if upstreamParams != nil {
				ss["params"] = upstreamParams
			}
			ce.StaleSource = ss
		}

		if in.StaleOnly && !ce.Stale {
			continue
		}
		entries = append(entries, ce)
	}

	out := customizeToolsOutput{
		OK:       true,
		Action:   "list_overrides",
		Affected: len(entries),
		Entries:  entries,
	}
	return nil, out, nil
}

// upstreamToolInfo returns the upstream description and param descs for a tool from the registry index.
// Returns an error if the tool is not found.
func (s *Server) upstreamToolInfo(mcpName, toolName string) (string, map[string]string, error) {
	tools := s.registry.SearchTools(toolName, mcpName)
	for _, t := range tools {
		if t.MCPName == mcpName && t.Name == toolName {
			return t.Description, extractParamDescs(t.InputSchema), nil
		}
	}
	return "", nil, fmt.Errorf("tool %q not found in MCP %q", toolName, mcpName)
}

// isStale computes whether the stored hash differs from the current upstream hash.
// Returns stale flag plus upstream description and params for stale_source.
func (s *Server) isStale(ctx context.Context, key, storedHash string) (bool, string, map[string]string) {
	dot := strings.LastIndex(key, ".")
	if dot < 0 {
		return false, "", nil
	}
	mcpName := key[:dot]
	toolName := key[dot+1:]

	// Best-effort connect; if we can't, report not-stale rather than erroring.
	_ = s.registry.EnsureConnected(ctx, mcpName)

	upstreamDesc, upstreamParams, err := s.upstreamToolInfo(mcpName, toolName)
	if err != nil {
		return false, "", nil
	}
	currentHash := overrides.ComputeHash(upstreamDesc, upstreamParams)
	return currentHash != storedHash, upstreamDesc, upstreamParams
}

// rebuildOverrideIndex triggers a registry index rebuild so search_tools sees updated overrides.
// It is a no-op if the server has no override provider wired yet (Task 16).
func (s *Server) rebuildOverrideIndex() {
	// If SetOverrideProvider was already called (Task 16), we need to re-call it
	// so the index picks up the latest store state. We do this by passing through
	// the existing provider. Since we don't hold a reference to it here, we
	// trigger a rebuild via SetOverrideProvider(nil) + restore is not safe.
	// Instead, call SetOverrideProvider with a fresh storeBackedProvider if
	// the store is set; otherwise the registry index already has no overrides so
	// no action needed.
	if s.overrideStore == nil {
		return
	}
	// Build a lightweight OverrideProvider adapter backed by the current store
	// and inject it. This is idempotent and always reflects latest store state.
	s.registry.SetOverrideProvider(&storeOverrideProvider{store: s.overrideStore})
}
