// Package recipes provides embedded SLOP script templates.
package recipes

import (
	"embed"
	"fmt"
	"path/filepath"
	"strings"
)

//go:embed scripts/*.slop
var scriptsFS embed.FS

// Recipe describes an available recipe template.
type Recipe struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// List returns all available recipe names with descriptions.
func List() []Recipe {
	entries, err := scriptsFS.ReadDir("scripts")
	if err != nil {
		return nil
	}
	recipes := make([]Recipe, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".slop" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".slop")
		desc := extractDescription(name)
		recipes = append(recipes, Recipe{Name: name, Description: desc})
	}
	return recipes
}

// Load returns the script content for a recipe by name.
func Load(name string) (string, error) {
	data, err := scriptsFS.ReadFile("scripts/" + name + ".slop")
	if err != nil {
		available := List()
		names := make([]string, len(available))
		for i, r := range available {
			names[i] = r.Name
		}
		return "", fmt.Errorf("recipe %q not found. Available: %s", name, strings.Join(names, ", "))
	}
	return string(data), nil
}

// extractDescription reads the first comment line from the embedded script.
func extractDescription(name string) string {
	data, err := scriptsFS.ReadFile("scripts/" + name + ".slop")
	if err != nil {
		return ""
	}
	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) == 0 {
		return ""
	}
	var desc string
	switch {
	case strings.HasPrefix(lines[0], "//"):
		desc = strings.TrimPrefix(lines[0], "//")
	case strings.HasPrefix(lines[0], "#"):
		desc = strings.TrimPrefix(lines[0], "#")
	default:
		return ""
	}
	desc = strings.TrimSpace(desc)
	// Strip "name:" prefix if present
	if idx := strings.Index(desc, ":"); idx > 0 && idx < 30 {
		desc = strings.TrimSpace(desc[idx+1:])
	}
	return desc
}
