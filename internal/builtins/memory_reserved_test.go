package builtins

import (
	"os"
	"strings"
	"testing"
)

func TestMemSave_RejectsReservedBank(t *testing.T) {
	dir := t.TempDir()
	store := NewMemoryStoreWithDir(dir)
	rt := NewRuntime()
	RegisterMemory(rt, store)
	defer rt.Close()

	_, err := rt.Execute(`mem_save("_slop.overrides", "foo.bar", "v")`)
	if err == nil {
		t.Fatal("expected error saving to reserved bank")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Errorf("error should mention reserved: %v", err)
	}
}

func TestMemDelete_RejectsReservedBank(t *testing.T) {
	dir := t.TempDir()
	store := NewMemoryStoreWithDir(dir)
	rt := NewRuntime()
	RegisterMemory(rt, store)
	defer rt.Close()

	_, err := rt.Execute(`mem_delete("_slop.overrides", "foo.bar")`)
	if err == nil {
		t.Fatal("expected error deleting from reserved bank")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Errorf("error should mention reserved: %v", err)
	}
}

func TestMemReadEntryPoints_RejectReservedBank(t *testing.T) {
	dir := t.TempDir()
	store := NewMemoryStoreWithDir(dir)
	rt := NewRuntime()
	RegisterMemory(rt, store)
	defer rt.Close()

	scripts := map[string]string{
		"mem_load":   `mem_load("_slop.overrides", "foo")`,
		"mem_keys":   `mem_keys("_slop.overrides")`,
		"mem_info":   `mem_info("_slop.overrides", "foo")`,
		"mem_list":   `mem_list("_slop.overrides")`,
		"mem_search": `mem_search("foo", bank: "_slop.overrides")`,
	}
	for name, script := range scripts {
		_, err := rt.Execute(script)
		if err == nil {
			t.Errorf("%s: expected error for reserved bank", name)
			continue
		}
		if !strings.Contains(err.Error(), "reserved") {
			t.Errorf("%s: error should mention reserved: %v", name, err)
		}
	}
}

func TestMemEntryPoints_RejectPathTraversalBankNames(t *testing.T) {
	dir := t.TempDir()
	store := NewMemoryStoreWithDir(dir)
	rt := NewRuntime()
	RegisterMemory(rt, store)
	defer rt.Close()

	bad := `../../evil`
	scripts := map[string]string{
		"mem_save":   `mem_save("` + bad + `", "k", "v")`,
		"mem_load":   `mem_load("` + bad + `", "k")`,
		"mem_delete": `mem_delete("` + bad + `", "k")`,
		"mem_keys":   `mem_keys("` + bad + `")`,
		"mem_info":   `mem_info("` + bad + `", "k")`,
		"mem_list":   `mem_list("` + bad + `")`,
		"mem_search": `mem_search("k", bank: "` + bad + `")`,
	}
	for name, script := range scripts {
		_, err := rt.Execute(script)
		if err == nil {
			t.Errorf("%s: expected error for path traversal bank name", name)
			continue
		}
		if !strings.Contains(err.Error(), "invalid bank name") {
			t.Errorf("%s: error should mention invalid bank name: %v", name, err)
		}
	}

	// Nothing should have been written outside the store dir.
	if _, err := os.Stat(dir + "/../../evil.json"); err == nil {
		t.Error("path traversal wrote a file outside the memory dir")
	}
}

func TestValidateBankName(t *testing.T) {
	valid := []string{"notes", "my-bank", "a1_b2", "z"}
	for _, name := range valid {
		if err := ValidateBankName(name); err != nil {
			t.Errorf("expected %q valid: %v", name, err)
		}
	}
	invalid := []string{"", "_hidden", "Upper", "1num", "has space", "dot.name", "../evil", "a/b", strings.Repeat("a", 65)}
	for _, name := range invalid {
		if err := ValidateBankName(name); err == nil {
			t.Errorf("expected %q invalid", name)
		}
	}
}
