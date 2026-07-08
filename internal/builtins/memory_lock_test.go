package builtins

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestMemoryStore_ConcurrentLockedWrites simulates mem_save's locked
// load-modify-write from many goroutines writing distinct keys to the same
// bank, verifying the store.mu + filelock combination loses no entries.
func TestMemoryStore_ConcurrentLockedWrites(t *testing.T) {
	store := NewMemoryStoreWithDir(t.TempDir())

	const workers = 24
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", n)

			store.mu.Lock()
			unlock, err := store.lockBank("bank")
			if err != nil {
				store.mu.Unlock()
				t.Errorf("lockBank: %v", err)
				return
			}
			b, err := store.loadBank("bank")
			if err != nil {
				_ = unlock()
				store.mu.Unlock()
				t.Errorf("loadBank: %v", err)
				return
			}
			if b == nil {
				b = &memoryBank{
					Meta:    memoryBankMeta{Version: 1, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)},
					Entries: map[string]*memoryEntry{},
				}
			}
			b.Entries[key] = &memoryEntry{Value: n}
			if err := store.saveBank("bank", b); err != nil {
				t.Errorf("saveBank: %v", err)
			}
			_ = unlock()
			store.mu.Unlock()
		}(i)
	}
	wg.Wait()

	final, err := store.loadBank("bank")
	if err != nil {
		t.Fatalf("final load: %v", err)
	}
	if final == nil {
		t.Fatal("bank missing after writes")
	}
	if len(final.Entries) != workers {
		t.Errorf("bank has %d entries, want %d (lost updates)", len(final.Entries), workers)
	}
}
