package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/auth"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/registry"
)

func cmdMCPAdd(args []string) {
	if len(args) < 1 {
		printMCPAddUsage()
		os.Exit(1)
	}

	// Parse flags
	name := ""
	command := ""
	var cmdArgs []string
	scope := config.ScopeProject
	transport := "stdio"
	env := make(map[string]string)
	headers := make(map[string]string)
	url := ""

	var positional []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--local" || args[i] == "-l":
			scope = config.ScopeLocal
		case args[i] == "--project" || args[i] == "-p":
			scope = config.ScopeProject
		case args[i] == "--user" || args[i] == "-u":
			scope = config.ScopeUser
		case args[i] == "--scope" || args[i] == "-s":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --scope requires a value (local, project, or user)")
				os.Exit(1)
			}
			i++
			switch args[i] {
			case "local":
				scope = config.ScopeLocal
			case "project":
				scope = config.ScopeProject
			case "user":
				scope = config.ScopeUser
			default:
				fmt.Fprintf(os.Stderr, "Error: invalid scope '%s' (must be local, project, or user)\n", args[i])
				os.Exit(1)
			}
		case strings.HasPrefix(args[i], "--scope="):
			scopeVal := strings.TrimPrefix(args[i], "--scope=")
			switch scopeVal {
			case "local":
				scope = config.ScopeLocal
			case "project":
				scope = config.ScopeProject
			case "user":
				scope = config.ScopeUser
			default:
				fmt.Fprintf(os.Stderr, "Error: invalid scope '%s' (must be local, project, or user)\n", scopeVal)
				os.Exit(1)
			}
		case args[i] == "--transport" || args[i] == "-t":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --transport requires a value")
				os.Exit(1)
			}
			i++
			transport = args[i]
		case strings.HasPrefix(args[i], "--transport="):
			transport = strings.TrimPrefix(args[i], "--transport=")
		case args[i] == "--url":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --url requires a value")
				os.Exit(1)
			}
			i++
			url = args[i]
		case strings.HasPrefix(args[i], "--url="):
			url = strings.TrimPrefix(args[i], "--url=")
		case args[i] == "--env" || args[i] == "-e":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --env requires KEY=VALUE")
				os.Exit(1)
			}
			i++
			kv := strings.SplitN(args[i], "=", 2)
			if len(kv) != 2 {
				fmt.Fprintf(os.Stderr, "Error: invalid env format '%s', expected KEY=VALUE\n", args[i])
				os.Exit(1)
			}
			env[kv[0]] = kv[1]
		case strings.HasPrefix(args[i], "--env="):
			kv := strings.SplitN(strings.TrimPrefix(args[i], "--env="), "=", 2)
			if len(kv) != 2 {
				fmt.Fprintf(os.Stderr, "Error: invalid env format, expected KEY=VALUE\n")
				os.Exit(1)
			}
			env[kv[0]] = kv[1]
		case args[i] == "--header" || args[i] == "-H":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --header requires 'Key: Value'")
				os.Exit(1)
			}
			i++
			parts := strings.SplitN(args[i], ":", 2)
			if len(parts) != 2 {
				fmt.Fprintf(os.Stderr, "Error: invalid header format '%s', expected 'Key: Value'\n", args[i])
				os.Exit(1)
			}
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		case strings.HasPrefix(args[i], "--header="):
			parts := strings.SplitN(strings.TrimPrefix(args[i], "--header="), ":", 2)
			if len(parts) != 2 {
				fmt.Fprintf(os.Stderr, "Error: invalid header format, expected 'Key: Value'\n")
				os.Exit(1)
			}
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		case args[i] == "--help" || args[i] == "-h":
			printMCPAddUsage()
			return
		default:
			positional = append(positional, args[i])
		}
	}

	// For HTTP transports, we don't need a command
	if transport == "sse" || transport == "http" || transport == "streamable" {
		if len(positional) < 1 {
			fmt.Fprintln(os.Stderr, "Error: name is required")
			printMCPAddUsage()
			os.Exit(1)
		}
		name = positional[0]
		if url == "" {
			fmt.Fprintln(os.Stderr, "Error: --url is required for HTTP transports")
			os.Exit(1)
		}
	} else {
		// stdio/command transport
		if len(positional) < 2 {
			fmt.Fprintln(os.Stderr, "Error: name and command are required for stdio transport")
			printMCPAddUsage()
			os.Exit(1)
		}
		name = positional[0]
		command = positional[1]
		if len(positional) > 2 {
			cmdArgs = positional[2:]
		}
	}

	// Determine config path
	cwd, _ := os.Getwd()
	cfgPath := config.ConfigPathForScope(scope, cwd)
	if cfgPath == "" {
		fmt.Fprintln(os.Stderr, "Error: could not determine config path")
		os.Exit(1)
	}

	// Build the MCP config
	mcpCfg := config.MCPConfig{
		Name:    name,
		Type:    transport,
		Command: command,
		Args:    cmdArgs,
		URL:     url,
	}
	if len(env) > 0 {
		mcpCfg.Env = env
	}
	if len(headers) > 0 {
		mcpCfg.Headers = headers
	}

	// Add to config file
	if err := config.AddMCPConfigToFile(cfgPath, mcpCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Added MCP '%s' to %s config (%s)\n", name, scope, cfgPath)
}

func printMCPAddUsage() {
	fmt.Print(`slop-mcp mcp add - Register an MCP server

Usage:
  slop-mcp mcp add <name> <command> [args...] [options]
  slop-mcp mcp add <name> --transport=sse --url=<url> [options]

Arguments:
  <name>       Name for the MCP server (used to reference it)
  <command>    Command to start the MCP server (for stdio transport)
  [args...]    Arguments to pass to the command

Options:
  -l, --local       Add to local config (.slop-mcp.local.kdl, gitignored)
  -p, --project     Add to project config (.slop-mcp.kdl) [default]
  -u, --user        Add to user config (~/.config/slop-mcp/config.kdl)
  -s, --scope=<scope>  Set scope: local, project, or user

  --transport=<type>, -t <type>
                Transport type: stdio (default), sse, http, streamable
  --url=<url>   Server URL (required for http/sse/streamable transports)

  --env KEY=VALUE, -e KEY=VALUE
                Set environment variable (can be repeated)
  --header "Key: Value", -H "Key: Value"
                Set HTTP header (can be repeated, for http transports)

  --help, -h   Show this help

Examples:
  # Stdio transport (default)
  slop-mcp mcp add filesystem npx -y @anthropic/mcp-server-filesystem /tmp
  slop-mcp mcp add --user github npx -y @anthropic/mcp-server-github

  # With environment variables
  slop-mcp mcp add brave npx -y @anthropic/mcp-server-brave-search -e BRAVE_API_KEY=xxx

  # SSE transport
  slop-mcp mcp add myapi --transport=sse --url=http://localhost:3000/mcp

  # HTTP transport with headers
  slop-mcp mcp add secure --transport=http --url=https://api.example.com/mcp \
    -H "Authorization: Bearer token"
`)
}

func cmdMCPAddJSON(args []string) {
	if len(args) < 2 {
		printMCPAddJSONUsage()
		os.Exit(1)
	}

	name := ""
	jsonStr := ""
	scope := config.ScopeProject

	var positional []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--local" || args[i] == "-l":
			scope = config.ScopeLocal
		case args[i] == "--project" || args[i] == "-p":
			scope = config.ScopeProject
		case args[i] == "--user" || args[i] == "-u":
			scope = config.ScopeUser
		case args[i] == "--scope" || args[i] == "-s":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --scope requires a value (local, project, or user)")
				os.Exit(1)
			}
			i++
			switch args[i] {
			case "local":
				scope = config.ScopeLocal
			case "project":
				scope = config.ScopeProject
			case "user":
				scope = config.ScopeUser
			default:
				fmt.Fprintf(os.Stderr, "Error: invalid scope '%s' (must be local, project, or user)\n", args[i])
				os.Exit(1)
			}
		case strings.HasPrefix(args[i], "--scope="):
			scopeVal := strings.TrimPrefix(args[i], "--scope=")
			switch scopeVal {
			case "local":
				scope = config.ScopeLocal
			case "project":
				scope = config.ScopeProject
			case "user":
				scope = config.ScopeUser
			default:
				fmt.Fprintf(os.Stderr, "Error: invalid scope '%s' (must be local, project, or user)\n", scopeVal)
				os.Exit(1)
			}
		case args[i] == "--help" || args[i] == "-h":
			printMCPAddJSONUsage()
			return
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) < 2 {
		fmt.Fprintln(os.Stderr, "Error: name and JSON config are required")
		os.Exit(1)
	}

	name = positional[0]
	jsonStr = positional[1]

	// Parse the JSON config
	mcpCfg, err := config.ParseJSONConfig(jsonStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		os.Exit(1)
	}
	mcpCfg.Name = name

	// Determine config path
	cwd, _ := os.Getwd()
	cfgPath := config.ConfigPathForScope(scope, cwd)
	if cfgPath == "" {
		fmt.Fprintln(os.Stderr, "Error: could not determine config path")
		os.Exit(1)
	}

	// Add to config file
	if err := config.AddMCPConfigToFile(cfgPath, *mcpCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Added MCP '%s' from JSON to %s config (%s)\n", name, scope, cfgPath)
}

func printMCPAddJSONUsage() {
	fmt.Print(`slop-mcp mcp add-json - Register an MCP server from JSON config

Usage:
  slop-mcp mcp add-json <name> '<json>' [--local|--project|--user]

Arguments:
  <name>       Name for the MCP server
  <json>       JSON configuration (Claude Desktop format)

Options:
  --local      Add to local config (.slop-mcp.local.kdl)
  --project    Add to project config (.slop-mcp.kdl) [default]
  --user       Add to user config (~/.config/slop-mcp/config.kdl)
  --help, -h   Show this help

Examples:
  slop-mcp mcp add-json filesystem '{"command":"npx","args":["-y","@anthropic/mcp-server-filesystem","/tmp"]}'
  slop-mcp mcp add-json api '{"type":"sse","url":"http://localhost:3000/mcp"}'
`)
}

func cmdMCPAddFromClaudeDesktop(args []string) {
	scope := config.ScopeProject
	var specificNames []string

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--local" || args[i] == "-l":
			scope = config.ScopeLocal
		case args[i] == "--project" || args[i] == "-p":
			scope = config.ScopeProject
		case args[i] == "--user" || args[i] == "-u":
			scope = config.ScopeUser
		case args[i] == "--scope" || args[i] == "-s":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --scope requires a value (local, project, or user)")
				os.Exit(1)
			}
			i++
			switch args[i] {
			case "local":
				scope = config.ScopeLocal
			case "project":
				scope = config.ScopeProject
			case "user":
				scope = config.ScopeUser
			default:
				fmt.Fprintf(os.Stderr, "Error: invalid scope '%s' (must be local, project, or user)\n", args[i])
				os.Exit(1)
			}
		case strings.HasPrefix(args[i], "--scope="):
			scopeVal := strings.TrimPrefix(args[i], "--scope=")
			switch scopeVal {
			case "local":
				scope = config.ScopeLocal
			case "project":
				scope = config.ScopeProject
			case "user":
				scope = config.ScopeUser
			default:
				fmt.Fprintf(os.Stderr, "Error: invalid scope '%s' (must be local, project, or user)\n", scopeVal)
				os.Exit(1)
			}
		case args[i] == "--help" || args[i] == "-h":
			printMCPAddFromClaudeDesktopUsage()
			return
		default:
			specificNames = append(specificNames, args[i])
		}
	}

	// Load Claude Desktop config
	claudeCfg, err := config.LoadClaudeDesktopConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading Claude Desktop config: %v\n", err)
		os.Exit(1)
	}

	if len(claudeCfg.MCPs) == 0 {
		fmt.Println("No MCPs found in Claude Desktop config")
		return
	}

	// Determine config path
	cwd, _ := os.Getwd()
	cfgPath := config.ConfigPathForScope(scope, cwd)
	if cfgPath == "" {
		fmt.Fprintln(os.Stderr, "Error: could not determine config path")
		os.Exit(1)
	}

	// Filter MCPs if specific names provided
	toAdd := make(map[string]config.MCPConfig)
	if len(specificNames) > 0 {
		for _, name := range specificNames {
			if mcp, ok := claudeCfg.MCPs[name]; ok {
				toAdd[name] = mcp
			} else {
				fmt.Fprintf(os.Stderr, "Warning: MCP '%s' not found in Claude Desktop config\n", name)
			}
		}
	} else {
		toAdd = claudeCfg.MCPs
	}

	if len(toAdd) == 0 {
		fmt.Println("No MCPs to add")
		return
	}

	// Add each MCP
	for name, mcp := range toAdd {
		mcp.Name = name
		if err := config.AddMCPConfigToFile(cfgPath, mcp); err != nil {
			fmt.Fprintf(os.Stderr, "Error adding %s: %v\n", name, err)
			continue
		}
		fmt.Printf("Added MCP '%s' from Claude Desktop to %s config\n", name, scope)
	}
}

func printMCPAddFromClaudeDesktopUsage() {
	fmt.Printf(`slop-mcp mcp add-from-claude-desktop - Import MCPs from Claude Desktop

Usage:
  slop-mcp mcp add-from-claude-desktop [names...] [--local|--project|--user]

Arguments:
  [names...]   Specific MCP names to import (imports all if omitted)

Options:
  --local      Add to local config (.slop-mcp.local.kdl)
  --project    Add to project config (.slop-mcp.kdl) [default]
  --user       Add to user config (~/.config/slop-mcp/config.kdl)
  --help, -h   Show this help

Claude Desktop config location:
  %s

Examples:
  # Import all MCPs from Claude Desktop
  slop-mcp mcp add-from-claude-desktop

  # Import specific MCPs
  slop-mcp mcp add-from-claude-desktop filesystem github

  # Import to user config
  slop-mcp mcp add-from-claude-desktop --user
`, config.ClaudeDesktopConfigPath())
}

func cmdMCPAddFromClaudeCode(args []string) {
	var specificNames []string
	dryRun := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			dryRun = true
		case "--help", "-h":
			printMCPAddFromClaudeCodeUsage()
			return
		default:
			specificNames = append(specificNames, args[i])
		}
	}

	// Load Claude Code config
	claudeCodeCfg, err := config.LoadClaudeCodeConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading Claude Code config: %v\n", err)
		os.Exit(1)
	}

	if len(claudeCodeCfg.MCPs) == 0 {
		fmt.Println("No MCPs found in Claude Code user settings")
		fmt.Printf("Checked: %s\n", config.ClaudeCodeConfigPath())
		return
	}

	// Determine user config path (always migrate to user scope)
	cfgPath := config.UserConfigPath()
	if cfgPath == "" {
		fmt.Fprintln(os.Stderr, "Error: could not determine user config path")
		os.Exit(1)
	}

	// Filter MCPs if specific names provided, and always exclude slop-mcp
	toAdd := make(map[string]config.MCPConfig)
	if len(specificNames) > 0 {
		for _, name := range specificNames {
			if isSlotMCP(name) {
				fmt.Printf("Skipping '%s' (slop-mcp itself)\n", name)
				continue
			}
			if mcp, ok := claudeCodeCfg.MCPs[name]; ok {
				toAdd[name] = mcp
			} else {
				fmt.Fprintf(os.Stderr, "Warning: MCP '%s' not found in Claude Code config\n", name)
			}
		}
	} else {
		for name, mcp := range claudeCodeCfg.MCPs {
			if isSlotMCP(name) {
				fmt.Printf("Skipping '%s' (slop-mcp itself)\n", name)
				continue
			}
			toAdd[name] = mcp
		}
	}

	if len(toAdd) == 0 {
		fmt.Println("No MCPs to migrate")
		return
	}

	if dryRun {
		fmt.Println("Dry run - would migrate the following MCPs to user config:")
		fmt.Printf("Target: %s\n\n", cfgPath)
		for name, mcp := range toAdd {
			fmt.Printf("  %s:\n", name)
			fmt.Printf("    type: %s\n", mcp.Type)
			if mcp.Command != "" {
				fmt.Printf("    command: %s\n", mcp.Command)
				if len(mcp.Args) > 0 {
					fmt.Printf("    args: %v\n", mcp.Args)
				}
			}
			if mcp.URL != "" {
				fmt.Printf("    url: %s\n", mcp.URL)
			}
			if len(mcp.Env) > 0 {
				fmt.Printf("    env: (redacted, %d variables)\n", len(mcp.Env))
			}
			fmt.Println()
		}
		return
	}

	// Add each MCP
	for name, mcp := range toAdd {
		mcp.Name = name
		if err := config.AddMCPConfigToFile(cfgPath, mcp); err != nil {
			fmt.Fprintf(os.Stderr, "Error adding %s: %v\n", name, err)
			continue
		}
		fmt.Printf("Migrated MCP '%s' from Claude Code to user config\n", name)
	}

	fmt.Printf("\nMigrated %d MCPs to %s\n", len(toAdd), cfgPath)
}

// isSlotMCP checks if the MCP name refers to slop-mcp itself.
func isSlotMCP(name string) bool {
	name = strings.ToLower(name)
	return name == "slop-mcp" || name == "slop_mcp" || name == "slopmcp"
}

func printMCPAddFromClaudeCodeUsage() {
	fmt.Printf(`slop-mcp mcp add-from-claude-code - Migrate MCPs from Claude Code user settings

Usage:
  slop-mcp mcp add-from-claude-code [names...] [--dry-run]

This command migrates MCP servers from Claude Code's user-level settings
to slop-mcp's user config. The slop-mcp server itself is automatically
excluded from migration.

Arguments:
  [names...]   Specific MCP names to migrate (migrates all if omitted)

Options:
  --dry-run    Show what would be migrated without making changes
  --help, -h   Show this help

Claude Code config locations:
  Main config: %s
  Plugins dir: %s

Target slop-mcp config:
  %s

Examples:
  # Preview what would be migrated
  slop-mcp mcp add-from-claude-code --dry-run

  # Migrate all MCPs from Claude Code
  slop-mcp mcp add-from-claude-code

  # Migrate specific MCPs
  slop-mcp mcp add-from-claude-code filesystem github

Note:
  This command reads MCPs from both ~/.claude.json and user-scoped plugin
  .mcp.json files. It always migrates to the user-level slop-mcp config,
  making the MCPs available across all projects. The original Claude Code
  configuration is not modified.
`, config.ClaudeCodeConfigPath(), config.ClaudeCodePluginsPath(), config.UserConfigPath())
}

func cmdMCPRemove(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: slop-mcp mcp remove <name> [--local|--project|--user]")
		os.Exit(1)
	}

	name := ""
	scope := config.ScopeProject

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--local" || args[i] == "-l":
			scope = config.ScopeLocal
		case args[i] == "--project" || args[i] == "-p":
			scope = config.ScopeProject
		case args[i] == "--user" || args[i] == "-u":
			scope = config.ScopeUser
		case args[i] == "--scope" || args[i] == "-s":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --scope requires a value (local, project, or user)")
				os.Exit(1)
			}
			i++
			switch args[i] {
			case "local":
				scope = config.ScopeLocal
			case "project":
				scope = config.ScopeProject
			case "user":
				scope = config.ScopeUser
			default:
				fmt.Fprintf(os.Stderr, "Error: invalid scope '%s' (must be local, project, or user)\n", args[i])
				os.Exit(1)
			}
		case strings.HasPrefix(args[i], "--scope="):
			scopeVal := strings.TrimPrefix(args[i], "--scope=")
			switch scopeVal {
			case "local":
				scope = config.ScopeLocal
			case "project":
				scope = config.ScopeProject
			case "user":
				scope = config.ScopeUser
			default:
				fmt.Fprintf(os.Stderr, "Error: invalid scope '%s' (must be local, project, or user)\n", scopeVal)
				os.Exit(1)
			}
		case args[i] == "--help" || args[i] == "-h":
			printMCPRemoveUsage()
			return
		default:
			if name == "" {
				name = args[i]
			}
		}
	}

	if name == "" {
		fmt.Fprintln(os.Stderr, "Error: name is required")
		os.Exit(1)
	}

	// Determine config path
	cwd, _ := os.Getwd()
	cfgPath := config.ConfigPathForScope(scope, cwd)
	if cfgPath == "" {
		fmt.Fprintln(os.Stderr, "Error: could not determine config path")
		os.Exit(1)
	}

	// Remove from config file
	if err := config.RemoveMCPFromFile(cfgPath, name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Removed MCP '%s' from %s config (%s)\n", name, scope, cfgPath)
}

func printMCPRemoveUsage() {
	fmt.Print(`slop-mcp mcp remove - Unregister an MCP server

Usage:
  slop-mcp mcp remove <name> [--local|--project|--user]

Arguments:
  <name>       Name of the MCP server to remove

Options:
  --local      Remove from local config (.slop-mcp.local.kdl)
  --project    Remove from project config (.slop-mcp.kdl) [default]
  --user       Remove from user config (~/.config/slop-mcp/config.kdl)
  --help, -h   Show this help

Examples:
  slop-mcp mcp remove filesystem
  slop-mcp mcp remove --user github
`)
}

func cmdMCPGet(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: slop-mcp mcp get <name>")
		os.Exit(1)
	}

	name := ""
	outputJSON := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			outputJSON = true
		case "--help", "-h":
			printMCPGetUsage()
			return
		default:
			if name == "" {
				name = args[i]
			}
		}
	}

	if name == "" {
		fmt.Fprintln(os.Stderr, "Error: name is required")
		os.Exit(1)
	}

	cwd, _ := os.Getwd()
	paths := config.ConfigPaths(cwd)

	// Search all config sources
	searchOrder := []struct {
		name string
		path string
	}{
		{"local", paths["local"]},
		{"project", paths["project"]},
		{"user", paths["user"]},
		{"claude_desktop", paths["claude_desktop"]},
	}

	for _, source := range searchOrder {
		mcp, err := config.GetMCP(source.path, name)
		if err != nil {
			continue
		}
		if mcp != nil {
			if outputJSON {
				fmt.Println(mcp.ToJSON())
			} else {
				fmt.Printf("MCP '%s' (from %s config):\n", name, source.name)
				fmt.Printf("  Type: %s\n", mcp.Type)
				if mcp.Command != "" {
					fmt.Printf("  Command: %s\n", mcp.Command)
				}
				if len(mcp.Args) > 0 {
					fmt.Printf("  Args: %v\n", mcp.Args)
				}
				if mcp.URL != "" {
					fmt.Printf("  URL: %s\n", mcp.URL)
				}
				if len(mcp.Env) > 0 {
					fmt.Printf("  Env:\n")
					for k, v := range mcp.Env {
						fmt.Printf("    %s=%s\n", k, v)
					}
				}
				if len(mcp.Headers) > 0 {
					fmt.Printf("  Headers:\n")
					for k, v := range mcp.Headers {
						fmt.Printf("    %s: %s\n", k, v)
					}
				}
				fmt.Printf("  Source: %s\n", source.path)
			}
			return
		}
	}

	fmt.Fprintf(os.Stderr, "MCP '%s' not found in any config\n", name)
	os.Exit(1)
}

func printMCPGetUsage() {
	fmt.Print(`slop-mcp mcp get - Get details of an MCP server

Usage:
  slop-mcp mcp get <name> [--json]

Arguments:
  <name>       Name of the MCP server

Options:
  --json       Output as JSON
  --help, -h   Show this help

Examples:
  slop-mcp mcp get filesystem
  slop-mcp mcp get github --json
`)
}

func cmdMCPList(args []string) {
	showLocal := true
	showProject := true
	showUser := true
	outputJSON := false

	for _, arg := range args {
		switch arg {
		case "--local":
			showProject = false
			showUser = false
		case "--project":
			showLocal = false
			showUser = false
		case "--user":
			showLocal = false
			showProject = false
		case "--all":
			showLocal = true
			showProject = true
			showUser = true
		case "--json":
			outputJSON = true
		case "--help", "-h":
			printMCPListUsage()
			return
		}
	}

	cwd, _ := os.Getwd()

	if outputJSON {
		allMCPs := make(map[string]config.MCPConfig)

		if showUser {
			userCfg, _ := config.LoadUserConfig()
			for name, mcp := range userCfg.MCPs {
				allMCPs[name] = mcp
			}
		}
		if showProject {
			projectCfg, _ := config.LoadProjectConfig(cwd)
			for name, mcp := range projectCfg.MCPs {
				allMCPs[name] = mcp
			}
		}
		if showLocal {
			localCfg, _ := config.LoadLocalConfig(cwd)
			for name, mcp := range localCfg.MCPs {
				allMCPs[name] = mcp
			}
		}

		data, _ := json.MarshalIndent(allMCPs, "", "  ")
		fmt.Println(string(data))
		return
	}

	// Load configs
	if showLocal {
		localCfg, err := config.LoadLocalConfig(cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load local config: %v\n", err)
		} else if len(localCfg.MCPs) > 0 {
			fmt.Println("Local config (.slop-mcp.local.kdl):")
			printMCPList(localCfg)
			fmt.Println()
		}
	}

	if showProject {
		projectCfg, err := config.LoadProjectConfig(cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load project config: %v\n", err)
		} else if len(projectCfg.MCPs) > 0 {
			fmt.Println("Project config (.slop-mcp.kdl):")
			printMCPList(projectCfg)
			fmt.Println()
		}
	}

	if showUser {
		userCfg, err := config.LoadUserConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load user config: %v\n", err)
		} else if len(userCfg.MCPs) > 0 {
			fmt.Println("User config (~/.config/slop-mcp/config.kdl):")
			printMCPList(userCfg)
			fmt.Println()
		}
	}
}

func printMCPList(cfg *config.Config) {
	for name, mcp := range cfg.MCPs {
		fmt.Printf("  %s:\n", name)
		fmt.Printf("    type: %s\n", mcp.Type)
		if mcp.Command != "" {
			fmt.Printf("    command: %s\n", mcp.Command)
			if len(mcp.Args) > 0 {
				fmt.Printf("    args: %v\n", mcp.Args)
			}
		}
		if mcp.URL != "" {
			fmt.Printf("    url: %s\n", mcp.URL)
		}
		if len(mcp.Env) > 0 {
			fmt.Printf("    env: %v\n", mcp.Env)
		}
	}
}

func printMCPListUsage() {
	fmt.Print(`slop-mcp mcp list - List registered MCP servers

Usage:
  slop-mcp mcp list [--local|--project|--user|--all] [--json]

Options:
  --local      Show only local config (.slop-mcp.local.kdl)
  --project    Show only project config (.slop-mcp.kdl)
  --user       Show only user config (~/.config/slop-mcp/config.kdl)
  --all        Show all configs [default]
  --json       Output as JSON
  --help, -h   Show this help

Examples:
  slop-mcp mcp list
  slop-mcp mcp list --project
  slop-mcp mcp list --json
`)
}

func cmdMCPPaths(args []string) {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			printMCPPathsUsage()
			return
		}
	}

	cwd, _ := os.Getwd()
	paths := config.ConfigPaths(cwd)

	fmt.Println("Config file paths:")
	fmt.Printf("  Local:               %s\n", paths["local"])
	fmt.Printf("  Project:             %s\n", paths["project"])
	fmt.Printf("  User:                %s\n", paths["user"])
	fmt.Printf("  Claude Desktop:      %s\n", paths["claude_desktop"])
	fmt.Printf("  Claude Code:         %s\n", paths["claude_code"])
	fmt.Printf("  Claude Code Plugins: %s\n", paths["claude_code_plugins"])

	fmt.Println("\nFile/directory status:")
	for name, path := range paths {
		exists := "not found"
		if info, err := os.Stat(path); err == nil {
			if info.IsDir() {
				exists = "exists (dir)"
			} else {
				exists = "exists"
			}
		}
		fmt.Printf("  %-20s  %s\n", name+":", exists)
	}
}

func printMCPPathsUsage() {
	fmt.Print(`slop-mcp mcp paths - Show config file paths

Usage:
  slop-mcp mcp paths

Shows the paths to all config files and their existence status.

Config file precedence (later overrides earlier):
  1. User config (~/.config/slop-mcp/config.kdl)
  2. Project config (.slop-mcp.kdl)
  3. Local config (.slop-mcp.local.kdl)
`)
}

func cmdMCPDump(args []string) {
	scope := ""
	outputJSON := false

	for _, arg := range args {
		switch arg {
		case "--local":
			scope = "local"
		case "--project":
			scope = "project"
		case "--user":
			scope = "user"
		case "--claude-desktop":
			scope = "claude_desktop"
		case "--claude-code":
			scope = "claude_code"
		case "--json":
			outputJSON = true
		case "--help", "-h":
			printMCPDumpUsage()
			return
		}
	}

	cwd, _ := os.Getwd()
	paths := config.ConfigPaths(cwd)

	dumpFile := func(name, path string) {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			if outputJSON {
				fmt.Printf("{\"error\": \"%s config not found at %s\"}\n", name, path)
			} else {
				fmt.Printf("# %s config not found at %s\n", name, path)
			}
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
			return
		}

		if outputJSON {
			// Try to parse and re-output as JSON
			if name == "claude_desktop" || name == "claude_code" {
				var jsonCfg config.ClaudeCodeSettings
				if err := json.Unmarshal(data, &jsonCfg); err == nil && jsonCfg.MCPServers != nil {
					pretty, _ := json.MarshalIndent(jsonCfg.MCPServers, "", "  ")
					fmt.Println(string(pretty))
					return
				}
				// Also try Claude Desktop format
				var desktopCfg config.JSONConfig
				if err := json.Unmarshal(data, &desktopCfg); err == nil && desktopCfg.MCPServers != nil {
					pretty, _ := json.MarshalIndent(desktopCfg.MCPServers, "", "  ")
					fmt.Println(string(pretty))
					return
				}
			}
			// For KDL files, load and convert to JSON
			cfg, err := config.ParseKDLConfig(string(data), config.SourceProject)
			if err == nil {
				pretty, _ := json.MarshalIndent(cfg.MCPs, "", "  ")
				fmt.Println(string(pretty))
				return
			}
			fmt.Printf("{\"raw\": %q}\n", string(data))
		} else {
			fmt.Printf("# %s (%s)\n", name, path)
			fmt.Println(string(data))
		}
	}

	if scope != "" {
		dumpFile(scope, paths[scope])
	} else {
		// Dump all
		for _, name := range []string{"local", "project", "user", "claude_desktop", "claude_code"} {
			dumpFile(name, paths[name])
			fmt.Println()
		}
	}
}

func printMCPDumpUsage() {
	fmt.Print(`slop-mcp mcp dump - Show config file contents

Usage:
  slop-mcp mcp dump [--local|--project|--user|--claude-desktop|--claude-code] [--json]

Options:
  --local           Dump local config only
  --project         Dump project config only
  --user            Dump user config only
  --claude-desktop  Dump Claude Desktop config only
  --claude-code     Dump Claude Code config only
  --json            Output as JSON
  --help, -h        Show this help

Examples:
  slop-mcp mcp dump                    # Dump all configs
  slop-mcp mcp dump --project          # Dump project config only
  slop-mcp mcp dump --claude-desktop   # Dump Claude Desktop config
  slop-mcp mcp dump --claude-code      # Dump Claude Code config
  slop-mcp mcp dump --json             # Dump all as JSON
`)
}

func cmdMCPStatus(args []string) {
	port := 8080
	outputJSON := false

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--json":
			outputJSON = true
		case strings.HasPrefix(args[i], "--port="):
			fmt.Sscanf(args[i], "--port=%d", &port)
		case args[i] == "--port" && i+1 < len(args):
			fmt.Sscanf(args[i+1], "%d", &port)
			i++
		case args[i] == "--help" || args[i] == "-h":
			printMCPStatusUsage()
			return
		}
	}

	// Query the running server via HTTP
	url := fmt.Sprintf("http://localhost:%d/", port)

	// Build MCP tool call request
	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "manage_mcps",
			"arguments": map[string]any{
				"action": "status",
			},
		},
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(reqJSON))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to server at %s: %v\n", url, err)
		fmt.Fprintf(os.Stderr, "Make sure slop-mcp is running with: slop-mcp serve --port %d\n", port)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
		os.Exit(1)
	}

	// Parse response
	var rpcResp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &rpcResp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if rpcResp.Error != nil {
		fmt.Fprintf(os.Stderr, "Server error: %s\n", rpcResp.Error.Message)
		os.Exit(1)
	}

	// Extract the status JSON from the text content
	if len(rpcResp.Result.Content) == 0 {
		fmt.Fprintf(os.Stderr, "No status data in response\n")
		os.Exit(1)
	}

	var output struct {
		Status []registry.MCPFullStatus `json:"status"`
	}
	if err := json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &output); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing status: %v\n", err)
		os.Exit(1)
	}

	if outputJSON {
		pretty, _ := json.MarshalIndent(output.Status, "", "  ")
		fmt.Println(string(pretty))
		return
	}

	// Print formatted status
	fmt.Printf("MCP Status (via localhost:%d):\n\n", port)
	if len(output.Status) == 0 {
		fmt.Println("  No MCPs configured")
		return
	}

	for _, s := range output.Status {
		fmt.Printf("  %s:\n", s.Name)
		fmt.Printf("    state: %s\n", s.State)
		fmt.Printf("    type: %s\n", s.Type)
		if s.ToolCount > 0 {
			fmt.Printf("    tools: %d\n", s.ToolCount)
		}
		if s.Error != "" {
			fmt.Printf("    error: %s\n", s.Error)
		}
		fmt.Println()
	}
}

func printMCPStatusUsage() {
	fmt.Print(`slop-mcp mcp status - Show live MCP connection status

Usage:
  slop-mcp mcp status [--port=<port>] [--json]

Options:
  --port=<port>    Server port (default: 8080)
  --json           Output as JSON
  --help, -h       Show this help

This command queries a running slop-mcp server to show the connection
status of all configured MCPs.

MCP States:
  configured    - MCP is in config but not yet connected
  connecting    - Connection is in progress
  connected     - Successfully connected
  disconnected  - Was connected but now disconnected
  error         - Connection failed
  needs_auth    - Requires OAuth authentication

Examples:
  slop-mcp mcp status                  # Query server on port 8080
  slop-mcp mcp status --port=3000      # Query server on port 3000
  slop-mcp mcp status --json           # Output as JSON
`)
}

func cmdMCPAuth(args []string) {
	if len(args) < 1 {
		printMCPAuthUsage()
		os.Exit(1)
	}

	action := args[0]
	if action == "--help" || action == "-h" || action == "help" {
		printMCPAuthUsage()
		return
	}

	var name string
	if len(args) > 1 {
		name = args[1]
	}

	store := auth.NewTokenStore()

	switch action {
	case "login":
		if name == "" {
			fmt.Fprintln(os.Stderr, "Error: MCP name is required for login")
			printMCPAuthUsage()
			os.Exit(1)
		}

		// Find MCP config to get URL
		cfg, err := findMCPConfigFromFiles(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if cfg.URL == "" {
			fmt.Fprintf(os.Stderr, "Error: MCP '%s' does not have a URL configured; OAuth requires HTTP transport\n", name)
			os.Exit(1)
		}

		flow := &auth.OAuthFlow{
			ServerName: name,
			ServerURL:  cfg.URL,
			Store:      store,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		fmt.Printf("Starting OAuth flow for %s...\n", name)
		result, err := flow.DiscoverAndAuth(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: OAuth flow failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully authenticated with %s\n", name)
		fmt.Printf("  Server URL: %s\n", result.Token.ServerURL)
		if !result.Token.ExpiresAt.IsZero() {
			fmt.Printf("  Expires at: %s\n", result.Token.ExpiresAt.Format(time.RFC3339))
		}

	case "logout":
		if name == "" {
			fmt.Fprintln(os.Stderr, "Error: MCP name is required for logout")
			printMCPAuthUsage()
			os.Exit(1)
		}

		if err := store.DeleteToken(name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Logged out from %s\n", name)

	case "status":
		if name == "" {
			fmt.Fprintln(os.Stderr, "Error: MCP name is required for status")
			printMCPAuthUsage()
			os.Exit(1)
		}

		token, err := store.GetToken(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if token == nil {
			fmt.Printf("%s: not authenticated\n", name)
			return
		}

		fmt.Printf("%s:\n", name)
		fmt.Printf("  Authenticated: yes\n")
		fmt.Printf("  Server URL: %s\n", token.ServerURL)
		if !token.ExpiresAt.IsZero() {
			fmt.Printf("  Expires at: %s\n", token.ExpiresAt.Format(time.RFC3339))
			if token.IsExpired() {
				fmt.Printf("  Status: EXPIRED\n")
			} else {
				fmt.Printf("  Status: valid\n")
			}
		}
		if token.RefreshToken != "" {
			fmt.Printf("  Has refresh token: yes\n")
		}

	case "list":
		tokens, err := store.ListTokens()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if len(tokens) == 0 {
			fmt.Println("No authenticated MCPs")
			return
		}

		fmt.Println("Authenticated MCPs:")
		for _, t := range tokens {
			status := "valid"
			if t.IsExpired() {
				status = "EXPIRED"
			}
			fmt.Printf("  %s: %s (%s)\n", t.ServerName, t.ServerURL, status)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown action: %s\n", action)
		printMCPAuthUsage()
		os.Exit(1)
	}
}

func findMCPConfigFromFiles(name string) (*config.MCPConfig, error) {
	cwd, _ := os.Getwd()

	// Check all config sources
	sources := []struct {
		loader func() (*config.Config, error)
		name   string
	}{
		{func() (*config.Config, error) { return config.LoadLocalConfig(cwd) }, "local"},
		{func() (*config.Config, error) { return config.LoadProjectConfig(cwd) }, "project"},
		{config.LoadUserConfig, "user"},
	}

	for _, src := range sources {
		cfg, err := src.loader()
		if err != nil {
			continue
		}
		if mcpCfg, ok := cfg.MCPs[name]; ok {
			return &mcpCfg, nil
		}
	}

	return nil, fmt.Errorf("MCP '%s' not found in any config file", name)
}

func printMCPAuthUsage() {
	fmt.Print(`slop-mcp mcp auth - Manage OAuth authentication for MCPs

Usage:
  slop-mcp mcp auth <action> [name]

Actions:
  login <name>     Initiate OAuth flow for an MCP
  logout <name>    Remove stored token for an MCP
  status <name>    Check authentication status for an MCP
  list             List all authenticated MCPs

Examples:
  slop-mcp mcp auth login figma     # Authenticate with Figma MCP
  slop-mcp mcp auth status figma    # Check Figma auth status
  slop-mcp mcp auth logout figma    # Remove Figma token
  slop-mcp mcp auth list            # List all authenticated MCPs

Notes:
  - OAuth requires the MCP to have an HTTP URL configured
  - Tokens are stored in ~/.config/slop-mcp/auth.json
  - This command works without a running server
`)
}

func cmdMCPMetadata(args []string) {
	port := 8080
	outputJSON := false
	outputFile := ""
	mcpName := ""

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--json":
			outputJSON = true
		case strings.HasPrefix(args[i], "--port="):
			fmt.Sscanf(args[i], "--port=%d", &port)
		case args[i] == "--port" && i+1 < len(args):
			fmt.Sscanf(args[i+1], "%d", &port)
			i++
		case strings.HasPrefix(args[i], "--output="):
			outputFile = strings.TrimPrefix(args[i], "--output=")
		case args[i] == "--output" && i+1 < len(args):
			outputFile = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--mcp="):
			mcpName = strings.TrimPrefix(args[i], "--mcp=")
		case args[i] == "--mcp" && i+1 < len(args):
			mcpName = args[i+1]
			i++
		case args[i] == "--help" || args[i] == "-h":
			printMCPMetadataUsage()
			return
		}
	}

	// Query the running server via HTTP
	url := fmt.Sprintf("http://localhost:%d/", port)

	// Build MCP tool call request
	arguments := map[string]any{}
	if mcpName != "" {
		arguments["mcp_name"] = mcpName
	}

	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "get_metadata",
			"arguments": arguments,
		},
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(reqJSON))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to server at %s: %v\n", url, err)
		fmt.Fprintf(os.Stderr, "Make sure slop-mcp is running with: slop-mcp serve --port %d\n", port)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
		os.Exit(1)
	}

	// Parse response
	var rpcResp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &rpcResp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if rpcResp.Error != nil {
		fmt.Fprintf(os.Stderr, "Server error: %s\n", rpcResp.Error.Message)
		os.Exit(1)
	}

	// Extract the metadata JSON from the text content
	if len(rpcResp.Result.Content) == 0 {
		fmt.Fprintf(os.Stderr, "No metadata in response\n")
		os.Exit(1)
	}

	var output struct {
		Metadata []json.RawMessage `json:"metadata"`
		Total    int               `json:"total"`
	}
	if err := json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &output); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing metadata: %v\n", err)
		os.Exit(1)
	}

	// Format output
	var formattedData []byte
	if outputJSON {
		formattedData, _ = json.MarshalIndent(output.Metadata, "", "  ")
	} else {
		formattedData, _ = json.MarshalIndent(output.Metadata, "", "  ")
	}

	// Write to file or stdout
	if outputFile != "" {
		if err := os.WriteFile(outputFile, formattedData, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Metadata written to %s (%d MCPs)\n", outputFile, output.Total)
	} else {
		if !outputJSON {
			fmt.Printf("MCP Metadata (%d servers):\n\n", output.Total)
		}
		fmt.Println(string(formattedData))
	}
}

func printMCPMetadataUsage() {
	fmt.Print(`slop-mcp mcp metadata - Get full MCP metadata from running server

Usage:
  slop-mcp mcp metadata [options]

Options:
  --port=<port>      Server port (default: 8080)
  --output=<file>    Write metadata to file
  --mcp=<name>       Filter to a specific MCP
  --json             Output as JSON (default when writing to file)
  --help, -h         Show this help

This command queries a running slop-mcp server to get full metadata
for all connected MCPs, including:
  - Tools with input schemas
  - Prompts with arguments
  - Resources
  - Resource templates

Examples:
  slop-mcp mcp metadata                        # Show all metadata
  slop-mcp mcp metadata --mcp=figma            # Show only Figma metadata
  slop-mcp mcp metadata --output=mcps.json     # Save to file
  slop-mcp mcp metadata --port=3000 --json     # Query different port
`)
}
