package builtins

import (
	"fmt"
	"strings"
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

// copyValue returns a deep copy of a SLOP value by round-tripping through its
// native Go representation. This isolates stored values from later mutation by
// the caller (and returned values from mutation of the stored copy), so
// concurrent scripts sharing the session store cannot race on a shared
// *MapValue / *ListValue. Scalars round-trip exactly; non-data values
// (functions, errors) are not meaningful to share across runtimes anyway.
func copyValue(v slop.Value) slop.Value {
	return slop.GoToValue(slop.ValueToGo(v))
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
			// Upstream store_get accepts an optional default; return it when the
			// key is absent instead of null.
			if len(args) >= 2 {
				return args[1], nil
			}
			return slop.NewNullValue(), nil
		}
		return copyValue(val), nil
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
		store.data[key.Value] = copyValue(args[1])
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
		// Upstream store_keys accepts an optional prefix filter.
		prefix := ""
		if len(args) >= 1 {
			if sv, ok := args[0].(*slop.StringValue); ok {
				prefix = sv.Value
			}
		}

		store.mu.RLock()
		keys := make([]any, 0, len(store.data))
		for k := range store.data {
			if prefix == "" || strings.HasPrefix(k, prefix) {
				keys = append(keys, k)
			}
		}
		store.mu.RUnlock()

		return slop.GoToValue(keys), nil
	})
}
