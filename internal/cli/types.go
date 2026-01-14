package cli

import (
	"encoding/json"
	"time"
)

// ToolConfig represents a CLI tool definition.
type ToolConfig struct {
	Name        string            `kdl:",arg"`
	Description string            `kdl:"description"`
	Command     string            `kdl:"command"`
	Args        []ArgConfig       `kdl:"arg,multiple"`
	Flags       []FlagConfig      `kdl:"flag,multiple"`
	Stdin       *StdinConfig      `kdl:"stdin"`
	Stdout      *StdoutConfig     `kdl:"stdout"`
	Stderr      *StderrConfig     `kdl:"stderr"`
	Timeout     int               `kdl:"timeout"` // milliseconds
	Workdir     string            `kdl:"workdir"`
	Env         map[string]string `kdl:"env"`
	ExpandEnv   bool              `kdl:"expand_env"`
	Shell       bool              `kdl:"shell"`
	AllowFail   bool              `kdl:"allow_failure"`
}

// ArgConfig represents a positional argument.
type ArgConfig struct {
	Name        string   `kdl:",arg"`
	Description string   `kdl:"description"`
	Required    bool     `kdl:"required"`
	Position    int      `kdl:"position"`
	Type        string   `kdl:"type"`    // "string", "number", "boolean", "array"
	Default     any      `kdl:"default"` // default value
	Enum        []string `kdl:"enum"`    // allowed values
}

// FlagConfig represents a command-line flag.
type FlagConfig struct {
	Name        string   `kdl:",arg"`
	Short       string   `kdl:"short"`
	Long        string   `kdl:"long"`
	Description string   `kdl:"description"`
	Type        string   `kdl:"type"`      // "boolean", "string", "number", "array"
	Default     any      `kdl:"default"`   // default value
	Enum        []string `kdl:"enum"`      // allowed values
	Separator   string   `kdl:"separator"` // for array: how to join values
	Repeat      bool     `kdl:"repeat"`    // for array: repeat flag per value
}

// StdinConfig defines stdin handling.
type StdinConfig struct {
	Description string `kdl:"description"`
	Type        string `kdl:"type"`   // input type for schema
	Format      string `kdl:"format"` // "json", "text", "binary"
	Required    bool   `kdl:"required"`
}

// StdoutConfig defines stdout handling.
type StdoutConfig struct {
	Type     string `kdl:"type"`     // output type
	Format   string `kdl:"format"`   // "json", "text", "auto"
	Trim     bool   `kdl:"trim"`     // trim whitespace (default true)
	Encoding string `kdl:"encoding"` // "utf8", "base64"
}

// StderrConfig defines stderr handling.
type StderrConfig struct {
	Capture      bool `kdl:"capture"`        // include in response
	FailOnOutput bool `kdl:"fail_on_output"` // treat output as failure
}

// ToolResult represents the result of executing a CLI tool.
type ToolResult struct {
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code"`
	Duration int64  `json:"duration_ms"`
	Error    string `json:"error,omitempty"`
}

// GetTimeout returns the timeout duration, defaulting to 30 seconds.
func (t *ToolConfig) GetTimeout() time.Duration {
	if t.Timeout <= 0 {
		return 30 * time.Second
	}
	return time.Duration(t.Timeout) * time.Millisecond
}

// GenerateInputSchema generates an MCP-compatible JSON Schema for the tool.
func (t *ToolConfig) GenerateInputSchema() map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}

	props := schema["properties"].(map[string]any)
	required := []string{}

	// Add positional arguments
	for _, arg := range t.Args {
		prop := map[string]any{
			"description": arg.Description,
		}

		switch arg.Type {
		case "number":
			prop["type"] = "number"
		case "boolean":
			prop["type"] = "boolean"
		case "array":
			prop["type"] = "array"
			prop["items"] = map[string]any{"type": "string"}
		default:
			prop["type"] = "string"
		}

		if len(arg.Enum) > 0 {
			prop["enum"] = arg.Enum
		}

		if arg.Default != nil {
			prop["default"] = arg.Default
		}

		// Use snake_case for property names
		propName := toSnakeCase(arg.Name)
		props[propName] = prop

		if arg.Required {
			required = append(required, propName)
		}
	}

	// Add flags
	for _, flag := range t.Flags {
		prop := map[string]any{
			"description": flag.Description,
		}

		switch flag.Type {
		case "number":
			prop["type"] = "number"
		case "boolean":
			prop["type"] = "boolean"
		case "array":
			prop["type"] = "array"
			prop["items"] = map[string]any{"type": "string"}
		default:
			prop["type"] = "string"
		}

		if len(flag.Enum) > 0 {
			prop["enum"] = flag.Enum
		}

		if flag.Default != nil {
			prop["default"] = flag.Default
		}

		// Use snake_case for property names
		propName := toSnakeCase(flag.Name)
		props[propName] = prop
	}

	// Add stdin if defined
	if t.Stdin != nil {
		prop := map[string]any{
			"type":        "string",
			"description": t.Stdin.Description,
		}
		props["stdin"] = prop

		if t.Stdin.Required {
			required = append(required, "stdin")
		}
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// GenerateOutputSchema generates an output schema for the tool.
func (t *ToolConfig) GenerateOutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"stdout": map[string]any{
				"type":        "string",
				"description": "Standard output from the command",
			},
			"stderr": map[string]any{
				"type":        "string",
				"description": "Standard error from the command",
			},
			"exit_code": map[string]any{
				"type":        "integer",
				"description": "Exit code from the command",
			},
		},
	}
}

// ToJSON converts the tool config to a JSON representation.
func (t *ToolConfig) ToJSON() string {
	data, _ := json.MarshalIndent(t, "", "  ")
	return string(data)
}
