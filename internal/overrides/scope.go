package overrides

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNoRepo is returned when project/local scope is requested outside a repo.
var ErrNoRepo = errors.New("not inside a slop-mcp repo (need .git or .slop-mcp.kdl)")

// findRepoRoot walks up from start looking for .git or .slop-mcp.kdl.
func findRepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, ".slop-mcp.kdl")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNoRepo
		}
		dir = parent
	}
}

// FindRepoRoot is the exported form for use by the server layer.
func FindRepoRoot(start string) (string, error) {
	return findRepoRoot(start)
}

// ScopeRoot returns the on-disk directory for the given scope.
// userHome and cwd let tests inject locations.
func ScopeRoot(scope Scope, userHome, cwd string) (string, error) {
	switch scope {
	case ScopeUser:
		return filepath.Join(userHome, ".config", "slop-mcp", "memory", "_slop"), nil
	case ScopeProject:
		root, err := findRepoRoot(cwd)
		if err != nil {
			return "", err
		}
		return filepath.Join(root, ".slop-mcp", "memory", "_slop"), nil
	case ScopeLocal:
		root, err := findRepoRoot(cwd)
		if err != nil {
			return "", err
		}
		return filepath.Join(root, ".slop-mcp", "memory.local", "_slop"), nil
	default:
		return "", errors.New("unknown scope: " + string(scope))
	}
}

// MergeOverride picks the entry from the scope map using precedence
// Local > Project > User. Returns nil if the map is empty.
func MergeOverride(per map[Scope]*OverrideEntry) *OverrideEntry {
	for _, s := range AllScopes { // Local, Project, User
		if e := per[s]; e != nil {
			cp := *e
			cp.Scope = s
			return &cp
		}
	}
	return nil
}

// MergeCustom picks the custom tool by the same precedence rule.
func MergeCustom(per map[Scope]*CustomTool) *CustomTool {
	for _, s := range AllScopes {
		if e := per[s]; e != nil {
			cp := *e
			cp.Scope = s
			return &cp
		}
	}
	return nil
}
