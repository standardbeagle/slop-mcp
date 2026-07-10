package debug

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyPatches(t *testing.T) {
	input := []byte(`{"failure":{"call":{"method":"wrong","args":[1]}}}`)
	out, err := ApplyPatches(input, []Patch{
		{Op: "test", Path: "/failure/call/method", Value: json.RawMessage(`"wrong"`)},
		{Op: "replace", Path: "/failure/call/method", Value: json.RawMessage(`"right"`)},
		{Op: "add", Path: "/failure/call/args/-", Value: json.RawMessage(`2`)},
		{Op: "remove", Path: "/failure/call/args/0"},
	}, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"failure":{"call":{"method":"right","args":[2]}}}`, string(out))
}

func TestApplyPatchesIsAtomicAndValidatesResult(t *testing.T) {
	input := []byte(`{"value":1}`)
	_, err := ApplyPatches(input, []Patch{{Op: "replace", Path: "/missing", Value: json.RawMessage(`2`)}}, nil)
	require.Error(t, err)
	assert.JSONEq(t, `{"value":1}`, string(input))

	_, err = ApplyPatches(input, []Patch{{Op: "replace", Path: "/value", Value: json.RawMessage(`2`)}}, func([]byte) error {
		return assert.AnError
	})
	assert.ErrorIs(t, err, assert.AnError)
}

func TestApplyPatchesRejectsInvalidPointersAndOperations(t *testing.T) {
	for _, patch := range []Patch{
		{Op: "move", Path: "/value"},
		{Op: "replace", Path: "value", Value: json.RawMessage(`2`)},
		{Op: "replace", Path: "/value", Value: json.RawMessage(`invalid`)},
	} {
		_, err := ApplyPatches([]byte(`{"value":1}`), []Patch{patch}, nil)
		require.Error(t, err)
	}
}
