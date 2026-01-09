package main

import (
	"fmt"
	"os"
)

const version = "0.3.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		cmdServe(os.Args[2:])
	case "run":
		cmdRun(os.Args[2:])
	case "mcp":
		if len(os.Args) < 3 {
			printMCPUsage()
			return
		}
		switch os.Args[2] {
		case "add":
			cmdMCPAdd(os.Args[3:])
		case "add-json":
			cmdMCPAddJSON(os.Args[3:])
		case "add-from-claude-desktop":
			cmdMCPAddFromClaudeDesktop(os.Args[3:])
		case "add-from-claude-code":
			cmdMCPAddFromClaudeCode(os.Args[3:])
		case "remove", "rm":
			cmdMCPRemove(os.Args[3:])
		case "get":
			cmdMCPGet(os.Args[3:])
		case "list", "ls":
			cmdMCPList(os.Args[3:])
		case "status":
			cmdMCPStatus(os.Args[3:])
		case "auth":
			cmdMCPAuth(os.Args[3:])
		case "paths":
			cmdMCPPaths(os.Args[3:])
		case "dump":
			cmdMCPDump(os.Args[3:])
		case "metadata":
			cmdMCPMetadata(os.Args[3:])
		case "help", "-h", "--help":
			printMCPUsage()
		default:
			printMCPUsage()
			os.Exit(1)
		}
	case "version", "-v", "--version":
		fmt.Printf("slop-mcp version %s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`slop-mcp - MCP Server for SLOP Orchestration

Usage:
  slop-mcp <command> [options]

Commands:
  serve                        Start the MCP server
  run                          Execute a SLOP script
  mcp add                      Register an MCP server
  mcp add-json                 Register an MCP server from JSON config
  mcp add-from-claude-desktop  Import MCPs from Claude Desktop
  mcp add-from-claude-code     Migrate MCPs from Claude Code user settings
  mcp remove                   Unregister an MCP server
  mcp get                      Get details of an MCP server
  mcp list                     List registered MCP servers
  mcp status                   Show live MCP connection status
  mcp auth                     Manage OAuth authentication
  mcp paths                    Show config file paths
  mcp dump                     Show config file contents
  mcp metadata                 Get full MCP metadata (tools, prompts, resources)
  version                      Show version
  help                         Show this help

Run 'slop-mcp <command> --help' for more information on a command.
`)
}

func printMCPUsage() {
	fmt.Print(`slop-mcp mcp - Manage MCP server registrations

Usage:
  slop-mcp mcp <subcommand> [options]

Subcommands:
  add <name> <command> [args...] [options]
      Register an MCP server (stdio transport)
      Options: --local, --project, --user, --transport, --url, --env, --header

  add-json <name> '<json>' [--local|--project|--user]
      Register an MCP server from JSON config

  add-from-claude-desktop [names...] [--local|--project|--user]
      Import MCPs from Claude Desktop config

  add-from-claude-code [names...] [--dry-run]
      Migrate MCPs from Claude Code user settings to slop-mcp user config
      Automatically excludes slop-mcp itself from migration

  remove <name> [--local|--project|--user]
      Unregister an MCP server

  get <name> [--json]
      Get details of an MCP server

  list [--local|--project|--user|--all] [--json]
      List registered MCP servers

  status [--port=<port>] [--json]
      Show live MCP connection status from a running server

  auth <action> [name]
      Manage OAuth authentication (login, logout, status, list)

  paths
      Show config file paths

  dump [--local|--project|--user|--claude-desktop|--claude-code] [--json]
      Show config file contents

  metadata [--port=<port>] [--output=<file>] [--mcp=<name>] [--json]
      Get full MCP metadata from a running server

Scope Options:
  --local      Local config (.slop-mcp.local.kdl) - gitignored
  --project    Project config (.slop-mcp.kdl) [default]
  --user       User config (~/.config/slop-mcp/config.kdl)

Transport Options:
  --transport=<type>  stdio (default), sse, http, streamable
  --url=<url>         Server URL (required for http transports)

Config Options:
  --env KEY=VALUE, -e KEY=VALUE      Set environment variable
  --header "Key: Value", -H "..."    Set HTTP header

Examples:
  # Add with stdio transport (default)
  slop-mcp mcp add filesystem npx -y @anthropic/mcp-server-filesystem /tmp

  # Add with environment variable
  slop-mcp mcp add brave npx -y @anthropic/mcp-server-brave-search -e BRAVE_API_KEY=xxx

  # Add with SSE transport
  slop-mcp mcp add myapi --transport=sse --url=http://localhost:3000/mcp

  # Import from Claude Desktop
  slop-mcp mcp add-from-claude-desktop

  # Migrate MCPs from Claude Code
  slop-mcp mcp add-from-claude-code --dry-run
  slop-mcp mcp add-from-claude-code

  # Add from JSON
  slop-mcp mcp add-json fs '{"command":"npx","args":["-y","@anthropic/mcp-server-filesystem","/tmp"]}'

  # Show all paths
  slop-mcp mcp paths

  # Check live connection status
  slop-mcp mcp status --port=8080

  # Authenticate with an MCP
  slop-mcp mcp auth login figma
  slop-mcp mcp auth list

  # Dump configs as JSON
  slop-mcp mcp dump --json
`)
}
