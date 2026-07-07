package overrides

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRepoRoot_GitMarker(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	got, err := findRepoRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("findRepoRoot = %q, want %q", got, dir)
	}
}

func TestFindRepoRoot_SlopMcpMarker(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".slop-mcp.kdl"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := findRepoRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("findRepoRoot = %q, want %q", got, dir)
	}
}

func TestFindRepoRoot_NotFound(t *testing.T) {
	// Verifies findRepoRoot returns ErrNoRepo when it walks to / without
	// finding a marker. TempDir ancestors are outside our control (e.g. a
	// stray /tmp/.git), so skip rather than fail if one carries a marker.
	dir := t.TempDir()
	for d := filepath.Dir(dir); ; d = filepath.Dir(d) {
		for _, m := range []string{".git", ".slop-mcp.kdl"} {
			if _, err := os.Stat(filepath.Join(d, m)); err == nil {
				t.Skipf("ancestor %s contains %s; environment not hermetic", d, m)
			}
		}
		if d == filepath.Dir(d) {
			break
		}
	}
	if _, err := findRepoRoot(dir); err == nil {
		t.Error("expected error when no repo marker is present")
	}
}

func TestMergeScopes_LocalBeatsProjectBeatsUser(t *testing.T) {
	userE := OverrideEntry{Description: "u", Scope: ScopeUser}
	projE := OverrideEntry{Description: "p", Scope: ScopeProject}
	locE := OverrideEntry{Description: "l", Scope: ScopeLocal}

	got := MergeOverride(map[Scope]*OverrideEntry{
		ScopeUser:    &userE,
		ScopeProject: &projE,
		ScopeLocal:   &locE,
	})
	if got.Description != "l" || got.Scope != ScopeLocal {
		t.Errorf("merge = %+v, want local winner", got)
	}

	got2 := MergeOverride(map[Scope]*OverrideEntry{
		ScopeUser:    &userE,
		ScopeProject: &projE,
	})
	if got2.Description != "p" || got2.Scope != ScopeProject {
		t.Errorf("merge = %+v, want project winner", got2)
	}
}
