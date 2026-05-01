package llm

import (
	"errors"
	"strings"
	"testing"
)

func TestNoBackendError_ErrorsAs(t *testing.T) {
	err := &NoBackendError{
		Failures: []ProviderFailure{
			{Name: "openai", Reason: "OPENAI_API_KEY unset"},
			{Name: "copilot", Reason: "no oauth token"},
		},
	}
	msg := err.Error()
	if !strings.Contains(msg, "openai") || !strings.Contains(msg, "OPENAI_API_KEY") {
		t.Fatalf("error must include provider details, got %q", msg)
	}

	// Should be detectable via errors.As.
	var target *NoBackendError
	if !errors.As(err, &target) {
		t.Fatal("errors.As must match NoBackendError")
	}
}

func TestNoBackendError_EmptyFailures(t *testing.T) {
	err := &NoBackendError{}
	msg := err.Error()
	if !strings.Contains(msg, "chain is empty") {
		t.Fatalf("empty failures error must mention chain is empty, got %q", msg)
	}
}
