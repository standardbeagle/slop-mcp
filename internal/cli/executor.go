package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Executor executes CLI tools.
type Executor struct {
	workdir string
}

// NewExecutor creates a new CLI tool executor.
func NewExecutor(workdir string) *Executor {
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	return &Executor{workdir: workdir}
}

// Execute runs a CLI tool with the given parameters.
func (e *Executor) Execute(ctx context.Context, tool *ToolConfig, params map[string]any) (*ToolResult, error) {
	startTime := time.Now()

	// Build command line arguments
	args, err := e.buildArgs(tool, params)
	if err != nil {
		return nil, fmt.Errorf("failed to build arguments: %w", err)
	}

	// Create command
	var cmd *exec.Cmd
	if tool.Shell {
		// Run through shell (less secure)
		shellCmd := tool.Command + " " + strings.Join(args, " ")
		cmd = exec.CommandContext(ctx, "sh", "-c", shellCmd)
	} else {
		// Direct execution (more secure)
		cmd = exec.CommandContext(ctx, tool.Command, args...)
	}

	// Set working directory
	workdir := tool.Workdir
	if workdir == "" || workdir == "." {
		workdir = e.workdir
	}
	cmd.Dir = workdir

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range tool.Env {
		if tool.ExpandEnv {
			v = os.ExpandEnv(v)
		}
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Handle stdin
	if stdinVal, ok := params["stdin"]; ok && stdinVal != nil {
		stdinStr, _ := stdinVal.(string)
		cmd.Stdin = strings.NewReader(stdinStr)
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Create timeout context if not already set
	timeout := tool.GetTimeout()
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd = exec.CommandContext(execCtx, cmd.Path, cmd.Args[1:]...)
	cmd.Dir = workdir
	cmd.Env = cmd.Env
	cmd.Stdin = cmd.Stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run command
	err = cmd.Run()

	result := &ToolResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(startTime).Milliseconds(),
	}

	// Get exit code
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = err.Error()
			result.ExitCode = -1
		}
	}

	// Check for failure conditions
	if !tool.AllowFail && result.ExitCode != 0 {
		result.Error = fmt.Sprintf("command failed with exit code %d", result.ExitCode)
	}

	// Handle stderr as failure if configured
	if tool.Stderr != nil && tool.Stderr.FailOnOutput && result.Stderr != "" {
		result.Error = "command produced stderr output"
	}

	// Trim stdout if configured (default true)
	if tool.Stdout == nil || tool.Stdout.Trim {
		result.Stdout = strings.TrimSpace(result.Stdout)
	}

	// Try to parse stdout as JSON if format is "json" or "auto"
	if tool.Stdout != nil {
		switch tool.Stdout.Format {
		case "json":
			// Validate JSON
			var js json.RawMessage
			if err := json.Unmarshal([]byte(result.Stdout), &js); err != nil {
				result.Error = fmt.Sprintf("output is not valid JSON: %v", err)
			}
		case "auto":
			// Try to detect JSON
			trimmed := strings.TrimSpace(result.Stdout)
			if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
				var js json.RawMessage
				if json.Unmarshal([]byte(trimmed), &js) == nil {
					result.Stdout = trimmed
				}
			}
		}
	}

	return result, nil
}

// buildArgs builds command line arguments from tool config and params.
func (e *Executor) buildArgs(tool *ToolConfig, params map[string]any) ([]string, error) {
	var args []string

	// Build positional arguments (sorted by position)
	positionalArgs := make([]ArgConfig, len(tool.Args))
	copy(positionalArgs, tool.Args)
	sort.Slice(positionalArgs, func(i, j int) bool {
		return positionalArgs[i].Position < positionalArgs[j].Position
	})

	for _, arg := range positionalArgs {
		paramName := toSnakeCase(arg.Name)
		val, exists := params[paramName]

		if !exists || val == nil {
			if arg.Required {
				return nil, fmt.Errorf("required argument '%s' not provided", arg.Name)
			}
			if arg.Default != nil {
				val = arg.Default
			} else {
				continue // Skip optional args without defaults
			}
		}

		// Handle array type
		if arg.Type == "array" {
			arrVals := parseArrayValue(val)
			for _, v := range arrVals {
				args = append(args, v)
			}
		} else {
			args = append(args, fmt.Sprintf("%v", val))
		}
	}

	// Build flags
	for _, flag := range tool.Flags {
		paramName := toSnakeCase(flag.Name)
		val, exists := params[paramName]

		if !exists || val == nil {
			if flag.Default != nil {
				val = flag.Default
			} else {
				continue
			}
		}

		// Determine which flag form to use
		flagStr := flag.Long
		if flagStr == "" {
			flagStr = flag.Short
		}
		if flagStr == "" {
			flagStr = "--" + flag.Name
		}

		switch flag.Type {
		case "boolean":
			if parseBoolValue(val) {
				args = append(args, flagStr)
			}

		case "array":
			arrVals := parseArrayValue(val)
			if flag.Repeat {
				// Repeat flag for each value: -t js -t py
				for _, v := range arrVals {
					args = append(args, flagStr, v)
				}
			} else {
				// Join values: --types js,py
				sep := flag.Separator
				if sep == "" {
					sep = ","
				}
				args = append(args, flagStr, strings.Join(arrVals, sep))
			}

		case "number":
			// Handle number formatting
			switch v := val.(type) {
			case float64:
				args = append(args, flagStr, strconv.FormatFloat(v, 'f', -1, 64))
			case int:
				args = append(args, flagStr, strconv.Itoa(v))
			default:
				args = append(args, flagStr, fmt.Sprintf("%v", val))
			}

		default: // string
			args = append(args, flagStr, fmt.Sprintf("%v", val))
		}
	}

	return args, nil
}
