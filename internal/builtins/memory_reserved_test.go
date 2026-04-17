package builtins

import (
	"strings"
	"testing"

	"github.com/standardbeagle/slop/pkg/slop"
)

func TestMemSave_RejectsReservedBank(t *testing.T) {
	dir := t.TempDir()
	store := NewMemoryStoreWithDir(dir)
	rt := slop.NewRuntime()
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
	rt := slop.NewRuntime()
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
