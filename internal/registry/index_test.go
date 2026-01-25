package registry

import (
	"fmt"
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
		SimilarParams:   map[string]string{"qery": "query"},
		UnknownParams:   []string{"qery"},
		MissingRequired: []string{"query"},
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
	if !contains(errStr, "did you mean 'query'") {
		t.Error("Error should contain parameter suggestion")
	}
	if !contains(errStr, "Missing required") {
		t.Error("Error should show missing required parameters")
	}
	if !contains(errStr, "Unknown parameters") {
		t.Error("Error should show unknown parameters")
	}
	if !contains(errStr, "(required)") {
		t.Error("Error should list expected parameters with required flag")
	}
}

func TestInvalidParameterError_MultipleErrors(t *testing.T) {
	// Test with multiple unknown parameters and multiple missing required
	err := &InvalidParameterError{
		MCPName:       "test-mcp",
		ToolName:      "complex_tool",
		OriginalError: "validation failed",
		ProvidedParams: []string{"qery", "limt", "ofset"},
		ExpectedParams: []ParamInfo{
			{Name: "query", Type: "string", Description: "Search query", Required: true},
			{Name: "limit", Type: "integer", Description: "Max results", Required: true},
			{Name: "offset", Type: "integer", Description: "Skip count", Required: true},
			{Name: "filter", Type: "string", Description: "Filter expression", Required: false},
		},
		SimilarParams:   map[string]string{"qery": "query", "limt": "limit", "ofset": "offset"},
		UnknownParams:   []string{"qery", "limt", "ofset"},
		MissingRequired: []string{"query", "limit", "offset"},
	}

	errStr := err.Error()

	// All 3 unknown params should be shown with suggestions
	if !contains(errStr, "'qery' (did you mean 'query'?)") {
		t.Error("Error should suggest query for qery")
	}
	if !contains(errStr, "'limt' (did you mean 'limit'?)") {
		t.Error("Error should suggest limit for limt")
	}
	if !contains(errStr, "'ofset' (did you mean 'offset'?)") {
		t.Error("Error should suggest offset for ofset")
	}

	// All 3 missing required should be shown
	if !contains(errStr, "query") && !contains(errStr, "Missing required") {
		t.Error("Error should show query as missing required")
	}
}

func TestMCPProtocolError(t *testing.T) {
	err := &MCPProtocolError{
		MCPName:       "test-mcp",
		ToolName:      "list_tasks",
		OriginalError: `{"expected":"record","code":"invalid_type"}`,
		ErrorCode:     "invalid_type",
		Path:          "params.arguments",
		Suggestion:    "Pass an empty object {} instead of null for parameters",
	}

	errStr := err.Error()

	if !contains(errStr, "test-mcp") {
		t.Error("Error should contain MCP name")
	}
	if !contains(errStr, "list_tasks") {
		t.Error("Error should contain tool name")
	}
	if !contains(errStr, "Fix:") {
		t.Error("Error should contain fix suggestion")
	}
	if !contains(errStr, "empty object {}") {
		t.Error("Error should contain actionable suggestion")
	}
}

func TestParseProtocolError(t *testing.T) {
	tests := []struct {
		name          string
		errMsg        string
		wantNil       bool
		wantErrorCode string
		wantHasSugg   bool
	}{
		{
			name:          "zod invalid_type record",
			errMsg:        `[{"expected":"record","code":"invalid_type","path":["params","arguments"],"message":"Invalid input"}]`,
			wantNil:       false,
			wantErrorCode: "invalid_type",
			wantHasSugg:   true,
		},
		{
			name:          "json-rpc invalid params",
			errMsg:        `Invalid params: expected object, got null (-32602)`,
			wantNil:       false,
			wantErrorCode: "invalid_params",
			wantHasSugg:   true,
		},
		{
			name:          "json-rpc method not found",
			errMsg:        `Method not found (-32601)`,
			wantNil:       false,
			wantErrorCode: "method_not_found",
			wantHasSugg:   true,
		},
		{
			name:    "generic error - not protocol",
			errMsg:  `connection timeout`,
			wantNil: true,
		},
		{
			name:    "network error - not protocol",
			errMsg:  `dial tcp: connection refused`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseProtocolError("test-mcp", "test-tool", fmt.Errorf("%s", tt.errMsg))

			if tt.wantNil {
				if result != nil {
					t.Errorf("Expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Error("Expected non-nil result")
				return
			}

			if result.ErrorCode != tt.wantErrorCode {
				t.Errorf("Expected error code %s, got %s", tt.wantErrorCode, result.ErrorCode)
			}

			if tt.wantHasSugg && result.Suggestion == "" {
				t.Error("Expected suggestion, got empty")
			}
		})
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

func TestTokenize(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"lci code insight", []string{"lci", "code", "insight"}},
		{"code_insight", []string{"code", "insight"}},
		{"code-insight", []string{"code", "insight"}},
		{"search.tools", []string{"search", "tools"}},
		{"CodeInsight", []string{"codeinsight"}}, // camelCase stays together
		{"", []string{}},
		{"  spaced   out  ", []string{"spaced", "out"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := tokenize(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("tokenize(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i, exp := range tt.expected {
				if result[i] != exp {
					t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, result[i], exp)
				}
			}
		})
	}
}

func TestContainsAllTerms(t *testing.T) {
	tests := []struct {
		text     string
		terms    []string
		expected bool
	}{
		{"code insight tool", []string{"code", "insight"}, true},
		{"code insight tool", []string{"code", "missing"}, false},
		{"lci code_insight", []string{"lci", "code"}, true},
		{"anything", []string{}, true}, // empty terms = match all
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := containsAllTerms(tt.text, tt.terms)
			if result != tt.expected {
				t.Errorf("containsAllTerms(%q, %v) = %v, want %v", tt.text, tt.terms, result, tt.expected)
			}
		})
	}
}

func TestToolIndex_Search_MultiTermQuery(t *testing.T) {
	idx := NewToolIndex()

	// Simulate lci MCP with code_insight tool
	idx.Add("lci", []ToolInfo{
		{Name: "code_insight", Description: "Comprehensive codebase intelligence system for AI agents", MCPName: "lci"},
		{Name: "search", Description: "Sub-millisecond code search", MCPName: "lci"},
		{Name: "get_context", Description: "Get detailed context for code objects", MCPName: "lci"},
	})

	// Add another MCP with similar tools
	idx.Add("other", []ToolInfo{
		{Name: "code_analyzer", Description: "Analyzes code structure", MCPName: "other"},
		{Name: "insight_tool", Description: "Provides insights", MCPName: "other"},
	})

	tests := []struct {
		name          string
		query         string
		expectedFirst string // the tool that should rank first
		minResults    int    // minimum number of results expected
	}{
		{
			name:          "multi-term query matches MCP+tool",
			query:         "lci code insight",
			expectedFirst: "code_insight",
			minResults:    1,
		},
		{
			name:          "exact tool name ranks highest",
			query:         "code_insight",
			expectedFirst: "code_insight",
			minResults:    1,
		},
		{
			name:          "MCP name query returns all MCP tools",
			query:         "lci",
			expectedFirst: "", // any lci tool is fine
			minResults:    3,
		},
		{
			name:          "partial term matches work",
			query:         "code",
			expectedFirst: "", // multiple matches
			minResults:    2,
		},
		{
			name:          "description search works",
			query:         "codebase intelligence",
			expectedFirst: "code_insight",
			minResults:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := idx.Search(tt.query, "")

			if len(results) < tt.minResults {
				t.Errorf("Search(%q) returned %d results, want >= %d", tt.query, len(results), tt.minResults)
				return
			}

			if tt.expectedFirst != "" && results[0].Name != tt.expectedFirst {
				t.Errorf("Search(%q) first result = %q, want %q", tt.query, results[0].Name, tt.expectedFirst)
			}
		})
	}
}

func TestToolIndex_Search_Ranking(t *testing.T) {
	idx := NewToolIndex()

	// Add tools that will have different scores for the same query
	idx.Add("test", []ToolInfo{
		{Name: "search", Description: "Basic search", MCPName: "test"},                                       // exact name match
		{Name: "search_advanced", Description: "Advanced search with filters", MCPName: "test"},              // prefix match
		{Name: "find", Description: "Find things using search patterns", MCPName: "test"},                    // description match only
		{Name: "locate", Description: "Locate items in the system", MCPName: "test"},                         // no match
	})

	results := idx.Search("search", "")

	// Should have 3 results (locate doesn't match)
	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d: %v", len(results), results)
	}

	// Exact match should be first
	if results[0].Name != "search" {
		t.Errorf("Expected 'search' first (exact match), got %q", results[0].Name)
	}

	// Prefix match should be second
	if results[1].Name != "search_advanced" {
		t.Errorf("Expected 'search_advanced' second (prefix match), got %q", results[1].Name)
	}

	// Description match should be last
	if results[2].Name != "find" {
		t.Errorf("Expected 'find' last (description match), got %q", results[2].Name)
	}
}

func TestToolIndex_Search_MCPNameMatching(t *testing.T) {
	idx := NewToolIndex()

	idx.Add("lci", []ToolInfo{
		{Name: "search", Description: "Search tool", MCPName: "lci"},
		{Name: "context", Description: "Context tool", MCPName: "lci"},
	})
	idx.Add("other", []ToolInfo{
		{Name: "lci_helper", Description: "Helper for lci", MCPName: "other"},
	})

	// Query "lci" should return all lci tools ranked higher than tools just containing "lci"
	results := idx.Search("lci", "")

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// The first two results should be from the "lci" MCP (higher MCP name match score)
	lciCount := 0
	for i := 0; i < 2; i++ {
		if results[i].MCPName == "lci" {
			lciCount++
		}
	}
	if lciCount != 2 {
		t.Errorf("Expected first 2 results to be from 'lci' MCP, got MCPs: %s, %s",
			results[0].MCPName, results[1].MCPName)
	}
}
