package overrides

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExport_PerMCPSelector(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(StoreOptions{UserRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	_ = s.SetOverride(ScopeUser, "figma.get_file", OverrideEntry{Description: "f1", SourceHash: "h1"})
	_ = s.SetOverride(ScopeUser, "github.search", OverrideEntry{Description: "g1", SourceHash: "h2"})
	_ = s.SetCustom(ScopeUser, "figma_helper", CustomTool{
		Body:      "",
		DependsOn: []Dependency{{MCP: "figma", Tool: "get_file", Hash: "h1"}},
	})

	pack, err := s.Export(Selector{MCP: "figma"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Overrides) != 1 || pack.Overrides[0].Key != "figma.get_file" {
		t.Errorf("expected single figma override: %+v", pack.Overrides)
	}
	if len(pack.CustomTools) != 1 || pack.CustomTools[0].Name != "figma_helper" {
		t.Errorf("expected figma_helper included by dep: %+v", pack.CustomTools)
	}
}

func TestExport_KeysGlob(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(StoreOptions{UserRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	_ = s.SetOverride(ScopeUser, "figma.get_file", OverrideEntry{SourceHash: "h"})
	_ = s.SetOverride(ScopeUser, "figma.list_files", OverrideEntry{SourceHash: "h"})
	_ = s.SetOverride(ScopeUser, "github.search", OverrideEntry{SourceHash: "h"})

	pack, err := s.Export(Selector{Keys: []string{"figma.*"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Overrides) != 2 {
		t.Errorf("glob should match 2 keys, got %d", len(pack.Overrides))
	}
}

func TestImport_RoundTrip(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	s1, err := OpenStore(StoreOptions{UserRoot: dir1})
	if err != nil {
		t.Fatal(err)
	}
	_ = s1.SetOverride(ScopeUser, "figma.get_file", OverrideEntry{Description: "short", SourceHash: "h"})
	_ = s1.SetCustom(ScopeUser, "my_tool", CustomTool{
		Description: "t", InputSchema: map[string]any{"type": "object"}, Body: "1",
	})

	pack, err := s1.Export(Selector{})
	if err != nil {
		t.Fatal(err)
	}
	_ = s1.Close()

	s2, err := OpenStore(StoreOptions{UserRoot: dir2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s2.Close() })

	report, err := s2.Import(pack, ScopeUser, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.ImportedOverrides != 1 || report.ImportedCustom != 1 {
		t.Errorf("import counts wrong: %+v", report)
	}

	got, ok := s2.GetOverride("figma.get_file")
	if !ok || got.Description != "short" {
		t.Errorf("override not imported: %+v", got)
	}
}

func TestImport_RejectsUnknownSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(StoreOptions{UserRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	var pack Pack
	_ = json.Unmarshal([]byte(`{"schema_version":999}`), &pack)
	_, err = s.Import(pack, ScopeUser, false)
	if err == nil || !strings.Contains(err.Error(), "schema") {
		t.Errorf("expected schema error, got %v", err)
	}
}

func TestImport_CollisionReportedWhenNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(StoreOptions{UserRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	_ = s.SetOverride(ScopeUser, "k", OverrideEntry{Description: "existing", SourceHash: "h"})

	pack := Pack{
		SchemaVersion: PackSchemaVersion,
		Overrides:     []PackOverride{{Key: "k", Description: "incoming", SourceHash: "h"}},
	}
	report, err := s.Import(pack, ScopeUser, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.ImportedOverrides != 0 {
		t.Errorf("collision should skip import")
	}
	if len(report.Skipped) != 1 || report.Skipped[0] != "k" {
		t.Errorf("collision not reported: %+v", report)
	}
}

func TestExport_ScopeFilter(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(StoreOptions{UserRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	_ = s.SetOverride(ScopeUser, "a.b", OverrideEntry{Description: "user", SourceHash: "h1"})

	pack, err := s.Export(Selector{Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Overrides) != 1 {
		t.Errorf("expected 1 override from user scope, got %d", len(pack.Overrides))
	}

	// Export with project scope should yield nothing (no project root configured).
	pack2, err := s.Export(Selector{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if len(pack2.Overrides) != 0 {
		t.Errorf("expected 0 overrides from empty project scope, got %d", len(pack2.Overrides))
	}
}

func TestImport_OverwriteReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(StoreOptions{UserRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	_ = s.SetOverride(ScopeUser, "k", OverrideEntry{Description: "old", SourceHash: "h1"})

	pack := Pack{
		SchemaVersion: PackSchemaVersion,
		Overrides:     []PackOverride{{Key: "k", Description: "new", SourceHash: "h2"}},
	}
	report, err := s.Import(pack, ScopeUser, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.ImportedOverrides != 1 || len(report.Skipped) != 0 {
		t.Errorf("overwrite should import: %+v", report)
	}

	got, ok := s.GetOverride("k")
	if !ok || got.Description != "new" {
		t.Errorf("override not replaced: %+v", got)
	}
}
