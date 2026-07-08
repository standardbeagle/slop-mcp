package filelock

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestLockSerializesReadModifyWrite runs many goroutines that each acquire the
// lock and do a read-modify-write increment of a shared counter file. Without
// mutual exclusion, concurrent load-modify-save loses updates; with the lock
// the final count must equal the number of increments.
func TestLockSerializesReadModifyWrite(t *testing.T) {
	dir := t.TempDir()
	counterPath := filepath.Join(dir, "counter.json")

	const workers = 20
	const perWorker = 10
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				unlock, err := Lock(counterPath)
				if err != nil {
					t.Errorf("Lock: %v", err)
					return
				}
				n := readCounter(t, counterPath)
				writeCounter(t, counterPath, n+1)
				if err := unlock(); err != nil {
					t.Errorf("unlock: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	got := readCounter(t, counterPath)
	if want := workers * perWorker; got != want {
		t.Errorf("counter = %d, want %d (lost updates -> lock not exclusive)", got, want)
	}
}

func readCounter(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return n
}

func writeCounter(t *testing.T, path string, n int) {
	t.Helper()
	data, _ := json.Marshal(n)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}
