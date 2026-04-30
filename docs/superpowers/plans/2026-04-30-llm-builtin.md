# LLM Builtin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a SLOP `llm()` builtin backed by three pluggable providers (ACP, Copilot, OpenAI-compatible) with named agents in KDL config, agentic tool-use loop, and progress notifications.

**Architecture:** New `internal/llm` package with a `Provider` interface, three implementations, and a tool-use loop. KDL parser extended with `llm {}` and `agent {}` blocks. Builtin registers `llm()` + `llm_last()` in the SLOP runtime; wired into all existing registration sites (run, monitor, run_slop handler, custom_exec). Fall-through chain decides backend per call.

**Tech Stack:** Go 1.24, KDL config (existing parser), `github.com/standardbeagle/slop/pkg/slop` runtime, `github.com/modelcontextprotocol/go-sdk` for progress, stdlib `net/http` for OpenAI/Copilot, `os/exec` + JSON-RPC for ACP.

**Spec:** `docs/superpowers/specs/2026-04-30-llm-builtin-design.md`

---

## File Structure

**New:**
- `internal/llm/types.go` — `Message`, `Request`, `Response`, `ToolCall`, `ToolSpec`, `Usage`, `StreamEvent`
- `internal/llm/provider.go` — `Provider` interface
- `internal/llm/registry.go` — `Registry`, `AgentDef`, `Resolve()`
- `internal/llm/chain.go` — fall-through ordering and error aggregation
- `internal/llm/loop.go` — agentic tool-use loop
- `internal/llm/tools.go` — adapter from slop-mcp's tool surface to `[]ToolSpec` + `Execute(name, args)`
- `internal/llm/providers/openai.go` — OpenAI-compatible HTTP provider (also serves Copilot via auth strategy)
- `internal/llm/providers/copilot.go` — Copilot wrapper around openai with auth header injection + token store hookup
- `internal/llm/providers/acp.go` — ACP subprocess client
- `internal/llm/types_test.go`, `registry_test.go`, `chain_test.go`, `loop_test.go`, `tools_test.go`, `providers/*_test.go`
- `internal/builtins/llm.go` — `RegisterLLM(rt, registry, progressFn)` + builtin handlers
- `internal/builtins/llm_test.go`
- `docs/usage/llm.md` — user-facing usage doc

**Modified:**
- `internal/config/config.go` (or sibling) — KDL parser extension for `llm {}` + `agent {}` blocks
- `internal/server/handlers.go` — wire `RegisterLLM` into `run_slop` runtime construction; pass progress emitter
- `internal/server/custom_exec.go` — wire `RegisterLLM` into custom tool runtime
- `cmd/slop-mcp/run.go` — wire `RegisterLLM` (no progress)
- `cmd/slop-mcp/monitor.go` — wire `RegisterLLM` (no progress)
- `internal/builtins/slop_reference.go` — add "llm" category entries for `llm`, `llm_last`
- `go.mod` / `go.sum` — no new deps required (all stdlib)

---

## Task 1: Define core types

**Files:**
- Create: `internal/llm/types.go`
- Test: `internal/llm/types_test.go`

- [ ] **Step 1: Write the failing test**

```go
package llm

import (
	"encoding/json"
	"testing"
)

func TestMessage_JSONRoundTrip(t *testing.T) {
	m := Message{Role: RoleUser, Content: "hi"}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got Message
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Role != RoleUser || got.Content != "hi" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestStopReason_Constants(t *testing.T) {
	if StopReasonEndTurn == "" || StopReasonToolUse == "" || StopReasonMaxTokens == "" {
		t.Fatal("stop-reason constants must be non-empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/...`
Expected: build failure (`types.go` missing).

- [ ] **Step 3: Write minimal implementation**

```go
// Package llm defines the LLM provider abstraction used by the SLOP llm() builtin.
package llm

import "encoding/json"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type StopReason string

const (
	StopReasonEndTurn      StopReason = "end_turn"
	StopReasonToolUse      StopReason = "tool_use"
	StopReasonMaxTokens    StopReason = "max_tokens"
	StopReasonStopSequence StopReason = "stop_sequence"
)

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
}

type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
	IsError    bool       `json:"is_error,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Request struct {
	Messages    []Message
	System      string
	Model       string
	Tools       []ToolSpec
	Temperature *float64
	MaxTokens   *int
	Stop        []string
}

type Response struct {
	Content    string
	ToolCalls  []ToolCall
	StopReason StopReason
	Usage      Usage
	Model      string
}

type StreamEvent struct {
	Delta    string
	ToolCall *ToolCall
	Done     bool
	Final    *Response
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/llm/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/llm/types.go internal/llm/types_test.go
git commit -m "feat(llm): define core types for LLM provider abstraction"
```

---

## Task 2: Provider interface

**Files:**
- Create: `internal/llm/provider.go`

- [ ] **Step 1: Add the interface (no separate test — it's a contract; tested via implementations)**

```go
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
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/llm/...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/llm/provider.go
git commit -m "feat(llm): add Provider interface"
```

---

## Task 3: Agent definitions and registry

**Files:**
- Create: `internal/llm/registry.go`
- Test: `internal/llm/registry_test.go`

- [ ] **Step 1: Write the failing test**

```go
package llm

import (
	"context"
	"testing"
)

type fakeProvider struct {
	name      string
	available bool
}

func (f *fakeProvider) Name() string                                    { return f.name }
func (f *fakeProvider) Available(_ context.Context) bool                { return f.available }
func (f *fakeProvider) Sample(_ context.Context, _ *Request, _ func(StreamEvent)) (*Response, error) {
	return &Response{Content: "ok", StopReason: StopReasonEndTurn, Model: "fake"}, nil
}

func TestRegistry_RegisterAndResolveByAgentBackend(t *testing.T) {
	r := NewRegistry()
	r.RegisterProvider(&fakeProvider{name: "openai", available: true})
	r.SetChain([]string{"openai"})
	r.RegisterAgent(&AgentDef{Name: "summarizer", Backend: "openai", Model: "gpt-4o-mini"})

	def, prov, err := r.Resolve(context.Background(), "summarizer")
	if err != nil {
		t.Fatal(err)
	}
	if def.Name != "summarizer" {
		t.Fatalf("want agent summarizer, got %q", def.Name)
	}
	if prov.Name() != "openai" {
		t.Fatalf("want provider openai, got %q", prov.Name())
	}
}

func TestRegistry_ResolveSyntheticDefault(t *testing.T) {
	r := NewRegistry()
	r.RegisterProvider(&fakeProvider{name: "openai", available: true})
	r.SetChain([]string{"openai"})

	def, prov, err := r.Resolve(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if def.MaxIters != defaultMaxIters {
		t.Fatalf("default max_iters: want %d, got %d", defaultMaxIters, def.MaxIters)
	}
	if prov.Name() != "openai" {
		t.Fatalf("want openai, got %q", prov.Name())
	}
}

func TestRegistry_AgentNotFound(t *testing.T) {
	r := NewRegistry()
	r.RegisterAgent(&AgentDef{Name: "summarizer"})
	r.RegisterAgent(&AgentDef{Name: "reviewer"})

	_, _, err := r.Resolve(context.Background(), "summarize")
	if err == nil {
		t.Fatal("want error for unknown agent")
	}
	// Error message must list candidates
	msg := err.Error()
	if !contains(msg, "summarizer") || !contains(msg, "reviewer") {
		t.Fatalf("error must list known agents, got %q", msg)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/...`
Expected: build failure.

- [ ] **Step 3: Write the implementation**

```go
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
	if a.MaxIters == 0 {
		a.MaxIters = defaultMaxIters
	}
	r.mu.Lock()
	defer r.mu.Unlock()
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
	defer r.mu.RUnlock()

	def := r.lookupOrDefault(name)
	if def == nil {
		return nil, nil, r.unknownAgentError(name)
	}

	// Honour explicit backend if available.
	if def.Backend != "" {
		if p, ok := r.providers[def.Backend]; ok && p.Available(ctx) {
			return def, p, nil
		}
	}

	// Walk fall-through chain.
	var failures []string
	for _, n := range r.chain {
		p, ok := r.providers[n]
		if !ok {
			failures = append(failures, fmt.Sprintf("%s: not registered", n))
			continue
		}
		if !p.Available(ctx) {
			failures = append(failures, fmt.Sprintf("%s: unavailable", n))
			continue
		}
		return def, p, nil
	}

	return nil, nil, fmt.Errorf("no llm backend available: %s", strings.Join(failures, "; "))
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/llm/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/llm/registry.go internal/llm/registry_test.go
git commit -m "feat(llm): add agent registry with fall-through resolution"
```

---

## Task 4: Chain error aggregation

**Files:**
- Create: `internal/llm/chain.go`
- Test: `internal/llm/chain_test.go`

(Resolver logic already in `registry.go`; `chain.go` exposes a typed error so callers can inspect per-provider failure reasons. Useful for the builtin's error reporting.)

- [ ] **Step 1: Write the failing test**

```go
package llm

import (
	"errors"
	"testing"
)

func TestNoBackendError_Unwrap(t *testing.T) {
	err := &NoBackendError{
		Failures: []ProviderFailure{
			{Name: "openai", Reason: "OPENAI_API_KEY unset"},
			{Name: "copilot", Reason: "no oauth token"},
		},
	}
	msg := err.Error()
	if !contains(msg, "openai") || !contains(msg, "OPENAI_API_KEY") {
		t.Fatalf("error must include provider details, got %q", msg)
	}

	// Should be detectable via errors.As.
	var target *NoBackendError
	if !errors.As(err, &target) {
		t.Fatal("errors.As must match NoBackendError")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/...`
Expected: build failure.

- [ ] **Step 3: Implement**

```go
package llm

import (
	"fmt"
	"strings"
)

type ProviderFailure struct {
	Name   string
	Reason string
}

type NoBackendError struct {
	Failures []ProviderFailure
}

func (e *NoBackendError) Error() string {
	parts := make([]string, len(e.Failures))
	for i, f := range e.Failures {
		parts[i] = fmt.Sprintf("%s: %s", f.Name, f.Reason)
	}
	return "no llm backend available: " + strings.Join(parts, "; ")
}
```

- [ ] **Step 4: Refactor registry to return `*NoBackendError`**

Edit `internal/llm/registry.go`. Replace the chain-walk error construction:

```go
// Walk fall-through chain.
var failures []ProviderFailure
for _, n := range r.chain {
	p, ok := r.providers[n]
	if !ok {
		failures = append(failures, ProviderFailure{Name: n, Reason: "not registered"})
		continue
	}
	if !p.Available(ctx) {
		failures = append(failures, ProviderFailure{Name: n, Reason: "unavailable"})
		continue
	}
	return def, p, nil
}
return nil, nil, &NoBackendError{Failures: failures}
```

- [ ] **Step 5: Run all tests**

Run: `go test ./internal/llm/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/llm/chain.go internal/llm/chain_test.go internal/llm/registry.go
git commit -m "feat(llm): typed NoBackendError carrying per-provider failure reasons"
```

---

## Task 5: Tool-surface adapter

**Files:**
- Create: `internal/llm/tools.go`
- Test: `internal/llm/tools_test.go`

The loop needs to (a) build a `[]ToolSpec` from slop-mcp's available tools and (b) execute a tool by name. Both are abstracted behind an interface so tests can inject a mock and the builtin can plug in slop-mcp's real registry without `internal/llm` depending on `internal/registry` or `internal/server`.

- [ ] **Step 1: Write the failing test**

```go
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type stubTools struct {
	specs    []ToolSpec
	calls    map[string]json.RawMessage
	results  map[string]string
	errs     map[string]error
}

func (s *stubTools) List(_ context.Context, allow []string) []ToolSpec {
	if len(allow) == 0 {
		return s.specs
	}
	keep := make([]ToolSpec, 0, len(allow))
	for _, sp := range s.specs {
		for _, a := range allow {
			if sp.Name == a {
				keep = append(keep, sp)
			}
		}
	}
	return keep
}

func (s *stubTools) Execute(_ context.Context, name string, args json.RawMessage) (string, bool, error) {
	if s.calls == nil {
		s.calls = map[string]json.RawMessage{}
	}
	s.calls[name] = args
	if err, ok := s.errs[name]; ok {
		return "", true, err
	}
	if r, ok := s.results[name]; ok {
		return r, false, nil
	}
	return "", true, errors.New("tool not found")
}

func TestStubTools_FilterByWhitelist(t *testing.T) {
	s := &stubTools{specs: []ToolSpec{{Name: "a"}, {Name: "b"}, {Name: "c"}}}
	got := s.List(context.Background(), []string{"a", "c"})
	if len(got) != 2 || got[0].Name != "a" || got[1].Name != "c" {
		t.Fatalf("filter mismatch: %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/...`
Expected: build failure (`ToolHost` interface missing).

- [ ] **Step 3: Define the interface**

```go
package llm

import (
	"context"
	"encoding/json"
)

// ToolHost is the surface the loop uses to enumerate and execute tools.
// slop-mcp's server provides a real implementation that bridges to the
// MCP registry and SLOP runtime. Tests use a stub.
type ToolHost interface {
	// List returns specs visible to a given call. allow is the agent's
	// whitelist; nil/empty means "all available".
	List(ctx context.Context, allow []string) []ToolSpec

	// Execute runs a tool by name. The string is the textual result fed
	// back to the model; isError reports whether the call failed (the
	// loop will mark the tool message accordingly).
	Execute(ctx context.Context, name string, args json.RawMessage) (result string, isError bool, err error)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/llm/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/llm/tools.go internal/llm/tools_test.go
git commit -m "feat(llm): ToolHost interface for tool enumeration and execution"
```

---

## Task 6: Agentic tool-use loop

**Files:**
- Create: `internal/llm/loop.go`
- Test: `internal/llm/loop_test.go`

- [ ] **Step 1: Write the failing test (single-turn, no tools)**

```go
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// scriptedProvider returns a pre-baked response per call.
type scriptedProvider struct {
	calls    int
	answers  []*Response
	failures []error
}

func (s *scriptedProvider) Name() string                          { return "scripted" }
func (s *scriptedProvider) Available(_ context.Context) bool      { return true }
func (s *scriptedProvider) Sample(_ context.Context, _ *Request, _ func(StreamEvent)) (*Response, error) {
	i := s.calls
	s.calls++
	if i < len(s.failures) && s.failures[i] != nil {
		return nil, s.failures[i]
	}
	if i >= len(s.answers) {
		return nil, errors.New("scripted provider exhausted")
	}
	return s.answers[i], nil
}

func TestLoop_SingleTurnNoTools(t *testing.T) {
	prov := &scriptedProvider{
		answers: []*Response{
			{Content: "hello", StopReason: StopReasonEndTurn, Model: "scripted"},
		},
	}
	host := &stubTools{}
	def := &AgentDef{Name: "x", MaxIters: 3}

	out, err := Run(context.Background(), prov, host, def, &Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Final.Content != "hello" {
		t.Fatalf("want content hello, got %q", out.Final.Content)
	}
	if out.Iterations != 1 {
		t.Fatalf("want 1 iteration, got %d", out.Iterations)
	}
}

func TestLoop_ToolUseThenEndTurn(t *testing.T) {
	prov := &scriptedProvider{
		answers: []*Response{
			{
				StopReason: StopReasonToolUse,
				ToolCalls: []ToolCall{
					{ID: "t1", Name: "echo", Arguments: json.RawMessage(`{"v":"hi"}`)},
				},
				Model: "scripted",
			},
			{Content: "done", StopReason: StopReasonEndTurn, Model: "scripted"},
		},
	}
	host := &stubTools{
		specs:   []ToolSpec{{Name: "echo"}},
		results: map[string]string{"echo": "hi"},
	}
	def := &AgentDef{Name: "x", MaxIters: 5}

	out, err := Run(context.Background(), prov, host, def, &Request{
		Messages: []Message{{Role: RoleUser, Content: "say hi via echo"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Iterations != 2 {
		t.Fatalf("want 2 iterations, got %d", out.Iterations)
	}
	if out.Final.Content != "done" {
		t.Fatalf("want done, got %q", out.Final.Content)
	}
	if _, called := host.calls["echo"]; !called {
		t.Fatal("echo tool was not executed")
	}
}

func TestLoop_MaxItersExceeded(t *testing.T) {
	// Provider always asks for a tool call; loop must halt at MaxIters.
	prov := &scriptedProvider{
		answers: []*Response{
			{StopReason: StopReasonToolUse, ToolCalls: []ToolCall{{ID: "t", Name: "echo"}}, Model: "scripted"},
			{StopReason: StopReasonToolUse, ToolCalls: []ToolCall{{ID: "t", Name: "echo"}}, Model: "scripted"},
			{StopReason: StopReasonToolUse, ToolCalls: []ToolCall{{ID: "t", Name: "echo"}}, Model: "scripted"},
		},
	}
	host := &stubTools{specs: []ToolSpec{{Name: "echo"}}, results: map[string]string{"echo": "x"}}
	def := &AgentDef{Name: "x", MaxIters: 2}

	_, err := Run(context.Background(), prov, host, def, &Request{
		Messages: []Message{{Role: RoleUser, Content: "loop"}},
	}, nil)
	if err == nil {
		t.Fatal("want max-iters error")
	}
	var mie *MaxItersExceededError
	if !errors.As(err, &mie) {
		t.Fatalf("want MaxItersExceededError, got %T", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/...`
Expected: build failure (`Run` undefined).

- [ ] **Step 3: Implement the loop**

```go
package llm

import (
	"context"
	"errors"
	"fmt"
)

// LoopResult captures everything the builtin needs to surface back to
// SLOP and to populate llm_last().
type LoopResult struct {
	Final      *Response
	Iterations int
	Provider   string
	Usage      Usage
}

type MaxItersExceededError struct {
	Iters int
}

func (e *MaxItersExceededError) Error() string {
	return fmt.Sprintf("max_iters exceeded (%d)", e.Iters)
}

// Run executes the agentic tool-use loop against a single provider.
//
// Caller responsibilities:
//   - Resolve the provider via Registry first.
//   - Pre-populate req.Messages with the user input. The loop appends
//     assistant + tool messages as it iterates.
//   - Build req.Tools by calling host.List(ctx, def.Tools) — Run does
//     this internally so callers stay simple.
func Run(
	ctx context.Context,
	prov Provider,
	host ToolHost,
	def *AgentDef,
	req *Request,
	onEvent func(StreamEvent),
) (*LoopResult, error) {
	if def.MaxIters <= 0 {
		def.MaxIters = defaultMaxIters
	}

	req.Tools = host.List(ctx, def.Tools)

	out := &LoopResult{Provider: prov.Name()}

	for i := 0; i < def.MaxIters; i++ {
		resp, err := prov.Sample(ctx, req, onEvent)
		if err != nil {
			return out, err
		}
		out.Iterations++
		out.Final = resp
		out.Usage.InputTokens += resp.Usage.InputTokens
		out.Usage.OutputTokens += resp.Usage.OutputTokens

		if resp.StopReason != StopReasonToolUse {
			return out, nil
		}

		// Append assistant turn carrying the tool calls.
		req.Messages = append(req.Messages, Message{
			Role:      RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute each tool call sequentially, append results.
		for _, tc := range resp.ToolCalls {
			result, isErr, execErr := host.Execute(ctx, tc.Name, tc.Arguments)
			if execErr != nil && !isErr {
				// Hard failure (tool not found, infrastructure error).
				return out, fmt.Errorf("tool %q execute: %w", tc.Name, execErr)
			}
			content := result
			if execErr != nil {
				content = execErr.Error()
			}
			req.Messages = append(req.Messages, Message{
				Role:       RoleTool,
				Name:       tc.Name,
				ToolCallID: tc.ID,
				Content:    content,
				IsError:    isErr,
			})
		}
	}

	return out, &MaxItersExceededError{Iters: def.MaxIters}
}

// errorsIs is a tiny shim so this file does not need to import errors
// just for tests; kept unexported.
var _ = errors.Is
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/llm/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/llm/loop.go internal/llm/loop_test.go
git commit -m "feat(llm): tool-use loop with max_iters and per-iteration accounting"
```

---

## Task 7: OpenAI-compatible provider (no streaming yet)

**Files:**
- Create: `internal/llm/providers/openai.go`
- Test: `internal/llm/providers/openai_test.go`

- [ ] **Step 1: Write the failing test (HTTP fake)**

```go
package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

func TestOpenAI_SimpleCompletion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer test-key") {
			t.Errorf("missing auth header: %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"hello"`) {
			t.Errorf("body missing prompt: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {"role": "assistant", "content": "world"},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 5, "completion_tokens": 1},
			"model": "gpt-test"
		}`))
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{
		BaseURL:      srv.URL,
		APIKey:       "test-key",
		DefaultModel: "gpt-test",
	})

	resp, err := p.Sample(context.Background(), &llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "world" {
		t.Fatalf("want world, got %q", resp.Content)
	}
	if resp.StopReason != llm.StopReasonEndTurn {
		t.Fatalf("want end_turn, got %q", resp.StopReason)
	}
}

func TestOpenAI_ToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": null,
					"tool_calls": [{
						"id": "call_1",
						"type": "function",
						"function": {"name": "echo", "arguments": "{\"v\":\"hi\"}"}
					}]
				},
				"finish_reason": "tool_calls"
			}],
			"model": "gpt-test"
		}`))
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL, APIKey: "k", DefaultModel: "gpt-test"})
	resp, err := p.Sample(context.Background(), &llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "use echo"}},
		Tools:    []llm.ToolSpec{{Name: "echo", Schema: json.RawMessage(`{"type":"object"}`)}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != llm.StopReasonToolUse {
		t.Fatalf("want tool_use, got %q", resp.StopReason)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "echo" {
		t.Fatalf("unexpected tool calls: %+v", resp.ToolCalls)
	}
}

func TestOpenAI_AvailableRequiresKey(t *testing.T) {
	noKey := NewOpenAI(OpenAIConfig{BaseURL: "http://x", DefaultModel: "m"})
	if noKey.Available(context.Background()) {
		t.Fatal("provider must report unavailable when API key empty")
	}
	withKey := NewOpenAI(OpenAIConfig{BaseURL: "http://x", APIKey: "k", DefaultModel: "m"})
	if !withKey.Available(context.Background()) {
		t.Fatal("provider must report available with key set")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/providers/...`
Expected: build failure.

- [ ] **Step 3: Implement (blocking-only first; streaming added in Task 13)**

```go
// Package providers contains concrete LLM provider implementations.
package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

type OpenAIConfig struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
	Name         string             // override; default "openai"
	HTTPClient   *http.Client       // optional; default http.DefaultClient
	AuthHeader   func(*http.Request) // optional; default Bearer APIKey
}

type OpenAI struct {
	cfg OpenAIConfig
}

func NewOpenAI(cfg OpenAIConfig) *OpenAI {
	if cfg.Name == "" {
		cfg.Name = "openai"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.AuthHeader == nil {
		cfg.AuthHeader = func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		}
	}
	return &OpenAI{cfg: cfg}
}

func (o *OpenAI) Name() string { return o.cfg.Name }

func (o *OpenAI) Available(_ context.Context) bool {
	return o.cfg.APIKey != ""
}

type chatReq struct {
	Model       string         `json:"model"`
	Messages    []chatMsg      `json:"messages"`
	Tools       []chatTool     `json:"tools,omitempty"`
	Temperature *float64       `json:"temperature,omitempty"`
	MaxTokens   *int           `json:"max_tokens,omitempty"`
	Stop        []string       `json:"stop,omitempty"`
}

type chatMsg struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

type chatTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Role      string         `json:"role"`
			Content   *string        `json:"content"`
			ToolCalls []chatToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

func (o *OpenAI) Sample(ctx context.Context, req *llm.Request, _ func(llm.StreamEvent)) (*llm.Response, error) {
	model := req.Model
	if model == "" {
		model = o.cfg.DefaultModel
	}

	body, err := json.Marshal(buildChatReq(model, req))
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	url := strings.TrimRight(o.cfg.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	o.cfg.AuthHeader(httpReq)

	httpResp, err := o.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", o.cfg.Name, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode/100 != 2 {
		buf, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("%s: http %d: %s", o.cfg.Name, httpResp.StatusCode, string(buf))
	}

	var raw chatResp
	if err := json.NewDecoder(httpResp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(raw.Choices) == 0 {
		return nil, fmt.Errorf("%s: empty choices", o.cfg.Name)
	}

	c := raw.Choices[0]
	out := &llm.Response{
		Model: raw.Model,
		Usage: llm.Usage{InputTokens: raw.Usage.PromptTokens, OutputTokens: raw.Usage.CompletionTokens},
	}
	if c.Message.Content != nil {
		out.Content = *c.Message.Content
	}
	for _, tc := range c.Message.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		})
	}
	switch c.FinishReason {
	case "tool_calls":
		out.StopReason = llm.StopReasonToolUse
	case "length":
		out.StopReason = llm.StopReasonMaxTokens
	case "stop":
		out.StopReason = llm.StopReasonEndTurn
	default:
		out.StopReason = llm.StopReason(c.FinishReason)
	}
	return out, nil
}

func buildChatReq(model string, in *llm.Request) chatReq {
	msgs := make([]chatMsg, 0, len(in.Messages)+1)
	if in.System != "" {
		msgs = append(msgs, chatMsg{Role: "system", Content: in.System})
	}
	for _, m := range in.Messages {
		out := chatMsg{Role: string(m.Role), Content: m.Content, ToolCallID: m.ToolCallID, Name: m.Name}
		for _, tc := range m.ToolCalls {
			ctc := chatToolCall{ID: tc.ID, Type: "function"}
			ctc.Function.Name = tc.Name
			ctc.Function.Arguments = string(tc.Arguments)
			out.ToolCalls = append(out.ToolCalls, ctc)
		}
		msgs = append(msgs, out)
	}
	tools := make([]chatTool, 0, len(in.Tools))
	for _, t := range in.Tools {
		ct := chatTool{Type: "function"}
		ct.Function.Name = t.Name
		ct.Function.Description = t.Description
		ct.Function.Parameters = t.Schema
		tools = append(tools, ct)
	}
	return chatReq{
		Model: model, Messages: msgs, Tools: tools,
		Temperature: in.Temperature, MaxTokens: in.MaxTokens, Stop: in.Stop,
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/llm/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/llm/providers/openai.go internal/llm/providers/openai_test.go
git commit -m "feat(llm): OpenAI-compatible provider (blocking)"
```

---

## Task 8: KDL config extension

**Files:**
- Modify: `internal/config/config.go` (or wherever the existing KDL parser lives — locate first)
- Test: extend the existing config test file with new cases

**Pre-step: Locate the parser.** Run `grep -rn "kdl\\|kdlfile" internal/config/`. Use whichever file currently parses `mcp` and `customization` blocks. Add `llm` and `agent` parsing alongside them, following the existing style.

The KDL data shape (must match the spec):

```kdl
llm {
    chain "acp" "copilot" "openai"

    provider "openai" {
        base_url "https://api.openai.com/v1"
        api_key_env "OPENAI_API_KEY"
        api_key "literal-key"   // optional alternative
        default_model "gpt-4o-mini"
    }
    provider "copilot" {
        default_model "gpt-4o"
    }
    provider "acp" {
        command "zed-agent" "--stdio"
    }

    tool_timeout 60
}

agent "reviewer" {
    backend "copilot"
    model "gpt-4o"
    system "You are a strict code reviewer..."
    tools "search_tools" "execute_tool"
    temperature 0.2
    max_iters 5
}
```

- [ ] **Step 1: Write the failing test**

Add to existing config test file:

```go
func TestParse_LLMAndAgents(t *testing.T) {
	src := `
llm {
    chain "acp" "openai"
    provider "openai" {
        base_url "https://api.example.com/v1"
        api_key_env "FOO_KEY"
        default_model "m1"
    }
    tool_timeout 30
}
agent "summarizer" {
    backend "openai"
    model "m1"
    system "Be brief."
    max_iters 1
}
`
	cfg, err := Parse([]byte(src))   // adapt to existing API
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM == nil {
		t.Fatal("expected LLM section")
	}
	if got := cfg.LLM.Chain; len(got) != 2 || got[0] != "acp" || got[1] != "openai" {
		t.Fatalf("chain mismatch: %+v", got)
	}
	openai := cfg.LLM.Providers["openai"]
	if openai.BaseURL != "https://api.example.com/v1" || openai.APIKeyEnv != "FOO_KEY" || openai.DefaultModel != "m1" {
		t.Fatalf("openai provider mismatch: %+v", openai)
	}
	if cfg.LLM.ToolTimeoutSec != 30 {
		t.Fatalf("tool_timeout: want 30, got %d", cfg.LLM.ToolTimeoutSec)
	}
	a := cfg.Agents["summarizer"]
	if a == nil || a.Backend != "openai" || a.Model != "m1" || a.System != "Be brief." || a.MaxIters != 1 {
		t.Fatalf("agent mismatch: %+v", a)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/...`
Expected: build/parse failure.

- [ ] **Step 3: Add types**

In `internal/config/config.go` (or sibling), add:

```go
type LLMConfig struct {
	Chain          []string
	Providers      map[string]*LLMProviderConfig
	ToolTimeoutSec int
}

type LLMProviderConfig struct {
	BaseURL          string
	APIKey           string   // literal; rare
	APIKeyEnv        string   // env var name; preferred
	DefaultModel     string
	Command          []string // ACP only: subprocess argv
	URL              string   // ACP only: socket URL alternative
}

type AgentConfig struct {
	Name        string
	Backend     string
	Model       string
	System      string
	Tools       []string
	Temperature *float64
	MaxTokens   *int
	MaxIters    int
}

// Extend the top-level Config struct:
//   LLM    *LLMConfig
//   Agents map[string]*AgentConfig
```

- [ ] **Step 4: Implement parse paths**

Extend the existing KDL block-parsing switch to handle `llm` and `agent`. Use the same helpers (`children()`, `arg(...)`, etc.) the file already uses for `mcp` and `customization`. Reject unknown keys inside the blocks with an error (consistent with existing strictness).

Three-tier merge: in the existing merge code that handles `MCPs`, add merging for `Agents` (later layer fully replaces by name) and `LLM` (later layer overrides whole struct if present, or merges providers map with name-replacement).

- [ ] **Step 5: Run tests**

Run: `go test ./internal/config/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/
git commit -m "feat(config): parse llm{} and agent{} blocks from KDL"
```

---

## Task 9: Build registry from config

**Files:**
- Create: `internal/llm/build.go`
- Test: `internal/llm/build_test.go`

A factory that takes parsed config + dependencies (HTTP client, OAuth token store) and returns a fully wired `*Registry`. Keeps the wire-up logic in one testable place instead of scattering it across `serve` / `run` / `monitor`.

- [ ] **Step 1: Write the failing test**

```go
package llm

import (
	"context"
	"os"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/config"
)

func TestBuild_RegistersProvidersAndAgents(t *testing.T) {
	t.Setenv("FOO_KEY", "test-key")
	cfg := &config.Config{
		LLM: &config.LLMConfig{
			Chain: []string{"openai"},
			Providers: map[string]*config.LLMProviderConfig{
				"openai": {BaseURL: "http://localhost:9999", APIKeyEnv: "FOO_KEY", DefaultModel: "m1"},
			},
		},
		Agents: map[string]*config.AgentConfig{
			"summarizer": {Name: "summarizer", Backend: "openai", Model: "m1", MaxIters: 1},
		},
	}

	r, err := Build(cfg, BuildDeps{})
	if err != nil {
		t.Fatal(err)
	}
	def, prov, err := r.Resolve(context.Background(), "summarizer")
	if err != nil {
		t.Fatal(err)
	}
	if def.MaxIters != 1 {
		t.Fatalf("want max_iters 1, got %d", def.MaxIters)
	}
	if prov.Name() != "openai" {
		t.Fatalf("want openai, got %q", prov.Name())
	}

	// Provider should not be available if env var unset (re-build with cleared env).
	os.Unsetenv("FOO_KEY")
	r2, err := Build(cfg, BuildDeps{})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r2.Resolve(context.Background(), "summarizer")
	if err == nil {
		t.Fatal("expected NoBackendError when key env unset")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/...`
Expected: build failure.

- [ ] **Step 3: Implement**

```go
package llm

import (
	"net/http"
	"os"

	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/llm/providers"
)

// BuildDeps lets callers inject collaborators that the registry itself
// should not own — primarily for testing or for sharing OAuth state with
// auth_mcp.
type BuildDeps struct {
	HTTPClient        *http.Client
	CopilotTokenStore CopilotTokenStore // optional; nil disables copilot
}

// CopilotTokenStore is the minimal hook the copilot provider needs.
// auth_mcp implements this in the real wire-up.
type CopilotTokenStore interface {
	Token() (string, error)
}

func Build(cfg *config.Config, deps BuildDeps) (*Registry, error) {
	r := NewRegistry()
	if cfg.LLM != nil {
		r.SetChain(cfg.LLM.Chain)
		for name, pc := range cfg.LLM.Providers {
			p, err := buildProvider(name, pc, deps)
			if err != nil {
				return nil, err
			}
			if p != nil {
				r.RegisterProvider(p)
			}
		}
	}
	for name, ac := range cfg.Agents {
		r.RegisterAgent(&AgentDef{
			Name:        name,
			Backend:     ac.Backend,
			Model:       ac.Model,
			System:      ac.System,
			Tools:       ac.Tools,
			Temperature: ac.Temperature,
			MaxTokens:   ac.MaxTokens,
			MaxIters:    ac.MaxIters,
		})
	}
	return r, nil
}

func buildProvider(name string, pc *config.LLMProviderConfig, deps BuildDeps) (Provider, error) {
	switch name {
	case "openai":
		return providers.NewOpenAI(providers.OpenAIConfig{
			BaseURL:      pc.BaseURL,
			APIKey:       resolveKey(pc),
			DefaultModel: pc.DefaultModel,
			HTTPClient:   deps.HTTPClient,
		}), nil
	case "copilot":
		if deps.CopilotTokenStore == nil {
			return nil, nil // skip silently when not wired
		}
		return providers.NewCopilot(providers.CopilotConfig{
			DefaultModel: pc.DefaultModel,
			TokenStore:   deps.CopilotTokenStore,
			HTTPClient:   deps.HTTPClient,
		}), nil
	case "acp":
		return providers.NewACP(providers.ACPConfig{
			Command: pc.Command,
			URL:     pc.URL,
		}), nil
	default:
		return nil, nil // unknown provider name — caller may add later
	}
}

func resolveKey(pc *config.LLMProviderConfig) string {
	if pc.APIKey != "" {
		return pc.APIKey
	}
	if pc.APIKeyEnv != "" {
		return os.Getenv(pc.APIKeyEnv)
	}
	return ""
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/llm/...`
Expected: PASS (NewCopilot/NewACP must at least exist as stubs — see Task 10/11. Implement minimal stubs in providers/copilot.go and providers/acp.go now: structs that return Available()=false and Sample() returning ErrNotImplemented).

- [ ] **Step 5: Commit**

```bash
git add internal/llm/build.go internal/llm/build_test.go internal/llm/providers/copilot.go internal/llm/providers/acp.go
git commit -m "feat(llm): Build() factory wiring config to registry"
```

---

## Task 10: Copilot provider

**Files:**
- Modify: `internal/llm/providers/copilot.go` (replace stub)
- Test: `internal/llm/providers/copilot_test.go`

Copilot exposes an OpenAI-compatible chat endpoint at `https://api.githubcopilot.com/chat/completions` and uses an OAuth token retrieved through the existing `auth_mcp` flow. The provider reuses the OpenAI implementation with two differences:
1. The auth header injection swaps the OAuth token in per request (it can refresh).
2. Required header `Editor-Version` and `Copilot-Integration-Id` per Copilot's API contract — confirm exact values at impl time; sketch below uses common defaults.

- [ ] **Step 1: Write the failing test**

```go
package providers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

type fakeTokenStore struct {
	tok string
	err error
}

func (f *fakeTokenStore) Token() (string, error) { return f.tok, f.err }

func TestCopilot_InjectsOAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer copilot-oauth-tok") {
			t.Errorf("auth header: %q", got)
		}
		if r.Header.Get("Editor-Version") == "" {
			t.Error("missing Editor-Version header")
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	p := NewCopilot(CopilotConfig{
		BaseURL:      srv.URL,
		DefaultModel: "gpt-4o",
		TokenStore:   &fakeTokenStore{tok: "copilot-oauth-tok"},
	})
	resp, err := p.Sample(context.Background(), &llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "ok" {
		t.Fatalf("want ok, got %q", resp.Content)
	}
}

func TestCopilot_AvailableRequiresToken(t *testing.T) {
	p := NewCopilot(CopilotConfig{TokenStore: &fakeTokenStore{tok: ""}})
	if p.Available(context.Background()) {
		t.Fatal("must be unavailable when token empty")
	}
	p2 := NewCopilot(CopilotConfig{TokenStore: &fakeTokenStore{tok: "x"}})
	if !p2.Available(context.Background()) {
		t.Fatal("must be available with token")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/providers/...`
Expected: failure (stub still in place from Task 9).

- [ ] **Step 3: Implement**

```go
package providers

import (
	"context"
	"net/http"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

type CopilotConfig struct {
	BaseURL      string                              // default https://api.githubcopilot.com
	DefaultModel string
	TokenStore   interface{ Token() (string, error) }
	HTTPClient   *http.Client
	EditorVersion string                              // default "slop-mcp/<version>"
}

type Copilot struct {
	inner *OpenAI
	store interface{ Token() (string, error) }
}

func NewCopilot(cfg CopilotConfig) *Copilot {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.githubcopilot.com"
	}
	if cfg.EditorVersion == "" {
		cfg.EditorVersion = "slop-mcp/0"
	}
	c := &Copilot{store: cfg.TokenStore}
	inner := NewOpenAI(OpenAIConfig{
		Name:         "copilot",
		BaseURL:      cfg.BaseURL,
		APIKey:       "placeholder", // overridden via AuthHeader closure
		DefaultModel: cfg.DefaultModel,
		HTTPClient:   cfg.HTTPClient,
		AuthHeader: func(r *http.Request) {
			tok, _ := cfg.TokenStore.Token()
			if tok != "" {
				r.Header.Set("Authorization", "Bearer "+tok)
			}
			r.Header.Set("Editor-Version", cfg.EditorVersion)
			r.Header.Set("Copilot-Integration-Id", "vscode-chat")
		},
	})
	c.inner = inner
	return c
}

func (c *Copilot) Name() string { return "copilot" }

func (c *Copilot) Available(_ context.Context) bool {
	if c.store == nil {
		return false
	}
	tok, err := c.store.Token()
	return err == nil && tok != ""
}

func (c *Copilot) Sample(ctx context.Context, req *llm.Request, on func(llm.StreamEvent)) (*llm.Response, error) {
	return c.inner.Sample(ctx, req, on)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/llm/providers/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/llm/providers/copilot.go internal/llm/providers/copilot_test.go
git commit -m "feat(llm): Copilot provider via OpenAI-compatible endpoint"
```

---

## Task 11: ACP provider (subprocess JSON-RPC)

**Files:**
- Modify: `internal/llm/providers/acp.go` (replace stub)
- Test: `internal/llm/providers/acp_test.go`

ACP minimal v1 surface: `initialize`, `prompt`, `cancel`. We start a subprocess on first use, hold one persistent connection, and serialize requests behind a mutex (no concurrent prompts in v1).

**Caveat:** the ACP spec evolves. Re-read https://agentclientprotocol.com/protocol when implementing and adjust message field names / capabilities as needed. The structure below is the shape, not a final wire format.

- [ ] **Step 1: Write the failing test (uses pipe-based fake subprocess)**

```go
package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

// fakeACPProc implements the ACPTransport interface so we can test without
// a real subprocess.
type fakeACPProc struct {
	in   io.WriteCloser  // we write here (server replies)
	out  io.ReadCloser   // we read here (client requests)
	resp string
}

func newFakeACP(reply string) (*fakeACPProc, *fakeACPProc) {
	cr, sw := io.Pipe()  // client reads, server writes
	sr, cw := io.Pipe()  // server reads, client writes
	client := &fakeACPProc{in: cw, out: cr}
	server := &fakeACPProc{in: sw, out: sr, resp: reply}
	return client, server
}

func TestACP_InitializeAndPrompt(t *testing.T) {
	t.Skip("structure-only test; full ACP integration covered by integration-test target")
}

func TestACP_AvailableFalseUntilConnected(t *testing.T) {
	p := NewACP(ACPConfig{Command: []string{"/nonexistent/binary"}})
	if p.Available(context.Background()) {
		t.Fatal("must be unavailable when subprocess not spawned")
	}
}

// JSON-RPC framing test — covers our line-delimited reader/writer.
func TestACP_FrameRoundTrip(t *testing.T) {
	r, w := io.Pipe()
	go func() {
		_ = writeFrame(w, map[string]any{"hello": "world"})
		_ = w.Close()
	}()
	got, err := readFrame(bufio.NewReader(r))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if m["hello"] != "world" {
		t.Fatalf("frame mismatch: %v", m)
	}
}

var _ = strings.Join
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/providers/...`
Expected: failure.

- [ ] **Step 3: Implement**

```go
package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

type ACPConfig struct {
	Command []string // argv for stdio transport; first element is program
	URL     string   // alternative socket transport (not implemented v1)
}

type ACP struct {
	cfg     ACPConfig
	mu      sync.Mutex
	proc    *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	nextID  atomic.Int64
	connected atomic.Bool
}

func NewACP(cfg ACPConfig) *ACP { return &ACP{cfg: cfg} }

func (a *ACP) Name() string { return "acp" }

func (a *ACP) Available(_ context.Context) bool {
	if a.connected.Load() {
		return true
	}
	// Lazy probe: try to spawn. If it fails, report unavailable.
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.proc != nil {
		return true
	}
	if len(a.cfg.Command) == 0 {
		return false
	}
	if err := a.spawnLocked(); err != nil {
		return false
	}
	return true
}

func (a *ACP) spawnLocked() error {
	cmd := exec.Command(a.cfg.Command[0], a.cfg.Command[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	a.proc = cmd
	a.stdin = stdin
	a.stdout = bufio.NewReader(stdout)
	a.connected.Store(true)
	return nil
}

func (a *ACP) Sample(ctx context.Context, req *llm.Request, on func(llm.StreamEvent)) (*llm.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.proc == nil {
		if err := a.spawnLocked(); err != nil {
			return nil, fmt.Errorf("acp spawn: %w", err)
		}
	}

	id := a.nextID.Add(1)
	rpc := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "prompt",
		"params": map[string]any{
			"messages": req.Messages,
			"system":   req.System,
			"model":    req.Model,
			"tools":    req.Tools,
		},
	}
	if err := writeFrame(a.stdin, rpc); err != nil {
		return nil, fmt.Errorf("acp write: %w", err)
	}

	frame, err := readFrame(a.stdout)
	if err != nil {
		return nil, fmt.Errorf("acp read: %w", err)
	}

	var raw struct {
		Result *struct {
			Content    string         `json:"content"`
			ToolCalls  []llm.ToolCall `json:"tool_calls"`
			StopReason string         `json:"stop_reason"`
			Model      string         `json:"model"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(frame, &raw); err != nil {
		return nil, fmt.Errorf("acp decode: %w", err)
	}
	if raw.Error != nil {
		return nil, fmt.Errorf("acp error %d: %s", raw.Error.Code, raw.Error.Message)
	}
	if raw.Result == nil {
		return nil, errors.New("acp response missing result")
	}
	return &llm.Response{
		Content:    raw.Result.Content,
		ToolCalls:  raw.Result.ToolCalls,
		StopReason: llm.StopReason(raw.Result.StopReason),
		Model:      raw.Result.Model,
	}, nil
}

// writeFrame emits a single JSON object followed by a newline.
func writeFrame(w io.Writer, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// readFrame reads one newline-terminated JSON message.
func readFrame(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	return line, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/llm/providers/...`
Expected: PASS (the meaningful one is the framing test; full ACP wire test deferred to integration).

- [ ] **Step 5: Commit**

```bash
git add internal/llm/providers/acp.go internal/llm/providers/acp_test.go
git commit -m "feat(llm): ACP subprocess provider with JSON-RPC framing"
```

---

## Task 12: SLOP llm() builtin + llm_last() helper

**Files:**
- Create: `internal/builtins/llm.go`
- Test: `internal/builtins/llm_test.go`

- [ ] **Step 1: Write the failing test**

```go
package builtins

import (
	"context"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/llm"
	"github.com/standardbeagle/slop/pkg/slop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProvider struct{ name string }

func (f *fakeProvider) Name() string                                 { return f.name }
func (f *fakeProvider) Available(_ context.Context) bool             { return true }
func (f *fakeProvider) Sample(_ context.Context, _ *llm.Request, _ func(llm.StreamEvent)) (*llm.Response, error) {
	return &llm.Response{Content: "answer", StopReason: llm.StopReasonEndTurn, Model: "fake"}, nil
}

type emptyHost struct{}

func (emptyHost) List(_ context.Context, _ []string) []llm.ToolSpec { return nil }
func (emptyHost) Execute(_ context.Context, _ string, _ []byte) (string, bool, error) {
	return "", true, assert.AnError
}

func newLLMRuntime(t *testing.T) *slop.Runtime {
	t.Helper()
	rt := slop.NewRuntime()
	r := llm.NewRegistry()
	r.RegisterProvider(&fakeProvider{name: "fake"})
	r.SetChain([]string{"fake"})
	r.RegisterAgent(&llm.AgentDef{Name: "test", Backend: "fake"})
	RegisterLLM(rt, r, emptyHost{}, nil)
	return rt
}

func TestLLM_BasicCall(t *testing.T) {
	rt := newLLMRuntime(t)
	defer rt.Close()
	v, err := rt.Execute(`llm("hi", agent: "test")`)
	require.NoError(t, err)
	assert.Equal(t, "answer", v.(*slop.StringValue).Value)
}

func TestLLM_LastReturnsMetadata(t *testing.T) {
	rt := newLLMRuntime(t)
	defer rt.Close()
	_, err := rt.Execute(`llm("hi", agent: "test")`)
	require.NoError(t, err)
	v, err := rt.Execute(`llm_last()`)
	require.NoError(t, err)
	m := v.(*slop.MapValue)
	model, _ := m.Get("model")
	assert.Equal(t, "fake", model.(*slop.StringValue).Value)
	prov, _ := m.Get("provider")
	assert.Equal(t, "fake", prov.(*slop.StringValue).Value)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/builtins/...`
Expected: build failure.

- [ ] **Step 3: Implement**

```go
package builtins

import (
	"context"
	"fmt"

	"github.com/standardbeagle/slop-mcp/internal/llm"
	"github.com/standardbeagle/slop/pkg/slop"
)

// ProgressFn receives streaming events. nil means no streaming surface.
type ProgressFn func(llm.StreamEvent)

// RegisterLLM wires the llm() and llm_last() builtins into the runtime.
// host provides tool enumeration/execution. progressFn is optional;
// when non-nil, the loop forwards stream events for the caller to relay
// as MCP progress notifications (or stderr in CLI mode).
func RegisterLLM(rt *slop.Runtime, r *llm.Registry, host llm.ToolHost, progressFn ProgressFn) {
	state := &llmState{}
	rt.RegisterBuiltin("llm", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		return state.callLLM(rt.Context(), r, host, progressFn, args, kwargs)
	})
	rt.RegisterBuiltin("llm_last", func(_ []slop.Value, _ map[string]slop.Value) (slop.Value, error) {
		return state.lastValue(), nil
	})
}

type llmState struct {
	last *llm.LoopResult
}

func (s *llmState) callLLM(
	ctx context.Context,
	r *llm.Registry,
	host llm.ToolHost,
	progressFn ProgressFn,
	args []slop.Value,
	kwargs map[string]slop.Value,
) (slop.Value, error) {
	prompt := ""
	if len(args) > 0 {
		if sv, ok := args[0].(*slop.StringValue); ok {
			prompt = sv.Value
		}
	}

	agentName := stringKwarg(kwargs, "agent")
	def, prov, err := r.Resolve(ctx, agentName)
	if err != nil {
		return slop.NewErrorValue(err.Error()), nil
	}

	// Per-call kwargs override agent fields.
	system := def.System
	if v := stringKwarg(kwargs, "system"); v != "" {
		system = v
	}
	model := def.Model
	if v := stringKwarg(kwargs, "model"); v != "" {
		model = v
	}
	maxIters := def.MaxIters
	if v, ok := intKwarg(kwargs, "max_iters"); ok {
		maxIters = v
	}

	req := &llm.Request{
		System:    system,
		Model:     model,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: prompt}},
	}
	if msgs, ok := kwargs["messages"]; ok {
		converted, err := messagesFromSLOP(msgs)
		if err != nil {
			return nil, err
		}
		req.Messages = converted
	}
	if v, ok := kwargs["tools"]; ok {
		def.Tools = stringListFromSLOP(v)
	}

	defOverride := *def
	defOverride.MaxIters = maxIters
	res, err := llm.Run(ctx, prov, host, &defOverride, req, progressFn)
	if err != nil {
		s.last = res
		return slop.NewErrorValue(err.Error()), nil
	}
	s.last = res
	return slop.NewStringValue(res.Final.Content), nil
}

func (s *llmState) lastValue() slop.Value {
	if s.last == nil {
		return slop.NewNullValue()
	}
	m := slop.NewMapValue()
	m.Set("model", slop.NewStringValue(s.last.Final.Model))
	m.Set("provider", slop.NewStringValue(s.last.Provider))
	m.Set("iterations", slop.NewIntValue(int64(s.last.Iterations)))
	m.Set("stop_reason", slop.NewStringValue(string(s.last.Final.StopReason)))
	usage := slop.NewMapValue()
	usage.Set("input_tokens", slop.NewIntValue(int64(s.last.Usage.InputTokens)))
	usage.Set("output_tokens", slop.NewIntValue(int64(s.last.Usage.OutputTokens)))
	m.Set("usage", usage)
	return m
}

// Helpers — stringKwarg, intKwarg, messagesFromSLOP, stringListFromSLOP —
// follow the same patterns used in memory.go and template.go. Implement
// them in this file for cohesion.

func stringKwarg(kw map[string]slop.Value, key string) string {
	v, ok := kw[key]
	if !ok {
		return ""
	}
	sv, ok := v.(*slop.StringValue)
	if !ok {
		return ""
	}
	return sv.Value
}

func intKwarg(kw map[string]slop.Value, key string) (int, bool) {
	v, ok := kw[key]
	if !ok {
		return 0, false
	}
	if iv, ok := v.(*slop.IntValue); ok {
		return int(iv.Value), true
	}
	if nv, ok := v.(*slop.NumberValue); ok {
		return int(nv.Value), true
	}
	return 0, false
}

func messagesFromSLOP(v slop.Value) ([]llm.Message, error) {
	lv, ok := v.(*slop.ListValue)
	if !ok {
		return nil, fmt.Errorf("messages: expected list")
	}
	out := make([]llm.Message, 0, len(lv.Elements))
	for i, el := range lv.Elements {
		mv, ok := el.(*slop.MapValue)
		if !ok {
			return nil, fmt.Errorf("messages[%d]: expected map", i)
		}
		var m llm.Message
		if r, ok := mv.Get("role"); ok {
			if s, ok := r.(*slop.StringValue); ok {
				m.Role = llm.Role(s.Value)
			}
		}
		if c, ok := mv.Get("content"); ok {
			if s, ok := c.(*slop.StringValue); ok {
				m.Content = s.Value
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func stringListFromSLOP(v slop.Value) []string {
	lv, ok := v.(*slop.ListValue)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(lv.Elements))
	for _, el := range lv.Elements {
		if s, ok := el.(*slop.StringValue); ok {
			out = append(out, s.Value)
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/builtins/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/builtins/llm.go internal/builtins/llm_test.go
git commit -m "feat(builtins): llm() and llm_last() SLOP builtins"
```

---

## Task 13: ToolHost adapter from slop-mcp registry

**Files:**
- Create: `internal/server/llm_host.go`
- Test: `internal/server/llm_host_test.go`

The builtin gets a `llm.ToolHost`. slop-mcp has a registry of MCP tools. This task wires them: List enumerates registered MCP tools (via `s.registry.ListTools()`) plus a handful of always-allowed slop builtins; Execute routes by prefix.

- [ ] **Step 1: Inspect existing registry surface**

Run: `grep -n "func.*Registry.*Tools\\|ListTools\\|GetMetadata" internal/registry/*.go`. Identify the call that returns currently-available tools with name + description + schema.

- [ ] **Step 2: Write the failing test**

```go
package server

import (
	"context"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

func TestLLMHost_ListReturnsMCPTools(t *testing.T) {
	s := newTestServer(t)  // existing helper if present; otherwise minimal stub
	host := &LLMHost{server: s}

	specs := host.List(context.Background(), nil)
	// We only assert the host returns a non-nil slice with expected shape.
	for _, sp := range specs {
		if sp.Name == "" {
			t.Fatalf("spec missing name: %+v", sp)
		}
		if len(sp.Schema) == 0 {
			t.Fatalf("spec %q missing schema", sp.Name)
		}
	}
	_ = llm.ToolSpec{} // ensure import
}
```

If `newTestServer` doesn't exist, write the bare minimum: register a mock MCP via `registry.AddToolsForTesting` (referenced in CLAUDE.md) to populate the registry, then construct `LLMHost`.

- [ ] **Step 3: Implement**

```go
package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

// LLMHost adapts the slop-mcp registry + execute path to llm.ToolHost.
type LLMHost struct {
	server *Server
}

func NewLLMHost(s *Server) *LLMHost { return &LLMHost{server: s} }

func (h *LLMHost) List(ctx context.Context, allow []string) []llm.ToolSpec {
	all := h.server.registry.ListTools(ctx) // adapt to actual API name
	out := make([]llm.ToolSpec, 0, len(all))
	allowSet := toSet(allow)
	for _, t := range all {
		if len(allowSet) > 0 {
			if _, ok := allowSet[t.QualifiedName]; !ok {
				continue
			}
		}
		schema, _ := json.Marshal(t.InputSchema)
		out = append(out, llm.ToolSpec{
			Name:        t.QualifiedName,
			Description: t.Description,
			Schema:      schema,
		})
	}
	return out
}

func (h *LLMHost) Execute(ctx context.Context, name string, args json.RawMessage) (string, bool, error) {
	timeout := h.server.config.LLM.ToolTimeoutSec
	if timeout <= 0 {
		timeout = 60
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	result, err := h.server.executeQualifiedTool(ctx, name, args)
	if err != nil {
		return err.Error(), true, err
	}
	return result.Text(), false, nil
}

func toSet(s []string) map[string]struct{} {
	if len(s) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(s))
	for _, v := range s {
		out[v] = struct{}{}
	}
	return out
}
```

The exact method names (`ListTools`, `executeQualifiedTool`, `t.QualifiedName`, `t.InputSchema`, `result.Text()`) must be adapted to the real registry API discovered in Step 1. Keep this file thin — it's an adapter.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/server/... -run LLMHost`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/llm_host.go internal/server/llm_host_test.go
git commit -m "feat(server): ToolHost adapter bridging registry to llm builtin"
```

---

## Task 14: Wire builtin into all registration sites

**Files (modify):**
- `internal/server/handlers.go` (line ~289 — `run_slop` handler runtime construction)
- `internal/server/custom_exec.go` (line ~126 — custom-tool runtime)
- `cmd/slop-mcp/run.go` (line ~93 — one-shot CLI)
- `cmd/slop-mcp/monitor.go` (line ~111 — monitor mode)

Each site already calls `builtins.RegisterCrypto(rt)` and friends. Add:

```go
builtins.RegisterLLM(rt, s.llmRegistry, server.NewLLMHost(s), progressFn)
```

In `run.go` and `monitor.go` there is no Server, so:

```go
host := llm.NoopHost{}                         // tools unavailable in CLI mode
builtins.RegisterLLM(rt, llmRegistry, host, nil)
```

`NoopHost` returns no specs and an error for any execution — the loop just does single-turn responses. Add it to `internal/llm/tools.go`.

- [ ] **Step 1: Add NoopHost to internal/llm/tools.go**

```go
type NoopHost struct{}

func (NoopHost) List(_ context.Context, _ []string) []ToolSpec { return nil }
func (NoopHost) Execute(_ context.Context, name string, _ json.RawMessage) (string, bool, error) {
	return "", true, fmt.Errorf("tools unavailable in this context: %s", name)
}
```

- [ ] **Step 2: Add `llmRegistry` field on Server, build it at startup**

Modify `internal/server/server.go`: add `llmRegistry *llm.Registry`. Construct in the existing server-init function from `s.config`:

```go
r, err := llm.Build(s.config, llm.BuildDeps{
	HTTPClient:        http.DefaultClient,
	CopilotTokenStore: s.authStore,  // existing auth_mcp store; adapt method name
})
if err != nil {
	return nil, fmt.Errorf("init llm: %w", err)
}
s.llmRegistry = r
```

- [ ] **Step 3: Insert `RegisterLLM` calls at the four sites**

For `internal/server/handlers.go` (`run_slop`):
```go
progressFn := func(ev llm.StreamEvent) {
	if ev.Delta != "" {
		_ = mcp.NotifyProgress(ctx, progressToken, ev.Delta)
	}
}
builtins.RegisterLLM(rt, s.llmRegistry, NewLLMHost(s), progressFn)
```

For `internal/server/custom_exec.go`: same as above (custom tools also have progress tokens).

For `cmd/slop-mcp/run.go`:
```go
r, _ := llm.Build(cfg, llm.BuildDeps{HTTPClient: http.DefaultClient})
builtins.RegisterLLM(rt, r, llm.NoopHost{}, func(ev llm.StreamEvent) {
	if ev.Delta != "" {
		fmt.Fprint(os.Stderr, ev.Delta)
	}
})
```

For `cmd/slop-mcp/monitor.go`: same pattern as `run.go`.

- [ ] **Step 4: Build and run all tests**

Run:
```bash
go build ./...
go test ./...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/server/handlers.go internal/server/custom_exec.go cmd/slop-mcp/run.go cmd/slop-mcp/monitor.go internal/llm/tools.go
git commit -m "feat(server): wire llm() builtin into all registration sites"
```

---

## Task 15: Add llm category to slop_reference

**Files:**
- Modify: `internal/builtins/slop_reference.go`
- Test: existing reference tests should keep passing; add cases asserting "llm" entries exist.

- [ ] **Step 1: Add entries**

```go
{Name: "llm", Category: "llm", Signature: `llm(prompt, agent: "name", system, model, tools, temperature, max_tokens, max_iters, messages)`, Description: "Call a language model. Returns the final assistant string. Use llm_last() for usage/model/iteration metadata.", Example: `llm("Summarize: " + text, agent: "summarizer")`, Returns: "string"},
{Name: "llm_last", Category: "llm", Signature: "llm_last()", Description: "Returns metadata about the most recent llm() call: model, provider, iterations, stop_reason, usage.", Example: `meta = llm_last(); log(meta["model"])`, Returns: "map"},
```

- [ ] **Step 2: Add a test asserting the category exists**

```go
func TestSlopReference_LLMCategory(t *testing.T) {
	var found bool
	for _, e := range builtinCatalog {
		if e.Category == "llm" && e.Name == "llm" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("llm category missing from slop_reference catalog")
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/builtins/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/builtins/slop_reference.go internal/builtins/slop_reference_test.go
git commit -m "docs(slop): add llm category to slop_reference catalog"
```

---

## Task 16: Streaming for OpenAI provider (SSE)

**Files:**
- Modify: `internal/llm/providers/openai.go`
- Test: extend `openai_test.go` with a streaming case.

OpenAI streaming uses Server-Sent Events with `data: {...}` lines and a final `data: [DONE]`. Implement only when `onEvent != nil`; otherwise keep the blocking path.

- [ ] **Step 1: Write the failing streaming test**

```go
func TestOpenAI_Streaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}, \"finish_reason\":\"stop\"}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL, APIKey: "k", DefaultModel: "m"})
	var collected string
	resp, err := p.Sample(context.Background(), &llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	}, func(ev llm.StreamEvent) {
		collected += ev.Delta
	})
	if err != nil {
		t.Fatal(err)
	}
	if collected != "hello" {
		t.Fatalf("collected deltas: %q", collected)
	}
	if resp.Content != "hello" {
		t.Fatalf("final content: %q", resp.Content)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/providers/... -run TestOpenAI_Streaming`
Expected: FAIL.

- [ ] **Step 3: Implement streaming branch**

In `Sample`, branch on `onEvent != nil`. When set, request `"stream": true` in the body and parse SSE:

```go
// when onEvent != nil
reqBody := buildChatReq(model, req)
type streamReq struct{ chatReq; Stream bool `json:"stream"` }
body, _ := json.Marshal(struct {
	chatReq
	Stream bool `json:"stream"`
}{reqBody, true})

httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
httpReq.Header.Set("Content-Type", "application/json")
httpReq.Header.Set("Accept", "text/event-stream")
o.cfg.AuthHeader(httpReq)

httpResp, err := o.cfg.HTTPClient.Do(httpReq)
if err != nil { return nil, err }
defer httpResp.Body.Close()

return parseSSE(httpResp.Body, onEvent)
```

```go
func parseSSE(r io.Reader, onEvent func(llm.StreamEvent)) (*llm.Response, error) {
	br := bufio.NewReader(r)
	out := &llm.Response{}
	var content strings.Builder
	for {
		line, err := br.ReadString('\n')
		if err == io.EOF { break }
		if err != nil { return nil, err }
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") { continue }
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" { break }

		var chunk struct {
			Choices []struct {
				Delta        struct {
					Content   string         `json:"content"`
					ToolCalls []chatToolCall `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Model string `json:"model"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil { continue }
		if chunk.Model != "" { out.Model = chunk.Model }
		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				content.WriteString(c.Delta.Content)
				onEvent(llm.StreamEvent{Delta: c.Delta.Content})
			}
			// tool-call deltas — append/merge by index; omitted from this sketch,
			// see OpenAI streaming docs. For v1, accept tool calls only on the
			// final non-streamed path or treat finish_reason="tool_calls" as
			// trigger to refetch in non-streaming mode (simpler).
			switch c.FinishReason {
			case "stop":  out.StopReason = llm.StopReasonEndTurn
			case "length": out.StopReason = llm.StopReasonMaxTokens
			case "tool_calls": out.StopReason = llm.StopReasonToolUse
			}
		}
	}
	out.Content = content.String()
	onEvent(llm.StreamEvent{Done: true, Final: out})
	return out, nil
}
```

For tool-call streaming complexity: v1 ships text-only streaming. If `finish_reason == "tool_calls"` arrives in the stream, fall back to a non-streaming refetch to get the structured tool_calls reliably. Document in code comment.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/llm/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/llm/providers/openai.go internal/llm/providers/openai_test.go
git commit -m "feat(llm): SSE streaming for OpenAI provider"
```

---

## Task 17: Integration test against real local endpoint

**Files:**
- Create: `internal/llm/providers/integration_test.go` (build tag `integration`)

- [ ] **Step 1: Write the test**

```go
//go:build integration

package providers

import (
	"context"
	"os"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/llm"
)

func TestIntegration_OpenAIRoundTrip(t *testing.T) {
	if os.Getenv("LLM_INTEGRATION_TEST") != "1" {
		t.Skip("set LLM_INTEGRATION_TEST=1 to run")
	}
	url := os.Getenv("LLM_TEST_URL")
	if url == "" {
		url = "http://localhost:11434/v1" // ollama default
	}
	model := os.Getenv("LLM_TEST_MODEL")
	if model == "" {
		model = "llama3.2"
	}

	p := NewOpenAI(OpenAIConfig{BaseURL: url, APIKey: "ollama", DefaultModel: model})
	resp, err := p.Sample(context.Background(), &llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Say only the word PONG."}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("response: %q (model=%s)", resp.Content, resp.Model)
	if resp.Content == "" {
		t.Fatal("empty response from real endpoint")
	}
}
```

- [ ] **Step 2: Verify it skips by default**

Run: `go test ./internal/llm/providers/...`
Expected: PASS (skipped).

Run with: `LLM_INTEGRATION_TEST=1 go test -tags integration ./internal/llm/providers/...` — verify against a running local endpoint.

- [ ] **Step 3: Commit**

```bash
git add internal/llm/providers/integration_test.go
git commit -m "test(llm): add OpenAI-compat integration test (gated by env)"
```

---

## Task 18: Documentation page

**Files:**
- Create: `docs/usage/llm.md`

- [ ] **Step 1: Write the doc**

Cover:
- KDL config example (full).
- `llm()` and `llm_last()` signatures with kwargs table.
- Fall-through chain semantics.
- Tool whitelist and how the model gets MCP tools.
- Each provider's auth (env, OAuth via auth_mcp, ACP subprocess).
- Examples in SLOP including a multi-turn case.

- [ ] **Step 2: Commit**

```bash
git add docs/usage/llm.md
git commit -m "docs: usage page for llm() builtin and agent config"
```

---

## Task 19: Final verification

- [ ] **Step 1: Full test pass**

Run:
```bash
make lint
make test
make test-mock
go build ./...
```
Expected: all green.

- [ ] **Step 2: End-to-end smoke**

Run a tiny SLOP program through `slop-mcp run`:

```bash
export OPENAI_API_KEY=...   # or point at ollama
./build/slop-mcp run -e 'log(llm("Say PONG.", agent: "summarizer"))'
```

Expected: prints model output to stdout.

- [ ] **Step 3: Final commit if any cleanup remained**

```bash
git status
# if clean, no commit; otherwise:
git commit -am "chore(llm): final cleanup"
```

---

## Reference Skills

- @superpowers:test-driven-development — every task uses red/green/refactor.
- @superpowers:verification-before-completion — Task 19 is the final gate.
- @superpowers:requesting-code-review — invoke after Task 19 before merge.
