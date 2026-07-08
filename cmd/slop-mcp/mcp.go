package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/atomicfile"
	"github.com/standardbeagle/slop-mcp/internal/auth"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/registry"
)

var getwd = os.Getwd

func currentWorkingDir() (string, error) {
	cwd, err := getwd()
	if err != nil {
		return "", fmt.Errorf("getting current directory: %w", err)
	}
	return cwd, nil
}

func configPathForScope(scope config.Scope) (string, error) {
	cwd, err := currentWorkingDir()
	if err != nil {
		return "", err
	}
	cfgPath := config.ConfigPathForScope(scope, cwd)
	if cfgPath == "" {
		return "", fmt.Errorf("could not determine config path")
	}
	return cfgPath, nil
}

func cmdMCPAdd(args []string) {
	opts, showHelp, err := parseMCPAddArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printMCPAddUsage()
		os.Exit(1)
	}
	if showHelp {
		printMCPAddUsage()
		return
	}

	cfgPath, err := configPathForScope(opts.scope)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Build the MCP config
	mcpCfg := config.MCPConfig{
		Name:    opts.name,
		Type:    opts.transport,
		Command: opts.command,
		Args:    opts.cmdArgs,
		URL:     opts.url,
	}
	if len(opts.env) > 0 {
		mcpCfg.Env = opts.env
	}
	if len(opts.headers) > 0 {
		mcpCfg.Headers = opts.headers
	}

	// Add to config file
	if err := config.AddMCPConfigToFile(cfgPath, mcpCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Added MCP '%s' to %s config (%s)\n", opts.name, opts.scope, cfgPath)
}

type mcpAddOptions struct {
	name      string
	command   string
	cmdArgs   []string
	scope     config.Scope
	transport string
	env       map[string]string
	headers   map[string]string
	url       string
}

func parseMCPAddArgs(args []string) (mcpAddOptions, bool, error) {
	opts := mcpAddOptions{
		scope:     config.ScopeProject,
		transport: "stdio",
		env:       make(map[string]string),
		headers:   make(map[string]string),
	}
	var positional []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--local" || args[i] == "-l":
			opts.scope = config.ScopeLocal
		case args[i] == "--project" || args[i] == "-p":
			opts.scope = config.ScopeProject
		case args[i] == "--user" || args[i] == "-u":
			opts.scope = config.ScopeUser
		case args[i] == "--scope" || args[i] == "-s":
			if i+1 >= len(args) {
				return opts, false, fmt.Errorf("--scope requires a value (local, project, or user)")
			}
			parsedScope, err := parseMCPConfigScope(args[i+1])
			if err != nil {
				return opts, false, err
			}
			opts.scope = parsedScope
			i++
		case strings.HasPrefix(args[i], "--scope="):
			parsedScope, err := parseMCPConfigScope(strings.TrimPrefix(args[i], "--scope="))
			if err != nil {
				return opts, false, err
			}
			opts.scope = parsedScope
		case args[i] == "--transport" || args[i] == "-t":
			if i+1 >= len(args) {
				return opts, false, fmt.Errorf("--transport requires a value")
			}
			opts.transport = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--transport="):
			opts.transport = strings.TrimPrefix(args[i], "--transport=")
		case args[i] == "--url":
			if i+1 >= len(args) {
				return opts, false, fmt.Errorf("--url requires a value")
			}
			opts.url = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--url="):
			opts.url = strings.TrimPrefix(args[i], "--url=")
		case args[i] == "--env" || args[i] == "-e":
			if i+1 >= len(args) {
				return opts, false, fmt.Errorf("--env requires KEY=VALUE")
			}
			key, value, err := parseMCPEnv(args[i+1])
			if err != nil {
				return opts, false, err
			}
			opts.env[key] = value
			i++
		case strings.HasPrefix(args[i], "--env="):
			key, value, err := parseMCPEnv(strings.TrimPrefix(args[i], "--env="))
			if err != nil {
				return opts, false, err
			}
			opts.env[key] = value
		case args[i] == "--header" || args[i] == "-H":
			if i+1 >= len(args) {
				return opts, false, fmt.Errorf("--header requires 'Key: Value'")
			}
			key, value, err := parseMCPHeader(args[i+1])
			if err != nil {
				return opts, false, err
			}
			opts.headers[key] = value
			i++
		case strings.HasPrefix(args[i], "--header="):
			key, value, err := parseMCPHeader(strings.TrimPrefix(args[i], "--header="))
			if err != nil {
				return opts, false, err
			}
			opts.headers[key] = value
		case args[i] == "--help" || args[i] == "-h":
			return opts, true, nil
		default:
			positional = append(positional, args[i])
		}
	}

	if !isValidMCPTransport(opts.transport) {
		return opts, false, fmt.Errorf("invalid transport %q (must be stdio, command, sse, http, or streamable)", opts.transport)
	}

	// For HTTP transports, we don't need a command
	if isURLMCPTransport(opts.transport) {
		if len(positional) != 1 {
			if len(positional) == 0 {
				return opts, false, fmt.Errorf("name is required")
			}
			return opts, false, fmt.Errorf("unexpected extra argument %q for %s transport", positional[1], opts.transport)
		}
		opts.name = positional[0]
		if opts.url == "" {
			return opts, false, fmt.Errorf("--url is required for HTTP transports")
		}
		return opts, false, nil
	}

	// stdio/command transport
	if len(positional) < 2 {
		return opts, false, fmt.Errorf("name and command are required for stdio transport")
	}
	opts.name = positional[0]
	opts.command = positional[1]
	if len(positional) > 2 {
		opts.cmdArgs = positional[2:]
	}
	return opts, false, nil
}

func isURLMCPTransport(transport string) bool {
	return transport == "sse" || transport == "http" || transport == "streamable"
}

func isValidMCPTransport(transport string) bool {
	switch transport {
	case "stdio", "command", "sse", "http", "streamable":
		return true
	}
	return false
}

func parseMCPEnv(raw string) (string, string, error) {
	kv := strings.SplitN(raw, "=", 2)
	if len(kv) != 2 || kv[0] == "" {
		return "", "", fmt.Errorf("invalid env format %q, expected KEY=VALUE", raw)
	}
	return kv[0], kv[1], nil
}

func parseMCPHeader(raw string) (string, string, error) {
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid header format %q, expected 'Key: Value'", raw)
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return "", "", fmt.Errorf("invalid header format %q, header name is required", raw)
	}
	return key, strings.TrimSpace(parts[1]), nil
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
  -u, --user        Add to user config ($XDG_CONFIG_HOME/slop-mcp/config.kdl or ~/.config/slop-mcp/config.kdl)
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
	name, jsonStr, scope, showHelp, err := parseMCPAddJSONArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printMCPAddJSONUsage()
		os.Exit(1)
	}
	if showHelp {
		printMCPAddJSONUsage()
		return
	}

	// Parse the JSON config
	mcpCfg, err := config.ParseJSONConfig(jsonStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		os.Exit(1)
	}
	mcpCfg.Name = name

	cfgPath, err := configPathForScope(scope)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Add to config file
	if err := config.AddMCPConfigToFile(cfgPath, *mcpCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Added MCP '%s' from JSON to %s config (%s)\n", name, scope, cfgPath)
}

func parseMCPAddJSONArgs(args []string) (name string, jsonStr string, scope config.Scope, showHelp bool, err error) {
	scope = config.ScopeProject
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
				return "", "", scope, false, fmt.Errorf("--scope requires a value (local, project, or user)")
			}
			parsedScope, err := parseMCPConfigScope(args[i+1])
			if err != nil {
				return "", "", scope, false, err
			}
			scope = parsedScope
			i++
		case strings.HasPrefix(args[i], "--scope="):
			parsedScope, err := parseMCPConfigScope(strings.TrimPrefix(args[i], "--scope="))
			if err != nil {
				return "", "", scope, false, err
			}
			scope = parsedScope
		case args[i] == "--help" || args[i] == "-h":
			showHelp = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", "", scope, false, fmt.Errorf("unknown add-json option %q", args[i])
			}
			if name == "" {
				name = args[i]
				continue
			}
			if jsonStr == "" {
				jsonStr = args[i]
				continue
			}
			return "", "", scope, false, fmt.Errorf("unexpected extra argument %q", args[i])
		}
	}
	if showHelp {
		return name, jsonStr, scope, true, nil
	}
	if name == "" || jsonStr == "" {
		return "", "", scope, false, fmt.Errorf("name and JSON config are required")
	}
	return name, jsonStr, scope, false, nil
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
  --user       Add to user config ($XDG_CONFIG_HOME/slop-mcp/config.kdl or ~/.config/slop-mcp/config.kdl)
  --help, -h   Show this help

Examples:
  slop-mcp mcp add-json filesystem '{"command":"npx","args":["-y","@anthropic/mcp-server-filesystem","/tmp"]}'
  slop-mcp mcp add-json api '{"type":"sse","url":"http://localhost:3000/mcp"}'
`)
}

func cmdMCPAddFromClaudeDesktop(args []string) {
	scope, specificNames, showHelp, err := parseMCPAddFromClaudeDesktopArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printMCPAddFromClaudeDesktopUsage()
		os.Exit(1)
	}
	if showHelp {
		printMCPAddFromClaudeDesktopUsage()
		return
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

	cfgPath, err := configPathForScope(scope)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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

	if err := config.AddMCPConfigsToFile(cfgPath, toAdd); err != nil {
		fmt.Fprintf(os.Stderr, "Error adding MCPs from Claude Desktop: %v\n", err)
		os.Exit(1)
	}

	for _, name := range sortedMCPNames(toAdd) {
		fmt.Printf("Added MCP '%s' from Claude Desktop to %s config\n", name, scope)
	}
}

func parseMCPAddFromClaudeDesktopArgs(args []string) (scope config.Scope, specificNames []string, showHelp bool, err error) {
	scope = config.ScopeProject
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
				return scope, nil, false, fmt.Errorf("--scope requires a value (local, project, or user)")
			}
			parsedScope, err := parseMCPConfigScope(args[i+1])
			if err != nil {
				return scope, nil, false, err
			}
			scope = parsedScope
			i++
		case strings.HasPrefix(args[i], "--scope="):
			parsedScope, err := parseMCPConfigScope(strings.TrimPrefix(args[i], "--scope="))
			if err != nil {
				return scope, nil, false, err
			}
			scope = parsedScope
		case args[i] == "--help" || args[i] == "-h":
			showHelp = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return scope, nil, false, fmt.Errorf("unknown add-from-claude-desktop option %q", args[i])
			}
			specificNames = append(specificNames, args[i])
		}
	}
	return scope, specificNames, showHelp, nil
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
  --user       Add to user config ($XDG_CONFIG_HOME/slop-mcp/config.kdl or ~/.config/slop-mcp/config.kdl)
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
	specificNames, dryRun, showHelp, err := parseMCPAddFromClaudeCodeArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printMCPAddFromClaudeCodeUsage()
		os.Exit(1)
	}
	if showHelp {
		printMCPAddFromClaudeCodeUsage()
		return
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
		for _, name := range sortedMCPNames(toAdd) {
			mcp := toAdd[name]
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

	if err := config.AddMCPConfigsToFile(cfgPath, toAdd); err != nil {
		fmt.Fprintf(os.Stderr, "Error migrating MCPs from Claude Code: %v\n", err)
		os.Exit(1)
	}

	for _, name := range sortedMCPNames(toAdd) {
		fmt.Printf("Migrated MCP '%s' from Claude Code to user config\n", name)
	}

	fmt.Printf("\nMigrated %d MCPs to %s\n", len(toAdd), cfgPath)
}

func parseMCPAddFromClaudeCodeArgs(args []string) (specificNames []string, dryRun bool, showHelp bool, err error) {
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			dryRun = true
		case "--help", "-h":
			showHelp = true
		default:
			if strings.HasPrefix(arg, "-") {
				return nil, false, false, fmt.Errorf("unknown add-from-claude-code option %q", arg)
			}
			specificNames = append(specificNames, arg)
		}
	}
	return specificNames, dryRun, showHelp, nil
}

func sortedMCPNames(mcps map[string]config.MCPConfig) []string {
	names := make([]string, 0, len(mcps))
	for name := range mcps {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
	name, scope, showHelp, err := parseMCPRemoveArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printMCPRemoveUsage()
		os.Exit(1)
	}
	if showHelp {
		printMCPRemoveUsage()
		return
	}

	cfgPath, err := configPathForScope(scope)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Remove from config file
	if err := config.RemoveMCPFromFile(cfgPath, name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Removed MCP '%s' from %s config (%s)\n", name, scope, cfgPath)
}

func parseMCPRemoveArgs(args []string) (name string, scope config.Scope, showHelp bool, err error) {
	scope = config.ScopeProject
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
				return "", scope, false, fmt.Errorf("--scope requires a value (local, project, or user)")
			}
			parsedScope, err := parseMCPConfigScope(args[i+1])
			if err != nil {
				return "", scope, false, err
			}
			scope = parsedScope
			i++
		case strings.HasPrefix(args[i], "--scope="):
			parsedScope, err := parseMCPConfigScope(strings.TrimPrefix(args[i], "--scope="))
			if err != nil {
				return "", scope, false, err
			}
			scope = parsedScope
		case args[i] == "--help" || args[i] == "-h":
			showHelp = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", scope, false, fmt.Errorf("unknown remove option %q", args[i])
			}
			if name != "" {
				return "", scope, false, fmt.Errorf("unexpected extra argument %q", args[i])
			}
			name = args[i]
		}
	}
	if showHelp {
		return name, scope, true, nil
	}
	if name == "" {
		return "", scope, false, fmt.Errorf("name is required")
	}
	return name, scope, false, nil
}

func parseMCPConfigScope(scope string) (config.Scope, error) {
	switch scope {
	case "local":
		return config.ScopeLocal, nil
	case "project":
		return config.ScopeProject, nil
	case "user":
		return config.ScopeUser, nil
	default:
		return config.ScopeProject, fmt.Errorf("invalid scope %q (must be local, project, or user)", scope)
	}
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
  --user       Remove from user config ($XDG_CONFIG_HOME/slop-mcp/config.kdl or ~/.config/slop-mcp/config.kdl)
  --help, -h   Show this help

Examples:
  slop-mcp mcp remove filesystem
  slop-mcp mcp remove --user github
`)
}

func cmdMCPGet(args []string) {
	name, outputJSON, showHelp, err := parseMCPGetArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printMCPGetUsage()
		os.Exit(1)
	}
	if showHelp {
		printMCPGetUsage()
		return
	}

	cwd, err := currentWorkingDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
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

func parseMCPGetArgs(args []string) (name string, outputJSON bool, showHelp bool, err error) {
	for _, arg := range args {
		switch arg {
		case "--json":
			outputJSON = true
		case "--help", "-h":
			showHelp = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", false, false, fmt.Errorf("unknown get option %q", arg)
			}
			if name != "" {
				return "", false, false, fmt.Errorf("unexpected extra argument %q", arg)
			}
			name = arg
		}
	}
	if showHelp {
		return name, outputJSON, true, nil
	}
	if name == "" {
		return "", false, false, fmt.Errorf("name is required")
	}
	return name, outputJSON, false, nil
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
	opts, err := parseMCPListArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printMCPListUsage()
		os.Exit(1)
	}
	if opts.showHelp {
		printMCPListUsage()
		return
	}

	cwd, err := currentWorkingDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if opts.outputJSON {
		allMCPs, err := loadMCPListJSON(cwd, opts.showUser, opts.showProject, opts.showLocal)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		data, _ := json.MarshalIndent(allMCPs, "", "  ")
		fmt.Println(string(data))
		return
	}

	// Load configs
	if opts.showLocal {
		localCfg, err := config.LoadLocalConfig(cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load local config: %v\n", err)
		} else if len(localCfg.MCPs) > 0 {
			fmt.Println("Local config (.slop-mcp.local.kdl):")
			printMCPList(localCfg)
			fmt.Println()
		}
	}

	if opts.showProject {
		projectCfg, err := config.LoadProjectConfig(cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load project config: %v\n", err)
		} else if len(projectCfg.MCPs) > 0 {
			fmt.Println("Project config (.slop-mcp.kdl):")
			printMCPList(projectCfg)
			fmt.Println()
		}
	}

	if opts.showUser {
		userCfg, err := config.LoadUserConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load user config: %v\n", err)
		} else if len(userCfg.MCPs) > 0 {
			fmt.Println("User config ($XDG_CONFIG_HOME/slop-mcp/config.kdl or ~/.config/slop-mcp/config.kdl):")
			printMCPList(userCfg)
			fmt.Println()
		}
	}
}

type mcpListOptions struct {
	showLocal   bool
	showProject bool
	showUser    bool
	outputJSON  bool
	showHelp    bool
}

func parseMCPListArgs(args []string) (mcpListOptions, error) {
	opts := mcpListOptions{showLocal: true, showProject: true, showUser: true}
	for _, arg := range args {
		switch arg {
		case "--local":
			opts.showLocal = true
			opts.showProject = false
			opts.showUser = false
		case "--project":
			opts.showLocal = false
			opts.showProject = true
			opts.showUser = false
		case "--user":
			opts.showLocal = false
			opts.showProject = false
			opts.showUser = true
		case "--all":
			opts.showLocal = true
			opts.showProject = true
			opts.showUser = true
		case "--json":
			opts.outputJSON = true
		case "--help", "-h":
			opts.showHelp = true
		default:
			return opts, fmt.Errorf("unknown list option %q", arg)
		}
	}
	return opts, nil
}

func loadMCPListJSON(cwd string, showUser, showProject, showLocal bool) (map[string]config.MCPConfig, error) {
	allMCPs := make(map[string]config.MCPConfig)

	if showUser {
		userCfg, err := config.LoadUserConfig()
		if err != nil {
			return nil, fmt.Errorf("could not load user config: %w", err)
		}
		for name, mcp := range userCfg.MCPs {
			allMCPs[name] = mcp
		}
	}
	if showProject {
		projectCfg, err := config.LoadProjectConfig(cwd)
		if err != nil {
			return nil, fmt.Errorf("could not load project config: %w", err)
		}
		for name, mcp := range projectCfg.MCPs {
			allMCPs[name] = mcp
		}
	}
	if showLocal {
		localCfg, err := config.LoadLocalConfig(cwd)
		if err != nil {
			return nil, fmt.Errorf("could not load local config: %w", err)
		}
		for name, mcp := range localCfg.MCPs {
			allMCPs[name] = mcp
		}
	}

	return allMCPs, nil
}

func printMCPList(cfg *config.Config) {
	names := make([]string, 0, len(cfg.MCPs))
	for name := range cfg.MCPs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		mcp := cfg.MCPs[name]
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
  --user       Show only user config ($XDG_CONFIG_HOME/slop-mcp/config.kdl or ~/.config/slop-mcp/config.kdl)
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
	showHelp, err := parseMCPPathsArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printMCPPathsUsage()
		os.Exit(1)
	}
	if showHelp {
		printMCPPathsUsage()
		return
	}

	cwd, err := currentWorkingDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
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

func parseMCPPathsArgs(args []string) (bool, error) {
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			return true, nil
		default:
			return false, fmt.Errorf("unknown paths option %q", arg)
		}
	}
	return false, nil
}

func printMCPPathsUsage() {
	fmt.Print(`slop-mcp mcp paths - Show config file paths

Usage:
  slop-mcp mcp paths

Shows the paths to all config files and their existence status.

Config file precedence (later overrides earlier):
  1. User config ($XDG_CONFIG_HOME/slop-mcp/config.kdl or ~/.config/slop-mcp/config.kdl)
  2. Project config (.slop-mcp.kdl)
  3. Local config (.slop-mcp.local.kdl)
`)
}

func cmdMCPDump(args []string) {
	scope, outputJSON, showHelp, err := parseMCPDumpArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printMCPDumpUsage()
		os.Exit(1)
	}
	if showHelp {
		printMCPDumpUsage()
		return
	}

	cwd, err := currentWorkingDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	paths := config.ConfigPaths(cwd)

	if outputJSON {
		output, err := buildMCPDumpJSON(paths, scope)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		pretty, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(pretty))
		return
	}

	dumpFile := func(name, path string) {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			fmt.Printf("# %s config not found at %s\n", name, path)
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
			return
		}

		fmt.Printf("# %s (%s)\n", name, path)
		fmt.Println(string(data))
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

func parseMCPDumpArgs(args []string) (scope string, outputJSON bool, showHelp bool, err error) {
	for _, arg := range args {
		switch arg {
		case "--local":
			scope, err = setMCPDumpScope(scope, "local")
		case "--project":
			scope, err = setMCPDumpScope(scope, "project")
		case "--user":
			scope, err = setMCPDumpScope(scope, "user")
		case "--claude-desktop":
			scope, err = setMCPDumpScope(scope, "claude_desktop")
		case "--claude-code":
			scope, err = setMCPDumpScope(scope, "claude_code")
		case "--json":
			outputJSON = true
		case "--help", "-h":
			showHelp = true
		default:
			return "", false, false, fmt.Errorf("unknown dump option %q", arg)
		}
		if err != nil {
			return "", false, false, err
		}
	}
	return scope, outputJSON, showHelp, nil
}

func setMCPDumpScope(current, next string) (string, error) {
	if current != "" && current != next {
		return "", fmt.Errorf("only one dump scope may be provided")
	}
	return next, nil
}

func buildMCPDumpJSON(paths map[string]string, scope string) (any, error) {
	if scope != "" {
		return loadMCPDumpJSON(scope, paths[scope])
	}

	output := make(map[string]any, len(mcpDumpScopes))
	for _, name := range mcpDumpScopes {
		value, err := loadMCPDumpJSON(name, paths[name])
		if err != nil {
			return nil, err
		}
		output[name] = value
	}
	return output, nil
}

var mcpDumpScopes = []string{"local", "project", "user", "claude_desktop", "claude_code"}

func loadMCPDumpJSON(name, path string) (any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{"error": fmt.Sprintf("%s config not found at %s", name, path)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	if name == "claude_desktop" || name == "claude_code" {
		var jsonCfg config.ClaudeCodeSettings
		if err := json.Unmarshal(data, &jsonCfg); err == nil && jsonCfg.MCPServers != nil {
			return jsonCfg.MCPServers, nil
		}
		var desktopCfg config.JSONConfig
		if err := json.Unmarshal(data, &desktopCfg); err == nil && desktopCfg.MCPServers != nil {
			return desktopCfg.MCPServers, nil
		}
	}

	cfg, err := config.ParseKDLConfig(string(data), config.SourceProject)
	if err == nil {
		return cfg.MCPs, nil
	}
	return map[string]string{"raw": string(data)}, nil
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
	opts, err := parseMCPStatusArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printMCPStatusUsage()
		os.Exit(1)
	}
	if opts.showHelp {
		printMCPStatusUsage()
		return
	}

	url := fmt.Sprintf("http://localhost:%d/status", opts.port)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to server at %s: %v\n", url, err)
		fmt.Fprintf(os.Stderr, "Make sure slop-mcp is running with: slop-mcp serve --port %d\n", opts.port)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Server error: %s\n", strings.TrimSpace(string(body)))
		os.Exit(1)
	}

	var output struct {
		Status []registry.MCPFullStatus `json:"status"`
	}
	if err := json.Unmarshal(body, &output); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if opts.outputJSON {
		pretty, _ := json.MarshalIndent(output.Status, "", "  ")
		fmt.Println(string(pretty))
		return
	}

	// Print formatted status
	fmt.Printf("MCP Status (via localhost:%d):\n\n", opts.port)
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

type mcpStatusOptions struct {
	port       int
	outputJSON bool
	showHelp   bool
}

func parseMCPStatusArgs(args []string) (mcpStatusOptions, error) {
	opts := mcpStatusOptions{port: 8080}
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--json":
			opts.outputJSON = true
		case strings.HasPrefix(args[i], "--port="):
			port, err := parseMCPPort(strings.TrimPrefix(args[i], "--port="))
			if err != nil {
				return opts, err
			}
			opts.port = port
		case args[i] == "--port":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--port requires a value")
			}
			port, err := parseMCPPort(args[i+1])
			if err != nil {
				return opts, err
			}
			opts.port = port
			i++
		case args[i] == "--help" || args[i] == "-h":
			opts.showHelp = true
		default:
			return opts, fmt.Errorf("unknown status option %q", args[i])
		}
	}
	return opts, nil
}

func parseMCPPort(raw string) (int, error) {
	port, err := strconv.Atoi(raw)
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("invalid --port value %q: expected a port number (1-65535)", raw)
	}
	return port, nil
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
	cwd, err := currentWorkingDir()
	if err != nil {
		return nil, err
	}

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
  - Tokens are stored in $XDG_CONFIG_HOME/slop-mcp/auth.json or ~/.config/slop-mcp/auth.json
  - This command works without a running server
`)
}

func cmdMCPMetadata(args []string) {
	opts, err := parseMCPMetadataArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printMCPMetadataUsage()
		os.Exit(1)
	}
	if opts.showHelp {
		printMCPMetadataUsage()
		return
	}

	endpoint := fmt.Sprintf("http://localhost:%d/metadata", opts.port)
	if opts.mcpName != "" {
		endpoint += "?mcp_name=" + url.QueryEscape(opts.mcpName)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to server at %s: %v\n", endpoint, err)
		fmt.Fprintf(os.Stderr, "Make sure slop-mcp is running with: slop-mcp serve --port %d\n", opts.port)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Server error: %s\n", strings.TrimSpace(string(body)))
		os.Exit(1)
	}

	var output struct {
		Metadata []json.RawMessage `json:"metadata"`
		Total    int               `json:"total"`
	}
	if err := json.Unmarshal(body, &output); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	// Format output
	var formattedData []byte
	if opts.outputJSON {
		formattedData, _ = json.MarshalIndent(output.Metadata, "", "  ")
	} else {
		formattedData, _ = json.MarshalIndent(output.Metadata, "", "  ")
	}

	// Write to file or stdout
	if opts.outputFile != "" {
		if err := writeCLIOutputFile(opts.outputFile, formattedData); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Metadata written to %s (%d MCPs)\n", opts.outputFile, output.Total)
	} else {
		if !opts.outputJSON {
			fmt.Printf("MCP Metadata (%d servers):\n\n", output.Total)
		}
		fmt.Println(string(formattedData))
	}
}

type mcpMetadataOptions struct {
	port       int
	outputJSON bool
	outputFile string
	mcpName    string
	showHelp   bool
}

func parseMCPMetadataArgs(args []string) (mcpMetadataOptions, error) {
	opts := mcpMetadataOptions{port: 8080}
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--json":
			opts.outputJSON = true
		case strings.HasPrefix(args[i], "--port="):
			port, err := parseMCPPort(strings.TrimPrefix(args[i], "--port="))
			if err != nil {
				return opts, err
			}
			opts.port = port
		case args[i] == "--port":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--port requires a value")
			}
			port, err := parseMCPPort(args[i+1])
			if err != nil {
				return opts, err
			}
			opts.port = port
			i++
		case strings.HasPrefix(args[i], "--output="):
			opts.outputFile = strings.TrimPrefix(args[i], "--output=")
			if opts.outputFile == "" {
				return opts, fmt.Errorf("--output requires a value")
			}
		case args[i] == "--output":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--output requires a value")
			}
			opts.outputFile = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--mcp="):
			opts.mcpName = strings.TrimPrefix(args[i], "--mcp=")
			if opts.mcpName == "" {
				return opts, fmt.Errorf("--mcp requires a value")
			}
		case args[i] == "--mcp":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--mcp requires a value")
			}
			opts.mcpName = args[i+1]
			i++
		case args[i] == "--help" || args[i] == "-h":
			opts.showHelp = true
		default:
			return opts, fmt.Errorf("unknown metadata option %q", args[i])
		}
	}
	return opts, nil
}

func writeCLIOutputFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return atomicfile.WriteFile(path, data, 0644)
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
