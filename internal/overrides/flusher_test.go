package overrides

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestFlusher_CoalescesBursts(t *testing.T) {
	var writeCount int32
	f := newFlusher("test", func() error {
		atomic.AddInt32(&writeCount, 1)
		time.Sleep(10 * time.Millisecond) // simulate disk
		return nil
	})
	defer f.close()

	// Burst of 100 dirty signals; flusher must coalesce.
	for i := 0; i < 100; i++ {
		f.markDirty()
	}
	// Give flusher time to run pending writes.
	time.Sleep(200 * time.Millisecond)

	got := atomic.LoadInt32(&writeCount)
	if got < 1 || got > 5 {
		t.Errorf("write count = %d, want 1..5 (coalesced)", got)
	}
}

func TestFlusher_ShutdownFlushesPending(t *testing.T) {
	var writeCount int32
	f := newFlusher("test", func() error {
		atomic.AddInt32(&writeCount, 1)
		return nil
	})
	f.markDirty()
	f.close() // Blocks until pending drain.

	if atomic.LoadInt32(&writeCount) < 1 {
		t.Error("shutdown should flush pending")
	}
}
