package llm

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const defaultMaxIters = 5

// AgentDef is a named agent loaded from KDL config.
//
// Backend == "" means "walk the fall-through chain". A non-empty Backend
// names a specific provider; if that provider is unavailable at call time,
// resolution still falls through to the chain.
//
// Tools == nil means "all available tools"; empty slice also means "all"
// (the parser may emit either).
type AgentDef struct {
	Name        string
	Backend     string
	Model       string
	System      string
	Tools       []string
	Temperature *float64
	MaxTokens   *int
	MaxIters    int
}

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	agents    map[string]*AgentDef
	chain     []string
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
		agents:    make(map[string]*AgentDef),
	}
}

func (r *Registry) RegisterProvider(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

func (r *Registry) RegisterAgent(a *AgentDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a.MaxIters == 0 {
		a.MaxIters = defaultMaxIters
	}
	r.agents[a.Name] = a
}

func (r *Registry) SetChain(names []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chain = append(r.chain[:0], names...)
}

// Resolve returns the agent definition and provider to use for a call.
// If name is "", a synthetic default agent is returned.
// If the agent's Backend is set and that provider is available, it is used.
// Otherwise, the chain is walked.
func (r *Registry) Resolve(ctx context.Context, name string) (*AgentDef, Provider, error) {
	r.mu.RLock()
	def := r.lookupOrDefault(name)
	if def == nil {
		err := r.unknownAgentError(name)
		r.mu.RUnlock()
		return nil, nil, err
	}
	var backendProvider Provider
	if def.Backend != "" {
		backendProvider = r.providers[def.Backend]
	}
	chain := append([]string(nil), r.chain...)
	providerSnapshot := make(map[string]Provider, len(r.providers))
	for k, v := range r.providers {
		providerSnapshot[k] = v
	}
	r.mu.RUnlock()

	// Honour explicit backend if available — probe without lock.
	if backendProvider != nil && backendProvider.Available(ctx) {
		return def, backendProvider, nil
	}

	// Walk fall-through chain.
	if len(chain) == 0 {
		return nil, nil, &NoBackendError{}
	}
	var failures []ProviderFailure
	for _, n := range chain {
		p, ok := providerSnapshot[n]
		if !ok {
			failures = append(failures, ProviderFailure{Name: n, Reason: "not registered"})
			continue
		}
		if !p.Available(ctx) {
			reason := p.UnavailableReason()
			if reason == "" {
				reason = "unavailable"
			}
			failures = append(failures, ProviderFailure{Name: n, Reason: reason})
			continue
		}
		return def, p, nil
	}

	return nil, nil, &NoBackendError{Failures: failures}
}

func (r *Registry) lookupOrDefault(name string) *AgentDef {
	if name == "" {
		return &AgentDef{Name: "(default)", MaxIters: defaultMaxIters}
	}
	return r.agents[name]
}

func (r *Registry) unknownAgentError(name string) error {
	names := make([]string, 0, len(r.agents))
	for k := range r.agents {
		names = append(names, k)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return fmt.Errorf("unknown agent %q (no agents registered)", name)
	}
	return fmt.Errorf("unknown agent %q (registered: %s)", name, strings.Join(names, ", "))
}
