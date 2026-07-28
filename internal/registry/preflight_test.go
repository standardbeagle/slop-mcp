package registry

import (
	"encoding/json"
	"strings"
	"testing"
)

// worktrackToolSchema mirrors the real shape the 2026-07-27 audit kept seeing
// mis-called: a task-class verb keyed on `id` with an OCC `version`, next to a
// workflow-step verb keyed on `taskId`.
func worktrackToolSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":      map[string]any{"type": "string", "description": "Task id."},
			"version": map[string]any{"type": "integer", "description": "Expected current version."},
			"patch":   map[string]any{"type": "object"},
			"leaseHolder": map[string]any{
				"type": "string", "description": "Lease holder identifier.",
			},
		},
		"required": []any{"id", "version", "patch"},
	}
}

func preflightRegistry(t *testing.T) *Registry {
	t.Helper()
	r := New()
	r.AddToolsForTesting("worktrack", []ToolInfo{
		{Name: "task_update", MCPName: "worktrack", InputSchema: worktrackToolSchema()},
		{Name: "opaque_tool", MCPName: "worktrack"},
	})
	return r
}

func preflight(t *testing.T, r *Registry, tool, params string) error {
	t.Helper()
	return r.PreflightParameters("worktrack", tool, json.RawMessage(params))
}

func TestPreflight_AcceptsAValidCall(t *testing.T) {
	r := preflightRegistry(t)
	if err := preflight(t, r, "task_update", `{"id":"01A","version":3,"patch":{"status":"done"}}`); err != nil {
		t.Fatalf("valid call rejected: %v", err)
	}
}

func TestPreflight_ReportsMissingRequiredParameter(t *testing.T) {
	r := preflightRegistry(t)

	err := preflight(t, r, "task_update", `{"id":"01A","patch":{"status":"done"}}`)
	if err == nil {
		t.Fatal("expected a missing-required rejection")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Missing required parameters") || !strings.Contains(msg, "version") {
		t.Fatalf("error must name the missing parameter, got: %s", msg)
	}
}

func TestPreflight_SuggestsTheCorrectNameForANearMiss(t *testing.T) {
	r := preflightRegistry(t)

	// The exact substitution the audit recorded: a task-class verb called with
	// the workflow-class key.
	err := preflight(t, r, "task_update", `{"taskId":"01A","version":3,"patch":{}}`)
	if err == nil {
		t.Fatal("expected a rejection naming the correct key")
	}
	msg := err.Error()
	if !strings.Contains(msg, "taskId") || !strings.Contains(msg, "did you mean 'id'") {
		t.Fatalf("error must map taskId -> id, got: %s", msg)
	}
	// The full valid set travels with the rejection so the retry needs no probe.
	if !strings.Contains(msg, "Expected parameters") || !strings.Contains(msg, "leaseHolder") {
		t.Fatalf("error must list the valid parameters, got: %s", msg)
	}
}

func TestPreflight_ForwardsAnUnknownNameWithNoNearMatch(t *testing.T) {
	r := preflightRegistry(t)

	// Additional properties are legal by default; the target server decides.
	err := preflight(t, r, "task_update", `{"id":"01A","version":3,"patch":{},"acpSecret":"x"}`)
	if err != nil {
		t.Fatalf("unrecognized-but-unrelated key must be forwarded, got: %v", err)
	}
}

func TestPreflight_HasNoOpinionWithoutASchema(t *testing.T) {
	r := preflightRegistry(t)

	if err := preflight(t, r, "opaque_tool", `{"anything":1}`); err != nil {
		t.Fatalf("a tool with no schema must not be preflighted, got: %v", err)
	}
	if err := preflight(t, r, "not_indexed", `{}`); err != nil {
		t.Fatalf("an unknown tool must not be preflighted, got: %v", err)
	}
}

func TestPreflight_HasNoOpinionOnUnparseableParameters(t *testing.T) {
	r := preflightRegistry(t)

	// Malformed input is already rejected upstream; preflight must not shadow
	// that with a confusing schema complaint.
	if err := preflight(t, r, "task_update", `{"id":`); err != nil {
		t.Fatalf("malformed JSON must pass through preflight, got: %v", err)
	}
}

func TestPreflight_LargeIntegersSurvive(t *testing.T) {
	r := preflightRegistry(t)

	// Preflight reads names only. If it ever decodes values through float64
	// this call would be the one to corrupt.
	params := json.RawMessage(`{"id":"01A","version":9007199254740993,"patch":{}}`)
	if err := r.PreflightParameters("worktrack", "task_update", params); err != nil {
		t.Fatalf("valid call rejected: %v", err)
	}
}

func TestEnrichParameterError_AttachesSchemaToAServerRejection(t *testing.T) {
	r := preflightRegistry(t)

	err := r.EnrichParameterError("worktrack", "task_update",
		"invalid_request: task_update — unknown parameter(s): 'fileScope'",
		json.RawMessage(`{"id":"01A","version":3,"patch":{},"fileScope":["a.cs"]}`))

	if err == nil {
		t.Fatal("a parameter complaint must be enriched")
	}
	if !strings.Contains(err.Error(), "Expected parameters") {
		t.Fatalf("enriched error must list the valid parameters, got: %s", err.Error())
	}
}

func TestEnrichParameterError_LeavesOrdinaryFailuresAlone(t *testing.T) {
	r := preflightRegistry(t)

	err := r.EnrichParameterError("worktrack", "task_update",
		"version_conflict: task was updated by another holder",
		json.RawMessage(`{"id":"01A","version":3,"patch":{}}`))

	if err != nil {
		t.Fatalf("a non-parameter failure must pass through, got: %v", err)
	}
}
