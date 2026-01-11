package registry

import (
	"strings"
	"sync"
)

// normalize converts a string to lowercase and normalizes separators
// (underscores, hyphens, spaces) to enable fuzzy matching.
// e.g., "code_insight", "code-insight", "code insight" all become "codeinsight"
func normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

// ToolIndex indexes tools for efficient searching.
type ToolIndex struct {
	// mcpName -> list of tools
	byMCP map[string][]ToolInfo
	mu    sync.RWMutex
}

// NewToolIndex creates a new ToolIndex.
func NewToolIndex() *ToolIndex {
	return &ToolIndex{
		byMCP: make(map[string][]ToolInfo),
	}
}

// Add adds tools for an MCP.
func (idx *ToolIndex) Add(mcpName string, tools []ToolInfo) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.byMCP[mcpName] = tools
}

// Remove removes all tools for an MCP.
func (idx *ToolIndex) Remove(mcpName string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.byMCP, mcpName)
}

// Search searches tools by query and optionally filters by MCP name.
// Search is case-insensitive and supports fuzzy matching by normalizing
// separators (underscores, hyphens, spaces). For example, "code insight"
// will match "code_insight", "code-insight", and "CodeInsight".
func (idx *ToolIndex) Search(query, mcpName string) []ToolInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []ToolInfo
	queryLower := strings.ToLower(query)
	queryNorm := normalize(query)

	for mcp, tools := range idx.byMCP {
		// Filter by MCP name if specified
		if mcpName != "" && mcp != mcpName {
			continue
		}

		for _, tool := range tools {
			// If no query, include all
			if query == "" {
				results = append(results, tool)
				continue
			}

			// Match against name or description using both exact and fuzzy matching
			nameLower := strings.ToLower(tool.Name)
			descLower := strings.ToLower(tool.Description)
			nameNorm := normalize(tool.Name)
			descNorm := normalize(tool.Description)

			// Exact case-insensitive match or fuzzy normalized match
			if strings.Contains(nameLower, queryLower) ||
				strings.Contains(descLower, queryLower) ||
				strings.Contains(nameNorm, queryNorm) ||
				strings.Contains(descNorm, queryNorm) {
				results = append(results, tool)
			}
		}
	}

	return results
}

// CountForMCP returns the number of tools for an MCP.
func (idx *ToolIndex) CountForMCP(mcpName string) int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.byMCP[mcpName])
}

// ListForMCP returns tool names for an MCP.
func (idx *ToolIndex) ListForMCP(mcpName string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	tools := idx.byMCP[mcpName]
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

// GetAllForMCP returns all tool info for an MCP.
func (idx *ToolIndex) GetAllForMCP(mcpName string) []ToolInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	tools := idx.byMCP[mcpName]
	result := make([]ToolInfo, len(tools))
	copy(result, tools)
	return result
}

// GetAll returns all indexed tools.
func (idx *ToolIndex) GetAll() []ToolInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var all []ToolInfo
	for _, tools := range idx.byMCP {
		all = append(all, tools...)
	}
	return all
}
