package server

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestParseExecuteToolArgs_PreservesLargeIntegers is the regression test for the
// silent write-path corruption: execute_tool used to decode `parameters` into a
// map[string]any, which coerces every JSON number to float64. float64 has a
// 53-bit mantissa, so any integer above 2^53 (lease tokens, snowflake IDs,
// nanosecond timestamps) was rounded before being forwarded to the target MCP.
// Reads passed (string ids); writes carrying large ints were silently mangled,
// breaking every mutation. Parameters must be carried verbatim as raw JSON.
func TestParseExecuteToolArgs_PreservesLargeIntegers(t *testing.T) {
	raw := json.RawMessage(`{` +
		`"mcp_name":"worktrack",` +
		`"tool_name":"task_workflow_step_advance",` +
		`"parameters":{"lease_token":9007199254740993,"id":1234567890123456789,"nested":{"big":1748700000000000123}}` +
		`}`)

	args, err := parseExecuteToolArgs(raw)
	if err != nil {
		t.Fatalf("parseExecuteToolArgs: %v", err)
	}
	if args.MCPName != "worktrack" {
		t.Errorf("mcp_name = %q, want worktrack", args.MCPName)
	}
	if args.ToolName != "task_workflow_step_advance" {
		t.Errorf("tool_name = %q, want task_workflow_step_advance", args.ToolName)
	}

	got := string(args.Parameters)
	for _, want := range []string{"9007199254740993", "1234567890123456789", "1748700000000000123"} {
		if !strings.Contains(got, want) {
			t.Errorf("large integer lost precision: want substring %q in forwarded parameters %q", want, got)
		}
	}
}

// TestParseExecuteToolArgs_EmptyParameters confirms absent/empty parameters
// decode to an empty raw value so the wrong-key detection and nil-normalization
// paths still trigger.
func TestParseExecuteToolArgs_EmptyParameters(t *testing.T) {
	for _, body := range []string{
		`{"mcp_name":"m","tool_name":"t"}`,
		`{"mcp_name":"m","tool_name":"t","parameters":{}}`,
		`{"mcp_name":"m","tool_name":"t","parameters":null}`,
	} {
		args, err := parseExecuteToolArgs(json.RawMessage(body))
		if err != nil {
			t.Fatalf("parseExecuteToolArgs(%s): %v", body, err)
		}
		if !isEmptyRawParams(args.Parameters) {
			t.Errorf("isEmptyRawParams=false for body %s (raw=%q)", body, string(args.Parameters))
		}
	}
}
