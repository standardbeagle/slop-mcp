package server

import (
	"github.com/standardbeagle/slop-mcp/internal/overrides"
	"github.com/standardbeagle/slop-mcp/internal/registry"
)

// storeOverrideProvider adapts *overrides.Store to registry.OverrideProvider.
type storeOverrideProvider struct{ store *overrides.Store }

func (p *storeOverrideProvider) OverrideFor(mcpName, toolName string) (string, map[string]string, string, bool) {
	if p.store == nil {
		return "", nil, "", false
	}
	e, ok := p.store.GetOverride(mcpName + "." + toolName)
	if !ok {
		return "", nil, "", false
	}
	return e.Description, e.Params, e.SourceHash, true
}

func (p *storeOverrideProvider) CustomTools() []registry.CustomToolDecl {
	if p.store == nil {
		return nil
	}
	// Merge by scope precedence: Local wins over Project wins over User.
	byScope := p.store.ListCustom()
	seen := make(map[string]bool)
	var out []registry.CustomToolDecl
	for _, scope := range overrides.AllScopes {
		for name, ct := range byScope[scope] {
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, registry.CustomToolDecl{
				Name:        name,
				Description: ct.Description,
				InputSchema: ct.InputSchema,
			})
		}
	}
	return out
}
