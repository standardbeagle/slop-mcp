package llm

import "context"

// Provider is implemented by each LLM backend.
//
// Available reports whether the provider can serve a request right now —
// e.g. an API key is configured, an OAuth token is fresh, a subprocess is
// alive. Resolution checks Available per call (no cross-call caching).
//
// Sample issues a single completion request. If onEvent is non-nil, the
// provider streams partial deltas through it; providers without native
// streaming may emit a single delta carrying the full content right
// before Done. The returned Response always reflects the final state.
type Provider interface {
	Name() string
	Available(ctx context.Context) bool
	Sample(ctx context.Context, req *Request, onEvent func(StreamEvent)) (*Response, error)
}
