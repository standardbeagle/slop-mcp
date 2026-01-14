package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	kdl "github.com/sblinch/kdl-go"
)

// CLIToolPrefix is the prefix added to CLI tool names in the MCP interface.
const CLIToolPrefix = "cli_"

// Registry manages CLI tool definitions.
type Registry struct {
	tools    map[string]*ToolConfig
	executor *Executor
	mu       sync.RWMutex
}

// NewRegistry creates a new CLI tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools:    make(map[string]*ToolConfig),
		executor: NewExecutor(""),
	}
}

// KDLFile represents the structure of a CLI tools KDL file.
type KDLFile struct {
	Tools []ToolConfig `kdl:"cli,multiple"`
}

// LoadFromDirectory loads all CLI tool definitions from a directory.
func (r *Registry) LoadFromDirectory(dir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if directory exists
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return nil // Directory doesn't exist, that's OK
	}
	if err != nil {
		return fmt.Errorf("failed to stat directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", dir)
	}

	// Find all .kdl files
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".kdl") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		if err := r.loadFile(path); err != nil {
			return fmt.Errorf("failed to load %s: %w", path, err)
		}
	}

	return nil
}

// LoadFromFile loads CLI tool definitions from a single KDL file.
func (r *Registry) LoadFromFile(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.loadFile(path)
}

func (r *Registry) loadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var kdlFile KDLFile
	if err := kdl.Unmarshal(data, &kdlFile); err != nil {
		return fmt.Errorf("failed to parse KDL: %w", err)
	}

	for i := range kdlFile.Tools {
		tool := &kdlFile.Tools[i]
		r.tools[tool.Name] = tool
	}

	return nil
}

// Register adds a CLI tool to the registry.
func (r *Registry) Register(tool *ToolConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name] = tool
}

// Unregister removes a CLI tool from the registry.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// Get retrieves a CLI tool by name.
func (r *Registry) Get(name string) *ToolConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try with and without prefix
	if tool, ok := r.tools[name]; ok {
		return tool
	}

	// Try stripping prefix
	if strings.HasPrefix(name, CLIToolPrefix) {
		stripped := strings.TrimPrefix(name, CLIToolPrefix)
		if tool, ok := r.tools[stripped]; ok {
			return tool
		}
	}

	return nil
}

// List returns all registered CLI tools.
func (r *Registry) List() []*ToolConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]*ToolConfig, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// Count returns the number of registered CLI tools.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Execute runs a CLI tool with the given parameters.
func (r *Registry) Execute(ctx context.Context, name string, params map[string]any) (*ToolResult, error) {
	tool := r.Get(name)
	if tool == nil {
		return nil, fmt.Errorf("CLI tool not found: %s", name)
	}

	return r.executor.Execute(ctx, tool, params)
}

// ToolInfo represents a CLI tool for the MCP tool index.
type ToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	MCPName     string         `json:"mcp_name"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// GetToolInfos returns tool info for all registered CLI tools.
func (r *Registry) GetToolInfos() []ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]ToolInfo, 0, len(r.tools))
	for _, tool := range r.tools {
		infos = append(infos, ToolInfo{
			Name:        CLIToolPrefix + tool.Name,
			Description: tool.Description,
			MCPName:     "cli",
			InputSchema: tool.GenerateInputSchema(),
		})
	}
	return infos
}

// IsCLITool checks if a tool name refers to a CLI tool.
func IsCLITool(name string) bool {
	return strings.HasPrefix(name, CLIToolPrefix)
}

// StripCLIPrefix removes the CLI prefix from a tool name.
func StripCLIPrefix(name string) string {
	return strings.TrimPrefix(name, CLIToolPrefix)
}
