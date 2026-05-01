package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

type stubTools struct {
	specs   []ToolSpec
	calls   map[string]json.RawMessage
	results map[string]string
	errs    map[string]error
}

func (s *stubTools) List(_ context.Context, allow []string) []ToolSpec {
	if len(allow) == 0 {
		return s.specs
	}
	keep := make([]ToolSpec, 0, len(allow))
	for _, sp := range s.specs {
		for _, a := range allow {
			if sp.Name == a {
				keep = append(keep, sp)
			}
		}
	}
	return keep
}

func (s *stubTools) Execute(_ context.Context, name string, args json.RawMessage) (string, bool, error) {
	if s.calls == nil {
		s.calls = map[string]json.RawMessage{}
	}
	s.calls[name] = args
	if err, ok := s.errs[name]; ok {
		// Tool-level error: tool ran but reported failure. err is the
		// textual reason which the stub also surfaces in result so loop
		// tests can inspect what the model would see.
		return err.Error(), true, nil
	}
	if r, ok := s.results[name]; ok {
		return r, false, nil
	}
	// Infrastructure failure: name not registered.
	return "", false, fmt.Errorf("tool not found: %s", name)
}

func TestStubTools_FilterByWhitelist(t *testing.T) {
	s := &stubTools{specs: []ToolSpec{{Name: "a"}, {Name: "b"}, {Name: "c"}}}
	got := s.List(context.Background(), []string{"a", "c"})
	if len(got) != 2 || got[0].Name != "a" || got[1].Name != "c" {
		t.Fatalf("filter mismatch: %+v", got)
	}
}

func TestStubTools_NilAllowReturnsAll(t *testing.T) {
	s := &stubTools{specs: []ToolSpec{{Name: "a"}, {Name: "b"}, {Name: "c"}}}
	got := s.List(context.Background(), nil)
	if len(got) != 3 {
		t.Fatalf("nil allow should return all, got %d: %+v", len(got), got)
	}
	got = s.List(context.Background(), []string{})
	if len(got) != 3 {
		t.Fatalf("empty allow should return all, got %d: %+v", len(got), got)
	}
}
