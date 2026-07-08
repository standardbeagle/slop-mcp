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

var getwd = os.Getwd

// Executor executes CLI tools.
type Executor struct {
	workdir    string
	workdirErr error
}

// NewExecutor creates a new CLI tool executor.
func NewExecutor(workdir string) *Executor {
	var err error
	if workdir == "" {
		workdir, err = getwd()
	}
	return &Executor{workdir: workdir, workdirErr: err}
}

// Execute runs a CLI tool with the given parameters.
func (e *Executor) Execute(ctx context.Context, tool *ToolConfig, params map[string]any) (*ToolResult, error) {
	startTime := time.Now()

	// Build command line arguments
	args, err := e.buildArgs(tool, params)
	if err != nil {
		return nil, fmt.Errorf("failed to build arguments: %w", err)
	}

	// Apply tool timeout
	timeout := tool.GetTimeout()
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create command
	var cmd *exec.Cmd
	if tool.Shell {
		// Run through shell; each argument is single-quoted so parameter
		// values cannot inject shell syntax.
		shellCmd := tool.Command
		for _, arg := range args {
			shellCmd += " " + shellQuote(arg)
		}
		cmd = exec.CommandContext(execCtx, "sh", "-c", shellCmd)
	} else {
		// Direct execution (more secure)
		cmd = exec.CommandContext(execCtx, tool.Command, args...)
	}

	// Set working directory
	workdir := tool.Workdir
	if workdir == "" || workdir == "." {
		if e.workdirErr != nil {
			return nil, fmt.Errorf("failed to determine CLI tool working directory: %w", e.workdirErr)
		}
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
		stdinStr, isStr := stdinVal.(string)
		if !isStr {
			return nil, fmt.Errorf("stdin must be a string, got %T", stdinVal)
		}
		cmd.Stdin = strings.NewReader(stdinStr)
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
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

	// A deadline hit means the process was killed by the timeout: report that
	// explicitly instead of a bare "exit code -1".
	if err != nil && execCtx.Err() == context.DeadlineExceeded {
		result.Error = fmt.Sprintf("command timed out after %s", timeout)
	}

	// Check for failure conditions (keep a more specific error if already set)
	if !tool.AllowFail && result.ExitCode != 0 && result.Error == "" {
		result.Error = fmt.Sprintf("command failed with exit code %d", result.ExitCode)
	}

	// Handle stderr as failure if configured. Keep any more specific error
	// already set (e.g. a timeout cause) rather than overwriting it.
	if tool.Stderr != nil && tool.Stderr.FailOnOutput && result.Stderr != "" && result.Error == "" {
		result.Error = "command produced stderr output"
	}

	// Trim stdout unless explicitly disabled (default true).
	if tool.Stdout.TrimEnabled() {
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

// shellQuote wraps s in single quotes for POSIX sh, escaping any embedded
// single quote as quote-backslash-quote-quote so the value is passed literally.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// buildArgs builds command line arguments from tool config and params.
func (e *Executor) buildArgs(tool *ToolConfig, params map[string]any) ([]string, error) {
	var args []string

	// Build positional arguments (sorted by position)
	positionalArgs := make([]ArgConfig, len(tool.Args))
	copy(positionalArgs, tool.Args)
	// Stable sort so args sharing a Position (e.g. all default 0) keep their
	// declared order instead of being shuffled nondeterministically per run.
	sort.SliceStable(positionalArgs, func(i, j int) bool {
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
		switch arg.Type {
		case "array":
			vals, err := parseArrayValue(val)
			if err != nil {
				return nil, fmt.Errorf("invalid argument '%s': %w", arg.Name, err)
			}
			if err := validateEnum("argument", arg.Name, arg.Enum, vals...); err != nil {
				return nil, err
			}
			args = append(args, vals...)
		case "boolean":
			bVal, err := parseBoolValue(val)
			if err != nil {
				return nil, fmt.Errorf("invalid argument '%s': %w", arg.Name, err)
			}
			sVal := strconv.FormatBool(bVal)
			if err := validateEnum("argument", arg.Name, arg.Enum, sVal); err != nil {
				return nil, err
			}
			args = append(args, sVal)
		case "number":
			sVal, err := parseNumberValue(val)
			if err != nil {
				return nil, fmt.Errorf("invalid argument '%s': %w", arg.Name, err)
			}
			if err := validateEnum("argument", arg.Name, arg.Enum, sVal); err != nil {
				return nil, err
			}
			args = append(args, sVal)
		default:
			sVal := fmt.Sprintf("%v", val)
			if err := validateEnum("argument", arg.Name, arg.Enum, sVal); err != nil {
				return nil, err
			}
			args = append(args, sVal)
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
			bVal, err := parseBoolValue(val)
			if err != nil {
				return nil, fmt.Errorf("invalid flag '%s': %w", flag.Name, err)
			}
			if bVal {
				args = append(args, flagStr)
			}

		case "array":
			arrVals, err := parseArrayValue(val)
			if err != nil {
				return nil, fmt.Errorf("invalid flag '%s': %w", flag.Name, err)
			}
			if err := validateEnum("flag", flag.Name, flag.Enum, arrVals...); err != nil {
				return nil, err
			}
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
			sVal, err := parseNumberValue(val)
			if err != nil {
				return nil, fmt.Errorf("invalid flag '%s': %w", flag.Name, err)
			}
			if err := validateEnum("flag", flag.Name, flag.Enum, sVal); err != nil {
				return nil, err
			}
			args = append(args, flagStr, sVal)

		default: // string
			sVal := fmt.Sprintf("%v", val)
			if err := validateEnum("flag", flag.Name, flag.Enum, sVal); err != nil {
				return nil, err
			}
			args = append(args, flagStr, sVal)
		}
	}

	return args, nil
}

// validateEnum returns a parameter error when any value is outside the
// declared enum. An empty enum means unrestricted.
func validateEnum(kind, name string, enum []string, values ...string) error {
	if len(enum) == 0 {
		return nil
	}
	for _, v := range values {
		ok := false
		for _, e := range enum {
			if v == e {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("invalid value %q for %s '%s': must be one of: %s", v, kind, name, strings.Join(enum, ", "))
		}
	}
	return nil
}
