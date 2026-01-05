package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/slop-mcp/internal/config"
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
		case args[i] == "--local":
			scope = config.ScopeLocal
		case args[i] == "--project":
			scope = config.ScopeProject
		case args[i] == "--user":
			scope = config.ScopeUser
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
  --local       Add to local config (.slop-mcp.local.kdl, gitignored)
  --project     Add to project config (.slop-mcp.kdl) [default]
  --user        Add to user config (~/.config/slop-mcp/config.kdl)

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
		switch args[i] {
		case "--local":
			scope = config.ScopeLocal
		case "--project":
			scope = config.ScopeProject
		case "--user":
			scope = config.ScopeUser
		case "--help", "-h":
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
		switch args[i] {
		case "--local":
			scope = config.ScopeLocal
		case "--project":
			scope = config.ScopeProject
		case "--user":
			scope = config.ScopeUser
		case "--help", "-h":
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

func cmdMCPRemove(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: slop-mcp mcp remove <name> [--local|--project|--user]")
		os.Exit(1)
	}

	name := ""
	scope := config.ScopeProject

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--local":
			scope = config.ScopeLocal
		case "--project":
			scope = config.ScopeProject
		case "--user":
			scope = config.ScopeUser
		case "--help", "-h":
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
	fmt.Printf("  Local:          %s\n", paths["local"])
	fmt.Printf("  Project:        %s\n", paths["project"])
	fmt.Printf("  User:           %s\n", paths["user"])
	fmt.Printf("  Claude Desktop: %s\n", paths["claude_desktop"])

	fmt.Println("\nFile status:")
	for name, path := range paths {
		exists := "not found"
		if _, err := os.Stat(path); err == nil {
			exists = "exists"
		}
		fmt.Printf("  %-14s  %s\n", name+":", exists)
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
			if name == "claude_desktop" {
				var jsonCfg config.JSONConfig
				if err := json.Unmarshal(data, &jsonCfg); err == nil {
					pretty, _ := json.MarshalIndent(jsonCfg.MCPServers, "", "  ")
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
		for _, name := range []string{"local", "project", "user", "claude_desktop"} {
			dumpFile(name, paths[name])
			fmt.Println()
		}
	}
}

func printMCPDumpUsage() {
	fmt.Print(`slop-mcp mcp dump - Show config file contents

Usage:
  slop-mcp mcp dump [--local|--project|--user|--claude-desktop] [--json]

Options:
  --local           Dump local config only
  --project         Dump project config only
  --user            Dump user config only
  --claude-desktop  Dump Claude Desktop config only
  --json            Output as JSON
  --help, -h        Show this help

Examples:
  slop-mcp mcp dump                    # Dump all configs
  slop-mcp mcp dump --project          # Dump project config only
  slop-mcp mcp dump --claude-desktop   # Dump Claude Desktop config
  slop-mcp mcp dump --json             # Dump all as JSON
`)
}
