package registry

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"code_insight", "codeinsight"},
		{"code-insight", "codeinsight"},
		{"code insight", "codeinsight"},
		{"CodeInsight", "codeinsight"},
		{"CODE_INSIGHT", "codeinsight"},
		{"code_insight_tool", "codeinsighttool"},
		{"", ""},
		{"simple", "simple"},
		{"UPPER", "upper"},
		{"mixed-case_with spaces", "mixedcasewithspaces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalize(tt.input)
			if result != tt.expected {
				t.Errorf("normalize(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestToolIndex_Search_FuzzyMatching(t *testing.T) {
	idx := NewToolIndex()

	// Add tools with various naming conventions
	idx.Add("test-mcp", []ToolInfo{
		{Name: "code_insight", Description: "Provides code analysis"},
		{Name: "search-tools", Description: "Search for tools"},
		{Name: "getUser", Description: "Get user info"},
		{Name: "list_all_items", Description: "Lists all items in database"},
	})

	tests := []struct {
		name     string
		query    string
		expected []string // expected tool names in result
	}{
		{
			name:     "underscore matches space",
			query:    "code insight",
			expected: []string{"code_insight"},
		},
		{
			name:     "space matches underscore",
			query:    "code_insight",
			expected: []string{"code_insight"},
		},
		{
			name:     "hyphen matches space",
			query:    "search tools",
			expected: []string{"search-tools"},
		},
		{
			name:     "space matches hyphen",
			query:    "search-tools",
			expected: []string{"search-tools"},
		},
		{
			name:     "case insensitive camelCase",
			query:    "getuser",
			expected: []string{"getUser"},
		},
		{
			name:     "partial match with spaces",
			query:    "all items",
			expected: []string{"list_all_items"},
		},
		{
			name:     "mixed separator query",
			query:    "list-all items",
			expected: []string{"list_all_items"},
		},
		{
			name:     "empty query returns all",
			query:    "",
			expected: []string{"code_insight", "search-tools", "getUser", "list_all_items"},
		},
		{
			name:     "no match",
			query:    "nonexistent",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := idx.Search(tt.query, "")

			if len(results) != len(tt.expected) {
				t.Errorf("Search(%q) returned %d results, want %d", tt.query, len(results), len(tt.expected))
				return
			}

			// Check that all expected tools are in results
			resultNames := make(map[string]bool)
			for _, r := range results {
				resultNames[r.Name] = true
			}

			for _, expected := range tt.expected {
				if !resultNames[expected] {
					t.Errorf("Search(%q) missing expected tool %q", tt.query, expected)
				}
			}
		})
	}
}

func TestToolIndex_Search_DescriptionMatching(t *testing.T) {
	idx := NewToolIndex()

	idx.Add("test-mcp", []ToolInfo{
		{Name: "analyze", Description: "Provides code_insight analysis"},
		{Name: "helper", Description: "A search-tools helper"},
	})

	tests := []struct {
		name     string
		query    string
		expected []string
	}{
		{
			name:     "fuzzy match in description",
			query:    "code insight",
			expected: []string{"analyze"},
		},
		{
			name:     "hyphen match in description",
			query:    "search tools",
			expected: []string{"helper"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := idx.Search(tt.query, "")

			if len(results) != len(tt.expected) {
				t.Errorf("Search(%q) returned %d results, want %d", tt.query, len(results), len(tt.expected))
				return
			}

			for i, expected := range tt.expected {
				if results[i].Name != expected {
					t.Errorf("Search(%q) result[%d] = %q, want %q", tt.query, i, results[i].Name, expected)
				}
			}
		})
	}
}

func TestToolIndex_Search_MCPFiltering(t *testing.T) {
	idx := NewToolIndex()

	idx.Add("mcp1", []ToolInfo{
		{Name: "code_insight", Description: "MCP1 insight tool"},
	})
	idx.Add("mcp2", []ToolInfo{
		{Name: "code_analysis", Description: "MCP2 analysis tool"},
	})

	// Search with MCP filter
	results := idx.Search("code", "mcp1")
	if len(results) != 1 {
		t.Errorf("Expected 1 result with MCP filter, got %d", len(results))
	}
	if len(results) > 0 && results[0].Name != "code_insight" {
		t.Errorf("Expected code_insight, got %s", results[0].Name)
	}

	// Search without filter should return both
	results = idx.Search("code", "")
	if len(results) != 2 {
		t.Errorf("Expected 2 results without filter, got %d", len(results))
	}
}

func TestToolIndex_GetTool(t *testing.T) {
	idx := NewToolIndex()

	idx.Add("test-mcp", []ToolInfo{
		{
			Name:        "search",
			Description: "Search tool",
			MCPName:     "test-mcp",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search query",
					},
				},
			},
		},
	})

	// Found
	tool := idx.GetTool("test-mcp", "search")
	if tool == nil {
		t.Fatal("Expected to find tool 'search'")
	}
	if tool.Name != "search" {
		t.Errorf("Expected name 'search', got %q", tool.Name)
	}
	if tool.InputSchema == nil {
		t.Error("Expected InputSchema to be set")
	}

	// Not found - wrong MCP
	tool = idx.GetTool("other-mcp", "search")
	if tool != nil {
		t.Error("Expected nil for wrong MCP")
	}

	// Not found - wrong tool
	tool = idx.GetTool("test-mcp", "nonexistent")
	if tool != nil {
		t.Error("Expected nil for nonexistent tool")
	}
}

func TestNormalizeParam(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"mcp_name", "mcpname"},
		{"mcp-name", "mcpname"},
		{"mcpName", "mcpname"},
		{"MCP_NAME", "mcpname"},
		{"tool name", "toolname"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeParam(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeParam(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSimilarity(t *testing.T) {
	tests := []struct {
		a        string
		b        string
		minScore int // minimum expected score
	}{
		// Identical strings
		{"test", "test", 100},
		// Substring matches
		{"name", "mcpname", 50},
		{"mcp", "mcpname", 40},
		// Similar strings
		{"query", "qery", 70},
		// Completely different
		{"abc", "xyz", 0},
		// Empty strings
		{"", "test", 0},
		{"test", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			result := similarity(tt.a, tt.b)
			if result < tt.minScore {
				t.Errorf("similarity(%q, %q) = %d, want >= %d", tt.a, tt.b, result, tt.minScore)
			}
		})
	}
}

func TestFindSimilarParams(t *testing.T) {
	expectedParams := []ParamInfo{
		{Name: "mcp_name", Type: "string", Required: true},
		{Name: "tool_name", Type: "string", Required: true},
		{Name: "parameters", Type: "object", Required: false},
		{Name: "query", Type: "string", Required: false},
	}

	tests := []struct {
		name     string
		provided []string
		expected map[string]string
	}{
		{
			name:     "typo in parameter name",
			provided: []string{"mcp_nam", "tool_name"},
			expected: map[string]string{"mcp_nam": "mcp_name"},
		},
		{
			name:     "normalized match no suggestion",
			provided: []string{"mcpname", "tool_name"}, // mcpname normalizes to mcp_name
			expected: map[string]string{},
		},
		{
			name:     "exact match no suggestion",
			provided: []string{"mcp_name", "tool_name"},
			expected: map[string]string{},
		},
		{
			name:     "completely wrong param",
			provided: []string{"xyz123"},
			expected: map[string]string{},
		},
		{
			name:     "similar to query",
			provided: []string{"qery"},
			expected: map[string]string{"qery": "query"},
		},
		{
			name:     "suggest for partial typo",
			provided: []string{"paramters"}, // missing 'e'
			expected: map[string]string{"paramters": "parameters"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findSimilarParams(tt.provided, expectedParams)

			if len(result) != len(tt.expected) {
				t.Errorf("findSimilarParams() returned %d suggestions, want %d", len(result), len(tt.expected))
				t.Logf("Got: %v", result)
				return
			}

			for provided, expectedSuggestion := range tt.expected {
				if result[provided] != expectedSuggestion {
					t.Errorf("findSimilarParams()[%q] = %q, want %q", provided, result[provided], expectedSuggestion)
				}
			}
		})
	}
}

func TestExtractParamsFromSchema(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mcp_name": map[string]any{
				"type":        "string",
				"description": "Target MCP server name",
			},
			"tool_name": map[string]any{
				"type":        "string",
				"description": "Tool to execute",
			},
			"parameters": map[string]any{
				"type":        "object",
				"description": "Tool parameters",
			},
		},
		"required": []any{"mcp_name", "tool_name"},
	}

	params := extractParamsFromSchema(schema)

	if len(params) != 3 {
		t.Fatalf("Expected 3 params, got %d", len(params))
	}

	// Build map for easier testing
	paramMap := make(map[string]ParamInfo)
	for _, p := range params {
		paramMap[p.Name] = p
	}

	// Check mcp_name
	if p, ok := paramMap["mcp_name"]; !ok {
		t.Error("Expected mcp_name parameter")
	} else {
		if !p.Required {
			t.Error("mcp_name should be required")
		}
		if p.Type != "string" {
			t.Errorf("mcp_name type = %q, want 'string'", p.Type)
		}
		if p.Description != "Target MCP server name" {
			t.Errorf("mcp_name description = %q", p.Description)
		}
	}

	// Check parameters (optional)
	if p, ok := paramMap["parameters"]; !ok {
		t.Error("Expected parameters parameter")
	} else {
		if p.Required {
			t.Error("parameters should not be required")
		}
	}

	// Test with nil schema
	params = extractParamsFromSchema(nil)
	if params != nil {
		t.Error("Expected nil for nil schema")
	}

	// Test with empty schema
	params = extractParamsFromSchema(map[string]any{})
	if params != nil {
		t.Error("Expected nil for empty schema")
	}
}

func TestInvalidParameterError(t *testing.T) {
	err := &InvalidParameterError{
		MCPName:        "test-mcp",
		ToolName:       "search",
		OriginalError:  "unknown parameter 'qery'",
		ProvidedParams: []string{"qery", "limit"},
		ExpectedParams: []ParamInfo{
			{Name: "query", Type: "string", Description: "Search query", Required: true},
			{Name: "limit", Type: "integer", Description: "Max results", Required: false},
		},
		SimilarParams: map[string]string{"qery": "query"},
	}

	errStr := err.Error()

	// Check error contains key information
	if !contains(errStr, "test-mcp") {
		t.Error("Error should contain MCP name")
	}
	if !contains(errStr, "search") {
		t.Error("Error should contain tool name")
	}
	if !contains(errStr, "unknown parameter 'qery'") {
		t.Error("Error should contain original error")
	}
	if !contains(errStr, "'qery' -> 'query'") {
		t.Error("Error should contain parameter suggestion")
	}
	if !contains(errStr, "query") && !contains(errStr, "(required)") {
		t.Error("Error should list expected parameters with required flag")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
