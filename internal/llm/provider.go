package llm

import "context"

// Provider is implemented by each LLM backend.
//
// Name returns the stable lowercase identifier for this backend
// (e.g. "openai", "copilot", "acp"). It must be constant for the
// lifetime of the process; the registry uses it as a map key.
//
// Available reports whether the provider can serve a request right now —
// e.g. an API key is configured, an OAuth token is fresh, a subprocess is
// alive. Resolution checks Available per call (no cross-call caching).
// When Available returns false, UnavailableReason should return a short
// human-readable explanation; the chain's NoBackendError uses these
// strings so users can tell why every backend was skipped.
//
// Sample issues a single completion request. If onEvent is non-nil, the
// provider streams partial deltas through it; providers without native
// streaming may emit a single delta carrying the full content right
// before Done. The returned Response always reflects the final state.
// After ctx is cancelled, onEvent will not be called again; in-flight
// events already dispatched are not rolled back.
type Provider interface {
	Name() string
	Available(ctx context.Context) bool
	UnavailableReason() string
	Sample(ctx context.Context, req *Request, onEvent func(StreamEvent)) (*Response, error)
}
