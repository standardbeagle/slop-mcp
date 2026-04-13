package server

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildAgntWatchCommand_AllTarget verifies the default "all" target produces
// a monitor command with no --types filter.
func TestBuildAgntWatchCommand_AllTarget(t *testing.T) {
	out, err := buildAgntWatchCommand(AgntWatchInput{
		Target:     "all",
		AgntBinary: "/usr/local/bin/agnt",
	})
	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin/agnt monitor --format compact", out.Command)
	assert.NotEmpty(t, out.Description)
}

// TestBuildAgntWatchCommand_DefaultTargetIsAll verifies that omitting target
// defaults to "all".
func TestBuildAgntWatchCommand_DefaultTargetIsAll(t *testing.T) {
	out, err := buildAgntWatchCommand(AgntWatchInput{
		AgntBinary: "/usr/local/bin/agnt",
	})
	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin/agnt monitor --format compact", out.Command)
}

// TestBuildAgntWatchCommand_ErrorsTarget verifies the "errors" target
// produces --types error,diagnostic.
func TestBuildAgntWatchCommand_ErrorsTarget(t *testing.T) {
	out, err := buildAgntWatchCommand(AgntWatchInput{
		Target:     "errors",
		AgntBinary: "/usr/local/bin/agnt",
	})
	require.NoError(t, err)
	assert.Contains(t, out.Command, "--types error,diagnostic")
	assert.Contains(t, out.Command, "/usr/local/bin/agnt monitor")
	assert.Contains(t, out.Command, "--format compact")
}

// TestBuildAgntWatchCommand_InteractionsTarget verifies the "interactions"
// target produces the correct comma-separated types.
func TestBuildAgntWatchCommand_InteractionsTarget(t *testing.T) {
	out, err := buildAgntWatchCommand(AgntWatchInput{
		Target:     "interactions",
		AgntBinary: "/usr/local/bin/agnt",
	})
	require.NoError(t, err)
	assert.Contains(t, out.Command, "--types panel_message,interaction,sketch")
}

// TestBuildAgntWatchCommand_ProcessTargetRequiresProcessID verifies that the
// "process" target errors out without a process_id.
func TestBuildAgntWatchCommand_ProcessTargetRequiresProcessID(t *testing.T) {
	_, err := buildAgntWatchCommand(AgntWatchInput{
		Target:     "process",
		AgntBinary: "/usr/local/bin/agnt",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "process_id")
}

// TestBuildAgntWatchCommand_ProcessTarget verifies the process target produces
// a command with --types process and --process.
func TestBuildAgntWatchCommand_ProcessTarget(t *testing.T) {
	out, err := buildAgntWatchCommand(AgntWatchInput{
		Target:     "process",
		ProcessID:  "app",
		AgntBinary: "/usr/local/bin/agnt",
	})
	require.NoError(t, err)
	assert.Contains(t, out.Command, "--types process")
	assert.Contains(t, out.Command, "--process app")
}

// TestBuildAgntWatchCommand_ProxyFilter verifies --proxy is added when given.
func TestBuildAgntWatchCommand_ProxyFilter(t *testing.T) {
	out, err := buildAgntWatchCommand(AgntWatchInput{
		Target:     "errors",
		ProxyID:    "dev",
		AgntBinary: "/usr/local/bin/agnt",
	})
	require.NoError(t, err)
	assert.Contains(t, out.Command, "--proxy dev")
}

// TestBuildAgntWatchCommand_SeverityFilter verifies --severity is forwarded.
func TestBuildAgntWatchCommand_SeverityFilter(t *testing.T) {
	out, err := buildAgntWatchCommand(AgntWatchInput{
		Target:     "errors",
		Severity:   "warning",
		AgntBinary: "/usr/local/bin/agnt",
	})
	require.NoError(t, err)
	assert.Contains(t, out.Command, "--severity warning")
}

// TestBuildAgntWatchCommand_FormatJSON verifies json format is forwarded.
func TestBuildAgntWatchCommand_FormatJSON(t *testing.T) {
	out, err := buildAgntWatchCommand(AgntWatchInput{
		Target:     "all",
		Format:     "json",
		AgntBinary: "/usr/local/bin/agnt",
	})
	require.NoError(t, err)
	assert.Contains(t, out.Command, "--format json")
	assert.NotContains(t, out.Command, "--format compact")
}

// TestBuildAgntWatchCommand_InvalidTarget verifies unknown targets error.
func TestBuildAgntWatchCommand_InvalidTarget(t *testing.T) {
	_, err := buildAgntWatchCommand(AgntWatchInput{
		Target:     "bogus",
		AgntBinary: "/usr/local/bin/agnt",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid target")
}

// TestBuildAgntWatchCommand_InvalidFormat verifies unknown formats error.
func TestBuildAgntWatchCommand_InvalidFormat(t *testing.T) {
	_, err := buildAgntWatchCommand(AgntWatchInput{
		Target:     "all",
		Format:     "xml",
		AgntBinary: "/usr/local/bin/agnt",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format")
}

// TestBuildAgntWatchCommand_InvalidSeverity verifies unknown severities error.
func TestBuildAgntWatchCommand_InvalidSeverity(t *testing.T) {
	_, err := buildAgntWatchCommand(AgntWatchInput{
		Target:     "all",
		Severity:   "fatal",
		AgntBinary: "/usr/local/bin/agnt",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid severity")
}

// TestBuildAgntWatchCommand_QuotesArgsWithSpaces ensures paths/IDs containing
// spaces get shell-escaped.
func TestBuildAgntWatchCommand_QuotesArgsWithSpaces(t *testing.T) {
	out, err := buildAgntWatchCommand(AgntWatchInput{
		Target:     "all",
		AgntBinary: "/opt/my agnt/bin/agnt",
	})
	require.NoError(t, err)
	assert.True(t, strings.Contains(out.Command, "'/opt/my agnt/bin/agnt'"),
		"binary path with spaces should be single-quoted, got: %s", out.Command)
}

// TestBuildAgntWatchCommand_CombinedFilters exercises all filters at once to
// verify the full command matches the flag order agnt monitor expects.
func TestBuildAgntWatchCommand_CombinedFilters(t *testing.T) {
	out, err := buildAgntWatchCommand(AgntWatchInput{
		Target:     "errors",
		ProxyID:    "dev",
		Severity:   "error",
		Format:     "json",
		AgntBinary: "/usr/local/bin/agnt",
	})
	require.NoError(t, err)
	expected := "/usr/local/bin/agnt monitor --types error,diagnostic --proxy dev --severity error --format json"
	assert.Equal(t, expected, out.Command)
	assert.Equal(t, "Errors on proxy dev", out.Description)
}

// TestHandleAgntWatch_ResolvesBinaryFromPath verifies the handler path resolves
// an agnt binary either from PATH or returns a clear error.
func TestHandleAgntWatch_ResolvesBinaryFromPath(t *testing.T) {
	s := &Server{}
	ctx := context.Background()

	// When agnt is not on PATH we still want a clean error rather than panic.
	_, output, err := s.handleAgntWatch(ctx, &mcp.CallToolRequest{}, AgntWatchInput{
		Target: "all",
	})
	// Either PATH has agnt (happy path) OR we got a clear error.
	if err == nil {
		assert.NotEmpty(t, output.Command, "command should be set on success")
		assert.Contains(t, output.Command, "monitor")
	} else {
		assert.Contains(t, err.Error(), "agnt")
	}
}

// TestHandleAgntWatch_HonorsAGNTBINARYEnv verifies that AGNT_BINARY wins over
// PATH lookup, allowing users to point slop-mcp at a non-standard install.
func TestHandleAgntWatch_HonorsAGNTBINARYEnv(t *testing.T) {
	// Create a fake binary file for the env var to point at.
	tmpFile, err := os.CreateTemp(t.TempDir(), "fake-agnt-*")
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	t.Setenv("AGNT_BINARY", tmpFile.Name())

	s := &Server{}
	ctx := context.Background()
	_, output, err := s.handleAgntWatch(ctx, &mcp.CallToolRequest{}, AgntWatchInput{
		Target: "errors",
	})
	require.NoError(t, err)
	assert.Contains(t, output.Command, tmpFile.Name())
	assert.Contains(t, output.Command, "--types error,diagnostic")
}

// TestHandleAgntWatch_AGNTBINARYPointingAtDirectoryFails ensures a bogus
// override produces a clean error instead of a cryptic exec failure later.
func TestHandleAgntWatch_AGNTBINARYPointingAtDirectoryFails(t *testing.T) {
	t.Setenv("AGNT_BINARY", t.TempDir())

	s := &Server{}
	ctx := context.Background()
	_, _, err := s.handleAgntWatch(ctx, &mcp.CallToolRequest{}, AgntWatchInput{Target: "all"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AGNT_BINARY")
}
