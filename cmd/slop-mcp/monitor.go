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
	opts, err := parseMonitorArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if opts.showHelp {
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

	if opts.timeout > 0 {
		var tcancel context.CancelFunc
		ctx, tcancel = context.WithTimeout(ctx, opts.timeout)
		defer tcancel()
	}

	// Always watch for incoming messages from 'slop-mcp message'
	go watchMessages(ctx)

	// If no script, just watch for messages until killed
	if opts.scriptFile == "" && opts.inlineScript == "" {
		<-ctx.Done()
		return
	}

	var script string
	if opts.inlineScript != "" {
		script = opts.inlineScript
	} else {
		data, err := os.ReadFile(opts.scriptFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading script file: %v\n", err)
			os.Exit(1)
		}
		script = string(data)
	}

	// Load merged three-tier config (same path as serve)
	cwd, err := currentWorkingDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	cfg, err := config.Load(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Create SLOP runtime
	rt := builtins.NewRuntime()
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
	_, err = rt.Execute(script)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		fmt.Fprintf(os.Stderr, "Monitor script error: %v\n", err)
		os.Exit(1)
	}
}

type monitorOptions struct {
	scriptFile   string
	inlineScript string
	showHelp     bool
	timeout      time.Duration
}

func parseMonitorArgs(args []string) (monitorOptions, error) {
	var opts monitorOptions
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-e":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("-e requires a script value")
			}
			opts.inlineScript = args[i+1]
			i++
		case args[i] == "--timeout":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--timeout requires a value")
			}
			timeout, err := parseTimeoutValue(args[i+1])
			if err != nil {
				return opts, err
			}
			opts.timeout = timeout
			i++
		case strings.HasPrefix(args[i], "--timeout="):
			timeout, err := parseTimeoutValue(strings.TrimPrefix(args[i], "--timeout="))
			if err != nil {
				return opts, err
			}
			opts.timeout = timeout
		case args[i] == "--help" || args[i] == "-h":
			opts.showHelp = true
		case strings.HasPrefix(args[i], "-"):
			return opts, fmt.Errorf("unknown monitor option %q", args[i])
		default:
			if opts.scriptFile != "" {
				return opts, fmt.Errorf("unexpected extra argument %q", args[i])
			}
			opts.scriptFile = args[i]
		}
	}
	if opts.scriptFile != "" && opts.inlineScript != "" {
		return opts, fmt.Errorf("provide either a script file or -e, not both")
	}
	return opts, nil
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
		if err != nil {
			continue
		}
		if info.Size() < offset {
			// File was truncated externally; start over from the beginning.
			offset = 0
		}
		if info.Size() == offset {
			continue
		}

		f, err := os.Open(path)
		if err != nil {
			continue
		}
		if offset > 0 {
			if _, err := f.Seek(offset, 0); err != nil {
				f.Close()
				continue
			}
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
  --timeout=<value>    Stop after the given time: seconds or duration like
                       "30s", "5m" (default: no timeout)
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
