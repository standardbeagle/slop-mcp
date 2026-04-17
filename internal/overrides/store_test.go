package overrides

import (
	"path/filepath"
	"testing"
)

func TestStore_WriteReadOverride(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(StoreOptions{UserRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	e := OverrideEntry{Description: "compressed", SourceHash: "h1"}
	if err := s.SetOverride(ScopeUser, "figma.get_file", e); err != nil {
		t.Fatal(err)
	}

	got, found := s.GetOverride("figma.get_file")
	if !found {
		t.Fatal("override not found after set")
	}
	if got.Description != "compressed" || got.Scope != ScopeUser {
		t.Errorf("got %+v", got)
	}
}

func TestStore_ScopePrecedence(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(StoreOptions{
		UserRoot:    filepath.Join(dir, "user"),
		ProjectRoot: filepath.Join(dir, "project"),
		LocalRoot:   filepath.Join(dir, "local"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_ = s.SetOverride(ScopeUser, "k", OverrideEntry{Description: "user"})
	_ = s.SetOverride(ScopeProject, "k", OverrideEntry{Description: "proj"})

	got, _ := s.GetOverride("k")
	if got.Description != "proj" || got.Scope != ScopeProject {
		t.Errorf("project should beat user: %+v", got)
	}

	_ = s.SetOverride(ScopeLocal, "k", OverrideEntry{Description: "loc"})
	got, _ = s.GetOverride("k")
	if got.Description != "loc" || got.Scope != ScopeLocal {
		t.Errorf("local should beat project: %+v", got)
	}
}

func TestStore_RemoveOverrideAllScopes(t *testing.T) {
	dir := t.TempDir()
	s, _ := OpenStore(StoreOptions{
		UserRoot:    filepath.Join(dir, "u"),
		ProjectRoot: filepath.Join(dir, "p"),
	})
	defer s.Close()

	_ = s.SetOverride(ScopeUser, "k", OverrideEntry{})
	_ = s.SetOverride(ScopeProject, "k", OverrideEntry{})

	n, err := s.RemoveOverride("", "k") // empty scope = all
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("removed %d, want 2", n)
	}
	if _, ok := s.GetOverride("k"); ok {
		t.Error("key should be gone")
	}
}

func TestStore_CustomToolRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, _ := OpenStore(StoreOptions{UserRoot: dir})
	defer s.Close()

	ct := CustomTool{
		Description: "batch create",
		InputSchema: map[string]any{"type": "object"},
		Body:        "emit(42)",
	}
	if err := s.SetCustom(ScopeUser, "my_tool", ct); err != nil {
		t.Fatal(err)
	}
	got, ok := s.GetCustom("my_tool")
	if !ok {
		t.Fatal("custom tool not found")
	}
	if got.Body != "emit(42)" || got.Scope != ScopeUser {
		t.Errorf("got %+v", got)
	}

	n, _ := s.RemoveCustom(ScopeUser, "my_tool")
	if n != 1 {
		t.Errorf("removed %d, want 1", n)
	}
}

func TestStore_CustomScopePrecedence(t *testing.T) {
	dir := t.TempDir()
	s, _ := OpenStore(StoreOptions{
		UserRoot:    filepath.Join(dir, "u"),
		ProjectRoot: filepath.Join(dir, "p"),
	})
	defer s.Close()

	_ = s.SetCustom(ScopeUser, "x", CustomTool{Body: "u"})
	_ = s.SetCustom(ScopeProject, "x", CustomTool{Body: "p"})

	got, _ := s.GetCustom("x")
	if got.Body != "p" {
		t.Errorf("project should beat user: %+v", got)
	}
}

func TestStore_PersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()

	s1, _ := OpenStore(StoreOptions{UserRoot: dir})
	_ = s1.SetOverride(ScopeUser, "k", OverrideEntry{Description: "persisted", SourceHash: "h"})
	_ = s1.Close() // must flush

	s2, err := OpenStore(StoreOptions{UserRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	got, ok := s2.GetOverride("k")
	if !ok {
		t.Fatal("override didn't persist")
	}
	if got.Description != "persisted" {
		t.Errorf("got %+v", got)
	}
}
