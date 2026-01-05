package registry

import (
	"strings"
	"sync"
)

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
func (idx *ToolIndex) Search(query, mcpName string) []ToolInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []ToolInfo
	queryLower := strings.ToLower(query)

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

			// Match against name or description
			if strings.Contains(strings.ToLower(tool.Name), queryLower) ||
				strings.Contains(strings.ToLower(tool.Description), queryLower) {
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
