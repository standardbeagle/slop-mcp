package atomicfile

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteFile_CreatesWithContentAndPerm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.txt")

	if err := WriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q, want %q", got, "hello")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("perm = %o, want 600", info.Mode().Perm())
	}
}

func TestWriteFile_OverwriteReplacesAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.txt")

	if err := WriteFile(path, []byte("first"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("second"), 0644); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "second" {
		t.Errorf("content = %q, want %q", got, "second")
	}

	// No leftover temp files in the directory.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory has %d entries %v, want 1", len(entries), names)
	}
}

// TestWriteFile_ConcurrentSamePath ensures unique temp names let concurrent
// writers to the same path finish without error and leave a valid file.
func TestWriteFile_ConcurrentSamePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.txt")

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := WriteFile(path, []byte("payload"), 0644); err != nil {
				t.Errorf("concurrent WriteFile: %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("content = %q, want %q", got, "payload")
	}
}
