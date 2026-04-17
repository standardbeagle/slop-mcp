package overrides

import "testing"

func TestCanonicalJSON_SortsKeys(t *testing.T) {
	out, err := canonicalJSON(map[string]string{"b": "2", "a": "1"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":"1","b":"2"}`
	if string(out) != want {
		t.Errorf("canonicalJSON = %s, want %s", out, want)
	}
}

func TestCanonicalJSON_NestedSorts(t *testing.T) {
	in := map[string]any{
		"z": map[string]any{"y": 1, "x": 2},
		"a": []int{3, 1, 2},
	}
	out, err := canonicalJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	// Arrays preserve order; map keys are sorted at every level.
	want := `{"a":[3,1,2],"z":{"x":2,"y":1}}`
	if string(out) != want {
		t.Errorf("canonicalJSON = %s, want %s", out, want)
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	params := map[string]string{"q": "query", "a": "anchor"}
	h1 := ComputeHash("desc", params)
	h2 := ComputeHash("desc", map[string]string{"a": "anchor", "q": "query"})
	if h1 != h2 {
		t.Errorf("hash should be deterministic across map ordering: %s vs %s", h1, h2)
	}
	if len(h1) != 16 {
		t.Errorf("hash len = %d, want 16", len(h1))
	}
}

func TestComputeHash_Sensitive(t *testing.T) {
	base := ComputeHash("desc", map[string]string{"a": "b"})
	diffDesc := ComputeHash("desc2", map[string]string{"a": "b"})
	diffParams := ComputeHash("desc", map[string]string{"a": "c"})
	if base == diffDesc || base == diffParams {
		t.Error("hash must differ on description or params change")
	}
}
