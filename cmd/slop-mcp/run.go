package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/builtins"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop/pkg/slop"
)

func cmdRun(args []string) {
	if len(args) < 1 {
		printRunUsage()
		os.Exit(1)
	}

	var scriptFile string
	var inlineScript string
	timeout := 5 * time.Minute
	outputJSON := false

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-e" && i+1 < len(args):
			inlineScript = args[i+1]
			i++
		case args[i] == "--json":
			outputJSON = true
		case strings.HasPrefix(args[i], "--timeout="):
			d, err := parseTimeoutValue(strings.TrimPrefix(args[i], "--timeout="))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			timeout = d
		case args[i] == "--help" || args[i] == "-h":
			printRunUsage()
			return
		case !strings.HasPrefix(args[i], "-"):
			if scriptFile == "" {
				scriptFile = args[i]
			}
		}
	}

	if scriptFile == "" && inlineScript == "" {
		fmt.Fprintln(os.Stderr, "Error: either script file or -e '<script>' is required")
		printRunUsage()
		os.Exit(1)
	}

	// Get script content
	var script string
	if inlineScript != "" {
		script = inlineScript
	} else {
		data, err := os.ReadFile(scriptFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading script file: %v\n", err)
			os.Exit(1)
		}
		script = string(data)
	}

	// Load merged three-tier config (same path as serve)
	cwd, _ := os.Getwd()
	cfg, err := config.Load(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Create SLOP runtime
	rt := builtins.NewRuntime()
	defer rt.Close()

	// Register built-in functions
	builtins.RegisterCrypto(rt)
	builtins.RegisterSlopSearch(rt)
	builtins.RegisterJWT(rt)
	builtins.RegisterTemplate(rt)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Connect all configured MCPs
	for _, mcpCfg := range cfg.MCPs {
		// Normalize type for SLOP runtime
		transportType := mcpCfg.Type
		if transportType == "stdio" {
			transportType = "command"
		}

		slopCfg := slop.MCPConfig{
			Name:    mcpCfg.Name,
			Type:    transportType,
			Command: mcpCfg.Command,
			Args:    mcpCfg.Args,
			Env:     mapToSlice(mcpCfg.Env),
			URL:     mcpCfg.URL,
			Headers: mcpCfg.Headers,
		}

		if err := rt.ConnectMCP(ctx, slopCfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to connect MCP %s: %v\n", mcpCfg.Name, err)
		}
	}

	// Execute script
	result, err := rt.Execute(script)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Script execution error: %v\n", err)
		os.Exit(1)
	}

	// Collect emitted values
	rawEmitted := rt.Emitted()
	emitted := make([]any, 0, len(rawEmitted))
	for _, v := range rawEmitted {
		emitted = append(emitted, convertValue(v))
	}

	if outputJSON {
		output := map[string]any{
			"result":  convertValue(result),
			"emitted": emitted,
		}
		pretty, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(pretty))
	} else {
		// Print result
		if result != nil {
			fmt.Println(formatAnyValue(result))
		}

		// Print emitted values
		if len(emitted) > 0 {
			fmt.Println("\nEmitted values:")
			for i, v := range emitted {
				fmt.Printf("  [%d] %s\n", i, formatAnyValue(v))
			}
		}
	}
}

// parseTimeoutValue parses a --timeout value: either a Go duration string
// ("30s", "5m") or a bare number of seconds ("300").
func parseTimeoutValue(s string) (time.Duration, error) {
	if d, err := time.ParseDuration(s); err == nil {
		if d <= 0 {
			return 0, fmt.Errorf("--timeout must be positive, got %q", s)
		}
		return d, nil
	}
	secs, err := strconv.Atoi(s)
	if err != nil || secs <= 0 {
		return 0, fmt.Errorf("invalid --timeout value %q: expected seconds (e.g. 300) or a duration (e.g. \"30s\", \"5m\")", s)
	}
	return time.Duration(secs) * time.Second, nil
}

func mapToSlice(m map[string]string) []string {
	result := make([]string, 0, len(m))
	for k, v := range m {
		result = append(result, k+"="+v)
	}
	return result
}

func convertValue(v any) any {
	if v == nil {
		return nil
	}

	// If it's already a basic type, return as-is
	switch val := v.(type) {
	case bool, int, int64, float64, string:
		return val
	case []any:
		return val
	case map[string]any:
		return val
	}

	// Try to convert via JSON for complex types
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}

	var result any
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Sprintf("%v", v)
	}

	return result
}

func formatAnyValue(v any) string {
	if v == nil {
		return "null"
	}

	switch val := v.(type) {
	case string:
		return val
	case nil:
		return "null"
	default:
		data, err := json.MarshalIndent(val, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(data)
	}
}

func printRunUsage() {
	fmt.Print(`slop-mcp run - Execute SLOP scripts

Usage:
  slop-mcp run <script.slop>       Execute a script file
  slop-mcp run -e '<script>'       Execute an inline script

Options:
  -e '<script>'      Execute inline script
  --json             Output as JSON
  --timeout=<value>  Execution timeout: seconds or duration like "30s", "5m"
                     (default: 300)
  --help, -h         Show this help

This command executes SLOP scripts with access to all configured MCPs.
MCPs are loaded from user, project, and local config files.

Examples:
  slop-mcp run hello.slop              # Execute script file
  slop-mcp run -e 'emit "hello"'       # Execute inline script
  slop-mcp run -e 'call fs:list_files {path: "/tmp"}' --json

Notes:
  - MCPs are connected before script execution
  - Script execution is standalone (no running server needed)
  - Use 'emit' in scripts to output values
`)
}
