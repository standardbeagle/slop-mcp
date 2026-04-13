package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AgntWatchInput is the input for the agnt_watch tool.
//
// This tool is a passthrough to agnt's own `watch` surface: it returns a
// shell command string that the caller runs through Claude Code's Monitor
// tool (or any other long-running shell runner). Flags mirror `agnt monitor`
// exactly so there are no new concepts to learn.
type AgntWatchInput struct {
	// Target selects a preset filter. One of: errors, interactions, process, all.
	// Default: all.
	Target string `json:"target,omitempty"`
	// ProxyID filters events to a specific agnt proxy.
	ProxyID string `json:"proxy_id,omitempty"`
	// ProcessID filters events to a specific agnt-managed process.
	// Required when target is "process".
	ProcessID string `json:"process_id,omitempty"`
	// Severity filters to a minimum severity: info, warning, error.
	Severity string `json:"severity,omitempty"`
	// Format is one of: compact (default), json.
	Format string `json:"format,omitempty"`

	// AgntBinary overrides the resolved agnt binary path. Internal-only,
	// used by tests and when the caller already knows the path.
	AgntBinary string `json:"-"`
}

// AgntWatchOutput is the output for the agnt_watch tool.
type AgntWatchOutput struct {
	// Command is the full shell command to run through Claude Code's Monitor
	// tool or any other shell runner. One line per daemon event to stdout.
	Command string `json:"command"`
	// Description is a short human-readable summary of what the command watches.
	Description string `json:"description"`
}

// agntWatchInputSchema is the JSON schema for the agnt_watch tool.
var agntWatchInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"target": {
			"type": "string",
			"description": "What to watch: errors, interactions, process, or all (default: all)"
		},
		"proxy_id": {
			"type": "string",
			"description": "Filter events to a specific agnt proxy"
		},
		"process_id": {
			"type": "string",
			"description": "Filter events to a specific agnt-managed process (required when target is process)"
		},
		"severity": {
			"type": "string",
			"description": "Minimum severity: info, warning, or error"
		},
		"format": {
			"type": "string",
			"description": "Output format: compact (default) or json"
		}
	},
	"additionalProperties": false
}`)

// agntWatchTarget describes a named preset of agnt event types.
type agntWatchTarget struct {
	// types is the value passed to agnt monitor --types. Empty means no filter.
	types string
	// needsProcess is true when the preset requires a process_id.
	needsProcess bool
	// describe builds a short human summary of the watch.
	describe func(AgntWatchInput) string
}

// agntWatchTargets maps preset names to their configuration. These mirror
// agnt's own watch tool exactly — see /home/beagle/work/core/agnt/internal/tools/watch.go.
var agntWatchTargets = map[string]agntWatchTarget{
	"errors": {
		types: "error,diagnostic",
		describe: func(in AgntWatchInput) string {
			if in.ProxyID != "" {
				return fmt.Sprintf("Errors on proxy %s", in.ProxyID)
			}
			return "All errors"
		},
	},
	"interactions": {
		types: "panel_message,interaction,sketch",
		describe: func(in AgntWatchInput) string {
			if in.ProxyID != "" {
				return fmt.Sprintf("User interactions on proxy %s", in.ProxyID)
			}
			return "All user interactions"
		},
	},
	"process": {
		types:        "process",
		needsProcess: true,
		describe: func(in AgntWatchInput) string {
			return fmt.Sprintf("Process output for %s", in.ProcessID)
		},
	},
	"all": {
		types: "",
		describe: func(in AgntWatchInput) string {
			return "All agnt daemon events"
		},
	},
}

// validSeverities are the severity levels accepted by agnt monitor.
var validSeverities = map[string]struct{}{
	"info":    {},
	"warning": {},
	"error":   {},
}

// validFormats are the output formats accepted by agnt monitor.
var validFormats = map[string]struct{}{
	"compact": {},
	"json":    {},
}

// buildAgntWatchCommand turns an AgntWatchInput into a ready-to-run shell
// command string plus a human-readable description. It validates inputs and
// shell-escapes any argument containing spaces.
func buildAgntWatchCommand(input AgntWatchInput) (AgntWatchOutput, error) {
	target := input.Target
	if target == "" {
		target = "all"
	}

	cfg, ok := agntWatchTargets[target]
	if !ok {
		return AgntWatchOutput{}, fmt.Errorf("invalid target %q: must be one of errors, interactions, process, all", target)
	}

	if cfg.needsProcess && input.ProcessID == "" {
		return AgntWatchOutput{}, fmt.Errorf("process_id is required for target %q", target)
	}

	format := input.Format
	if format == "" {
		format = "compact"
	}
	if _, ok := validFormats[format]; !ok {
		return AgntWatchOutput{}, fmt.Errorf("invalid format %q: must be compact or json", format)
	}

	if input.Severity != "" {
		if _, ok := validSeverities[input.Severity]; !ok {
			return AgntWatchOutput{}, fmt.Errorf("invalid severity %q: must be info, warning, or error", input.Severity)
		}
	}

	if input.AgntBinary == "" {
		return AgntWatchOutput{}, fmt.Errorf("agnt binary path not provided")
	}

	args := []string{input.AgntBinary, "monitor"}
	if cfg.types != "" {
		args = append(args, "--types", cfg.types)
	}
	if input.ProxyID != "" {
		args = append(args, "--proxy", input.ProxyID)
	}
	if input.ProcessID != "" {
		args = append(args, "--process", input.ProcessID)
	}
	if input.Severity != "" {
		args = append(args, "--severity", input.Severity)
	}
	args = append(args, "--format", format)

	return AgntWatchOutput{
		Command:     joinShellArgs(args),
		Description: cfg.describe(input),
	}, nil
}

// joinShellArgs joins args with spaces, single-quoting any arg that contains
// a space. This is deliberately minimal: agnt IDs and socket paths may have
// spaces in edge cases, but none of the fixed flags do.
func joinShellArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t") {
			quoted[i] = "'" + a + "'"
		} else {
			quoted[i] = a
		}
	}
	return strings.Join(quoted, " ")
}

// resolveAgntBinary finds the agnt binary. It first honors the AGNT_BINARY
// environment variable (for non-standard install locations), then falls back
// to looking up `agnt` on PATH.
func resolveAgntBinary() (string, error) {
	if override := strings.TrimSpace(os.Getenv("AGNT_BINARY")); override != "" {
		if info, err := os.Stat(override); err != nil || info.IsDir() {
			return "", fmt.Errorf("AGNT_BINARY=%q is not a usable file", override)
		}
		return override, nil
	}
	path, err := exec.LookPath("agnt")
	if err != nil {
		return "", fmt.Errorf("agnt binary not found on PATH: install agnt (https://github.com/standardbeagle/agnt) or set AGNT_BINARY: %w", err)
	}
	return path, nil
}

// handleAgntWatch is the MCP tool handler for agnt_watch.
func (s *Server) handleAgntWatch(
	ctx context.Context,
	req *mcp.CallToolRequest,
	input AgntWatchInput,
) (*mcp.CallToolResult, AgntWatchOutput, error) {
	if input.AgntBinary == "" {
		binary, err := resolveAgntBinary()
		if err != nil {
			return nil, AgntWatchOutput{}, err
		}
		input.AgntBinary = binary
	}

	out, err := buildAgntWatchCommand(input)
	if err != nil {
		return nil, AgntWatchOutput{}, err
	}
	return nil, out, nil
}
