package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestToolConfig_GenerateInputSchema(t *testing.T) {
	tool := &ToolConfig{
		Name:        "jq",
		Description: "JSON processor",
		Command:     "jq",
		Args: []ArgConfig{
			{Name: "filter", Description: "jq filter", Required: true, Position: 0, Type: "string"},
		},
		Flags: []FlagConfig{
			{Name: "raw-output", Short: "-r", Description: "Raw output", Type: "boolean"},
			{Name: "compact", Short: "-c", Description: "Compact", Type: "boolean"},
		},
		Stdin: &StdinConfig{Description: "JSON input", Type: "string"},
	}

	schema := tool.GenerateInputSchema()

	// Check type
	if schema["type"] != "object" {
		t.Errorf("expected type 'object', got %v", schema["type"])
	}

	// Check properties
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be a map")
	}

	// Check filter arg
	filter, ok := props["filter"].(map[string]any)
	if !ok {
		t.Fatal("expected filter property")
	}
	if filter["type"] != "string" {
		t.Errorf("expected filter type 'string', got %v", filter["type"])
	}

	// Check raw_output flag (converted from raw-output)
	rawOutput, ok := props["raw_output"].(map[string]any)
	if !ok {
		t.Fatal("expected raw_output property")
	}
	if rawOutput["type"] != "boolean" {
		t.Errorf("expected raw_output type 'boolean', got %v", rawOutput["type"])
	}

	// Check stdin
	stdin, ok := props["stdin"].(map[string]any)
	if !ok {
		t.Fatal("expected stdin property")
	}
	if stdin["type"] != "string" {
		t.Errorf("expected stdin type 'string', got %v", stdin["type"])
	}

	// Check required
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("expected required to be string slice")
	}
	if len(required) != 1 || required[0] != "filter" {
		t.Errorf("expected required ['filter'], got %v", required)
	}
}

func TestRegistry_LoadFromDirectory(t *testing.T) {
	// Create temp directory with test file
	tmpDir, err := os.MkdirTemp("", "cli-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write test KDL file
	kdlContent := `
cli "echo" {
    description "Echo input"
    command "echo"

    arg "message" {
        description "Message to echo"
        required true
        position 0
    }
}

cli "cat" {
    description "Concatenate files"
    command "cat"

    arg "file" {
        description "File to read"
        required true
        position 0
    }
}
`
	testFile := filepath.Join(tmpDir, "test.kdl")
	if err := os.WriteFile(testFile, []byte(kdlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load registry
	reg := NewRegistry()
	if err := reg.LoadFromDirectory(tmpDir); err != nil {
		t.Fatalf("LoadFromDirectory failed: %v", err)
	}

	// Verify tools loaded
	if reg.Count() != 2 {
		t.Errorf("expected 2 tools, got %d", reg.Count())
	}

	// Verify echo tool
	echo := reg.Get("echo")
	if echo == nil {
		t.Fatal("expected echo tool")
	}
	if echo.Description != "Echo input" {
		t.Errorf("expected description 'Echo input', got %s", echo.Description)
	}
	if echo.Command != "echo" {
		t.Errorf("expected command 'echo', got %s", echo.Command)
	}
}

func TestExecutor_Execute(t *testing.T) {
	tool := &ToolConfig{
		Name:        "echo",
		Description: "Echo test",
		Command:     "echo",
		Args: []ArgConfig{
			{Name: "message", Required: true, Position: 0, Type: "string"},
		},
	}

	executor := NewExecutor("")
	result, err := executor.Execute(context.Background(), tool, map[string]any{
		"message": "hello world",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	if result.Stdout != "hello world" {
		t.Errorf("expected stdout 'hello world', got '%s'", result.Stdout)
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"raw-output", "raw_output"},
		{"rawOutput", "raw_output"},
		{"RawOutput", "raw_output"},
		{"RAW_OUTPUT", "raw_output"},
		{"simple", "simple"},
		{"with spaces", "with_spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toSnakeCase(tt.input)
			if result != tt.expected {
				t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsCLITool(t *testing.T) {
	if !IsCLITool("cli_jq") {
		t.Error("expected cli_jq to be CLI tool")
	}
	if IsCLITool("jq") {
		t.Error("expected jq to not be CLI tool")
	}
}

func TestStripCLIPrefix(t *testing.T) {
	if StripCLIPrefix("cli_jq") != "jq" {
		t.Error("expected StripCLIPrefix(cli_jq) = jq")
	}
	if StripCLIPrefix("jq") != "jq" {
		t.Error("expected StripCLIPrefix(jq) = jq")
	}
}
