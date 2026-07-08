package overrides

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/atomicfile"
	"github.com/standardbeagle/slop-mcp/internal/filelock"
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
	// touched tracks keys this process has written or deleted, keyed by
	// "<scope>:<bank>". On flush, the on-disk file is re-read and only
	// touched keys are overwritten (or deleted), so entries written by other
	// processes survive. Guarded by mu.
	touched map[string]map[string]bool
}

// OpenStore creates and warms the store from disk.
func OpenStore(opts StoreOptions) (*Store, error) {
	s := &Store{
		opts:      opts,
		overrides: map[Scope]map[string]OverrideEntry{},
		custom:    map[Scope]map[string]CustomTool{},
		touched:   map[string]map[string]bool{},
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

	if err := readOverrides(filepath.Join(root, BankOverrides+".json"), s.overrides[scope]); err != nil {
		return err
	}
	return readCustom(filepath.Join(root, BankCustomTools+".json"), s.custom[scope])
}

func readOverrides(path string, dst map[string]OverrideEntry) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var m map[string]OverrideEntry
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	for k, v := range m {
		dst[k] = v
	}
	return nil
}

func readCustom(path string, dst map[string]CustomTool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var m map[string]CustomTool
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	for k, v := range m {
		dst[k] = v
	}
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
	s.markTouched(scope, BankOverrides, key)
	s.mu.Unlock()

	// Write synchronously and surface the error: the customize handler reports
	// success to the agent based on this return value, so a fire-and-forget
	// flush that failed later would silently lose the customization.
	return s.writeBank(scope, BankOverrides)
}

// GetOverride returns the merged (scope-winning) override entry for a key.
func (s *Store) GetOverride(key string) (OverrideEntry, bool) {
	s.mu.RLock()
	per := map[Scope]*OverrideEntry{}
	for scope, m := range s.overrides {
		if e, ok := m[key]; ok {
			cp := e
			per[scope] = &cp
		}
	}
	s.mu.RUnlock()

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
	for sc := range touched {
		s.markTouched(sc, BankOverrides, key)
	}
	s.mu.Unlock()

	for sc := range touched {
		if err := s.writeBank(sc, BankOverrides); err != nil {
			return n, err
		}
	}
	return n, nil
}

// SetCustom stores or replaces a custom tool at the given scope.
func (s *Store) SetCustom(scope Scope, name string, ct CustomTool) error {
	if s.rootFor(scope) == "" {
		return fmt.Errorf("scope %s unavailable", scope)
	}
	ct.Scope = scope
	ct.UpdatedAt = time.Now()

	s.mu.Lock()
	if s.custom[scope] == nil {
		s.custom[scope] = map[string]CustomTool{}
	}
	s.custom[scope][name] = ct
	s.markTouched(scope, BankCustomTools, name)
	s.mu.Unlock()

	// Synchronous write so a failed persist is reported instead of silently
	// losing the custom tool (see SetOverride).
	return s.writeBank(scope, BankCustomTools)
}

// GetCustom returns the merged (scope-winning) custom tool.
func (s *Store) GetCustom(name string) (CustomTool, bool) {
	s.mu.RLock()
	per := map[Scope]*CustomTool{}
	for scope, m := range s.custom {
		if e, ok := m[name]; ok {
			cp := e
			per[scope] = &cp
		}
	}
	s.mu.RUnlock()

	merged := MergeCustom(per)
	if merged == nil {
		return CustomTool{}, false
	}
	return *merged, true
}

// RemoveCustom deletes from the given scope, or all scopes if scope=="".
func (s *Store) RemoveCustom(scope Scope, name string) (int, error) {
	s.mu.Lock()
	n := 0
	touched := map[Scope]bool{}
	if scope == "" {
		for sc, m := range s.custom {
			if _, ok := m[name]; ok {
				delete(m, name)
				n++
				touched[sc] = true
			}
		}
	} else {
		if m := s.custom[scope]; m != nil {
			if _, ok := m[name]; ok {
				delete(m, name)
				n = 1
				touched[scope] = true
			}
		}
	}
	for sc := range touched {
		s.markTouched(sc, BankCustomTools, name)
	}
	s.mu.Unlock()

	for sc := range touched {
		if err := s.writeBank(sc, BankCustomTools); err != nil {
			return n, err
		}
	}
	return n, nil
}

// ListOverrides returns a snapshot by scope of all override entries.
func (s *Store) ListOverrides() map[Scope]map[string]OverrideEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[Scope]map[string]OverrideEntry{}
	for scope, m := range s.overrides {
		cp := make(map[string]OverrideEntry, len(m))
		for k, v := range m {
			cp[k] = v
		}
		out[scope] = cp
	}
	return out
}

// ListCustom returns a snapshot by scope of all custom tools.
func (s *Store) ListCustom() map[Scope]map[string]CustomTool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[Scope]map[string]CustomTool{}
	for scope, m := range s.custom {
		cp := make(map[string]CustomTool, len(m))
		for k, v := range m {
			cp[k] = v
		}
		out[scope] = cp
	}
	return out
}

// Close is retained for API compatibility. Writes are now synchronous (see
// SetOverride/SetCustom/RemoveOverride/RemoveCustom), so there is nothing to
// flush at shutdown.
func (s *Store) Close() error {
	return nil
}

// markTouched records that this process modified (wrote or deleted) a key in
// the given scope+bank. Caller must hold s.mu.
func (s *Store) markTouched(scope Scope, bank, key string) {
	fkey := string(scope) + ":" + bank
	if s.touched[fkey] == nil {
		s.touched[fkey] = map[string]bool{}
	}
	s.touched[fkey][key] = true
}

// writeBank snapshots the bank under a brief read lock, then writes without
// holding any lock. Before writing, the current on-disk file is re-read and
// merged: keys touched by this process win (including deletions); untouched
// keys keep their on-disk value, so entries written by other processes since
// startup usually survive the flush. A missing or unreadable file falls back
// to the in-memory snapshot alone.
//
// Known cross-process limitations: the read-merge-write itself is not
// protected by a file lock, so two processes flushing the same bank
// concurrently can still lose the other's just-written keys (merge is
// best-effort, not a guarantee), and entries written by other processes
// become visible on disk but are not folded into this process's in-memory
// view.
func (s *Store) writeBank(scope Scope, bank string) error {
	root := s.rootFor(scope)
	if root == "" {
		return errors.New("scope root not set")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	path := filepath.Join(root, bank+".json")

	// Cross-process lock across the read-merge-write so a concurrent flush (this
	// process or another) cannot clobber the other's just-written keys.
	unlock, err := filelock.Lock(path)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()

	var data []byte

	s.mu.RLock()
	fkey := string(scope) + ":" + bank
	touched := make(map[string]bool, len(s.touched[fkey]))
	for k := range s.touched[fkey] {
		touched[k] = true
	}
	switch bank {
	case BankOverrides:
		mem := make(map[string]OverrideEntry, len(s.overrides[scope]))
		for k, v := range s.overrides[scope] {
			mem[k] = v
		}
		s.mu.RUnlock()

		merged := map[string]OverrideEntry{}
		if err := readOverrides(path, merged); err != nil {
			merged = mem
		} else {
			for k := range touched {
				if v, ok := mem[k]; ok {
					merged[k] = v
				} else {
					delete(merged, k)
				}
			}
		}
		data, err = json.MarshalIndent(merged, "", "  ")
	case BankCustomTools:
		mem := make(map[string]CustomTool, len(s.custom[scope]))
		for k, v := range s.custom[scope] {
			mem[k] = v
		}
		s.mu.RUnlock()

		merged := map[string]CustomTool{}
		if err := readCustom(path, merged); err != nil {
			merged = mem
		} else {
			for k := range touched {
				if v, ok := mem[k]; ok {
					merged[k] = v
				} else {
					delete(merged, k)
				}
			}
		}
		data, err = json.MarshalIndent(merged, "", "  ")
	default:
		s.mu.RUnlock()
		return fmt.Errorf("unknown bank: %s", bank)
	}
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(path, data, 0644)
}
