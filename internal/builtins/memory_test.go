package builtins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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

// mapGet extracts a Go value from a MapValue by key.
func mapGet(m *slop.MapValue, key string) any {
	v, _ := m.Get(key)
	return slop.ValueToGo(v)
}

// mapGetVal extracts a SLOP Value from a MapValue by key.
func mapGetVal(m *slop.MapValue, key string) slop.Value {
	v, _ := m.Get(key)
	return v
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

func TestMemoryStore_SaveWithDescription(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("test", "k1", 42, description: "my number")`)
	require.NoError(t, err)

	result, err := rt.Execute(`mem_info("test", "k1")`)
	require.NoError(t, err)

	m := result.(*slop.MapValue)
	desc := mapGet(m, "description")
	assert.Equal(t, "my number", desc)
}

func TestMemoryStore_SaveWithSchema(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("test", "k1", {"name": "alice"}, schema: {"type": "object", "fields": ["name"]})`)
	require.NoError(t, err)

	result, err := rt.Execute(`mem_info("test", "k1")`)
	require.NoError(t, err)

	m := result.(*slop.MapValue)
	schema := mapGet(m, "schema")
	schemaMap, ok := schema.(map[string]any)
	require.True(t, ok, "schema should be a map")
	assert.Equal(t, "object", schemaMap["type"])
}

func TestMemoryStore_SavePreservesMetadata(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	// Save with description
	_, err := rt.Execute(`mem_save("test", "k1", 1, description: "original desc")`)
	require.NoError(t, err)

	// Re-save without description kwarg — should preserve
	_, err = rt.Execute(`mem_save("test", "k1", 2)`)
	require.NoError(t, err)

	result, err := rt.Execute(`mem_info("test", "k1")`)
	require.NoError(t, err)

	m := result.(*slop.MapValue)
	desc := mapGet(m, "description")
	assert.Equal(t, "original desc", desc)
}

func TestMemoryStore_Info(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("test", "k1", "hello", description: "greeting")`)
	require.NoError(t, err)

	result, err := rt.Execute(`mem_info("test", "k1")`)
	require.NoError(t, err)

	m := result.(*slop.MapValue)
	assert.Equal(t, "k1", mapGet(m, "key"))
	assert.Equal(t, "greeting", mapGet(m, "description"))

	size := mapGet(m, "size")
	assert.NotNil(t, size)

	createdAt := mapGet(m, "created_at")
	assert.NotNil(t, createdAt)
	updatedAt := mapGet(m, "updated_at")
	assert.NotNil(t, updatedAt)
}

func TestMemoryStore_InfoMissing(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	// Missing bank
	result, err := rt.Execute(`mem_info("missing", "k1")`)
	require.NoError(t, err)
	_, isNone := result.(*slop.NullValue)
	assert.True(t, isNone, "missing bank should return none")

	// Missing key in existing bank
	_, err = rt.Execute(`mem_save("test", "k1", 1)`)
	require.NoError(t, err)

	result, err = rt.Execute(`mem_info("test", "missing")`)
	require.NoError(t, err)
	_, isNone = result.(*slop.NullValue)
	assert.True(t, isNone, "missing key should return none")
}

func TestMemoryStore_List(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("test", "alpha", 1, description: "first")
mem_save("test", "beta", 2, description: "second")
mem_save("test", "gamma", 3)`)
	require.NoError(t, err)

	result, err := rt.Execute(`mem_list("test")`)
	require.NoError(t, err)

	list := result.(*slop.ListValue)
	assert.Equal(t, 3, len(list.Elements))

	// Results should be sorted by key
	first := list.Elements[0].(*slop.MapValue)
	assert.Equal(t, "alpha", mapGet(first, "key"))
}

func TestMemoryStore_ListWithPattern(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("test", "user_prefs", 1)
mem_save("test", "user_cache", 2)
mem_save("test", "system_log", 3)`)
	require.NoError(t, err)

	result, err := rt.Execute(`mem_list("test", pattern: "user_*")`)
	require.NoError(t, err)

	list := result.(*slop.ListValue)
	assert.Equal(t, 2, len(list.Elements))
}

func TestMemoryStore_Search(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("bank1", "user_prefs", 1)
mem_save("bank2", "user_cache", 2)
mem_save("bank2", "system_log", 3)`)
	require.NoError(t, err)

	// Cross-bank search by key name
	result, err := rt.Execute(`mem_search("user")`)
	require.NoError(t, err)

	list := result.(*slop.ListValue)
	assert.Equal(t, 2, len(list.Elements))
}

func TestMemoryStore_SearchByDescription(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("test", "k1", 1, description: "user preferences")
mem_save("test", "k2", 2, description: "system config")`)
	require.NoError(t, err)

	result, err := rt.Execute(`mem_search("preferences")`)
	require.NoError(t, err)

	list := result.(*slop.ListValue)
	assert.Equal(t, 1, len(list.Elements))

	m := list.Elements[0].(*slop.MapValue)
	assert.Equal(t, "k1", mapGet(m, "key"))
}

func TestMemoryStore_SearchByValue(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("test", "k1", {"theme": "dark"})
mem_save("test", "k2", {"theme": "light"})`)
	require.NoError(t, err)

	result, err := rt.Execute(`mem_search("dark", include_values: true)`)
	require.NoError(t, err)

	list := result.(*slop.ListValue)
	assert.Equal(t, 1, len(list.Elements))

	m := list.Elements[0].(*slop.MapValue)
	assert.Equal(t, "k1", mapGet(m, "key"))
	// include_values should include the value
	assert.NotNil(t, mapGetVal(m, "value"))
}

func TestMemoryStore_SearchSingleBank(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("bank1", "user_prefs", 1)
mem_save("bank2", "user_cache", 2)`)
	require.NoError(t, err)

	result, err := rt.Execute(`mem_search("user", bank: "bank1")`)
	require.NoError(t, err)

	list := result.(*slop.ListValue)
	assert.Equal(t, 1, len(list.Elements))

	m := list.Elements[0].(*slop.MapValue)
	assert.Equal(t, "bank1", mapGet(m, "bank"))
}

func TestMemoryStore_Size(t *testing.T) {
	rt, _ := newMemoryRuntime(t)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("test", "k1", "hello world")`)
	require.NoError(t, err)

	result, err := rt.Execute(`mem_info("test", "k1")`)
	require.NoError(t, err)

	m := result.(*slop.MapValue)
	size := mapGet(m, "size")
	// "hello world" serialized as JSON is `"hello world"` = 13 bytes
	assert.Equal(t, int64(13), size)
}

func TestMemoryStore_BackwardCompat(t *testing.T) {
	dir := t.TempDir()
	store := NewMemoryStoreWithDir(dir)

	// Write an old-format bank file (no description/schema/size)
	oldBank := map[string]any{
		"_meta": map[string]any{
			"version":    1,
			"created_at": time.Now().Format(time.RFC3339Nano),
			"updated_at": time.Now().Format(time.RFC3339Nano),
		},
		"entries": map[string]any{
			"old_key": map[string]any{
				"value":      "old_value",
				"created_at": time.Now().Format(time.RFC3339Nano),
				"updated_at": time.Now().Format(time.RFC3339Nano),
			},
		},
	}
	data, err := json.MarshalIndent(oldBank, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "compat.json"), data, 0644))

	rt := slop.NewRuntime()
	RegisterMemory(rt, store)
	defer rt.Close()

	// Load should work
	result, err := rt.Execute(`mem_load("compat", "old_key")`)
	require.NoError(t, err)
	assert.Equal(t, "old_value", result.(*slop.StringValue).Value)

	// Info should return graceful defaults
	result, err = rt.Execute(`mem_info("compat", "old_key")`)
	require.NoError(t, err)

	m := result.(*slop.MapValue)
	desc := mapGet(m, "description")
	assert.Equal(t, "", desc)

	size := mapGet(m, "size")
	assert.Equal(t, int64(0), size)

	// List should also work
	result, err = rt.Execute(`mem_list("compat")`)
	require.NoError(t, err)
	list := result.(*slop.ListValue)
	assert.Equal(t, 1, len(list.Elements))
}
