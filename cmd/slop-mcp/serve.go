package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/server"
)

func cmdServe(args []string) {
	opts, err := parseServeArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if opts.showHelp {
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
	var runErr error
	if opts.port > 0 {
		runErr = srv.RunHTTP(ctx, opts.port)
	} else {
		runErr = srv.RunStdio(ctx)
	}
	// Context cancellation from signals is clean shutdown, not an error
	if runErr != nil && runErr != context.Canceled {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", runErr)
		os.Exit(1)
	}
}

type serveOptions struct {
	port     int
	showHelp bool
}

func parseServeArgs(args []string) (serveOptions, error) {
	var opts serveOptions
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--port" || args[i] == "-p":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--port requires a value")
			}
			port, err := parseServePort(args[i+1])
			if err != nil {
				return opts, err
			}
			opts.port = port
			i++
		case strings.HasPrefix(args[i], "--port="):
			port, err := parseServePort(strings.TrimPrefix(args[i], "--port="))
			if err != nil {
				return opts, err
			}
			opts.port = port
		case args[i] == "--help" || args[i] == "-h":
			opts.showHelp = true
		default:
			return opts, fmt.Errorf("unknown serve option %q", args[i])
		}
	}
	return opts, nil
}

func parseServePort(raw string) (int, error) {
	p, err := strconv.Atoi(raw)
	if err != nil || p <= 0 || p > 65535 {
		return 0, fmt.Errorf("invalid --port value %q: expected a port number (1-65535)", raw)
	}
	return p, nil
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
  The server loads MCP configurations from (later overrides earlier):
  1. User config: $XDG_CONFIG_HOME/slop-mcp/config.kdl or ~/.config/slop-mcp/config.kdl
  2. Project config: .slop-mcp.kdl (in current directory)
  3. Local config: .slop-mcp.local.kdl (gitignored, for secrets)
`)
}
