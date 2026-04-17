# Tool Customization Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a ninth meta-tool `customize_tools` that lets agents override descriptions/param docs and define SLOP-backed custom tools, scoped to user/project/local with import/export and hash-based staleness detection.

**Architecture:** New `internal/overrides/` package owns storage, scope merge, hashing, and import/export. Writes go through a per-bank flusher (single-slot coalescing, no locks held over I/O). Registry's ranking index becomes `atomic.Pointer[ToolIndex]` so rebuilds never block readers. Server grows one new meta-tool with action dispatch. Custom tools execute through the existing `run_slop` engine with arg validation and a recursion depth guard.

**Tech Stack:** Go 1.24, github.com/modelcontextprotocol/go-sdk, github.com/standardbeagle/slop, crypto/sha256, encoding/json, sync/atomic.

**Spec:** `docs/superpowers/specs/2026-04-16-tool-customization-layer-design.md`

---

## File Structure

### New files

- `internal/overrides/overrides.go` — entry types, bank name constants, public API surface
- `internal/overrides/hash.go` — canonical JSON + truncated SHA-256
- `internal/overrides/scope.go` — scope roots, repo root detection, merge
- `internal/overrides/store.go` — scope-aware store with per-bank in-memory map
- `internal/overrides/flusher.go` — single-slot coalescing background writer
- `internal/overrides/pack.go` — import/export pack schema + validation
- `internal/overrides/hash_test.go`
- `internal/overrides/scope_test.go`
- `internal/overrides/store_test.go`
- `internal/overrides/flusher_test.go`
- `internal/overrides/pack_test.go`
- `internal/builtins/reserved.go` — list of SLOP builtin names (for shorthand binding collision detection)
- `internal/server/customize_handler.go` — `handleCustomizeTools` with action dispatch
- `internal/server/customize_handler_test.go`
- `internal/server/custom_exec.go` — custom tool routing, arg validation, binding, recursion guard
- `internal/server/custom_exec_test.go`
- `docs/internal/description-style.md` — developer reference for the caveman-style description rules
- `docs/docs/concepts/customization.md` — user-facing docs with Figma example

### Modified files

- `internal/builtins/memory.go` — reject mutating ops on `_slop.*` banks
- `cmd/memory-cli/main.go` — same reserved-prefix guard
- `internal/server/schemas.go` — `customize_tools` input schema; rewrite existing descriptions
- `internal/server/tools.go` — register `customize_tools`; rewrite existing descriptions
- `internal/server/handlers.go` — wire override merge into `handleSearchTools`, `handleGetMetadata`; route custom tools in `handleExecuteTool`
- `internal/registry/registry.go` — swap ranking index to `atomic.Pointer[ToolIndex]`; accept an overrides provider interface
- `internal/registry/index.go` — build index via injected provider (override + custom tool source)
- `internal/server/server.go` — construct `overrides.Store`, wire into registry, bump `serverVersion` to `0.14.0`
- `CHANGELOG.md` — 0.14.0 entry

---

## Phase 1 — Foundations (pure data)

### Task 1: Entry types + constants

**Files:**
- Create: `internal/overrides/overrides.go`
- Create: `internal/overrides/overrides_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/overrides/overrides_test.go
package overrides

import (
	"encoding/json"
	"testing"
)

func TestOverrideEntry_JSONRoundTrip(t *testing.T) {
	e := OverrideEntry{
		Description: "short",
		Params:      map[string]string{"q": "query"},
		SourceHash:  "abc123",
		Scope:       ScopeUser,
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var got OverrideEntry
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Description != "short" || got.SourceHash != "abc123" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestBankNames(t *testing.T) {
	if BankOverrides != "_slop.overrides" {
		t.Errorf("BankOverrides = %q, want _slop.overrides", BankOverrides)
	}
	if BankCustomTools != "_slop.tools" {
		t.Errorf("BankCustomTools = %q, want _slop.tools", BankCustomTools)
	}
	if !IsReservedBank("_slop.overrides") || !IsReservedBank("_slop.anything") {
		t.Error("IsReservedBank should match _slop. prefix")
	}
	if IsReservedBank("user_bank") {
		t.Error("IsReservedBank should not match arbitrary names")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/overrides/... -run TestOverrideEntry_JSONRoundTrip -v`
Expected: FAIL (package does not exist yet)

- [ ] **Step 3: Write minimal implementation**

```go
// internal/overrides/overrides.go
// Package overrides owns tool-description overrides and agent-authored
// custom tools, scoped to user / project / local tiers.
package overrides

import (
	"strings"
	"time"
)

// Bank names reserved for the overrides subsystem.
const (
	BankOverrides   = "_slop.overrides"
	BankCustomTools = "_slop.tools"
	ReservedPrefix  = "_slop."
)

// IsReservedBank reports whether a bank name is owned by this subsystem.
func IsReservedBank(name string) bool {
	return strings.HasPrefix(name, ReservedPrefix)
}

// Scope identifies a storage tier.
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
	ScopeLocal   Scope = "local"
)

// AllScopes is the canonical scope ordering used when no scope is specified.
// Iterating in this order respects precedence at merge time (Local > Project > User).
var AllScopes = []Scope{ScopeLocal, ScopeProject, ScopeUser}

// OverrideEntry is the value shape stored under BankOverrides, keyed by "<mcp>.<tool>".
type OverrideEntry struct {
	Description string            `json:"description"`
	Params      map[string]string `json:"params,omitempty"`
	SourceHash  string            `json:"source_hash"`
	Scope       Scope             `json:"scope,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at,omitempty"`
}

// Dependency is a hash-pinned reference to a native tool used by a custom tool.
type Dependency struct {
	MCP  string `json:"mcp"`
	Tool string `json:"tool"`
	Hash string `json:"hash"`
}

// CustomTool is the value shape stored under BankCustomTools, keyed by tool name.
type CustomTool struct {
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Body        string         `json:"body"`
	DependsOn   []Dependency   `json:"depends_on,omitempty"`
	Scope       Scope          `json:"scope,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at,omitempty"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/overrides/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/overrides/overrides.go internal/overrides/overrides_test.go
git commit -m "feat(overrides): add entry types and reserved bank constants"
```

---

### Task 2: Canonical JSON + hashing

**Files:**
- Create: `internal/overrides/hash.go`
- Create: `internal/overrides/hash_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/overrides/hash_test.go
package overrides

import "testing"

func TestCanonicalJSON_SortsKeys(t *testing.T) {
	out, err := canonicalJSON(map[string]string{"b": "2", "a": "1"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":"1","b":"2"}`
	if string(out) != want {
		t.Errorf("canonicalJSON = %s, want %s", out, want)
	}
}

func TestCanonicalJSON_NestedSorts(t *testing.T) {
	in := map[string]any{
		"z": map[string]any{"y": 1, "x": 2},
		"a": []int{3, 1, 2},
	}
	out, err := canonicalJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	// Arrays preserve order; map keys are sorted at every level.
	want := `{"a":[3,1,2],"z":{"x":2,"y":1}}`
	if string(out) != want {
		t.Errorf("canonicalJSON = %s, want %s", out, want)
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	params := map[string]string{"q": "query", "a": "anchor"}
	h1 := ComputeHash("desc", params)
	h2 := ComputeHash("desc", map[string]string{"a": "anchor", "q": "query"})
	if h1 != h2 {
		t.Errorf("hash should be deterministic across map ordering: %s vs %s", h1, h2)
	}
	if len(h1) != 16 {
		t.Errorf("hash len = %d, want 16", len(h1))
	}
}

func TestComputeHash_Sensitive(t *testing.T) {
	base := ComputeHash("desc", map[string]string{"a": "b"})
	diffDesc := ComputeHash("desc2", map[string]string{"a": "b"})
	diffParams := ComputeHash("desc", map[string]string{"a": "c"})
	if base == diffDesc || base == diffParams {
		t.Error("hash must differ on description or params change")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/overrides/... -run TestComputeHash -v`
Expected: FAIL (symbols undefined)

- [ ] **Step 3: Write minimal implementation**

```go
// internal/overrides/hash.go
package overrides

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// canonicalJSON produces a deterministic JSON encoding with sorted map keys
// at every level and no extra whitespace. Arrays preserve their input order.
func canonicalJSON(v any) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := encodeCanonical(buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeCanonical(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
	case map[string]any:
		return encodeMap(buf, x)
	case map[string]string:
		m := make(map[string]any, len(x))
		for k, s := range x {
			m[k] = s
		}
		return encodeMap(buf, m)
	case []any:
		buf.WriteByte('[')
		for i, el := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeCanonical(buf, el); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	default:
		// Fall through to stdlib for scalars + anything else.
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}

func encodeMap(buf *bytes.Buffer, m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		if err := encodeCanonical(buf, m[k]); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

// ComputeHash returns the first 16 hex characters of
// SHA-256(description + "\n" + canonical_json(params)).
func ComputeHash(description string, params map[string]string) string {
	pb, err := canonicalJSON(params)
	if err != nil {
		// Fall back to fmt.Sprintf so hashing is always total.
		pb = []byte(fmt.Sprintf("%v", params))
	}
	h := sha256.New()
	h.Write([]byte(description))
	h.Write([]byte("\n"))
	h.Write(pb)
	return hex.EncodeToString(h.Sum(nil))[:16]
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/overrides/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/overrides/hash.go internal/overrides/hash_test.go
git commit -m "feat(overrides): add canonical JSON and truncated SHA-256 hashing"
```

---

### Task 3: Scope roots + repo detection + merge

**Files:**
- Create: `internal/overrides/scope.go`
- Create: `internal/overrides/scope_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/overrides/scope_test.go
package overrides

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRepoRoot_GitMarker(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	got, err := findRepoRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("findRepoRoot = %q, want %q", got, dir)
	}
}

func TestFindRepoRoot_SlopMcpMarker(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".slop-mcp.kdl"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := findRepoRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("findRepoRoot = %q, want %q", got, dir)
	}
}

func TestFindRepoRoot_NotFound(t *testing.T) {
	dir := t.TempDir()
	if _, err := findRepoRoot(dir); err == nil {
		t.Error("expected error when no repo marker is present")
	}
}

func TestMergeScopes_LocalBeatsProjectBeatsUser(t *testing.T) {
	userE := OverrideEntry{Description: "u", Scope: ScopeUser}
	projE := OverrideEntry{Description: "p", Scope: ScopeProject}
	locE := OverrideEntry{Description: "l", Scope: ScopeLocal}

	got := MergeOverride(map[Scope]*OverrideEntry{
		ScopeUser:    &userE,
		ScopeProject: &projE,
		ScopeLocal:   &locE,
	})
	if got.Description != "l" || got.Scope != ScopeLocal {
		t.Errorf("merge = %+v, want local winner", got)
	}

	got2 := MergeOverride(map[Scope]*OverrideEntry{
		ScopeUser:    &userE,
		ScopeProject: &projE,
	})
	if got2.Description != "p" || got2.Scope != ScopeProject {
		t.Errorf("merge = %+v, want project winner", got2)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/overrides/... -run TestFindRepoRoot -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// internal/overrides/scope.go
package overrides

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNoRepo is returned when project/local scope is requested outside a repo.
var ErrNoRepo = errors.New("not inside a slop-mcp repo (need .git or .slop-mcp.kdl)")

// findRepoRoot walks up from start looking for .git or .slop-mcp.kdl.
func findRepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, ".slop-mcp.kdl")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNoRepo
		}
		dir = parent
	}
}

// ScopeRoot returns the on-disk directory for the given scope.
// userHome and cwd let tests inject locations.
func ScopeRoot(scope Scope, userHome, cwd string) (string, error) {
	switch scope {
	case ScopeUser:
		return filepath.Join(userHome, ".config", "slop-mcp", "memory", "_slop"), nil
	case ScopeProject:
		root, err := findRepoRoot(cwd)
		if err != nil {
			return "", err
		}
		return filepath.Join(root, ".slop-mcp", "memory", "_slop"), nil
	case ScopeLocal:
		root, err := findRepoRoot(cwd)
		if err != nil {
			return "", err
		}
		return filepath.Join(root, ".slop-mcp", "memory.local", "_slop"), nil
	default:
		return "", errors.New("unknown scope: " + string(scope))
	}
}

// MergeOverride picks the entry from the scope map using precedence
// Local > Project > User. Returns nil if the map is empty.
func MergeOverride(per map[Scope]*OverrideEntry) *OverrideEntry {
	for _, s := range AllScopes { // Local, Project, User
		if e := per[s]; e != nil {
			cp := *e
			cp.Scope = s
			return &cp
		}
	}
	return nil
}

// MergeCustom picks custom tool by same precedence.
func MergeCustom(per map[Scope]*CustomTool) *CustomTool {
	for _, s := range AllScopes {
		if e := per[s]; e != nil {
			cp := *e
			cp.Scope = s
			return &cp
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/overrides/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/overrides/scope.go internal/overrides/scope_test.go
git commit -m "feat(overrides): add scope resolution and merge precedence"
```

---

## Phase 2 — Storage

### Task 4: Reserved-bank guard in memory builtins

**Files:**
- Modify: `internal/builtins/memory.go` — add `IsReserved` guard at entry of `mem_save`, and add `mem_delete`/`mem_clear` guards if those exist (or skip if not yet present)
- Modify: `cmd/memory-cli/main.go` — same guard on write subcommands
- Create: `internal/builtins/memory_reserved_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/builtins/memory_reserved_test.go
package builtins

import (
	"testing"

	"github.com/standardbeagle/slop/pkg/slop"
)

func TestMemSave_RejectsReservedBank(t *testing.T) {
	dir := t.TempDir()
	store := NewMemoryStoreWithDir(dir)
	rt := slop.NewRuntime()
	RegisterMemory(rt, store)

	_, err := rt.Call("mem_save", []slop.Value{
		slop.NewStringValue("_slop.overrides"),
		slop.NewStringValue("foo.bar"),
		slop.NewStringValue("v"),
	}, nil)
	if err == nil {
		t.Fatal("expected error saving to reserved bank")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Errorf("error should mention reserved: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/builtins/... -run TestMemSave_RejectsReservedBank -v`
Expected: FAIL

- [ ] **Step 3: Implement the guard**

Add the check at the top of each mutating builtin in `internal/builtins/memory.go`. Pattern:

```go
// Add near the top of memory.go:
import "github.com/standardbeagle/slop-mcp/internal/overrides"

// Inside mem_save handler, immediately after parsing bankName:
if overrides.IsReservedBank(bankName.Value) {
    return nil, fmt.Errorf("mem_save: bank %q is reserved; use customize_tools", bankName.Value)
}
```

**Discovery step first:** grep for existing mutating builtins so the guard goes on every one that exists and not on ones that don't.

```bash
grep -n 'RegisterBuiltin("mem_\|RegisterBuiltin("store_set' internal/builtins/memory.go internal/builtins/session.go
```

Expected hits: `mem_save`, `mem_delete` (add guard). If `mem_clear` shows up, guard it too; otherwise skip. Skip read-only builtins (`mem_load`, `mem_list`, `mem_search`, `mem_info`).

Mirror the guard in `cmd/memory-cli/main.go`. Run `grep -n 'func.*[Cc]md' cmd/memory-cli/main.go` to enumerate subcommands, then guard only the write subcommands (whatever they are named — likely `save`, `delete`) with an exit code 2 and the same error text.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/builtins/... -v && go build ./...`
Expected: PASS + clean build

- [ ] **Step 5: Commit**

```bash
git add internal/builtins/memory.go internal/builtins/memory_reserved_test.go cmd/memory-cli/main.go
git commit -m "feat(builtins): block _slop.* writes via mem_save and memory-cli"
```

---

### Task 5: Single-slot per-bank flusher

**Files:**
- Create: `internal/overrides/flusher.go`
- Create: `internal/overrides/flusher_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/overrides/flusher_test.go
package overrides

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestFlusher_CoalescesBursts(t *testing.T) {
	var writeCount int32
	f := newFlusher("test", func() error {
		atomic.AddInt32(&writeCount, 1)
		time.Sleep(10 * time.Millisecond) // simulate disk
		return nil
	})
	defer f.close()

	// Burst of 100 dirty signals; flusher must coalesce.
	for i := 0; i < 100; i++ {
		f.markDirty()
	}
	// Give flusher time to run pending writes.
	time.Sleep(200 * time.Millisecond)

	got := atomic.LoadInt32(&writeCount)
	if got < 1 || got > 5 {
		t.Errorf("write count = %d, want 1..5 (coalesced)", got)
	}
}

func TestFlusher_ShutdownFlushesPending(t *testing.T) {
	var writeCount int32
	f := newFlusher("test", func() error {
		atomic.AddInt32(&writeCount, 1)
		return nil
	})
	f.markDirty()
	f.close() // Blocks until pending drain.

	if atomic.LoadInt32(&writeCount) < 1 {
		t.Error("shutdown should flush pending")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/overrides/... -run TestFlusher -v`
Expected: FAIL

- [ ] **Step 3: Write implementation**

```go
// internal/overrides/flusher.go
package overrides

import (
	"log/slog"
)

// flusher coalesces bank-dirty signals and writes in the background.
// A capacity-1 buffered channel is the coalescing primitive: a dropped
// send simply means a write is already scheduled.
type flusher struct {
	name  string
	write func() error
	dirty chan struct{}
	done  chan struct{}
	quit  chan struct{}
}

func newFlusher(name string, write func() error) *flusher {
	f := &flusher{
		name:  name,
		write: write,
		dirty: make(chan struct{}, 1),
		done:  make(chan struct{}),
		quit:  make(chan struct{}),
	}
	go f.run()
	return f
}

// markDirty signals the bank has pending changes.
// Non-blocking: if a signal is already pending, this call is dropped.
func (f *flusher) markDirty() {
	select {
	case f.dirty <- struct{}{}:
	default:
	}
}

// close flushes any pending signal and stops the goroutine.
func (f *flusher) close() {
	close(f.quit)
	<-f.done
}

func (f *flusher) run() {
	defer close(f.done)
	for {
		select {
		case <-f.quit:
			// Drain one last pending signal if any.
			select {
			case <-f.dirty:
				if err := f.write(); err != nil {
					slog.Warn("flusher shutdown write", "bank", f.name, "err", err)
				}
			default:
			}
			return
		case <-f.dirty:
			if err := f.write(); err != nil {
				slog.Warn("flusher write", "bank", f.name, "err", err)
			}
		}
	}
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/overrides/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/overrides/flusher.go internal/overrides/flusher_test.go
git commit -m "feat(overrides): add single-slot coalescing bank flusher"
```

---

### Task 6: Scope-aware store

**Files:**
- Create: `internal/overrides/store.go`
- Create: `internal/overrides/store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/overrides/store_test.go
package overrides

import (
	"path/filepath"
	"testing"
)

func TestStore_WriteReadOverride(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(StoreOptions{UserRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	e := OverrideEntry{Description: "compressed", SourceHash: "h1"}
	if err := s.SetOverride(ScopeUser, "figma.get_file", e); err != nil {
		t.Fatal(err)
	}

	got, found := s.GetOverride("figma.get_file")
	if !found {
		t.Fatal("override not found after set")
	}
	if got.Description != "compressed" || got.Scope != ScopeUser {
		t.Errorf("got %+v", got)
	}
}

func TestStore_ScopePrecedence(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(StoreOptions{
		UserRoot:    filepath.Join(dir, "user"),
		ProjectRoot: filepath.Join(dir, "project"),
		LocalRoot:   filepath.Join(dir, "local"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_ = s.SetOverride(ScopeUser, "k", OverrideEntry{Description: "user"})
	_ = s.SetOverride(ScopeProject, "k", OverrideEntry{Description: "proj"})

	got, _ := s.GetOverride("k")
	if got.Description != "proj" || got.Scope != ScopeProject {
		t.Errorf("project should beat user: %+v", got)
	}

	_ = s.SetOverride(ScopeLocal, "k", OverrideEntry{Description: "loc"})
	got, _ = s.GetOverride("k")
	if got.Description != "loc" || got.Scope != ScopeLocal {
		t.Errorf("local should beat project: %+v", got)
	}
}

func TestStore_RemoveOverrideAllScopes(t *testing.T) {
	dir := t.TempDir()
	s, _ := OpenStore(StoreOptions{
		UserRoot:    filepath.Join(dir, "u"),
		ProjectRoot: filepath.Join(dir, "p"),
	})
	defer s.Close()

	_ = s.SetOverride(ScopeUser, "k", OverrideEntry{})
	_ = s.SetOverride(ScopeProject, "k", OverrideEntry{})

	n, err := s.RemoveOverride("", "k") // empty scope = all
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("removed %d, want 2", n)
	}
	if _, ok := s.GetOverride("k"); ok {
		t.Error("key should be gone")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/overrides/... -run TestStore -v`
Expected: FAIL

- [ ] **Step 3: Write implementation**

```go
// internal/overrides/store.go
package overrides

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StoreOptions configures the on-disk roots. Empty values mean the scope is
// unavailable (e.g. ProjectRoot empty outside a repo).
type StoreOptions struct {
	UserRoot    string
	ProjectRoot string
	LocalRoot   string
}

// Store holds the in-memory state for both reserved banks across all scopes.
type Store struct {
	opts StoreOptions

	mu        sync.RWMutex
	overrides map[Scope]map[string]OverrideEntry
	custom    map[Scope]map[string]CustomTool

	flushers map[string]*flusher // keyed by "<scope>:<bank>"
}

// OpenStore creates and warms the store from disk.
func OpenStore(opts StoreOptions) (*Store, error) {
	s := &Store{
		opts:      opts,
		overrides: map[Scope]map[string]OverrideEntry{},
		custom:    map[Scope]map[string]CustomTool{},
		flushers:  map[string]*flusher{},
	}
	for _, scope := range AllScopes {
		if s.rootFor(scope) == "" {
			continue
		}
		if err := s.loadScope(scope); err != nil {
			return nil, fmt.Errorf("load %s: %w", scope, err)
		}
	}
	return s, nil
}

func (s *Store) rootFor(scope Scope) string {
	switch scope {
	case ScopeUser:
		return s.opts.UserRoot
	case ScopeProject:
		return s.opts.ProjectRoot
	case ScopeLocal:
		return s.opts.LocalRoot
	}
	return ""
}

func (s *Store) loadScope(scope Scope) error {
	root := s.rootFor(scope)
	s.overrides[scope] = map[string]OverrideEntry{}
	s.custom[scope] = map[string]CustomTool{}

	if err := readBankFile(filepath.Join(root, BankOverrides+".json"), &s.overrides, scope); err != nil {
		return err
	}
	return readBankFile(filepath.Join(root, BankCustomTools+".json"), &s.custom, scope)
}

// readBankFile loads one bank JSON into the provided map at the given scope.
// Missing file is fine.
func readBankFile[T any](path string, dst *map[Scope]map[string]T, scope Scope) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var m map[string]T
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	(*dst)[scope] = m
	return nil
}

// SetOverride stores or replaces an override entry at the given scope.
func (s *Store) SetOverride(scope Scope, key string, e OverrideEntry) error {
	if s.rootFor(scope) == "" {
		return fmt.Errorf("scope %s unavailable", scope)
	}
	e.Scope = scope
	e.UpdatedAt = time.Now()

	s.mu.Lock()
	if s.overrides[scope] == nil {
		s.overrides[scope] = map[string]OverrideEntry{}
	}
	s.overrides[scope][key] = e
	s.mu.Unlock()

	s.markDirty(scope, BankOverrides)
	return nil
}

// GetOverride returns the merged (scope-winning) override entry for a key.
func (s *Store) GetOverride(key string) (OverrideEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	per := map[Scope]*OverrideEntry{}
	for scope, m := range s.overrides {
		if e, ok := m[key]; ok {
			cp := e
			per[scope] = &cp
		}
	}
	merged := MergeOverride(per)
	if merged == nil {
		return OverrideEntry{}, false
	}
	return *merged, true
}

// RemoveOverride deletes from the given scope, or all scopes if scope=="".
// Returns the number of entries removed.
func (s *Store) RemoveOverride(scope Scope, key string) (int, error) {
	s.mu.Lock()
	n := 0
	touched := map[Scope]bool{}
	if scope == "" {
		for sc, m := range s.overrides {
			if _, ok := m[key]; ok {
				delete(m, key)
				n++
				touched[sc] = true
			}
		}
	} else {
		if m := s.overrides[scope]; m != nil {
			if _, ok := m[key]; ok {
				delete(m, key)
				n = 1
				touched[scope] = true
			}
		}
	}
	s.mu.Unlock()
	for sc := range touched {
		s.markDirty(sc, BankOverrides)
	}
	return n, nil
}

// Close flushes all pending writes.
func (s *Store) Close() error {
	for _, f := range s.flushers {
		f.close()
	}
	return nil
}

// markDirty enqueues a flush for the given scope+bank.
func (s *Store) markDirty(scope Scope, bank string) {
	fkey := string(scope) + ":" + bank
	s.mu.Lock()
	f, ok := s.flushers[fkey]
	if !ok {
		f = newFlusher(fkey, func() error { return s.writeBank(scope, bank) })
		s.flushers[fkey] = f
	}
	s.mu.Unlock()
	f.markDirty()
}

// writeBank snapshots the bank under a brief lock, then writes without a lock held.
func (s *Store) writeBank(scope Scope, bank string) error {
	root := s.rootFor(scope)
	if root == "" {
		return errors.New("scope root not set")
	}
	s.mu.RLock()
	var snapshot any
	switch bank {
	case BankOverrides:
		cp := make(map[string]OverrideEntry, len(s.overrides[scope]))
		for k, v := range s.overrides[scope] {
			cp[k] = v
		}
		snapshot = cp
	case BankCustomTools:
		cp := make(map[string]CustomTool, len(s.custom[scope]))
		for k, v := range s.custom[scope] {
			cp[k] = v
		}
		snapshot = cp
	}
	s.mu.RUnlock()

	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(root, bank+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/overrides/... -v`
Expected: PASS

- [ ] **Step 5: Add custom tool write/read/remove (mirror override paths)**

Add `SetCustom`, `GetCustom`, `RemoveCustom`, `ListCustom` following the same shape as the override methods. Add tests covering scope merge for custom tools.

- [ ] **Step 6: Run tests and commit**

```bash
go test ./internal/overrides/... -v
git add internal/overrides/store.go internal/overrides/store_test.go
git commit -m "feat(overrides): add scope-aware store with background flush"
```

---

## Phase 3 — Routing + Integration

### Task 7: Atomic-pointer ranking index

**Files:**
- Modify: `internal/registry/index.go` — swap internal index storage to `atomic.Pointer[ToolIndex]`
- Modify: `internal/registry/registry.go` — introduce `OverrideProvider` interface and use it during index build
- Modify: `internal/registry/index_test.go` — add concurrency safety test

- [ ] **Step 1: Write the failing test**

```go
// internal/registry/index_test.go — append
// Match the existing SearchTools signature in registry.go — currently
//   SearchTools(query, mcpName string) []ToolInfo
// If the signature has changed, adjust accordingly.
func TestIndex_AtomicSwapUnderLoad(t *testing.T) {
	reg := New()
	reg.AddToolsForTesting("m", []ToolInfo{{Name: "a", Description: "alpha"}})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			_ = reg.SearchTools("alp", "")
		}
	}()
	for i := 0; i < 100; i++ {
		// Rewrite tools repeatedly to force index rebuilds.
		reg.AddToolsForTesting("m", []ToolInfo{{
			Name:        "a",
			Description: fmt.Sprintf("alpha %d", i),
		}})
	}
	<-done
}
```

The race detector catches concurrent map writes / torn pointer reads.

- [ ] **Step 2: Run to verify failure**

Run: `go test -race ./internal/registry/... -run TestIndex_AtomicSwap -v`
Expected: FAIL or race-detector hit

- [ ] **Step 3: Implement atomic pointer swap**

In `internal/registry/index.go`, store the index behind `atomic.Pointer[ToolIndex]`. Rebuild function:
1. Under `reg.mu.RLock()`, snapshot tool list.
2. Release the lock.
3. Build new `ToolIndex` struct (pure CPU, no lock).
4. `reg.indexPtr.Store(newIndex)`.

Reader paths (`SearchTools`, etc.) call `reg.indexPtr.Load()` once per query.

Add `OverrideProvider` interface to `registry.go`:

```go
type OverrideProvider interface {
    OverrideFor(mcpName, toolName string) (description string, params map[string]string, sourceHash string, ok bool)
    CustomTools() []CustomToolDecl
}

type CustomToolDecl struct {
    Name        string
    Description string
    InputSchema map[string]any
}

func (r *Registry) SetOverrideProvider(p OverrideProvider) {
    r.overrides = p
    r.rebuildIndex()
}
```

Integrate during index build: when copying each tool's description, call the provider and substitute. Append custom tools to the index with `mcp: "_custom"`.

- [ ] **Step 4: Run test with race detector**

Run: `go test -race ./internal/registry/... -v`
Expected: PASS, no races

- [ ] **Step 5: Commit**

```bash
git add internal/registry/index.go internal/registry/registry.go internal/registry/index_test.go
git commit -m "refactor(registry): swap ranking index to atomic.Pointer + add override hook"
```

---

### Task 8: Override-aware search and metadata

**Files:**
- Modify: `internal/server/handlers.go` — `handleSearchTools`, `handleGetMetadata` consult the override store
- Modify: `internal/server/handlers_test.go` — add override-surface tests

- [ ] **Step 1: Write failing tests**

Add to `handlers_test.go`:

```go
func TestSearchTools_AppliesOverride(t *testing.T) {
    s, cleanup := newTestServerWithOverride(t, "mock.tool_one", "SHORT DESC", "h1", "h1")
    defer cleanup()

    in := SearchToolsInput{Query: "tool_one"}
    _, out, err := s.handleSearchTools(context.Background(), nil, in)
    if err != nil {
        t.Fatal(err)
    }
    // Description from override should be returned, not upstream.
    if !strings.Contains(out.String(), "SHORT DESC") {
        t.Errorf("override description missing: %s", out)
    }
}

func TestGetMetadata_FlagsStaleOverride(t *testing.T) {
    s, cleanup := newTestServerWithOverride(t, "mock.tool_one", "override", "stale_hash", "current_hash")
    defer cleanup()

    in := GetMetadataInput{MCPName: "mock", ToolName: "tool_one"}
    _, out, _ := s.handleGetMetadata(context.Background(), nil, in)
    js := out.String()
    if !strings.Contains(js, `"stale":true`) {
        t.Errorf("expected stale flag: %s", js)
    }
    if !strings.Contains(js, `stale_source`) {
        t.Errorf("expected stale_source block: %s", js)
    }
}
```

Add `newTestServerWithOverride` helper beside existing fixtures.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/server/... -run TestSearchTools_AppliesOverride -v`
Expected: FAIL

- [ ] **Step 3: Implement**

In `handleSearchTools`: after the registry returns `Tool` records, look up `store.GetOverride(mcp+"."+tool)`. If found, swap `Description`, recompute upstream hash, and add `stale: true` if mismatch.

In `handleGetMetadata`: same merge, plus splice override params into `inputSchema.properties[*].description`. On stale mismatch, include `stale_source` block with original description + params.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/server/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/handlers.go internal/server/handlers_test.go
git commit -m "feat(server): apply overrides and stale flag in search/metadata"
```

---

### Task 9: Custom tool routing in execute_tool

**Files:**
- Create: `internal/server/custom_exec.go`
- Create: `internal/server/custom_exec_test.go`
- Modify: `internal/server/handlers.go` — add custom-tool branch at top of `handleExecuteTool`

- [ ] **Step 1: Write failing test**

```go
// internal/server/custom_exec_test.go
func TestExecuteTool_RoutesCustomTool(t *testing.T) {
    s, cleanup := newTestServerWithCustomTool(t, "greet", `"hello " + $args.name`,
        map[string]any{
            "type": "object",
            "properties": map[string]any{"name": map[string]any{"type": "string"}},
            "required": []string{"name"},
        })
    defer cleanup()

    in := ExecuteToolInput{MCPName: "_custom", ToolName: "greet", Params: map[string]any{"name": "world"}}
    _, out, err := s.handleExecuteTool(context.Background(), nil, in)
    if err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(out.String(), "hello world") {
        t.Errorf("expected hello world, got %s", out)
    }
}
```

Add helper `newTestServerWithCustomTool`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/server/... -run TestExecuteTool_RoutesCustomTool -v`
Expected: FAIL

- [ ] **Step 3: Implement**

```go
// internal/server/custom_exec.go
package server

import (
    "context"
    "fmt"

    "github.com/standardbeagle/slop-mcp/internal/builtins"
    "github.com/standardbeagle/slop-mcp/internal/overrides"
    "github.com/standardbeagle/slop/pkg/slop"
)

type customExecutor struct {
    store    *overrides.Store
    runtime  *slop.Runtime
    reserved map[string]bool
}

// resolveCustom returns the custom tool matching name across scopes, or false.
func (c *customExecutor) resolveCustom(name string) (overrides.CustomTool, bool) {
    return c.store.GetCustom(name)
}

func (c *customExecutor) execute(ctx context.Context, name string, args map[string]any) (slop.Value, error) {
    tool, ok := c.resolveCustom(name)
    if !ok {
        return nil, fmt.Errorf("custom tool %q not found", name)
    }
    if err := validateAgainstSchema(args, tool.InputSchema); err != nil {
        return nil, fmt.Errorf("args: %w", err)
    }

    ctx = withCustomDepth(ctx)
    if customDepth(ctx) > 16 {
        return nil, ErrCustomToolRecursion
    }

    bindings := map[string]slop.Value{
        "args": slop.GoToValue(args),
    }
    for k, v := range args {
        if c.reserved[k] {
            continue
        }
        bindings[k] = slop.GoToValue(v)
    }
    return c.runtime.Eval(ctx, tool.Body, bindings)
}
```

`validateAgainstSchema` reuses the existing schema validator; `withCustomDepth`/`customDepth` use a private `context.Value` key. `ErrCustomToolRecursion` is a package-level sentinel.

In `handleExecuteTool`, at the very top (before CLI branch, before native MCP):

```go
if input.MCPName == "_custom" || (input.MCPName == "" && s.customExec.hasCustom(input.ToolName)) {
    result, err := s.customExec.execute(ctx, input.ToolName, input.Params)
    if err != nil {
        return nil, nil, err
    }
    return nil, toolOutput(result), nil
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/server/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/custom_exec.go internal/server/custom_exec_test.go internal/server/handlers.go
git commit -m "feat(server): route custom tools through SLOP runtime via execute_tool"
```

---

### Task 10: Reserved shorthand list + collision reporting

**Files:**
- Create: `internal/builtins/reserved.go`
- Create: `internal/builtins/reserved_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/builtins/reserved_test.go
package builtins

import "testing"

func TestReservedNames_KnownBuiltins(t *testing.T) {
    cases := []string{"mem_save", "mem_load", "store_set", "store_get", "emit"}
    for _, c := range cases {
        if !IsReservedBuiltin(c) {
            t.Errorf("%q should be reserved", c)
        }
    }
    if IsReservedBuiltin("my_custom_arg") {
        t.Error("non-builtin must not be reserved")
    }
}
```

- [ ] **Step 2: Implement**

```go
// internal/builtins/reserved.go
package builtins

// reserved lists the SLOP builtin names that must not shadow custom tool
// arg shorthand bindings. Order and case match slop/pkg/slop builtin registry.
var reserved = map[string]struct{}{
    "mem_save": {}, "mem_load": {}, "mem_list": {}, "mem_search": {}, "mem_info": {},
    "mem_delete": {}, "mem_clear": {},
    "store_set": {}, "store_get": {}, "store_list": {}, "store_clear": {},
    "execute_tool": {},
    "emit": {}, "json_parse": {}, "json_stringify": {},
    "map": {}, "filter": {}, "reduce": {}, "len": {},
    "http_get": {}, "http_post": {},
    "args": {},
}

func IsReservedBuiltin(name string) bool {
    _, ok := reserved[name]
    return ok
}
```

- [ ] **Step 3: Run and commit**

```bash
go test ./internal/builtins/... -v
git add internal/builtins/reserved.go internal/builtins/reserved_test.go
git commit -m "feat(builtins): list reserved builtin names for shorthand binding"
```

---

## Phase 4 — Meta-tool surface

### Task 11: customize_tools JSON schema

**Files:**
- Modify: `internal/server/schemas.go` — add `customizeToolsInputSchema`

- [ ] **Step 1: Define the schema inline**

Add the schema as a `*jsonschema.Schema` (match existing pattern). Required: `action` (enum of the 8 action names). Conditional requirements per action are documented in the description string (JSON Schema `oneOf` would balloon the schema; cleaner to validate at handler level).

Properties: `action`, `mcp`, `tool`, `description`, `params`, `scope`, `stale_only`, `name`, `inputSchema`, `body`, `keys`, `include_custom`, `data`, `overwrite`.

- [ ] **Step 2: Build and verify**

Run: `go build ./...`
Expected: clean

- [ ] **Step 3: Commit**

```bash
git add internal/server/schemas.go
git commit -m "feat(server): add customize_tools input schema"
```

---

### Task 12: set_override / remove_override / list_overrides handlers

**Files:**
- Create: `internal/server/customize_handler.go`
- Create: `internal/server/customize_handler_test.go`
- Modify: `internal/server/tools.go` — register `customize_tools`

- [ ] **Step 1: Write failing tests per action**

Tests cover: `set_override` persists + computes hash; upstream disconnected → error, nothing stored; `remove_override` with no scope removes from all; `list_overrides stale_only:true` recomputes hashes and returns only mismatches with `stale_source`.

- [ ] **Step 2: Implement action dispatch**

```go
// internal/server/customize_handler.go
func (s *Server) handleCustomizeTools(ctx context.Context, req *mcp.CallToolRequest, in CustomizeToolsInput) (*mcp.CallToolResult, output, error) {
    switch in.Action {
    case "set_override":
        return s.setOverride(ctx, in)
    case "remove_override":
        return s.removeOverride(ctx, in)
    case "list_overrides":
        return s.listOverrides(ctx, in)
    // ...
    default:
        return nil, nil, fmt.Errorf("unknown action: %q", in.Action)
    }
}
```

`setOverride`:
1. Require `mcp`, `tool`, `description`.
2. Ensure upstream connected: `s.registry.EnsureConnected(ctx, mcp)` — match the existing signature in `internal/registry/registry.go` which takes `context.Context` first.
3. Look up upstream tool to get current description + param descriptions.
4. `hash := overrides.ComputeHash(upstreamDesc, upstreamParams)`.
5. `s.overrideStore.SetOverride(scope, mcp+"."+tool, OverrideEntry{Description: desc, Params: params, SourceHash: hash})`.
6. Return summary `{ok:true, affected:1, entries:[{key, hash, scope}]}`.

`removeOverride` + `listOverrides` follow the store APIs built in Phase 2.

- [ ] **Step 3: Register the tool**

In `tools.go`, add:

```go
s.mcpServer.AddTool(
    &mcp.Tool{
        Name:        "customize_tools",
        Description: "Override descriptions + define custom tools. Actions: set_override, remove_override, list_overrides, define_custom, remove_custom, list_custom, export, import.",
        InputSchema: customizeToolsInputSchema,
    },
    s.wrapCustomizeTools,
)
```

Add `wrapCustomizeTools` following the existing wrapper pattern.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/server/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/customize_handler.go internal/server/customize_handler_test.go internal/server/tools.go
git commit -m "feat(server): add customize_tools set/remove/list override actions"
```

---

### Task 13: define_custom / remove_custom / list_custom handlers

**Files:**
- Modify: `internal/server/customize_handler.go` — add three more action branches
- Modify: `internal/server/customize_handler_test.go` — three more tests

- [ ] **Step 1: Failing tests**

Cover: define_custom stores tool; validates inputSchema (reject invalid JSON Schema); rejects name regex mismatch; rejects collision with existing meta-tool name; rejects collision with currently-connected MCP tool; reports shorthand collisions in response without blocking; recomputes dependency hashes.

- [ ] **Step 2: Implement**

```go
func (s *Server) defineCustom(ctx context.Context, in CustomizeToolsInput) (...) {
    if !customNameRegex.MatchString(in.Name) {
        return nil, nil, fmt.Errorf("name must match %s", customNameRegex)
    }
    if s.isMetaToolName(in.Name) || s.registry.ToolExists(in.Name) {
        return nil, nil, fmt.Errorf("name collides with existing tool")
    }
    if err := validateInputSchema(in.InputSchema); err != nil {
        return nil, nil, err
    }
    if len(in.Body) > bodyLimit() {
        return nil, nil, fmt.Errorf("body exceeds %d bytes", bodyLimit())
    }
    shorthandSkipped := detectShorthandCollisions(in.InputSchema)
    deps := extractDependencies(in.Body, s.registry) // best-effort parse
    tool := overrides.CustomTool{
        Description: in.Description,
        InputSchema: in.InputSchema,
        Body:        in.Body,
        DependsOn:   deps,
    }
    if err := s.overrideStore.SetCustom(scope, in.Name, tool); err != nil {
        return nil, nil, err
    }
    return ok(map[string]any{
        "action": "define_custom",
        "affected": 1,
        "entries": []any{map[string]any{"name": in.Name, "scope": scope}},
        "shorthand_skipped": shorthandSkipped,
    })
}
```

`customNameRegex = regexp.MustCompile("^[a-z][a-z0-9_]{0,63}$")`.
`bodyLimit` reads `SLOP_MAX_CUSTOM_BODY` env (default 65536).
`extractDependencies` can be a simple substring scan of `execute_tool(` and `mcp_name.tool_name(` forms; hashes pulled via the store.

- [ ] **Step 3: Run + commit**

```bash
go test ./internal/server/... -v
git add internal/server/customize_handler.go internal/server/customize_handler_test.go
git commit -m "feat(server): add customize_tools custom tool define/remove/list"
```

---

### Task 14: export + import handlers

**Files:**
- Create: `internal/overrides/pack.go`
- Create: `internal/overrides/pack_test.go`
- Modify: `internal/server/customize_handler.go` — two more action branches

- [ ] **Step 1: Failing tests for pack**

Cover: export by mcp expands to `keys: ["figma.*"]` for overrides + custom tools whose `depends_on` references figma; export by keys glob; round-trip (export → import into fresh store preserves entries); import with unknown schema_version errors; import with `overwrite:false` on existing key reports collision; per-bank atomicity on partial failure.

- [ ] **Step 2: Implement pack types**

```go
// internal/overrides/pack.go
type Pack struct {
    SchemaVersion int             `json:"schema_version"`
    ExportedAt    time.Time       `json:"exported_at"`
    Source        string          `json:"source"`
    Selector      any             `json:"selector,omitempty"`
    Overrides     []PackOverride  `json:"overrides,omitempty"`
    CustomTools   []PackCustom    `json:"custom_tools,omitempty"`
}

type PackOverride struct {
    Key         string            `json:"key"`
    Description string            `json:"description"`
    Params      map[string]string `json:"params,omitempty"`
    SourceHash  string            `json:"source_hash"`
}

type PackCustom struct {
    Name        string         `json:"name"`
    Description string         `json:"description"`
    InputSchema map[string]any `json:"inputSchema"`
    Body        string         `json:"body"`
    DependsOn   []Dependency   `json:"depends_on,omitempty"`
}

const PackSchemaVersion = 1

func (s *Store) Export(selector Selector) (Pack, error) { /* glob match across all scopes (no scope on wire) */ }
func (s *Store) Import(pack Pack, scope Scope, overwrite bool) (ImportReport, error) { /* atomic per bank */ }
```

- [ ] **Step 3: Hook into handler**

```go
case "export":
    pack, err := s.overrideStore.Export(selectorFrom(in))
    return okJSON(pack)
case "import":
    var pack overrides.Pack
    if err := json.Unmarshal([]byte(in.Data), &pack); err != nil {
        return errorFor("invalid pack: %w", err)
    }
    report, err := s.overrideStore.Import(pack, scopeFrom(in), in.Overwrite)
    return okJSON(report)
```

- [ ] **Step 4: Run + commit**

```bash
go test ./internal/overrides/... ./internal/server/... -v
git add internal/overrides/pack.go internal/overrides/pack_test.go internal/server/customize_handler.go
git commit -m "feat(overrides): add pack import/export + customize_tools actions"
```

---

### Task 15: manage_mcps list_stale_overrides shortcut

**Files:**
- Modify: `internal/server/handlers.go` — add subaction in `handleManageMCPs`
- Modify: `internal/server/handlers_test.go` — test it

- [ ] **Step 1: Failing test**

Call `manage_mcps action:list_stale_overrides`; expect same output shape as `customize_tools list_overrides stale_only:true`.

- [ ] **Step 2: Implement as thin delegate**

```go
case "list_stale_overrides":
    return s.listOverrides(ctx, CustomizeToolsInput{StaleOnly: true})
```

- [ ] **Step 3: Run + commit**

```bash
go test ./internal/server/... -v
git add internal/server/handlers.go internal/server/handlers_test.go
git commit -m "feat(server): add manage_mcps list_stale_overrides shortcut"
```

---

### Task 16: Wire store into server lifecycle

**Files:**
- Modify: `internal/server/server.go` — construct `overrides.Store`, pass to registry, close on shutdown

- [ ] **Step 1: Integration test**

Start a test server end-to-end, call `customize_tools set_override`, restart, verify override persists.

- [ ] **Step 2: Implement**

In `NewServer`:

```go
home, _ := os.UserHomeDir()
cwd, _ := os.Getwd()
opts := overrides.StoreOptions{
    UserRoot: filepath.Join(home, ".config", "slop-mcp", "memory", "_slop"),
}
if root, err := overrides.FindRepoRoot(cwd); err == nil {
    opts.ProjectRoot = filepath.Join(root, ".slop-mcp", "memory", "_slop")
    opts.LocalRoot   = filepath.Join(root, ".slop-mcp", "memory.local", "_slop")
}
store, err := overrides.OpenStore(opts)
if err != nil {
    return nil, err
}
s.overrideStore = store
s.registry.SetOverrideProvider(store)
```

On `Server.Shutdown`, call `s.overrideStore.Close()`.

- [ ] **Step 3: Run + commit**

```bash
go test ./internal/server/... -v
git add internal/server/server.go
git commit -m "feat(server): wire overrides store into server lifecycle"
```

---

## Phase 5 — Polish

### Task 17: Dogfood caveman-style meta-tool descriptions

**Files:**
- Modify: `internal/server/tools.go` — rewrite the `Description` string for every existing meta-tool
- Modify: `internal/server/schemas.go` — tighten param descriptions
- Create: `docs/internal/description-style.md` — style rules

- [ ] **Step 1: Write style doc**

Short doc: drop articles and filler, fragments OK, technical terms exact, examples preserved, enum values listed explicitly, param descriptions ≤80 chars where possible. Include 3-5 before/after examples.

- [ ] **Step 2: Rewrite descriptions**

Walk every `Description: "…"` in `tools.go` and param descriptions in `schemas.go`. Apply the style. Example transformations are in spec §7.

- [ ] **Step 3: Regenerate baseline — no tests broken**

Run: `go test ./... -v`
Expected: PASS (descriptions are strings, not asserted on in most tests)

If any test is asserting on the exact string: update the assertion.

- [ ] **Step 4: Commit**

```bash
git add internal/server/tools.go internal/server/schemas.go docs/internal/description-style.md
git commit -m "docs: dogfood caveman-style descriptions on meta-tools"
```

---

### Task 18: User docs page

**Files:**
- Create: `docs/docs/concepts/customization.md`
- Modify: `docs/sidebars.js` — link the page

- [ ] **Step 1: Draft docs page**

Sections: Why customize? Override example (Figma). Custom tool example (batch add subtasks). Scope tiers. Import/export flow. Staleness handling. Reserved banks.

Use real JSON blocks for each `customize_tools` action.

- [ ] **Step 2: Verify docs build**

Run: `cd docs && npm run build`
Expected: clean build, no dead link warnings.

- [ ] **Step 3: Commit**

```bash
git add docs/docs/concepts/customization.md docs/sidebars.js
git commit -m "docs: add customization concepts page"
```

---

### Task 19: Version bump + CHANGELOG

**Files:**
- Modify: `internal/server/server.go` — `serverVersion = "0.14.0"`
- Modify: `CHANGELOG.md` — new section

- [ ] **Step 1: Update version**

```go
const serverVersion = "0.14.0"
```

- [ ] **Step 2: Add CHANGELOG entry**

```markdown
## [0.14.0] - 2026-04-16

### Added
- `customize_tools` meta-tool (9th meta-tool) for per-tool description overrides, agent-defined SLOP-backed custom tools, and scope-aware import/export.
- Three storage scopes (user, project, local) for reserved `_slop.*` memory banks.
- Hash-tied staleness detection: overrides flag `stale: true` when upstream descriptions change.
- Custom tool execution through the existing SLOP engine with arg validation, recursion depth guard (default 16), and body size limit (`SLOP_MAX_CUSTOM_BODY`, default 64 KB).
- `manage_mcps list_stale_overrides` shortcut.

### Changed
- Meta-tool descriptions rewritten in caveman-style for token efficiency (see `docs/internal/description-style.md`).
- Registry ranking index now uses `atomic.Pointer` for lock-free reads during rebuilds.
- `mem_save`, `mem_delete`, `mem_clear`, and memory-cli reject writes to banks prefixed with `_slop.`.

### Internal
- New `internal/overrides/` package owns storage, hashing, scope merge, and import/export.
- Per-bank background flusher with single-slot coalescing removes lock-over-I/O antipattern.
```

- [ ] **Step 3: Commit**

```bash
git add internal/server/server.go CHANGELOG.md
git commit -m "chore: bump serverVersion to 0.14.0 and update CHANGELOG"
```

---

### Task 20: End-to-end integration test

**Files:**
- Create: `internal/server/customize_e2e_test.go`

- [ ] **Step 1: Write an e2e scenario test**

```go
//go:build integration

package server

// TestE2E_CustomizeTools_FullFlow exercises the full flow:
// 1. Start server with mock MCP.
// 2. set_override; search_tools returns override.
// 3. export, clear state, import, override still present.
// 4. define_custom, execute_tool via _custom routes through SLOP, returns result.
// 5. Mock MCP description changes; list_overrides stale_only:true flags the entry.
// 6. remove_override; get_metadata returns upstream again.
```

- [ ] **Step 2: Run**

Run: `go test -tags integration ./internal/server/... -run TestE2E_CustomizeTools -v`
Expected: PASS

- [ ] **Step 3: Run full unit suite + lint**

```bash
make test
make lint
```
Expected: PASS, no new lint findings.

- [ ] **Step 4: Commit**

```bash
git add internal/server/customize_e2e_test.go
git commit -m "test(server): add end-to-end customize_tools integration scenario"
```

---

## Final Verification

- [ ] `make test` passes
- [ ] `make test-integration` passes (requires npx)
- [ ] `make lint` passes
- [ ] `go build -tags mcp_go_client_oauth ./...` clean
- [ ] `docs/docs/concepts/customization.md` renders via `cd docs && npm run build`
- [ ] `CHANGELOG.md` entry present for 0.14.0
- [ ] `serverVersion` bumped in `internal/server/server.go`
- [ ] Existing pre-existing test failures (documented in memory) unchanged — this plan does NOT fix them and must not newly break them
- [ ] Single PR ready; branch squashed/rebased on main
