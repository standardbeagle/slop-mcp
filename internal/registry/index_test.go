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
