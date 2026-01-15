package builtins

import (
	"testing"

	"github.com/standardbeagle/slop/pkg/slop"
)

func TestRegisterSlopSearch(t *testing.T) {
	rt := slop.NewRuntime()
	defer rt.Close()

	RegisterSlopSearch(rt)

	// Test slop_search is registered and works
	result, err := rt.Execute(`slop_search("map")`)
	if err != nil {
		t.Fatalf("slop_search failed: %v", err)
	}

	lv, ok := result.(*slop.ListValue)
	if !ok {
		t.Fatalf("expected ListValue, got %T", result)
	}

	if len(lv.Elements) == 0 {
		t.Error("expected results for 'map' search")
	}
}

func TestSlopSearch(t *testing.T) {
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterSlopSearch(rt)

	tests := []struct {
		name       string
		script     string
		minResults int
	}{
		{
			name:       "search map",
			script:     `slop_search("map")`,
			minResults: 1,
		},
		{
			name:       "search string functions",
			script:     `slop_search("upper")`,
			minResults: 1,
		},
		{
			name:       "empty query returns all",
			script:     `slop_search("")`,
			minResults: 20, // default limit
		},
		{
			name:       "with limit",
			script:     `slop_search("", 5)`,
			minResults: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := rt.Execute(tt.script)
			if err != nil {
				t.Fatalf("execution failed: %v", err)
			}

			lv, ok := result.(*slop.ListValue)
			if !ok {
				t.Fatalf("expected ListValue, got %T", result)
			}

			if len(lv.Elements) < tt.minResults {
				t.Errorf("expected at least %d results, got %d", tt.minResults, len(lv.Elements))
			}
		})
	}
}

func TestSlopCategories(t *testing.T) {
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterSlopSearch(rt)

	result, err := rt.Execute(`slop_categories()`)
	if err != nil {
		t.Fatalf("slop_categories failed: %v", err)
	}

	lv, ok := result.(*slop.ListValue)
	if !ok {
		t.Fatalf("expected ListValue, got %T", result)
	}

	// Should have multiple categories
	if len(lv.Elements) < 5 {
		t.Errorf("expected at least 5 categories, got %d", len(lv.Elements))
	}
}

func TestSlopHelp(t *testing.T) {
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterSlopSearch(rt)

	result, err := rt.Execute(`slop_help("map")`)
	if err != nil {
		t.Fatalf("slop_help failed: %v", err)
	}

	mv, ok := result.(*slop.MapValue)
	if !ok {
		t.Fatalf("expected MapValue, got %T", result)
	}

	// Check required fields
	if _, ok := mv.Pairs["name"]; !ok {
		t.Error("missing 'name' field")
	}
	if _, ok := mv.Pairs["signature"]; !ok {
		t.Error("missing 'signature' field")
	}
	if _, ok := mv.Pairs["description"]; !ok {
		t.Error("missing 'description' field")
	}
}

func TestSlopHelpNotFound(t *testing.T) {
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterSlopSearch(rt)

	result, err := rt.Execute(`slop_help("nonexistent_function")`)
	if err != nil {
		t.Fatalf("slop_help failed: %v", err)
	}

	mv, ok := result.(*slop.MapValue)
	if !ok {
		t.Fatalf("expected MapValue, got %T", result)
	}

	// Should have error field
	if _, ok := mv.Pairs["error"]; !ok {
		t.Error("expected 'error' field for not found")
	}
}

func TestSearchSlopFunctions(t *testing.T) {
	// Test the exported search function
	results := SearchSlopFunctions("string", "", 0)
	if len(results) == 0 {
		t.Error("expected results for 'string' search")
	}

	// Test with category filter
	results = SearchSlopFunctions("", "math", 0)
	for _, fn := range results {
		if fn.Category != "math" {
			t.Errorf("expected category 'math', got '%s'", fn.Category)
		}
	}

	// Test with limit
	results = SearchSlopFunctions("", "", 5)
	if len(results) > 5 {
		t.Errorf("expected max 5 results, got %d", len(results))
	}
}

func TestGetCategories(t *testing.T) {
	categories := GetCategories()

	if len(categories) < 5 {
		t.Errorf("expected at least 5 categories, got %d", len(categories))
	}

	// Check some expected categories
	expectedCategories := []string{"string", "list", "math", "random", "type"}
	for _, cat := range expectedCategories {
		if _, ok := categories[cat]; !ok {
			t.Errorf("missing expected category: %s", cat)
		}
	}
}

func TestSlopReferenceComplete(t *testing.T) {
	// Verify all functions have required fields
	for _, fn := range SlopReference {
		if fn.Name == "" {
			t.Error("found function with empty name")
		}
		if fn.Category == "" {
			t.Errorf("function %s has empty category", fn.Name)
		}
		if fn.Signature == "" {
			t.Errorf("function %s has empty signature", fn.Name)
		}
		if fn.Description == "" {
			t.Errorf("function %s has empty description", fn.Name)
		}
	}
}
