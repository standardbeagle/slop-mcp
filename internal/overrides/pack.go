package overrides

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"
)

// PackSchemaVersion is the current version of the pack JSON format.
const PackSchemaVersion = 1

// Pack is the portable export/import format for overrides and custom tools.
type Pack struct {
	SchemaVersion int            `json:"schema_version"`
	ExportedAt    time.Time      `json:"exported_at"`
	Source        string         `json:"source"`
	Selector      any            `json:"selector,omitempty"`
	Overrides     []PackOverride `json:"overrides,omitempty"`
	CustomTools   []PackCustom   `json:"custom_tools,omitempty"`
}

// PackOverride is the pack representation of a single override entry.
// Scope and UpdatedAt are intentionally omitted — the recipient assigns scope on import.
type PackOverride struct {
	Key         string            `json:"key"`
	Description string            `json:"description"`
	Params      map[string]string `json:"params,omitempty"`
	SourceHash  string            `json:"source_hash"`
}

// PackCustom is the pack representation of a custom tool.
type PackCustom struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Body        string         `json:"body"`
	DependsOn   []Dependency   `json:"depends_on,omitempty"`
}

// Selector controls which entries are included in an export.
type Selector struct {
	// MCP filters overrides to keys matching "<mcp>.*" and custom tools whose
	// DependsOn references this MCP.
	MCP string `json:"mcp,omitempty"`
	// Keys is a list of glob patterns applied to override keys.
	// When non-empty, MCP is ignored for overrides and custom tool filtering
	// falls back to MCP-based inclusion (if MCP is also set).
	Keys []string `json:"keys,omitempty"`
	// IncludeCustom explicitly enables custom-tool export even when Keys is set
	// and no MCP filter is active.
	IncludeCustom bool `json:"include_custom,omitempty"`
	// Scope, when non-empty, restricts export to a single scope layer (no merge).
	Scope Scope `json:"scope,omitempty"`
}

// ImportReport summarises the result of an import operation.
type ImportReport struct {
	ImportedOverrides int      `json:"imported_overrides"`
	ImportedCustom    int      `json:"imported_custom"`
	Skipped           []string `json:"skipped,omitempty"`
	MissingDeps       []string `json:"missing_deps,omitempty"`
}

// Export builds a Pack from the store according to sel.
//
// Scope precedence (Local > Project > User) is applied per key so the
// pack always contains the effective winning value. If sel.Scope is set,
// only that layer is consulted (no merge).
func (s *Store) Export(sel Selector) (Pack, error) {
	pack := Pack{
		SchemaVersion: PackSchemaVersion,
		ExportedAt:    time.Now().UTC(),
		Source:        "slop-mcp",
	}
	if sel.MCP != "" || len(sel.Keys) > 0 || sel.Scope != "" {
		pack.Selector = sel
	}

	// Build the effective glob list for override key matching.
	globs := append([]string(nil), sel.Keys...)
	if len(globs) == 0 && sel.MCP != "" {
		globs = []string{sel.MCP + ".*"}
	}
	matchKey := func(k string) bool {
		if len(globs) == 0 {
			return true
		}
		for _, g := range globs {
			if ok, _ := filepath.Match(g, k); ok {
				return true
			}
		}
		return false
	}

	// Overrides — iterate AllScopes in precedence order; first occurrence wins per key.
	allOvr := s.ListOverrides()
	keysSeen := map[string]bool{}
	for _, scope := range AllScopes {
		if sel.Scope != "" && scope != sel.Scope {
			continue
		}
		for k, e := range allOvr[scope] {
			if keysSeen[k] {
				continue
			}
			if !matchKey(k) {
				continue
			}
			keysSeen[k] = true
			pack.Overrides = append(pack.Overrides, PackOverride{
				Key:         k,
				Description: e.Description,
				Params:      e.Params,
				SourceHash:  e.SourceHash,
			})
		}
	}
	sort.Slice(pack.Overrides, func(i, j int) bool {
		return pack.Overrides[i].Key < pack.Overrides[j].Key
	})

	// Determine whether to include custom tools.
	// Default: include when no Keys filter and no explicit IncludeCustom=false.
	wantCustom := sel.MCP != "" || len(sel.Keys) == 0 || sel.IncludeCustom

	if wantCustom {
		allCustom := s.ListCustom()
		namesSeen := map[string]bool{}
		for _, scope := range AllScopes {
			if sel.Scope != "" && scope != sel.Scope {
				continue
			}
			for name, ct := range allCustom[scope] {
				if namesSeen[name] {
					continue
				}
				var include bool
				switch {
				case sel.MCP != "":
					// Include only if DependsOn references sel.MCP.
					for _, dep := range ct.DependsOn {
						if dep.MCP == sel.MCP {
							include = true
							break
						}
					}
				case sel.IncludeCustom:
					include = true
				case len(sel.Keys) == 0:
					// No filter at all — export everything.
					include = true
				}
				if include {
					namesSeen[name] = true
					pack.CustomTools = append(pack.CustomTools, PackCustom{
						Name:        name,
						Description: ct.Description,
						InputSchema: ct.InputSchema,
						Body:        ct.Body,
						DependsOn:   ct.DependsOn,
					})
				}
			}
		}
		sort.Slice(pack.CustomTools, func(i, j int) bool {
			return pack.CustomTools[i].Name < pack.CustomTools[j].Name
		})
	}

	return pack, nil
}

// Import applies a pack to the given scope. When overwrite is false,
// colliding keys/names are skipped and reported in ImportReport.Skipped.
//
// Atomicity note: writes are per-bank atomic (tmp+rename) but the two
// banks (overrides and custom tools) are written independently. A partial
// apply is possible if the process exits mid-import.
func (s *Store) Import(pack Pack, scope Scope, overwrite bool) (ImportReport, error) {
	if pack.SchemaVersion != PackSchemaVersion {
		return ImportReport{}, fmt.Errorf(
			"unsupported schema_version %d; this slop-mcp supports schema_version %d",
			pack.SchemaVersion, PackSchemaVersion,
		)
	}

	rep := ImportReport{}

	for _, po := range pack.Overrides {
		if !overwrite {
			if existing, ok := s.GetOverride(po.Key); ok && existing.Scope == scope {
				rep.Skipped = append(rep.Skipped, po.Key)
				continue
			}
		}
		if err := s.SetOverride(scope, po.Key, OverrideEntry{
			Description: po.Description,
			Params:      po.Params,
			SourceHash:  po.SourceHash,
		}); err != nil {
			return rep, err
		}
		rep.ImportedOverrides++
	}

	// Collect unique MCP names from deps to report missing ones.
	depMCPs := map[string]bool{}
	for _, pc := range pack.CustomTools {
		for _, dep := range pc.DependsOn {
			depMCPs[dep.MCP] = true
		}
	}

	for _, pc := range pack.CustomTools {
		if !overwrite {
			if _, ok := s.GetCustom(pc.Name); ok {
				rep.Skipped = append(rep.Skipped, pc.Name)
				continue
			}
		}
		if err := s.SetCustom(scope, pc.Name, CustomTool{
			Description: pc.Description,
			InputSchema: pc.InputSchema,
			Body:        pc.Body,
			DependsOn:   pc.DependsOn,
		}); err != nil {
			return rep, err
		}
		rep.ImportedCustom++
	}

	// MissingDeps: report dep MCPs that have no presence in any scope.
	// We can't check the registry from here, so we report based on what
	// the caller already knows — leave the field empty for now. The
	// staleness check at list_custom time will surface stale deps.
	_ = depMCPs

	return rep, nil
}
