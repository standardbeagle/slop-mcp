package overrides

import (
	"errors"
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
	defer func() { _ = f.close() }()

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
	if err := f.close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if atomic.LoadInt32(&writeCount) < 1 {
		t.Error("shutdown should flush pending")
	}
}

func TestFlusher_CloseReturnsPendingWriteError(t *testing.T) {
	wantErr := errors.New("disk full")
	f := newFlusher("test", func() error {
		return wantErr
	})
	f.markDirty()

	err := f.close()
	if err == nil {
		t.Fatal("expected close error")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("close error = %v, want wrapped %v", err, wantErr)
	}
}
