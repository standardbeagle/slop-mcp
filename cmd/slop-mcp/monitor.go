package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/builtins"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop/pkg/slop"
)

func cmdMonitor(args []string) {
	var scriptFile string
	var inlineScript string
	showHelp := false
	timeout := 0

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-e" && i+1 < len(args):
			inlineScript = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--timeout="):
			var secs int
			fmt.Sscanf(args[i], "--timeout=%d", &secs)
			timeout = secs
		case args[i] == "--help" || args[i] == "-h":
			showHelp = true
		case !strings.HasPrefix(args[i], "-"):
			if scriptFile == "" {
				scriptFile = args[i]
			}
		}
	}

	if showHelp {
		printMonitorUsage()
		return
	}

	// Set up context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	if timeout > 0 {
		var tcancel context.CancelFunc
		ctx, tcancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer tcancel()
	}

	// Always watch for incoming messages from 'slop-mcp message'
	go watchMessages(ctx)

	// If no script, just watch for messages until killed
	if scriptFile == "" && inlineScript == "" {
		<-ctx.Done()
		return
	}

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

	// Load merged config
	cwd, _ := os.Getwd()
	cfg := config.NewConfig()
	if userCfg, err := config.LoadUserConfig(); err == nil {
		for name, mcp := range userCfg.MCPs {
			cfg.MCPs[name] = mcp
		}
	}
	if projectCfg, err := config.LoadProjectConfig(cwd); err == nil {
		for name, mcp := range projectCfg.MCPs {
			cfg.MCPs[name] = mcp
		}
	}
	if localCfg, err := config.LoadLocalConfig(cwd); err == nil {
		for name, mcp := range localCfg.MCPs {
			cfg.MCPs[name] = mcp
		}
	}

	// Create SLOP runtime
	rt := slop.NewRuntime()
	defer rt.Close()

	// Register standard built-ins
	builtins.RegisterCrypto(rt)
	builtins.RegisterSlopSearch(rt)
	builtins.RegisterJWT(rt)
	builtins.RegisterTemplate(rt)

	// Register persistent memory (mem_save, mem_load, mem_list, etc.)
	memStore := builtins.NewMemoryStore()
	builtins.RegisterMemory(rt, memStore)

	// Register monitor-specific built-ins
	registerMonitorBuiltins(rt)

	// Connect all configured MCPs
	for _, mcpCfg := range cfg.MCPs {
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

	// Execute the monitor script — print() goes to stdout for Monitor tool
	_, err := rt.Execute(script)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		fmt.Fprintf(os.Stderr, "Monitor script error: %v\n", err)
		os.Exit(1)
	}
}

// watchMessages polls the monitor messages file for new lines from 'slop-mcp message'.
// Each new line is printed to stdout (becoming a Monitor event).
func watchMessages(ctx context.Context) {
	path := monitorMessagesPath()

	// Truncate any stale messages from previous runs
	_ = os.Truncate(path, 0)

	var offset int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}

		info, err := os.Stat(path)
		if err != nil || info.Size() <= offset {
			continue
		}

		f, err := os.Open(path)
		if err != nil {
			continue
		}
		if offset > 0 {
			f.Seek(offset, 0)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if line != "" {
				fmt.Println(line)
			}
		}
		offset = info.Size()
		f.Close()
	}
}

// registerMonitorBuiltins adds convenience functions for monitor scripts.
func registerMonitorBuiltins(rt *slop.Runtime) {
	// changed(key, value) — returns true when value differs from last call
	// with the same key. In-memory only (use mem_save/mem_load for persistence).
	var mu sync.Mutex
	seen := make(map[string]string)

	rt.RegisterBuiltin("changed", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("changed(key, value) requires 2 arguments")
		}
		keyRaw := slop.ValueToGo(args[0])
		keyStr, ok := keyRaw.(string)
		if !ok {
			return nil, fmt.Errorf("changed: key must be a string")
		}

		// Serialize value for comparison
		valBytes, err := json.Marshal(slop.ValueToGo(args[1]))
		if err != nil {
			return nil, fmt.Errorf("changed: cannot serialize value: %w", err)
		}
		current := string(valBytes)

		mu.Lock()
		prev, exists := seen[keyStr]
		seen[keyStr] = current
		mu.Unlock()

		return slop.NewBoolValue(!exists || prev != current), nil
	})
}

func printMonitorUsage() {
	fmt.Print(`slop-mcp monitor - Run a SLOP script as a Claude Code monitor source

Executes a SLOP script with all configured MCPs connected. The script's
print() output goes to stdout as one line per event, designed for Claude
Code's Monitor tool. Also watches for messages sent via 'slop-mcp message'.

Without a script, just watches for incoming messages.

Usage:
  slop-mcp monitor [script.slop]        Run a monitor script file
  slop-mcp monitor -e '<script>'        Run an inline monitor script
  slop-mcp monitor                      Watch for messages only

Options:
  -e '<script>'        Execute inline script
  --timeout=<secs>     Stop after N seconds (default: no timeout)
  --help, -h           Show this help

Built-in Functions (in addition to standard SLOP built-ins):
  changed(key, value)  Returns true if value differs from last call with
                       same key (in-memory, resets on restart).
  mem_save/mem_load    Persistent memory across restarts (standard SLOP).

Examples:
  # Watch for messages only (no script)
  slop-mcp monitor &
  slop-mcp message "deploy started"

  # Poll for changes with a script
  slop-mcp monitor watch-deploy.slop

  # Use with Claude Code Monitor tool:
  #   Monitor({
  #     command: "slop-mcp monitor",
  #     description: "build events",
  #     persistent: true
  #   })
`)
}
