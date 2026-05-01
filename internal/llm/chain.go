package llm

import (
	"fmt"
	"strings"
)

// ProviderFailure records why a single provider could not be used during chain resolution.
type ProviderFailure struct {
	Name   string
	Reason string
}

// NoBackendError is returned by Resolve when no provider in the fall-through chain
// is available. Failures contains one entry per tried provider. An empty Failures
// slice means the chain itself was empty (no providers configured).
type NoBackendError struct {
	Failures []ProviderFailure
}

func (e *NoBackendError) Error() string {
	if len(e.Failures) == 0 {
		return "no llm backend available (chain is empty)"
	}
	parts := make([]string, len(e.Failures))
	for i, f := range e.Failures {
		parts[i] = fmt.Sprintf("%s: %s", f.Name, f.Reason)
	}
	return "no llm backend available: " + strings.Join(parts, "; ")
}
