package builtins

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/llm"
	"github.com/standardbeagle/slop/pkg/slop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProvider struct{ name string }

func (f *fakeProvider) Name() string                     { return f.name }
func (f *fakeProvider) Available(_ context.Context) bool { return true }
func (f *fakeProvider) UnavailableReason() string        { return "" }
func (f *fakeProvider) Sample(_ context.Context, _ *llm.Request, _ func(llm.StreamEvent)) (*llm.Response, error) {
	return &llm.Response{Content: "answer", StopReason: llm.StopReasonEndTurn, Model: "fake"}, nil
}

type emptyHost struct{}

func (emptyHost) List(_ context.Context, _ []string) []llm.ToolSpec { return nil }
func (emptyHost) Execute(_ context.Context, name string, _ json.RawMessage) (string, bool, error) {
	return "", false, assert.AnError
}

func newLLMRuntime(t *testing.T) *slop.Runtime {
	t.Helper()
	rt := slop.NewRuntime()
	r := llm.NewRegistry()
	r.RegisterProvider(&fakeProvider{name: "fake"})
	r.SetChain([]string{"fake"})
	r.RegisterAgent(&llm.AgentDef{Name: "test", Backend: "fake"})
	RegisterLLM(rt, r, emptyHost{}, nil)
	return rt
}

func TestLLM_BasicCall(t *testing.T) {
	rt := newLLMRuntime(t)
	defer rt.Close()
	v, err := rt.Execute(`llm("hi", agent: "test")`)
	require.NoError(t, err)
	assert.Equal(t, "answer", v.(*slop.StringValue).Value)
}

func TestLLM_LastReturnsMetadata(t *testing.T) {
	rt := newLLMRuntime(t)
	defer rt.Close()
	_, err := rt.Execute(`llm("hi", agent: "test")`)
	require.NoError(t, err)
	v, err := rt.Execute(`llm_last()`)
	require.NoError(t, err)
	m := v.(*slop.MapValue)
	model, _ := m.Get("model")
	assert.Equal(t, "fake", model.(*slop.StringValue).Value)
	prov, _ := m.Get("provider")
	assert.Equal(t, "fake", prov.(*slop.StringValue).Value)
}

func TestLLM_LastNilBeforeCall(t *testing.T) {
	rt := newLLMRuntime(t)
	defer rt.Close()
	v, err := rt.Execute(`llm_last()`)
	require.NoError(t, err)
	_, ok := v.(*slop.NullValue)
	assert.True(t, ok, "expected NullValue before any llm() call")
}

func TestLLM_UnknownAgent(t *testing.T) {
	rt := newLLMRuntime(t)
	defer rt.Close()
	v, err := rt.Execute(`llm("hi", agent: "nonexistent")`)
	require.NoError(t, err)
	_, ok := v.(*slop.ErrorValue)
	assert.True(t, ok, "expected ErrorValue for unknown agent")
}
