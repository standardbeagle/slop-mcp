package builtins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/standardbeagle/slop/pkg/slop"
)

// MemoryStore provides disk-backed persistent memory for SLOP scripts.
// Data is stored as JSON files in ~/.config/slop-mcp/memory/<bank>.json,
// compatible with the memory-cli data format.
type MemoryStore struct {
	mu      sync.Mutex
	baseDir string
}

// memoryBank mirrors the memory-cli Bank struct for JSON compatibility.
type memoryBank struct {
	Meta    memoryBankMeta          `json:"_meta"`
	Entries map[string]*memoryEntry `json:"entries"`
}

type memoryBankMeta struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type memoryEntry struct {
	Value       any       `json:"value"`
	Description string    `json:"description,omitempty"`
	Schema      any       `json:"schema,omitempty"`
	Size        int       `json:"size,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// NewMemoryStore creates a MemoryStore using the default directory.
func NewMemoryStore() *MemoryStore {
	home, _ := os.UserHomeDir()
	return &MemoryStore{
		baseDir: filepath.Join(home, ".config", "slop-mcp", "memory"),
	}
}

// NewMemoryStoreWithDir creates a MemoryStore using a custom directory.
func NewMemoryStoreWithDir(dir string) *MemoryStore {
	return &MemoryStore{
		baseDir: dir,
	}
}

func (m *MemoryStore) bankPath(bank string) string {
	return filepath.Join(m.baseDir, bank+".json")
}

func (m *MemoryStore) loadBank(bank string) (*memoryBank, error) {
	path := m.bankPath(bank)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var b memoryBank
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("corrupt bank file %s: %w", bank, err)
	}
	return &b, nil
}

func (m *MemoryStore) saveBank(bank string, b *memoryBank) error {
	if err := os.MkdirAll(m.baseDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	// Atomic write: write to temp file then rename
	tmpPath := m.bankPath(bank) + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, m.bankPath(bank))
}

// RegisterMemory registers persistent memory functions with the SLOP runtime.
func RegisterMemory(rt *slop.Runtime, store *MemoryStore) {
	rt.RegisterBuiltin("mem_save", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		if len(args) < 3 {
			return nil, fmt.Errorf("mem_save: requires bank, key, value arguments")
		}
		bankName, ok := args[0].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("mem_save: bank must be a string")
		}
		key, ok := args[1].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("mem_save: key must be a string")
		}
		value := slop.ValueToGo(args[2])

		store.mu.Lock()
		defer store.mu.Unlock()

		b, err := store.loadBank(bankName.Value)
		if err != nil {
			return nil, fmt.Errorf("mem_save: %w", err)
		}

		now := time.Now()
		if b == nil {
			b = &memoryBank{
				Meta: memoryBankMeta{
					Version:   1,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Entries: make(map[string]*memoryEntry),
			}
		}

		existing := b.Entries[key.Value]
		entry := &memoryEntry{
			Value:     value,
			UpdatedAt: now,
		}
		if existing != nil {
			entry.CreatedAt = existing.CreatedAt
			// Preserve existing metadata if not provided in kwargs
			entry.Description = existing.Description
			entry.Schema = existing.Schema
		} else {
			entry.CreatedAt = now
		}

		// Apply kwargs: description
		if v, ok := kwargs["description"]; ok {
			if sv, ok := v.(*slop.StringValue); ok {
				entry.Description = sv.Value
			}
		}
		// Apply kwargs: schema
		if v, ok := kwargs["schema"]; ok {
			entry.Schema = slop.ValueToGo(v)
		}

		// Auto-compute size from serialized value
		if valBytes, err := json.Marshal(value); err == nil {
			entry.Size = len(valBytes)
		}

		b.Entries[key.Value] = entry
		b.Meta.UpdatedAt = now

		if err := store.saveBank(bankName.Value, b); err != nil {
			return nil, fmt.Errorf("mem_save: %w", err)
		}
		return slop.NewNullValue(), nil
	})

	rt.RegisterBuiltin("mem_load", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("mem_load: requires bank, key arguments")
		}
		bankName, ok := args[0].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("mem_load: bank must be a string")
		}
		key, ok := args[1].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("mem_load: key must be a string")
		}

		store.mu.Lock()
		b, err := store.loadBank(bankName.Value)
		store.mu.Unlock()

		if err != nil {
			return nil, fmt.Errorf("mem_load: %w", err)
		}
		if b == nil {
			if len(args) > 2 {
				return args[2], nil
			}
			return slop.NewNullValue(), nil
		}

		entry, exists := b.Entries[key.Value]
		if !exists {
			if len(args) > 2 {
				return args[2], nil
			}
			return slop.NewNullValue(), nil
		}

		return slop.GoToValue(entry.Value), nil
	})

	rt.RegisterBuiltin("mem_delete", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("mem_delete: requires bank, key arguments")
		}
		bankName, ok := args[0].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("mem_delete: bank must be a string")
		}
		key, ok := args[1].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("mem_delete: key must be a string")
		}

		store.mu.Lock()
		defer store.mu.Unlock()

		b, err := store.loadBank(bankName.Value)
		if err != nil {
			return nil, fmt.Errorf("mem_delete: %w", err)
		}
		if b == nil {
			return slop.NewNullValue(), nil
		}

		delete(b.Entries, key.Value)
		b.Meta.UpdatedAt = time.Now()

		if err := store.saveBank(bankName.Value, b); err != nil {
			return nil, fmt.Errorf("mem_delete: %w", err)
		}
		return slop.NewNullValue(), nil
	})

	rt.RegisterBuiltin("mem_keys", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("mem_keys: requires bank argument")
		}
		bankName, ok := args[0].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("mem_keys: bank must be a string")
		}

		store.mu.Lock()
		b, err := store.loadBank(bankName.Value)
		store.mu.Unlock()

		if err != nil {
			return nil, fmt.Errorf("mem_keys: %w", err)
		}
		if b == nil {
			return slop.GoToValue([]any{}), nil
		}

		keys := make([]any, 0, len(b.Entries))
		for k := range b.Entries {
			keys = append(keys, k)
		}
		return slop.GoToValue(keys), nil
	})

	rt.RegisterBuiltin("mem_banks", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		store.mu.Lock()
		defer store.mu.Unlock()

		entries, err := os.ReadDir(store.baseDir)
		if err != nil {
			if os.IsNotExist(err) {
				return slop.GoToValue([]any{}), nil
			}
			return nil, fmt.Errorf("mem_banks: %w", err)
		}

		banks := make([]any, 0)
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			name := entry.Name()[:len(entry.Name())-5] // strip .json
			banks = append(banks, name)
		}
		return slop.GoToValue(banks), nil
	})

	rt.RegisterBuiltin("mem_info", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("mem_info: requires bank, key arguments")
		}
		bankName, ok := args[0].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("mem_info: bank must be a string")
		}
		key, ok := args[1].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("mem_info: key must be a string")
		}

		store.mu.Lock()
		b, err := store.loadBank(bankName.Value)
		store.mu.Unlock()

		if err != nil {
			return nil, fmt.Errorf("mem_info: %w", err)
		}
		if b == nil {
			return slop.NewNullValue(), nil
		}

		entry, exists := b.Entries[key.Value]
		if !exists {
			return slop.NewNullValue(), nil
		}

		result := map[string]any{
			"key":        key.Value,
			"description": entry.Description,
			"size":       entry.Size,
			"created_at": entry.CreatedAt.Format(time.RFC3339),
			"updated_at": entry.UpdatedAt.Format(time.RFC3339),
		}
		if entry.Schema != nil {
			result["schema"] = entry.Schema
		}
		return slop.GoToValue(result), nil
	})

	rt.RegisterBuiltin("mem_list", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("mem_list: requires bank argument")
		}
		bankName, ok := args[0].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("mem_list: bank must be a string")
		}

		pattern := ""
		if v, ok := kwargs["pattern"]; ok {
			if sv, ok := v.(*slop.StringValue); ok {
				pattern = sv.Value
			}
		}

		store.mu.Lock()
		b, err := store.loadBank(bankName.Value)
		store.mu.Unlock()

		if err != nil {
			return nil, fmt.Errorf("mem_list: %w", err)
		}
		if b == nil {
			return slop.GoToValue([]any{}), nil
		}

		// Collect and sort keys for stable output
		keys := make([]string, 0, len(b.Entries))
		for k := range b.Entries {
			if pattern != "" {
				matched, _ := filepath.Match(pattern, k)
				if !matched {
					continue
				}
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)

		results := make([]any, 0, len(keys))
		for _, k := range keys {
			e := b.Entries[k]
			results = append(results, map[string]any{
				"key":         k,
				"description": e.Description,
				"size":        e.Size,
				"updated_at":  e.UpdatedAt.Format(time.RFC3339),
			})
		}
		return slop.GoToValue(results), nil
	})

	rt.RegisterBuiltin("mem_search", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("mem_search: requires query argument")
		}
		querySV, ok := args[0].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("mem_search: query must be a string")
		}
		query := strings.ToLower(querySV.Value)

		bankFilter := ""
		if v, ok := kwargs["bank"]; ok {
			if sv, ok := v.(*slop.StringValue); ok {
				bankFilter = sv.Value
			}
		}

		includeValues := false
		if v, ok := kwargs["include_values"]; ok {
			if bv, ok := v.(*slop.BoolValue); ok {
				includeValues = bv.Value
			}
		}

		store.mu.Lock()
		defer store.mu.Unlock()

		// Determine which banks to search
		var bankNames []string
		if bankFilter != "" {
			bankNames = []string{bankFilter}
		} else {
			entries, err := os.ReadDir(store.baseDir)
			if err != nil {
				if os.IsNotExist(err) {
					return slop.GoToValue([]any{}), nil
				}
				return nil, fmt.Errorf("mem_search: %w", err)
			}
			for _, entry := range entries {
				if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
					continue
				}
				bankNames = append(bankNames, entry.Name()[:len(entry.Name())-5])
			}
		}
		sort.Strings(bankNames)

		var results []any
		for _, bn := range bankNames {
			b, err := store.loadBank(bn)
			if err != nil || b == nil {
				continue
			}

			for key, entry := range b.Entries {
				matched := false

				// Match key name
				if strings.Contains(strings.ToLower(key), query) {
					matched = true
				}

				// Match description
				if !matched && strings.Contains(strings.ToLower(entry.Description), query) {
					matched = true
				}

				// Match serialized value if include_values
				if !matched && includeValues {
					if valBytes, err := json.Marshal(entry.Value); err == nil {
						if strings.Contains(strings.ToLower(string(valBytes)), query) {
							matched = true
						}
					}
				}

				if matched {
					m := map[string]any{
						"bank":        bn,
						"key":         key,
						"description": entry.Description,
						"size":        entry.Size,
					}
					if includeValues {
						m["value"] = entry.Value
					}
					results = append(results, m)
				}
			}
		}
		return slop.GoToValue(results), nil
	})
}
