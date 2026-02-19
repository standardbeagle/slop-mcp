package builtins

import (
	"sync"
	"testing"

	"github.com/standardbeagle/slop/pkg/slop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionStore_BasicOperations(t *testing.T) {
	store := NewSessionStore()
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterSession(rt, store)

	// store_set then store_get
	result, err := rt.Execute(`store_set("k1", 42)
store_get("k1")`)
	require.NoError(t, err)
	assert.Equal(t, int64(42), result.(*slop.IntValue).Value)

	// store_exists
	result, err = rt.Execute(`store_exists("k1")`)
	require.NoError(t, err)
	assert.Equal(t, true, result.(*slop.BoolValue).Value)

	// store_delete
	result, err = rt.Execute(`store_delete("k1")
store_exists("k1")`)
	require.NoError(t, err)
	assert.Equal(t, false, result.(*slop.BoolValue).Value)
}

func TestSessionStore_GetMissing(t *testing.T) {
	store := NewSessionStore()
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterSession(rt, store)

	result, err := rt.Execute(`store_get("nonexistent")`)
	require.NoError(t, err)
	_, isNone := result.(*slop.NullValue)
	assert.True(t, isNone, "missing key should return none")
}

func TestSessionStore_Keys(t *testing.T) {
	store := NewSessionStore()
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterSession(rt, store)

	_, err := rt.Execute(`store_set("a", 1)
store_set("b", 2)
store_set("c", 3)`)
	require.NoError(t, err)

	result, err := rt.Execute(`len(store_keys())`)
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.(*slop.IntValue).Value)
}

func TestSessionStore_ConcurrentAccess(t *testing.T) {
	store := NewSessionStore()
	const goroutines = 50
	const ops = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			rt := slop.NewRuntime()
			defer rt.Close()
			RegisterSession(rt, store)

			for j := 0; j < ops; j++ {
				key := &slop.StringValue{Value: "key"}
				val := &slop.IntValue{Value: int64(id*ops + j)}

				// Write
				store.mu.Lock()
				store.data[key.Value] = val
				store.mu.Unlock()

				// Read
				store.mu.RLock()
				_ = store.data[key.Value]
				store.mu.RUnlock()
			}
		}(i)
	}

	wg.Wait()
	// If we got here without a race condition panic, the test passes
}

func TestSessionStore_PersistsAcrossRuntimes(t *testing.T) {
	store := NewSessionStore()

	// First runtime writes
	rt1 := slop.NewRuntime()
	RegisterSession(rt1, store)
	_, err := rt1.Execute(`store_set("shared", "hello")`)
	require.NoError(t, err)
	rt1.Close()

	// Second runtime reads
	rt2 := slop.NewRuntime()
	RegisterSession(rt2, store)
	result, err := rt2.Execute(`store_get("shared")`)
	require.NoError(t, err)
	assert.Equal(t, "hello", result.(*slop.StringValue).Value)
	rt2.Close()
}
