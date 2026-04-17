// Package overrides owns tool-description overrides and agent-authored
// custom tools, scoped to user / project / local tiers.
package overrides

import (
	"strings"
	"time"
)

// Bank names reserved for the overrides subsystem.
const (
	BankOverrides   = "_slop.overrides"
	BankCustomTools = "_slop.tools"
	ReservedPrefix  = "_slop."
)

// IsReservedBank reports whether a bank name is owned by this subsystem.
func IsReservedBank(name string) bool {
	return strings.HasPrefix(name, ReservedPrefix)
}

// Scope identifies a storage tier.
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
	ScopeLocal   Scope = "local"
)

// AllScopes is the canonical scope ordering used when no scope is specified.
// Iterating in this order respects precedence at merge time (Local > Project > User).
var AllScopes = []Scope{ScopeLocal, ScopeProject, ScopeUser}

// OverrideEntry is the value shape stored under BankOverrides, keyed by "<mcp>.<tool>".
type OverrideEntry struct {
	Description string            `json:"description"`
	Params      map[string]string `json:"params,omitempty"`
	SourceHash  string            `json:"source_hash"`
	Scope       Scope             `json:"scope,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at,omitempty"`
}

// Dependency is a hash-pinned reference to a native tool used by a custom tool.
type Dependency struct {
	MCP  string `json:"mcp"`
	Tool string `json:"tool"`
	Hash string `json:"hash"`
}

// CustomTool is the value shape stored under BankCustomTools, keyed by tool name.
type CustomTool struct {
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Body        string         `json:"body"`
	DependsOn   []Dependency   `json:"depends_on,omitempty"`
	Scope       Scope          `json:"scope,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at,omitempty"`
}
