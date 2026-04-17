package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/builtins"
	"github.com/standardbeagle/slop-mcp/internal/overrides"
)

// customToolNameRE validates custom tool names: lowercase letter, then [a-z0-9_], max 64 chars total.
var customToolNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// metaToolNames is the set of built-in slop-mcp meta-tools that cannot be shadowed.
var metaToolNames = map[string]struct{}{
	"search_tools":   {},
	"execute_tool":   {},
	"run_slop":       {},
	"manage_mcps":    {},
	"auth_mcp":       {},
	"get_metadata":   {},
	"slop_reference": {},
	"slop_help":      {},
	"agnt_watch":     {},
	"customize_tools": {},
}

// bodyLimit returns the maximum allowed body size in bytes.
// Reads SLOP_MAX_CUSTOM_BODY env var; defaults to 65536.
func bodyLimit() int {
	if v := os.Getenv("SLOP_MAX_CUSTOM_BODY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 65536
}

// depScanRE matches mcp_name.tool_name( patterns for best-effort dependency extraction.
var depScanRE = regexp.MustCompile(`\b([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*)\(`)

// customToolEntry is the wire format for a single custom tool in list_custom responses.
type customToolEntry struct {
	Name      string               `json:"name"`
	Scope     string               `json:"scope,omitempty"`
	Description string             `json:"description,omitempty"`
	DependsOn []overrides.Dependency `json:"depends_on,omitempty"`
	Stale     bool                 `json:"stale,omitempty"`
	StaleDeps []staleDep           `json:"stale_deps,omitempty"`
}

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

// staleDep describes a single stale dependency on a custom tool.
type staleDep struct {
	MCP         string `json:"mcp"`
	Tool        string `json:"tool"`
	StoredHash  string `json:"stored_hash"`
	CurrentHash string `json:"current_hash"`
}

// customizeToolsOutput is the wire format returned by customize_tools actions.
type customizeToolsOutput struct {
	OK               bool                     `json:"ok"`
	Action           string                   `json:"action"`
	Affected         int                      `json:"affected"`
	Entries          []customizeOverrideEntry `json:"entries,omitempty"`
	CustomEntries    []customToolEntry        `json:"custom_entries,omitempty"`
	ShorthandSkipped []string                 `json:"shorthand_skipped,omitempty"`
	Pack             *overrides.Pack          `json:"pack,omitempty"`
	ImportReport     *overrides.ImportReport  `json:"import_report,omitempty"`
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
	case "define_custom":
		return s.customizeDefineCustom(ctx, in)
	case "remove_custom":
		return s.customizeRemoveCustom(in)
	case "list_custom":
		return s.customizeListCustom(ctx, in)
	case "export":
		return s.customizeExport(in)
	case "import":
		return s.customizeImport(in)
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

func (s *Server) customizeDefineCustom(_ context.Context, in CustomizeToolsInput) (*mcp.CallToolResult, customizeToolsOutput, error) {
	if in.Name == "" {
		return nil, customizeToolsOutput{}, fmt.Errorf("name is required for define_custom")
	}
	if in.Description == "" {
		return nil, customizeToolsOutput{}, fmt.Errorf("description is required for define_custom")
	}
	if in.InputSchema == nil {
		return nil, customizeToolsOutput{}, fmt.Errorf("inputSchema is required for define_custom")
	}
	if in.Body == "" {
		return nil, customizeToolsOutput{}, fmt.Errorf("body is required for define_custom")
	}

	// Validate name format.
	if !customToolNameRE.MatchString(in.Name) {
		return nil, customizeToolsOutput{}, fmt.Errorf("invalid name %q: must match ^[a-z][a-z0-9_]{0,63}$", in.Name)
	}

	// Reject meta-tool collisions.
	if _, ok := metaToolNames[in.Name]; ok {
		return nil, customizeToolsOutput{}, fmt.Errorf("name %q collides with a built-in meta-tool", in.Name)
	}

	// Validate body size.
	if len(in.Body) > bodyLimit() {
		return nil, customizeToolsOutput{}, fmt.Errorf("body exceeds maximum size of %d bytes", bodyLimit())
	}

	// Validate InputSchema minimally.
	if t, ok := in.InputSchema["type"]; ok {
		if _, isStr := t.(string); !isStr {
			return nil, customizeToolsOutput{}, fmt.Errorf("inputSchema.type must be a string")
		}
	}
	if props, ok := in.InputSchema["properties"]; ok {
		if _, isMap := props.(map[string]any); !isMap {
			return nil, customizeToolsOutput{}, fmt.Errorf("inputSchema.properties must be an object")
		}
	}
	if ref, ok := in.InputSchema["$ref"]; ok {
		if refStr, isStr := ref.(string); isStr && !strings.HasPrefix(refStr, "#") {
			return nil, customizeToolsOutput{}, fmt.Errorf("external $ref not allowed: %q", refStr)
		}
	}

	// Detect shorthand collisions in property names.
	var shorthandSkipped []string
	if props, ok := in.InputSchema["properties"].(map[string]any); ok {
		names := make([]string, 0, len(props))
		for k := range props {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			if builtins.IsReservedBuiltin(k) {
				shorthandSkipped = append(shorthandSkipped, k)
			}
		}
	}

	// Best-effort dependency extraction: scan body for mcp.tool( patterns.
	var deps []overrides.Dependency
	matches := depScanRE.FindAllStringSubmatch(in.Body, -1)
	seen := map[string]struct{}{}
	for _, m := range matches {
		mcpName, toolName := m[1], m[2]
		key := mcpName + "." + toolName
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		upDesc, upParams, err := s.upstreamToolInfo(mcpName, toolName)
		if err != nil {
			continue // tool not known — skip
		}
		deps = append(deps, overrides.Dependency{
			MCP:  mcpName,
			Tool: toolName,
			Hash: overrides.ComputeHash(upDesc, upParams),
		})
	}

	scope := overrides.Scope(in.Scope)
	if scope == "" {
		scope = overrides.ScopeUser
	}

	ct := overrides.CustomTool{
		Description: in.Description,
		InputSchema: in.InputSchema,
		Body:        in.Body,
		DependsOn:   deps,
	}
	if err := s.overrideStore.SetCustom(scope, in.Name, ct); err != nil {
		return nil, customizeToolsOutput{}, fmt.Errorf("storing custom tool: %w", err)
	}

	s.rebuildOverrideIndex()

	out := customizeToolsOutput{
		OK:       true,
		Action:   "define_custom",
		Affected: 1,
		CustomEntries: []customToolEntry{
			{Name: in.Name, Scope: string(scope)},
		},
		ShorthandSkipped: shorthandSkipped,
	}
	return nil, out, nil
}

func (s *Server) customizeRemoveCustom(in CustomizeToolsInput) (*mcp.CallToolResult, customizeToolsOutput, error) {
	if in.Name == "" {
		return nil, customizeToolsOutput{}, fmt.Errorf("name is required for remove_custom")
	}

	n, err := s.overrideStore.RemoveCustom(overrides.Scope(in.Scope), in.Name)
	if err != nil {
		return nil, customizeToolsOutput{}, fmt.Errorf("removing custom tool: %w", err)
	}

	s.rebuildOverrideIndex()

	out := customizeToolsOutput{
		OK:       true,
		Action:   "remove_custom",
		Affected: n,
	}
	return nil, out, nil
}

func (s *Server) customizeListCustom(ctx context.Context, in CustomizeToolsInput) (*mcp.CallToolResult, customizeToolsOutput, error) {
	all := s.overrideStore.ListCustom()

	var entries []customToolEntry
	for scope, m := range all {
		if in.Scope != "" && string(scope) != in.Scope {
			continue
		}
		names := make([]string, 0, len(m))
		for name := range m {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			ct := m[name]
			entry := customToolEntry{
				Name:        name,
				Scope:       string(scope),
				Description: ct.Description,
				DependsOn:   ct.DependsOn,
			}

			if in.StaleOnly || len(ct.DependsOn) > 0 {
				var staleDeps []staleDep
				for _, dep := range ct.DependsOn {
					_ = s.registry.EnsureConnected(ctx, dep.MCP)
					upDesc, upParams, err := s.upstreamToolInfo(dep.MCP, dep.Tool)
					if err != nil {
						continue
					}
					cur := overrides.ComputeHash(upDesc, upParams)
					if cur != dep.Hash {
						staleDeps = append(staleDeps, staleDep{
							MCP:         dep.MCP,
							Tool:        dep.Tool,
							StoredHash:  dep.Hash,
							CurrentHash: cur,
						})
					}
				}
				if len(staleDeps) > 0 {
					entry.Stale = true
					entry.StaleDeps = staleDeps
				}
			}

			if in.StaleOnly && !entry.Stale {
				continue
			}
			entries = append(entries, entry)
		}
	}

	out := customizeToolsOutput{
		OK:            true,
		Action:        "list_custom",
		Affected:      len(entries),
		CustomEntries: entries,
	}
	return nil, out, nil
}

func (s *Server) customizeExport(in CustomizeToolsInput) (*mcp.CallToolResult, customizeToolsOutput, error) {
	sel := overrides.Selector{
		MCP:           in.MCP,
		Keys:          in.Keys,
		IncludeCustom: in.IncludeCustom,
		Scope:         overrides.Scope(in.Scope),
	}
	pack, err := s.overrideStore.Export(sel)
	if err != nil {
		return nil, customizeToolsOutput{}, err
	}
	out := customizeToolsOutput{
		OK:       true,
		Action:   "export",
		Affected: len(pack.Overrides) + len(pack.CustomTools),
		Pack:     &pack,
	}
	return nil, out, nil
}

func (s *Server) customizeImport(in CustomizeToolsInput) (*mcp.CallToolResult, customizeToolsOutput, error) {
	if in.Data == "" {
		return nil, customizeToolsOutput{}, fmt.Errorf("data is required for import (JSON pack)")
	}
	var pack overrides.Pack
	if err := json.Unmarshal([]byte(in.Data), &pack); err != nil {
		return nil, customizeToolsOutput{}, fmt.Errorf("invalid pack JSON: %w", err)
	}
	scope := overrides.Scope(in.Scope)
	if scope == "" {
		scope = overrides.ScopeUser
	}
	rep, err := s.overrideStore.Import(pack, scope, in.Overwrite)
	if err != nil {
		return nil, customizeToolsOutput{}, err
	}
	s.rebuildOverrideIndex()
	out := customizeToolsOutput{
		OK:           true,
		Action:       "import",
		Affected:     rep.ImportedOverrides + rep.ImportedCustom,
		ImportReport: &rep,
	}
	return nil, out, nil
}

// rebuildOverrideIndex re-registers the override provider on the registry,
// triggering an index rebuild that applies current overrides and custom tools.
func (s *Server) rebuildOverrideIndex() {
	if s.overrideStore == nil {
		return
	}
	// Build a lightweight OverrideProvider adapter backed by the current store
	// and inject it. This is idempotent and always reflects latest store state.
	s.registry.SetOverrideProvider(&storeOverrideProvider{store: s.overrideStore})
}
