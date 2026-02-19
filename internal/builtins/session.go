package builtins

import (
	"fmt"
	"sync"

	"github.com/standardbeagle/slop/pkg/slop"
)

// SessionStore is a thread-safe key-value store for SLOP session memory.
// It overrides SLOP's built-in store_* functions which use a package-level
// unsynchronized map. Multiple concurrent run_slop calls share this store.
type SessionStore struct {
	mu   sync.RWMutex
	data map[string]slop.Value
}

// NewSessionStore creates a new empty SessionStore.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		data: make(map[string]slop.Value),
	}
}

// RegisterSession overrides the SLOP runtime's store_* functions with
// thread-safe versions backed by the given SessionStore.
func RegisterSession(rt *slop.Runtime, store *SessionStore) {
	rt.RegisterBuiltin("store_get", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("store_get: requires key argument")
		}
		key, ok := args[0].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("store_get: key must be a string")
		}

		store.mu.RLock()
		val, exists := store.data[key.Value]
		store.mu.RUnlock()

		if !exists {
			return slop.NewNullValue(), nil
		}
		return val, nil
	})

	rt.RegisterBuiltin("store_set", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("store_set: requires key and value arguments")
		}
		key, ok := args[0].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("store_set: key must be a string")
		}

		store.mu.Lock()
		store.data[key.Value] = args[1]
		store.mu.Unlock()

		return slop.NewNullValue(), nil
	})

	rt.RegisterBuiltin("store_delete", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("store_delete: requires key argument")
		}
		key, ok := args[0].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("store_delete: key must be a string")
		}

		store.mu.Lock()
		delete(store.data, key.Value)
		store.mu.Unlock()

		return slop.NewNullValue(), nil
	})

	rt.RegisterBuiltin("store_exists", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("store_exists: requires key argument")
		}
		key, ok := args[0].(*slop.StringValue)
		if !ok {
			return nil, fmt.Errorf("store_exists: key must be a string")
		}

		store.mu.RLock()
		_, exists := store.data[key.Value]
		store.mu.RUnlock()

		return &slop.BoolValue{Value: exists}, nil
	})

	rt.RegisterBuiltin("store_keys", func(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
		store.mu.RLock()
		keys := make([]any, 0, len(store.data))
		for k := range store.data {
			keys = append(keys, k)
		}
		store.mu.RUnlock()

		return slop.GoToValue(keys), nil
	})
}
