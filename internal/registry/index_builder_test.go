package registry

// mutableIndex is a test-only helper for building ToolIndex instances
// incrementally. Production code constructs snapshots via buildIndex directly;
// this builder lives in a _test.go file so it never ships in the binary.
type mutableIndex struct {
	data map[string][]ToolInfo
}

// NewToolIndex returns a mutable builder for constructing ToolIndex snapshots.
func NewToolIndex() *mutableIndex {
	return &mutableIndex{data: make(map[string][]ToolInfo)}
}

// Add adds or replaces tools for the named MCP.
func (m *mutableIndex) Add(mcpName string, tools []ToolInfo) {
	m.data[mcpName] = tools
}

// snapshot returns an immutable ToolIndex from the current state.
func (m *mutableIndex) snapshot() *ToolIndex {
	return buildIndex(m.data, nil)
}

// Search delegates to the current immutable snapshot.
func (m *mutableIndex) Search(query, mcpName string) []ToolInfo {
	return m.snapshot().Search(query, mcpName)
}

// GetTool delegates to the current immutable snapshot.
func (m *mutableIndex) GetTool(mcpName, toolName string) *ToolInfo {
	return m.snapshot().GetTool(mcpName, toolName)
}

// CountForMCP delegates to the current immutable snapshot.
func (m *mutableIndex) CountForMCP(mcpName string) int {
	return m.snapshot().CountForMCP(mcpName)
}
