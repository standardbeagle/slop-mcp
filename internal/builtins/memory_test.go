package builtins

import (
	"testing"

	"github.com/standardbeagle/slop/pkg/slop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMemoryRuntime(t *testing.T) (*slop.Runtime, *MemoryStore) {
	t.Helper()
	store := NewMemoryStoreWithDir(t.TempDir())
	rt := slop.NewRuntime()
	RegisterMemory(rt, store)
	return rt, store
}

func TestMemoryStore_SaveAndLoad(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("test", "k1", 42)`)
	require.NoError(t, err)

	result, err := rt.Execute(`mem_load("test", "k1")`)
	require.NoError(t, err)
	// JSON round-trip converts int to float64
	assert.Equal(t, float64(42), result.(*slop.NumberValue).Value)
}

func TestMemoryStore_LoadMissing(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	// Missing bank returns none
	result, err := rt.Execute(`mem_load("missing", "k1")`)
	require.NoError(t, err)
	_, isNone := result.(*slop.NullValue)
	assert.True(t, isNone, "missing bank should return none")

	// Missing key with default
	result, err = rt.Execute(`mem_load("missing", "k1", "default_val")`)
	require.NoError(t, err)
	assert.Equal(t, "default_val", result.(*slop.StringValue).Value)
}

func TestMemoryStore_Delete(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("test", "k1", "val")
mem_delete("test", "k1")`)
	require.NoError(t, err)

	result, err := rt.Execute(`mem_load("test", "k1")`)
	require.NoError(t, err)
	_, isNone := result.(*slop.NullValue)
	assert.True(t, isNone, "deleted key should return none")
}

func TestMemoryStore_Keys(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("test", "a", 1)
mem_save("test", "b", 2)
mem_save("test", "c", 3)`)
	require.NoError(t, err)

	result, err := rt.Execute(`len(mem_keys("test"))`)
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.(*slop.IntValue).Value)
}

func TestMemoryStore_Banks(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("bank1", "k", 1)
mem_save("bank2", "k", 2)`)
	require.NoError(t, err)

	result, err := rt.Execute(`len(mem_banks())`)
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.(*slop.IntValue).Value)
}

func TestMemoryStore_BanksEmpty(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	result, err := rt.Execute(`len(mem_banks())`)
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.(*slop.IntValue).Value)
}

func TestMemoryStore_PersistsAcrossRuntimes(t *testing.T) {
	store := NewMemoryStoreWithDir(t.TempDir())

	// First runtime writes
	rt1 := slop.NewRuntime()
	RegisterMemory(rt1, store)
	_, err := rt1.Execute(`mem_save("persist", "greeting", "hello")`)
	require.NoError(t, err)
	rt1.Close()

	// Second runtime reads
	rt2 := slop.NewRuntime()
	RegisterMemory(rt2, store)
	result, err := rt2.Execute(`mem_load("persist", "greeting")`)
	require.NoError(t, err)
	assert.Equal(t, "hello", result.(*slop.StringValue).Value)
	rt2.Close()
}

func TestMemoryStore_ComplexValues(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("test", "map", {"a": 1, "b": [2, 3]})`)
	require.NoError(t, err)

	result, err := rt.Execute(`mem_load("test", "map")`)
	require.NoError(t, err)
	// Should come back as a map
	assert.NotNil(t, result)
}

func TestMemoryStore_DeleteNonexistent(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	// Deleting from non-existent bank should not error
	_, err := rt.Execute(`mem_delete("missing", "k1")`)
	assert.NoError(t, err)
}
