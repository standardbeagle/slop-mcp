package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/server"
)

func cmdServe(args []string) {
	port := 0
	showHelp := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port", "-p":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &port)
				i++
			}
		case "--help", "-h":
			showHelp = true
		}
	}

	if showHelp {
		printServeUsage()
		return
	}

	// Get current directory for project config
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	// Load and merge configs
	cfg, err := config.Load(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Create server
	srv, err := server.NewFromConfig(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating server: %v\n", err)
		os.Exit(1)
	}
	defer srv.Close()

	// Set up context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// Run with appropriate transport
	if port > 0 {
		if err := srv.RunHTTP(ctx, port); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	} else {
		if err := srv.RunStdio(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}
}

func printServeUsage() {
	fmt.Print(`slop-mcp serve - Start the MCP server

Usage:
  slop-mcp serve [options]

Options:
  --port, -p PORT    Run with SSE/HTTP transport on PORT
                     (default: stdio transport)
  --help, -h         Show this help

Examples:
  slop-mcp serve                    # Run with stdio transport
  slop-mcp serve --port 8080        # Run with HTTP/SSE on port 8080

Configuration:
  The server loads MCP configurations from:
  1. User config: ~/.config/slop-mcp/config.kdl
  2. Project config: .slop-mcp.kdl (in current directory)

  Project config overrides user config for the same MCP name.
`)
}
