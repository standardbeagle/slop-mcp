package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// monitorMessagesPath returns the path to the monitor messages file.
func monitorMessagesPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "slop-mcp", "monitor-messages")
}

func cmdMessage(args []string) {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printMessageUsage()
		return
	}

	msg := strings.Join(args, " ")
	path := monitorMessagesPath()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, msg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printMessageUsage() {
	fmt.Print(`slop-mcp message - Send a message to a running monitor

Appends a message line to the monitor message file. Any running
'slop-mcp monitor' will print it to stdout (becoming a Monitor event).

Usage:
  slop-mcp message <text...>

Examples:
  slop-mcp message "deploy started"
  slop-mcp message Build failed: exit code 1

  # Background notify after a command finishes
  make build && slop-mcp message "build done" || slop-mcp message "build failed"
`)
}
