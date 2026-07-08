package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/standardbeagle/slop-mcp/internal/config"
)

// monitorMessagesPath returns the path to the monitor messages file, scoped to
// the current project (working directory). Scoping by cwd keeps a `message`
// sender and a `monitor` in the same project talking to each other while
// isolating unrelated projects, which previously shared one global file and
// truncated each other's messages. Falls back to the unscoped name if the cwd
// cannot be determined.
func monitorMessagesPath() string {
	configDir := config.UserConfigDirPath()
	if configDir == "" {
		return ""
	}
	name := "monitor-messages"
	if cwd, err := getwd(); err == nil {
		sum := sha256.Sum256([]byte(cwd))
		name = "monitor-messages-" + hex.EncodeToString(sum[:8])
	}
	return filepath.Join(configDir, name)
}

func cmdMessage(args []string) {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printMessageUsage()
		return
	}

	msg := strings.Join(args, " ")
	path := monitorMessagesPath()
	if path == "" {
		fmt.Fprintln(os.Stderr, "Error: user config path unavailable")
		os.Exit(1)
	}

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
