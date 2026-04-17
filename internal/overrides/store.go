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

	fmu      sync.Mutex
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
	s.mu.Unlock()

	s.scheduleFlush(scope, BankOverrides)
	return nil
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
	s.mu.Unlock()

	for sc := range touched {
		s.scheduleFlush(sc, BankOverrides)
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
	s.mu.Unlock()

	s.scheduleFlush(scope, BankCustomTools)
	return nil
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
	s.mu.Unlock()

	for sc := range touched {
		s.scheduleFlush(sc, BankCustomTools)
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

// Close flushes all pending writes and stops background goroutines.
func (s *Store) Close() error {
	s.fmu.Lock()
	flushers := make([]*flusher, 0, len(s.flushers))
	for _, f := range s.flushers {
		flushers = append(flushers, f)
	}
	s.flushers = map[string]*flusher{}
	s.fmu.Unlock()

	for _, f := range flushers {
		f.close()
	}
	return nil
}

// scheduleFlush enqueues a flush for the given scope+bank.
func (s *Store) scheduleFlush(scope Scope, bank string) {
	fkey := string(scope) + ":" + bank
	s.fmu.Lock()
	f, ok := s.flushers[fkey]
	if !ok {
		f = newFlusher(fkey, func() error { return s.writeBank(scope, bank) })
		s.flushers[fkey] = f
	}
	s.fmu.Unlock()
	f.markDirty()
}

// writeBank snapshots the bank under a brief read lock, then writes without holding any lock.
func (s *Store) writeBank(scope Scope, bank string) error {
	root := s.rootFor(scope)
	if root == "" {
		return errors.New("scope root not set")
	}

	var data []byte
	var err error

	s.mu.RLock()
	switch bank {
	case BankOverrides:
		cp := make(map[string]OverrideEntry, len(s.overrides[scope]))
		for k, v := range s.overrides[scope] {
			cp[k] = v
		}
		s.mu.RUnlock()
		data, err = json.MarshalIndent(cp, "", "  ")
	case BankCustomTools:
		cp := make(map[string]CustomTool, len(s.custom[scope]))
		for k, v := range s.custom[scope] {
			cp[k] = v
		}
		s.mu.RUnlock()
		data, err = json.MarshalIndent(cp, "", "  ")
	default:
		s.mu.RUnlock()
		return fmt.Errorf("unknown bank: %s", bank)
	}
	if err != nil {
		return err
	}

	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	path := filepath.Join(root, bank+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
