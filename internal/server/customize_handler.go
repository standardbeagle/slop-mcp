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
	"search_tools":    {},
	"execute_tool":    {},
	"run_slop":        {},
	"manage_mcps":     {},
	"auth_mcp":        {},
	"get_metadata":    {},
	"slop_reference":  {},
	"slop_help":       {},
	"agnt_watch":      {},
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

// depScanRE matches mcp_name.tool_name( patterns for best-effort dependency
// extraction. The MCP-name segment allows hyphens (e.g. dart-query.create_task);
// this is scan-only metadata, so an occasional false positive on a subtraction
// expression is harmless.
var depScanRE = regexp.MustCompile(`\b([a-z_][a-z0-9_-]*)\.([a-z_][a-z0-9_]*)\(`)

// customToolEntry is the wire format for a single custom tool in list_custom responses.
type customToolEntry struct {
	Name        string                 `json:"name"`
	Scope       string                 `json:"scope,omitempty"`
	Description string                 `json:"description,omitempty"`
	DependsOn   []overrides.Dependency `json:"depends_on,omitempty"`
	Stale       bool                   `json:"stale,omitempty"`
	StaleDeps   []staleDep             `json:"stale_deps,omitempty"`
	// UnknownDeps lists "mcp.tool" dependencies whose staleness could not be
	// determined because the MCP is neither connected nor cached.
	UnknownDeps []string `json:"unknown_deps,omitempty"`
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
	StaleStatus string            `json:"stale_status,omitempty"`
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
	// Fail fast on an invalid scope rather than silently writing to the wrong
	// tier (set/define) or returning an empty result (list filters).
	if !overrides.IsValidScope(in.Scope) {
		return nil, customizeToolsOutput{}, fmt.Errorf("invalid scope %q: must be one of user, project, local", in.Scope)
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

func (s *Server) customizeListOverrides(_ context.Context, in CustomizeToolsInput) (*mcp.CallToolResult, customizeToolsOutput, error) {
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

		// Compute stale status from already-indexed tool info only; listing
		// must not fan out connections to every referenced MCP.
		status, upstreamDesc, upstreamParams := s.staleStatus(f.key, f.e.SourceHash)
		ce.Stale = status == staleStatusStale
		if status == staleStatusUnknown {
			ce.StaleStatus = staleStatusUnknown
		}
		if ce.Stale {
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
			// The index applies override descriptions; UpstreamDescription
			// returns the original server description so hashes computed from
			// this value reflect the true upstream, not the override.
			return t.UpstreamDescription(), extractParamDescs(t.InputSchema), nil
		}
	}
	return "", nil, fmt.Errorf("tool %q not found in MCP %q", toolName, mcpName)
}

// Stale statuses reported by list actions. Listing never triggers MCP
// connections: when the tool is not in the index (MCP neither connected nor
// cached), staleness is honestly reported as unknown instead of blocking on a
// connection fan-out across every referenced MCP.
const (
	staleStatusFresh   = "fresh"
	staleStatusStale   = "stale"
	staleStatusUnknown = "unknown"
)

// staleStatus compares the stored hash against the current upstream hash using
// only already-indexed tool info (connected or cached MCPs). Returns the
// status plus upstream description and params for stale_source when stale.
func (s *Server) staleStatus(key, storedHash string) (string, string, map[string]string) {
	dot := strings.LastIndex(key, ".")
	if dot < 0 {
		return staleStatusUnknown, "", nil
	}
	mcpName := key[:dot]
	toolName := key[dot+1:]

	upstreamDesc, upstreamParams, err := s.upstreamToolInfo(mcpName, toolName)
	if err != nil {
		return staleStatusUnknown, "", nil
	}
	if overrides.ComputeHash(upstreamDesc, upstreamParams) != storedHash {
		return staleStatusStale, upstreamDesc, upstreamParams
	}
	return staleStatusFresh, "", nil
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

	// Syntax-check the body now, while the defining context is present, instead
	// of deferring the error to the first execution of the custom tool.
	// Constructing a runtime rebinds slop's package-global pipeline caller, so
	// hold the process-wide exec lock across construct+parse to avoid racing a
	// concurrent script execution (see builtins.slopExecMu).
	if err := func() error {
		builtins.LockSlopExec()
		defer builtins.UnlockSlopExec()
		syntaxRT := builtins.NewRuntime()
		defer syntaxRT.Close()
		_, perr := syntaxRT.Parse(in.Body)
		return perr
	}(); err != nil {
		return nil, customizeToolsOutput{}, fmt.Errorf("body has SLOP syntax errors: %w", err)
	}

	if err := validateCustomInputSchema(in.InputSchema); err != nil {
		return nil, customizeToolsOutput{}, err
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

func validateCustomInputSchema(schema map[string]any) error {
	t, ok := schema["type"].(string)
	if !ok {
		return fmt.Errorf("inputSchema.type must be \"object\"")
	}
	if t != "object" {
		return fmt.Errorf("inputSchema.type must be \"object\", got %q", t)
	}

	if propsRaw, ok := schema["properties"]; ok {
		props, ok := propsRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("inputSchema.properties must be an object")
		}
		for name, raw := range props {
			prop, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("inputSchema.properties.%s must be an object", name)
			}
			if typ, ok := prop["type"]; ok {
				typStr, isStr := typ.(string)
				if !isStr {
					return fmt.Errorf("inputSchema.properties.%s.type must be a string", name)
				}
				if !isSupportedCustomSchemaType(typStr) {
					return fmt.Errorf("inputSchema.properties.%s.type must be one of string, number, integer, boolean, array, object", name)
				}
			}
		}
	}

	if requiredRaw, ok := schema["required"]; ok {
		switch required := requiredRaw.(type) {
		case []any:
			for i, raw := range required {
				if _, ok := raw.(string); !ok {
					return fmt.Errorf("inputSchema.required[%d] must be a string", i)
				}
			}
		case []string:
		default:
			return fmt.Errorf("inputSchema.required must be an array of strings")
		}
	}

	if ref, ok := schema["$ref"]; ok {
		refStr, isStr := ref.(string)
		if !isStr {
			return fmt.Errorf("inputSchema.$ref must be a string")
		}
		if !strings.HasPrefix(refStr, "#") {
			return fmt.Errorf("external $ref not allowed: %q", refStr)
		}
	}

	return nil
}

func isSupportedCustomSchemaType(typ string) bool {
	switch typ {
	case "string", "number", "integer", "boolean", "array", "object":
		return true
	default:
		return false
	}
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

func (s *Server) customizeListCustom(_ context.Context, in CustomizeToolsInput) (*mcp.CallToolResult, customizeToolsOutput, error) {
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

			// Stale detection uses already-indexed tool info only; listing
			// must not trigger a connection per dependency. Deps whose MCP is
			// neither connected nor cached are reported as unknown.
			if in.StaleOnly || len(ct.DependsOn) > 0 {
				var staleDeps []staleDep
				var unknownDeps []string
				for _, dep := range ct.DependsOn {
					upDesc, upParams, err := s.upstreamToolInfo(dep.MCP, dep.Tool)
					if err != nil {
						unknownDeps = append(unknownDeps, dep.MCP+"."+dep.Tool)
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
				entry.UnknownDeps = unknownDeps
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
