package registry

import (
	"sort"
	"strings"
	"sync"
)

// Search scoring constants - higher scores rank first
const (
	scoreExactToolName     = 1000 // query exactly matches tool name
	scoreMCPNameMatch      = 800  // query matches MCP name (high priority - beats prefix+term combos)
	scoreToolNamePrefix    = 300  // tool name starts with query
	scoreAllTermsInName    = 200  // all query terms found in tool name
	scoreAllTermsCrossed   = 150  // all terms found across MCP name + tool name
	scoreAllTermsInDesc    = 100  // all query terms found in description
	scorePartialTermName   = 50   // per term found in tool name
	scorePartialTermDesc   = 25   // per term found in description
	scoreFuzzyMatch        = 10   // fuzzy normalized match (fallback)
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

// tokenize splits a string into lowercase terms, splitting on common separators
func tokenize(s string) []string {
	s = strings.ToLower(s)
	// Replace common separators with spaces
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, ".", " ")

	fields := strings.Fields(s)
	// Filter out empty strings
	result := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			result = append(result, f)
		}
	}
	return result
}

// containsAllTerms checks if all terms are found in the text
func containsAllTerms(text string, terms []string) bool {
	textLower := strings.ToLower(text)
	for _, term := range terms {
		if !strings.Contains(textLower, term) {
			return false
		}
	}
	return true
}

// countMatchingTerms counts how many terms are found in the text
func countMatchingTerms(text string, terms []string) int {
	textLower := strings.ToLower(text)
	count := 0
	for _, term := range terms {
		if strings.Contains(textLower, term) {
			count++
		}
	}
	return count
}

// scoredTool holds a tool with its search relevance score
type scoredTool struct {
	tool  ToolInfo
	score int
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
// Uses multiple ranking strategies to return the most relevant results first:
//   1. Exact tool name match (highest priority)
//   2. MCP name match
//   3. Tool name prefix match
//   4. All query terms in tool name
//   5. All query terms across MCP name + tool name
//   6. All query terms in description
//   7. Partial term matches (scored by count)
//   8. Fuzzy normalized match (fallback)
func (idx *ToolIndex) Search(query, mcpName string) []ToolInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// If no query, return all tools (optionally filtered by MCP)
	if query == "" {
		var results []ToolInfo
		for mcp, tools := range idx.byMCP {
			if mcpName != "" && mcp != mcpName {
				continue
			}
			results = append(results, tools...)
		}
		return results
	}

	queryLower := strings.ToLower(query)
	queryNorm := normalize(query)
	queryTerms := tokenize(query)

	var scored []scoredTool

	for mcp, tools := range idx.byMCP {
		// Filter by MCP name if specified
		if mcpName != "" && mcp != mcpName {
			continue
		}

		mcpLower := strings.ToLower(mcp)

		for _, tool := range tools {
			score := idx.scoreTool(tool, mcp, mcpLower, queryLower, queryNorm, queryTerms)
			if score > 0 {
				scored = append(scored, scoredTool{tool: tool, score: score})
			}
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Extract tools from scored results
	results := make([]ToolInfo, len(scored))
	for i, s := range scored {
		results[i] = s.tool
	}

	return results
}

// scoreTool calculates the relevance score for a tool given a query.
// Returns 0 if the tool doesn't match at all.
func (idx *ToolIndex) scoreTool(tool ToolInfo, mcpName, mcpLower, queryLower, queryNorm string, queryTerms []string) int {
	nameLower := strings.ToLower(tool.Name)
	descLower := strings.ToLower(tool.Description)
	nameNorm := normalize(tool.Name)
	descNorm := normalize(tool.Description)

	score := 0

	// Strategy 1: Exact tool name match (case-insensitive)
	if nameLower == queryLower || nameNorm == queryNorm {
		score += scoreExactToolName
	}

	// Strategy 2: Query matches MCP name
	if mcpLower == queryLower || normalize(mcpName) == queryNorm {
		score += scoreMCPNameMatch
	}

	// Strategy 3: Tool name starts with query
	if strings.HasPrefix(nameLower, queryLower) || strings.HasPrefix(nameNorm, queryNorm) {
		score += scoreToolNamePrefix
	}

	// Multi-term strategies (only if we have terms)
	if len(queryTerms) > 0 {
		// Strategy 4: All terms found in tool name
		if containsAllTerms(nameLower, queryTerms) {
			score += scoreAllTermsInName
		}

		// Strategy 5: All terms found across MCP name + tool name
		combined := mcpLower + " " + nameLower
		if containsAllTerms(combined, queryTerms) {
			score += scoreAllTermsCrossed
		}

		// Strategy 6: All terms found in description
		if containsAllTerms(descLower, queryTerms) {
			score += scoreAllTermsInDesc
		}

		// Strategy 7a: Partial term matches in name
		nameMatches := countMatchingTerms(nameLower, queryTerms)
		if nameMatches > 0 {
			score += nameMatches * scorePartialTermName
		}

		// Strategy 7b: Partial term matches in description
		descMatches := countMatchingTerms(descLower, queryTerms)
		if descMatches > 0 {
			score += descMatches * scorePartialTermDesc
		}
	}

	// Strategy 8: Fuzzy normalized match (fallback)
	if score == 0 {
		if strings.Contains(nameLower, queryLower) ||
			strings.Contains(descLower, queryLower) ||
			strings.Contains(nameNorm, queryNorm) ||
			strings.Contains(descNorm, queryNorm) {
			score += scoreFuzzyMatch
		}
	}

	return score
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

// GetTool returns the ToolInfo for a specific tool on an MCP.
// Returns nil if not found.
func (idx *ToolIndex) GetTool(mcpName, toolName string) *ToolInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	tools, ok := idx.byMCP[mcpName]
	if !ok {
		return nil
	}

	for _, tool := range tools {
		if tool.Name == toolName {
			return &tool
		}
	}
	return nil
}
